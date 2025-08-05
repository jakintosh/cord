package server

import (
	"database/sql"
	"fmt"
	"os"

	db "git.sr.ht/~jakintosh/cord/internal/database"
)

type Data interface {
	OpenDatabase(name string) (*sql.DB, error)
	DeleteDatabase(name string) error
}

// FsData
// Uses the filesystem to manage the data

type FsData struct {
	Directory string
}

func (d *FsData) OpenDatabase(name string) (*sql.DB, error) {

	os.MkdirAll(d.Directory, 0755)
	database, err := db.Open(name, d.Directory)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	return database, nil
}

func (d *FsData) DeleteDatabase(name string) error {

	if err := db.Delete(name, d.Directory); err != nil {
		return fmt.Errorf("failed to delete network: %w", err)
	}
	return nil
}

// MemData
// Uses memory to manage the data

type MemData struct{}

func (d *MemData) OpenDatabase(name string) (*sql.DB, error) {

	database, err := db.Open(name, ":memory:")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	return database, nil
}

func (d *MemData) DeleteDatabase(name string) error {

	return nil
}
