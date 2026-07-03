package database

import (
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
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
			state,
			private_key,
			public_key,
			main_interface_name,
			invite_interface_name,
			assigned_cidr,
			server_pubkey,
			server_endpoint,
			server_api_addr,
			temp_priv_key,
			temp_cidr,
			invite_server_pubkey,
			invite_server_endpoint,
			temp_api_addr,
			enabled,
			created_at_unix
		FROM network
		WHERE name = ?1`,
		name,
	)

	var net service.Network
	var enabledInt int64
	var createdUnix int64
	if err := Scanner(row).Scan(
		&net.Name,
		&net.State,
		&net.PrivateKey,
		&net.PublicKey,
		&net.MainInterfaceName,
		&net.InviteInterfaceName,
		&net.AssignedCidr,
		&net.ServerPubkey,
		&net.ServerEndpoint,
		&net.ServerApiAddr,
		&net.TempPeerPrivKey,
		&net.TempPeerAssignedRoute,
		&net.InviteServerPubkey,
		&net.InviteServerEndpoint,
		&net.InviteServerAddr,
		&enabledInt,
		&createdUnix,
	); err != nil {
		return nil, CheckSqliteErr("scan network", err)
	}
	net.Enabled = enabledInt != 0
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

func (db *DB) InsertNetwork(
	network *service.Network,
) error {
	_, err := db.Conn.Exec(`
		INSERT INTO network (
			name,
			state,
			private_key,
			public_key,
			main_interface_name,
			invite_interface_name,
			assigned_cidr,
			server_pubkey,
			server_endpoint,
			server_api_addr,
			temp_priv_key,
			temp_cidr,
			invite_server_pubkey,
			invite_server_endpoint,
			temp_api_addr,
			enabled,
			created_at_unix
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17)`,
		network.Name,
		network.State,
		network.PrivateKey,
		network.PublicKey,
		network.MainInterfaceName,
		network.InviteInterfaceName,
		network.AssignedCidr,
		network.ServerPubkey,
		network.ServerEndpoint,
		network.ServerApiAddr,
		network.TempPeerPrivKey,
		network.TempPeerAssignedRoute,
		network.InviteServerPubkey,
		network.InviteServerEndpoint,
		network.InviteServerAddr,
		boolToInt(network.Enabled),
		network.CreatedAt.Unix(),
	)
	return CheckSqliteErr("insert network", err)
}

func (db *DB) SetNetworkRedeemed(
	name string,
	assignedCidr string,
	serverPubkey string,
	serverEndpoint string,
	serverApiAddr string,
) error {
	result, err := db.Conn.Exec(`
		UPDATE network
		SET
			state = 'redeemed',
			assigned_cidr = ?2,
			server_pubkey = ?3,
			server_endpoint = ?4,
			server_api_addr = ?5
		WHERE name = ?1`,
		name,
		assignedCidr,
		serverPubkey,
		serverEndpoint,
		serverApiAddr,
	)
	if err != nil {
		return CheckSqliteErr("set network redeemed", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set network redeemed: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: network %q not found", service.ErrNotFound, name)
	}
	return nil
}

func (db *DB) SetNetworkConfirmed(
	name string,
) error {
	result, err := db.Conn.Exec(`
		UPDATE network
		SET
			state = 'confirmed',
			temp_priv_key = '',
			temp_cidr = '',
			invite_server_pubkey = '',
			invite_server_endpoint = '',
			temp_api_addr = ''
		WHERE name = ?1`,
		name,
	)
	if err != nil {
		return CheckSqliteErr("set network confirmed", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set network confirmed: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: network %q not found", service.ErrNotFound, name)
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
