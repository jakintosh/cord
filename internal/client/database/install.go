package database

import (
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

func (db *DB) GetInstall(
	name string,
) (
	*service.Install,
	error,
) {
	row := db.Conn.QueryRow(`
		SELECT
			name,
			phase,
			invite_iface_name,
			invite_peer_private_key,
			invite_peer_route,
			invite_server_pubkey,
			invite_server_endpoint,
			invite_server_route,
			invite_server_api_port,
			main_iface_name,
			main_peer_private_key,
			main_peer_route,
			main_server_pubkey,
			main_server_endpoint,
			main_server_route,
			main_server_api_port,
			created_at_unix
		FROM install
		WHERE name = ?1`,
		name,
	)

	var inst service.Install
	if err := scanInstallRow(row, &inst); err != nil {
		return nil, CheckSqliteErr("scan install", err)
	}
	return &inst, nil
}

func (db *DB) ListInstalls() (
	[]*service.Install,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT
			name,
			phase,
			invite_iface_name,
			invite_peer_private_key,
			invite_peer_route,
			invite_server_pubkey,
			invite_server_endpoint,
			invite_server_route,
			invite_server_api_port,
			main_iface_name,
			main_peer_private_key,
			main_peer_route,
			main_server_pubkey,
			main_server_endpoint,
			main_server_route,
			main_server_api_port,
			created_at_unix
		FROM install
		ORDER BY name ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list installs: %w", err)
	}
	defer rows.Close()

	var installs []*service.Install
	for rows.Next() {
		var inst service.Install
		if err := scanInstallRow(rows, &inst); err != nil {
			return nil, fmt.Errorf("scan install: %w", err)
		}
		installs = append(installs, &inst)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate installs: %w", err)
	}

	return installs, nil
}

func (db *DB) InsertInstall(
	install *service.Install,
) error {
	_, err := db.Conn.Exec(`
		INSERT INTO install (
			name,
			phase,
			invite_iface_name,
			invite_peer_private_key,
			invite_peer_route,
			invite_server_pubkey,
			invite_server_endpoint,
			invite_server_route,
			invite_server_api_port,
			main_iface_name,
			main_peer_private_key,
			main_peer_route,
			main_server_pubkey,
			main_server_endpoint,
			main_server_route,
			main_server_api_port,
			created_at_unix
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17)`,
		install.Name,
		install.Phase,
		install.InviteIfaceName,
		install.InvitePrivateKey,
		install.InviteAssignedRoute,
		install.InviteServer.PublicKey,
		install.InviteServer.Endpoint,
		install.InviteServer.Route,
		install.InviteServer.APIPort,
		install.MainIfaceName,
		install.MainPrivateKey,
		install.MainAssignedRoute,
		install.MainServer.PublicKey,
		install.MainServer.Endpoint,
		install.MainServer.Route,
		install.MainServer.APIPort,
		install.CreatedAt.Unix(),
	)
	return CheckSqliteErr("insert install", err)
}

func (db *DB) ConfirmInstall(
	name string,
	nc *service.NetworkConfig,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin confirm tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.Exec(`
		DELETE FROM install
		WHERE name = ?1`,
		name,
	)
	if err != nil {
		return CheckSqliteErr("confirm delete install", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm delete install: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: install %q not found", service.ErrNotFound, name)
	}

	_, err = tx.Exec(`
		INSERT INTO network (
			name,
			peer_private_key,
			interface_name,
			peer_route,
			server_pubkey,
			server_endpoint,
			server_route,
			server_api_port,
			enabled,
			created_at_unix
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10)`,
		nc.Name,
		nc.PrivateKey,
		nc.InterfaceName,
		nc.AssignedRoute,
		nc.Server.PublicKey,
		nc.Server.Endpoint,
		nc.Server.Route,
		nc.Server.APIPort,
		boolToInt(nc.Enabled),
		nc.CreatedAt.Unix(),
	)
	if err != nil {
		return CheckSqliteErr("confirm insert network", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit confirm tx: %w", err)
	}
	return nil
}

func (db *DB) RedeemInstall(
	name string,
	assignedRoute string,
	server service.ServerInfo,
) error {
	result, err := db.Conn.Exec(`
		UPDATE install
		SET
			phase = 'redeemed',
			main_peer_route = ?2,
			main_server_pubkey = ?3,
			main_server_endpoint = ?4,
			main_server_route = ?5,
			main_server_api_port = ?6
		WHERE name = ?1`,
		name,
		assignedRoute,
		server.PublicKey,
		server.Endpoint,
		server.Route,
		server.APIPort,
	)
	if err != nil {
		return CheckSqliteErr("redeem install", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("redeem install: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: install %q not found", service.ErrNotFound, name)
	}
	return nil
}

func (db *DB) DeleteInstall(
	name string,
) error {
	result, err := db.Conn.Exec(`
		DELETE FROM install
		WHERE name = ?1`,
		name,
	)
	if err != nil {
		return CheckSqliteErr("delete install", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete install: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: install %q not found", service.ErrNotFound, name)
	}
	return nil
}

func scanInstallRow(
	scanner Scanner,
	inst *service.Install,
) error {
	var createdUnix int64
	if err := scanner.Scan(
		&inst.Name,
		&inst.Phase,
		&inst.InviteIfaceName,
		&inst.InvitePrivateKey,
		&inst.InviteAssignedRoute,
		&inst.InviteServer.PublicKey,
		&inst.InviteServer.Endpoint,
		&inst.InviteServer.Route,
		&inst.InviteServer.APIPort,
		&inst.MainIfaceName,
		&inst.MainPrivateKey,
		&inst.MainAssignedRoute,
		&inst.MainServer.PublicKey,
		&inst.MainServer.Endpoint,
		&inst.MainServer.Route,
		&inst.MainServer.APIPort,
		&createdUnix,
	); err != nil {
		return err
	}
	inst.CreatedAt = time.Unix(createdUnix, 0)
	return nil
}
