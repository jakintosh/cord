package invite

import (
	"errors"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (a *API) writeServiceError(
	w http.ResponseWriter,
	r *http.Request,
	network string,
	operation string,
	err error,
) {
	if r.Context().Err() != nil {
		return
	}
	switch {
	case errors.Is(err, service.ErrNotFound):
		wire.WriteError(w, http.StatusNotFound, "resource not found")

	case errors.Is(err, service.ErrRegistrationExpired):
		wire.WriteError(w, http.StatusGone, "registration expired")

	case errors.Is(err, service.ErrRegistrationRedeemed):
		wire.WriteError(w, http.StatusConflict, "registration is no longer redeemable")

	case errors.Is(err, service.ErrConflict):
		wire.WriteError(w, http.StatusConflict, "resource conflict")

	case errors.Is(err, runtime.ErrNotRunning):
		wire.WriteError(w, http.StatusServiceUnavailable, "service unavailable")

	default:
		a.log.Error(
			"invite request failed",
			"operation", operation,
			"network", network,
			"err", err,
		)
		wire.WriteError(w, http.StatusInternalServerError, "internal server error")
	}
}
