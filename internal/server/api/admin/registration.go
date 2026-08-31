package admin

import (
	"encoding/json"
	"net"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (a *API) handleListRegistrations(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	regs, err := a.service.ListRegistrations(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, registrationsFromService(regs))
}

func (a *API) handlePostRegistration(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req CreateRegistrationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	parsed := net.ParseIP(req.IP)
	if parsed == nil {
		wire.WriteError(w, http.StatusBadRequest, "IP address is required")
		return
	}

	opts := service.RegistrationOptions{
		PeerIP: parsed,
		Admin:  req.Admin,
	}
	invitation, err := a.service.CreateRegistration(network, req.Name, opts)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, invitation)
}

func (a *API) handleDeleteRegistration(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	registration := r.PathValue("registration")

	if err := a.service.RevokeRegistration(network, registration); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (a *API) handleListRegistrationGroups(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	registration := r.PathValue("registration")

	groups, err := a.service.ListRegistrationGroups(network, registration)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	wire.WriteData(w, http.StatusOK, groupsFromService(groups))
}

func (a *API) handlePostRegistrationGroup(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	registration := r.PathValue("registration")

	var req RegistrationGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.service.AssignRegistrationGroup(network, registration, req.Group); err != nil {
		writeServiceError(w, err)
		return
	}
	wire.WriteData(w, http.StatusCreated, nil)
}

func (a *API) handleDeleteRegistrationGroup(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	registration := r.PathValue("registration")
	group := r.PathValue("group")

	if err := a.service.RemoveRegistrationGroup(network, registration, group); err != nil {
		writeServiceError(w, err)
		return
	}
	wire.WriteData(w, http.StatusOK, nil)
}

func registrationFromService(
	reg service.Registration,
) Registration {
	return Registration{
		Name:      reg.Name,
		Route:     reg.MainRoute,
		Admin:     reg.Admin,
		Redeemed:  reg.Redeemed,
		ExpiresAt: reg.ExpiresAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

func registrationsFromService(
	regs []*service.Registration,
) []Registration {
	if regs == nil {
		return []Registration{}
	}
	result := make([]Registration, len(regs))
	for i, reg := range regs {
		result[i] = registrationFromService(*reg)
	}
	return result
}
