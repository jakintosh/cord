package api

import (
	"context"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

// PeerDTO is a cached peer joined with live WireGuard device state, for
// display by the CLI.
type PeerDTO struct {
	Name          string  `json:"name"`
	Route         string  `json:"route"`
	Endpoint      string  `json:"endpoint,omitempty"`
	LastHandshake *string `json:"last_handshake,omitempty"`
	Connected     bool    `json:"connected"`
}

func peerDTOFromStatus(
	p service.PeerStatus,
) PeerDTO {
	dto := PeerDTO{
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

func peerDTOsFromStatuses(
	statuses []service.PeerStatus,
) []PeerDTO {
	dtos := make([]PeerDTO, len(statuses))
	for i, s := range statuses {
		dtos[i] = peerDTOFromStatus(s)
	}
	return dtos
}

func (a *API) handlePeerList(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	statuses, err := a.service.ListPeerStatus(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, peerDTOsFromStatuses(statuses))
}

func (c *Client) ListPeers(
	ctx context.Context,
	network string,
) (
	[]PeerDTO,
	error,
) {
	resp, err := c.t.Get(ctx, "/networks/"+network+"/peers")
	if err != nil {
		return nil, err
	}
	return daemon.DecodeResponse[[]PeerDTO](resp)
}
