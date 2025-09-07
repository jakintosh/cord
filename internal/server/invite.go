package server

import (
	"encoding/json"
	"io"
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

func (i *PeerInvite) Write(w io.Writer) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(i)
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
	return ctx.Store.InviteGetByIP(ip)
}

var tempIPAddr = 1

func (ctx *Context) CreateInvite(
	req CreateInviteRequest,
) (
	*PeerInvite,
	error,
) {
	tempPrivKey, err := wg.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}

	// Generate a temporary IP in the temp range
	tempIP := net.IPv4(10, 0, 255, byte(tempIPAddr))
	tempIPAddr += 1

	// If no expiration provided, default to 24h from now to ensure redeemable
	if req.Expiration.IsZero() {
		req.Expiration = time.Now().Add(24 * time.Hour)
	}

	err = ctx.Store.InviteCreate(
		req.Name,
		tempPrivKey.PublicKey().String(),
		tempIP,
		req.IP,
		req.Admin,
		req.Expiration.Unix(),
	)
	if err != nil {
		return nil, err
	}

	// Create the invite struct with proper network information
	// TODO: Need to implement config reading to get actual server info
	invite := &PeerInvite{}
	invite.Interface.NetworkName = ctx.Name
	invite.Interface.PrivateKey = tempPrivKey.String()
	invite.Interface.AssignedCidr = tempIP.String() + "/32" // Single host assignment

	// These should come from server configuration - placeholder values for now
	invite.Server.PublicKey = "placeholder_server_pubkey"
	invite.Server.ExternalEndpoint = "placeholder_external_endpoint"
	invite.Server.InternalEndpoint = "placeholder_internal_endpoint"

	return invite, nil
}

func (ctx *Context) RedeemInvite(
	pubKey string,
	newKey string,
) error {
	return ctx.Store.InviteRedeem(pubKey, newKey)
}
