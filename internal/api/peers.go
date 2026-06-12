package api

import (
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

// PublicPeerDTO is a peer as seen by other peers: identity plus
// recently witnessed endpoints, newest first.
type PublicPeerDTO struct {
	Name      string               `json:"name"`
	Cidr      string               `json:"cidr"`
	PublicKey string               `json:"publicKey"`
	Endpoints []EndpointWitnessDTO `json:"endpoints"`
}

// EndpointWitnessDTO is one witnessed endpoint sighting of a peer.
type EndpointWitnessDTO struct {
	WitnessKey string `json:"witnessKey"`
	Endpoint   string `json:"endpoint"`
	Timestamp  int64  `json:"timestamp"`
}

// EndpointSightingDTO is a peer's report of an endpoint it observed.
// The witness is always the authenticated caller.
type EndpointSightingDTO struct {
	PeerKey   string `json:"peerKey"`
	Endpoint  string `json:"endpoint"`
	Timestamp int64  `json:"timestamp"`
}

func PublicPeerDTOFromServer(
	p server.PublicPeer,
) PublicPeerDTO {
	endpoints := make([]EndpointWitnessDTO, 0, len(p.Endpoints))
	for _, witness := range p.Endpoints {
		endpoints = append(endpoints, EndpointWitnessDTO(witness))
	}
	return PublicPeerDTO{
		Name:      p.Name,
		Cidr:      p.Cidr,
		PublicKey: p.PublicKey,
		Endpoints: endpoints,
	}
}

func (p PublicPeerDTO) ToServer() server.PublicPeer {
	endpoints := make([]server.EndpointWitness, 0, len(p.Endpoints))
	for _, witness := range p.Endpoints {
		endpoints = append(endpoints, server.EndpointWitness(witness))
	}
	return server.PublicPeer{
		Name:      p.Name,
		Cidr:      p.Cidr,
		PublicKey: p.PublicKey,
		Endpoints: endpoints,
	}
}

func (s EndpointSightingDTO) ToServer() server.EndpointSighting {
	return server.EndpointSighting{
		PeerKey:   s.PeerKey,
		Endpoint:  s.Endpoint,
		Timestamp: s.Timestamp,
	}
}

// handleListPeers returns the peers visible to the calling peer.
func (a *API) handleListPeers(
	w http.ResponseWriter,
	r *http.Request,
) {
	name, ok := peerName(r)
	if !ok {
		wire.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	peers, err := a.service.GetVisiblePeers(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	dtos := make([]PublicPeerDTO, 0, len(peers))
	for _, peer := range peers {
		dtos = append(dtos, PublicPeerDTOFromServer(*peer))
	}

	wire.WriteData(w, http.StatusOK, dtos)
}

// handleReportEndpoints records endpoint sightings witnessed by the
// calling peer. The witness identity comes from authentication, never
// from the request body.
func (a *API) handleReportEndpoints(
	w http.ResponseWriter,
	r *http.Request,
) {
	name, ok := peerName(r)
	if !ok {
		wire.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var reports []EndpointSightingDTO
	if err := decodeJSON(r, &reports); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "malformed json")
		return
	}

	witness, err := a.service.GetPeer(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	sightings := make([]server.EndpointSighting, 0, len(reports))
	for _, report := range reports {
		sighting := report.ToServer()
		sighting.WitnessKey = witness.PublicKey
		sightings = append(sightings, sighting)
	}

	if err := a.service.ReportEndpoints(sightings); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}
