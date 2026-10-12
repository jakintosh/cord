package admin

import (
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (a *API) handleListPeers(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	if peers, err := a.runtime.ListPeers(
		r.Context(),
		network,
	); err != nil {
		a.writeServiceError(w, r, err)
	} else {
		peersDTO := peersFromService(peers)
		wire.WriteData(w, http.StatusOK, peersDTO)
	}
}

func (a *API) handlePatchPeer(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	peer := r.PathValue("peer")

	var req UpdatePeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	diff := service.PeerDiff{
		Name:    req.Name,
		Enabled: req.Enabled,
	}
	if updated, err := a.runtime.UpdatePeer(
		r.Context(),
		network,
		peer,
		diff,
	); err != nil {
		a.writeServiceError(w, r, err)
	} else {
		peerDTO := peerFromService(*updated)
		wire.WriteData(w, http.StatusOK, peerDTO)
	}
}

func (a *API) handleDeletePeer(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	peer := r.PathValue("peer")

	if err := a.runtime.RemovePeer(
		r.Context(),
		network,
		peer,
	); err != nil {
		a.writeServiceError(w, r, err)
	} else {
		wire.WriteData(w, http.StatusOK, nil)
	}
}

func peerFromService(
	p service.Peer,
) Peer {
	return Peer{
		Name:      p.Name,
		PublicKey: p.PublicKey,
		Route:     p.Route,
		Admin:     p.Admin,
		Enabled:   p.Enabled,
	}
}

func peersFromService(
	peers []*service.Peer,
) []Peer {
	if peers == nil {
		return []Peer{}
	}
	result := make([]Peer, len(peers))
	for i, p := range peers {
		result[i] = peerFromService(*p)
	}
	return result
}
