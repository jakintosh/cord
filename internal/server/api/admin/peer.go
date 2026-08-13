package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type Peer struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	Route     string `json:"route"`
	Admin     bool   `json:"admin"`
	Enabled   bool   `json:"enabled"`
}

type UpdatePeerRequest struct {
	Name    *string `json:"name,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
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

func (a *API) handleListPeers(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	peers, err := a.service.ListPeers(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, peersFromService(peers))
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
	updated, err := a.service.UpdatePeer(network, peer, diff)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, peerFromService(*updated))
}

func (a *API) handleDeletePeer(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	peer := r.PathValue("peer")

	if err := a.service.RemovePeer(network, peer); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (c *Client) ListPeers(
	ctx context.Context,
	network string,
) (
	[]Peer,
	error,
) {
	var result []Peer
	return result, c.wire.Get(ctx, "/networks/"+network+"/peers", &result)
}

func (c *Client) UpdatePeer(
	ctx context.Context,
	network string,
	peer string,
	newName *string,
	enabled *bool,
) (
	Peer,
	error,
) {
	req := UpdatePeerRequest{
		Name:    newName,
		Enabled: enabled,
	}
	body, err := marshalJSON(req)
	if err != nil {
		return Peer{}, err
	}
	var result Peer
	return result, c.wire.Patch(ctx, "/networks/"+network+"/peers/"+peer, body, &result)
}

func (c *Client) DeletePeer(
	ctx context.Context,
	network string,
	peer string,
) error {
	return c.wire.Delete(ctx, "/networks/"+network+"/peers/"+peer, nil)
}
