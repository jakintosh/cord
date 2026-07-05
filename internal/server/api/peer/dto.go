package peer

import (
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

// toVisiblePeer collapses a domain VisiblePeer's per-witness endpoint
// observations into the wire shape: one entry per endpoint, keeping the
// newest sighting timestamp.
func toVisiblePeer(
	p *service.VisiblePeer,
) protocol.VisiblePeer {
	seen := map[string]time.Time{}
	for _, e := range p.Endpoints {
		if prev, ok := seen[e.Endpoint]; !ok || e.Timestamp.After(prev) {
			seen[e.Endpoint] = e.Timestamp
		}
	}

	endpoints := make([]protocol.EndpointWitness, 0, len(seen))
	for ep, ts := range seen {
		endpoints = append(endpoints, protocol.EndpointWitness{
			Endpoint:  ep,
			Timestamp: ts,
		})
	}

	return protocol.VisiblePeer{
		Name:      p.Name,
		Route:     p.Route,
		PublicKey: p.PublicKey,
		Endpoints: endpoints,
	}
}
