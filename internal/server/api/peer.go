package api

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type PeerDTO struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key,omitempty"`
	Ip        string `json:"ip,omitempty"`
	Admin     bool   `json:"admin"`
	Enabled   bool   `json:"enabled"`
}

type AddPeerRequest struct {
	Name  string `json:"name"`
	Ip    string `json:"ip"`
	Admin bool   `json:"admin"`
}

type RenamePeerRequest struct {
	Name string `json:"name"`
}

func PeerDTOFromService(
	p service.Peer,
) PeerDTO {
	return PeerDTO{
		Name:      p.Name,
		PublicKey: p.PublicKey,
		Ip:        p.Cidr,
		Admin:     p.Admin,
		Enabled:   p.Enabled,
	}
}

func PeerDTOsFromService(
	peers []*service.Peer,
) []PeerDTO {
	if peers == nil {
		return []PeerDTO{}
	}
	result := make([]PeerDTO, len(peers))
	for i, p := range peers {
		result[i] = PeerDTOFromService(*p)
	}
	return result
}

func (a *API) handlePeerList(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	peers, err := a.service.ListPeers(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, PeerDTOsFromService(peers))
}

func (a *API) handlePeerAdd(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req AddPeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	cfg := service.PeerConfig{
		Name:  req.Name,
		IP:    req.Ip,
		Admin: req.Admin,
	}

	invite, err := a.service.AddPeer(network, cfg)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, invite)
}

func (a *API) handlePeerRename(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	peer := r.PathValue("peer")

	var req RenamePeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	updated, err := a.service.UpdatePeer(network, peer, service.UpdatePeerRequest{
		Name: &req.Name,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, PeerDTOFromService(*updated))
}

func (a *API) handlePeerDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	peer := r.PathValue("peer")

	if err := a.service.RemovePeer(network, peer); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, DeleteResponse{
		Status: "deleted",
		ID:     peer,
	})
}

func (a *API) handlePeerEnable(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	peer := r.PathValue("peer")

	if err := a.service.EnablePeer(network, peer); err != nil {
		writeServiceError(w, err)
		return
	}

	updated, err := a.service.GetPeer(network, peer)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, PeerDTOFromService(*updated))
}

func (a *API) handlePeerDisable(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	peer := r.PathValue("peer")

	if err := a.service.DisablePeer(network, peer); err != nil {
		writeServiceError(w, err)
		return
	}

	updated, err := a.service.GetPeer(network, peer)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, PeerDTOFromService(*updated))
}

func (a *API) handlePeerVisible(
	w http.ResponseWriter,
	r *http.Request,
) {
	wire.WriteError(w, http.StatusNotImplemented, "peer visibility requires WireGuard identity resolution (not yet implemented)")
}

func (c *Client) ListPeers(
	ctx context.Context,
	network string,
) (
	[]PeerDTO,
	error,
) {
	resp, err := c.t.Get(ctx, "/networks/"+network+"/peers")
	if err != nil {
		return nil, err
	}
	return daemon.DecodeResponse[[]PeerDTO](resp)
}

func (c *Client) AddPeer(
	ctx context.Context,
	network string,
	req AddPeerRequest,
) (
	*service.PeerInvite,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+network+"/peers", req)
	if err != nil {
		return nil, err
	}
	return daemon.DecodeResponse[*service.PeerInvite](resp)
}

func (c *Client) RenamePeer(
	ctx context.Context,
	network string,
	peer string,
	newName string,
) (
	PeerDTO,
	error,
) {
	req := RenamePeerRequest{Name: newName}
	resp, err := c.t.Patch(ctx, "/networks/"+network+"/peers/"+peer, req)
	if err != nil {
		return PeerDTO{}, err
	}
	return daemon.DecodeResponse[PeerDTO](resp)
}

func (c *Client) DeletePeer(
	ctx context.Context,
	network string,
	peer string,
) error {
	resp, err := c.t.Delete(ctx, "/networks/"+network+"/peers/"+peer)
	if err != nil {
		return err
	}
	_, err = daemon.DecodeResponse[struct{}](resp)
	return err
}

func (c *Client) EnablePeer(
	ctx context.Context,
	network string,
	peer string,
) (
	PeerDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+network+"/peers/"+peer+"/enable", nil)
	if err != nil {
		return PeerDTO{}, err
	}
	return daemon.DecodeResponse[PeerDTO](resp)
}

func (c *Client) DisablePeer(
	ctx context.Context,
	network string,
	peer string,
) (
	PeerDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+network+"/peers/"+peer+"/disable", nil)
	if err != nil {
		return PeerDTO{}, err
	}
	return daemon.DecodeResponse[PeerDTO](resp)
}

func (c *Client) ListPeersVisible(
	ctx context.Context,
	network string,
) (
	[]PeerDTO,
	error,
) {
	resp, err := c.t.Get(ctx, "/networks/"+network+"/peers/visible")
	if err != nil {
		return nil, err
	}
	return daemon.DecodeResponse[[]PeerDTO](resp)
}
