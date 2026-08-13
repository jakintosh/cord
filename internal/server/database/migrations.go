package database

import (
	"fmt"
)

type Migration struct {
	Version int
	Name    string
	SQL     string
}

var migrations = []Migration{
	{
		Version: 1,
		Name:    "create initial schema",
		SQL: `
CREATE TABLE network (
    name                TEXT PRIMARY KEY,
    private_key         TEXT NOT NULL,
    public_key          TEXT NOT NULL,
    external_ip         TEXT NOT NULL,
    main_name           TEXT NOT NULL,
    main_cidr           TEXT NOT NULL,
    main_wg_port        INTEGER NOT NULL,
    main_api_port       INTEGER NOT NULL,
    invite_name         TEXT NOT NULL,
    invite_cidr         TEXT NOT NULL,
    invite_wg_port      INTEGER NOT NULL,
    invite_api_port     INTEGER NOT NULL,
    enabled             INTEGER DEFAULT 0 NOT NULL,
    created_at_unix     INTEGER NOT NULL
);

CREATE TABLE cidr (
    id              INTEGER PRIMARY KEY,
    network_name    TEXT NOT NULL,
    name            TEXT NOT NULL,
    cidr            TEXT NOT NULL,
    length          INTEGER NOT NULL,
    prefix          INTEGER NOT NULL,
    base            BLOB NOT NULL,
    last            BLOB NOT NULL,
    terminal        INTEGER DEFAULT 0 NOT NULL,
    FOREIGN KEY (network_name)
        REFERENCES network (name)
        ON DELETE CASCADE,
    UNIQUE (network_name, name),
    UNIQUE (network_name, cidr),
    UNIQUE (network_name, base, prefix)
);

CREATE TABLE "group" (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    network_name    TEXT NOT NULL,
    name            TEXT NOT NULL,
    FOREIGN KEY (network_name)
        REFERENCES network (name)
        ON DELETE CASCADE,
    UNIQUE (network_name, name)
);

CREATE TABLE cidr_assignment (
    cidr_id         INTEGER NOT NULL,
    group_id        INTEGER NOT NULL,
    FOREIGN KEY (cidr_id)
        REFERENCES cidr (id)
        ON DELETE CASCADE,
    FOREIGN KEY (group_id)
        REFERENCES "group" (id)
        ON DELETE CASCADE,
    PRIMARY KEY (cidr_id, group_id)
);

CREATE TABLE association (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    network_name    TEXT NOT NULL,
    group1_id       INTEGER NOT NULL,
    group2_id       INTEGER NOT NULL,
    CHECK (group1_id <= group2_id),
    FOREIGN KEY (network_name)
        REFERENCES network (name)
        ON DELETE CASCADE,
    FOREIGN KEY (group1_id)
        REFERENCES "group" (id)
        ON DELETE CASCADE,
    FOREIGN KEY (group2_id)
        REFERENCES "group" (id)
        ON DELETE CASCADE,
    UNIQUE (group1_id, group2_id)
);

CREATE TABLE peer (
    id              INTEGER PRIMARY KEY,
    network_name    TEXT NOT NULL,
    name            TEXT NOT NULL,
    cidr_id         INTEGER NOT NULL,
    public_key      TEXT NOT NULL,
    admin           INTEGER DEFAULT 0 NOT NULL,
    enabled         INTEGER DEFAULT 0 NOT NULL,
    confirmed       INTEGER DEFAULT 0 NOT NULL,
    FOREIGN KEY (network_name)
        REFERENCES network (name)
        ON DELETE CASCADE,
    FOREIGN KEY (cidr_id)
        REFERENCES cidr (id),
    UNIQUE (network_name, name),
    UNIQUE (network_name, cidr_id),
    UNIQUE (network_name, public_key)
);

CREATE TABLE registration (
    id              INTEGER PRIMARY KEY,
    network_name    TEXT NOT NULL,
    name            TEXT NOT NULL,
    temp_public_key TEXT NOT NULL,
    temp_route      TEXT NOT NULL,
    final_route     TEXT NOT NULL,
    admin           INTEGER DEFAULT 0 NOT NULL,
    redeemed        INTEGER DEFAULT 0 NOT NULL,
    redeemed_key    TEXT DEFAULT '' NOT NULL,
    confirmed       INTEGER DEFAULT 0 NOT NULL,
    expires_at_unix INTEGER NOT NULL,
    created_at_unix INTEGER NOT NULL,
    FOREIGN KEY (network_name)
        REFERENCES network (name)
        ON DELETE CASCADE,
    UNIQUE (network_name, name),
    UNIQUE (network_name, temp_public_key),
    UNIQUE (network_name, final_route)
);

CREATE UNIQUE INDEX registration_active_temp_route
    ON registration (network_name, temp_route)
    WHERE confirmed = 0;

CREATE TABLE registration_assignment (
    registration_id INTEGER NOT NULL,
    group_id        INTEGER NOT NULL,
    FOREIGN KEY (registration_id)
        REFERENCES registration (id)
        ON DELETE CASCADE,
    FOREIGN KEY (group_id)
        REFERENCES "group" (id)
        ON DELETE CASCADE,
    PRIMARY KEY (registration_id, group_id)
);

CREATE TABLE endpoint (
    id              INTEGER PRIMARY KEY,
    network_name    TEXT NOT NULL,
    peer            INTEGER NOT NULL,
    witness         INTEGER NOT NULL,
    endpoint        TEXT NOT NULL,
    time_unix       INTEGER NOT NULL,
    FOREIGN KEY (network_name)
        REFERENCES network (name)
        ON DELETE CASCADE,
    FOREIGN KEY (peer)
        REFERENCES peer (id)
        ON DELETE CASCADE,
    FOREIGN KEY (witness)
        REFERENCES peer (id)
        ON DELETE CASCADE,
    UNIQUE (network_name, witness, peer)
);
`,
	},
}

func (db *DB) migrate() error {
	current, err := db.userVersion()
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for _, m := range migrations {
		if m.Version <= current {
			continue
		}

		tx, err := db.Conn.Begin()
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

func (db *DB) userVersion() (int, error) {
	var version int
	if err := db.Conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}
