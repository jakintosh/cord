package service

import (
	"errors"
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// Peer is the canonical server-side record of a network participant.
// It includes the identity, assigned address, and management flags.
type Peer struct {
	Name      string
	Route     string // terminal host route, e.g. "10.42.0.5/32" or "fd00::5/128"
	PublicKey string
	Admin     bool // whether the peer has administrative privileges
	Enabled   bool // whether the peer's WireGuard config is applied
	Confirmed bool // whether the peer has proven reachability on the main network
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

	since := s.clock().Add(-defaultEndpointTTL)
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
			Route:     p.Route,
			PublicKey: p.PublicKey,
			Endpoints: endpoints,
		})
	}

	return visible, nil
}

// UpdatePeer applies a partial update to a peer and returns the
// updated record. Nil pointer fields mean no change.
func (s *Service) UpdatePeer(
	network string,
	name string,
	newName *string,
	admin *bool,
	enabled *bool,
	confirmed *bool,
) (
	*Peer,
	error,
) {
	_, err := s.store.GetPeer(network, name)
	if err != nil {
		return nil, fmt.Errorf("get peer for update: %w", mapStoreError(err))
	}

	p, err := s.store.UpdatePeer(network, name, newName, admin, enabled, confirmed)
	if err != nil {
		return nil, fmt.Errorf("update peer %q: %w", name, mapStoreError(err))
	}
	s.reconcile(network)
	return p, nil
}

// EnablePeer allows a peer to connect. Its WireGuard configuration is
// included at the next reconciliation.
func (s *Service) EnablePeer(
	network string,
	name string,
) error {
	enabled := true
	_, err := s.store.UpdatePeer(network, name, nil, nil, &enabled, nil)
	if err != nil {
		return fmt.Errorf("enable peer %q: %w", name, mapStoreError(err))
	}
	s.reconcile(network)
	return nil
}

// DisablePeer prevents a peer from connecting. Its WireGuard
// configuration is removed at the next reconciliation.
func (s *Service) DisablePeer(
	network string,
	name string,
) error {
	disabled := false
	_, err := s.store.UpdatePeer(network, name, nil, nil, &disabled, nil)
	if err != nil {
		return fmt.Errorf("disable peer %q: %w", name, mapStoreError(err))
	}
	s.reconcile(network)
	return nil
}

// ConfirmPeer marks a peer as confirmed — it has proven WireGuard
// reachability on the main network from its assigned IP. Only the
// confirmed flag is flipped; enabled is left untouched, so an admin
// who disabled the peer before confirm gets a peer that is
// confirmed but not live until re-enabled.
//
// The corresponding registration is marked confirmed, which removes
// the temp peer from the invite device and releases the invite IPs.
// If the registration is already gone (revoked or already confirmed)
// the registration update is treated as a no-op rather than an error,
// so confirm remains idempotent.
func (s *Service) ConfirmPeer(
	network string,
	name string,
) error {
	confirmed := true
	_, err := s.store.UpdatePeer(network, name, nil, nil, nil, &confirmed)
	if err != nil {
		return fmt.Errorf("confirm peer %q: %w", name, mapStoreError(err))
	}

	if err := s.store.ConfirmRegistration(network, name); err != nil {
		if !errors.Is(err, ErrNotFound) {
			return fmt.Errorf("confirm registration %q: %w", name, mapStoreError(err))
		}
	}
	s.reconcile(network)

	return nil
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
	s.reconcile(network)
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

func peersToWireGuardPeers(
	peers []*Peer,
) (
	[]wireguard.PeerConfig,
	error,
) {
	var wgpeers []wireguard.PeerConfig
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
