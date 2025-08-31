package server

import (
	"net"
	"time"

	wg "git.sr.ht/~jakintosh/cord/internal/wireguard"
)

type PeerInvite struct {
	Interface struct {
		NetworkName  string `json:"networkName"`
		PrivateKey   string `json:"privateKey"`
		AssignedCidr string `json:"assignedCidr"`
	} `json:"interface"`
	Server struct {
		PublicKey        string `json:"publicKey"`
		ExternalEndpoint string `json:"externalEndpoint"`
		InternalEndpoint string `json:"internalEndpoint"`
	} `json:"server"`
}

type ServerInvite struct {
	PublicKey   string
	InviteCidr  string
	NetworkCidr string
	Name        string
	Admin       bool
	Redeemed    bool
	Expiration  time.Time
}

func (ctx *Context) GetInviteByIP(
	ip net.IP,
) (
	*ServerInvite,
	error,
) {
	return &ServerInvite{
		PublicKey:   "abc123",
		InviteCidr:  "10.0.64.1/32",
		NetworkCidr: "10.0.0.1/32",
		Name:        "example",
		Admin:       true,
		Redeemed:    false,
		Expiration:  time.Now(),
	}, nil
}

var tempIPAddr = 1

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
	_, pubKey, err := wg.GenerateKeypair()
	if err != nil {
		return nil, nil, err
	}

	// Generate a temporary IP in the temp range (you may want to make this more sophisticated)
	tempIP := net.IPv4(10, 0, 255, byte(tempIPAddr))
	tempIPAddr += 1

	err = ctx.Store.InviteCreate(
		name,
		pubKey.String(),
		tempIP,
		ip,
		admin,
		inviteExpires,
	)
	if err != nil {
		return nil, nil, err
	}

	return &wg.DeviceConfig{}, &wg.PeerConfig{
		Name:      name,
		Cidr:      &net.IPNet{},
		PublicKey: pubKey,
	}, nil
}

func (ctx *Context) RedeemInvite(
	pubKey string,
	newKey string,
) error {
	return ctx.Store.InviteRedeem(pubKey, newKey)
}
