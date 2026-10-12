package dns_test

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/dns"
)

func testConfig() dns.Config {
	return dns.Config{
		Network:    "example",
		Domain:     "example.internal",
		Nameserver: net.ParseIP("10.0.0.1"),
		Interface:  "cord-example",
	}
}

func resolverRoot(
	t *testing.T,
) string {
	t.Helper()
	base, err := filepath.Abs("../../../.opencode-tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(base, 0755); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(base, "dns-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(root); err != nil {
			t.Errorf("remove test resolver directory: %v", err)
		}
	})
	return root
}

func TestNew_ValidatesOptionsWithoutSideEffects(
	t *testing.T,
) {
	for _, opts := range []dns.Options{
		{Timeout: -time.Second},
		{ResolverDir: "relative"},
	} {
		manager, err := dns.New(opts)
		if !errors.Is(err, dns.ErrInvalidConfig) || manager != nil {
			t.Fatalf("New = %v, %v; want invalid options", manager, err)
		}
	}

	missingDirectory := filepath.Join(resolverRoot(t), "not-created")
	_, err := dns.New(dns.Options{
		ResolverDir: missingDirectory,
		Run: func(context.Context, string, ...string) ([]byte, error) {
			t.Fatal("constructor executed a command")
			return nil, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(missingDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("constructor touched resolver directory: %v", err)
	}
}

func TestApply_RejectsInvalidConfiguration(
	t *testing.T,
) {
	manager, err := dns.New(dns.Options{ResolverDir: resolverRoot(t)})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(*dns.Config)
	}{
		{"empty network", func(c *dns.Config) { c.Network = "" }},
		{"uppercase", func(c *dns.Config) { c.Network = "Example" }},
		{"path escape", func(c *dns.Config) { c.Network = "../outside" }},
		{"different domain", func(c *dns.Config) { c.Domain = "other.internal" }},
		{"missing nameserver", func(c *dns.Config) { c.Nameserver = nil }},
		{"missing interface", func(c *dns.Config) { c.Interface = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			config := testConfig()
			tc.change(&config)
			if err := manager.Apply(t.Context(), config); !errors.Is(err, dns.ErrInvalidConfig) {
				t.Fatalf("Apply = %v, want invalid configuration", err)
			}
			if err := manager.Remove(t.Context(), config); !errors.Is(err, dns.ErrInvalidConfig) {
				t.Fatalf("Remove = %v, want invalid configuration", err)
			}
		})
	}
}

func TestManager_CancelledOperationsDoNothing(
	t *testing.T,
) {
	manager, err := dns.New(dns.Options{ResolverDir: resolverRoot(t)})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	config := testConfig()

	for _, err := range []error{
		manager.Apply(ctx, config),
		manager.Remove(ctx, config),
		manager.CleanupStale(ctx, nil),
	} {
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("operation = %v, want cancellation", err)
		}
	}
}
