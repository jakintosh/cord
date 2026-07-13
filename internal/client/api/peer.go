package api

import (
	"context"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

// Peer is a cached peer joined with live WireGuard device state, for
// display by the CLI.
type Peer struct {
	Name          string  `json:"name"`
	Route         string  `json:"route"`
	Endpoint      string  `json:"endpoint,omitempty"`
	LastHandshake *string `json:"last_handshake,omitempty"`
	Connected     bool    `json:"connected"`
}

func peerFromStatus(
	p service.PeerStatus,
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

func peersFromStatuses(
	statuses []service.PeerStatus,
) []Peer {
	dtos := make([]Peer, len(statuses))
	for i, s := range statuses {
		dtos[i] = peerFromStatus(s)
	}
	return dtos
}

func (a *API) handleListPeers(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	statuses, err := a.service.ListPeerStatus(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, peersFromStatuses(statuses))
}

func (c *Client) ListPeers(
	ctx context.Context,
	network string,
) (
	[]Peer,
	error,
) {
	var result []Peer
	return result, c.wire.Get(ctx, "/networks/"+network+"/peers", &result)
}
