package database

import "fmt"

type Migration struct {
	Version int
	Name    string
	SQL     string
}

var migrations = []Migration{
	{
		Version: 1,
		Name:    "create client schema",
		SQL: `
CREATE TABLE install (
    name                       TEXT PRIMARY KEY,
    phase                      TEXT NOT NULL,
    invite_iface_name          TEXT NOT NULL,
    invite_peer_private_key    TEXT NOT NULL,
    invite_peer_route          TEXT NOT NULL,
    invite_server_pubkey       TEXT NOT NULL,
    invite_server_endpoint     TEXT NOT NULL,
    invite_server_route        TEXT NOT NULL,
    invite_server_network_cidr TEXT NOT NULL DEFAULT '',
    invite_server_api_port     INTEGER NOT NULL,
    main_iface_name            TEXT NOT NULL,
    main_peer_private_key      TEXT NOT NULL,
    main_peer_route            TEXT NOT NULL DEFAULT '',
    main_server_pubkey         TEXT NOT NULL DEFAULT '',
    main_server_endpoint       TEXT NOT NULL DEFAULT '',
    main_server_route          TEXT NOT NULL DEFAULT '',
    main_server_network_cidr   TEXT NOT NULL DEFAULT '',
    main_server_api_port       INTEGER NOT NULL DEFAULT 0,
    listen_port                INTEGER NOT NULL DEFAULT 0,
    created_at_unix            INTEGER NOT NULL
);

CREATE TABLE network (
    name                TEXT PRIMARY KEY,
    peer_private_key    TEXT NOT NULL,
    peer_route          TEXT NOT NULL,
    interface_name      TEXT NOT NULL,
    server_pubkey       TEXT NOT NULL,
    server_endpoint     TEXT NOT NULL,
    server_route        TEXT NOT NULL,
    server_network_cidr TEXT NOT NULL DEFAULT '',
    server_api_port     INTEGER NOT NULL,
    listen_port         INTEGER NOT NULL DEFAULT 0,
    enabled             INTEGER NOT NULL DEFAULT 0,
    created_at_unix     INTEGER NOT NULL
);

CREATE TABLE peer (
    id              INTEGER PRIMARY KEY,
    network_name    TEXT NOT NULL,
    name            TEXT NOT NULL,
    public_key      TEXT NOT NULL,
    route           TEXT NOT NULL,
    FOREIGN KEY (network_name)
        REFERENCES network (name)
        ON DELETE CASCADE,
    UNIQUE (network_name, public_key)
);

CREATE TABLE endpoint (
    id                  INTEGER PRIMARY KEY,
    network_name        TEXT NOT NULL,
    peer_id             INTEGER NOT NULL,
    endpoint            TEXT NOT NULL,
    server_observed_at  INTEGER NOT NULL DEFAULT 0,
    local_observed_at   INTEGER NOT NULL DEFAULT 0,
    last_attempted_at   INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (network_name)
        REFERENCES network (name)
        ON DELETE CASCADE,
    FOREIGN KEY (peer_id)
        REFERENCES peer (id)
        ON DELETE CASCADE,
    UNIQUE (network_name, peer_id, endpoint)
);

CREATE INDEX idx_peer_endpoint_lookup
    ON endpoint (network_name, peer_id, server_observed_at DESC);
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
