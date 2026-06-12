package database

import (
	"fmt"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

// EndpointReport records endpoint sightings witnessed by peers.
// Sightings referencing unknown peer or witness keys are skipped.
func (s *ServerDB) EndpointReport(
	sightings []server.EndpointSighting,
) error {
	tx, err := s.Conn.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin report tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for _, sighting := range sightings {
		if _, err := tx.Exec(`
			INSERT INTO endpoint (witness, peer, endpoint, time)
			SELECT w.id, p.id, ?3, ?4
			FROM peer w, peer p
			WHERE w.public_key = ?1
			  AND p.public_key = ?2;`,
			sighting.WitnessKey,
			sighting.PeerKey,
			sighting.Endpoint,
			sighting.Timestamp,
		); err != nil {
			return CheckSqliteErr("recording endpoint sighting", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit report tx: %w", err)
	}

	return nil
}

// EndpointsRecent returns sightings recorded at or after the given Unix
// time, newest first, keyed by the sighted peer's public key.
func (s *ServerDB) EndpointsRecent(
	since int64,
) (
	map[string][]server.EndpointWitness,
	error,
) {
	rows, err := s.Conn.Query(`
		SELECT p.public_key, w.public_key, e.endpoint, e.time
		FROM endpoint e
		JOIN peer p ON p.id = e.peer
		JOIN peer w ON w.id = e.witness
		WHERE e.time >= ?1
		ORDER BY e.time DESC;`,
		since,
	)
	if err != nil {
		return nil, CheckSqliteErr("querying recent endpoints", err)
	}
	defer rows.Close()

	endpoints := map[string][]server.EndpointWitness{}
	for rows.Next() {
		var peerKey string
		var witness server.EndpointWitness
		if err := rows.Scan(
			&peerKey,
			&witness.WitnessKey,
			&witness.Endpoint,
			&witness.Timestamp,
		); err != nil {
			return nil, CheckSqliteErr("scanning endpoint sighting", err)
		}
		endpoints[peerKey] = append(endpoints[peerKey], witness)
	}

	return endpoints, nil
}

// EndpointsPrune deletes sightings recorded before the given Unix time.
func (s *ServerDB) EndpointsPrune(
	before int64,
) error {
	_, err := s.Conn.Exec(`
		DELETE FROM endpoint
		WHERE time < ?1;`,
		before,
	)
	return CheckSqliteErr("pruning endpoints", err)
}
