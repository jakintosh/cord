package api

import (
	"encoding/json"
	"net/http"
)

type EndpointReport struct {
	PeerKey    string `json:"peerKey"`
	WitnessKey string `json:"witnessKey"`
	Endpoint   string `json:"endpoint"`
	Timestamp  int64  `json:"timestamp"`
}

func (api *API) handlePostEndpoint(
	w http.ResponseWriter,
	r *http.Request,
) {
	var reports []EndpointReport
	err := json.NewDecoder(r.Body).Decode(&reports)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Malformed JSON")
		return
	}

	writeData(w, http.StatusOK, nil)
}
