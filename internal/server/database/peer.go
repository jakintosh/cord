package database

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (db *DB) GetPeer(
	network string,
	name string,
) (
	*service.Peer,
	error,
) {
	row := db.Conn.QueryRow(`
		SELECT
			p.name,
			p.public_key,
			c.name,
			c.cidr,
			p.admin,
			p.enabled,
			p.confirmed
		FROM peer p
		JOIN cidr c ON c.id = p.cidr_id
		WHERE p.network_name = ?1
			AND p.name = ?2`,
		network,
		name,
	)

	return scanPeer(row)
}

func (db *DB) GetPeerByIP(
	network string,
	ip net.IP,
) (
	*service.Peer,
	error,
) {
	route := netaddr.HostRoute(netaddr.Normalize(ip))
	row := db.Conn.QueryRow(`
		SELECT
			p.name,
			p.public_key,
			c.name,
			c.cidr,
			p.admin,
			p.enabled,
			p.confirmed
		FROM peer p
		JOIN cidr c ON c.id = p.cidr_id
		WHERE p.network_name = ?1
			AND c.cidr = ?2
			AND p.confirmed = 1
			AND p.enabled = 1`,
		network,
		route.String(),
	)

	return scanPeer(row)
}

func (db *DB) GetProvisionalPeerByIP(
	network string,
	ip net.IP,
) (
	*service.Peer,
	error,
) {
	route := netaddr.HostRoute(netaddr.Normalize(ip))
	row := db.Conn.QueryRow(`
		SELECT
			p.name,
			p.public_key,
			c.name,
			c.cidr,
			p.admin,
			p.enabled,
			p.confirmed
		FROM peer p
		JOIN cidr c ON c.id = p.cidr_id
		WHERE p.network_name = ?1
			AND c.cidr = ?2
			AND p.confirmed = 0
			AND p.enabled = 1`,
		network,
		route.String(),
	)

	return scanPeer(row)
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
			c.name,
			c.cidr,
			p.admin,
			p.enabled,
			p.confirmed
		FROM peer p
		JOIN cidr c ON c.id = p.cidr_id
		WHERE p.network_name = ?1
		ORDER BY p.name ASC`,
		network,
	)
	if err != nil {
		return nil, fmt.Errorf("list peers: %w", err)
	}
	defer rows.Close()

	var peers []*service.Peer
	for rows.Next() {
		peer, err := scanPeer(rows)
		if err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate peers: %w", err)
	}

	return peers, nil
}

func (db *DB) InsertPeer(
	network string,
	peer *service.Peer,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin insert peer tx: %w", err)
	}
	defer tx.Rollback()

	cidrID, err := sqlGetCidrIdTx(tx, network, peer.CidrName, "lookup cidr for peer")
	if err != nil {
		return err
	}
	if err := sqlInsertPeerTx(tx, network, cidrID, peer); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit insert peer tx: %w", err)
	}
	return nil
}

func (db *DB) UpdatePeer(
	network string,
	name string,
	update service.PeerDiff,
) (
	*service.Peer,
	error,
) {
	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin update peer tx: %w", err)
	}
	defer tx.Rollback()

	state, err := sqlUpdatePeerTx(tx, network, name, update)
	if err != nil {
		return nil, err
	}
	cidrName, cidrStr, err := sqlGetCidrTx(tx, state.cidrID)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit update peer tx: %w", err)
	}

	return &service.Peer{
		Name:      state.name,
		PublicKey: state.publicKey,
		CidrName:  cidrName,
		Route:     cidrStr,
		Admin:     state.admin,
		Enabled:   state.enabled,
		Confirmed: state.confirmed,
	}, nil
}

func (db *DB) DeletePeer(
	network string,
	name string,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin delete peer tx: %w", err)
	}
	defer tx.Rollback()

	peerID, cidrID, publicKey, err := sqlGetPeerForDeletionTx(tx, network, name)
	if err != nil {
		return err
	}
	if err := sqlDeleteRegistrationByKeyTx(tx, network, publicKey); err != nil {
		return err
	}
	if err := sqlDeletePeerTx(tx, peerID); err != nil {
		return err
	}
	if err := sqlDeleteCidrTx(tx, cidrID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete peer tx: %w", err)
	}

	return nil
}

type peerConfirmationState struct {
	id        int64
	cidrID    int64
	publicKey string
	confirmed bool
}

type peerUpdateState struct {
	name      string
	publicKey string
	cidrID    int64
	admin     bool
	enabled   bool
	confirmed bool
}

func sqlGetPeerForDeletionTx(
	tx *sql.Tx,
	network string,
	name string,
) (
	int64,
	int64,
	string,
	error,
) {
	row := tx.QueryRow(`
		SELECT id, cidr_id, public_key
		FROM peer
		WHERE network_name = ?1 AND name = ?2`,
		network,
		name,
	)

	var peerID int64
	var cidrID int64
	var publicKey string
	if err := row.Scan(&peerID, &cidrID, &publicKey); err != nil {
		return 0, 0, "", CheckSqliteErr("find peer to delete", err)
	}
	return peerID, cidrID, publicKey, nil
}
func sqlInsertPeerTx(
	tx *sql.Tx,
	network string,
	cidrID int64,
	peer *service.Peer,
) error {
	_, err := tx.Exec(`
		INSERT INTO peer (
			network_name,
			name,
			cidr_id,
			public_key,
			admin,
			enabled,
			confirmed
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)`,
		network,
		peer.Name,
		cidrID,
		peer.PublicKey,
		boolToInt(peer.Admin),
		boolToInt(peer.Enabled),
		boolToInt(peer.Confirmed),
	)
	return CheckSqliteErr("insert peer", err)
}

func sqlUpdatePeerTx(
	tx *sql.Tx,
	network string,
	name string,
	update service.PeerDiff,
) (
	peerUpdateState,
	error,
) {
	row := tx.QueryRow(`
		UPDATE peer
		SET
			name = CASE
				WHEN ?3 IS NOT NULL THEN ?3
				ELSE name
			END,
			admin = CASE
				WHEN ?4 IS NOT NULL THEN ?4
				ELSE admin
			END,
			enabled = CASE
				WHEN ?5 IS NOT NULL THEN ?5
				ELSE enabled
			END
		WHERE network_name = ?1
			AND name = ?2
		RETURNING
			name,
			public_key,
			cidr_id,
			admin,
			enabled,
			confirmed`,
		network,
		name,
		update.Name,
		validOptBool(update.Admin),
		validOptBool(update.Enabled),
	)

	var state peerUpdateState
	var admin int64
	var enabled int64
	var confirmed int64
	if err := row.Scan(
		&state.name,
		&state.publicKey,
		&state.cidrID,
		&admin,
		&enabled,
		&confirmed,
	); err != nil {
		return peerUpdateState{}, CheckSqliteErr("update peer", err)
	}
	state.admin = admin != 0
	state.enabled = enabled != 0
	state.confirmed = confirmed != 0
	return state, nil
}

func sqlDeletePeerTx(
	tx *sql.Tx,
	peerID int64,
) error {
	_, err := tx.Exec(`DELETE FROM peer WHERE id = ?1`, peerID)
	return CheckSqliteErr("delete peer", err)
}

func sqlGetPeerIDByKeyTx(
	tx *sql.Tx,
	network string,
	publicKey string,
) (
	int64,
	error,
) {
	var id int64
	if err := tx.QueryRow(`
		SELECT id
		FROM peer
		WHERE network_name = ?1
			AND public_key = ?2`,
		network,
		publicKey,
	).Scan(&id); err != nil {
		return 0, CheckSqliteErr("get peer ID by key", err)
	}
	return id, nil
}

func sqlGetPeerForConfirmationTx(
	tx *sql.Tx,
	network string,
	name string,
) (
	peerConfirmationState,
	error,
) {
	var state peerConfirmationState
	var confirmed int64
	if err := tx.QueryRow(`
		SELECT id, cidr_id, public_key, confirmed
		FROM peer
		WHERE network_name = ?1 AND name = ?2`,
		network,
		name,
	).Scan(
		&state.id,
		&state.cidrID,
		&state.publicKey,
		&confirmed,
	); err != nil {
		return peerConfirmationState{}, CheckSqliteErr("find peer to confirm", err)
	}
	state.confirmed = confirmed != 0
	return state, nil
}

func sqlInsertBootstrapPeerTx(
	tx *sql.Tx,
	network string,
	cidrID int64,
	peer *service.Peer,
) error {
	_, err := tx.Exec(`
		INSERT INTO peer (
			network_name,
			name,
			cidr_id,
			public_key,
			admin,
			enabled,
			confirmed
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)`,
		network,
		peer.Name,
		cidrID,
		peer.PublicKey,
		boolToInt(peer.Admin),
		boolToInt(peer.Enabled),
		boolToInt(peer.Confirmed),
	)
	return CheckSqliteErr("insert server peer", err)
}

func sqlInsertRedeemedPeerTx(
	tx *sql.Tx,
	network string,
	registration *registrationRedemption,
	cidrID int64,
	permPubKey string,
) error {
	if _, err := tx.Exec(`
		INSERT INTO peer (
			network_name,
			name,
			cidr_id,
			public_key,
			admin,
			enabled,
			confirmed
		)
		VALUES (?1, ?2, ?3, ?4, ?5, 1, 0)`,
		network,
		registration.name,
		cidrID,
		permPubKey,
		boolToInt(registration.admin),
	); err != nil {
		return CheckSqliteErr("redeem create peer", err)
	}

	return nil
}

func sqlMarkPeerConfirmedTx(
	tx *sql.Tx,
	peerID int64,
) error {
	_, err := tx.Exec(`
		UPDATE peer
		SET confirmed = 1
		WHERE id = ?1`,
		peerID,
	)
	return CheckSqliteErr("confirm peer", err)
}

func sqlDeleteRevokedPeerTx(
	tx *sql.Tx,
	network string,
	publicKey string,
) error {
	_, err := tx.Exec(`
		DELETE FROM peer
		WHERE network_name = ?1 AND public_key = ?2`,
		network,
		publicKey,
	)
	return CheckSqliteErr("delete revoked provisional peer", err)
}

func sqlLookupProvisionalPeerCidrTx(
	tx *sql.Tx,
	network string,
	publicKey string,
) (
	int64,
	bool,
	error,
) {
	var cidrID int64
	if err := tx.QueryRow(`
		SELECT cidr_id FROM peer
		WHERE network_name = ?1
			AND public_key = ?2
			AND confirmed = 0`,
		network,
		publicKey,
	).Scan(&cidrID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, nil
		}
		return 0, false, CheckSqliteErr("find provisional peer to revoke", err)
	}
	return cidrID, true, nil
}

func sqlListPrunablePeerCidrIDsTx(
	tx *sql.Tx,
	network string,
	now time.Time,
) (
	[]int64,
	error,
) {
	rows, err := tx.Query(`
		SELECT cidr_id
		FROM peer p
		WHERE p.network_name = ?1
			AND p.confirmed = 0
			AND NOT EXISTS (
				SELECT 1 FROM registration r
				WHERE r.network_name = p.network_name
					AND r.redeemed_key = p.public_key
					AND r.confirmed = 0
					AND r.expires_at_unix > ?2
			)`,
		network,
		now.Unix(),
	)
	if err != nil {
		return nil, CheckSqliteErr("find provisional peer CIDRs to prune", err)
	}
	defer rows.Close()

	var cidrIDs []int64
	for rows.Next() {
		var cidrID int64
		if err := rows.Scan(&cidrID); err != nil {
			return nil, CheckSqliteErr("scan provisional peer CIDR", err)
		}
		cidrIDs = append(cidrIDs, cidrID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate provisional peer CIDRs: %w", err)
	}

	return cidrIDs, nil
}

func sqlDeletePrunablePeersTx(
	tx *sql.Tx,
	network string,
	now time.Time,
) error {
	_, err := tx.Exec(`
		DELETE FROM peer
		WHERE network_name = ?1 AND confirmed = 0
			AND NOT EXISTS (
				SELECT 1 FROM registration r
				WHERE r.network_name = peer.network_name
					AND r.redeemed_key = peer.public_key
					AND r.confirmed = 0
					AND r.expires_at_unix > ?2
			)`,
		network,
		now.Unix(),
	)
	return CheckSqliteErr("prune provisional peers", err)
}

func scanPeer(
	s Scanner,
) (
	*service.Peer,
	error,
) {
	var name string
	var pubKey string
	var cidrName string
	var cidrStr string
	var admin int64
	var enabled int64
	var confirmed int64
	if err := s.Scan(
		&name,
		&pubKey,
		&cidrName,
		&cidrStr,
		&admin,
		&enabled,
		&confirmed,
	); err != nil {
		if errors.Is(err, errScan) {
			return nil, err
		}
		return nil, CheckSqliteErr("scan peer", err)
	}

	return &service.Peer{
		Name:      name,
		PublicKey: pubKey,
		CidrName:  cidrName,
		Route:     cidrStr,
		Admin:     admin != 0,
		Enabled:   enabled != 0,
		Confirmed: confirmed != 0,
	}, nil
}
