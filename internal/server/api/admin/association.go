package admin

import (
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (a *API) handleListAssociations(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	assocs, err := a.service.ListAssociations(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, associationsFromService(assocs))
}

func (a *API) handlePostAssociation(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req CreateAssociationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.service.CreateAssociation(
		network,
		req.Group1,
		req.Group2,
	); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, nil)
}

func (a *API) handleDeleteAssociation(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	group1 := r.PathValue("group1")
	group2 := r.PathValue("group2")

	if err := a.service.DeleteAssociation(
		network,
		group1,
		group2,
	); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func associationFromService(
	a service.Association,
) Association {
	return Association{
		Group1: a.Group1,
		Group2: a.Group2,
	}
}

func associationsFromService(
	assocs []*service.Association,
) []Association {
	if assocs == nil {
		return []Association{}
	}
	result := make([]Association, len(assocs))
	for i, a := range assocs {
		result[i] = associationFromService(*a)
	}
	return result
}
