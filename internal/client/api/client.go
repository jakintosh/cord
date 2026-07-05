package api

import (
	"errors"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

type Client struct {
	t *daemon.Transport
}

func NewClient(
	socketPath string,
) *Client {
	return &Client{
		t: daemon.NewTransport(socketPath),
	}
}

func writeServiceError(
	w http.ResponseWriter,
	err error,
) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		wire.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrNetworkNotInstalled):
		wire.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrConflict):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNetworkExists):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNetworkEnabled):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNetworkNotEnabled):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		wire.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		wire.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}
