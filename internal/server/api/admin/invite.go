package admin

import (
	"context"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type InviteDTO struct {
	Name      string `json:"name"`
	Redeemed  bool   `json:"redeemed"`
	ExpiresAt string `json:"expires_at"`
}

func InviteDTOFromService(
	inv service.Invite,
) InviteDTO {
	return InviteDTO{
		Name:      inv.Name,
		Redeemed:  inv.Redeemed,
		ExpiresAt: inv.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func InviteDTOsFromService(
	invites []*service.Invite,
) []InviteDTO {
	if invites == nil {
		return []InviteDTO{}
	}
	result := make([]InviteDTO, len(invites))
	for i, inv := range invites {
		result[i] = InviteDTOFromService(*inv)
	}
	return result
}

func (a *API) handleInviteList(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	invites, err := a.service.ListInvites(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, InviteDTOsFromService(invites))
}

func (c *Client) ListInvites(
	ctx context.Context,
	network string,
) (
	[]InviteDTO,
	error,
) {
	resp, err := c.t.Get(ctx, "/networks/"+network+"/invites")
	if err != nil {
		return nil, err
	}
	return daemon.DecodeResponse[[]InviteDTO](resp)
}
