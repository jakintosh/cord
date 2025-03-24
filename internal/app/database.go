package app

import (
	"database/sql"
	"fmt"
	"os"
	"path"

	_ "modernc.org/sqlite"
)

func openDatabase(name string, dataPath string) (*sql.DB, error) {

	dbName := name + ".db"
	dbPath := path.Join(dataPath, dbName)
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open to database: %w\n", err)
	}
	return db, nil
}

func initDatabase(name string, dataPath string) (*sql.DB, error) {

	// ensure dataPath exists
	os.MkdirAll(dataPath, os.ModePerm)

	db, err := openDatabase(name, dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if _, err = db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to init database schema: couldn't enable foreign keys: %w\n", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS cidr (
			id					INTEGER PRIMARY KEY,
			name				TEXT NOT NULL UNIQUE,
			cidr				TEXT NOT NULL UNIQUE,
			length				INTEGER NOT NULL,
			prefix				INTEGER NOT NULL,
			base				BLOB NOT NULL,
			last				BLOB NOT NULL,
			UNIQUE (base, prefix)
		);
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to init 'cidr' table schema: %w\n", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS association (
			id					INTEGER PRIMARY KEY,
			cidr1				INTEGER NOT NULL,
			cidr2				INTEGER NOT NULL,
			FOREIGN KEY (cidr1)
				REFERENCES cidr (id)
					ON UPDATE RESTRICT,
			FOREIGN KEY (cidr2)
				REFERENCES cidr (id)
					ON UPDATE RESTRICT
		);
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to init 'association' table schema: %w\n", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS peer (
			id					INTEGER PRIMARY KEY,
			name				TEXT NOT NULL UNIQUE,
			ip					BLOB NOT NULL UNIQUE,
			public_key			TEXT NOT NULL UNIQUE,
			is_admin			INTEGER DEFAULT 0 NOT NULL,
			is_disabled			INTEGER DEFAULT 0 NOT NULL,
			is_redeemed			INTEGER DEFAULT 0 NOT NULL,
			invite_expires 		INTEGER
		);
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to init 'peer' table schema: %w\n", err)
	}

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS endpoint (
			id					INTEGER PRIMARY KEY,
			peer_ip				BLOB NOT NULL,
			peer_key			TEXT NOT NULL,
			endpoint			TEXT NOT NULL,
			time				INTEGER NOT NULL
		);
	`)
	if err != nil {
		return nil, fmt.Errorf("failed to init 'endpoint' table schema: %w\n", err)
	}

	return db, nil
}

func resultsEmpty(result sql.Result) bool {
	count, err := result.RowsAffected()
	if err != nil {
		return false
	}
	return count == 0
}
