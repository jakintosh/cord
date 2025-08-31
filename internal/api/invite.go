package api

import (
	"github.com/gorilla/mux"
	"net/http"
)

type RedeemResponse struct {
	Interface struct {
		NetworkName  string `json:"networkName"`
		PrivateKey   string `json:"privateKey"`
		AssignedCidr string `json:"assignedCidr"`
	} `json:"interface"`
	Server struct {
		PublicKey        string `json:"publicKey"`
		ExternalEndpoint string `json:"externalEndpoint"`
		InternalEndpoint string `json:"internalEndpoint"`
	} `json:"server"`
}

func (api *API) handlePostInviteRedeem(
	w http.ResponseWriter,
	r *http.Request,
) {
	vars := mux.Vars(r)
	if vars["key"] == "" {
		writeError(w, http.StatusBadRequest, "Malformed Request")
		return
	}

	// Placeholder: return empty Invite payload per spec shape
	writeData(w, http.StatusOK, RedeemResponse{})
}

func (api *API) handlePostInviteConfirm(
	w http.ResponseWriter,
	r *http.Request,
) {
	vars := mux.Vars(r)
	if vars["key"] == "" {
		writeError(w, http.StatusBadRequest, "Malformed Request")
		return
	}

	// Placeholder: respond success
	writeData(w, http.StatusOK, nil)
}
