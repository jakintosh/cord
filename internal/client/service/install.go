package service

import (
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// InstallPhase tracks where an install is in the lifecycle.
const (
	PhaseInvited  = "invited"  // invite parsed, permanent key generated
	PhaseRedeemed = "redeemed" // invite redeemed, main network info stored
)

// inviteSuffix is appended to the network name for the temporary invite
// device.
const inviteSuffix = "-i"

// ServerInfo describes how to reach the coordination server over one
// WireGuard network (invite or main). Shared by Install and
// NetworkConfig as persisted domain state.
type ServerInfo struct {
	PublicKey   string
	Endpoint    string // public WG endpoint, host:port
	Route       string // server's in-network route
	NetworkCidr string // full overlay CIDR, e.g. "10.42.0.0/16"
	APIPort     uint16
}

// NetworkAssignment is the durable network identity assigned to a client.
type NetworkAssignment struct {
	AssignedRoute string
	Server        ServerInfo
}

func networkAssignmentFromInvitation(
	invitation protocol.Invitation,
) NetworkAssignment {
	return NetworkAssignment{
		AssignedRoute: invitation.Peer.Route,
		Server: ServerInfo{
			PublicKey:   invitation.Network.PublicKey,
			Endpoint:    invitation.Network.Endpoint,
			Route:       invitation.Network.ServerRoute,
			NetworkCidr: invitation.Network.NetworkCidr,
			APIPort:     invitation.Network.APIPort,
		},
	}
}

// Install is the transient record of an in-progress network install.
// Created by BeginInstall, consumed by ConfirmInstall.
type Install struct {
	Name  string
	Phase string // PhaseInvited or PhaseRedeemed

	// ListenPort is the locally selected WireGuard UDP port. Zero lets the
	// operating system choose an ephemeral port.
	ListenPort uint16

	InviteIfaceName     string
	InvitePrivateKey    string // invite-network identity
	InviteAssignedRoute string
	InviteServer        ServerInfo

	MainIfaceName     string
	MainPrivateKey    string     // permanent identity
	MainAssignedRoute string     // filled at redeem
	MainServer        ServerInfo // filled at redeem

	CreatedAt time.Time
}

// BeginInstallParams contains the validated, service-produced values needed
// to begin an install.
type BeginInstallParams struct {
	Name                string
	ListenPort          uint16
	InviteIfaceName     string
	InvitePrivateKey    string
	InviteAssignedRoute string
	InviteServer        ServerInfo
	MainIfaceName       string
	MainPrivateKey      string
	CreatedAt           time.Time
}

// GetInstall returns the persisted install record by name.
func (s *Service) GetInstall(
	name string,
) (
	*Install,
	error,
) {
	return s.store.GetInstall(name)
}

// ListInstalls returns all in-progress install records.
func (s *Service) ListInstalls() (
	[]*Install,
	error,
) {
	return s.store.ListInstalls()
}

// InstallNetwork runs the full onboarding flow: BeginInstall → RedeemInstall →
// ConfirmInstall. It is a convenience driver for callers that want to complete
// onboarding in a single call. For retry-safe onboarding, call each step
// individually — the permanent key is persisted at BeginInstall and
// reused across retries.
//
// If the install already exists (from a previous partial run),
// InstallNetwork resumes from whatever phase it is at.
func (s *Service) InstallNetwork(
	invitation protocol.Invitation,
	options NetworkOptions,
) (
	*NetworkConfig,
	error,
) {
	install, err := s.BeginInstall(invitation, options)
	if err != nil {
		return nil, err
	}

	installName := install.Name

	if _, err := s.RedeemInstall(installName); err != nil {
		return nil, err
	}

	if err := s.ConfirmInstall(installName); err != nil {
		return nil, err
	}

	return s.store.GetNetwork(installName)
}

// BeginInstall validates an invite, generates a permanent keypair, and
// persists an Install record at phase "invited". No WireGuard devices
// are brought up. Idempotent: if a completed network already exists,
// returns ErrNetworkExists; a compatible install retry returns the
// existing record unchanged, while incompatible invitation identity or
// local options return ErrConflict.
func (s *Service) BeginInstall(
	invitation protocol.Invitation,
	options NetworkOptions,
) (
	*Install,
	error,
) {
	// A parseable-but-incomplete invitation is a bad request; the
	// completeness check is a protocol concern, but the resulting error
	// maps to invalid input at the service boundary.
	if err := invitation.Validate(true); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	networkName := invitation.Network.Name

	// Validate interface names up front.
	mainIfaceName := networkName
	if err := wireguard.ValidateDeviceName(mainIfaceName); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	inviteIfaceName := networkName + inviteSuffix
	if err := wireguard.ValidateDeviceName(inviteIfaceName); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}

	permPrivKey, err := wireguard.GenerateKey()
	if err != nil {
		return nil, err
	}

	listenPort := uint16(0)
	if options.ListenPort != nil {
		listenPort = *options.ListenPort
	}

	install, err := s.store.BeginInstall(BeginInstallParams{
		Name:                networkName,
		ListenPort:          listenPort,
		InviteIfaceName:     inviteIfaceName,
		InvitePrivateKey:    invitation.Peer.PrivateKey,
		InviteAssignedRoute: invitation.Peer.Route,
		InviteServer: ServerInfo{
			PublicKey:   invitation.Network.PublicKey,
			Endpoint:    invitation.Network.Endpoint,
			Route:       invitation.Network.ServerRoute,
			NetworkCidr: invitation.Network.NetworkCidr,
			APIPort:     invitation.Network.APIPort,
		},
		MainIfaceName:  mainIfaceName,
		MainPrivateKey: permPrivKey,
		CreatedAt:      s.clock(),
	})
	if err != nil {
		return nil, err
	}
	s.log.Info("install started", "network", networkName)
	return install, nil
}

// RedeemInstall brings up the temporary invite WireGuard interface, calls
// /redeem with the stored permanent public key, records the main
// network parameters, and tears down the invite interface. Idempotent:
// an already-redeemed install is returned unchanged without contacting
// the server again.
func (s *Service) RedeemInstall(
	name string,
) (
	*Install,
	error,
) {
	install, err := s.store.GetInstall(name)
	if err != nil {
		return nil, err
	}

	if install.Phase == PhaseRedeemed {
		return install, nil
	} else if install.Phase != PhaseInvited {
		return nil, fmt.Errorf(
			"%w: install %q is in phase %q",
			ErrInstallState,
			name,
			install.Phase,
		)
	}

	permPubKey, err := wireguard.PublicKey(install.MainPrivateKey)
	if err != nil {
		return nil, err
	}

	inviteHostRoute, err := netaddr.HostRouteFromCidr(install.InviteAssignedRoute)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid invite route %q", ErrInvalidInput, install.InviteAssignedRoute)
	}

	tunnel, err := newTunnel(
		s.wireguard,
		install.InviteIfaceName,
		install.InvitePrivateKey,
		inviteHostRoute.String(),
		install.InviteServer,
		install.ListenPort,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf("create invite tunnel: %w", err)
	}
	defer func() { _ = tunnel.stop() }()

	inviteClient, err := s.newInviteClient(tunnel)
	if err != nil {
		return nil, fmt.Errorf("create invite client: %w", err)
	}

	result, err := inviteClient.RedeemInvitation(permPubKey)
	if err != nil {
		return nil, fmt.Errorf("redeem invite: %w", err)
	}

	if err := result.Validate(false); err != nil {
		return nil, fmt.Errorf("redeem invite: result invalid: %w", err)
	}
	assignment := networkAssignmentFromInvitation(*result)

	install, err = s.store.RedeemInstall(name, assignment)
	if err != nil {
		return nil, err
	}

	s.log.Info("invite redeemed", "network", name, "route", result.Peer.Route)
	return install, nil
}

// ConfirmInstall brings up the main WireGuard interface, calls /confirm to
// prove reachability, and transitions the install into a permanent
// membership. On success the install row is consumed: the NetworkConfig
// is inserted and the Install row deleted in one transaction. Then a
// runtime Network is constructed adopting the live tunnel, registered,
// and an initial sync runs. Install ends with the network up and enabled.
//
// If the confirm call or store transaction fails, the tunnel comes down
// and the install row remains at "redeemed" — retryable.
func (s *Service) ConfirmInstall(
	name string,
) error {
	install, err := s.store.GetInstall(name)
	if err != nil {
		return err
	}

	if install.Phase != PhaseRedeemed {
		return fmt.Errorf(
			"%w: install %q is in phase %q, expected redeemed",
			ErrInstallState,
			name,
			install.Phase,
		)
	}

	tunnel, err := newTunnel(
		s.wireguard,
		install.MainIfaceName,
		install.MainPrivateKey,
		install.MainAssignedRoute,
		install.MainServer,
		install.ListenPort,
		PersistentKeepaliveInterval,
	)
	if err != nil {
		return fmt.Errorf("create main tunnel: %w", err)
	}

	peerClient, err := s.newPeerClient(tunnel)
	if err != nil {
		_ = tunnel.stop()
		return fmt.Errorf("create peer client: %w", err)
	}

	if err := peerClient.ConfirmPeer(); err != nil {
		_ = tunnel.stop()
		return fmt.Errorf("confirm peer: %w", err)
	}

	nc, err := s.store.ConfirmInstall(
		name,
		install.MainPrivateKey,
		s.clock(),
	)
	if err != nil {
		_ = tunnel.stop()
		return err
	}

	// Adopt the live tunnel into a runtime Network and register it.
	network := s.newNetwork(nc, tunnel, peerClient)
	if err := network.start(); err != nil {
		_ = tunnel.stop()
		return err
	}

	s.mu.Lock()
	s.running[name] = network
	s.mu.Unlock()

	s.log.Info("network confirmed", "network", name, "interface", install.MainIfaceName)
	return nil
}

// UninstallNetwork stops a running network and removes all of its local
// state, whether onboarding is complete or still in progress.
func (s *Service) UninstallNetwork(
	networkName string,
) error {
	s.mu.Lock()
	network, running := s.running[networkName]
	if running {
		delete(s.running, networkName)
	}
	s.mu.Unlock()

	if running {
		if err := network.stop(); err != nil {
			s.log.Warn("uninstall: stop network failed", "network", networkName, "err", err)
		}
	}

	return s.store.DeleteNetworkState(networkName)
}
