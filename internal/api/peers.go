package api

import (
	"net/http"
)

type PublicPeer struct {
	Name      string `json:"name"`
	Cidr      string `json:"cidr"`
	PublicKey string `json:"publicKey"`
	Endpoints map[string]struct {
		WitnessKey string `json:"witnessKey"`
		Endpoint   string `json:"endpoint"`
		Timestamp  int64  `json:"timestamp"`
	} `json:"endpoints"`
}

func (api *API) handleGetPeers(
	w http.ResponseWriter,
	r *http.Request,
) {
	peerName, ok := r.Context().Value("peerName").(string)
	if !ok {
		// auth failed
	}

	peers, err := api.server.GetPeersOfPeerNamed(peerName)
	if err != nil {
		// service failed
	}

	writeData(w, http.StatusOK, peers)
}
