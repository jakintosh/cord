package server

import (
	"fmt"
	"net"

	"git.sr.ht/~jakintosh/cord/internal/utils"
	wg "git.sr.ht/~jakintosh/cord/internal/wireguard"
)

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

	if err := ctx.Store.InviteCreate(
		name,
		pubKey.String(),
		utils.GetPeerCidrFromIp(ip).String(),
		admin,
		inviteExpires,
	); err != nil {
		return nil, nil, err
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
	return ctx.Store.InviteRedeem(pubKey, newKey)
}
