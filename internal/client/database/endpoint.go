package database

import (
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

func (db *DB) RecordLocalEndpoint(
	network string,
	pubKey string,
	endpoint string,
	observedAt time.Time,
) error {
	result, err := db.Conn.Exec(`
		INSERT INTO endpoint (peer_id, endpoint, local_observed_at)
		SELECT id, ?3, ?4
		FROM peer
		WHERE network_name = ?1 AND public_key = ?2
		ON CONFLICT (peer_id, endpoint) DO UPDATE SET
			local_observed_at = MAX(local_observed_at, ?4)`,
		network,
		pubKey,
		endpoint,
		observedAt.Unix(),
	)
	if err != nil {
		return CheckSqliteErr("record local peer endpoint", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("record local peer endpoint rows affected: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf(
			"%w: peer %q in network %q",
			service.ErrNotFound,
			pubKey,
			network,
		)
	}

	return nil
}

func (db *DB) RecordEndpointAttempt(
	network string,
	pubKey string,
	endpoint string,
	attemptedAt time.Time,
) error {
	result, err := db.Conn.Exec(`
		UPDATE endpoint
		SET last_attempted_at = MAX(last_attempted_at, ?4)
		WHERE peer_id = (
				SELECT id FROM peer
				WHERE network_name = ?1 AND public_key = ?2
			)
			AND endpoint = ?3`,
		network,
		pubKey,
		endpoint,
		attemptedAt.Unix(),
	)
	if err != nil {
		return CheckSqliteErr("record peer endpoint attempt", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("record peer endpoint attempt rows affected: %w", err)
	}

	if affected == 0 {
		return fmt.Errorf(
			"%w: endpoint %q for peer %q in network %q",
			service.ErrNotFound,
			endpoint,
			pubKey,
			network,
		)
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
		var serverObservedAt int64
		var localObservedAt int64
		var lastAttemptedAt int64
		if err := rows.Scan(
			&ep.Endpoint,
			&serverObservedAt,
			&localObservedAt,
			&lastAttemptedAt,
		); err != nil {
			return nil, fmt.Errorf("scan endpoint: %w", err)
		}
		ep.ServerObservedAt = unixTimeOrZero(serverObservedAt)
		ep.LocalObservedAt = unixTimeOrZero(localObservedAt)
		ep.LastAttemptedAt = unixTimeOrZero(lastAttemptedAt)
		endpoints = append(endpoints, ep)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate peer endpoints: %w", err)
	}

	return endpoints, nil
}

func (db *DB) ListLocalEndpointsSince(
	network string,
	since time.Time,
) (
	[]service.EndpointSighting,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT p.public_key, e.endpoint
		FROM endpoint e
		JOIN peer p ON p.id = e.peer_id
		WHERE p.network_name = ?1 AND e.local_observed_at >= ?2
		ORDER BY p.public_key, e.endpoint`,
		network,
		since.Unix(),
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

func unixTimeOrZero(
	unix int64,
) time.Time {
	if unix == 0 {
		return time.Time{}
	}
	return time.Unix(unix, 0)
}
