package serverd

import (
	"context"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

type InviteDTO struct {
	Name      string `json:"name"`
	Redeemed  bool   `json:"redeemed"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

func handleInviteList(
	w http.ResponseWriter,
	r *http.Request,
) {
	wire.WriteData(w, http.StatusOK, []InviteDTO{})
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
