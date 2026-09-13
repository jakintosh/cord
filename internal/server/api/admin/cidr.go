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

	if cidrs, err := a.runtime.ListCidrs(r.Context(), network); err != nil {
		writeServiceError(w, err)
	} else {
		cidrsDTO := cidrsFromService(cidrs)
		wire.WriteData(w, http.StatusOK, cidrsDTO)
	}
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

	if err := a.runtime.CreateCidr(
		r.Context(),
		network,
		req.Name,
		req.Cidr,
	); err != nil {
		writeServiceError(w, err)
	} else {
		wire.WriteData(w, http.StatusCreated, nil)
	}
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

	if err := a.runtime.UpdateCidr(
		r.Context(),
		network,
		cidr,
		req.Name,
	); err != nil {
		writeServiceError(w, err)
	} else {
		wire.WriteData(w, http.StatusOK, nil)
	}
}

func (a *API) handleDeleteCidr(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	cidr := r.PathValue("cidr")

	if err := a.runtime.DeleteCidr(
		r.Context(),
		network,
		cidr,
	); err != nil {
		writeServiceError(w, err)
	} else {
		wire.WriteData(w, http.StatusOK, nil)
	}
}

func (a *API) handleListCidrGroups(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	cidr := r.PathValue("cidr")

	if groups, err := a.runtime.ListCidrGroups(
		r.Context(),
		network,
		cidr,
	); err != nil {
		writeServiceError(w, err)
	} else {
		groupsDTO := groupsFromService(groups)
		wire.WriteData(w, http.StatusOK, groupsDTO)
	}
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

	if err := a.runtime.AssignCidrGroup(
		r.Context(),
		network,
		cidr,
		req.Group,
	); err != nil {
		writeServiceError(w, err)
	} else {
		wire.WriteData(w, http.StatusCreated, nil)
	}
}

func (a *API) handleDeleteCidrGroup(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	cidr := r.PathValue("cidr")
	group := r.PathValue("group")

	if err := a.runtime.RemoveCidrGroup(
		r.Context(),
		network,
		cidr,
		group,
	); err != nil {
		writeServiceError(w, err)
	} else {
		wire.WriteData(w, http.StatusOK, nil)
	}
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
