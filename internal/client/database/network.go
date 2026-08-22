package database

import (
	"database/sql"
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

	var nc service.Network
	if err := scanNetworkRow(row, &nc); err != nil {
		return nil, CheckSqliteErr("scan network", err)
	}
	return &nc, nil
}

func (db *DB) ListNetworks() (
	[]*service.Network,
	error,
) {
	rows, err := db.Conn.Query(`
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
		ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	defer rows.Close()

	var networks []*service.Network
	for rows.Next() {
		var nc service.Network
		if err := scanNetworkRow(rows, &nc); err != nil {
			return nil, fmt.Errorf("scan network: %w", err)
		}
		networks = append(networks, &nc)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate networks: %w", err)
	}

	return networks, nil
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

func (db *DB) UpdateNetwork(
	name string,
	update service.NetworkOptions,
) error {
	if update.ListenPort == nil {
		return service.ErrInvalidInput
	}
	result, err := db.Conn.Exec(`
		UPDATE network
		SET listen_port = ?1
		WHERE name = ?2`,
		*update.ListenPort,
		name,
	)
	if err != nil {
		return CheckSqliteErr("update network", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update network: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: network %q not found", service.ErrNotFound, name)
	}
	return nil
}

func (db *DB) DeleteNetworkState(
	name string,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin delete network state tx: %w", err)
	}
	defer tx.Rollback()

	networkAffected, err := sqlDeleteNetworkTx(tx, name)
	if err != nil {
		return err
	}

	installAffected, err := sqlDeleteInstallTx(tx, name)
	if err != nil {
		return err
	}

	if networkAffected+installAffected == 0 {
		return fmt.Errorf("%w: network %q not found", service.ErrNotFound, name)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete network state tx: %w", err)
	}
	return nil
}

func sqlGetNetworkTx(
	tx *sql.Tx,
	name string,
) (
	*service.Network,
	error,
) {
	row := tx.QueryRow(`
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

	var network service.Network
	if err := scanNetworkRow(row, &network); err != nil {
		return nil, CheckSqliteErr("get network", err)
	}
	return &network, nil
}

func sqlRequireNetworkTx(
	tx *sql.Tx,
	name string,
) error {
	var exists int
	if err := tx.QueryRow(`
		SELECT 1
		FROM network
		WHERE name = ?1`,
		name,
	).Scan(&exists); err != nil {
		return CheckSqliteErr("require network", err)
	}
	return nil
}

func sqlInsertNetworkTx(
	tx *sql.Tx,
	nc *service.Network,
) error {
	_, err := tx.Exec(`
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

func sqlDeleteNetworkTx(
	tx *sql.Tx,
	name string,
) (
	int64,
	error,
) {
	result, err := tx.Exec(`
		DELETE FROM network
		WHERE name = ?1`,
		name,
	)
	if err != nil {
		return 0, CheckSqliteErr("delete network state", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete network state rows affected: %w", err)
	}
	return affected, nil
}

func scanNetworkRow(
	scanner Scanner,
	nc *service.Network,
) error {
	var enabledInt int64
	var createdUnix int64
	if err := scanner.Scan(
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
		return err
	}
	nc.Enabled = enabledInt != 0
	nc.CreatedAt = time.Unix(createdUnix, 0)
	return nil
}
