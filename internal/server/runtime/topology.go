package runtime

import (
	"context"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

// NetworkTopology is the network's full topology joined with live peer
// connectivity for display.
type NetworkTopology struct {
	View      topology.View
	Connected map[string]bool
}

// peerConnectivity returns configured peers joined with live WireGuard state. A
// network that is not running reports its peers as disconnected.
func (r *Runtime) peerConnectivity(
	name string,
) (
	map[string]bool,
	error,
) {
	record, err := r.service.GetNetwork(name)
	if err != nil {
		return nil, err
	}

	peers, err := r.service.ListPeers(name)
	if err != nil {
		return nil, err
	}

	running := r.running[name]

	live := make(map[string]wireguard.PeerStatus)
	if running != nil {
		statuses, err := running.main.device.GetPeerStatuses()
		if err != nil {
			return nil, err
		}

		for _, status := range statuses {
			live[status.PublicKey.String()] = status
		}
	}

	now := r.clock()
	result := make(map[string]bool, len(peers))
	for _, peer := range peers {
		connected := running != nil && peer.PublicKey == record.PublicKey
		if status, ok := live[peer.PublicKey]; ok {
			connected = peerHealthy(status, now)
		}

		result[peer.Name] = connected
	}

	return result, nil
}

// GetNetworkTopology returns the complete topology joined with live peer
// connectivity. A network that is not running reports its peers as
// disconnected.
func (r *Runtime) GetNetworkTopology(
	ctx context.Context,
	name string,
) (
	NetworkTopology,
	error,
) {
	return executeValue(r, ctx, func() (NetworkTopology, error) {
		view, err := r.service.GetNetworkTopology(name)
		if err != nil {
			return NetworkTopology{}, err
		}

		connected, err := r.peerConnectivity(name)
		if err != nil {
			return NetworkTopology{}, err
		}

		return NetworkTopology{
			View:      view,
			Connected: connected,
		}, nil
	})
}

// GetVisibleNetworkSnapshot returns one peer's visible peers, endpoint
// observations, and projected topology. The subject view comes from the
// network's compiled model; endpoint observations are loaded fresh.
func (r *Runtime) GetVisibleNetworkSnapshot(
	ctx context.Context,
	network string,
	peerName string,
) (
	*service.VisibleNetworkSnapshot,
	error,
) {
	return executeValue(r, ctx, func() (*service.VisibleNetworkSnapshot, error) {
		return r.service.GetVisibleNetworkSnapshot(network, peerName)
	})
}
