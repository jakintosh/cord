package admin

import (
	"context"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
)

func (a *API) handleStatus(
	w http.ResponseWriter,
	r *http.Request,
) {
	wire.WriteData(w, http.StatusOK, api.StatusResponse{
		Status: "ok",
	})
}

func (c *Client) Status(
	ctx context.Context,
) error {
	resp, err := c.t.Get(ctx, "/status")
	if err != nil {
		return err
	}
	_, err = daemon.DecodeResponse[api.StatusResponse](resp)
	return err
}
