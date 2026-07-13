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
			name,
			public_key,
			route,
			admin,
			enabled,
			confirmed
		FROM peer
		WHERE network_name = ?1
			AND name = ?2`,
		network,
		name,
	)

	peer, err := scanPeer(row)
	if err != nil {
		return nil, err
	}
	return peer, nil
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
			name,
			public_key,
			route,
			admin,
			enabled,
			confirmed
		FROM peer
		WHERE network_name = ?1
			AND route = ?2
			AND confirmed = 1
			AND enabled = 1`,
		network,
		route.String(),
	)

	peer, err := scanPeer(row)
	if err != nil {
		return nil, err
	}
	return peer, nil
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
			name,
			public_key,
			route,
			admin,
			enabled,
			confirmed
		FROM peer
		WHERE network_name = ?1
			AND route = ?2
			AND confirmed = 0
			AND enabled = 1`,
		network,
		route.String(),
	)

	peer, err := scanPeer(row)
	if err != nil {
		return nil, err
	}
	return peer, nil
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
			name,
			public_key,
			route,
			admin,
			enabled,
			confirmed
		FROM peer
		WHERE network_name = ?1
			AND public_key = ?2`,
		network,
		pubKey,
	)

	peer, err := scanPeer(row)
	if err != nil {
		return nil, err
	}
	return peer, nil
}

func (db *DB) ListPeers(
	network string,
) (
	[]*service.Peer,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT
			name,
			public_key,
			route,
			admin,
			enabled,
			confirmed
		FROM peer
		WHERE network_name = ?1
		ORDER BY name ASC`,
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
	_, cidr, err := net.ParseCIDR(peer.Route)
	if err != nil {
		return fmt.Errorf("insert peer %q: parse route: %w", peer.Name, err)
	}

	ones, bits := cidr.Mask.Size()
	if ones != bits {
		return fmt.Errorf(
			"%w: peer route %q must be a terminal prefix (/%d)",
			service.ErrInvalidInput, peer.Route, bits,
		)
	}

	_, err = db.Conn.Exec(`
		INSERT INTO peer (
			network_name,
			name,
			public_key,
			route,
			admin,
			enabled,
			confirmed
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)`,
		network,
		peer.Name,
		peer.PublicKey,
		peer.Route,
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
			route,
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

	peer, err := scanPeer(row)
	if err != nil {
		return nil, err
	}
	return peer, nil
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

	if _, err := tx.Exec(`
		DELETE FROM registration
		WHERE final_route IN (
			SELECT route FROM peer
			WHERE network_name = ?1 AND name = ?2
		)`,
		network,
		name,
	); err != nil {
		return CheckSqliteErr("delete peer registration", err)
	}

	result, err := tx.Exec(`
		DELETE FROM peer
		WHERE network_name = ?1
			AND name = ?2`,
		network,
		name,
	)
	if err != nil {
		return CheckSqliteErr("delete peer", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete peer rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: peer %q not found", service.ErrNotFound, name)
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
	var publicKey string
	var route string
	var admin int64
	var enabled int64
	var confirmed int64

	if err := s.Scan(
		&name,
		&publicKey,
		&route,
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
		PublicKey: publicKey,
		Route:     route,
		Admin:     admin != 0,
		Enabled:   enabled != 0,
		Confirmed: confirmed != 0,
	}, nil
}
