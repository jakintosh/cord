package admin

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
	case errors.Is(err, service.ErrRegistrationAddressExhausted):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrRegistrationExpired):
		wire.WriteError(w, http.StatusGone, err.Error())
	case errors.Is(err, service.ErrRegistrationRedeemed):
		wire.WriteError(w, http.StatusGone, err.Error())
	case errors.Is(err, service.ErrNetworkEnabled):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNotImplemented):
		wire.WriteError(w, http.StatusNotImplemented, err.Error())
	default:
		wire.WriteError(w, http.StatusInternalServerError, err.Error())
	}
}
