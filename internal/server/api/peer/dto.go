package peer

import (
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type VisiblePeerDTO struct {
	Name      string               `json:"name"`
	Route     string               `json:"route"`
	PublicKey string               `json:"public_key"`
	Endpoints []EndpointWitnessDTO `json:"endpoints"`
}

type EndpointWitnessDTO struct {
	Endpoint  string    `json:"endpoint"`
	Timestamp time.Time `json:"timestamp"`
}

type EndpointSightingDTO struct {
	WitnessKey string `json:"witness_key"`
	PeerKey    string `json:"peer_key"`
	Endpoint   string `json:"endpoint"`
}

func toVisiblePeerDTO(p *service.VisiblePeer) VisiblePeerDTO {
	seen := map[string]time.Time{}
	for _, e := range p.Endpoints {
		if prev, ok := seen[e.Endpoint]; !ok || e.Timestamp.After(prev) {
			seen[e.Endpoint] = e.Timestamp
		}
	}

	endpoints := make([]EndpointWitnessDTO, 0, len(seen))
	for ep, ts := range seen {
		endpoints = append(endpoints, EndpointWitnessDTO{
			Endpoint:  ep,
			Timestamp: ts,
		})
	}

	return VisiblePeerDTO{
		Name:      p.Name,
		Route:     p.Route,
		PublicKey: p.PublicKey,
		Endpoints: endpoints,
	}
}
