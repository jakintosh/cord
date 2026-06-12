package server

import (
	"fmt"
	"io"
	"net"
	"time"

	"github.com/BurntSushi/toml"

	"git.sr.ht/~jakintosh/cord/internal/utils"
	wg "git.sr.ht/~jakintosh/cord/internal/wireguard"
)

// ServerInfo describes how peers reach the coordination server: its
// WireGuard identity, public endpoint, and internal API address. It
// travels as JSON over the API and as TOML in files on disk.
type ServerInfo struct {
	PublicKey        string `json:"publicKey" toml:"public_key"`
	ExternalEndpoint string `json:"externalEndpoint" toml:"external_endpoint"`
	InternalEndpoint string `json:"internalEndpoint" toml:"internal_endpoint"`
}

// InviteInterface is the temporary identity an invitee uses on the
// invite network.
type InviteInterface struct {
	NetworkName  string `json:"networkName" toml:"network_name"`
	PrivateKey   string `json:"privateKey" toml:"private_key"`
	AssignedCidr string `json:"assignedCidr" toml:"assigned_cidr"`
}

// PeerInvite is the payload written to an invite file and delivered
// out-of-band to a prospective peer. It contains everything the client
// needs to join the invite network and redeem its place on the main one.
type PeerInvite struct {
	Interface InviteInterface `json:"interface" toml:"interface"`
	Server    ServerInfo      `json:"server" toml:"server"`
}

func (i *PeerInvite) Write(w io.Writer) error {
	return toml.NewEncoder(w).Encode(i)
}

func ReadPeerInvite(r io.Reader) (*PeerInvite, error) {
	invite := &PeerInvite{}
	if _, err := toml.NewDecoder(r).Decode(invite); err != nil {
		return nil, fmt.Errorf("failed to parse invite: %w", err)
	}
	return invite, nil
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

// RedeemResult is what a redeeming peer receives: its assignment on the
// main network and how to reach the server there.
type RedeemResult struct {
	NetworkName  string     `json:"networkName"`
	AssignedCidr string     `json:"assignedCidr"`
	Server       ServerInfo `json:"server"`
}

func (srv *Server) GetInviteByIP(
	ip net.IP,
) (
	*ServerInvite,
	error,
) {
	return srv.Store.InviteGetByIPAny(ip)
}

// CreateInvite reserves the requested main-network IP for a new peer,
// assigns it a temporary identity on the invite network, and returns
// the invite payload to deliver out-of-band.
func (srv *Server) CreateInvite(
	req CreateInviteRequest,
) (
	*PeerInvite,
	error,
) {
	cfg, err := srv.LoadConfig()
	if err != nil {
		return nil, err
	}

	inviteNet, err := cfg.InviteNet()
	if err != nil {
		return nil, err
	}

	tempPrivKey, err := wg.GeneratePrivateKey()
	if err != nil {
		return nil, err
	}

	tempIP, err := srv.nextInviteIP(inviteNet)
	if err != nil {
		return nil, err
	}

	// If no expiration provided, default to 24h from now
	if req.Expiration.IsZero() {
		req.Expiration = time.Now().Add(24 * time.Hour)
	}

	err = srv.Store.InviteCreate(
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

	prefix, _ := inviteNet.Mask.Size()
	inviteApiEndpoint, err := cfg.InviteApiEndpoint()
	if err != nil {
		return nil, err
	}

	invite := &PeerInvite{}
	invite.Interface.NetworkName = srv.Network
	invite.Interface.PrivateKey = tempPrivKey.String()
	invite.Interface.AssignedCidr = fmt.Sprintf("%s/%d", tempIP.String(), prefix)
	invite.Server.PublicKey = cfg.PublicKey
	invite.Server.ExternalEndpoint = cfg.ExternalInviteEndpoint()
	invite.Server.InternalEndpoint = inviteApiEndpoint

	return invite, nil
}

// nextInviteIP finds the lowest free address on the invite network,
// skipping the network address, the server's own invite address, and
// addresses held by existing invite records.
func (srv *Server) nextInviteIP(
	inviteNet *net.IPNet,
) (
	net.IP,
	error,
) {
	invites, err := srv.Store.InviteList()
	if err != nil {
		return nil, err
	}

	used := map[string]bool{}
	for _, invite := range invites {
		ip, _, err := net.ParseCIDR(invite.InviteCidr)
		if err != nil {
			continue
		}
		used[utils.NormalizeIP(ip).String()] = true
	}

	serverIP := utils.GetFirstAssignableIpFromCidr(inviteNet)
	_, last := utils.GetIpRangeFromCidr(inviteNet)

	candidate := utils.IncrementIP(serverIP)
	for inviteNet.Contains(candidate) && !candidate.Equal(last) {
		if !used[candidate.String()] {
			return candidate, nil
		}
		candidate = utils.IncrementIP(candidate)
	}

	return nil, fmt.Errorf("invite network '%s' has no free addresses", inviteNet.String())
}

// RedeemInvite trades an invite's temporary key for a permanent peer
// registration. Idempotent: redeeming an already-redeemed invite with
// the same permanent key returns the same result.
func (srv *Server) RedeemInvite(
	invite *ServerInvite,
	permKey string,
) (
	*RedeemResult,
	error,
) {
	if err := srv.Store.InviteRedeem(invite.PublicKey, permKey); err != nil {
		// the invite may already be redeemed with this same key; if
		// so, return the same configuration again so the client can
		// retry a flow the network interrupted
		peer, lookupErr := srv.Store.PeerGetByKey(permKey)
		if lookupErr == nil && !peer.Confirmed && peer.Name == invite.Name {
			return srv.redeemResultForPeer(peer)
		}
		return nil, err
	}

	peer, err := srv.Store.PeerGetByKey(permKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load redeemed peer: %w", err)
	}

	return srv.redeemResultForPeer(peer)
}

func (srv *Server) redeemResultForPeer(
	peer *Peer,
) (
	*RedeemResult,
	error,
) {
	cfg, err := srv.LoadConfig()
	if err != nil {
		return nil, err
	}

	rootNet, err := cfg.RootNet()
	if err != nil {
		return nil, err
	}

	peerIP, _, err := net.ParseCIDR(peer.Cidr)
	if err != nil {
		return nil, fmt.Errorf("invalid peer cidr '%s': %w", peer.Cidr, err)
	}

	apiEndpoint, err := cfg.InternalApiEndpoint()
	if err != nil {
		return nil, err
	}

	prefix, _ := rootNet.Mask.Size()
	result := &RedeemResult{
		NetworkName:  srv.Network,
		AssignedCidr: fmt.Sprintf("%s/%d", peerIP.String(), prefix),
	}
	result.Server.PublicKey = cfg.PublicKey
	result.Server.ExternalEndpoint = cfg.ExternalEndpoint()
	result.Server.InternalEndpoint = apiEndpoint

	return result, nil
}
