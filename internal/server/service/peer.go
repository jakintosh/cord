package service

import (
	"fmt"
	"net"
	"time"
)

// Peer is the canonical server-side record of a network participant.
// It includes the identity, assigned address, and management flags.
type Peer struct {
	Name      string
	Cidr      string // e.g. "10.42.0.5/16" — assigned IP and network mask
	PublicKey string
	Admin     bool // whether the peer has administrative privileges
	Enabled   bool // whether the peer's WireGuard config is applied
	Confirmed bool // whether the peer has proven reachability on the main network
}

// PeerConfig is the input for creating a peer invite. If IP is empty
// an address is auto-assigned from the root CIDR.
type PeerConfig struct {
	Name  string
	IP    string // optional; auto-assigned if empty
	Admin bool
}

// UpdatePeerRequest carries the fields that can be changed on a peer.
// Nil pointer means "no change."
type UpdatePeerRequest struct {
	Name      *string
	Admin     *bool
	Enabled   *bool
	Confirmed *bool
}

// VisiblePeer is a peer as it appears to other peers on the network.
// It includes identity plus recently witnessed endpoints but omits
// management flags (Admin, Enabled, Confirmed).
type VisiblePeer struct {
	Name      string
	Cidr      string
	PublicKey string
	Endpoints []EndpointWitness
}

// EndpointSighting is one peer observing another peer's public endpoint
// at a point in time. Reported during endpoint gossip.
type EndpointSighting struct {
	WitnessKey string
	PeerKey    string
	Endpoint   string
	Timestamp  time.Time
}

// EndpointWitness records who observed an endpoint and when.
type EndpointWitness struct {
	Witness   string
	Endpoint  string
	Timestamp time.Time
}

// ResolvePeerIdentity looks up a confirmed peer by IP address within
// the network. Used by the identity middleware to authenticate incoming
// peer requests by their WireGuard source IP.
func (s *Service) ResolvePeerIdentity(network string, ip net.IP) (*Peer, error) {
	p, err := s.store.GetPeerByIP(network, ip)
	if err != nil {
		return nil, fmt.Errorf("resolve peer identity: %w", mapStoreError(err))
	}
	return p, nil
}

// GetPeer returns a peer by name within the given network.
func (s *Service) GetPeer(
	network string,
	name string,
) (
	*Peer,
	error,
) {
	p, err := s.store.GetPeer(network, name)
	if err != nil {
		return nil, fmt.Errorf("get peer %q: %w", name, mapStoreError(err))
	}
	return p, nil
}

// ListPeers returns every peer in this network, including the server
// peer itself.
func (s *Service) ListPeers(
	network string,
) (
	[]*Peer,
	error,
) {
	peers, err := s.store.ListPeers(network)
	if err != nil {
		return nil, fmt.Errorf("list peers: %w", err)
	}
	return peers, nil
}

// ListVisiblePeers returns the peers visible to the named peer, each
// with recently witnessed endpoints. Used for endpoint gossip — peers
// discover each other's endpoints through this list.
func (s *Service) ListVisiblePeers(
	network string,
	peerName string,
) (
	[]*VisiblePeer,
	error,
) {
	peers, err := s.store.ListPeers(network)
	if err != nil {
		return nil, fmt.Errorf("list peers for visibility: %w", err)
	}

	since := s.clock().Add(-endpointTTL)
	recentEndpoints, err := s.store.GetRecentEndpoints(network, since)
	if err != nil {
		return nil, fmt.Errorf("get recent endpoints: %w", err)
	}

	var visible []*VisiblePeer
	for _, p := range peers {
		if p.Name == peerName {
			continue
		}

		witnesses := recentEndpoints[p.PublicKey]
		endpoints := make([]EndpointWitness, len(witnesses))
		for i, w := range witnesses {
			endpoints[i] = EndpointWitness{
				Witness:   w.Witness,
				Endpoint:  w.Endpoint,
				Timestamp: w.Timestamp,
			}
		}

		visible = append(visible, &VisiblePeer{
			Name:      p.Name,
			Cidr:      p.Cidr,
			PublicKey: p.PublicKey,
			Endpoints: endpoints,
		})
	}

	return visible, nil
}

// AddPeer creates an invite for a new participant. If cfg.IP is empty
// an address is auto-assigned from the root CIDR. Returns the
// PeerInvite payload to deliver out-of-band to the invitee.
func (s *Service) AddPeer(
	network string,
	cfg PeerConfig,
) (
	*PeerInvite,
	error,
) {
	if cfg.Name == "" {
		return nil, fmt.Errorf("%w: peer name required", ErrInvalidInput)
	}

	exists, err := s.store.PeerExists(network, cfg.Name)
	if err != nil {
		return nil, fmt.Errorf("check peer exists: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("%w: peer %q already exists", ErrConflict, cfg.Name)
	}

	nw, err := s.store.GetNetwork(network)
	if err != nil {
		return nil, fmt.Errorf("get network: %w", mapStoreError(err))
	}

	var ip string
	if cfg.IP != "" {
		parsedIP := net.ParseIP(cfg.IP)
		if parsedIP == nil {
			return nil, fmt.Errorf("%w: invalid IP %q", ErrInvalidInput, cfg.IP)
		}
		_, rootNet, _ := net.ParseCIDR(nw.MainCidr)
		if !rootNet.Contains(parsedIP) {
			return nil, fmt.Errorf(
				"%w: IP %q is not within main CIDR %q",
				ErrInvalidInput, cfg.IP, nw.MainCidr,
			)
		}
		ip = fmt.Sprintf("%s/%d", parsedIP.String(), terminalPrefix(parsedIP))
	} else {
		ip, err = s.nextFreePeerIP(network, nw.MainCidr)
		if err != nil {
			return nil, fmt.Errorf("auto-assign IP: %w", err)
		}
	}

	return s.CreateInvite(network, CreateInviteRequest{
		Name:  cfg.Name,
		IP:    ip,
		Admin: cfg.Admin,
	})
}

// RemovePeer deletes a peer and its associated invite (if any). The
// peer's WireGuard configuration will be removed at the next
// reconciliation.
func (s *Service) RemovePeer(
	network string,
	name string,
) error {
	if err := s.store.DeletePeer(network, name); err != nil {
		return fmt.Errorf("delete peer %q: %w", name, mapStoreError(err))
	}
	return nil
}

// UpdatePeer applies a partial update to a peer and returns the
// updated record. Nil fields in the request mean no change.
func (s *Service) UpdatePeer(
	network string,
	name string,
	req UpdatePeerRequest,
) (
	*Peer,
	error,
) {
	_, err := s.store.GetPeer(network, name)
	if err != nil {
		return nil, fmt.Errorf("get peer for update: %w", mapStoreError(err))
	}

	p, err := s.store.UpdatePeer(network, name, req)
	if err != nil {
		return nil, fmt.Errorf("update peer %q: %w", name, mapStoreError(err))
	}
	return p, nil
}

// EnablePeer allows a peer to connect. Its WireGuard configuration is
// included at the next reconciliation.
func (s *Service) EnablePeer(
	network string,
	name string,
) error {
	enabled := true
	_, err := s.store.UpdatePeer(network, name, UpdatePeerRequest{
		Enabled: &enabled,
	})
	if err != nil {
		return fmt.Errorf("enable peer %q: %w", name, mapStoreError(err))
	}
	return nil
}

// DisablePeer prevents a peer from connecting. Its WireGuard
// configuration is removed at the next reconciliation.
func (s *Service) DisablePeer(
	network string,
	name string,
) error {
	disabled := false
	_, err := s.store.UpdatePeer(network, name, UpdatePeerRequest{
		Enabled: &disabled,
	})
	if err != nil {
		return fmt.Errorf("disable peer %q: %w", name, mapStoreError(err))
	}
	return nil
}

// ConfirmPeer marks a peer as confirmed — it has proven WireGuard
// reachability on the main network from its assigned IP. The
// temporary invite record is removed on first confirmation.
func (s *Service) ConfirmPeer(
	network string,
	name string,
) error {
	confirmed := true
	_, err := s.store.UpdatePeer(network, name, UpdatePeerRequest{
		Enabled:   &confirmed,
		Confirmed: &confirmed,
	})
	if err != nil {
		return fmt.Errorf("confirm peer %q: %w", name, mapStoreError(err))
	}

	_ = s.store.DeleteInvite(network, name)

	return nil
}

// ReportEndpoints records endpoint sightings submitted by a peer for
// the endpoint gossip system. Sightings are stored and made available
// to other peers via ListVisiblePeers.
func (s *Service) ReportEndpoints(
	network string,
	sightings []EndpointSighting,
) error {
	_, err := s.store.GetNetwork(network)
	if err != nil {
		return fmt.Errorf("get network: %w", mapStoreError(err))
	}

	if err := s.store.InsertEndpointSightings(network, sightings); err != nil {
		return fmt.Errorf("insert endpoint sightings: %w", err)
	}
	return nil
}

// nextFreePeerIP finds the lowest free address in the root CIDR,
// skipping the network address, the server address, and addresses
// held by existing peers or active invites.
func (s *Service) nextFreePeerIP(
	network string,
	rootCidr string,
) (
	string,
	error,
) {
	_, ipNet, err := net.ParseCIDR(rootCidr)
	if err != nil {
		return "", fmt.Errorf("parse root CIDR: %w", err)
	}

	peers, err := s.store.ListPeers(network)
	if err != nil {
		return "", fmt.Errorf("list peers: %w", err)
	}

	invites, err := s.store.ListActiveInvites(network, s.clock())
	if err != nil {
		return "", fmt.Errorf("list active invites: %w", err)
	}

	used := map[string]bool{}
	for _, p := range peers {
		ip, _, _ := net.ParseCIDR(p.Cidr)
		if ip != nil {
			used[normalizeIP(ip).String()] = true
		}
	}
	for _, inv := range invites {
		if inv.FinalIP != nil {
			used[normalizeIP(inv.FinalIP).String()] = true
		}
	}

	first := firstAssignableIP(ipNet)
	_, last := cidrRange(ipNet)

	candidate := incrementIP(first)
	for ipNet.Contains(candidate) && !candidate.Equal(last) {
		if !used[normalizeIP(candidate).String()] {
			return fmt.Sprintf("%s/%d", candidate.String(), terminalPrefix(candidate)), nil
		}
		candidate = incrementIP(candidate)
	}

	return "", fmt.Errorf("%w: no free addresses in %s", ErrInvalidInput, rootCidr)
}
