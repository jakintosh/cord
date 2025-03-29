package server

import (
	"fmt"
	"net"

	db "git.sr.ht/~jakintosh/innernet-go/internal/database"
	"git.sr.ht/~jakintosh/innernet-go/internal/utils"
	wg "git.sr.ht/~jakintosh/innernet-go/internal/wireguard"
)

type Peer struct {
	PeerId    int64
	CidrId    int64
	Name      string
	PublicKey wg.PublicKey
	Cidr      *net.IPNet
	Admin     bool
	Redeemed  bool
	Enabled   bool
}

func (p *Peer) String() string {
	return fmt.Sprintf(
		"%s | %s | %s",
		p.PublicKey.String(),
		p.Cidr.String(),
		p.Name,
	)
}

func (ctx *Context) CreatePeer(
	name string,
	ip net.IP,
	admin bool,
	inviteExpires int64,
) (
	*wg.PublicKey,
	*wg.PeerConfig,
	error,
) {

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

	peerConfig := &wg.PeerConfig{
		Name:       name,
		Ip:         ip,
		PrivateKey: privKey,
	}

	return &pubKey, peerConfig, nil
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
		WHERE name=?1;
		`,
		peer,
		!enabled,
	)
	return db.CheckSqliteErr("setting peer dis/enabled", err)
}

func (ctx *Context) GetPeersofPeerNamed(
	peerName string,
) (
	[]Peer,
	error,
) {

	peerCidrId, parentCidrId, err := ctx.getPeerAndParentCidrIdsForPeerNamed(peerName)
	if err != nil {
		return nil, err
	}

	associatedCidrIds, err := ctx.getAssociatedCidrIdsForCidrId(parentCidrId)
	if err != nil {
		return nil, err
	}

	cidrs := []int64{parentCidrId}
	cidrs = append(cidrs, associatedCidrIds...)

	// get all peers for each "parent" cidr
	peerMap := make(map[Peer]struct{})
	for _, cidrId := range cidrs {

		cidrPeers, err := ctx.GetChildPeersForCidrId(cidrId)
		if err != nil {
			return nil, err
		}

		for _, peer := range cidrPeers {
			if peer.CidrId != peerCidrId {
				peerMap[peer] = struct{}{}
			}
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
	// find all child peers for given cidr id
	rows, err := ctx.Db.Query(`
		SELECT p.id, c.id, c.name, p.public_key, c.cidr, p.admin, p.redeemed, p.disabled
		FROM cidr c
		INNER JOIN (
			SELECT c.name, c.length, c.prefix, c.base, c.last
			FROM cidr c
			WHERE c.id=?
		) AS parent
		JOIN peer p ON p.cidr=c.id
		WHERE c.length=parent.length
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
		var pubKeyString string
		var cidrString string
		err := rows.Scan(
			&peer.PeerId,
			&peer.CidrId,
			&peer.Name,
			&pubKeyString,
			&cidrString,
			&peer.Admin,
			&peer.Redeemed,
			&peer.Enabled,
		)
		if err != nil {
			return nil, db.CheckSqliteErr("scanning peer info", err)
		}

		_, peer.Cidr, err = net.ParseCIDR(cidrString)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to parse cidr (%s) for peer '%s': %w",
				cidrString, peer.Name, err,
			)
		}

		peer.PublicKey, err = wg.ParsePubKey(pubKeyString)
		if err != nil {
			return nil, fmt.Errorf(
				"failed to parse pubkey (%s) for peer '%s': %w",
				pubKeyString, peer.Name, err,
			)
		}

		peers = append(peers, peer)
	}

	return peers, nil
}
