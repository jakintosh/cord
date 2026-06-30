package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
)

// Domain Invite types — these represent the server-side stored state.

// Invite is the server-side stored representation of an invite. It
// tracks the temporary key, assigned IPs, and redemption state.
type Invite struct {
	Name         string
	InvitePubKey string    // the temporary public key the invitee uses to redeem
	InviteIP     net.IP    // the temporary IP on the invite network
	MainIP       net.IP    // the permanent IP on the main network
	Admin        bool      // whether the invite grants admin privileges
	Redeemed     bool      // whether the invite has been redeemed
	RedeemedKey  string    // the permanent public key after redemption
	Confirmed    bool      // whether the peer has confirmed via /confirm
	CreatedAt    time.Time // when the invite was created
	ExpiresAt    time.Time // when the invite expires
}

// RedeemResult is returned to a peer after successful invite
// redemption. It contains the permanent network identity.
type RedeemResult struct {
	NetworkName  string     `json:"network_name"`
	AssignedCidr string     `json:"assigned_cidr"`
	Server       ServerInfo `json:"server"`
}

// External Invite types — these travel over the wire in JSON form.

// ServerInfo describes how to reach the coordination server. It travels
// in invite payloads and is stored in client-side network config.
type ServerInfo struct {
	PublicKey        string `json:"public_key"`
	ExternalEndpoint string `json:"external_endpoint"`
	InternalEndpoint string `json:"internal_endpoint"`
}

// InviteInterface describes the temporary WireGuard identity given to
// an invitee for connecting to the invite network.
type InviteInterface struct {
	NetworkName  string `json:"network_name"`
	PrivateKey   string `json:"private_key"`
	AssignedCidr string `json:"assigned_cidr"`
}

// PeerInvite is the opaque JSON payload delivered out-of-band to a
// prospective peer. It contains everything the client needs to connect
// to the invite network and redeem a permanent identity.
type PeerInvite struct {
	Interface InviteInterface `json:"interface"`
	Server    ServerInfo      `json:"server"`
}

// ParseInvite reads and validates a PeerInvite from a JSON reader.
func ParseInvite(
	r io.Reader,
) (
	*PeerInvite,
	error,
) {
	var inv PeerInvite
	if err := json.NewDecoder(r).Decode(&inv); err != nil {
		return nil, fmt.Errorf("%w: parse invite: %v", ErrInvalidInput, err)
	}
	return &inv, nil
}

// Write serializes the invite as JSON to the writer.
func (inv *PeerInvite) Write(
	w io.Writer,
) error {
	if err := json.NewEncoder(w).Encode(inv); err != nil {
		return fmt.Errorf("write invite: %w", err)
	}
	return nil
}

// endpointTTL is how long an endpoint sighting is considered current.
const endpointTTL = 24 * time.Hour

// ListInvites returns all invites for the given network (active,
// expired, and redeemed).
func (s *Service) ListInvites(
	network string,
) (
	[]*Invite,
	error,
) {
	invites, err := s.store.ListInvites(network)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	return invites, nil
}

// CreateInvite reserves an IP on the main network, allocates a
// temporary IP on the invite network, generates a temporary keypair,
// persists the invite record, and returns the PeerInvite payload to
// deliver to the invitee out-of-band.
func (s *Service) CreateInvite(
	networkName string,
	name string,
	ip *net.IP,
	admin bool,
	expiresIn *time.Duration,
) (
	*PeerInvite,
	error,
) {
	network, err := s.store.GetNetwork(networkName)
	if err != nil {
		return nil, fmt.Errorf("get network: %w", mapStoreError(err))
	}

	if name == "" {
		return nil, fmt.Errorf("%w: invite name required", ErrInvalidInput)
	}

	var peerMainAssignedIP net.IP
	if ip != nil {
		peerMainAssignedIP = netaddr.Normalize(*ip)
	} else {
		freeIP, err := s.nextFreePeerIP(networkName, network.MainCidr)
		if err != nil {
			return nil, fmt.Errorf("auto-assign permanent IP: %w", err)
		}
		peerMainAssignedIP = freeIP
	}

	peerInvitePrivKey, err := s.wg.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generate temp key: %w", err)
	}

	peerInvitePubKey, err := s.wg.PublicKey(peerInvitePrivKey)
	if err != nil {
		return nil, fmt.Errorf("derive temp public key: %w", err)
	}

	peerInviteAssignedIP, err := s.nextFreeInviteIP(networkName, network.InviteCidr)
	if err != nil {
		return nil, fmt.Errorf("allocate invite IP: %w", err)
	}

	expiry := 24 * time.Hour
	if expiresIn != nil && *expiresIn != 0 {
		expiry = *expiresIn
	}

	now := s.clock()
	invite := &Invite{
		Name:         name,
		InvitePubKey: peerInvitePubKey,
		InviteIP:     peerInviteAssignedIP,
		MainIP:       peerMainAssignedIP,
		Admin:        admin,
		ExpiresAt:    now.Add(expiry),
		CreatedAt:    now,
	}

	if err := s.store.InsertInvite(networkName, invite); err != nil {
		return nil, fmt.Errorf("insert invite: %w", mapStoreError(err))
	}
	s.reconcileOnce(networkName)

	_, inviteNet, err := net.ParseCIDR(network.InviteCidr)
	if err != nil {
		return nil, fmt.Errorf("parse invite CIDR %q: %w", network.InviteCidr, err)
	}
	inviteNetPrefix, _ := inviteNet.Mask.Size()

	peerInviteNet := fmt.Sprintf("%s/%d", peerInviteAssignedIP.String(), inviteNetPrefix)

	serverExternalIp := net.ParseIP(network.ExternalIP)
	serverInviteExternalAddr := netaddr.Endpoint(serverExternalIp, network.InviteWireguardPort)

	serverInternalIp := netaddr.FirstAssignable(inviteNet)
	serverIniviteInternalAddr := netaddr.Endpoint(serverInternalIp, network.InviteApiPort)

	payload := &PeerInvite{
		Interface: InviteInterface{
			NetworkName:  network.Name,
			PrivateKey:   peerInvitePrivKey,
			AssignedCidr: peerInviteNet,
		},
		Server: ServerInfo{
			PublicKey:        network.PublicKey,
			ExternalEndpoint: serverInviteExternalAddr,
			InternalEndpoint: serverIniviteInternalAddr,
		},
	}

	return payload, nil
}

// RedeemInvite exchanges a temporary invite key for a permanent peer
// registration. Idempotent: redeeming an already-redeemed invite with
// the same permanent key returns the same result.
func (s *Service) RedeemInvite(
	networkName string,
	tempPubKey string,
	permPubKey string,
) (
	*RedeemResult,
	error,
) {
	network, err := s.store.GetNetwork(networkName)
	if err != nil {
		return nil, fmt.Errorf("get network: %w", mapStoreError(err))
	}

	err = s.store.RedeemInvite(networkName, tempPubKey, permPubKey, s.clock())
	if err != nil {
		peer, lookupErr := s.store.GetPeerByKey(networkName, permPubKey)
		if lookupErr == nil && !peer.Confirmed {
			s.reconcileOnce(networkName)
			return s.buildRedeemResult(network, peer)
		}
		return nil, fmt.Errorf("redeem invite: %w", mapStoreError(err))
	}
	s.reconcileOnce(networkName)

	peer, err := s.store.GetPeerByKey(networkName, permPubKey)
	if err != nil {
		return nil, fmt.Errorf("get redeemed peer: %w", mapStoreError(err))
	}

	return s.buildRedeemResult(network, peer)
}

// RevokeInvite deletes an invite by name (the only operation that
// physically removes an invite record), preventing it from being
// redeemed. Any associated temporary key and IP are released.
// After redemption, use ConfirmPeer to confirm the peer instead.
func (s *Service) RevokeInvite(
	network string,
	name string,
) error {
	if err := s.store.DeleteInvite(network, name); err != nil {
		return fmt.Errorf("delete invite %q: %w", name, mapStoreError(err))
	}
	s.reconcileOnce(network)
	return nil
}

// buildRedeemResult constructs the RedeemResult from a network and
// redeemed peer.
func (s *Service) buildRedeemResult(
	network *Network,
	peer *Peer,
) (
	*RedeemResult,
	error,
) {
	_, rootNet, err := net.ParseCIDR(network.MainCidr)
	if err != nil {
		return nil, fmt.Errorf("parse main CIDR %q: %w", network.MainCidr, err)
	}
	networkPrefix, _ := rootNet.Mask.Size()

	peerIP, _, err := net.ParseCIDR(peer.Cidr)
	if err != nil {
		return nil, fmt.Errorf("parse peer CIDR %q: %w", peer.Cidr, err)
	}

	return &RedeemResult{
		NetworkName:  network.Name,
		AssignedCidr: fmt.Sprintf("%s/%d", peerIP.String(), networkPrefix),
		Server: ServerInfo{
			PublicKey:        network.PublicKey,
			ExternalEndpoint: netaddr.Endpoint(net.ParseIP(network.ExternalIP), network.MainWireguardPort),
			InternalEndpoint: netaddr.Endpoint(netaddr.FirstAssignable(rootNet), network.MainApiPort),
		},
	}, nil
}

// nextFreeInviteIP finds the lowest free address on the invite network,
// skipping the network address and the server's invite address.
func (s *Service) nextFreeInviteIP(
	network string,
	inviteCidr string,
) (
	net.IP,
	error,
) {
	_, ipNet, err := net.ParseCIDR(inviteCidr)
	if err != nil {
		return nil, fmt.Errorf("parse invite CIDR: %w", err)
	}

	invites, err := s.store.ListInvites(network)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}

	used := map[string]bool{}
	for _, inv := range invites {
		if inv.InviteIP != nil {
			used[netaddr.Normalize(inv.InviteIP).String()] = true
		}
	}

	first := netaddr.FirstAssignable(ipNet)
	_, last := netaddr.Range(ipNet)

	candidate := netaddr.Increment(first)
	for ipNet.Contains(candidate) && !candidate.Equal(last) {
		if !used[netaddr.Normalize(candidate).String()] {
			return netaddr.Normalize(candidate), nil
		}
		candidate = netaddr.Increment(candidate)
	}

	return nil, fmt.Errorf("%w: no free addresses in invite CIDR %s", ErrInvalidInput, inviteCidr)
}
