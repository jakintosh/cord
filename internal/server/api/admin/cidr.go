package admin

import (
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (a *API) handleListCidrs(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	cidrs, err := a.service.ListCidrs(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, cidrsFromService(cidrs))
}

func (a *API) handlePostCidr(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req CreateCidrRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.service.CreateCidr(network, req.Name, req.Cidr); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, nil)
}

func (a *API) handlePatchCidr(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	cidr := r.PathValue("cidr")

	var req UpdateCidrRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.service.UpdateCidr(network, cidr, req.Name); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (a *API) handleDeleteCidr(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	cidr := r.PathValue("cidr")

	if err := a.service.DeleteCidr(network, cidr); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (a *API) handleListCidrGroups(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	cidr := r.PathValue("cidr")

	groups, err := a.service.ListCidrGroups(network, cidr)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	wire.WriteData(w, http.StatusOK, groupsFromService(groups))
}

func (a *API) handlePostCidrGroup(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	cidr := r.PathValue("cidr")

	var req CidrGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.service.AssignCidrGroup(network, cidr, req.Group); err != nil {
		writeServiceError(w, err)
		return
	}
	wire.WriteData(w, http.StatusCreated, nil)
}

func (a *API) handleDeleteCidrGroup(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	cidr := r.PathValue("cidr")
	group := r.PathValue("group")

	if err := a.service.RemoveCidrGroup(network, cidr, group); err != nil {
		writeServiceError(w, err)
		return
	}
	wire.WriteData(w, http.StatusOK, nil)
}

func cidrFromService(
	c service.Cidr,
) Cidr {
	return Cidr{
		Name: c.Name,
		Cidr: c.Cidr,
	}
}

func cidrsFromService(
	cidrs []*service.Cidr,
) []Cidr {
	if cidrs == nil {
		return []Cidr{}
	}
	result := make([]Cidr, len(cidrs))
	for i, c := range cidrs {
		result[i] = cidrFromService(*c)
	}
	return result
}
