package app

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"path"

	_ "modernc.org/sqlite"
)

var db *sql.DB

func initDatabase(name string, dataPath string) {

	// ensure dataPath exists
	os.MkdirAll(dataPath, os.ModePerm)

	dbName := name + ".db"
	dbPath := path.Join(dataPath, dbName)

	var err error
	db, err = sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("failed to connect to database: %v\n", err)
	}

	if _, err = db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		log.Fatalf("failed to init database schema: couldn't enable foreign keys: %v\n", err)
	}

	err = initTable("cidr", `
		CREATE TABLE IF NOT EXISTS cidr (
			id					INTEGER PRIMARY KEY,
			name				TEXT NOT NULL UNIQUE,
			ip					TEXT NOT NULL,
			prefix				INTEGER NOT NULL,
			parent				INTEGER,
			UNIQUE (ip, prefix),
			FOREIGN KEY (parent)
				REFERENCES cidrs (id)
					ON UPDATE RESTRICT
					ON DELETE RESTRICT
		);
	`)
	if err != nil {
		log.Fatalf("failed to init database: %v\n", err)
	}

	err = initTable("association", `
		CREATE TABLE IF NOT EXISTS association (
			id					INTEGER PRIMARY KEY,
			subject				INTEGER NOT NULL,
			peer				INTEGER NOT NULL,
			FOREIGN KEY (subject)
				REFERENCES cidr (id)
					ON UPDATE RESTRICT
					ON DELETE RESTRICT,
			FOREIGN KEY (peer)
				REFERENCES cidr (id)
					ON UPDATE RESTRICT
					ON DELETE RESTRICT
		);
	`)
	if err != nil {
		log.Fatalf("failed to init database: %v\n", err)
	}

	err = initTable("peer", `
		CREATE TABLE IF NOT EXISTS peer (
			id					INTEGER PRIMARY KEY,
			name				TEXT NOT NULL UNIQUE,
			ip					TEXT NOT NULL UNIQUE,
			public_key			TEXT NOT NULL UNIQUE,
			cidr				INTEGER NOT NULL,
			is_admin			INTEGER DEFAULT 0 NOT NULL,
			is_disabled			INTEGER DEFAULT 0 NOT NULL,
			is_redeemed			INTEGER DEFAULT 0 NOT NULL,
			invite_expires 		INTEGER
		);
	`)
	if err != nil {
		log.Fatalf("failed to init database: %v\n", err)
	}

	err = initTable("endpoint", `
		CREATE TABLE IF NOT EXISTS endpoint (
			id					INTEGER PRIMARY KEY,
			peer_ip				TEXT NOT NULL,
			peer_key			TEXT NOT NULL,
			endpoint			TEXT NOT NULL,
			time				INTEGER NOT NULL
		);
	`)

	if err != nil {
		log.Fatalf("failed to init database: %v\n", err)
	}
}

func initTable(name string, sql string) error {
	_, err := db.Exec(sql)
	if err != nil {
		return fmt.Errorf("failed to init '%s' table schema: %v\n", name, err)
	}
	return nil
}

func resultsEmpty(result sql.Result) bool {
	count, err := result.RowsAffected()
	if err != nil {
		return false
	}
	return count == 0
}
