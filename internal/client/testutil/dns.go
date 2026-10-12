package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/dns"
)

// NewDNSManager uses isolated resolver files and inert Linux commands. It
// exercises the real platform manager without accessing the host's DNS setup.
func NewDNSManager(
	t *testing.T,
) *dns.Manager {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".opencode-tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "client-dns-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })

	indices := make(map[string]int)
	manager, err := dns.New(dns.Options{
		ResolverDir: dir,
		Run: func(ctx context.Context, _ string, _ ...string) ([]byte, error) {
			return nil, ctx.Err()
		},
		LookupInterface: func(name string) (int, error) {
			if indices[name] == 0 {
				indices[name] = len(indices) + 1
			}
			return indices[name], nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}
