package runtime_test

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/dns"
	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestRun_RecoversResolverFilesAfterAbnormalExit(
	t *testing.T,
) {
	// Leave files behind as a terminated process would, without calling Remove.
	directory := restartResolverDirectory(t)
	previous, err := dns.New(dns.Options{ResolverDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"active", "disabled", "uninstalled"} {
		config := dns.Config{
			Network:    name,
			Domain:     name + ".internal",
			Nameserver: net.ParseIP("192.0.2.53"),
			Interface:  "utun9",
		}
		if err := previous.Apply(t.Context(), config); err != nil {
			t.Fatal(err)
		}
	}
	userPath := filepath.Join(directory, "personal.internal")
	userContent := "nameserver 192.0.2.54\n"
	if err := os.WriteFile(userPath, []byte(userContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Saved intent contains one enabled network and one disabled network.
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "active")
	testutil.SeedNetworkDirect(t, env.Database, "disabled")
	if err := env.Service.SetNetworkEnabled("active", true); err != nil {
		t.Fatal(err)
	}
	restarted, err := dns.New(dns.Options{ResolverDir: directory})
	if err != nil {
		t.Fatal(err)
	}
	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("tunnel temporarily unavailable")
	rt := newRuntime(t, env, backend, runtime.Options{
		DNS:      restarted,
		Interval: time.Hour,
	})
	stop := testutil.RunRuntime(t, rt)
	if _, err := rt.GetStatus(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Startup cleanup preserves active domains even before their tunnels return.
	activePath := filepath.Join(directory, "active.internal")
	content, err := os.ReadFile(activePath)
	if err != nil || !strings.Contains(string(content), "nameserver 192.0.2.53\n") {
		t.Fatalf("active resolver was not preserved: %q, %v", content, err)
	}
	for _, name := range []string{"disabled", "uninstalled"} {
		path := filepath.Join(directory, name+".internal")
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("abandoned resolver %s survived: %v", name, err)
		}
	}

	// Repair rewrites the active resolver from current saved configuration.
	backend.CreateErr = nil
	if err := rt.Converge(t.Context(), "active"); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(activePath)
	if err != nil || !strings.Contains(string(content), "nameserver 10.42.0.1\n") {
		t.Fatalf("active resolver was not repaired: %q, %v", content, err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(activePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clean shutdown left active resolver: %v", err)
	}
	content, err = os.ReadFile(userPath)
	if err != nil || string(content) != userContent {
		t.Fatalf("user-owned resolver changed: %q, %v", content, err)
	}
}

func restartResolverDirectory(
	t *testing.T,
) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".opencode-tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	directory, err := os.MkdirTemp(root, "dns-restart-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(directory) })
	return directory
}
