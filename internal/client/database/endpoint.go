package database

import (
	"fmt"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

func (db *DB) SetPeerEndpoints(
	network string,
	pubKey string,
	endpoints []service.PeerEndpoint,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin set peer endpoints tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Look up the peer id.
	var peerID int64
	err = tx.QueryRow(`
		SELECT id FROM peer
		WHERE network_name = ?1 AND public_key = ?2`,
		network, pubKey,
	).Scan(&peerID)
	if err != nil {
		return fmt.Errorf("lookup peer %q: %w", pubKey, err)
	}

	// Upsert each endpoint.
	for _, ep := range endpoints {
		if _, err := tx.Exec(`
			INSERT INTO endpoint (network_name, peer_id, endpoint, server_observed_at)
			VALUES (?1, ?2, ?3, ?4)
			ON CONFLICT (network_name, peer_id, endpoint) DO UPDATE SET
				server_observed_at = CASE
					WHEN ?4 > server_observed_at THEN ?4
					ELSE server_observed_at
				END;`,
			network,
			peerID,
			ep.Endpoint,
			ep.ServerObservedAt,
		); err != nil {
			return fmt.Errorf("upsert endpoint %q for peer %q: %w", ep.Endpoint, pubKey, err)
		}
	}

	// Delete endpoints not in the incoming list.
	if len(endpoints) > 0 {
		eps := make([]any, 0, len(endpoints)+2)
		eps = append(eps, network, peerID)
		for _, ep := range endpoints {
			eps = append(eps, ep.Endpoint)
		}
		query := `DELETE FROM endpoint
			WHERE network_name = ?1 AND peer_id = ?2
			AND endpoint NOT IN (` + placeholders(3, len(endpoints)) + `);`
		if _, err := tx.Exec(query, eps...); err != nil {
			return fmt.Errorf("prune stale endpoints for peer %q: %w", pubKey, err)
		}
	} else {
		if _, err := tx.Exec(`
			DELETE FROM endpoint
			WHERE network_name = ?1 AND peer_id = ?2;`,
			network, peerID,
		); err != nil {
			return fmt.Errorf("clear endpoints for peer %q: %w", pubKey, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set peer endpoints tx: %w", err)
	}
	return nil
}

func (db *DB) UpdatePeerEndpointLocal(
	network string,
	pubKey string,
	endpoint string,
	when int64,
) error {
	_, err := db.Conn.Exec(`
		UPDATE endpoint
		SET local_observed_at = MAX(local_observed_at, ?4)
		WHERE network_name = ?1
			AND peer_id = (SELECT id FROM peer WHERE network_name = ?1 AND public_key = ?2)
			AND endpoint = ?3`,
		network,
		pubKey,
		endpoint,
		when,
	)
	if err != nil {
		return fmt.Errorf("update peer endpoint local: %w", err)
	}
	return nil
}

func (db *DB) ListPeerEndpoints(
	network string,
	pubKey string,
) (
	[]service.PeerEndpoint,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT e.endpoint, e.server_observed_at, e.local_observed_at
		FROM endpoint e
		JOIN peer p ON p.id = e.peer_id
		WHERE p.network_name = ?1 AND p.public_key = ?2
		ORDER BY e.server_observed_at DESC, e.local_observed_at DESC`,
		network,
		pubKey,
	)
	if err != nil {
		return nil, fmt.Errorf("query peer endpoints: %w", err)
	}
	defer rows.Close()

	var endpoints []service.PeerEndpoint
	for rows.Next() {
		var ep service.PeerEndpoint
		if err := rows.Scan(
			&ep.Endpoint,
			&ep.ServerObservedAt,
			&ep.LocalObservedAt,
		); err != nil {
			return nil, fmt.Errorf("scan endpoint: %w", err)
		}
		endpoints = append(endpoints, ep)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate peer endpoints: %w", err)
	}

	return endpoints, nil
}
