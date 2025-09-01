package api

import (
	"encoding/json"
	"github.com/gorilla/mux"
	"net/http"
)

type CreatePeerRequest struct {
	Name  string `json:"name"`
	Cidr  string `json:"cidr"`
	Admin bool   `json:"admin"`
}

type UpdatePeerRequest struct {
	Name    *string `json:"name"`
	Admin   *bool   `json:"admin"`
	Enabled *bool   `json:"enabled"`
}

type AdminPeer struct {
	Name      string `json:"name"`
	Cidr      string `json:"cidr"`
	PublicKey string `json:"publicKey"`
	Admin     bool   `json:"admin"`
	Enabled   bool   `json:"enabled"`
	Confirmed bool   `json:"confirmed"`
}

func (api *API) handlePostAdminPeer(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req CreatePeerRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Malformed JSON")
		return
	}

	// Placeholder: return default AdminPeer
	writeData(w, http.StatusCreated, AdminPeer{})
}

func (api *API) handleGetAdminPeers(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Placeholder: return empty list
	writeData(w, http.StatusOK, []AdminPeer{})
}

func (api *API) handleGetAdminPeer(
	w http.ResponseWriter,
	r *http.Request,
) {
	vars := mux.Vars(r)
	if vars["name"] == "" {
		writeError(w, http.StatusBadRequest, "Malformed Request")
		return
	}

	// Placeholder: return default AdminPeer
	writeData(w, http.StatusOK, AdminPeer{})
}

func (api *API) handlePatchAdminPeer(
	w http.ResponseWriter,
	r *http.Request,
) {
	vars := mux.Vars(r)
	if vars["name"] == "" {
		writeError(w, http.StatusBadRequest, "Malformed Request")
		return
	}

	var req UpdatePeerRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Malformed JSON")
		return
	}

	// Placeholder: return default AdminPeer
	writeData(w, http.StatusOK, AdminPeer{})
}

func (api *API) handleDeleteAdminPeer(
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
