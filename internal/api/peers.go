package api

import (
	"net/http"
)

// PublicPeer mirrors the spec shape for GET /peers
type PublicPeer struct {
	Name      string              `json:"name"`
	Cidr      string              `json:"cidr"`
	PublicKey string              `json:"publicKey"`
	Endpoints map[string]Endpoint `json:"endpoints"`
}

type Endpoint struct {
	WitnessKey string `json:"witnessKey"`
	Endpoint   string `json:"endpoint"`
	Timestamp  int64  `json:"timestamp"`
}

func (a *API) handleGetPeers(
	w http.ResponseWriter,
	r *http.Request,
) {
	// Placeholder: return an empty list per spec shape
	writeData(w, http.StatusOK, []PublicPeer{})
}
