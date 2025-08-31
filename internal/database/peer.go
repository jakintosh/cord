package database

import (
	"git.sr.ht/~jakintosh/cord/internal/server"
)

func (store *SQLiteStore) PeerList() (
	[]server.Peer,
	error,
) {
	panic("unimplemented")
}

func (s *SQLiteStore) PeerListPeers(
	peerName string,
) (
	[]server.Peer,
	error,
) {
	rows, err := s.db.Query(`
		-- First, find all CIDRs containing the requesting peer
		WITH requesting_peer_cidrs AS (
		    SELECT c.id
		    FROM cidr c, peer p
		    WHERE p.name = ?
		      AND c.base <= p.ip
		      AND p.ip <= c.last
		),
		-- Then find all associated CIDRs
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
		SELECT DISTINCT p.*
		FROM peer p, cidr c
		WHERE c.id IN (SELECT cidr_id FROM associated_cidrs)
		  AND c.base <= p.ip
		  AND p.ip <= c.last
		  AND p.confirmed = 1
		  AND p.enabled = 1
		  AND p.name != ?;  -- Exclude the requesting peer
		`,
		peerName,
	)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	var peers []server.Peer
	for rows.Next() {
		var peer server.Peer
		err := rows.Scan(
			&peer.Name,
			&peer.PublicKey,
			&peer.Cidr,
			&peer.Admin,
			&peer.Confirmed,
			&peer.Enabled,
		)
		if err != nil {
			return nil, CheckSqliteErr("scanning peer info", err)
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
	panic("unimplemented")
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
			END,
		WHERE name = ?1
		RETURNING
			external_id,
			created_at,
			name,
			CASE
				WHEN ?4 IS NOT NULL THEN ?4
				ELSE (SELECT external_id FROM area p WHERE p.id=parent)
			END;
		`,
		name,
		req.Name,
		req.Admin,
		req.Enabled,
	)

	return scanPeer(row)

}

func (s *SQLiteStore) PeerExists(
	peerName string,
) bool {
	row := s.db.QueryRow(`
		SELECT COUNT(*)
		FROM peer p
		JOIN cidr c ON p.cidr=c.id
		WHERE c.name=?;
		`,
		peerName,
	)

	var count int64
	if err := row.Scan(&count); err != nil {
		return false
	}

	return count > 0
}

func scanPeer(s Scanner) (
	*server.Peer,
	error,
) {
	var peer server.Peer
	err := s.Scan(
		&peer.Name,
		&peer.PublicKey,
		&peer.Cidr,
		&peer.Admin,
		&peer.Confirmed,
		&peer.Enabled,
	)
	if err != nil {
		return nil, CheckSqliteErr("scanning peer info", err)
	}
	return &peer, nil
}
