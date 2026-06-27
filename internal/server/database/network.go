package database

import (
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (db *DB) GetNetwork(
	name string,
) (
	*service.Network,
	error,
) {
	row := db.Conn.QueryRow(`
		SELECT
			name,
			main_name,
			invite_name,
			private_key,
			public_key,
			main_cidr,
			invite_cidr,
			external_ip,
			listen_port,
			invite_listen_port,
			api_port,
			enabled,
			created_at_unix
		FROM network
		WHERE name = ?1`,
		name,
	)

	var net service.Network
	var createdUnix int64
	if err := row.Scan(
		&net.Name,
		&net.MainName,
		&net.InviteName,
		&net.PrivateKey,
		&net.PublicKey,
		&net.MainCidr,
		&net.InviteCidr,
		&net.ExternalIP,
		&net.ListenPort,
		&net.InviteListenPort,
		&net.ApiPort,
		&net.Enabled,
		&createdUnix,
	); err != nil {
		return nil, CheckSqliteErr("get network", err)
	}

	net.CreatedAt = time.Unix(createdUnix, 0)
	return &net, nil
}

func (db *DB) ListNetworkNames() (
	[]string,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT name
		FROM network
		ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list network names: %w", err)
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan network name: %w", err)
		}
		names = append(names, name)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate network names: %w", err)
	}

	return names, nil
}

func (db *DB) BootstrapNetwork(
	network *service.Network,
	rootCidr *service.Cidr,
	serverPeer *service.Peer,
) error {
	_, rootIPNet, err := net.ParseCIDR(rootCidr.Cidr)
	if err != nil {
		return fmt.Errorf("parse root cidr: %w", err)
	}
	rootOnes, rootBits := rootIPNet.Mask.Size()
	rootFirst, rootLast := cidrFirstAndLast(rootIPNet)

	_, peerIPNet, err := net.ParseCIDR(serverPeer.Cidr)
	if err != nil {
		return fmt.Errorf("parse server peer cidr: %w", err)
	}
	peerIP := normalizeIP(peerIPNet.IP)
	peerOnes, _ := peerIPNet.Mask.Size()

	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin bootstrap tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO network (
			name,
			main_name,
			invite_name,
			private_key,
			public_key,
			main_cidr,
			invite_cidr,
			external_ip,
			listen_port,
			invite_listen_port,
			api_port,
			enabled,
			created_at_unix
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13)`,
		network.Name,
		network.MainName,
		network.InviteName,
		network.PrivateKey,
		network.PublicKey,
		network.MainCidr,
		network.InviteCidr,
		network.ExternalIP,
		network.ListenPort,
		network.InviteListenPort,
		network.ApiPort,
		boolToInt(network.Enabled),
		network.CreatedAt.Unix(),
	)
	if err != nil {
		return CheckSqliteErr("insert network", err)
	}

	_, err = tx.Exec(`
		INSERT INTO cidr (network_name, name, cidr, length, prefix, base, last)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7)`,
		network.Name,
		rootCidr.Name,
		rootCidr.Cidr,
		rootBits,
		rootOnes,
		rootFirst,
		rootLast,
	)
	if err != nil {
		return CheckSqliteErr("insert root cidr", err)
	}

	_, err = tx.Exec(`
		INSERT INTO peer (
			network_name,
			name,
			public_key,
			ip,
			prefix,
			admin,
			enabled,
			confirmed
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8)`,
		network.Name,
		serverPeer.Name,
		serverPeer.PublicKey,
		peerIP,
		peerOnes,
		boolToInt(serverPeer.Admin),
		boolToInt(serverPeer.Enabled),
		boolToInt(serverPeer.Confirmed),
	)
	if err != nil {
		return CheckSqliteErr("insert server peer", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit bootstrap tx: %w", err)
	}

	return nil
}

func (db *DB) SetNetworkEnabled(
	name string,
	enabled bool,
) error {
	result, err := db.Conn.Exec(`
		UPDATE network
		SET enabled = ?1
		WHERE name = ?2`,
		boolToInt(enabled),
		name,
	)
	if err != nil {
		return CheckSqliteErr("set network enabled", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set network enabled: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: network %q not found", service.ErrNotFound, name)
	}
	return nil
}

func (db *DB) DeleteNetwork(
	name string,
) error {
	result, err := db.Conn.Exec(`
		DELETE FROM network
		WHERE name = ?1`,
		name,
	)
	if err != nil {
		return CheckSqliteErr("delete network", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete network: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: network %q not found", service.ErrNotFound, name)
	}
	return nil
}
