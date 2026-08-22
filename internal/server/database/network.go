package database

import (
	"database/sql"
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
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
			external_ip,
			main_name,
			main_cidr,
			main_wg_port,
			main_api_port,
			invite_name,
			invite_cidr,
			invite_wg_port,
			invite_api_port,
			enabled,
			created_at_unix
		FROM network
		WHERE name = ?1`,
		name,
	)

	var network service.Network
	if err := scanNetwork(row, &network); err != nil {
		return nil, CheckSqliteErr("get network", err)
	}
	return &network, nil
}

func (db *DB) ListNetworks() (
	[]*service.Network,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT
			name,
			private_key,
			public_key,
			external_ip,
			main_name,
			main_cidr,
			main_wg_port,
			main_api_port,
			invite_name,
			invite_cidr,
			invite_wg_port,
			invite_api_port,
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
		var network service.Network
		if err := scanNetwork(rows, &network); err != nil {
			return nil, fmt.Errorf("scan network: %w", err)
		}
		networks = append(networks, &network)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate networks: %w", err)
	}

	return networks, nil
}

func (db *DB) BootstrapNetwork(
	network *service.Network,
	rootCidr *service.Cidr,
	serverCidr *service.Cidr,
	serverPeer *service.Peer,
) error {
	_, rootIPNet, err := net.ParseCIDR(rootCidr.Cidr)
	if err != nil {
		return fmt.Errorf("parse root cidr: %w", err)
	}
	rootOnes, rootBits := rootIPNet.Mask.Size()
	rootFirst, rootLast := netaddr.Range(rootIPNet)

	_, serverIPNet, err := net.ParseCIDR(serverCidr.Cidr)
	if err != nil {
		return fmt.Errorf("parse server cidr: %w", err)
	}
	serverOnes, serverBits := serverIPNet.Mask.Size()
	serverFirst, serverLast := netaddr.Range(serverIPNet)

	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin bootstrap tx: %w", err)
	}
	defer tx.Rollback()

	if err := sqlInsertNetworkTx(tx, network); err != nil {
		return err
	}

	if err := sqlInsertRootCidrTx(
		tx,
		network.Name,
		rootCidr,
		rootBits,
		rootOnes,
		rootFirst,
		rootLast,
	); err != nil {
		return err
	}

	serverCidrID, err := sqlInsertServerCidrTx(
		tx,
		network.Name,
		serverCidr,
		serverBits,
		serverOnes,
		serverFirst,
		serverLast,
	)
	if err != nil {
		return err
	}

	if err := sqlInsertBootstrapPeerTx(
		tx,
		network.Name,
		serverCidrID,
		serverPeer,
	); err != nil {
		return err
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
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin delete network tx: %w", err)
	}
	defer tx.Rollback()

	enabled, err := sqlGetNetworkEnabledTx(tx, name)
	if err != nil {
		return err
	}

	if enabled {
		return fmt.Errorf("%w: network %q", service.ErrNetworkEnabled, name)
	}

	if err := sqlDeleteNetworkTx(tx, name); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete network tx: %w", err)
	}
	return nil
}

func sqlGetNetworkEnabledTx(
	tx *sql.Tx,
	name string,
) (
	bool,
	error,
) {
	var enabled bool
	if err := tx.QueryRow(`
		SELECT enabled
		FROM network
		WHERE name = ?1`,
		name,
	).Scan(&enabled); err != nil {
		return false, CheckSqliteErr("get network to delete", err)
	}
	return enabled, nil
}

func sqlDeleteNetworkTx(
	tx *sql.Tx,
	name string,
) error {
	_, err := tx.Exec(`
		DELETE FROM network
		WHERE name = ?1`,
		name,
	)
	return CheckSqliteErr("delete network", err)
}

func sqlInsertNetworkTx(
	tx *sql.Tx,
	network *service.Network,
) error {
	_, err := tx.Exec(`
		INSERT INTO network (
			name,
			private_key,
			public_key,
			external_ip,
			main_name,
			main_cidr,
			main_wg_port,
			main_api_port,
			invite_name,
			invite_cidr,
			invite_wg_port,
			invite_api_port,
			enabled,
			created_at_unix
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11, ?12, ?13, ?14)`,
		network.Name,
		network.PrivateKey,
		network.PublicKey,
		network.ExternalIP,
		network.Main.Name,
		network.Main.Cidr,
		network.Main.WireguardPort,
		network.Main.ApiPort,
		network.Invite.Name,
		network.Invite.Cidr,
		network.Invite.WireguardPort,
		network.Invite.ApiPort,
		boolToInt(network.Enabled),
		network.CreatedAt.Unix(),
	)
	if err != nil {
		return CheckSqliteErr("insert network", err)
	}
	return nil
}

func sqlRequireNetworkTx(
	tx *sql.Tx,
	network string,
) error {
	var exists int
	if err := tx.QueryRow(`
		SELECT 1
		FROM network
		WHERE name = ?1`,
		network,
	).Scan(&exists); err != nil {
		return CheckSqliteErr("require network", err)
	}
	return nil
}

func sqlGetNetworkMainCidrTx(
	tx *sql.Tx,
	network string,
) (
	string,
	error,
) {
	var mainCIDR string
	if err := tx.QueryRow(`
		SELECT main_cidr
		FROM network
		WHERE name = ?1`,
		network,
	).Scan(&mainCIDR); err != nil {
		return "", CheckSqliteErr("get main CIDR", err)
	}
	return mainCIDR, nil
}

func sqlGetNetworkInviteCidrTx(
	tx *sql.Tx,
	network string,
) (
	string,
	error,
) {
	row := tx.QueryRow(`
		SELECT invite_cidr
		FROM network
		WHERE name = ?1`,
		network,
	)

	var inviteCIDR string
	if err := row.Scan(&inviteCIDR); err != nil {
		return "", CheckSqliteErr("get invite CIDR", err)
	}

	return inviteCIDR, nil
}

func sqlValidateRegistrationMainRouteTx(
	tx *sql.Tx,
	network string,
	mainRoute string,
) error {
	var mainCIDR string
	if err := tx.QueryRow(
		`SELECT main_cidr FROM network WHERE name = ?1`,
		network,
	).Scan(&mainCIDR); err != nil {
		return CheckSqliteErr("get main CIDR for registration", err)
	}

	_, persistedMainNet, err := net.ParseCIDR(mainCIDR)
	if err != nil {
		return fmt.Errorf("parse persisted main CIDR %q: %w", mainCIDR, err)
	}

	_, registrationNet, err := net.ParseCIDR(mainRoute)
	if err != nil {
		return fmt.Errorf("parse registration main route %q: %w", mainRoute, err)
	}

	if !netaddr.Contains(persistedMainNet, registrationNet) {
		return fmt.Errorf(
			"%w: registration main route %q is not contained within main CIDR %q",
			service.ErrInvalidInput,
			mainRoute,
			mainCIDR,
		)
	}

	return nil
}

func scanNetwork(
	scanner Scanner,
	network *service.Network,
) error {
	var createdUnix int64
	if err := scanner.Scan(
		&network.Name,
		&network.PrivateKey,
		&network.PublicKey,
		&network.ExternalIP,
		&network.Main.Name,
		&network.Main.Cidr,
		&network.Main.WireguardPort,
		&network.Main.ApiPort,
		&network.Invite.Name,
		&network.Invite.Cidr,
		&network.Invite.WireguardPort,
		&network.Invite.ApiPort,
		&network.Enabled,
		&createdUnix,
	); err != nil {
		return err
	}

	network.CreatedAt = time.Unix(createdUnix, 0)
	return nil
}
