package runtime

import (
	"context"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// PeerStatus is a cached peer joined with its live WireGuard device
// state. The device fields are zero-valued when the network isn't
// currently running.
type PeerStatus struct {
	Name          string
	Route         string
	Endpoint      string
	LastHandshake time.Time
	Connected     bool
}

// GetPeerStatus returns the network's cached peers joined with live device
// state. A network that is installed but not running reports its cached
// peers with zero-valued device fields rather than an error.
func (r *Runtime) GetPeerStatus(
	ctx context.Context,
	network string,
) (
	[]PeerStatus,
	error,
) {
	return executeValue(r, ctx, func() ([]PeerStatus, error) {
		peers, err := r.service.ListPeers(network)
		if err != nil {
			return nil, err
		}

		livePeerStatuses, err := r.getLivePeerStatuses(network)
		if err != nil {
			return nil, err
		}

		return joinPeerStatuses(peers, livePeerStatuses, r.clock()), nil
	})
}

// getLivePeerStatuses returns the running network's live WireGuard peers
// indexed by public key. An installed network that is not running has no
// live device state and returns an empty map without an error.
func (r *Runtime) getLivePeerStatuses(
	network string,
) (
	map[string]wireguard.PeerStatus,
	error,
) {
	running, isRunning := r.running[network]

	if !isRunning {
		return nil, nil
	}

	peerStatuses, err := running.getPeerStatuses()
	if err != nil {
		return nil, err
	}
	livePeerStatuses := make(map[string]wireguard.PeerStatus, len(peerStatuses))
	for _, peer := range peerStatuses {
		livePeerStatuses[peer.PublicKey.String()] = peer
	}
	return livePeerStatuses, nil
}

// joinPeerStatuses combines cached peer records with one live device
// snapshot.
func joinPeerStatuses(
	peers []*service.Peer,
	livePeerStatuses map[string]wireguard.PeerStatus,
	now time.Time,
) []PeerStatus {
	statuses := make([]PeerStatus, len(peers))
	for i, peer := range peers {
		status := PeerStatus{
			Name:  peer.Name,
			Route: peer.Route,
		}
		if devicePeerStatus, ok := livePeerStatuses[peer.PublicKey]; ok {
			if devicePeerStatus.Endpoint != nil {
				status.Endpoint = devicePeerStatus.Endpoint.String()
			}
			status.LastHandshake = devicePeerStatus.LastHandshake
			status.Connected = peerHealthy(devicePeerStatus, now)
		}
		statuses[i] = status
	}
	return statuses
}

// peerHealthy reports whether a peer's last handshake is recent enough
// to consider it connected. Peers that have never handshaken are not
// healthy.
func peerHealthy(
	peerStatus wireguard.PeerStatus,
	now time.Time,
) bool {
	return !peerStatus.LastHandshake.IsZero() &&
		now.Sub(peerStatus.LastHandshake) < StaleThreshold
}
