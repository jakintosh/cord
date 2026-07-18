package database

import (
	"database/sql"
	"errors"
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (db *DB) GetRegistration(
	network string,
	name string,
) (
	*service.Registration,
	error,
) {
	row := db.Conn.QueryRow(`
		SELECT
			r.name,
			r.temp_public_key,
			r.temp_route,
			r.final_route,
			r.admin,
			r.redeemed,
			r.redeemed_key,
			r.confirmed,
			r.expires_at_unix,
			r.created_at_unix
		FROM registration r
		WHERE r.network_name = ?1
			AND r.name = ?2`,
		network,
		name,
	)

	return scanRegistration(row)
}

func (db *DB) GetRegistrationByIP(
	network string,
	ip net.IP,
	now time.Time,
) (
	*service.Registration,
	error,
) {
	route := netaddr.HostRoute(netaddr.Normalize(ip))
	row := db.Conn.QueryRow(`
		SELECT
			r.name,
			r.temp_public_key,
			r.temp_route,
			r.final_route,
			r.admin,
			r.redeemed,
			r.redeemed_key,
			r.confirmed,
			r.expires_at_unix,
			r.created_at_unix
		FROM registration r
		WHERE r.network_name = ?1
			AND r.temp_route = ?2
			AND r.confirmed = 0
			AND r.expires_at_unix > ?3`,
		network,
		route.String(),
		now.Unix(),
	)

	return scanRegistration(row)
}

func (db *DB) ListRegistrations(
	network string,
) (
	[]*service.Registration,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT
			r.name,
			r.temp_public_key,
			r.temp_route,
			r.final_route,
			r.admin,
			r.redeemed,
			r.redeemed_key,
			r.confirmed,
			r.expires_at_unix,
			r.created_at_unix
		FROM registration r
		WHERE r.network_name = ?1
		ORDER BY r.created_at_unix DESC`,
		network,
	)
	if err != nil {
		return nil, CheckSqliteErr("list registrations", err)
	}
	defer rows.Close()

	var regs []*service.Registration
	for rows.Next() {
		reg, err := scanRegistration(rows)
		if err != nil {
			return nil, err
		}
		regs = append(regs, reg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate registrations: %w", err)
	}

	return regs, nil
}

func (db *DB) ListActiveRegistrations(
	network string,
	now time.Time,
) (
	[]*service.Registration,
	error,
) {
	rows, err := db.Conn.Query(`
		SELECT
			r.name,
			r.temp_public_key,
			r.temp_route,
			r.final_route,
			r.admin,
			r.redeemed,
			r.redeemed_key,
			r.confirmed,
			r.expires_at_unix,
			r.created_at_unix
		FROM registration r
		WHERE r.network_name = ?1
			AND r.confirmed = 0
			AND r.expires_at_unix > ?2
		ORDER BY r.created_at_unix DESC`,
		network,
		now.Unix(),
	)
	if err != nil {
		return nil, CheckSqliteErr("list active registrations", err)
	}
	defer rows.Close()

	var regs []*service.Registration
	for rows.Next() {
		reg, err := scanRegistration(rows)
		if err != nil {
			return nil, err
		}
		regs = append(regs, reg)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active registrations: %w", err)
	}

	return regs, nil
}

func (db *DB) InsertRegistration(
	network string,
	reg *service.Registration,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin insert registration tx: %w", err)
	}
	defer tx.Rollback()

	_, mainNet, err := net.ParseCIDR(reg.MainRoute)
	if err != nil {
		return fmt.Errorf("parse registration main route %q: %w", reg.MainRoute, err)
	}
	ones, bits := mainNet.Mask.Size()
	base, _ := cidrFirstAndLast(mainNet)

	row := tx.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM cidr
			WHERE network_name = ?1
				AND (
					name = ?2
					OR (base = ?3 AND prefix = ?4 AND length = ?5)
				)
		)`,
		network,
		reg.Name,
		base,
		ones,
		bits,
	)
	var cidrConflict int
	if err := row.Scan(&cidrConflict); err != nil {
		return CheckSqliteErr("check registration CIDR reservation", err)
	}
	if cidrConflict != 0 {
		return fmt.Errorf(
			"%w: registration name or route conflicts with an existing CIDR",
			service.ErrConflict,
		)
	}

	_, err = tx.Exec(`
		INSERT INTO registration (
			network_name,
			name,
			temp_public_key,
			temp_route,
			final_route,
			admin,
			redeemed,
			redeemed_key,
			confirmed,
			expires_at_unix,
			created_at_unix
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ?10, ?11)`,
		network,
		reg.Name,
		reg.InvitePublicKey,
		reg.InviteRoute,
		reg.MainRoute,
		boolToInt(reg.Admin),
		boolToInt(reg.Redeemed),
		reg.RedeemedKey,
		boolToInt(reg.Confirmed),
		reg.ExpiresAt.Unix(),
		reg.CreatedAt.Unix(),
	)
	if err != nil {
		return CheckSqliteErr("insert registration", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit insert registration tx: %w", err)
	}
	return nil
}

func (db *DB) RedeemRegistration(
	network string,
	tempPubKey string,
	permPubKey string,
	now time.Time,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin redeem tx: %w", err)
	}
	defer tx.Rollback()

	var registrationID int64
	var name string
	var mainRoute string
	var admin int64
	row := tx.QueryRow(`
		SELECT id, name, final_route, admin
		FROM registration
		WHERE network_name = ?1
			AND temp_public_key = ?2
			AND redeemed = 0
			AND confirmed = 0
			AND expires_at_unix > ?3`,
		network,
		tempPubKey,
		now.Unix(),
	)
	if err := row.Scan(&registrationID, &name, &mainRoute, &admin); err != nil {
		return CheckSqliteErr("find redeemable registration", err)
	}

	_, cidrNet, err := net.ParseCIDR(mainRoute)
	if err != nil {
		return fmt.Errorf("parse main route %q: %w", mainRoute, err)
	}
	ones, bits := cidrNet.Mask.Size()
	first, last := cidrFirstAndLast(cidrNet)

	result, err := tx.Exec(`
		INSERT INTO cidr (
			network_name,
			name,
			cidr,
			length,
			prefix,
			base,
			last,
			terminal
		)
		VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, 1)`,
		network,
		name,
		mainRoute,
		bits,
		ones,
		first,
		last,
	)
	if err != nil {
		return CheckSqliteErr("redeem create CIDR", err)
	}
	cidrID, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("get redeemed CIDR id: %w", err)
	}

	_, err = tx.Exec(`
		INSERT INTO peer (
			network_name,
			name,
			cidr_id,
			public_key,
			admin,
			enabled,
			confirmed
		)
		VALUES (?1, ?2, ?3, ?4, ?5, 1, 0)`,
		network,
		name,
		cidrID,
		permPubKey,
		admin,
	)
	if err != nil {
		return CheckSqliteErr("redeem create peer", err)
	}

	if _, err := tx.Exec(`
		UPDATE registration
		SET redeemed = 1, redeemed_key = ?2
		WHERE id = ?1 AND redeemed = 0`,
		registrationID,
		permPubKey,
	); err != nil {
		return CheckSqliteErr("redeem mark registration", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit redeem tx: %w", err)
	}

	return nil
}

func (db *DB) ListRegistrationGroups(
	network string,
	registration string,
) (
	[]*service.Group,
	error,
) {
	row := db.Conn.QueryRow(`
		SELECT id FROM registration
		WHERE network_name = ?1 AND name = ?2`,
		network,
		registration,
	)
	var registrationID int64
	if err := row.Scan(&registrationID); err != nil {
		return nil, CheckSqliteErr("find registration for group list", err)
	}

	rows, err := db.Conn.Query(`
		SELECT g.name
		FROM registration_assignment a
		JOIN "group" g ON g.id = a.group_id
		WHERE a.registration_id = ?1
		ORDER BY g.name`,
		registrationID,
	)
	if err != nil {
		return nil, CheckSqliteErr("list registration groups", err)
	}
	defer rows.Close()

	var groups []*service.Group
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, CheckSqliteErr("scan registration group", err)
		}
		groups = append(groups, &service.Group{Name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate registration groups: %w", err)
	}
	return groups, nil
}

func (db *DB) AssignRegistrationGroup(
	network string,
	registration string,
	group string,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin assign registration group tx: %w", err)
	}
	defer tx.Rollback()

	registrationID, err := lookupMutableRegistrationTx(tx, network, registration)
	if err != nil {
		return err
	}

	row := tx.QueryRow(`
		SELECT id FROM "group"
		WHERE network_name = ?1 AND name = ?2`,
		network,
		group,
	)
	var groupID int64
	if err := row.Scan(&groupID); err != nil {
		return CheckSqliteErr("find group for registration assignment", err)
	}

	if _, err := tx.Exec(`
		INSERT INTO registration_assignment (registration_id, group_id)
		VALUES (?1, ?2)`,
		registrationID,
		groupID,
	); err != nil {
		return CheckSqliteErr("assign registration group", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit assign registration group tx: %w", err)
	}
	return nil
}

func (db *DB) RemoveRegistrationGroup(
	network string,
	registration string,
	group string,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin remove registration group tx: %w", err)
	}
	defer tx.Rollback()

	registrationID, err := lookupMutableRegistrationTx(tx, network, registration)
	if err != nil {
		return err
	}

	row := tx.QueryRow(`
		SELECT id FROM "group"
		WHERE network_name = ?1 AND name = ?2`,
		network,
		group,
	)
	var groupID int64
	if err := row.Scan(&groupID); err != nil {
		return CheckSqliteErr("find group for registration removal", err)
	}

	if _, err := tx.Exec(`
		DELETE FROM registration_assignment
		WHERE registration_id = ?1 AND group_id = ?2`,
		registrationID,
		groupID,
	); err != nil {
		return CheckSqliteErr("remove registration group", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit remove registration group tx: %w", err)
	}
	return nil
}

func lookupMutableRegistrationTx(
	tx *sql.Tx,
	network string,
	registration string,
) (
	int64,
	error,
) {
	row := tx.QueryRow(`
		SELECT id, confirmed FROM registration
		WHERE network_name = ?1 AND name = ?2`,
		network,
		registration,
	)
	var registrationID int64
	var confirmed int64
	if err := row.Scan(&registrationID, &confirmed); err != nil {
		return 0, CheckSqliteErr("find mutable registration", err)
	}
	if confirmed != 0 {
		return 0, fmt.Errorf(
			"%w: confirmed registration %q cannot be modified",
			service.ErrConflict,
			registration,
		)
	}
	return registrationID, nil
}

func (db *DB) DeleteRegistration(
	network string,
	name string,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin delete registration tx: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRow(`
		SELECT id, redeemed_key, confirmed
		FROM registration
		WHERE network_name = ?1 AND name = ?2`,
		network,
		name,
	)
	var registrationID int64
	var redeemedKey string
	var confirmed int64
	if err := row.Scan(&registrationID, &redeemedKey, &confirmed); err != nil {
		return CheckSqliteErr("find registration to delete", err)
	}
	if confirmed != 0 {
		return fmt.Errorf(
			"%w: confirmed registration %q cannot be revoked",
			service.ErrConflict,
			name,
		)
	}

	var cidrID int64
	if redeemedKey != "" {
		row := tx.QueryRow(`
			SELECT cidr_id FROM peer
			WHERE network_name = ?1
				AND public_key = ?2
				AND confirmed = 0`,
			network,
			redeemedKey,
		)
		err := row.Scan(&cidrID)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return CheckSqliteErr("find provisional peer to revoke", err)
		}
		if err == nil {
			if _, err := tx.Exec(`DELETE FROM peer WHERE network_name = ?1 AND public_key = ?2`, network, redeemedKey); err != nil {
				return CheckSqliteErr("delete revoked provisional peer", err)
			}
		}
	}

	if _, err := tx.Exec(`DELETE FROM registration WHERE id = ?1`, registrationID); err != nil {
		return CheckSqliteErr("delete registration", err)
	}
	if cidrID != 0 {
		if _, err := tx.Exec(`DELETE FROM cidr WHERE id = ?1`, cidrID); err != nil {
			return CheckSqliteErr("delete revoked peer CIDR", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit delete registration tx: %w", err)
	}
	return nil
}

func (db *DB) ConfirmPeer(
	network string,
	name string,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin confirm peer tx: %w", err)
	}
	defer tx.Rollback()

	row := tx.QueryRow(`
		SELECT id, cidr_id, public_key, confirmed
		FROM peer
		WHERE network_name = ?1 AND name = ?2`,
		network,
		name,
	)
	var peerID int64
	var cidrID int64
	var publicKey string
	var peerConfirmed int64
	if err := row.Scan(&peerID, &cidrID, &publicKey, &peerConfirmed); err != nil {
		return CheckSqliteErr("find peer to confirm", err)
	}
	if peerConfirmed != 0 {
		return nil
	}

	row = tx.QueryRow(`
		SELECT id FROM registration
		WHERE network_name = ?1
			AND redeemed_key = ?2
			AND redeemed = 1
			AND confirmed = 0`,
		network,
		publicKey,
	)
	var registrationID int64
	if err := row.Scan(&registrationID); err != nil {
		return CheckSqliteErr("find registration to confirm", err)
	}

	if _, err := tx.Exec(`
		INSERT OR IGNORE INTO cidr_assignment (cidr_id, group_id)
		SELECT ?1, group_id
		FROM registration_assignment
		WHERE registration_id = ?2`,
		cidrID,
		registrationID,
	); err != nil {
		return CheckSqliteErr("transfer registration groups", err)
	}
	if _, err := tx.Exec(`DELETE FROM registration_assignment WHERE registration_id = ?1`, registrationID); err != nil {
		return CheckSqliteErr("clear registration groups", err)
	}
	if _, err := tx.Exec(`UPDATE peer SET confirmed = 1 WHERE id = ?1`, peerID); err != nil {
		return CheckSqliteErr("confirm peer", err)
	}
	if _, err := tx.Exec(`UPDATE registration SET confirmed = 1 WHERE id = ?1`, registrationID); err != nil {
		return CheckSqliteErr("confirm registration", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit confirm peer tx: %w", err)
	}
	return nil
}

// PruneExpiredRegistrations removes expired unconfirmed registrations
// and any provisional peer rows that no longer have a live
// registration. A peer is provisional when confirmed = 0; it is kept
// only while its registration is unconfirmed and unexpired. Confirmed
// peers are never pruned here — their registrations are retained as
// audit state.
//
// Endpoint rows referencing pruned peers are removed via the ON DELETE
// CASCADE foreign keys on the endpoint table.
func (db *DB) PruneExpiredRegistrations(
	network string,
	now time.Time,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin prune tx: %w", err)
	}
	defer tx.Rollback()

	rows, err := tx.Query(`
		SELECT cidr_id
		FROM peer p
		WHERE p.network_name = ?1
			AND p.confirmed = 0
			AND NOT EXISTS (
				SELECT 1 FROM registration r
				WHERE r.network_name = p.network_name
					AND r.redeemed_key = p.public_key
					AND r.confirmed = 0
					AND r.expires_at_unix > ?2
			)`,
		network,
		now.Unix(),
	)
	if err != nil {
		return CheckSqliteErr("find provisional peer CIDRs to prune", err)
	}
	var cidrIDs []int64
	for rows.Next() {
		var cidrID int64
		if err := rows.Scan(&cidrID); err != nil {
			rows.Close()
			return CheckSqliteErr("scan provisional peer CIDR", err)
		}
		cidrIDs = append(cidrIDs, cidrID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterate provisional peer CIDRs: %w", err)
	}
	rows.Close()

	if _, err := tx.Exec(`
		DELETE FROM peer
		WHERE network_name = ?1 AND confirmed = 0
			AND NOT EXISTS (
				SELECT 1 FROM registration r
				WHERE r.network_name = peer.network_name
					AND r.redeemed_key = peer.public_key
					AND r.confirmed = 0
					AND r.expires_at_unix > ?2
			)`,
		network,
		now.Unix(),
	); err != nil {
		return CheckSqliteErr("prune provisional peers", err)
	}
	if _, err := tx.Exec(`
		DELETE FROM registration
		WHERE network_name = ?1
			AND confirmed = 0
			AND expires_at_unix <= ?2`,
		network,
		now.Unix(),
	); err != nil {
		return CheckSqliteErr("prune expired registrations", err)
	}
	for _, cidrID := range cidrIDs {
		if _, err := tx.Exec(`DELETE FROM cidr WHERE id = ?1`, cidrID); err != nil {
			return CheckSqliteErr("prune provisional peer CIDR", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit prune tx: %w", err)
	}
	return nil
}

func scanRegistration(
	s Scanner,
) (
	*service.Registration,
	error,
) {
	var name string
	var tempPubKey string
	var tempRoute string
	var finalRoute string
	var admin int64
	var redeemed int64
	var redeemedKey string
	var confirmed int64
	var expiresUnix int64
	var createdUnix int64

	if err := s.Scan(
		&name,
		&tempPubKey,
		&tempRoute,
		&finalRoute,
		&admin,
		&redeemed,
		&redeemedKey,
		&confirmed,
		&expiresUnix,
		&createdUnix,
	); err != nil {
		return nil, CheckSqliteErr("scan registration", err)
	}

	return &service.Registration{
		Name:            name,
		InvitePublicKey: tempPubKey,
		InviteRoute:     tempRoute,
		MainRoute:       finalRoute,
		Admin:           admin != 0,
		Redeemed:        redeemed != 0,
		RedeemedKey:     redeemedKey,
		Confirmed:       confirmed != 0,
		ExpiresAt:       time.Unix(expiresUnix, 0),
		CreatedAt:       time.Unix(createdUnix, 0),
	}, nil
}
