package server

import (
	"fmt"
	"net"

	db "git.sr.ht/~jakintosh/innernet-go/internal/database"
	"git.sr.ht/~jakintosh/innernet-go/internal/utils"
	wg "git.sr.ht/~jakintosh/innernet-go/internal/wireguard"
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
	Redeemed  bool   `json:"redeemed"`
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

func (ctx *Context) CreatePeer(
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

	_, err = ctx.Db.Exec(`
		INSERT INTO peer (cidr, public_key, admin, invite_expires)
		SELECT c.id, ?2, ?3, ?4
		FROM cidr c
		WHERE c.name=?1;
		`,
		name, pubKey.String(), admin, inviteExpires,
	)
	if err != nil {
		return nil, nil, db.CheckSqliteErr("adding peer", err)
	}

	deviceConfig := &wg.DeviceConfig{
		PrivateKey: privKey,
		Cidr:       cidr,
		ListenPort: 0,
	}

	peerConfig := &wg.PeerConfig{
		Name:      name,
		Cidr:      cidr,
		PublicKey: pubKey,
	}

	return deviceConfig, peerConfig, nil
}

func (ctx *Context) RedeemPeer(
	pubKey string,
	newKey string,
) error {

	result, err := ctx.Db.Exec(`
		UPDATE peer
		SET   redeemed=1,
			  public_key=?2
		WHERE redeemed=0
		AND   public_key=?1
		`,
		pubKey[:],
		newKey[:],
	)

	if err != nil {
		return fmt.Errorf("failed to redeem peer: %w", err)
	}

	if db.ResultsEmpty(result) {
		return fmt.Errorf("failed to redeem peer: no redeemable peers")
	}

	// TODO: alert other peers?

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
	// find all (redeemed) child peers for given cidr id
	rows, err := ctx.Db.Query(`
		SELECT p.id, c.id, c.name, p.public_key, c.cidr, p.admin, p.redeemed, p.disabled
		FROM cidr c
		INNER JOIN (
			SELECT c.name, c.length, c.prefix, c.base, c.last
			FROM cidr c
			WHERE c.id=?
		) AS parent
		JOIN peer p ON p.cidr=c.id
		WHERE p.redeemed=1
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
			&peer.Redeemed,
			&peer.Disabled,
		)
		if err != nil {
			return nil, db.CheckSqliteErr("scanning peer info", err)
		}

		peers = append(peers, peer)
	}

	return peers, nil
}
