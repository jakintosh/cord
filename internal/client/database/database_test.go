package database_test

import (
	"os"
	"path/filepath"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
)

func TestOpen_ReadyForUse(
	t *testing.T,
) {
	opts := database.Options{
		Path: ":memory:",
	}
	db, err := database.Open(opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	testutil.SeedNetworkDirect(t, db, "testnet")
	if _, err := db.GetNetwork("testnet"); err != nil {
		t.Fatalf("read newly opened database: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := db.ListNetworks(); err == nil {
		t.Fatal("closed database accepted a read")
	}
}

func TestOpen_ReopenPreservesStateWithWAL(
	t *testing.T,
) {
	root := filepath.Join("..", "..", "..", ".opencode-tmp")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(root, "client-database-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	opts := database.Options{Path: filepath.Join(dir, "client.db"), WAL: true}
	db, err := database.Open(opts)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	want := testutil.SeedNetworkDirect(t, db, "testnet")
	var mode string
	if err := db.Conn.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatal(err)
	}
	if mode != "wal" {
		t.Fatalf("journal mode = %q, want wal", mode)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := database.Open(opts)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	t.Cleanup(func() { _ = reopened.Close() })
	got, err := reopened.GetNetwork(want.Name)
	if err != nil {
		t.Fatalf("read persisted network: %v", err)
	}
	if got.PrivateKey != want.PrivateKey || got.Server != want.Server ||
		got.AssignedRoute != want.AssignedRoute || !got.CreatedAt.Equal(want.CreatedAt) {
		t.Fatalf("reopened network = %#v, want %#v", got, want)
	}
}
