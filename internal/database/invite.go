package database

import (
	"fmt"
	"net"
	"time"

	"git.sr.ht/~jakintosh/cord/internal/server"
	"git.sr.ht/~jakintosh/cord/internal/utils"
)

func (store *SQLiteStore) InviteList() (
	[]*server.ServerInvite,
	error,
) {
	rows, err := store.db.Query(`
		SELECT name, public_key, temp_ip, final_ip, admin, redeemed, expiration
		FROM invite
		ORDER BY expiration DESC;`,
	)
	if err != nil {
		return nil, CheckSqliteErr("querying invites", err)
	}
	defer rows.Close()

	var invites []*server.ServerInvite
	for rows.Next() {
		invite, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}

	return invites, nil
}

func (store *SQLiteStore) InviteGet(
	name string,
) (
	*server.ServerInvite,
	error,
) {
	row := store.db.QueryRow(`
		SELECT name, public_key, temp_ip, final_ip, admin, redeemed, expiration
		FROM invite
		WHERE name = ?1;`,
		name,
	)

	return scanInvite(row)
}

// InviteListActive returns unredeemed, unexpired invites. These are the
// peers that belong on the invite network interface.
func (store *SQLiteStore) InviteListActive() (
	[]*server.ServerInvite,
	error,
) {
	rows, err := store.db.Query(`
		SELECT name, public_key, temp_ip, final_ip, admin, redeemed, expiration
		FROM invite
		WHERE redeemed = 0
		  AND expiration > ?1
		ORDER BY expiration DESC;`,
		time.Now().Unix(),
	)
	if err != nil {
		return nil, CheckSqliteErr("querying active invites", err)
	}
	defer rows.Close()

	var invites []*server.ServerInvite
	for rows.Next() {
		invite, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		invites = append(invites, invite)
	}

	return invites, nil
}

func (store *SQLiteStore) InviteGetByIP(
	ip net.IP,
) (
	*server.ServerInvite,
	error,
) {
	// Normalize IPv4 to 4-byte representation
	ip = utils.NormalizeIP(ip)
	row := store.db.QueryRow(`
		SELECT name, public_key, temp_ip, final_ip, admin, redeemed, expiration
		FROM invite
		WHERE temp_ip = ?1
		  AND redeemed = 0
		  AND expiration > ?2;`,
		ip,
		time.Now().Unix(),
	)

	return scanInvite(row)
}

// InviteGetByIPAny is like InviteGetByIP but also returns invites that
// have already been redeemed. Redemption must stay reachable for a
// redeemed-but-unconfirmed invite so the flow can be retried.
func (store *SQLiteStore) InviteGetByIPAny(
	ip net.IP,
) (
	*server.ServerInvite,
	error,
) {
	ip = utils.NormalizeIP(ip)
	row := store.db.QueryRow(`
		SELECT name, public_key, temp_ip, final_ip, admin, redeemed, expiration
		FROM invite
		WHERE temp_ip = ?1
		  AND expiration > ?2;`,
		ip,
		time.Now().Unix(),
	)

	return scanInvite(row)
}

func (s *SQLiteStore) InviteCreate(
	name string,
	pubKey string,
	tempIP net.IP,
	finalIP net.IP,
	admin bool,
	expiration int64,
) error {
	// normalize IPs
	tempIP = utils.NormalizeIP(tempIP)
	finalIP = utils.NormalizeIP(finalIP)
	_, err := s.db.Exec(`
		INSERT INTO invite (name, public_key, temp_ip, final_ip, admin, redeemed, expiration)
		VALUES (?1, ?2, ?3, ?4, ?5, 0, ?6);`,
		name,
		pubKey,
		tempIP,
		finalIP,
		admin,
		expiration,
	)
	return CheckSqliteErr("adding invite", err)
}

// InvitesPruneExpired removes invite records whose expiration has
// passed, freeing their reserved invite-network addresses.
func (s *SQLiteStore) InvitesPruneExpired(
	before int64,
) error {
	_, err := s.db.Exec(`
		DELETE FROM invite
		WHERE expiration < ?1;`,
		before,
	)
	return CheckSqliteErr("pruning expired invites", err)
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

	// Insert into peer using details from invite. The peer starts
	// unconfirmed: it becomes operational once it calls confirm over
	// the main network (see PeerConfirm).
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
			0
		FROM invite i
		WHERE i.redeemed=0
			AND i.public_key=?1
			AND i.expiration > ?3;`,
		pubKey,
		newKey,
		time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("failed to create peer from invite: %w", err)
	}
	if ResultsEmpty(res) {
		return fmt.Errorf("%w: no redeemable invite for that key", server.ErrNotFound)
	}

	// Mark invite as redeemed
	if _, err := tx.Exec(`
		UPDATE invite
		SET redeemed=1
		WHERE redeemed=0
		AND public_key=?1;`,
		pubKey,
	); err != nil {
		return fmt.Errorf("failed to mark invite redeemed: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit redeem tx: %w", err)
	}

	return nil
}

func scanInvite(s Scanner) (
	*server.ServerInvite,
	error,
) {
	var name, publicKey string
	var tempIP, finalIP []byte
	var admin, redeemed int
	var expiration int64

	err := s.Scan(
		&name,
		&publicKey,
		&tempIP,
		&finalIP,
		&admin,
		&redeemed,
		&expiration,
	)
	if err != nil {
		return nil, CheckSqliteErr("scanning invite info", err)
	}

	// Convert IP bytes to net.IP and then to CIDR strings
	tempIPNet := utils.GetPeerCidrFromIp(net.IP(tempIP))
	finalIPNet := utils.GetPeerCidrFromIp(net.IP(finalIP))

	invite := &server.ServerInvite{
		Name:        name,
		PublicKey:   publicKey,
		InviteCidr:  tempIPNet.String(),
		NetworkCidr: finalIPNet.String(),
		Admin:       admin != 0,
		Redeemed:    redeemed != 0,
		Expiration:  time.Unix(expiration, 0),
	}

	return invite, nil
}
