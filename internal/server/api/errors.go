package api

import (
	"errors"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func writeServiceError(
	w http.ResponseWriter,
	err error,
) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		wire.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrConflict):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNetworkExists):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrCIDROverlap):
		wire.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		wire.WriteError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrInviteExpired):
		wire.WriteError(w, http.StatusGone, err.Error())
	case errors.Is(err, service.ErrInviteRedeemed):
		wire.WriteError(w, http.StatusGone, err.Error())
	case errors.Is(err, service.ErrNetworkRunning):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNotImplemented):
		wire.WriteError(w, http.StatusNotImplemented, err.Error())
	default:
		wire.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}
