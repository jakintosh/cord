package database

import (
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (db *DB) GetInvite(
	network string,
	name string,
) (
	*service.Invite,
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
		FROM invite
		WHERE network_name = ?1
			AND name = ?2`,
		network,
		name,
	)

	return scanInvite(row)
}

func (db *DB) GetInviteByIP(
	network string,
	ip net.IP,
	now time.Time,
) (
	*service.Invite,
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
		FROM invite
		WHERE network_name = ?1
			AND temp_ip = ?2
			AND confirmed = 0
			AND expires_at_unix > ?3`,
		network,
		ip,
		now.Unix(),
	)

	return scanInvite(row)
}

func (db *DB) ListInvites(
	network string,
) (
	[]*service.Invite,
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
		FROM invite
		WHERE network_name = ?1
		ORDER BY created_at_unix DESC`,
		network,
	)
	if err != nil {
		return nil, CheckSqliteErr("list invites", err)
	}
	defer rows.Close()

	var invites []*service.Invite
	for rows.Next() {
		inv, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate invites: %w", err)
	}

	return invites, nil
}

func (db *DB) ListActiveInvites(
	network string,
	now time.Time,
) (
	[]*service.Invite,
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
		FROM invite
		WHERE network_name = ?1
			AND confirmed = 0
			AND expires_at_unix > ?2
		ORDER BY created_at_unix DESC`,
		network,
		now.Unix(),
	)
	if err != nil {
		return nil, CheckSqliteErr("list active invites", err)
	}
	defer rows.Close()

	var invites []*service.Invite
	for rows.Next() {
		inv, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, inv)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active invites: %w", err)
	}

	return invites, nil
}

func (db *DB) InsertInvite(
	network string,
	invite *service.Invite,
) error {
	tempIP := netaddr.Normalize(invite.TempIP)
	finalIP := netaddr.Normalize(invite.FinalIP)

	_, err := db.Conn.Exec(`
		INSERT INTO invite (
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
		invite.Name,
		invite.TempPubKey,
		tempIP,
		finalIP,
		boolToInt(invite.Admin),
		boolToInt(invite.Redeemed),
		invite.RedeemedKey,
		boolToInt(invite.Confirmed),
		invite.ExpiresAt.Unix(),
		invite.CreatedAt.Unix(),
	)
	return CheckSqliteErr("insert invite", err)
}

func (db *DB) RedeemInvite(
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
			i.network_name,
			i.name,
			i.final_ip,
			?3,
			i.admin,
			1,
			0
		FROM invite i
		WHERE i.network_name = ?1
			AND i.temp_public_key = ?2
			AND i.redeemed = 0
			AND i.expires_at_unix > ?4`,
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
		return fmt.Errorf("%w: no redeemable invite for key", service.ErrNotFound)
	}

	if _, err := tx.Exec(`
		UPDATE invite
		SET redeemed = 1,
			redeemed_key = ?3
		WHERE network_name = ?1
			AND temp_public_key = ?2
			AND redeemed = 0`,
		network,
		tempPubKey,
		permPubKey,
	); err != nil {
		return CheckSqliteErr("redeem mark invite", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit redeem tx: %w", err)
	}

	return nil
}

func (db *DB) DeleteInvite(
	network string,
	name string,
) error {
	result, err := db.Conn.Exec(`
		DELETE FROM invite
		WHERE network_name = ?1
			AND name = ?2`,
		network,
		name,
	)
	if err != nil {
		return CheckSqliteErr("delete invite", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete invite rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: invite %q not found", service.ErrNotFound, name)
	}
	return nil
}

func (db *DB) ConfirmInvite(
	network string,
	name string,
) error {
	result, err := db.Conn.Exec(`
		UPDATE invite
		SET confirmed = 1
		WHERE network_name = ?1
			AND name = ?2
			AND confirmed = 0`,
		network,
		name,
	)
	if err != nil {
		return CheckSqliteErr("confirm invite", err)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("confirm invite rows affected: %w", err)
	}
	if affected == 0 {
		return fmt.Errorf("%w: invite %q not found or already confirmed", service.ErrNotFound, name)
	}
	return nil
}

func (db *DB) DeleteExpiredInvites(
	network string,
	before time.Time,
) error {
	_, err := db.Conn.Exec(`
		DELETE FROM invite
		WHERE network_name = ?1
			AND expires_at_unix < ?2`,
		network,
		before.Unix(),
	)
	return CheckSqliteErr("delete expired invites", err)
}

// PruneExpiredInvites removes expired unconfirmed invites and any
// provisional peer rows that no longer have a live invite. A peer is
// provisional when confirmed = 0; it is kept only while its invite is
// unconfirmed and unexpired. Confirmed peers are never pruned here —
// their invites are retained as audit state.
//
// Endpoint rows referencing pruned peers are removed via the ON DELETE
// CASCADE foreign keys on the endpoint table.
func (db *DB) PruneExpiredInvites(
	network string,
	now time.Time,
) error {
	tx, err := db.Conn.Begin()
	if err != nil {
		return fmt.Errorf("begin prune tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`
		DELETE FROM invite
		WHERE network_name = ?1
			AND confirmed = 0
			AND expires_at_unix <= ?2`,
		network,
		now.Unix(),
	); err != nil {
		return CheckSqliteErr("prune expired invites", err)
	}

	if _, err := tx.Exec(`
		DELETE FROM peer
		WHERE network_name = ?1
			AND confirmed = 0
			AND NOT EXISTS (
				SELECT 1 FROM invite
				WHERE invite.network_name = peer.network_name
					AND invite.name = peer.name
					AND invite.confirmed = 0
					AND invite.expires_at_unix > ?2
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

func scanInvite(
	s Scanner,
) (
	*service.Invite,
	error,
) {
	var name string
	var tempPubKey string
	var tempIP []byte
	var finalIP []byte
	var admin int64
	var redeemed int64
	var redeemedKey string
	var confirmed int64
	var expiresUnix int64
	var createdUnix int64

	if err := s.Scan(
		&name,
		&tempPubKey,
		&tempIP,
		&finalIP,
		&admin,
		&redeemed,
		&redeemedKey,
		&confirmed,
		&expiresUnix,
		&createdUnix,
	); err != nil {
		return nil, CheckSqliteErr("scan invite", err)
	}

	return &service.Invite{
		Name:        name,
		TempPubKey:  tempPubKey,
		TempIP:      net.IP(tempIP),
		FinalIP:     net.IP(finalIP),
		Admin:       admin != 0,
		Redeemed:    redeemed != 0,
		RedeemedKey: redeemedKey,
		Confirmed:   confirmed != 0,
		ExpiresAt:   time.Unix(expiresUnix, 0),
		CreatedAt:   time.Unix(createdUnix, 0),
	}, nil
}
