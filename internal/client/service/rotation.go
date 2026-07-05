package service

import "time"

// Endpoint rotation is the recovery path for a peer whose handshake
// has gone stale: cycle through the peer's known endpoints until one
// of them handshakes. Rotation is purely local — it only writes to
// the local device — so there is no backoff; a stale peer just keeps
// round-robining at a steady pace. Attempt state lives in the store
// (last_attempted_at on each endpoint row), so rotation survives
// daemon restarts.

// RotateInterval is the minimum time between endpoint rotation
// attempts for a single stale peer.
const RotateInterval = 90 * time.Second

// rotate points a stale peer at its least-recently-attempted candidate
// endpoint, no more than once per RotateInterval. Never-attempted
// candidates sort first in catalog (best-first) order, so fresh
// endpoints from the server are tried immediately and known ones are
// round-robined until a handshake returns.
func (n *Network) rotate(
	pubKey string,
	now time.Time,
) {
	endpoints, err := n.store.ListPeerEndpoints(n.cfg.Name, pubKey)
	if err != nil {
		n.logf("scan %s: list endpoints for %q: %v", n.cfg.Name, pubKey, err)
		return
	}

	candidate, ok := nextCandidate(endpoints, now)
	if !ok {
		return
	}

	if err := n.tunnel.device.SetPeerEndpoint(pubKey, candidate); err != nil {
		n.logf("scan %s: rotate %q to %q: %v", n.cfg.Name, pubKey, candidate, err)
		return
	}

	if err := n.store.MarkPeerEndpointAttempt(
		n.cfg.Name, pubKey, candidate, now.Unix(),
	); err != nil {
		n.logf("scan %s: mark attempt for %q: %v", n.cfg.Name, pubKey, err)
	}
}

// nextCandidate picks the least-recently-attempted endpoint, or
// reports false while the most recent attempt across the peer's
// endpoints is still within RotateInterval. Ties keep catalog order.
func nextCandidate(
	endpoints []PeerEndpoint,
	now time.Time,
) (
	string,
	bool,
) {
	if len(endpoints) == 0 {
		return "", false
	}

	oldest := endpoints[0]
	newest := int64(0)
	for _, ep := range endpoints {
		if ep.LastAttemptedAt < oldest.LastAttemptedAt {
			oldest = ep
		}
		if ep.LastAttemptedAt > newest {
			newest = ep.LastAttemptedAt
		}
	}

	if now.Unix()-newest < int64(RotateInterval/time.Second) {
		return "", false
	}
	return oldest.Endpoint, true
}
