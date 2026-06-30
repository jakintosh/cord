package database

import (
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
			name,
			temp_public_key,
			temp_ip,
			final_ip,
			admin,
			redeemed,
			redeemed_key,
			confirmed,
			expires_at_unix,
			created_at_unix
		FROM registration
		WHERE network_name = ?1
			AND name = ?2`,
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
	ip = netaddr.Normalize(ip)
	row := db.Conn.QueryRow(`
		SELECT
			name,
			temp_public_key,
			temp_ip,
			final_ip,
			admin,
			redeemed,
			redeemed_key,
			confirmed,
			expires_at_unix,
			created_at_unix
		FROM registration
		WHERE network_name = ?1
			AND temp_ip = ?2
			AND confirmed = 0
			AND expires_at_unix > ?3`,
		network,
		ip,
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
			name,
			temp_public_key,
			temp_ip,
			final_ip,
			admin,
			redeemed,
			redeemed_key,
			confirmed,
			expires_at_unix,
			created_at_unix
		FROM registration
		WHERE network_name = ?1
		ORDER BY created_at_unix DESC`,
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
			name,
			temp_public_key,
			temp_ip,
			final_ip,
			admin,
			redeemed,
			redeemed_key,
			confirmed,
			expires_at_unix,
			created_at_unix
		FROM registration
		WHERE network_name = ?1
			AND confirmed = 0
			AND expires_at_unix > ?2
		ORDER BY created_at_unix DESC`,
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
	tempIP := netaddr.Normalize(reg.InviteIP)
	finalIP := netaddr.Normalize(reg.MainIP)

	_, err := db.Conn.Exec(`
		INSERT INTO registration (
			network_name,
			name,
			temp_public_key,
			temp_ip,
			final_ip,
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
		tempIP,
		finalIP,
		boolToInt(reg.Admin),
		boolToInt(reg.Redeemed),
		reg.RedeemedKey,
		boolToInt(reg.Confirmed),
		reg.ExpiresAt.Unix(),
		reg.CreatedAt.Unix(),
	)
	return CheckSqliteErr("insert registration", err)
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

	result, err := tx.Exec(`
		INSERT INTO peer (
			network_name,
			name,
			ip,
			public_key,
			admin,
			enabled,
			confirmed
		)
		SELECT
			r.network_name,
			r.name,
			r.final_ip,
			?3,
			r.admin,
			1,
			0
		FROM registration r
		WHERE r.network_name = ?1
			AND r.temp_public_key = ?2
			AND r.redeemed = 0
			AND r.expires_at_unix > ?4`,
		network,
		tempPubKey,
		permPubKey,
		now.Unix(),
	)
	if err != nil {
		return CheckSqliteErr("redeem create peer", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("redeem rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: no redeemable registration for key", service.ErrNotFound)
	}

	if _, err := tx.Exec(`
		UPDATE registration
		SET redeemed = 1,
			redeemed_key = ?3
		WHERE network_name = ?1
			AND temp_public_key = ?2
			AND redeemed = 0`,
		network,
		tempPubKey,
		permPubKey,
	); err != nil {
		return CheckSqliteErr("redeem mark registration", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit redeem tx: %w", err)
	}

	return nil
}

func (db *DB) DeleteRegistration(
	network string,
	name string,
) error {
	result, err := db.Conn.Exec(`
		DELETE FROM registration
		WHERE network_name = ?1
			AND name = ?2`,
		network,
		name,
	)
	if err != nil {
		return CheckSqliteErr("delete registration", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete registration rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: registration %q not found", service.ErrNotFound, name)
	}
	return nil
}

func (db *DB) ConfirmRegistration(
	network string,
	name string,
) error {
	result, err := db.Conn.Exec(`
		UPDATE registration
		SET confirmed = 1
		WHERE network_name = ?1
			AND name = ?2
			AND confirmed = 0`,
		network,
		name,
	)
	if err != nil {
		return CheckSqliteErr("confirm registration", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm registration rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: registration %q not found or already confirmed", service.ErrNotFound, name)
	}
	return nil
}

func (db *DB) DeleteExpiredRegistrations(
	network string,
	before time.Time,
) error {
	_, err := db.Conn.Exec(`
		DELETE FROM registration
		WHERE network_name = ?1
			AND expires_at_unix < ?2`,
		network,
		before.Unix(),
	)
	return CheckSqliteErr("delete expired registrations", err)
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

	if _, err := tx.Exec(`
		DELETE FROM peer
		WHERE network_name = ?1
			AND confirmed = 0
			AND NOT EXISTS (
				SELECT 1 FROM registration
				WHERE registration.network_name = peer.network_name
					AND registration.name = peer.name
					AND registration.confirmed = 0
					AND registration.expires_at_unix > ?2
			)`,
		network,
		now.Unix(),
	); err != nil {
		return CheckSqliteErr("prune provisional peers", err)
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
	var tempIPBytes []byte
	var finalIPBytes []byte
	var admin int64
	var redeemed int64
	var redeemedKey string
	var confirmed int64
	var expiresUnix int64
	var createdUnix int64

	if err := s.Scan(
		&name,
		&tempPubKey,
		&tempIPBytes,
		&finalIPBytes,
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
		InviteIP:        net.IP(tempIPBytes),
		MainIP:          net.IP(finalIPBytes),
		Admin:           admin != 0,
		Redeemed:        redeemed != 0,
		RedeemedKey:     redeemedKey,
		Confirmed:       confirmed != 0,
		ExpiresAt:       time.Unix(expiresUnix, 0),
		CreatedAt:       time.Unix(createdUnix, 0),
	}, nil
}
