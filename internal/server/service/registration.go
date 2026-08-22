package service

import (
	"fmt"
	"net"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Registration is the server-side stored representation of peer onboarding.
// It reserves its main route but does not own a CIDR. Before redemption there
// is no peer or terminal CIDR. Redemption creates both while group intent stays
// on the registration. Confirmation atomically moves that intent to the CIDR
// and makes the registration an immutable audit record.
type Registration struct {
	Name            string
	InvitePublicKey string    // the temporary public key the peer uses to redeem
	InviteRoute     string    // the temporary host route on the invite overlay
	MainRoute       string    // the permanent host route on the main overlay
	Admin           bool      // whether the registration grants admin privileges
	Redeemed        bool      // whether the registration has been redeemed
	RedeemedKey     string    // the permanent public key after redemption
	Confirmed       bool      // whether the peer has confirmed via /confirm
	CreatedAt       time.Time // when the registration was created
	ExpiresAt       time.Time // when the registration expires
}

type RegistrationOptions struct {
	PeerIP    net.IP
	Admin     bool
	ExpiresIn *time.Duration
}

// CreateRegistrationParams contains the registration properties known before the
// store allocates its invite route.
type CreateRegistrationParams struct {
	Name            string
	InvitePublicKey string
	MainRoute       string
	Admin           bool
	CreatedAt       time.Time
	ExpiresAt       time.Time
}

// ListRegistrationGroups returns the groups held by a registration before
// confirmation. Confirmed registrations have no registration-held groups.
func (s *Service) ListRegistrationGroups(
	networkName string,
	regName string,
) (
	[]*Group,
	error,
) {
	groups, err := s.store.ListRegistrationGroups(networkName, regName)
	if err != nil {
		return nil, fmt.Errorf("list registration groups: %w", mapStoreError(err))
	}

	return groups, nil
}

// AssignRegistrationGroup records a group assignment as registration intent.
// The assignment is transferred to the peer's terminal CIDR at confirmation.
func (s *Service) AssignRegistrationGroup(
	networkName string,
	regName string,
	group string,
) error {
	if networkName == "" || regName == "" || group == "" {
		return fmt.Errorf(
			"%w: network, registration, and group names are required",
			ErrInvalidInput,
		)
	}

	if err := s.store.AssignRegistrationGroup(
		networkName,
		regName,
		group,
		s.clock(),
	); err != nil {
		return fmt.Errorf("assign registration group: %w", mapStoreError(err))
	}

	return nil
}

// RemoveRegistrationGroup removes group intent from an unconfirmed
// registration. Removing an assignment that is not present is idempotent.
func (s *Service) RemoveRegistrationGroup(
	networkName string,
	regName string,
	group string,
) error {
	if networkName == "" || regName == "" || group == "" {
		return fmt.Errorf(
			"%w: network, registration, and group names are required",
			ErrInvalidInput,
		)
	}

	if err := s.store.RemoveRegistrationGroup(
		networkName,
		regName,
		group,
		s.clock(),
	); err != nil {
		return fmt.Errorf("remove registration group: %w", mapStoreError(err))
	}

	return nil
}

// ListRegistrations returns all registrations for the given network
// (active, expired, and redeemed).
func (s *Service) ListRegistrations(
	networkName string,
) (
	[]*Registration,
	error,
) {
	registrations, err := s.store.ListRegistrations(networkName)
	if err != nil {
		return nil, fmt.Errorf("list registrations: %w", err)
	}

	return registrations, nil
}

// ListRegistrationPeers returns the WireGuard peer set the network's invite plane
// should carry — one temporary peer per unexpired, unredeemed
// registration — together with the earliest expiry among them. The
// expiry is zero when no registration is pending; the runtime uses it to
// schedule the reconciliation that retires the peer.
func (s *Service) ListRegistrationPeers(
	network string,
) (
	[]wireguard.PeerConfig,
	time.Time,
	error,
) {
	regs, err := s.store.ListActiveRegistrations(network, s.clock())
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("invite peers: %w", mapStoreError(err))
	}

	var expiry time.Time
	for _, reg := range regs {
		if expiry.IsZero() || reg.ExpiresAt.Before(expiry) {
			expiry = reg.ExpiresAt
		}
	}

	return registrationsToWireGuardPeers(regs), expiry, nil
}

// CreateRegistration reserves an IP on the main network, allocates a
// temporary IP on the invite network, generates a temporary keypair,
// persists the registration record, and returns the Invitation payload
// to deliver to the peer out-of-band.
func (s *Service) CreateRegistration(
	networkName string,
	regName string,
	opts RegistrationOptions,
) (
	*protocol.Invitation,
	error,
) {
	now := s.clock()

	if networkName == "" {
		return nil, fmt.Errorf("%w: network name required", ErrInvalidInput)
	}

	if regName == "" {
		return nil, fmt.Errorf("%w: registration name required", ErrInvalidInput)
	}

	if opts.PeerIP == nil {
		return nil, fmt.Errorf("%w: peer IP is required", ErrInvalidInput)
	}

	peerTempPrivKey, err := wireguard.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generate temp key: %w", err)
	}

	peerTempPubKey, err := wireguard.PublicKey(peerTempPrivKey)
	if err != nil {
		return nil, fmt.Errorf("derive temp public key: %w", err)
	}

	network, err := s.store.GetNetwork(networkName)
	if err != nil {
		return nil, fmt.Errorf("get network: %w", mapStoreError(err))
	}

	serverExternalIP := net.ParseIP(network.ExternalIP)
	if serverExternalIP == nil {
		return nil, fmt.Errorf("parse external IP %q: invalid address", network.ExternalIP)
	}
	serverInviteExternalAddr := netaddr.Endpoint(serverExternalIP, network.Invite.WireguardPort)

	_, mainNet, err := net.ParseCIDR(network.Main.Cidr)
	if err != nil {
		return nil, fmt.Errorf("parse main CIDR: %w", err)
	}

	if !mainNet.Contains(opts.PeerIP) {
		return nil, fmt.Errorf(
			"%w: requested IP %s is not within main CIDR %s",
			ErrInvalidInput, opts.PeerIP, network.Main.Cidr,
		)
	}
	mainRoute := netaddr.HostRoute(opts.PeerIP)

	_, inviteNet, err := net.ParseCIDR(network.Invite.Cidr)
	if err != nil {
		return nil, fmt.Errorf("parse invite CIDR %q: %w", network.Invite.Cidr, err)
	}

	expiry := 24 * time.Hour
	if opts.ExpiresIn != nil && *opts.ExpiresIn != 0 {
		expiry = *opts.ExpiresIn
	}

	reg, err := s.store.CreateRegistration(
		networkName,
		CreateRegistrationParams{
			Name:            regName,
			InvitePublicKey: peerTempPubKey,
			MainRoute:       mainRoute.String(),
			Admin:           opts.Admin,
			ExpiresAt:       now.Add(expiry),
			CreatedAt:       now,
		},
		now,
	)
	if err != nil {
		return nil, fmt.Errorf("create registration: %w", mapStoreError(err))
	}

	s.requestReconcile(networkName)

	s.log.Info("registration created",
		"network", networkName,
		"peer", regName,
		"route", mainRoute.String(),
		"expires_at", reg.ExpiresAt,
	)

	serverRoute := netaddr.HostRoute(netaddr.FirstAssignable(inviteNet))
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
			Route:      reg.InviteRoute,
			PrivateKey: peerTempPrivKey,
		},
	}

	return payload, nil
}

// RedeemRegistration exchanges a temporary registration key for a
// permanent peer identity. Redeeming an already-redeemed registration with
// the same permanent key is idempotent while the peer remains unconfirmed.
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

	peer, err := s.store.RedeemRegistration(
		networkName,
		tempPubKey,
		permPubKey,
		s.clock(),
	)
	if err != nil {
		return nil, fmt.Errorf("redeem registration: %w", mapStoreError(err))
	}

	s.requestReconcile(networkName)

	s.log.Info("registration redeemed",
		"network", networkName,
		"peer", peer.Name,
		"route", peer.Route,
	)

	return s.buildInvitation(network, peer)
}

// RevokeRegistration deletes an unconfirmed registration by name, preventing
// redemption or confirmation. Any provisional peer and its routes are also
// removed. Confirmed registrations are immutable audit records.
func (s *Service) RevokeRegistration(
	networkName string,
	regName string,
) error {
	if err := s.store.DeleteRegistration(
		networkName,
		regName,
	); err != nil {
		return fmt.Errorf("delete registration %q: %w", regName, mapStoreError(err))
	}

	s.requestReconcile(networkName)

	s.log.Info(
		"registration revoked",
		"network", networkName,
		"peer", regName,
	)

	return nil
}

// buildInvitation constructs an Invitation from a network config and a
// redeemed peer.
func (s *Service) buildInvitation(
	network *Network,
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

func registrationsToWireGuardPeers(
	regs []*Registration,
) []wireguard.PeerConfig {
	wgpeers := make([]wireguard.PeerConfig, 0, len(regs))
	for _, reg := range regs {
		wgpeers = append(wgpeers, wireguard.PeerConfig{
			PublicKey:      reg.InvitePublicKey,
			AllowedIPs:     []string{reg.InviteRoute},
			EndpointPolicy: wireguard.EndpointDynamic,
		})
	}
	return wgpeers
}
