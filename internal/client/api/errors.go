package api

import (
	"errors"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

func writeServiceError(
	w http.ResponseWriter,
	err error,
) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		wire.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrConflict),
		errors.Is(err, service.ErrInstallState),
		errors.Is(err, service.ErrNetworkExists),
		errors.Is(err, service.ErrNetworkNotEnabled),
		errors.Is(err, service.ErrTopologyUnavailable):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		wire.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		wire.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}
