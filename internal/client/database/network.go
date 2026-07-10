package database

import (
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

func (db *DB) GetNetwork(
	name string,
) (
	*service.NetworkConfig,
	error,
) {
	row := db.Conn.QueryRow(`
		SELECT
			name,
			peer_private_key,
			interface_name,
			peer_route,
			server_pubkey,
			server_endpoint,
			server_route,
			server_network_cidr,
			server_api_port,
			listen_port,
			enabled,
			created_at_unix
		FROM network
		WHERE name = ?1`,
		name,
	)

	var nc service.NetworkConfig
	var enabledInt int64
	var createdUnix int64
	if err := Scanner(row).Scan(
		&nc.Name,
		&nc.PrivateKey,
		&nc.InterfaceName,
		&nc.AssignedRoute,
		&nc.Server.PublicKey,
		&nc.Server.Endpoint,
		&nc.Server.Route,
		&nc.Server.NetworkCidr,
		&nc.Server.APIPort,
		&nc.ListenPort,
		&enabledInt,
		&createdUnix,
	); err != nil {
		return nil, CheckSqliteErr("scan network", err)
	}
	nc.Enabled = enabledInt != 0
	nc.CreatedAt = time.Unix(createdUnix, 0)
	return &nc, nil
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
	nc *service.NetworkConfig,
) error {
	_, err := db.Conn.Exec(`
		INSERT INTO network (
			name,
			peer_private_key,
			interface_name,
			peer_route,
			server_pubkey,
			server_endpoint,
			server_route,
			server_network_cidr,
			server_api_port,
			listen_port,
			enabled,
			created_at_unix
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12)`,
		nc.Name,
		nc.PrivateKey,
		nc.InterfaceName,
		nc.AssignedRoute,
		nc.Server.PublicKey,
		nc.Server.Endpoint,
		nc.Server.Route,
		nc.Server.NetworkCidr,
		nc.Server.APIPort,
		nc.ListenPort,
		boolToInt(nc.Enabled),
		nc.CreatedAt.Unix(),
	)
	return CheckSqliteErr("insert network", err)
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

func (db *DB) SetNetworkListenPort(
	name string,
	listenPort uint16,
) error {
	result, err := db.Conn.Exec(`
		UPDATE network
		SET listen_port = ?1
		WHERE name = ?2`,
		listenPort,
		name,
	)
	if err != nil {
		return CheckSqliteErr("set network listen port", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("set network listen port: %w", err)
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
