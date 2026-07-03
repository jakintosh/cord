package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Registration is the server-side stored representation of a pending peer
// registration. It tracks the temporary key, assigned IPs, and redemption state.
type Registration struct {
	Name            string
	InvitePublicKey string    // the temporary public key the peer uses to redeem
	InviteIP        net.IP    // the temporary IP on the invite network
	MainIP          net.IP    // the permanent IP on the main network
	Admin           bool      // whether the registration grants admin privileges
	Redeemed        bool      // whether the registration has been redeemed
	RedeemedKey     string    // the permanent public key after redemption
	Confirmed       bool      // whether the peer has confirmed via /confirm
	CreatedAt       time.Time // when the registration was created
	ExpiresAt       time.Time // when the registration expires
}

// NetworkInfo describes a cord network and how to reach it. It travels
// in invitation payloads and is stored in client-side network config.
type NetworkInfo struct {
	Name        string `json:"name"`
	PublicKey   string `json:"public_key"`
	Endpoint    string `json:"endpoint"`     // external WG endpoint
	APIEndpoint string `json:"api_endpoint"` // internal API endpoint
}

// PeerIdentity describes a peer's assigned identity on the network.
// The PrivateKey is only present in the initial invitation; it is
// omitted from redemption responses.
type PeerIdentity struct {
	CIDR       string `json:"cidr"`
	PrivateKey string `json:"private_key,omitempty"`
}

// Invitation is the opaque JSON payload delivered to a peer. It contains
// everything the peer needs to connect to and authenticate on the invite
// network and redeem a permanent identity.
type Invitation struct {
	Network NetworkInfo  `json:"network"`
	Peer    PeerIdentity `json:"peer"`
}

// ParseInvitation reads and validates an Invitation from a JSON reader.
func ParseInvitation(
	r io.Reader,
) (
	*Invitation,
	error,
) {
	var inv Invitation
	if err := json.NewDecoder(r).Decode(&inv); err != nil {
		return nil, fmt.Errorf("%w: parse invitation: %v", ErrInvalidInput, err)
	}
	return &inv, nil
}

// Write serializes the invitation as JSON to the writer.
func (inv *Invitation) Write(
	w io.Writer,
) error {
	if err := json.NewEncoder(w).Encode(inv); err != nil {
		return fmt.Errorf("write invitation: %w", err)
	}
	return nil
}

// ListRegistrations returns all registrations for the given network
// (active, expired, and redeemed).
func (s *Service) ListRegistrations(
	network string,
) (
	[]*Registration,
	error,
) {
	registrations, err := s.store.ListRegistrations(network)
	if err != nil {
		return nil, fmt.Errorf("list registrations: %w", err)
	}
	return registrations, nil
}

// CreateRegistration reserves an IP on the main network, allocates a
// temporary IP on the invite network, generates a temporary keypair,
// persists the registration record, and returns the Invitation payload
// to deliver to the peer out-of-band.
func (s *Service) CreateRegistration(
	networkName string,
	name string,
	ip *net.IP,
	admin bool,
	expiresIn *time.Duration,
) (
	*Invitation,
	error,
) {
	network, err := s.store.GetNetwork(networkName)
	if err != nil {
		return nil, fmt.Errorf("get network: %w", mapStoreError(err))
	}

	if name == "" {
		return nil, fmt.Errorf("%w: registration name required", ErrInvalidInput)
	}

	exists, err := s.store.PeerExists(networkName, name)
	if err != nil {
		return nil, fmt.Errorf("check peer exists: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("%w: peer %q already exists", ErrConflict, name)
	}

	var peerMainAssignedIP net.IP
	if ip != nil {
		peerMainAssignedIP = netaddr.Normalize(*ip)
		_, mainNet, err := net.ParseCIDR(network.Main.Cidr)
		if err != nil {
			return nil, fmt.Errorf("parse main CIDR: %w", err)
		}
		if !mainNet.Contains(peerMainAssignedIP) {
			return nil, fmt.Errorf(
				"%w: requested IP %s is not within main CIDR %s",
				ErrInvalidInput, peerMainAssignedIP, network.Main.Cidr,
			)
		}
	} else {
		freeIP, err := s.nextFreePeerIP(networkName, network.Main.Cidr)
		if err != nil {
			return nil, fmt.Errorf("auto-assign permanent IP: %w", err)
		}
		peerMainAssignedIP = freeIP
	}

	peerTempPrivKey, err := wireguard.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generate temp key: %w", err)
	}

	peerTempPubKey, err := wireguard.PublicKey(peerTempPrivKey)
	if err != nil {
		return nil, fmt.Errorf("derive temp public key: %w", err)
	}

	peerTempAssignedIP, err := s.nextFreeRegistrationIP(networkName, network.Invite.Cidr)
	if err != nil {
		return nil, fmt.Errorf("allocate invite IP: %w", err)
	}

	expiry := 24 * time.Hour
	if expiresIn != nil && *expiresIn != 0 {
		expiry = *expiresIn
	}

	now := s.clock()
	reg := &Registration{
		Name:            name,
		InvitePublicKey: peerTempPubKey,
		InviteIP:        peerTempAssignedIP,
		MainIP:          peerMainAssignedIP,
		Admin:           admin,
		ExpiresAt:       now.Add(expiry),
		CreatedAt:       now,
	}

	if err := s.store.InsertRegistration(networkName, reg); err != nil {
		return nil, fmt.Errorf("insert registration: %w", mapStoreError(err))
	}
	s.reconcile(networkName)

	_, inviteNet, err := net.ParseCIDR(network.Invite.Cidr)
	if err != nil {
		return nil, fmt.Errorf("parse invite CIDR %q: %w", network.Invite.Cidr, err)
	}
	inviteNetPrefix, _ := inviteNet.Mask.Size()

	peerInviteNet := fmt.Sprintf("%s/%d", peerTempAssignedIP.String(), inviteNetPrefix)

	serverExternalIP := net.ParseIP(network.ExternalIP)
	serverInviteExternalAddr := netaddr.Endpoint(serverExternalIP, network.Invite.WireguardPort)

	serverInternalIP := netaddr.FirstAssignable(inviteNet)
	serverInviteInternalAddr := netaddr.Endpoint(serverInternalIP, network.Invite.ApiPort)

	payload := &Invitation{
		Network: NetworkInfo{
			Name:        network.Name,
			PublicKey:   network.PublicKey,
			Endpoint:    serverInviteExternalAddr,
			APIEndpoint: serverInviteInternalAddr,
		},
		Peer: PeerIdentity{
			CIDR:       peerInviteNet,
			PrivateKey: peerTempPrivKey,
		},
	}

	return payload, nil
}

// RedeemRegistration exchanges a temporary registration key for a
// permanent peer identity. Idempotent: redeeming an already-redeemed
// registration with the same permanent key returns the same result.
func (s *Service) RedeemRegistration(
	networkName string,
	tempPubKey string,
	permPubKey string,
) (
	*Invitation,
	error,
) {
	network, err := s.store.GetNetwork(networkName)
	if err != nil {
		return nil, fmt.Errorf("get network: %w", mapStoreError(err))
	}

	err = s.store.RedeemRegistration(networkName, tempPubKey, permPubKey, s.clock())
	if err != nil {
		peer, lookupErr := s.store.GetPeerByKey(networkName, permPubKey)
		if lookupErr == nil && !peer.Confirmed {
			s.reconcile(networkName)
			return s.buildInvitation(network, peer)
		}
		return nil, fmt.Errorf("redeem registration: %w", mapStoreError(err))
	}
	s.reconcile(networkName)

	peer, err := s.store.GetPeerByKey(networkName, permPubKey)
	if err != nil {
		return nil, fmt.Errorf("get redeemed peer: %w", mapStoreError(err))
	}

	return s.buildInvitation(network, peer)
}

// RevokeRegistration deletes a registration by name, preventing it from
// being redeemed. Any associated temporary key and IP are released.
// After redemption, use ConfirmPeer to confirm the peer instead.
func (s *Service) RevokeRegistration(
	network string,
	name string,
) error {
	if err := s.store.DeleteRegistration(network, name); err != nil {
		return fmt.Errorf("delete registration %q: %w", name, mapStoreError(err))
	}
	s.reconcile(network)
	return nil
}

// buildInvitation constructs an Invitation from a network config and a
// redeemed peer.
func (s *Service) buildInvitation(
	network *NetworkConfig,
	peer *Peer,
) (
	*Invitation,
	error,
) {
	_, rootNet, err := net.ParseCIDR(network.Main.Cidr)
	if err != nil {
		return nil, fmt.Errorf("parse main CIDR %q: %w", network.Main.Cidr, err)
	}
	networkPrefix, _ := rootNet.Mask.Size()

	peerIP, _, err := net.ParseCIDR(peer.Cidr)
	if err != nil {
		return nil, fmt.Errorf("parse peer CIDR %q: %w", peer.Cidr, err)
	}

	return &Invitation{
		Network: NetworkInfo{
			Name:        network.Name,
			PublicKey:   network.PublicKey,
			Endpoint:    netaddr.Endpoint(net.ParseIP(network.ExternalIP), network.Main.WireguardPort),
			APIEndpoint: netaddr.Endpoint(netaddr.FirstAssignable(rootNet), network.Main.ApiPort),
		},
		Peer: PeerIdentity{
			CIDR: fmt.Sprintf("%s/%d", peerIP.String(), networkPrefix),
		},
	}, nil
}

// nextFreeRegistrationIP finds the lowest free address on the invite
// network, skipping the network address and the server's invite address.
func (s *Service) nextFreeRegistrationIP(
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

	regs, err := s.store.ListRegistrations(network)
	if err != nil {
		return nil, fmt.Errorf("list registrations: %w", err)
	}

	used := map[string]bool{}
	for _, reg := range regs {
		if reg.InviteIP != nil {
			used[netaddr.Normalize(reg.InviteIP).String()] = true
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

// nextFreePeerIP finds the lowest free address in the root CIDR,
// skipping the network address, the server address, and addresses
// held by existing peers or active registrations.
func (s *Service) nextFreePeerIP(
	network string,
	rootCidr string,
) (
	net.IP,
	error,
) {
	_, ipNet, err := net.ParseCIDR(rootCidr)
	if err != nil {
		return nil, fmt.Errorf("parse root CIDR: %w", err)
	}

	peers, err := s.store.ListPeers(network)
	if err != nil {
		return nil, fmt.Errorf("list peers: %w", err)
	}

	regs, err := s.store.ListActiveRegistrations(network, s.clock())
	if err != nil {
		return nil, fmt.Errorf("list active registrations: %w", err)
	}

	used := map[string]bool{}
	for _, p := range peers {
		ip, _, _ := net.ParseCIDR(p.Cidr)
		if ip != nil {
			used[netaddr.Normalize(ip).String()] = true
		}
	}
	for _, reg := range regs {
		if reg.MainIP != nil {
			used[netaddr.Normalize(reg.MainIP).String()] = true
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

	return nil, fmt.Errorf("%w: no free addresses in %s", ErrInvalidInput, rootCidr)
}
