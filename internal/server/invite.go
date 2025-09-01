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
	Name        string
	PublicKey   string
	InviteCidr  string
	NetworkCidr string
	Admin       bool
	Redeemed    bool
	Expiration  time.Time
}

type CreateInviteRequest struct {
	Name       string
	IP         net.IP
	Admin      bool
	Expiration time.Time
}

func (ctx *Context) GetInviteByIP(
	ip net.IP,
) (
	*ServerInvite,
	error,
) {
	return nil, nil
}

var tempIPAddr = 1

func (ctx *Context) CreateInvite(
	req CreateInviteRequest,
) (
	*wg.DeviceConfig,
	*wg.PeerConfig,
	error,
) {
	_, pubKey, err := wg.GenerateKeypair()
	if err != nil {
		return nil, nil, err
	}

	// Generate a temporary IP in the temp range (normalize to 4-byte v4)
	tempIP := net.IPv4(10, 0, 255, byte(tempIPAddr)).To4()
	tempIPAddr += 1

	// If no expiration provided, default to 24h from now to ensure redeemable
	if req.Expiration.IsZero() {
		req.Expiration = time.Now().Add(24 * time.Hour)
	}

	// Normalize final IP to 4-byte v4 if applicable for consistent DB storage
	if v4 := req.IP.To4(); v4 != nil {
		req.IP = v4
	}

	err = ctx.Store.InviteCreate(
		req.Name,
		pubKey.String(),
		tempIP,
		req.IP,
		req.Admin,
		req.Expiration.Unix(),
	)
	if err != nil {
		return nil, nil, err
	}

	return &wg.DeviceConfig{}, &wg.PeerConfig{
		Name:      req.Name,
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
