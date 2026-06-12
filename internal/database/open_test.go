package database_test

import (
	"errors"
	"os"
	"path"
	"testing"

	"git.sr.ht/~jakintosh/cord/internal/database"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

func TestOpenServerMustExistMissingDatabaseErrors(t *testing.T) {
	dataDir := path.Join(t.TempDir(), "data")

	_, err := database.OpenServer(database.Options{
		Name:      "ghost",
		Dir:       dataDir,
		MustExist: true,
	})

	if !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}

func TestOpenServerMustExistMissingDatabaseCreatesNoState(t *testing.T) {
	dataDir := path.Join(t.TempDir(), "data")

	_, _ = database.OpenServer(database.Options{
		Name:      "ghost",
		Dir:       dataDir,
		MustExist: true,
	})

	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Fatalf("expected data dir to not exist, stat err: %v", err)
	}
}

func TestOpenServerMustExistOpensExistingDatabase(t *testing.T) {
	dataDir := path.Join(t.TempDir(), "data")
	created, err := database.OpenServer(database.Options{
		Name: "homenet",
		Dir:  dataDir,
	})
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	if err := created.Close(); err != nil {
		t.Fatalf("failed to close created database: %v", err)
	}

	store, err := database.OpenServer(database.Options{
		Name:      "homenet",
		Dir:       dataDir,
		MustExist: true,
	})
	if err != nil {
		t.Fatalf("failed to open existing database: %v", err)
	}
	_ = store.Close()
}

func TestOpenClientMustExistMissingDatabaseErrors(t *testing.T) {
	dataDir := path.Join(t.TempDir(), "data")

	_, err := database.OpenClient(database.Options{
		Name:      "ghost",
		Dir:       dataDir,
		MustExist: true,
	})

	if !errors.Is(err, server.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got: %v", err)
	}
}
