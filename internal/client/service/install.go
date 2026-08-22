package service

import (
	"fmt"
	"time"

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
// WireGuard network (invite or main). Shared by Install and Network as
// persisted domain state.
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

	s.log.Info(
		"install started",
		"network",
		networkName,
	)

	return install, nil
}

// RedeemInstall validates the invitation a redemption returned and
// records the main-network identity it assigns, moving the install to
// phase "redeemed". The redemption itself is a network call the runtime
// makes; this is the durable half of it. Idempotent: repeating the same
// redemption returns the existing record unchanged.
func (s *Service) RedeemInstall(
	name string,
	invitation protocol.Invitation,
) (
	*Install,
	error,
) {
	// The assignment is the server's answer, not user input, so an
	// incomplete one is a protocol failure reported as invalid input.
	if err := invitation.Validate(false); err != nil {
		return nil, fmt.Errorf("%w: redeem result invalid: %v", ErrInvalidInput, err)
	}

	install, err := s.store.RedeemInstall(
		name,
		networkAssignmentFromInvitation(invitation),
	)
	if err != nil {
		return nil, err
	}

	s.log.Info(
		"invite redeemed",
		"network",
		name,
		"route",
		invitation.Peer.Route,
	)

	return install, nil
}

// ConfirmInstall consumes a redeemed install: the permanent membership
// record is created enabled and the transient install row is deleted in
// one transaction. The runtime brings the network up when it converges
// toward the newly recorded intent.
func (s *Service) ConfirmInstall(
	name string,
) (
	*Network,
	error,
) {
	install, err := s.store.GetInstall(name)
	if err != nil {
		return nil, err
	}

	switch install.Phase {
	case PhaseRedeemed:
	default:
		return nil, fmt.Errorf(
			"%w: install %q is in phase %q, expected redeemed",
			ErrInstallState,
			name,
			install.Phase,
		)
	}

	network, err := s.store.ConfirmInstall(name, install.MainPrivateKey, s.clock())
	if err != nil {
		return nil, err
	}

	s.requestReconcile(name)

	s.log.Info(
		"network confirmed",
		"network",
		name,
		"interface",
		install.MainIfaceName,
	)

	return network, nil
}

// UninstallNetwork removes all local state for a network, whether
// onboarding is complete or still in progress. The runtime takes the
// network's device down when it converges toward the absent record.
func (s *Service) UninstallNetwork(
	name string,
) error {
	if err := s.store.DeleteNetworkState(name); err != nil {
		return err
	}

	s.requestReconcile(name)

	s.log.Info(
		"network uninstalled",
		"network",
		name,
	)

	return nil
}
