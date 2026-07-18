package database

import (
	"errors"
	"fmt"
	"net"

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

func (db *DB) GetPeerByKey(
	network string,
	pubKey string,
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
			AND p.public_key = ?2`,
		network,
		pubKey,
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
	var cidrID int64
	err := db.Conn.QueryRow(`
		SELECT id FROM cidr
		WHERE network_name = ?1 AND name = ?2`,
		network,
		peer.CidrName,
	).Scan(&cidrID)
	if err != nil {
		return CheckSqliteErr("lookup cidr for peer", err)
	}

	_, err = db.Conn.Exec(`
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

func (db *DB) UpdatePeer(
	network string,
	name string,
	update service.PeerUpdate,
) (
	*service.Peer,
	error,
) {
	row := db.Conn.QueryRow(`
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
			END,
			confirmed = CASE
				WHEN ?6 IS NOT NULL THEN ?6
				ELSE confirmed
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
		validOptBool(update.Confirmed),
	)

	var peerName string
	var pubKey string
	var cidrID int64
	var admin int64
	var enabled int64
	var confirmed int64

	if err := row.Scan(&peerName, &pubKey, &cidrID, &admin, &enabled, &confirmed); err != nil {
		return nil, CheckSqliteErr("update peer", err)
	}

	var cidrName string
	var cidrStr string
	err := db.Conn.QueryRow(`
		SELECT name, cidr FROM cidr WHERE id = ?1`,
		cidrID,
	).Scan(&cidrName, &cidrStr)
	if err != nil {
		return nil, fmt.Errorf("lookup cidr after update: %w", err)
	}

	return &service.Peer{
		Name:      peerName,
		PublicKey: pubKey,
		CidrName:  cidrName,
		Route:     cidrStr,
		Admin:     admin != 0,
		Enabled:   enabled != 0,
		Confirmed: confirmed != 0,
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
	if err := row.Scan(
		&peerID,
		&cidrID,
		&publicKey,
	); err != nil {
		return CheckSqliteErr("find peer to delete", err)
	}

	if _, err := tx.Exec(`
		DELETE FROM registration
		WHERE network_name = ?1 AND redeemed_key = ?2`,
		network,
		publicKey,
	); err != nil {
		return CheckSqliteErr("delete peer registration", err)
	}

	if _, err := tx.Exec(`DELETE FROM peer WHERE id = ?1`, peerID); err != nil {
		return CheckSqliteErr("delete peer", err)
	}
	if _, err := tx.Exec(`DELETE FROM cidr WHERE id = ?1`, cidrID); err != nil {
		return CheckSqliteErr("delete peer CIDR", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete peer tx: %w", err)
	}

	return nil
}

func (db *DB) PeerExists(
	network string,
	name string,
) (
	bool,
	error,
) {
	row := db.Conn.QueryRow(`
		SELECT COUNT(*)
		FROM peer
		WHERE network_name = ?1
			AND name = ?2`,
		network,
		name,
	)

	var count int64
	if err := row.Scan(&count); err != nil {
		return false, fmt.Errorf("peer exists %q: %w", name, err)
	}

	return count > 0, nil
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
