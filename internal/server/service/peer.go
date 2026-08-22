package service

import (
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Peer is the canonical server-side record of a network participant.
// It includes the identity, assigned address, and management flags.
type Peer struct {
	Name      string
	CidrName  string // the name of the terminal CIDR the peer is assigned to
	Route     string // terminal host route, e.g. "10.42.0.5/32" or "fd00::5/128", derived from CIDR
	PublicKey string
	Admin     bool // whether the peer has administrative privileges
	Enabled   bool // whether the peer's WireGuard config is applied
	Confirmed bool // whether the peer has proven reachability on the main network
}

// PeerDiff describes a partial change to a persisted peer. Nil fields mean
// no change; lifecycle methods construct updates for their own state changes.
type PeerDiff struct {
	Name    *string
	Admin   *bool
	Enabled *bool
}

// VisiblePeer is a peer as it appears to other peers on the network.
// It includes identity plus recently witnessed endpoints but omits
// management flags (Admin, Enabled, Confirmed).
type VisiblePeer struct {
	Name      string
	Route     string
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

// ListMainPeers returns the WireGuard peer set the network's main plane
// should carry: every enabled peer, each on its own terminal host route.
func (s *Service) ListMainPeers(
	network string,
) (
	[]wireguard.PeerConfig,
	error,
) {
	peers, err := s.store.ListPeers(network)
	if err != nil {
		return nil, fmt.Errorf("main peers: %w", mapStoreError(err))
	}

	wgpeers, err := peersToWireGuardPeers(peers)
	if err != nil {
		return nil, fmt.Errorf("main peers: %w", err)
	}
	return wgpeers, nil
}

// ListVisiblePeers returns the peers visible to the named peer, each
// with recently witnessed endpoints. Visibility is determined by the
// group-based topology model: effective groups (direct + inherited)
// combined with group associations. Used for endpoint gossip — peers
// discover each other's endpoints through this list.
func (s *Service) ListVisiblePeers(
	network string,
	peerName string,
) (
	[]*VisiblePeer,
	error,
) {
	snapshot, err := s.GetVisibleNetworkSnapshot(network, peerName)
	if err != nil {
		return nil, err
	}
	return snapshot.Peers, nil
}

// UpdatePeer applies a partial update to a peer and returns the
// updated record. Nil pointer fields mean no change.
func (s *Service) UpdatePeer(
	network string,
	name string,
	diff PeerDiff,
) (
	*Peer,
	error,
) {
	if diff.Name == nil &&
		diff.Admin == nil &&
		diff.Enabled == nil {
		return nil, ErrInvalidInput
	}

	p, err := s.store.UpdatePeer(network, name, diff)
	if err != nil {
		return nil, fmt.Errorf("update peer %q: %w", name, mapStoreError(err))
	}
	s.requestReconcile(network)

	return p, nil
}

// ConfirmPeer marks a peer as confirmed — it has proven WireGuard
// reachability on the main network from its assigned IP. Only the
// confirmed flag is flipped; enabled is left untouched, so an admin
// who disabled the peer before confirm gets a peer that is
// confirmed but not live until re-enabled.
//
// The corresponding registration is marked confirmed in the same
// transaction, and its group assignments are transferred to the peer's
// terminal CIDR. Confirmation is idempotent.
func (s *Service) ConfirmPeer(
	network string,
	name string,
) error {
	defer s.requestReconcile(network)

	if err := s.store.ConfirmPeer(
		network,
		name,
		s.clock(),
	); err != nil {
		return fmt.Errorf("confirm peer %q: %w", name, mapStoreError(err))
	}

	s.log.Info(
		"peer confirmed",
		"network",
		network,
		"peer",
		name,
	)

	return nil
}

// RemovePeer deletes a peer and its associated invite (if any). The
// peer's WireGuard configuration will be removed at the next
// reconciliation.
func (s *Service) RemovePeer(
	network string,
	name string,
) error {
	if err := s.store.DeletePeer(
		network,
		name,
	); err != nil {
		return fmt.Errorf("delete peer %q: %w", name, mapStoreError(err))
	}
	s.requestReconcile(network)

	return nil
}

// ReportEndpoints records endpoint sightings submitted by a peer for
// the endpoint gossip system. Sightings are stored and made available
// to other peers via ListVisiblePeers.
func (s *Service) ReportEndpoints(
	network string,
	sightings []EndpointSighting,
) error {
	if err := s.store.InsertEndpointSightings(
		network,
		sightings,
	); err != nil {
		return fmt.Errorf("insert endpoint sightings: %w", mapStoreError(err))
	}

	return nil
}

// ObserveEndpoints records endpoint sightings the server's own WireGuard
// device made of the peers connected to it. These are the first endpoint
// candidates a peer has, before it can hear about others from gossip.
// Sightings of anything that is not a confirmed, enabled peer of the
// network — the witness itself included — are dropped.
func (s *Service) ObserveEndpoints(
	network string,
	sightings []EndpointSighting,
) error {
	peers, err := s.store.ListPeers(network)
	if err != nil {
		return fmt.Errorf("observe endpoints: %w", mapStoreError(err))
	}

	known := make(map[string]struct{}, len(peers))
	for _, peer := range peers {
		if !peer.Enabled || !peer.Confirmed {
			continue
		}
		known[peer.PublicKey] = struct{}{}
	}

	observed := make([]EndpointSighting, 0, len(sightings))
	for _, sighting := range sightings {
		if sighting.PeerKey == sighting.WitnessKey {
			continue
		}
		if _, ok := known[sighting.PeerKey]; !ok {
			continue
		}
		observed = append(observed, sighting)
	}
	if len(observed) == 0 {
		return nil
	}

	return s.ReportEndpoints(network, observed)
}

func peersToWireGuardPeers(
	peers []*Peer,
) (
	[]wireguard.PeerConfig,
	error,
) {
	wgpeers := make([]wireguard.PeerConfig, 0, len(peers))
	for _, peer := range peers {
		if !peer.Enabled {
			continue
		}

		peerRoute, err := netaddr.ParseRoute(peer.Route)
		if err != nil {
			return nil, fmt.Errorf("parse peer route %q: %w", peer.Route, err)
		}

		wgpeers = append(wgpeers, wireguard.PeerConfig{
			PublicKey:      peer.PublicKey,
			AllowedIPs:     []string{peerRoute.String()},
			EndpointPolicy: wireguard.EndpointDynamic,
		})
	}
	return wgpeers, nil
}
