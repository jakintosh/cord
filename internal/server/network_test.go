package server_test

import (
	"os"
	"path"
	"testing"

	"git.sr.ht/~jakintosh/cord/internal/database"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

func TestCreateNetwork(t *testing.T) {
	ctx, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}
	if !ctx.CheckPeerExists("cord-server") {
		t.Fatalf("expected cord-server peer to exist after network creation")
	}
}

func TestDeleteNetwork_RemovesFilesAndEmptyDirectories(t *testing.T) {
	srv, configDir, dataDir := createFilesystemNetwork(t)

	if err := srv.DeleteNetwork(); err != nil {
		t.Fatalf("failed to delete network: %v", err)
	}

	assertPathMissing(t, path.Join(configDir, testNetwork.Name+".toml"))
	assertPathMissing(t, path.Join(dataDir, testNetwork.Name+".db"))
	assertPathMissing(t, configDir)
	assertPathMissing(t, dataDir)
}

func TestDeleteNetwork_KeepsNonEmptyDirectories(t *testing.T) {
	srv, configDir, dataDir := createFilesystemNetwork(t)
	configMarker := path.Join(configDir, "keep")
	dataMarker := path.Join(dataDir, "keep")
	if err := os.WriteFile(configMarker, nil, 0600); err != nil {
		t.Fatalf("failed to write config marker: %v", err)
	}
	if err := os.WriteFile(dataMarker, nil, 0600); err != nil {
		t.Fatalf("failed to write data marker: %v", err)
	}

	if err := srv.DeleteNetwork(); err != nil {
		t.Fatalf("failed to delete network: %v", err)
	}

	assertPathMissing(t, path.Join(configDir, testNetwork.Name+".toml"))
	assertPathMissing(t, path.Join(dataDir, testNetwork.Name+".db"))
	assertPathExists(t, configMarker)
	assertPathExists(t, dataMarker)
}

func createFilesystemNetwork(t *testing.T) (*server.Server, string, string) {
	t.Helper()

	root := t.TempDir()
	configDir := path.Join(root, "config")
	dataDir := path.Join(root, "data")
	store, err := database.OpenServer(database.Options{
		Name: testNetwork.Name,
		Dir:  dataDir,
		WAL:  true,
	})
	if err != nil {
		t.Fatalf("failed to open network store: %v", err)
	}
	srv, err := server.New(server.Options{
		Network: testNetwork.Name,
		Config:  server.NewFsConfig(configDir),
		Store:   store,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}
	if err := addNetwork(srv, testNetwork); err != nil {
		t.Fatalf("failed to create network: %v", err)
	}
	return srv, configDir, dataDir
}

func assertPathMissing(t *testing.T, name string) {
	t.Helper()
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatalf("expected path %q to be missing, got %v", name, err)
	}
}

func assertPathExists(t *testing.T, name string) {
	t.Helper()
	if _, err := os.Stat(name); err != nil {
		t.Fatalf("expected path %q to exist: %v", name, err)
	}
}
