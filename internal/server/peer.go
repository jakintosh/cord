package server

import (
	"fmt"
	"net"

	db "git.sr.ht/~jakintosh/cord/internal/database"
	"git.sr.ht/~jakintosh/cord/internal/utils"
	wg "git.sr.ht/~jakintosh/cord/internal/wireguard"
)

type PeerDesc struct {
	Name    string
	Ip      net.IP
	Admin   bool
	Expires int64
}

type Peer struct {
	PeerId    int64  `json:"peerId"`
	CidrId    int64  `json:"cidrId"`
	Name      string `json:"name"`
	PublicKey string `json:"publicKey"`
	Cidr      string `json:"cidr"`
	Admin     bool   `json:"admin"`
	Confirmed bool   `json:"confirmed"`
	Disabled  bool   `json:"disabled"`
}

func (p *Peer) String() string {
	return fmt.Sprintf(
		"%s | %s | %s",
		p.PublicKey,
		p.Cidr,
		p.Name,
	)
}

func (ctx *Context) CreateInvite(
	name string,
	ip net.IP,
	admin bool,
	inviteExpires int64,
) (
	*wg.DeviceConfig,
	*wg.PeerConfig,
	error,
) {

	if err := utils.ValidateHostName(name); err != nil {
		return nil, nil, fmt.Errorf("failed to validate peer name: %w", err)
	}

	privKey, pubKey, err := wg.GenerateKeypair()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate keypair: %v", err)
	}

	cidr := utils.GetPeerCidrFromIp(ip)
	err = ctx.CreateCidr(name, cidr)
	if err != nil {
		return nil, nil, err
	}

	// Store as an invite until redeemed
	_, err = ctx.Db.Exec(`
		INSERT INTO invite (public_key, temp_cidr, final_cidr, name, admin, redeemed, expiration)
		SELECT ?2, ?3, c.id, ?1, ?4, 0, ?5
		FROM cidr c
		WHERE c.name=?1;
		`,
		name,                                 // 1: name lookup and insert value
		pubKey.String(),                      // 2: temporary public key
		utils.GetPeerCidrFromIp(ip).String(), // 3: temp_cidr (placeholder)
		admin,                                // 4: admin flag
		inviteExpires,                        // 5: expiration
	)
	if err != nil {
		return nil, nil, db.CheckSqliteErr("adding invite", err)
	}

	peerInterface := &wg.DeviceConfig{
		PrivateKey: privKey,
		Cidr:       cidr,
		ListenPort: 0,
	}

	peerInfo := &wg.PeerConfig{
		Name:      name,
		Cidr:      cidr,
		PublicKey: pubKey,
	}

	return peerInterface, peerInfo, nil
}

func (ctx *Context) RedeemInvite(
	pubKey string,
	newKey string,
) error {

	// Create a peer from an unredeemed invite and mark invite redeemed.
	tx, err := ctx.Db.Begin()
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
	if db.ResultsEmpty(res) {
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

func (ctx *Context) RenamePeer(
	peer string,
	newName string,
) error {

	return ctx.RenameCidr(peer, newName)
}

func (ctx *Context) SetPeerEnabled(
	peer string,
	enabled bool,
) error {
	_, err := ctx.Db.Exec(`
		UPDATE peer
		SET disabled=?2
		FROM peer p
		JOIN cidr c ON p.cidr=c.id
		WHERE c.name=?1;
		`,
		peer,
		!enabled,
	)
	return db.CheckSqliteErr("setting peer dis/enabled", err)
}

func (ctx *Context) CheckPeerExists(
	peerName string,
) bool {
	row := ctx.Db.QueryRow(`
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

func (ctx *Context) GetParentCidrIdForPeerNamed(
	peerName string,
) (
	int64,
	error,
) {
	// query the parent cidr id given the peer name
	row := ctx.Db.QueryRow(`
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
		`,
		peerName,
	)

	// scan the parent cidr id
	var parentCidrId int64
	if err := row.Scan(&parentCidrId); err != nil {
		return -1, db.CheckSqliteErr("getting parent cidrs", err)
	}
	return parentCidrId, nil
}

func (ctx *Context) GetAssociatedCidrIdsOfPeerNamed(
	peerName string,
) (
	[]int64,
	error,
) {
	parentCidrId, err := ctx.GetParentCidrIdForPeerNamed(peerName)
	if err != nil {
		return nil, err
	}

	associatedCidrIds, err := ctx.GetAssociatedCidrIdsForCidrId(parentCidrId)
	if err != nil {
		return nil, err
	}

	// build list of all associated cidr ids
	cidrs := []int64{parentCidrId}
	cidrs = append(cidrs, associatedCidrIds...)

	return cidrs, nil
}

func (ctx *Context) GetPeersOfPeerNamed(
	peerName string,
) (
	[]Peer,
	error,
) {
	// get associated cidrs
	cidrs, err := ctx.GetAssociatedCidrIdsOfPeerNamed(peerName)
	if err != nil {
		return nil, err
	}

	// get all peers for each associated cidr
	peerMap := make(map[Peer]struct{})
	for _, cidrId := range cidrs {

		cidrPeers, err := ctx.GetChildPeersForCidrId(cidrId)
		if err != nil {
			return nil, err
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
	peers := make([]Peer, len(peerMap))
	for peer := range peerMap {
		peers[i] = peer
		i += 1
	}

	return peers, nil
}

func (ctx *Context) GetChildPeersForCidrId(
	cidrId int64,
) (
	[]Peer,
	error,
) {
	// find all (confirmed) child peers for given cidr id
	rows, err := ctx.Db.Query(`
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
		`,
		cidrId,
	)
	if err != nil {
		return nil, db.CheckSqliteErr("getting peers for cidr", err)
	}

	// scan peers from rows
	defer rows.Close()
	var peers []Peer
	for rows.Next() {
		var peer Peer
		err := rows.Scan(
			&peer.PeerId,
			&peer.CidrId,
			&peer.Name,
			&peer.PublicKey,
			&peer.Cidr,
			&peer.Admin,
			&peer.Confirmed,
			&peer.Disabled,
		)
		if err != nil {
			return nil, db.CheckSqliteErr("scanning peer info", err)
		}

		peers = append(peers, peer)
	}

	return peers, nil
}
