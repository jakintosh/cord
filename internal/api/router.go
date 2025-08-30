package api

import (
	"github.com/gorilla/mux"
)

func BuildRouter(r *mux.Router, api *API) {
	v1 := r.PathPrefix("/api/v1").Subrouter()

	// main network endpoints
	v1.HandleFunc("/peers", api.withMainAuth(api.handleGetPeers)).Methods("GET")
	v1.HandleFunc("/endpoint", api.withMainAuth(api.handlePostEndpoint)).Methods("POST")
	v1.HandleFunc("/invite/confirm/{key}", api.withMainAuth(api.handlePostInviteConfirm)).Methods("POST")
	v1.HandleFunc("/invite/redeem/{key}", api.withInviteAuth(api.handlePostInviteRedeem)).Methods("POST")

	// admin endpoints
	admin := v1.PathPrefix("/admin").Subrouter()
	// peers
	admin.HandleFunc("/peers", api.withAdminAuth(api.handleGetAdminPeers)).Methods("GET")
	admin.HandleFunc("/peer", api.withAdminAuth(api.handlePostAdminPeer)).Methods("POST")
	admin.HandleFunc("/peer/{name}", api.withAdminAuth(api.handleGetAdminPeer)).Methods("GET")
	admin.HandleFunc("/peer/{name}", api.withAdminAuth(api.handlePutAdminPeer)).Methods("PUT")
	admin.HandleFunc("/peer/{name}", api.withAdminAuth(api.handleDeleteAdminPeer)).Methods("DELETE")

	// cidrs
	admin.HandleFunc("/cidrs", api.withAdminAuth(api.handleGetAdminCidrs)).Methods("GET")
	admin.HandleFunc("/cidr", api.withAdminAuth(api.handlePostAdminCidr)).Methods("POST")
	admin.HandleFunc("/cidr/{name}", api.withAdminAuth(api.handleGetAdminCidr)).Methods("GET")
	admin.HandleFunc("/cidr/{name}", api.withAdminAuth(api.handlePutAdminCidr)).Methods("PUT")
	admin.HandleFunc("/cidr/{name}", api.withAdminAuth(api.handleDeleteAdminCidr)).Methods("DELETE")

	// associations
	admin.HandleFunc("/associations", api.withAdminAuth(api.handleGetAssociations)).Methods("GET")
	admin.HandleFunc("/association", api.withAdminAuth(api.handlePostAssociation)).Methods("POST")
	admin.HandleFunc("/association", api.withAdminAuth(api.handleDeleteAssociation)).Methods("DELETE")
}
