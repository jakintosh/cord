package runtime

import (
	"context"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// ListPeers returns every peer in this network, including the server
// peer itself.
func (r *Runtime) ListPeers(
	ctx context.Context,
	network string,
) (
	[]*service.Peer,
	error,
) {
	return executeValue(r, ctx, func() ([]*service.Peer, error) {
		return r.service.ListPeers(network)
	})
}

// UpdatePeer saves the domain change and applies local peer configuration.
// Application failures are recorded without discarding the saved result.
func (r *Runtime) UpdatePeer(
	ctx context.Context,
	network string,
	name string,
	diff service.PeerDiff,
) (
	*service.Peer,
	error,
) {
	return executeValue(r, ctx, func() (*service.Peer, error) {
		result, err := r.service.UpdatePeer(network, name, diff)
		if err != nil {
			return nil, err
		}

		r.applyPeers(network)
		return result, nil
	})
}

// ConfirmPeer saves the domain change and applies local peer configuration.
// Application failures are recorded without discarding the saved result.
func (r *Runtime) ConfirmPeer(
	ctx context.Context,
	network string,
	name string,
) error {
	return execute(r, ctx, func() error {
		if err := r.service.ConfirmPeer(network, name); err != nil {
			return err
		}

		r.applyPeers(network)
		return nil
	})
}

// RemovePeer saves the domain change and applies local peer configuration.
// Application failures are recorded without discarding the saved result.
func (r *Runtime) RemovePeer(
	ctx context.Context,
	network string,
	name string,
) error {
	return execute(r, ctx, func() error {
		if err := r.service.RemovePeer(network, name); err != nil {
			return err
		}

		r.applyPeers(network)
		return nil
	})
}

// ReportEndpoints records endpoint sightings submitted by a peer for
// the endpoint gossip system. Sightings are stored and made available
// to other peers via ListVisiblePeers.
func (r *Runtime) ReportEndpoints(
	ctx context.Context,
	network string,
	sightings []service.EndpointSighting,
) error {
	return execute(r, ctx, func() error {
		return r.service.ReportEndpoints(network, sightings)
	})
}

// peerHealthy reports whether a peer's last handshake is recent enough
// to consider it connected. Peers that have never handshaken are not
// healthy.
func peerHealthy(
	peerStatus wireguard.PeerStatus,
	now time.Time,
) bool {
	return !peerStatus.LastHandshake.IsZero() &&
		now.Sub(peerStatus.LastHandshake) < wireguard.ActiveHandshakeThreshold
}
