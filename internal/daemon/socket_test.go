package daemon_test

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

func TestDefaultSocketMode(t *testing.T) {
	if daemon.DefaultSocketMode != 0660 {
		t.Fatalf("default socket mode = %#o, want %#o", daemon.DefaultSocketMode, 0660)
	}
}

func TestListenUnix_RejectsActiveDaemon(t *testing.T) {
	path := tempSocketPath(t)
	first, err := daemon.ListenUnix(path, daemon.DefaultSocketMode)
	if err != nil {
		t.Fatalf("first listen: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })

	second, err := daemon.ListenUnix(path, daemon.DefaultSocketMode)
	if second != nil {
		_ = second.Close()
		t.Fatal("second listener replaced the active socket")
	}
	if !errors.Is(err, daemon.ErrAlreadyRunning) {
		t.Fatalf("second listen: got %v, want ErrAlreadyRunning", err)
	}

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("active socket was disturbed: %v", err)
	}
	_ = conn.Close()
}

func TestListenUnix_CreatesProtectedDirectory(t *testing.T) {
	root := tempDirectory(t)
	dir := filepath.Join(root, "runtime")
	path := filepath.Join(dir, "daemon.sock")

	ln, err := daemon.ListenUnix(path, daemon.DefaultSocketMode)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat runtime directory: %v", err)
	}
	if got := info.Mode().Perm(); got != 0755 {
		t.Fatalf("runtime directory permissions = %#o, want 0755", got)
	}
}

func TestListenUnix_AppliesSocketMode(t *testing.T) {
	tests := []struct {
		name string
		mode os.FileMode
		want os.FileMode
	}{
		{name: "default", want: daemon.DefaultSocketMode},
		{name: "owner", mode: 0600, want: 0600},
		{name: "group", mode: 0660, want: 0660},
		{name: "local users", mode: 0666, want: 0666},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tempSocketPath(t)
			ln, err := daemon.ListenUnix(path, tt.mode)
			if err != nil {
				t.Fatalf("listen: %v", err)
			}
			t.Cleanup(func() { _ = ln.Close() })

			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat socket: %v", err)
			}
			if got := info.Mode().Perm(); got != tt.want {
				t.Fatalf("socket permissions = %#o, want %#o", got, tt.want)
			}
		})
	}
}

func TestParseSocketMode(t *testing.T) {
	tests := []struct {
		value string
		want  os.FileMode
		ok    bool
	}{
		{value: "0600", want: 0600, ok: true},
		{value: "0660", want: 0660, ok: true},
		{value: "0666", want: 0666, ok: true},
		{value: "0644"},
		{value: "invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := daemon.ParseSocketMode(tt.value)
			if tt.ok && err != nil {
				t.Fatalf("ParseSocketMode: %v", err)
			}
			if !tt.ok && err == nil {
				t.Fatal("ParseSocketMode unexpectedly succeeded")
			}
			if got != tt.want {
				t.Fatalf("mode = %#o, want %#o", got, tt.want)
			}
		})
	}
}

func TestListenUnix_ReplacesStaleSocket(t *testing.T) {
	path := tempSocketPath(t)
	stale, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("create stale socket: %v", err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	if err := stale.Close(); err != nil {
		t.Fatalf("close stale socket: %v", err)
	}

	ln, err := daemon.ListenUnix(path, daemon.DefaultSocketMode)
	if err != nil {
		t.Fatalf("replace stale socket: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if got := info.Mode().Perm(); got != daemon.DefaultSocketMode {
		t.Fatalf("socket permissions = %#o, want %#o", got, daemon.DefaultSocketMode)
	}
}

func TestListenUnix_RefusesNonSocketPath(t *testing.T) {
	path := tempSocketPath(t)
	if err := os.WriteFile(path, []byte("keep me"), 0600); err != nil {
		t.Fatalf("write sentinel: %v", err)
	}

	ln, err := daemon.ListenUnix(path, daemon.DefaultSocketMode)
	if ln != nil {
		_ = ln.Close()
		t.Fatal("listener replaced a non-socket path")
	}
	if err == nil {
		t.Fatal("expected an error for a non-socket path")
	}

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read sentinel: %v", err)
	}
	if string(contents) != "keep me" {
		t.Fatalf("sentinel contents = %q, want unchanged", contents)
	}
}

func tempSocketPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(tempDirectory(t), "daemon.sock")
}

func tempDirectory(t *testing.T) string {
	t.Helper()

	dir, err := os.MkdirTemp("", "cord-")
	if err != nil {
		t.Fatalf("create temporary socket directory: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}
