package api

import (
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

// AssociationDTO is a symmetric visibility link between two CIDRs.
type AssociationDTO struct {
	Cidr1 string `json:"cidr1"`
	Cidr2 string `json:"cidr2"`
}

func AssociationDTOFromServer(
	a server.Association,
) AssociationDTO {
	return AssociationDTO(a)
}

func (a AssociationDTO) ToServer() server.Association {
	return server.Association(a)
}

func (a *API) addAdminAssociationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /associations", a.handleListAssociations)
	mux.HandleFunc("POST /association", a.handleCreateAssociation)
	mux.HandleFunc("DELETE /association/{cidr1}/{cidr2}", a.handleDeleteAssociation)
}

func (a *API) handleCreateAssociation(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req AssociationDTO
	if err := decodeJSON(r, &req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "malformed json")
		return
	}
	if req.Cidr1 == "" || req.Cidr2 == "" {
		wire.WriteError(w, http.StatusBadRequest, "cidr1 and cidr2 are required")
		return
	}

	if err := a.service.CreateAssociation(req.Cidr1, req.Cidr2); err != nil {
		writeServiceError(w, err)
		return
	}

	a.mutated()
	wire.WriteData(w, http.StatusCreated, req)
}

func (a *API) handleListAssociations(
	w http.ResponseWriter,
	r *http.Request,
) {
	associations, err := a.service.ListAssociations()
	if err != nil {
		writeServiceError(w, err)
		return
	}

	dtos := make([]AssociationDTO, 0, len(associations))
	for _, association := range associations {
		dtos = append(dtos, AssociationDTOFromServer(*association))
	}

	wire.WriteData(w, http.StatusOK, dtos)
}

func (a *API) handleDeleteAssociation(
	w http.ResponseWriter,
	r *http.Request,
) {
	cidr1 := r.PathValue("cidr1")
	cidr2 := r.PathValue("cidr2")

	if err := a.service.DeleteAssociation(cidr1, cidr2); err != nil {
		writeServiceError(w, err)
		return
	}

	a.mutated()
	wire.WriteData(w, http.StatusNoContent, nil)
}
