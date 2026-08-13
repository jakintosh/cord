package database

import (
	"database/sql"
	"errors"
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
			invite_server_network_cidr,
			invite_server_api_port,
			main_iface_name,
			main_peer_private_key,
			main_peer_route,
			main_server_pubkey,
			main_server_endpoint,
			main_server_route,
			main_server_network_cidr,
			main_server_api_port,
			listen_port,
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
			invite_server_network_cidr,
			invite_server_api_port,
			main_iface_name,
			main_peer_private_key,
			main_peer_route,
			main_server_pubkey,
			main_server_endpoint,
			main_server_route,
			main_server_network_cidr,
			main_server_api_port,
			listen_port,
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

func (db *DB) BeginInstall(
	params service.BeginInstallParams,
) (
	*service.Install,
	error,
) {
	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin install tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := sqlGetNetworkTx(tx, params.Name); err == nil {
		return nil, fmt.Errorf(
			"%w: network %q is already installed",
			service.ErrNetworkExists,
			params.Name,
		)
	} else if !errors.Is(err, service.ErrNotFound) {
		return nil, err
	}

	existing, err := sqlGetInstallTx(tx, params.Name)
	if err == nil {
		if !installMatchesBegin(existing, params) {
			return nil, fmt.Errorf(
				"%w: install %q has different invitation or options",
				service.ErrConflict,
				params.Name,
			)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit existing install tx: %w", err)
		}
		return existing, nil
	}
	if !errors.Is(err, service.ErrNotFound) {
		return nil, err
	}

	install := &service.Install{
		Name:                params.Name,
		Phase:               service.PhaseInvited,
		ListenPort:          params.ListenPort,
		InviteIfaceName:     params.InviteIfaceName,
		InvitePrivateKey:    params.InvitePrivateKey,
		InviteAssignedRoute: params.InviteAssignedRoute,
		InviteServer:        params.InviteServer,
		MainIfaceName:       params.MainIfaceName,
		MainPrivateKey:      params.MainPrivateKey,
		CreatedAt:           params.CreatedAt,
	}
	if err := sqlInsertInstallTx(tx, install); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit begin install tx: %w", err)
	}
	return install, nil
}

func (db *DB) RedeemInstall(
	name string,
	assignment service.NetworkAssignment,
) (
	*service.Install,
	error,
) {
	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin redeem install tx: %w", err)
	}
	defer tx.Rollback()

	install, err := sqlGetInstallTx(tx, name)
	if errors.Is(err, service.ErrNotFound) {
		if _, networkErr := sqlGetNetworkTx(tx, name); networkErr == nil {
			return nil, fmt.Errorf(
				"%w: install %q is already complete",
				service.ErrInstallState,
				name,
			)
		} else if !errors.Is(networkErr, service.ErrNotFound) {
			return nil, networkErr
		}
		return nil, fmt.Errorf("%w: install %q", service.ErrNotFound, name)
	}
	if err != nil {
		return nil, err
	}

	switch install.Phase {
	case service.PhaseInvited:
		if err := sqlRedeemInstallTx(tx, name, assignment); err != nil {
			return nil, err
		}

		install.Phase = service.PhaseRedeemed
		install.MainAssignedRoute = assignment.AssignedRoute
		install.MainServer = assignment.Server

	case service.PhaseRedeemed:
		if install.MainAssignedRoute != assignment.AssignedRoute ||
			install.MainServer != assignment.Server {
			return nil, fmt.Errorf(
				"%w: install %q was redeemed with different network identity",
				service.ErrConflict,
				name,
			)
		}

	default:
		return nil, fmt.Errorf(
			"%w: install %q is in phase %q",
			service.ErrInstallState,
			name,
			install.Phase,
		)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit redeem install tx: %w", err)
	}
	return install, nil
}

func (db *DB) ConfirmInstall(
	name string,
	mainPrivateKey string,
	confirmedAt time.Time,
) (
	*service.NetworkConfig,
	error,
) {
	tx, err := db.Conn.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin confirm install tx: %w", err)
	}
	defer tx.Rollback()

	network, err := sqlGetNetworkTx(tx, name)
	if err == nil {
		if network.PrivateKey != mainPrivateKey {
			return nil, fmt.Errorf(
				"%w: network %q has a different permanent identity",
				service.ErrConflict,
				name,
			)
		}
		if err := tx.Commit(); err != nil {
			return nil, fmt.Errorf("commit confirmed install retry tx: %w", err)
		}
		return network, nil
	}
	if !errors.Is(err, service.ErrNotFound) {
		return nil, err
	}

	install, err := sqlGetInstallTx(tx, name)
	if err != nil {
		if errors.Is(err, service.ErrNotFound) {
			return nil, fmt.Errorf("%w: install %q", service.ErrNotFound, name)
		}
		return nil, err
	}

	if install.Phase != service.PhaseRedeemed {
		return nil, fmt.Errorf(
			"%w: install %q is in phase %q",
			service.ErrInstallState,
			name,
			install.Phase,
		)
	}

	if install.MainPrivateKey != mainPrivateKey {
		return nil, fmt.Errorf(
			"%w: install %q has a different permanent identity",
			service.ErrConflict,
			name,
		)
	}

	network = &service.NetworkConfig{
		Name:          install.Name,
		PrivateKey:    install.MainPrivateKey,
		InterfaceName: install.MainIfaceName,
		AssignedRoute: install.MainAssignedRoute,
		ListenPort:    install.ListenPort,
		Server:        install.MainServer,
		Enabled:       true,
		CreatedAt:     confirmedAt,
	}

	if err := sqlInsertNetworkTx(tx, network); err != nil {
		return nil, err
	}

	if err := sqlDeleteRedeemedInstallTx(tx, name); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit confirm install tx: %w", err)
	}
	return network, nil
}

func sqlGetInstallTx(
	tx *sql.Tx,
	name string,
) (
	*service.Install,
	error,
) {
	row := tx.QueryRow(`
		SELECT
			name,
			phase,
			invite_iface_name,
			invite_peer_private_key,
			invite_peer_route,
			invite_server_pubkey,
			invite_server_endpoint,
			invite_server_route,
			invite_server_network_cidr,
			invite_server_api_port,
			main_iface_name,
			main_peer_private_key,
			main_peer_route,
			main_server_pubkey,
			main_server_endpoint,
			main_server_route,
			main_server_network_cidr,
			main_server_api_port,
			listen_port,
			created_at_unix
		FROM install
		WHERE name = ?1`,
		name,
	)

	var install service.Install
	if err := scanInstallRow(row, &install); err != nil {
		return nil, CheckSqliteErr("get install", err)
	}
	return &install, nil
}

func sqlInsertInstallTx(
	tx *sql.Tx,
	install *service.Install,
) error {
	_, err := tx.Exec(`
		INSERT INTO install (
			name,
			phase,
			invite_iface_name,
			invite_peer_private_key,
			invite_peer_route,
			invite_server_pubkey,
			invite_server_endpoint,
			invite_server_route,
			invite_server_network_cidr,
			invite_server_api_port,
			main_iface_name,
			main_peer_private_key,
			main_peer_route,
			main_server_pubkey,
			main_server_endpoint,
			main_server_route,
			main_server_network_cidr,
			main_server_api_port,
			listen_port,
			created_at_unix
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14, ?15, ?16, ?17, ?18, ?19, ?20)`,
		install.Name,
		install.Phase,
		install.InviteIfaceName,
		install.InvitePrivateKey,
		install.InviteAssignedRoute,
		install.InviteServer.PublicKey,
		install.InviteServer.Endpoint,
		install.InviteServer.Route,
		install.InviteServer.NetworkCidr,
		install.InviteServer.APIPort,
		install.MainIfaceName,
		install.MainPrivateKey,
		install.MainAssignedRoute,
		install.MainServer.PublicKey,
		install.MainServer.Endpoint,
		install.MainServer.Route,
		install.MainServer.NetworkCidr,
		install.MainServer.APIPort,
		install.ListenPort,
		install.CreatedAt.Unix(),
	)
	return CheckSqliteErr("insert install", err)
}

func sqlRedeemInstallTx(
	tx *sql.Tx,
	name string,
	assignment service.NetworkAssignment,
) error {
	result, err := tx.Exec(`
		UPDATE install
		SET
			phase = 'redeemed',
			main_peer_route = ?2,
			main_server_pubkey = ?3,
			main_server_endpoint = ?4,
			main_server_route = ?5,
			main_server_network_cidr = ?6,
			main_server_api_port = ?7
		WHERE name = ?1 AND phase = 'invited'`,
		name,
		assignment.AssignedRoute,
		assignment.Server.PublicKey,
		assignment.Server.Endpoint,
		assignment.Server.Route,
		assignment.Server.NetworkCidr,
		assignment.Server.APIPort,
	)
	if err != nil {
		return CheckSqliteErr("redeem install", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("redeem install rows affected: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf(
			"%w: install %q changed phase",
			service.ErrInstallState,
			name,
		)
	}

	return nil
}

func sqlDeleteInstallTx(
	tx *sql.Tx,
	name string,
) (
	int64,
	error,
) {
	result, err := tx.Exec(`
		DELETE FROM install
		WHERE name = ?1`,
		name,
	)
	if err != nil {
		return 0, CheckSqliteErr("delete install state", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("delete install state rows affected: %w", err)
	}
	return affected, nil
}

func sqlDeleteRedeemedInstallTx(
	tx *sql.Tx,
	name string,
) error {
	result, err := tx.Exec(`
		DELETE FROM install
		WHERE name = ?1 AND phase = 'redeemed'`,
		name,
	)
	if err != nil {
		return CheckSqliteErr("confirm delete install", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm delete install rows affected: %w", err)
	}
	if affected != 1 {
		return fmt.Errorf(
			"%w: install %q changed phase",
			service.ErrInstallState,
			name,
		)
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
		&inst.InviteServer.NetworkCidr,
		&inst.InviteServer.APIPort,
		&inst.MainIfaceName,
		&inst.MainPrivateKey,
		&inst.MainAssignedRoute,
		&inst.MainServer.PublicKey,
		&inst.MainServer.Endpoint,
		&inst.MainServer.Route,
		&inst.MainServer.NetworkCidr,
		&inst.MainServer.APIPort,
		&inst.ListenPort,
		&createdUnix,
	); err != nil {
		return err
	}
	inst.CreatedAt = time.Unix(createdUnix, 0)
	return nil
}

func installMatchesBegin(
	install *service.Install,
	params service.BeginInstallParams,
) bool {
	return install.Name == params.Name &&
		install.ListenPort == params.ListenPort &&
		install.InviteIfaceName == params.InviteIfaceName &&
		install.InvitePrivateKey == params.InvitePrivateKey &&
		install.InviteAssignedRoute == params.InviteAssignedRoute &&
		install.InviteServer == params.InviteServer &&
		install.MainIfaceName == params.MainIfaceName
}
