package api

import (
	"encoding/json"
	"net/http"
)

type EndpointSighting struct {
	PeerKey    string `json:"peerKey"`
	WitnessKey string `json:"witnessKey"`
	Endpoint   string `json:"endpoint"`
	Timestamp  int64  `json:"timestamp"`
}

func (a *API) handlePostReport(
	w http.ResponseWriter,
	r *http.Request,
) {
	var sightings []EndpointSighting
	err := json.NewDecoder(r.Body).Decode(&sightings)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Malformed JSON")
		return
	}

	// Placeholder: accept input and return 200 OK
	writeData(w, http.StatusOK, nil)
}
