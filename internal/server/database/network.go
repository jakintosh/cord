package database

import (
	"fmt"
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
			private_key,
			public_key,
			root_cidr,
			invite_cidr,
			external_ip,
			listen_port,
			invite_listen_port,
			api_port,
			created_at_unix
		FROM network
		WHERE name = ?1`,
		name,
	)

	var net service.Network
	var createdUnix int64
	if err := row.Scan(
		&net.Name,
		&net.PrivateKey,
		&net.PublicKey,
		&net.RootCidr,
		&net.InviteCidr,
		&net.ExternalIP,
		&net.ListenPort,
		&net.InviteListenPort,
		&net.ApiPort,
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

func (db *DB) InsertNetwork(
	network *service.Network,
) error {
	_, err := db.Conn.Exec(`
		INSERT INTO network (
			name,
			private_key,
			public_key,
			root_cidr,
			invite_cidr,
			external_ip,
			listen_port,
			invite_listen_port,
			api_port,
			created_at_unix
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)`,
		network.Name,
		network.PrivateKey,
		network.PublicKey,
		network.RootCidr,
		network.InviteCidr,
		network.ExternalIP,
		network.ListenPort,
		network.InviteListenPort,
		network.ApiPort,
		network.CreatedAt.Unix(),
	)
	return CheckSqliteErr("insert network", err)
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
