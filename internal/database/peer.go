package database

import (
	"net"

	"git.sr.ht/~jakintosh/cord/internal/server"
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
