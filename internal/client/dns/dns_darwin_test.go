package dns_test

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/dns"
)

const marker = "# Managed by Cord DNS resolver v1.\n" +
	"# Cord may overwrite or remove this file while this marker is present.\n" +
	"# Remove or modify this marker to make Cord disown the file.\n"

func newFileManager(
	t *testing.T,
	root string,
) *dns.Manager {
	t.Helper()
	manager, err := dns.New(dns.Options{ResolverDir: root})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func readFile(
	t *testing.T,
	path string,
) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestApply_WritesCanonicalFileAndReplacesAtomically(
	t *testing.T,
) {
	root := resolverRoot(t)
	manager := newFileManager(t, root)
	config := testConfig()
	path := filepath.Join(root, config.Domain)
	want := marker + "nameserver 10.0.0.1\nport 53\n" +
		"domain example.internal\nsearch example.internal\n"

	for range 2 {
		if err := manager.Apply(t.Context(), config); err != nil {
			t.Fatal(err)
		}
		if content := readFile(t, path); content != want {
			t.Fatalf("resolver content = %q, want %q", content, want)
		}
	}

	// An existing reader retains the old complete file during replacement.
	oldReader, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer oldReader.Close()
	config.Nameserver = net.ParseIP("fd00::1")
	if err := manager.Apply(t.Context(), config); err != nil {
		t.Fatal(err)
	}
	oldContent, err := io.ReadAll(oldReader)
	if err != nil || string(oldContent) != want {
		t.Fatalf("old reader = %q, %v", oldContent, err)
	}
	wantReplacement := strings.Replace(want, "10.0.0.1", "fd00::1", 1)
	if content := readFile(t, path); content != wantReplacement {
		t.Fatalf("replacement = %q, want %q", content, wantReplacement)
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temporary files left behind: %v, %v", entries, err)
	}
}

func TestApply_RespectsUnownedFilesAndSymlinks(
	t *testing.T,
) {
	for _, kind := range []string{"unmarked", "modified marker", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := resolverRoot(t)
			manager := newFileManager(t, root)
			config := testConfig()
			path := filepath.Join(root, config.Domain)
			content := "nameserver 192.0.2.1\n"
			if kind == "modified marker" {
				content = strings.Replace(marker, "v1.", "v2.", 1) + content
			}
			if kind == "symlink" {
				target := filepath.Join(resolverRoot(t), "outside")
				content = marker + content
				if err := os.WriteFile(target, []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}

			if err := manager.Apply(t.Context(), config); !errors.Is(err, dns.ErrConflict) {
				t.Fatalf("Apply = %v, want ownership conflict", err)
			}
			if err := manager.Remove(t.Context(), config); err != nil {
				t.Fatal(err)
			}
			if err := manager.CleanupStale(t.Context(), nil); err != nil {
				t.Fatal(err)
			}
			if got := readFile(t, path); got != content {
				t.Fatalf("unowned file changed: %q", got)
			}
		})
	}
}

func TestCleanupStale_PreservesActiveDomainsAndRemovesAbandonedFiles(
	t *testing.T,
) {
	root := resolverRoot(t)
	manager := newFileManager(t, root)
	for _, name := range []string{"alpha", "beta", "abandoned"} {
		config := testConfig()
		config.Network = name
		config.Domain = name + ".internal"
		if err := manager.Apply(t.Context(), config); err != nil {
			t.Fatal(err)
		}
	}

	// A fresh manager models process restart without an in-memory manifest.
	restarted := newFileManager(t, root)
	active := map[string]bool{
		"alpha.internal": true,
		"beta.internal":  true,
	}
	if err := restarted.CleanupStale(t.Context(), active); err != nil {
		t.Fatal(err)
	}
	for domain := range active {
		if content := readFile(t, filepath.Join(root, domain)); !strings.HasPrefix(content, marker) {
			t.Fatalf("active resolver was changed: %s", domain)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "abandoned.internal")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("abandoned resolver survived: %v", err)
	}
}

func TestRemove_DisownedFileSurvivesAndMissingFileSucceeds(
	t *testing.T,
) {
	root := resolverRoot(t)
	manager := newFileManager(t, root)
	config := testConfig()
	if err := manager.Apply(t.Context(), config); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, config.Domain)
	disowned := "# User-owned now.\nnameserver 192.0.2.1\n"
	if err := os.WriteFile(path, []byte(disowned), 0644); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(t.Context(), config); err != nil {
		t.Fatal(err)
	}
	if content := readFile(t, path); content != disowned {
		t.Fatalf("disowned content changed: %q", content)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := manager.Remove(t.Context(), config); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRemove_OwnedFileIsRemovedIdempotently(
	t *testing.T,
) {
	root := resolverRoot(t)
	manager := newFileManager(t, root)
	config := testConfig()
	if err := manager.Apply(t.Context(), config); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := manager.Remove(t.Context(), config); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(root, config.Domain)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("owned resolver survived removal: %v", err)
	}
}

func TestRemove_FailedRemovalCanBeRecoveredAfterRestart(
	t *testing.T,
) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permissions")
	}
	root := resolverRoot(t)
	manager := newFileManager(t, root)
	config := testConfig()
	if err := manager.Apply(t.Context(), config); err != nil {
		t.Fatal(err)
	}

	// A read-only directory permits ownership inspection but denies unlink.
	if err := os.Chmod(root, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(root, 0700) })
	if err := manager.Remove(t.Context(), config); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("Remove = %v, want permission failure", err)
	}
	path := filepath.Join(root, config.Domain)
	if content := readFile(t, path); !strings.HasPrefix(content, marker) {
		t.Fatal("failed removal lost ownership marker")
	}

	// A fresh process can finish cleanup using only the file marker.
	if err := os.Chmod(root, 0700); err != nil {
		t.Fatal(err)
	}
	restarted := newFileManager(t, root)
	if err := restarted.CleanupStale(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale resolver survived recovery: %v", err)
	}
}
