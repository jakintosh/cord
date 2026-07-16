package service

import (
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Registration is the server-side stored representation of a pending peer
// registration. It tracks the temporary key, assigned routes, CIDR allocation,
// and redemption state.
type Registration struct {
	Name            string
	InvitePublicKey string    // the temporary public key the peer uses to redeem
	InviteRoute     string    // the temporary host route on the invite overlay
	MainRoute       string    // the permanent host route on the main overlay
	CidrName        string    // the name of the terminal CIDR allocated for this peer
	Admin           bool      // whether the registration grants admin privileges
	Redeemed        bool      // whether the registration has been redeemed
	RedeemedKey     string    // the permanent public key after redemption
	Confirmed       bool      // whether the peer has confirmed via /confirm
	CreatedAt       time.Time // when the registration was created
	ExpiresAt       time.Time // when the registration expires
}

type RegistrationOptions struct {
	IP        net.IP
	Admin     bool
	ExpiresIn *time.Duration
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
	options RegistrationOptions,
) (
	*protocol.Invitation,
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

	if options.IP == nil {
		return nil, fmt.Errorf("%w: peer IP is required", ErrInvalidInput)
	}

	peerMainAssignedIP := netaddr.Normalize(options.IP)
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
	if options.ExpiresIn != nil && *options.ExpiresIn != 0 {
		expiry = *options.ExpiresIn
	}

	now := s.clock()
	tempRoute := netaddr.HostRoute(peerTempAssignedIP)
	mainRoute := netaddr.HostRoute(peerMainAssignedIP)
	reg := &Registration{
		Name:            name,
		InvitePublicKey: peerTempPubKey,
		InviteRoute:     tempRoute.String(),
		MainRoute:       mainRoute.String(),
		CidrName:        name,
		Admin:           options.Admin,
		ExpiresAt:       now.Add(expiry),
		CreatedAt:       now,
	}

	if err := s.store.InsertRegistration(networkName, reg); err != nil {
		return nil, fmt.Errorf("insert registration: %w", mapStoreError(err))
	}
	s.reconcile(networkName)
	s.log.Info("registration created",
		"network", networkName,
		"peer", name,
		"route", mainRoute.String(),
		"expires_at", reg.ExpiresAt,
	)

	_, inviteNet, err := net.ParseCIDR(network.Invite.Cidr)
	if err != nil {
		return nil, fmt.Errorf("parse invite CIDR %q: %w", network.Invite.Cidr, err)
	}

	serverExternalIP := net.ParseIP(network.ExternalIP)
	serverInviteExternalAddr := netaddr.Endpoint(serverExternalIP, network.Invite.WireguardPort)

	serverInternalIP := netaddr.FirstAssignable(inviteNet)

	serverRoute := netaddr.HostRoute(serverInternalIP)
	peerRoute := netaddr.HostRoute(peerTempAssignedIP)

	payload := &protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        network.Name,
			PublicKey:   network.PublicKey,
			Endpoint:    serverInviteExternalAddr,
			ServerRoute: serverRoute.String(),
			NetworkCidr: network.Invite.Cidr,
			APIPort:     network.Invite.ApiPort,
		},
		Peer: protocol.PeerIdentity{
			Route:      peerRoute.String(),
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
	*protocol.Invitation,
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
	s.log.Info("registration redeemed",
		"network", networkName,
		"peer", peer.Name,
		"route", peer.Route,
	)

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
	s.log.Info("registration revoked", "network", network, "peer", name)
	return nil
}

// buildInvitation constructs an Invitation from a network config and a
// redeemed peer.
func (s *Service) buildInvitation(
	network *NetworkConfig,
	peer *Peer,
) (
	*protocol.Invitation,
	error,
) {
	_, rootNet, err := net.ParseCIDR(network.Main.Cidr)
	if err != nil {
		return nil, fmt.Errorf("parse main CIDR %q: %w", network.Main.Cidr, err)
	}

	serverIP := netaddr.FirstAssignable(rootNet)
	serverRoute := netaddr.HostRoute(serverIP)

	peerRoute, err := netaddr.ParseRoute(peer.Route)
	if err != nil {
		return nil, fmt.Errorf("parse peer route %q: %w", peer.Route, err)
	}

	return &protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        network.Name,
			PublicKey:   network.PublicKey,
			Endpoint:    netaddr.Endpoint(net.ParseIP(network.ExternalIP), network.Main.WireguardPort),
			ServerRoute: serverRoute.String(),
			NetworkCidr: network.Main.Cidr,
			APIPort:     network.Main.ApiPort,
		},
		Peer: protocol.PeerIdentity{
			Route: peerRoute.String(),
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
		if reg.InviteRoute != "" {
			ip, _, _ := net.ParseCIDR(reg.InviteRoute)
			if ip != nil {
				used[netaddr.Normalize(ip).String()] = true
			}
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

func registrationsToWireGuardPeers(
	regs []*Registration,
) []wireguard.PeerConfig {
	var wgpeers []wireguard.PeerConfig
	for _, reg := range regs {
		wgpeers = append(wgpeers, wireguard.PeerConfig{
			PublicKey:      reg.InvitePublicKey,
			AllowedIPs:     []string{reg.InviteRoute},
			EndpointPolicy: wireguard.EndpointDynamic,
		})
	}
	return wgpeers
}
