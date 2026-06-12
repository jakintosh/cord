package database

import (
	"database/sql"
	"fmt"

	"git.sr.ht/~jakintosh/cord/internal/client"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

// ClientDB is the SQLite adapter backing a client's local peer cache
// for one network.
type ClientDB struct {
	Conn *sql.DB
	name string
	dir  string
}

var _ client.PeerStore = (*ClientDB)(nil)

// OpenClient opens (creating and migrating as needed) the client
// database for a network and returns a handle ready for use.
func OpenClient(
	opts Options,
) (
	*ClientDB,
	error,
) {
	conn, err := openConn(opts)
	if err != nil {
		return nil, err
	}

	if err := migrate(conn, clientMigrations); err != nil {
		conn.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &ClientDB{Conn: conn, name: opts.Name, dir: opts.Dir}, nil
}

func (c *ClientDB) Close() error {
	return c.Conn.Close()
}

// Delete closes the database and removes its files; missing files are
// fine.
func (c *ClientDB) Delete() error {
	if err := c.Conn.Close(); err != nil {
		return fmt.Errorf("failed to close database: %w", err)
	}
	if c.dir == ":memory:" {
		return nil
	}
	return removeDbFiles(dbPath(c.name, c.dir), true)
}

// ReconcilePeers replaces the stored peer set with the server's view,
// keeping locally observed endpoints when they are fresher than what
// the server reports.
func (c *ClientDB) ReconcilePeers(
	peers []server.PublicPeer,
) error {
	tx, err := c.Conn.Begin()
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

// ListPeers returns all stored peers, ordered by name.
func (c *ClientDB) ListPeers() ([]client.LocalPeer, error) {
	rows, err := c.Conn.Query(`
		SELECT name, public_key, cidr, endpoint, endpoint_time
		FROM peer
		ORDER BY name ASC;`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query peers: %w", err)
	}
	defer rows.Close()

	var peers []client.LocalPeer
	for rows.Next() {
		var peer client.LocalPeer
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

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate peers: %w", err)
	}

	return peers, nil
}

// UpdateEndpoint records a locally observed peer endpoint.
func (c *ClientDB) UpdateEndpoint(
	publicKey string,
	endpoint string,
	when int64,
) error {
	_, err := c.Conn.Exec(`
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
