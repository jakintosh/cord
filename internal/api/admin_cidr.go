package api

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"net/http"
)

type CreateCidrRequest struct {
	Name string `json:"name"`
	Cidr string `json:"cidr"`
}

type RenameCidrRequest struct {
	Name string `json:"name"`
}

type CidrResponse struct {
	Name   string `json:"name"`
	Cidr   string `json:"cidr"`
	Length int    `json:"length"`
	Prefix int    `json:"prefix"`
}

func (api *API) handlePostAdminCidr(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req CreateCidrRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Malformed JSON")
		return
	}

	// Placeholder: return default CIDR
	writeData(w, http.StatusCreated, CidrResponse{})
}

func (api *API) handleGetAdminCidrs(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Placeholder: return empty list
	writeData(w, http.StatusOK, []CidrResponse{})
}

func (api *API) handleGetAdminCidr(
	w http.ResponseWriter,
	r *http.Request,
) {
	vars := mux.Vars(r)
	if vars["name"] == "" {
		writeError(w, http.StatusBadRequest, "Malformed Request")
		return
	}

	// Placeholder: return default CIDR
	writeData(w, http.StatusOK, CidrResponse{})
}

func (api *API) handlePatchAdminCidr(
	w http.ResponseWriter,
	r *http.Request,
) {
	vars := mux.Vars(r)
	if vars["name"] == "" {
		writeError(w, http.StatusBadRequest, "Malformed Request")
		return
	}

	var req RenameCidrRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Malformed JSON")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "Missing Required Fields")
		return
	}

	// Placeholder: return default CIDR
	writeData(w, http.StatusOK, CidrResponse{})
}

func (api *API) handleDeleteAdminCidr(
	w http.ResponseWriter,
	r *http.Request,
) {
	vars := mux.Vars(r)
	if vars["name"] == "" {
		writeError(w, http.StatusBadRequest, "Malformed Request")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
