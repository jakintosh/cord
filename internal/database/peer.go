package database

import (
	"fmt"
	"net"

	"git.sr.ht/~jakintosh/cord/internal/server"
	"git.sr.ht/~jakintosh/cord/internal/utils"
)

func (s *SQLiteStore) PeerExists(
	peerName string,
) bool {
	row := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM peer
		WHERE name = ?;`,
		peerName,
	)

	var count int64
	if err := row.Scan(&count); err != nil {
		return false
	}

	return count > 0
}

func (store *SQLiteStore) PeerList() (
	[]*server.Peer,
	error,
) {
	rows, err := store.db.Query(`
		SELECT name, public_key, ip, prefix, admin, enabled, confirmed
		FROM peer
		ORDER BY name ASC;`,
	)
	if err != nil {
		return nil, CheckSqliteErr("querying peers", err)
	}
	defer rows.Close()

	var peers []*server.Peer
	for rows.Next() {
		peer, err := scanPeer(rows)
		if err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}

	return peers, nil
}

func (s *SQLiteStore) PeerListPeers(
	peerName string,
) (
	[]*server.Peer,
	error,
) {
	rows, err := s.db.Query(`
		-- Get the requesting peer IP
		WITH req_ip AS (
			SELECT ip FROM peer WHERE name = ?1
		),
		-- Find only the most specific CIDR(s) containing the requesting peer
		requesting_peer_cidrs AS (
			SELECT c.id
			FROM cidr c
			WHERE c.base <= (SELECT ip FROM req_ip)
				AND (SELECT ip FROM req_ip) <= c.last
				AND c.prefix = (
					SELECT MAX(c2.prefix)
					FROM cidr c2
					WHERE c2.base <= (SELECT ip FROM req_ip)
						AND (SELECT ip FROM req_ip) <= c2.last
				)
		),
		-- Then find all associated CIDRs (plus the requesting CIDR itself)
		associated_cidrs AS (
			SELECT DISTINCT cidr_id FROM (
				SELECT a.cidr2 as cidr_id
				FROM association a
				WHERE a.cidr1 IN (SELECT id FROM requesting_peer_cidrs)
				UNION
				SELECT a.cidr1 as cidr_id
				FROM association a
				WHERE a.cidr2 IN (SELECT id FROM requesting_peer_cidrs)
				UNION
				SELECT id as cidr_id FROM requesting_peer_cidrs
			)
		)
		-- Finally, find all peers in those CIDRs
		SELECT DISTINCT p.name, p.public_key, p.ip, p.prefix, p.admin, p.enabled, p.confirmed
		FROM peer p, cidr c
		WHERE c.id IN (SELECT cidr_id FROM associated_cidrs)
			AND c.base <= p.ip
			AND p.ip <= c.last
			AND p.confirmed = 1
			AND p.enabled = 1
			AND p.name != ?1;  -- Exclude the requesting peer`,
		peerName,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	var peers []*server.Peer
	for rows.Next() {
		peer, err := scanPeer(rows)
		if err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}

	return peers, nil
}

func (store *SQLiteStore) PeerGet(
	name string,
) (
	*server.Peer,
	error,
) {
	row := store.db.QueryRow(`
		SELECT name, public_key, ip, prefix, admin, enabled, confirmed
		FROM peer
		WHERE name = ?;`,
		name,
	)

	return scanPeer(row)
}

func (store *SQLiteStore) PeerGetByIP(
	ip net.IP,
) (
	*server.Peer,
	error,
) {
	ip = utils.NormalizeIP(ip)
	row := store.db.QueryRow(`
		SELECT name, public_key, ip, prefix, admin, enabled, confirmed
		FROM peer
		WHERE ip = ?1
			AND confirmed = 1
			AND enabled = 1;`,
		ip,
	)

	return scanPeer(row)
}

// PeerGetByKey looks up a peer by its permanent public key, regardless
// of confirmation state. Used for idempotent redeem/confirm handling.
func (store *SQLiteStore) PeerGetByKey(
	pubKey string,
) (
	*server.Peer,
	error,
) {
	row := store.db.QueryRow(`
		SELECT name, public_key, ip, prefix, admin, enabled, confirmed
		FROM peer
		WHERE public_key = ?1;`,
		pubKey,
	)

	return scanPeer(row)
}

// PeerConfirm marks the peer with the given key and IP as confirmed and
// deletes the invite that created it. Idempotent: confirming an
// already-confirmed peer succeeds.
func (s *SQLiteStore) PeerConfirm(
	pubKey string,
	ip net.IP,
) error {
	ip = utils.NormalizeIP(ip)

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin confirm tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.Exec(`
		UPDATE peer
		SET confirmed = 1
		WHERE public_key = ?1
		  AND ip = ?2;`,
		pubKey,
		ip,
	)
	if err != nil {
		return CheckSqliteErr("confirming peer", err)
	}
	if ResultsEmpty(res) {
		return fmt.Errorf("%w: no peer with that key and IP to confirm", server.ErrNotFound)
	}

	// The peer is operational; its invite has served its purpose
	if _, err := tx.Exec(`
		DELETE FROM invite
		WHERE final_ip = ?1;`,
		ip,
	); err != nil {
		return CheckSqliteErr("deleting confirmed invite", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit confirm tx: %w", err)
	}

	return nil
}

// PeerDelete removes a peer and its endpoint history. Any invite that
// reserved the peer's IP is removed as well.
func (s *SQLiteStore) PeerDelete(
	name string,
) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin delete tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`
		DELETE FROM endpoint
		WHERE peer IN (SELECT id FROM peer WHERE name = ?1)
		   OR witness IN (SELECT id FROM peer WHERE name = ?1);`,
		name,
	); err != nil {
		return CheckSqliteErr("deleting peer endpoints", err)
	}

	if _, err := tx.Exec(`
		DELETE FROM invite
		WHERE final_ip IN (SELECT ip FROM peer WHERE name = ?1);`,
		name,
	); err != nil {
		return CheckSqliteErr("deleting peer invite", err)
	}

	res, err := tx.Exec(`
		DELETE FROM peer
		WHERE name = ?1;`,
		name,
	)
	if err != nil {
		return CheckSqliteErr("deleting peer", err)
	}
	if ResultsEmpty(res) {
		return fmt.Errorf("%w: no peer named '%s'", server.ErrNotFound, name)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit delete tx: %w", err)
	}

	return nil
}

func (s *SQLiteStore) PeerUpdate(
	name string,
	req server.UpdatePeerRequest,
) (
	*server.Peer,
	error,
) {
	row := s.db.QueryRow(`
		UPDATE peer
		SET
			name = CASE
				WHEN ?2 IS NOT NULL THEN ?2
				ELSE name
			END,
			admin = CASE
				WHEN ?3 IS NOT NULL THEN ?3
				ELSE admin
			END,
			enabled = CASE
				WHEN ?4 IS NOT NULL THEN ?4
				ELSE enabled
			END
		WHERE name = ?1
		RETURNING name, public_key, ip, prefix, admin, enabled, confirmed;`,
		name,
		req.Name,
		req.Admin,
		req.Enabled,
	)

	return scanPeer(row)
}

func scanPeer(s Scanner) (
	*server.Peer,
	error,
) {
	var name, publicKey string
	var ip []byte
	var prefix int
	var admin, enabled, confirmed int

	err := s.Scan(
		&name,
		&publicKey,
		&ip,
		&prefix,
		&admin,
		&enabled,
		&confirmed,
	)
	if err != nil {
		return nil, CheckSqliteErr("scanning peer info", err)
	}

	// Convert IP bytes and prefix to CIDR string
	ipNet := &net.IPNet{
		IP:   net.IP(ip),
		Mask: net.CIDRMask(prefix, len(ip)*8),
	}

	peer := &server.Peer{
		Name:      name,
		PublicKey: publicKey,
		Cidr:      ipNet.String(),
		Admin:     admin != 0,
		Enabled:   enabled != 0,
		Confirmed: confirmed != 0,
	}

	return peer, nil
}
