package database

import (
	"fmt"
	"strings"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

func (db *DB) SetPeers(
	network string,
	peers []service.Peer,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin reconcile tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	names := make([]any, 0, len(peers))
	for _, peer := range peers {
		names = append(names, peer.Name)

		if _, err := tx.Exec(`
			INSERT INTO peer (network_name, name, public_key, route)
			VALUES (?1, ?2, ?3, ?4)
			ON CONFLICT (network_name, public_key) DO UPDATE SET
				name = ?2,
				route = ?4;`,
			network,
			peer.Name,
			peer.PublicKey,
			peer.Route,
		); err != nil {
			return fmt.Errorf("failed to upsert peer '%s': %w", peer.Name, err)
		}
	}

	query := `DELETE FROM peer WHERE network_name = ?1;`
	if len(names) > 0 {
		query = `DELETE FROM peer WHERE network_name = ?1 AND name NOT IN (` +
			placeholders(2, len(names)) + `);`
	}
	args := append([]any{network}, names...)
	if _, err := tx.Exec(query, args...); err != nil {
		return fmt.Errorf("failed to prune departed peers: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit reconcile tx: %w", err)
	}
	return nil
}

func (db *DB) ListPeers(
	network string,
) (
	[]*service.Peer,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT
			p.name,
			p.public_key,
			p.route,
			COALESCE(
				(SELECT e.endpoint FROM endpoint e
				 WHERE e.peer_id = p.id
					 ORDER BY e.local_observed_at DESC, e.server_observed_at DESC
				 LIMIT 1),
				''
			) AS endpoint
		FROM peer p
		WHERE p.network_name = ?1
		ORDER BY p.name ASC`,
		network,
	)
	if err != nil {
		return nil, fmt.Errorf("query peers: %w", err)
	}
	defer rows.Close()

	var peers []*service.Peer
	for rows.Next() {
		var peer service.Peer
		if err := rows.Scan(
			&peer.Name,
			&peer.PublicKey,
			&peer.Route,
			&peer.Endpoint,
		); err != nil {
			return nil, fmt.Errorf("scan peer: %w", err)
		}
		peers = append(peers, &peer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate peers: %w", err)
	}

	return peers, nil
}

func placeholders(start, n int) string {
	var s strings.Builder
	for i := range n {
		if i > 0 {
			s.WriteString(", ")
		}
		fmt.Fprintf(&s, "?%d", start+i)
	}
	return s.String()
}
