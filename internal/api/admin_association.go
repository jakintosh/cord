package api

import (
	"encoding/json"
	"net/http"
)

type Association struct {
	Cidr1 string `json:"cidr1"`
	Cidr2 string `json:"cidr2"`
}

func (a *API) handlePostAssociation(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req Association
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Malformed JSON")
		return
	}
	if req.Cidr1 == "" || req.Cidr2 == "" {
		writeError(w, http.StatusBadRequest, "Missing Required Fields")
		return
	}

	// Placeholder: echo association
	writeData(w, http.StatusCreated, req)
}

func (a *API) handleDeleteAssociation(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req Association
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Malformed JSON")
		return
	}
	if req.Cidr1 == "" || req.Cidr2 == "" {
		writeError(w, http.StatusBadRequest, "Missing Required Fields")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// GET /api/v1/admin/associations
func (a *API) handleGetAssociations(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Placeholder: return empty list
	writeData(w, http.StatusOK, []Association{})
}
