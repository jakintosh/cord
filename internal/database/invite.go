package database

import (
	"fmt"
	"net"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

func (store *SQLiteStore) InviteList() (
	[]server.ServerInvite,
	error,
) {
	panic("unimplemented")
}

func (store *SQLiteStore) InviteGet(
	name string,
) (
	*server.ServerInvite,
	error,
) {
	panic("unimplemented")
}

func (s *SQLiteStore) InviteCreate(
	name string,
	pubKey string,
	tempIP net.IP,
	finalIP net.IP,
	admin bool,
	expiration int64,
) error {
	_, err := s.db.Exec(`
		INSERT INTO invite (name, public_key, temp_ip, final_ip, admin, redeemed, expiration)
		VALUES (?, ?, ?, ?, ?, 0, ?);
		`,
		name,
		pubKey,
		tempIP,
		finalIP,
		admin,
		expiration,
	)
	return CheckSqliteErr("adding invite", err)
}
func (s *SQLiteStore) InviteRedeem(
	pubKey string,
	newKey string,
) error {

	// Create a peer from an unredeemed invite and mark invite redeemed.
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin redeem tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Insert into peer using details from invite
	res, err := tx.Exec(`
		INSERT INTO peer (name, ip, prefix, public_key, admin, enabled, confirmed)
		SELECT
			i.name,
			i.final_ip,
			CASE
				WHEN LENGTH(i.final_ip) = 4 THEN 32
				ELSE 128
			END,
			?2,
			i.admin,
			1,
			1
		FROM invite i
		WHERE i.redeemed=0 AND i.public_key=?1;
		`,
		pubKey[:],
		newKey[:],
	)
	if err != nil {
		return fmt.Errorf("failed to create peer from invite: %w", err)
	}
	if ResultsEmpty(res) {
		return fmt.Errorf("failed to redeem peer: no redeemable invites")
	}

	// Mark invite as redeemed
	if _, err := tx.Exec(`
		UPDATE invite
		SET redeemed=1
		WHERE redeemed=0
		AND public_key=?1;
		`,
		pubKey[:],
	); err != nil {
		return fmt.Errorf("failed to mark invite redeemed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit redeem tx: %w", err)
	}

	return nil
}
