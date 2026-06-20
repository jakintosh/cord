package serverd

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
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

func handlePeerList(
	w http.ResponseWriter,
	r *http.Request,
) {
	wire.WriteData(w, http.StatusOK, []PeerDTO{})
}

func handlePeerAdd(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req AddPeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	wire.WriteData(w, http.StatusCreated, PeerDTO{
		Name:  req.Name,
		Ip:    req.Ip,
		Admin: req.Admin,
	})
}

func handlePeerRename(
	w http.ResponseWriter,
	r *http.Request,
) {
	peer := r.PathValue("peer")

	var req RenamePeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	wire.WriteData(w, http.StatusOK, PeerDTO{
		Name: req.Name,
		Ip:   peer,
	})
}

func handlePeerDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	peer := r.PathValue("peer")
	wire.WriteData(w, http.StatusOK, DeleteResponse{
		Status: "deleted",
		ID:     peer,
	})
}

func handlePeerEnable(
	w http.ResponseWriter,
	r *http.Request,
) {
	peer := r.PathValue("peer")
	wire.WriteData(w, http.StatusOK, PeerDTO{
		Name:    peer,
		Enabled: true,
	})
}

func handlePeerDisable(
	w http.ResponseWriter,
	r *http.Request,
) {
	peer := r.PathValue("peer")
	wire.WriteData(w, http.StatusOK, PeerDTO{
		Name:    peer,
		Enabled: false,
	})
}

func handlePeerVisible(
	w http.ResponseWriter,
	r *http.Request,
) {
	wire.WriteData(w, http.StatusOK, []PeerDTO{})
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
	PeerDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+network+"/peers", req)
	if err != nil {
		return PeerDTO{}, err
	}
	return daemon.DecodeResponse[PeerDTO](resp)
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
