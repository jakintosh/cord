package service

import (
	"fmt"
	"net"
	"time"
)

// Network is the persistent identity of a server network. It holds the
// server's keypair, address space configuration, and network endpoints.
// It is inert domain data — the Service owns all behavior.
type Network struct {
	Name             string
	PrivateKey       string
	PublicKey        string
	RootCidr         string    // e.g. "10.42.0.0/16" — the network's address space
	InviteCidr       string    // e.g. "10.43.0.0/24" — the invite subnet
	ExternalIP       string    // the server's public IP address
	ListenPort       uint16    // WireGuard listen port for the main interface
	InviteListenPort uint16    // WireGuard listen port for the invite interface
	ApiPort          uint16    // the internal API port served over the tunnel
	CreatedAt        time.Time // when the network was created
}

// GetNetwork returns the persisted network record by name.
// Returns ErrNotFound if the network does not exist.
func (s *Service) GetNetwork(
	name string,
) (
	*Network,
	error,
) {
	nw, err := s.store.GetNetwork(name)
	if err != nil {
		return nil, fmt.Errorf("get network %q: %w", name, mapStoreError(err))
	}
	return nw, nil
}

// ListNetworks returns the names of all persisted server networks,
// ordered alphabetically.
func (s *Service) ListNetworks() (
	[]string,
	error,
) {
	names, err := s.store.ListNetworkNames()
	if err != nil {
		return nil, fmt.Errorf("list networks: %w", err)
	}
	return names, nil
}

// CreateNetwork defines a new server network: validates the config,
// generates the server keypair, and persists the record. It does not
// bring interfaces up — call StartNetwork for that.
//
// On success the returned Network includes the generated keys. Returns
// ErrNetworkExists if the name is already taken, ErrInvalidInput if
// the configuration is invalid.
func (s *Service) CreateNetwork(
	cfg Network,
) (
	*Network,
	error,
) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("%w: network name required", ErrInvalidInput)
	}

	_, rootNet, err := net.ParseCIDR(cfg.RootCidr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid root CIDR %q: %v", ErrInvalidInput, cfg.RootCidr, err)
	}

	if cfg.ExternalIP == "" {
		return nil, fmt.Errorf("%w: external IP required", ErrInvalidInput)
	}

	if cfg.ListenPort == 0 {
		return nil, fmt.Errorf("%w: listen port required", ErrInvalidInput)
	}

	if cfg.InviteCidr == "" {
		cfg.InviteCidr = defaultInviteCidr
	}

	_, inviteNet, err := net.ParseCIDR(cfg.InviteCidr)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid invite CIDR %q: %v", ErrInvalidInput, cfg.InviteCidr, err)
	}

	if cfg.InviteListenPort == 0 {
		cfg.InviteListenPort = cfg.ListenPort + 1
	}

	if cfg.ApiPort == 0 {
		cfg.ApiPort = cfg.ListenPort + 2
	}

	if rootNet.Contains(inviteNet.IP) || inviteNet.Contains(rootNet.IP) {
		return nil, fmt.Errorf(
			"%w: invite CIDR %q overlaps root CIDR %q",
			ErrCIDROverlap, cfg.InviteCidr, cfg.RootCidr,
		)
	}

	privKey, err := s.wg.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generate key: %w", err)
	}

	pubKey, err := s.wg.PublicKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("derive public key: %w", err)
	}

	ones, bits := rootNet.Mask.Size()
	rootCidr := &Cidr{
		Name:   cfg.Name,
		Cidr:   cfg.RootCidr,
		Length: ones,
		Prefix: bits,
	}

	serverIP := firstAssignableIP(rootNet)
	serverPeer := &Peer{
		Name:      "cord-server",
		Cidr:      fmt.Sprintf("%s/%d", serverIP.String(), terminalPrefix(serverIP)),
		PublicKey: pubKey,
		Admin:     true,
		Enabled:   true,
		Confirmed: true,
	}

	nw := &Network{
		Name:             cfg.Name,
		PrivateKey:       privKey,
		PublicKey:        pubKey,
		RootCidr:         cfg.RootCidr,
		InviteCidr:       cfg.InviteCidr,
		ExternalIP:       cfg.ExternalIP,
		ListenPort:       cfg.ListenPort,
		InviteListenPort: cfg.InviteListenPort,
		ApiPort:          cfg.ApiPort,
		CreatedAt:        s.clock(),
	}

	if err := s.store.BootstrapNetwork(nw, rootCidr, serverPeer); err != nil {
		return nil, fmt.Errorf("bootstrap network: %w", mapStoreError(err))
	}

	return nw, nil
}

// DeleteNetwork removes the named network and all of its resources
// (peers, CIDRs, associations, invites, endpoints). The network must
// be stopped before deletion — returns ErrNetworkRunning otherwise.
func (s *Service) DeleteNetwork(
	name string,
) error {
	s.mu.Lock()
	_, running := s.running[name]
	s.mu.Unlock()

	if running {
		return fmt.Errorf("%w: %s", ErrNetworkRunning, name)
	}

	if err := s.store.DeleteNetwork(name); err != nil {
		return fmt.Errorf("delete network %q: %w", name, mapStoreError(err))
	}
	return nil
}

// defaultInviteCidr is the invite subnet used when CreateNetwork is
// called without an explicit InviteCidr. It lives in the 172.16/12
// private range, which is unlikely to overlap with typical root CIDRs
// in the 10/8 range.
const defaultInviteCidr = "172.16.10.0/24"
