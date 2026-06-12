package database

import (
	"database/sql"
	"fmt"
)

type Migration struct {
	Version int
	Name    string
	SQL     string
}

var serverMigrations = []Migration{
	{
		Version: 1,
		Name:    "create server schema",
		SQL: `
			CREATE TABLE association (
				id                  INTEGER PRIMARY KEY,
				cidr1               INTEGER NOT NULL,
				cidr2               INTEGER NOT NULL,
				CHECK (cidr1 < cidr2),
				UNIQUE (cidr1, cidr2),
				FOREIGN KEY (cidr1)
					REFERENCES cidr (id),
				FOREIGN KEY (cidr2)
					REFERENCES cidr (id)
			);
			CREATE TABLE cidr (
				id                  INTEGER PRIMARY KEY,
				name                TEXT NOT NULL UNIQUE,
				cidr                TEXT NOT NULL UNIQUE,
				length              INTEGER NOT NULL,
				prefix              INTEGER NOT NULL,
				base                BLOB NOT NULL,
				last                BLOB NOT NULL,
				UNIQUE (base, prefix)
			);
			CREATE TABLE endpoint (
				id                  INTEGER PRIMARY KEY,
				witness             INTEGER NOT NULL,
				peer                INTEGER NOT NULL,
				endpoint            TEXT NOT NULL,
				time                INTEGER NOT NULL,
				FOREIGN KEY (peer)
					REFERENCES peer (id),
				FOREIGN KEY (witness)
					REFERENCES peer (id)
			);
			CREATE TABLE invite (
				id                  INTEGER PRIMARY KEY,
				name                TEXT NOT NULL UNIQUE,
				public_key          TEXT NOT NULL UNIQUE,
				temp_ip             BLOB NOT NULL UNIQUE,
				final_ip            BLOB NOT NULL UNIQUE,
				admin               INTEGER DEFAULT 0 NOT NULL,
				redeemed            INTEGER DEFAULT 0 NOT NULL,
				expiration          INTEGER NOT NULL
			);
			CREATE TABLE peer (
				id                  INTEGER PRIMARY KEY,
				name                TEXT NOT NULL UNIQUE,
				ip                  BLOB NOT NULL UNIQUE,
				prefix              INTEGER NOT NULL,
				public_key          TEXT NOT NULL UNIQUE,
				admin               INTEGER DEFAULT 0 NOT NULL,
				enabled             INTEGER DEFAULT 0 NOT NULL,
				confirmed           INTEGER DEFAULT 0 NOT NULL
			);
		`,
	},
}

var clientMigrations = []Migration{
	{
		Version: 1,
		Name:    "create peer cache table",
		SQL: `
			CREATE TABLE peer (
				id                INTEGER PRIMARY KEY,
				name              TEXT NOT NULL UNIQUE,
				public_key        TEXT NOT NULL UNIQUE,
				cidr              TEXT NOT NULL,
				endpoint          TEXT DEFAULT '' NOT NULL,
				endpoint_time     INTEGER DEFAULT 0 NOT NULL
			);
		`,
	},
}

func migrate(
	conn *sql.DB,
	migrations []Migration,
) error {
	current, err := userVersion(conn)
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, m := range migrations {
		if m.Version <= current {
			continue
		}

		tx, err := conn.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %d %q: %w", m.Version, m.Name, err)
		}

		if _, err := tx.Exec(m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("run migration %d %q: %w", m.Version, m.Name, err)
		}
		if _, err := tx.Exec(fmt.Sprintf("PRAGMA user_version = %d", m.Version)); err != nil {
			tx.Rollback()
			return fmt.Errorf("set schema version %d: %w", m.Version, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %d %q: %w", m.Version, m.Name, err)
		}

		current = m.Version
	}

	return nil
}

func userVersion(
	conn *sql.DB,
) (
	int,
	error,
) {
	var version int
	if err := conn.QueryRow(`PRAGMA user_version;`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}
