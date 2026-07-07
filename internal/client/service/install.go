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
	PublicKey string
	Endpoint  string // public WG endpoint, host:port
	Route     string // server's in-network route
	APIPort   uint16
}

// Install is the transient record of an in-progress network install.
// Created by BeginInstall, consumed by Confirm.
type Install struct {
	Name  string
	Phase string // PhaseInvited or PhaseRedeemed

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

// InstallNetwork runs the full onboarding flow: BeginInstall → Redeem →
// Confirm. It is a convenience driver for callers that want to complete
// onboarding in a single call. For retry-safe onboarding, call each step
// individually — the permanent key is persisted at BeginInstall and
// reused across retries.
//
// If the install already exists (from a previous partial run),
// InstallNetwork resumes from whatever phase it is at.
func (s *Service) InstallNetwork(
	invitation protocol.Invitation,
) (
	*NetworkConfig,
	error,
) {
	inst, err := s.BeginInstall(invitation)
	if err != nil {
		return nil, err
	}

	if inst.Phase == PhaseInvited {
		if _, err := s.Redeem(inst.Name); err != nil {
			return nil, err
		}
	}

	inst, err = s.store.GetInstall(inst.Name)
	if err != nil {
		return nil, err
	}

	if inst.Phase == PhaseRedeemed {
		if err := s.Confirm(inst.Name); err != nil {
			return nil, err
		}
	}

	return s.store.GetNetwork(inst.Name)
}

// BeginInstall validates an invite, generates a permanent keypair, and
// persists an Install record at phase "invited". No WireGuard devices
// are brought up. Idempotent: if a completed network already exists,
// returns ErrNetworkExists; if an install with the same name already
// exists at any phase, the existing record is returned unchanged.
func (s *Service) BeginInstall(
	invitation protocol.Invitation,
) (
	*Install,
	error,
) {
	// A parseable-but-incomplete invitation is a bad request; the
	// completeness check is a protocol concern, but the resulting error
	// maps to invalid input at the service boundary.
	if err := invitation.Validate(); err != nil {
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

	if _, err := s.store.GetNetwork(networkName); err == nil {
		return nil, ErrNetworkExists
	}

	if existing, err := s.store.GetInstall(networkName); err == nil {
		return existing, nil
	}

	permPrivKey, err := wireguard.GenerateKey()
	if err != nil {
		return nil, err
	}

	install := &Install{
		Name:                networkName,
		Phase:               PhaseInvited,
		InviteIfaceName:     inviteIfaceName,
		InvitePrivateKey:    invitation.Peer.PrivateKey,
		InviteAssignedRoute: invitation.Peer.Route,
		InviteServer: ServerInfo{
			PublicKey: invitation.Network.PublicKey,
			Endpoint:  invitation.Network.Endpoint,
			Route:     invitation.Network.ServerRoute,
			APIPort:   invitation.Network.APIPort,
		},
		MainIfaceName:  mainIfaceName,
		MainPrivateKey: permPrivKey,
		CreatedAt:      s.clock(),
	}
	if err := s.store.InsertInstall(install); err != nil {
		return nil, err
	}
	s.log.Info("install started", "network", networkName)
	return install, nil
}

// Redeem brings up the temporary invite WireGuard interface, calls
// /redeem with the stored permanent public key, records the main
// network parameters, and tears down the invite interface. The install
// must be at phase "invited" or "redeemed". Idempotent: re-calling in
// the redeemed state re-contacts the server with the same key.
func (s *Service) Redeem(
	name string,
) (
	*protocol.Invitation,
	error,
) {
	install, err := s.store.GetInstall(name)
	if err != nil {
		return nil, err
	}

	if install.Phase != PhaseInvited && install.Phase != PhaseRedeemed {
		return nil, fmt.Errorf("%w: install %q is in phase %q, expected invited or redeemed",
			ErrInvalidInput, name, install.Phase)
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

	if err := s.store.RedeemInstall(
		name,
		result.Peer.Route,
		ServerInfo{
			PublicKey: result.Network.PublicKey,
			Endpoint:  result.Network.Endpoint,
			Route:     result.Network.ServerRoute,
			APIPort:   result.Network.APIPort,
		},
	); err != nil {
		return nil, err
	}

	s.log.Info("invite redeemed", "network", name, "route", result.Peer.Route)
	return result, nil
}

// Confirm brings up the main WireGuard interface, calls /confirm to
// prove reachability, and transitions the install into a permanent
// membership. On success the install row is consumed: the NetworkConfig
// is inserted and the Install row deleted in one transaction. Then a
// runtime Network is constructed adopting the live tunnel, registered,
// and an initial sync runs. Install ends with the network up and enabled.
//
// If the confirm call or store transaction fails, the tunnel comes down
// and the install row remains at "redeemed" — retryable.
func (s *Service) Confirm(
	name string,
) error {
	install, err := s.store.GetInstall(name)
	if err != nil {
		return err
	}

	if install.Phase != PhaseRedeemed {
		return fmt.Errorf("%w: install %q is in phase %q, expected redeemed",
			ErrInvalidInput, name, install.Phase)
	}

	tunnel, err := newTunnel(
		s.wireguard,
		install.MainIfaceName,
		install.MainPrivateKey,
		install.MainAssignedRoute,
		install.MainServer,
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

	nc := &NetworkConfig{
		Name:          name,
		PrivateKey:    install.MainPrivateKey,
		InterfaceName: install.MainIfaceName,
		AssignedRoute: install.MainAssignedRoute,
		Server:        install.MainServer,
		Enabled:       true,
		CreatedAt:     s.clock(),
	}
	if err := s.store.ConfirmInstall(name, nc); err != nil {
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

// UninstallNetwork removes a network and all its local state. If the
// network is currently enabled, it is disabled first. If a mid-install
// Install record exists but no network, the install row is deleted.
func (s *Service) UninstallNetwork(
	networkName string,
) error {
	_ = s.DisableNetwork(networkName)

	if err := s.store.DeleteNetwork(networkName); err == nil {
		return nil
	}

	return s.store.DeleteInstall(networkName)
}
