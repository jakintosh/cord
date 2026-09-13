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

	if regs, err := a.runtime.ListRegistrations(
		r.Context(),
		network,
	); err != nil {
		writeServiceError(w, err)
	} else {
		registrationsDTO := registrationsFromService(regs)
		wire.WriteData(w, http.StatusOK, registrationsDTO)
	}
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
	if invitation, err := a.runtime.CreateRegistration(
		r.Context(),
		network,
		req.Name,
		opts,
	); err != nil {
		writeServiceError(w, err)
	} else {
		wire.WriteData(w, http.StatusCreated, invitation)
	}
}

func (a *API) handleDeleteRegistration(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	registration := r.PathValue("registration")

	if err := a.runtime.RevokeRegistration(
		r.Context(),
		network,
		registration,
	); err != nil {
		writeServiceError(w, err)
	} else {
		wire.WriteData(w, http.StatusOK, nil)
	}
}

func (a *API) handleListRegistrationGroups(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	registration := r.PathValue("registration")

	if groups, err := a.runtime.ListRegistrationGroups(
		r.Context(),
		network,
		registration,
	); err != nil {
		writeServiceError(w, err)
	} else {
		groupsDTO := groupsFromService(groups)
		wire.WriteData(w, http.StatusOK, groupsDTO)
	}
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

	if err := a.runtime.AssignRegistrationGroup(
		r.Context(),
		network,
		registration,
		req.Group,
	); err != nil {
		writeServiceError(w, err)
	} else {
		wire.WriteData(w, http.StatusCreated, nil)
	}
}

func (a *API) handleDeleteRegistrationGroup(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	registration := r.PathValue("registration")
	group := r.PathValue("group")

	if err := a.runtime.RemoveRegistrationGroup(
		r.Context(),
		network,
		registration,
		group,
	); err != nil {
		writeServiceError(w, err)
	} else {
		wire.WriteData(w, http.StatusOK, nil)
	}
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
