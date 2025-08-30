package database

import (
	"fmt"
)

func (s *SQLiteStore) InviteCreate(name string,
	pubKey string,
	cidr string,
	admin bool,
	inviteExpires int64,
) error {
	_, err := s.db.Exec(`
		INSERT INTO invite (public_key, temp_cidr, final_cidr, name, admin, redeemed, expiration)
		SELECT ?2, ?3, c.id, ?1, ?4, 0, ?5
		FROM cidr c
		WHERE c.name=?1;
		`,
		name,
		pubKey,
		cidr,
		admin,
		inviteExpires,
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
		INSERT INTO peer (cidr, public_key, admin, disabled, confirmed)
		SELECT i.final_cidr, ?2, i.admin, 0, 1
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
