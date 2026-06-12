package api

import (
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

// CidrDTO is the admin view of a named CIDR.
type CidrDTO struct {
	Name   string `json:"name"`
	Cidr   string `json:"cidr"`
	Length int    `json:"length"`
	Prefix int    `json:"prefix"`
}

// CreateCidrRequest adds a child CIDR within the root range.
type CreateCidrRequest struct {
	Name string `json:"name"`
	Cidr string `json:"cidr"`
}

// RenameCidrRequest renames an existing CIDR.
type RenameCidrRequest struct {
	Name string `json:"name"`
}

func CidrDTOFromServer(
	c server.Cidr,
) CidrDTO {
	return CidrDTO(c)
}

func (c CidrDTO) ToServer() server.Cidr {
	return server.Cidr(c)
}

func (a *API) addAdminCidrRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /cidrs", a.handleListCidrs)
	mux.HandleFunc("POST /cidr", a.handleCreateCidr)
	mux.HandleFunc("GET /cidr/{name}", a.handleGetCidr)
	mux.HandleFunc("PATCH /cidr/{name}", a.handleRenameCidr)
	mux.HandleFunc("DELETE /cidr/{name}", a.handleDeleteCidr)
}

func (a *API) handleCreateCidr(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req CreateCidrRequest
	if err := decodeJSON(r, &req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "malformed json")
		return
	}
	if req.Name == "" || req.Cidr == "" {
		wire.WriteError(w, http.StatusBadRequest, "name and cidr are required")
		return
	}

	createReq := server.CreateCidrRequest{
		Name: req.Name,
		Cidr: req.Cidr,
	}
	if err := a.service.CreateCidr(createReq); err != nil {
		writeServiceError(w, err)
		return
	}

	cidr, err := a.service.GetCidr(req.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	a.mutated()
	wire.WriteData(w, http.StatusCreated, CidrDTOFromServer(*cidr))
}

func (a *API) handleListCidrs(
	w http.ResponseWriter,
	r *http.Request,
) {
	cidrs, err := a.service.ListCidrs()
	if err != nil {
		writeServiceError(w, err)
		return
	}

	dtos := make([]CidrDTO, 0, len(cidrs))
	for _, cidr := range cidrs {
		dtos = append(dtos, CidrDTOFromServer(*cidr))
	}

	wire.WriteData(w, http.StatusOK, dtos)
}

func (a *API) handleGetCidr(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	cidr, err := a.service.GetCidr(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, CidrDTOFromServer(*cidr))
}

func (a *API) handleRenameCidr(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	var req RenameCidrRequest
	if err := decodeJSON(r, &req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "malformed json")
		return
	}
	if req.Name == "" {
		wire.WriteError(w, http.StatusBadRequest, "name is required")
		return
	}

	updateReq := server.UpdateCidrRequest{
		Name: req.Name,
	}
	if err := a.service.UpdateCidr(name, updateReq); err != nil {
		writeServiceError(w, err)
		return
	}

	cidr, err := a.service.GetCidr(req.Name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	a.mutated()
	wire.WriteData(w, http.StatusOK, CidrDTOFromServer(*cidr))
}

func (a *API) handleDeleteCidr(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.DeleteCidr(name); err != nil {
		writeServiceError(w, err)
		return
	}

	a.mutated()
	wire.WriteData(w, http.StatusNoContent, nil)
}
