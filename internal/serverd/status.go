package serverd

import (
	"context"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

type StatusResponse struct {
	Status string `json:"status"`
}

type DeleteResponse struct {
	Status string `json:"status"`
	ID     string `json:"id"`
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	wire.WriteData(w, http.StatusOK, StatusResponse{
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
	_, err = daemon.DecodeResponse[StatusResponse](resp)
	return err
}
