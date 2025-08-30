package database

import "git.sr.ht/~jakintosh/cord/internal/server"

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

func (s *SQLiteStore) PeerListPeers(
	peerName string,
) (
	[]server.Peer,
	error,
) {
	// get associated cidrs
	row := s.db.QueryRow(`
		SELECT parent.id
		FROM cidr parent
		INNER JOIN (
			SELECT c.id, c.length, c.prefix, c.base
			FROM peer p
			JOIN cidr c
			ON c.id=p.cidr
			WHERE c.name=?
		) as client
		WHERE parent.length=client.length
			AND parent.base<=client.base
			AND client.base<parent.last
			AND parent.prefix<client.prefix
			ORDER BY parent.prefix DESC
		LIMIT 1;
		`, peerName)
	var parentCidrId int64
	if err := row.Scan(&parentCidrId); err != nil {
		return nil, CheckSqliteErr("getting parent cidrs", err)
	}
	rows, err := s.db.Query(`
		SELECT DISTINCT COALESCE(c1.id, c2.id) as cidr
		FROM association a
		LEFT JOIN (
			SELECT * FROM cidr c
			WHERE c.id<>?1
		) AS c1 ON a.cidr1=c1.id
		LEFT JOIN (
			SELECT * FROM cidr c
			WHERE c.id<>?1
		) AS c2 ON a.cidr2=c2.id
		WHERE a.cidr1=?1 OR a.cidr2<>?1;
		`, parentCidrId)
	if err != nil {
		return nil, CheckSqliteErr("getting associated cidrs", err)
	}
	defer rows.Close()
	var id int64
	var associatedCidrIds []int64
	for rows.Next() {
		if err := rows.Scan(&id); err != nil {
			return nil, CheckSqliteErr("scanning cidr id", err)
		}
		associatedCidrIds = append(associatedCidrIds, id)
	}
	cidrs := []int64{parentCidrId}
	cidrs = append(cidrs, associatedCidrIds...)

	// get all peers for each associated cidr
	peerMap := make(map[server.Peer]struct{})
	for _, cidrId := range cidrs {

		rows, err := s.db.Query(`
		SELECT p.id, c.id, c.name, p.public_key, c.cidr, p.admin, p.confirmed, p.disabled
		FROM cidr c
		INNER JOIN (
			SELECT c.name, c.length, c.prefix, c.base, c.last
			FROM cidr c
			WHERE c.id=?
		) AS parent
		JOIN peer p ON p.cidr=c.id
		WHERE p.confirmed=1
			AND p.disabled=0
			AND c.length=parent.length
			AND c.length=c.prefix
			AND c.prefix>parent.prefix
			AND c.base>=parent.base
			AND c.last<=parent.last;
		`, cidrId)
		if err != nil {
			return nil, CheckSqliteErr("getting peers for cidr", err)
		}
		defer rows.Close()
		var cidrPeers []server.Peer
		for rows.Next() {
			var peer server.Peer
			err := rows.Scan(&peer.PeerId, &peer.CidrId, &peer.Name, &peer.PublicKey, &peer.Cidr, &peer.Admin, &peer.Confirmed, &peer.Disabled)
			if err != nil {
				return nil, CheckSqliteErr("scanning peer info", err)
			}
			cidrPeers = append(cidrPeers, peer)
		}

		for _, peer := range cidrPeers {
			if peer.Name == peerName {
				continue // do not include the original peer
			}
			peerMap[peer] = struct{}{}
		}
	}

	// create slice from map
	i := 0
	peers := make([]server.Peer, len(peerMap))
	for peer := range peerMap {
		peers[i] = peer
		i += 1
	}

	return peers, nil
}

func (s *SQLiteStore) PeerRename(
	name string,
	newName string,
) error {
	return s.CidrRename(name, newName)
}

func (s *SQLiteStore) PeerSetAdmin(
	name string,
	admin bool,
) error {
	_, err := s.db.Exec(`
		UPDATE peer
		SET admin=?2
		FROM peer p
		JOIN cidr c ON p.cidr=c.id
		WHERE c.name=?1;
		`,
		name,
		admin,
	)
	return CheckSqliteErr("setting peer admin", err)
}

func (s *SQLiteStore) PeerSetEnabled(
	peer string,
	enabled bool,
) error {
	_, err := s.db.Exec(`
		UPDATE peer
		SET disabled=?2
		FROM peer p
		JOIN cidr c ON p.cidr=c.id
		WHERE c.name=?1;
		`,
		peer,
		!enabled,
	)
	return CheckSqliteErr("setting peer dis/enabled", err)
}
