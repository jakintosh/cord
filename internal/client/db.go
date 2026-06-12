package client

import (
	"database/sql"
	"fmt"
	"os"
	"path"

	db "git.sr.ht/~jakintosh/cord/internal/database"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

// LocalPeer is the client's record of a network peer: identity plus
// the most recently known endpoint.
type LocalPeer struct {
	Name         string
	PublicKey    string
	Cidr         string
	Endpoint     string
	EndpointTime int64
}

func (ctx *Context) dbPath() string {
	return path.Join(ctx.DataDir, ctx.Name+".db")
}

// openDb opens (and initializes) the network's local peer database.
func (ctx *Context) openDb() (*sql.DB, error) {
	if err := os.MkdirAll(ctx.DataDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create data dir: %w", err)
	}

	d, err := db.Open(ctx.Name, ctx.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.InitTable(d, "peer", `
		CREATE TABLE IF NOT EXISTS peer (
			id                INTEGER PRIMARY KEY,
			name              TEXT NOT NULL UNIQUE,
			public_key        TEXT NOT NULL UNIQUE,
			cidr              TEXT NOT NULL,
			endpoint          TEXT DEFAULT '' NOT NULL,
			endpoint_time     INTEGER DEFAULT 0 NOT NULL
		);
	`); err != nil {
		d.Close()
		return nil, err
	}

	return d, nil
}

// deleteDb removes the local database file; missing files are fine.
func (ctx *Context) deleteDb() error {
	err := os.Remove(ctx.dbPath())
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete database: %w", err)
	}
	return nil
}

// reconcilePeers replaces the local peer set with the server's view,
// keeping locally observed endpoints when they are fresher than what
// the server reports.
func reconcilePeers(d *sql.DB, peers []server.PublicPeer) error {
	tx, err := d.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin reconcile tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// collect remote names for the deletion pass
	names := make([]any, 0, len(peers))
	for _, peer := range peers {
		names = append(names, peer.Name)

		// the server returns sightings newest-first
		endpoint := ""
		var endpointTime int64
		if len(peer.Endpoints) > 0 {
			endpoint = peer.Endpoints[0].Endpoint
			endpointTime = peer.Endpoints[0].Timestamp
		}

		if _, err := tx.Exec(`
			INSERT INTO peer (name, public_key, cidr, endpoint, endpoint_time)
			VALUES (?1, ?2, ?3, ?4, ?5)
			ON CONFLICT (public_key) DO UPDATE SET
				name = ?1,
				cidr = ?3,
				endpoint = CASE
					WHEN ?5 > endpoint_time THEN ?4
					ELSE endpoint
				END,
				endpoint_time = MAX(?5, endpoint_time);`,
			peer.Name,
			peer.PublicKey,
			peer.Cidr,
			endpoint,
			endpointTime,
		); err != nil {
			return fmt.Errorf("failed to upsert peer '%s': %w", peer.Name, err)
		}
	}

	// remove peers the server no longer reports
	query := `DELETE FROM peer;`
	if len(names) > 0 {
		query = `DELETE FROM peer WHERE name NOT IN (` +
			placeholders(len(names)) + `);`
	}
	if _, err := tx.Exec(query, names...); err != nil {
		return fmt.Errorf("failed to prune departed peers: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit reconcile tx: %w", err)
	}
	return nil
}

// placeholders renders "?1, ?2, ... ?n" for SQL IN clauses.
func placeholders(n int) string {
	s := ""
	for i := range n {
		if i > 0 {
			s += ", "
		}
		s += fmt.Sprintf("?%d", i+1)
	}
	return s
}

// listLocalPeers returns all locally known peers.
func listLocalPeers(d *sql.DB) ([]LocalPeer, error) {
	rows, err := d.Query(`
		SELECT name, public_key, cidr, endpoint, endpoint_time
		FROM peer
		ORDER BY name ASC;`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query peers: %w", err)
	}
	defer rows.Close()

	var peers []LocalPeer
	for rows.Next() {
		var peer LocalPeer
		if err := rows.Scan(
			&peer.Name,
			&peer.PublicKey,
			&peer.Cidr,
			&peer.Endpoint,
			&peer.EndpointTime,
		); err != nil {
			return nil, fmt.Errorf("failed to scan peer: %w", err)
		}
		peers = append(peers, peer)
	}

	return peers, nil
}

// updateLocalEndpoint records a locally observed peer endpoint.
func updateLocalEndpoint(d *sql.DB, publicKey, endpoint string, when int64) error {
	_, err := d.Exec(`
		UPDATE peer
		SET endpoint = ?2, endpoint_time = ?3
		WHERE public_key = ?1;`,
		publicKey,
		endpoint,
		when,
	)
	if err != nil {
		return fmt.Errorf("failed to update endpoint: %w", err)
	}
	return nil
}
