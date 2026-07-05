package admin

import (
	"context"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

type NetworkStatusDTO struct {
	Name                     string `json:"name"`
	Enabled                  bool   `json:"enabled"`
	PeerCount                int    `json:"peer_count"`
	PendingRegistrationCount int    `json:"pending_registration_count"`
}

type StatusDTO struct {
	Status   string             `json:"status"`
	Version  string             `json:"version"`
	Networks []NetworkStatusDTO `json:"networks"`
}

// listNetworkStatusDTOs composes per-network status from existing
// service calls: peer and pending-registration counts alongside the
// network's enabled flag.
func (a *API) listNetworkStatusDTOs() (
	[]NetworkStatusDTO,
	error,
) {
	names, err := a.service.ListNetworks()
	if err != nil {
		return nil, err
	}

	dtos := make([]NetworkStatusDTO, 0, len(names))
	for _, name := range names {
		network, err := a.service.GetNetwork(name)
		if err != nil {
			continue
		}

		peers, err := a.service.ListPeers(name)
		if err != nil {
			continue
		}

		regs, err := a.service.ListRegistrations(name)
		if err != nil {
			continue
		}
		pending := 0
		for _, reg := range regs {
			if !reg.Redeemed {
				pending++
			}
		}

		dtos = append(dtos, NetworkStatusDTO{
			Name:                     network.Name,
			Enabled:                  network.Enabled,
			PeerCount:                len(peers),
			PendingRegistrationCount: pending,
		})
	}

	return dtos, nil
}

func (a *API) handleStatus(
	w http.ResponseWriter,
	r *http.Request,
) {
	dtos, err := a.listNetworkStatusDTOs()
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, StatusDTO{
		Status:   "ok",
		Version:  a.version,
		Networks: dtos,
	})
}

func (c *Client) Status(
	ctx context.Context,
) (
	StatusDTO,
	error,
) {
	resp, err := c.t.Get(ctx, "/status")
	if err != nil {
		return StatusDTO{}, err
	}
	return daemon.DecodeResponse[StatusDTO](resp)
}
