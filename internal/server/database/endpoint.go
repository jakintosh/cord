package database

import (
	"database/sql"
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (db *DB) GetRecentEndpoints(
	network string,
	since time.Time,
) (
	map[string][]service.EndpointWitness,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT
			p.public_key,
			w.public_key,
			e.endpoint,
			e.time_unix
		FROM endpoint e
		JOIN peer p ON p.id = e.peer
		JOIN peer w ON w.id = e.witness
		WHERE e.network_name = ?1
			AND e.time_unix >= ?2
		ORDER BY e.time_unix DESC`,
		network,
		since.Unix(),
	)
	if err != nil {
		return nil, CheckSqliteErr("get recent endpoints", err)
	}
	defer rows.Close()

	endpoints := map[string][]service.EndpointWitness{}
	for rows.Next() {
		var peerKey string
		var witness service.EndpointWitness
		var timestamp int64
		if err := rows.Scan(
			&peerKey,
			&witness.Witness,
			&witness.Endpoint,
			&timestamp,
		); err != nil {
			return nil, CheckSqliteErr("scan endpoint", err)
		}
		witness.Timestamp = time.Unix(timestamp, 0)
		endpoints[peerKey] = append(endpoints[peerKey], witness)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate endpoints: %w", err)
	}

	return endpoints, nil
}

func (db *DB) InsertEndpointSightings(
	network string,
	sightings []service.EndpointSighting,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin endpoint sightings tx: %w", err)
	}
	defer tx.Rollback()

	if err := sqlRequireNetworkTx(tx, network); err != nil {
		return err
	}

	for _, s := range sightings {
		peerID, err := sqlGetPeerIDByKeyTx(tx, network, s.PeerKey)
		if err != nil {
			return fmt.Errorf("get observed peer: %w", err)
		}

		witnessID, err := sqlGetPeerIDByKeyTx(tx, network, s.WitnessKey)
		if err != nil {
			return fmt.Errorf("get witness peer: %w", err)
		}

		if err := sqlUpsertEndpointSightingTx(
			tx,
			network,
			peerID,
			witnessID,
			s,
		); err != nil {
			return err
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit endpoint sightings tx: %w", err)
	}

	return nil
}

func (db *DB) DeleteEndpointsBefore(
	network string,
	before time.Time,
) error {
	_, err := db.Conn.Exec(`
		DELETE FROM endpoint
		WHERE network_name = ?1
			AND time_unix < ?2`,
		network,
		before.Unix(),
	)
	return CheckSqliteErr("delete endpoints before", err)
}

func sqlUpsertEndpointSightingTx(
	tx *sql.Tx,
	network string,
	peerID int64,
	witnessID int64,
	sighting service.EndpointSighting,
) error {
	_, err := tx.Exec(`
		INSERT INTO endpoint (
			network_name,
			peer,
			witness,
			endpoint,
			time_unix
		)
		VALUES (?1, ?2, ?3, ?4, ?5)
		ON CONFLICT (network_name, witness, peer)
		DO UPDATE SET
			endpoint = excluded.endpoint,
			time_unix = excluded.time_unix
		WHERE excluded.time_unix >= endpoint.time_unix`,
		network,
		peerID,
		witnessID,
		sighting.Endpoint,
		sighting.Timestamp.Unix(),
	)
	return CheckSqliteErr("insert endpoint sighting", err)
}
