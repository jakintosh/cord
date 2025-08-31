package database

import (
	"database/sql"
	"fmt"
)

type migration struct {
	version int
	sql     string
}

var migrations = []migration{
	{
		version: 1,
		sql: `
			CREATE TABLE IF NOT EXISTS association (
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
			CREATE TABLE IF NOT EXISTS cidr (
				id                  INTEGER PRIMARY KEY,
				name                TEXT NOT NULL UNIQUE,
				cidr                TEXT NOT NULL UNIQUE,
				length              INTEGER NOT NULL,
				prefix              INTEGER NOT NULL,
				base                BLOB NOT NULL,
				last                BLOB NOT NULL,
				UNIQUE (base, prefix)
			);
			CREATE TABLE IF NOT EXISTS endpoint (
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
			CREATE TABLE IF NOT EXISTS invite (
				id                  INTEGER PRIMARY KEY,
				name                TEXT NOT NULL UNIQUE,
				public_key          TEXT NOT NULL UNIQUE,
				temp_ip             BLOB NOT NULL UNIQUE,
				final_ip            BLOB NOT NULL UNIQUE,
				admin               INTEGER DEFAULT 0 NOT NULL,
				redeemed            INTEGER DEFAULT 0 NOT NULL,
				expiration          INTEGER NOT NULL
			);
			CREATE TABLE IF NOT EXISTS peer (
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

func getSchemaVersion(
	db *sql.DB,
) (
	int,
	error,
) {
	var version int
	row := db.QueryRow(`PRAGMA user_version;`)
	err := row.Scan(&version)
	if err != nil {
		return -1, err
	}
	return version, nil
}

func setSchemaVersion(
	db *sql.DB,
	version int,
) error {
	stmt := fmt.Sprintf(`PRAGMA user_version = %d;`, version)
	_, err := db.Exec(stmt)
	if err != nil {
		return err
	}
	return nil
}

func migrate(
	db *sql.DB,
) error {

	version, err := getSchemaVersion(db)
	if err != nil {
		return fmt.Errorf("failed to get schema version: %w", err)
	}

	for _, migration := range migrations {
		if version < migration.version {
			_, err := db.Exec(migration.sql)
			if err != nil {
				return fmt.Errorf("error migrating to version %d: %w", migration.version, err)
			}

			err = setSchemaVersion(db, migration.version)
			if err != nil {
				return fmt.Errorf("failed to set schema version: %w", err)
			}
		}
	}
	return nil
}
