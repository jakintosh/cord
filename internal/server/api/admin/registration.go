package admin

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type CreateRegistrationRequest struct {
	Name  string `json:"name"`
	IP    string `json:"ip"`
	Admin bool   `json:"admin"`
}

type RegistrationGroupRequest struct {
	Group string `json:"group"`
}

type Registration struct {
	Name      string `json:"name"`
	Route     string `json:"route"`
	Admin     bool   `json:"admin"`
	Redeemed  bool   `json:"redeemed"`
	ExpiresAt string `json:"expires_at"`
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
		IP:    parsed,
		Admin: req.Admin,
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

func (c *Client) ListRegistrations(
	ctx context.Context,
	network string,
) (
	[]Registration,
	error,
) {
	var result []Registration
	return result, c.wire.Get(ctx, "/networks/"+network+"/registrations", &result)
}

func (c *Client) CreateInvite(
	ctx context.Context,
	network string,
	name string,
	ip string,
	admin bool,
) (
	*protocol.Invitation,
	error,
) {
	req := CreateRegistrationRequest{
		Name:  name,
		IP:    ip,
		Admin: admin,
	}
	body, err := marshalJSON(req)
	if err != nil {
		return nil, err
	}
	var result *protocol.Invitation
	return result, c.wire.Post(ctx, "/networks/"+network+"/registrations", body, &result)
}

func (c *Client) RevokeRegistration(
	ctx context.Context,
	network string,
	registration string,
) error {
	return c.wire.Delete(ctx, "/networks/"+network+"/registrations/"+registration, nil)
}

func (c *Client) ListRegistrationGroups(
	ctx context.Context,
	network string,
	registration string,
) (
	[]Group,
	error,
) {
	var result []Group
	path := "/networks/" + network + "/registrations/" + registration + "/groups"
	return result, c.wire.Get(ctx, path, &result)
}

func (c *Client) AssignRegistrationGroup(
	ctx context.Context,
	network string,
	registration string,
	group string,
) error {
	body, err := marshalJSON(RegistrationGroupRequest{Group: group})
	if err != nil {
		return err
	}
	path := "/networks/" + network + "/registrations/" + registration + "/groups"
	return c.wire.Post(ctx, path, body, nil)
}

func (c *Client) RemoveRegistrationGroup(
	ctx context.Context,
	network string,
	registration string,
	group string,
) error {
	path := "/networks/" + network + "/registrations/" + registration + "/groups/" + group
	return c.wire.Delete(ctx, path, nil)
}
