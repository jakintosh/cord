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
			private_key,
			public_key,
			assigned_cidr,
			server_pubkey,
			server_endpoint,
			server_api_addr,
			enabled,
			created_at_unix
		FROM network
		WHERE name = ?1`,
		name,
	)

	var net service.Network
	var enabledInt int64
	var createdUnix int64
	if err := row.Scan(
		&net.Name,
		&net.PrivateKey,
		&net.PublicKey,
		&net.AssignedCidr,
		&net.ServerPubkey,
		&net.ServerEndpoint,
		&net.ServerApiAddr,
		&enabledInt,
		&createdUnix,
	); err != nil {
		return nil, CheckSqliteErr("get network", err)
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
			private_key,
			public_key,
			assigned_cidr,
			server_pubkey,
			server_endpoint,
			server_api_addr,
			enabled,
			created_at_unix
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9)`,
		network.Name,
		network.PrivateKey,
		network.PublicKey,
		network.AssignedCidr,
		network.ServerPubkey,
		network.ServerEndpoint,
		network.ServerApiAddr,
		boolToInt(network.Enabled),
		network.CreatedAt.Unix(),
	)
	return CheckSqliteErr("insert network", err)
}

func (db *DB) UpdateNetwork(
	name string,
	req service.UpdateNetworkRequest,
) (
	*service.Network,
	error,
) {
	if req.Enabled != nil {
		_, err := db.Conn.Exec(`
			UPDATE network
			SET enabled = ?1
			WHERE name = ?2`,
			boolToInt(*req.Enabled),
			name,
		)
		if err != nil {
			return nil, CheckSqliteErr("update network enabled", err)
		}
	}

	return db.GetNetwork(name)
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
