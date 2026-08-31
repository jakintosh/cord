package api

import (
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/runtime"
)

func (a *API) handleListPeers(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	statuses, err := a.runtime.GetPeerStatus(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, peersFromRuntime(statuses))
}

func peerFromRuntime(
	p runtime.PeerStatus,
) Peer {
	dto := Peer{
		Name:      p.Name,
		Route:     p.Route,
		Endpoint:  p.Endpoint,
		Connected: p.Connected,
	}
	if !p.LastHandshake.IsZero() {
		formatted := p.LastHandshake.Format(time.RFC3339)
		dto.LastHandshake = &formatted
	}
	return dto
}

func peersFromRuntime(
	statuses []runtime.PeerStatus,
) []Peer {
	dtos := make([]Peer, len(statuses))
	for i, status := range statuses {
		dtos[i] = peerFromRuntime(status)
	}
	return dtos
}
