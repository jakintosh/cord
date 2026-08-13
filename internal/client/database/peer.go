package database

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

func (db *DB) ListPeers(
	network string,
) (
	[]*service.Peer,
	error,
) {
	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin list peers tx: %w", err)
	}
	defer tx.Rollback()

	if err := sqlRequireNetworkTx(tx, network); err != nil {
		return nil, err
	}

	peers, err := sqlListPeersTx(tx, network)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit list peers tx: %w", err)
	}
	return peers, nil
}

func (db *DB) ApplyPeerReconciliation(
	network string,
	reconciliation service.PeerReconciliation,
) error {
	publicKeys := make([]string, len(reconciliation.Peers))
	seenPublicKeys := make(map[string]struct{}, len(reconciliation.Peers))
	for i, observation := range reconciliation.Peers {
		publicKey := observation.Peer.PublicKey
		if _, exists := seenPublicKeys[publicKey]; exists {
			return fmt.Errorf(
				"%w: duplicate peer public key %q in reconciliation",
				service.ErrConflict,
				publicKey,
			)
		}
		seenPublicKeys[publicKey] = struct{}{}
		publicKeys[i] = publicKey
	}

	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin apply peer reconciliation tx: %w", err)
	}
	defer tx.Rollback()

	if err := sqlRequireNetworkTx(tx, network); err != nil {
		return err
	}

	if err := sqlDeletePeersAbsentFromReconciliationTx(
		tx,
		network,
		publicKeys,
	); err != nil {
		return err
	}

	for _, observation := range reconciliation.Peers {
		peerID, err := sqlUpsertObservedPeerTx(
			tx,
			network,
			observation.Peer,
		)
		if err != nil {
			return err
		}

		for _, endpoint := range observation.Endpoints {
			if err := sqlMergeObservedEndpointTx(
				tx,
				peerID,
				observation.Peer.PublicKey,
				endpoint,
			); err != nil {
				return err
			}
		}
	}

	if err := sqlPruneStalePeerEndpointsTx(
		tx,
		network,
		reconciliation.PruneBefore,
	); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit apply peer reconciliation tx: %w", err)
	}
	return nil
}

func sqlListPeersTx(
	tx *sql.Tx,
	network string,
) (
	[]*service.Peer,
	error,
) {
	rows, err := tx.Query(`
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
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close peer rows: %w", err)
	}
	return peers, nil
}

func sqlUpsertObservedPeerTx(
	tx *sql.Tx,
	network string,
	peer service.Peer,
) (
	int64,
	error,
) {
	var peerID int64
	if err := tx.QueryRow(`
		INSERT INTO peer (network_name, name, public_key, route)
		VALUES (?1, ?2, ?3, ?4)
		ON CONFLICT (network_name, public_key) DO UPDATE SET
			name = ?2,
			route = ?4
		RETURNING id`,
		network,
		peer.Name,
		peer.PublicKey,
		peer.Route,
	).Scan(&peerID); err != nil {
		return 0, CheckSqliteErr(
			fmt.Sprintf("upsert peer %q", peer.Name),
			err,
		)
	}
	return peerID, nil
}

func sqlMergeObservedEndpointTx(
	tx *sql.Tx,
	peerID int64,
	peerPublicKey string,
	endpoint service.PeerEndpoint,
) error {
	if _, err := tx.Exec(`
		INSERT INTO endpoint (peer_id, endpoint, server_observed_at)
		VALUES (?1, ?2, ?3)
		ON CONFLICT (peer_id, endpoint) DO UPDATE SET
			server_observed_at = MAX(server_observed_at, ?3)`,
		peerID,
		endpoint.Endpoint,
		endpoint.ServerObservedAt.Unix(),
	); err != nil {
		return CheckSqliteErr(
			fmt.Sprintf(
				"merge endpoint %q for peer %q",
				endpoint.Endpoint,
				peerPublicKey,
			),
			err,
		)
	}
	return nil
}

func sqlPruneStalePeerEndpointsTx(
	tx *sql.Tx,
	network string,
	pruneBefore time.Time,
) error {
	_, err := tx.Exec(`
		DELETE FROM endpoint
		WHERE peer_id IN (
			SELECT id FROM peer WHERE network_name = ?1
		)
			AND server_observed_at < ?2
			AND local_observed_at < ?2`,
		network,
		pruneBefore.Unix(),
	)
	return CheckSqliteErr("prune stale peer endpoints", err)
}

func sqlDeletePeersAbsentFromReconciliationTx(
	tx *sql.Tx,
	network string,
	publicKeys []string,
) error {
	if len(publicKeys) == 0 {
		_, err := tx.Exec(`
			DELETE FROM peer
			WHERE network_name = ?1`,
			network,
		)
		return CheckSqliteErr("delete all reconciled peers", err)
	}

	args := make([]any, len(publicKeys)+1)
	args[0] = network
	for i, publicKey := range publicKeys {
		args[i+1] = publicKey
	}

	query := fmt.Sprintf(`
		DELETE FROM peer
		WHERE network_name = ?1
			AND public_key NOT IN (%s)`,
		placeholders(2, len(publicKeys)),
	)
	_, err := tx.Exec(query, args...)
	return CheckSqliteErr("delete peers absent from reconciliation", err)
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
