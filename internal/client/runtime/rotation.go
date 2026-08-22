package runtime

import (
	"fmt"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

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
) error {
	endpoints, err := n.service.ListPeerEndpoints(n.record.Name, pubKey)
	if err != nil {
		return fmt.Errorf("rotate peer %q: list endpoints: %w", pubKey, err)
	}

	candidate, ok := nextCandidate(endpoints, now)
	if !ok {
		return nil
	}

	n.log.Debug(
		"rotating endpoint",
		"peer",
		pubKey,
		"endpoint",
		candidate,
	)

	if err := n.tunnel.device.SetPeerEndpoint(
		pubKey,
		candidate,
	); err != nil {
		return fmt.Errorf("rotate peer %q: set endpoint %q: %w", pubKey, candidate, err)
	}

	if err := n.service.RecordEndpointAttempt(
		n.record.Name,
		pubKey,
		candidate,
		now,
	); err != nil {
		return fmt.Errorf("rotate peer %q: record attempt: %w", pubKey, err)
	}

	return nil
}

// nextCandidate picks the least-recently-attempted endpoint, or
// reports false while the most recent attempt across the peer's
// endpoints is still within RotateInterval. Ties keep catalog order.
func nextCandidate(
	endpoints []service.PeerEndpoint,
	now time.Time,
) (
	string,
	bool,
) {
	if len(endpoints) == 0 {
		return "", false
	}

	oldest := endpoints[0]
	var newest time.Time
	for _, ep := range endpoints {
		if ep.LastAttemptedAt.Before(oldest.LastAttemptedAt) {
			oldest = ep
		}
		if ep.LastAttemptedAt.After(newest) {
			newest = ep.LastAttemptedAt
		}
	}

	if !newest.IsZero() && now.Sub(newest) < RotateInterval {
		return "", false
	}

	return oldest.Endpoint, true
}
