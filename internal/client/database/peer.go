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
			INSERT INTO peer (network_name, name, public_key, cidr, endpoint, endpoint_time)
			VALUES (?1, ?2, ?3, ?4, ?5, ?6)
			ON CONFLICT (network_name, public_key) DO UPDATE SET
				name = ?2,
				cidr = ?4,
				endpoint = CASE
					WHEN ?6 > endpoint_time THEN ?5
					ELSE endpoint
				END,
				endpoint_time = MAX(?6, endpoint_time);`,
			network,
			peer.Name,
			peer.PublicKey,
			peer.Cidr,
			peer.Endpoint,
			peer.EndpointTime,
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
		SELECT name, public_key, cidr, endpoint, endpoint_time
		FROM peer
		WHERE network_name = ?1
		ORDER BY name ASC`,
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
			&peer.Cidr,
			&peer.Endpoint,
			&peer.EndpointTime,
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

func (db *DB) UpdatePeerEndpoint(
	network string,
	pubKey string,
	endpoint string,
	when int64,
) error {
	_, err := db.Conn.Exec(`
		UPDATE peer
		SET endpoint = ?3, endpoint_time = ?4
		WHERE network_name = ?1 AND public_key = ?2`,
		network,
		pubKey,
		endpoint,
		when,
	)
	if err != nil {
		return fmt.Errorf("update peer endpoint: %w", err)
	}
	return nil
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
