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
		INSERT INTO endpoint (network_name, peer_id, endpoint, local_observed_at)
		SELECT ?1, id, ?3, ?4
		FROM peer
		WHERE network_name = ?1 AND public_key = ?2
		ON CONFLICT (network_name, peer_id, endpoint) DO UPDATE SET
			local_observed_at = MAX(local_observed_at, ?4)`,
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

func (db *DB) MarkPeerEndpointAttempt(
	network string,
	pubKey string,
	endpoint string,
	when int64,
) error {
	_, err := db.Conn.Exec(`
		UPDATE endpoint
		SET last_attempted_at = ?4
		WHERE network_name = ?1
			AND peer_id = (SELECT id FROM peer WHERE network_name = ?1 AND public_key = ?2)
			AND endpoint = ?3`,
		network,
		pubKey,
		endpoint,
		when,
	)
	if err != nil {
		return fmt.Errorf("mark peer endpoint attempt: %w", err)
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
		SELECT e.endpoint, e.server_observed_at, e.local_observed_at, e.last_attempted_at
		FROM endpoint e
		JOIN peer p ON p.id = e.peer_id
		WHERE p.network_name = ?1 AND p.public_key = ?2
		ORDER BY e.local_observed_at DESC, e.server_observed_at DESC`,
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
			&ep.LastAttemptedAt,
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

func (db *DB) DeletePeerEndpointsBefore(
	network string,
	before int64,
) error {
	_, err := db.Conn.Exec(`
		DELETE FROM endpoint
		WHERE network_name = ?1
			AND server_observed_at < ?2
			AND local_observed_at < ?2`,
		network,
		before,
	)
	if err != nil {
		return fmt.Errorf("delete stale peer endpoints: %w", err)
	}
	return nil
}

func (db *DB) ListLocalEndpointsSince(
	network string,
	since int64,
) (
	[]service.EndpointSighting,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT p.public_key, e.endpoint
		FROM endpoint e
		JOIN peer p ON p.id = e.peer_id
		WHERE e.network_name = ?1 AND e.local_observed_at >= ?2
		ORDER BY p.public_key, e.endpoint`,
		network,
		since,
	)
	if err != nil {
		return nil, fmt.Errorf("query local endpoints: %w", err)
	}
	defer rows.Close()

	var sightings []service.EndpointSighting
	for rows.Next() {
		var s service.EndpointSighting
		if err := rows.Scan(&s.PeerKey, &s.Endpoint); err != nil {
			return nil, fmt.Errorf("scan local endpoint: %w", err)
		}
		sightings = append(sightings, s)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate local endpoints: %w", err)
	}

	return sightings, nil
}
