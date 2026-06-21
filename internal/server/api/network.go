package api

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type NetworkDTO struct {
	Name       string `json:"name"`
	Cidr       string `json:"cidr"`
	ExternalIP string `json:"external_ip"`
	Port       uint16 `json:"port"`
	InviteCidr string `json:"invite_cidr,omitempty"`
}

type AddNetworkRequest struct {
	Name       string `json:"name"`
	Cidr       string `json:"cidr"`
	ExternalIP string `json:"external_ip"`
	Port       uint16 `json:"port"`
}

func NetworkDTOFromService(
	n service.Network,
) NetworkDTO {
	return NetworkDTO{
		Name:       n.Name,
		Cidr:       n.RootCidr,
		ExternalIP: n.ExternalIP,
		Port:       n.ListenPort,
		InviteCidr: n.InviteCidr,
	}
}

func (a *API) handleNetworkList(
	w http.ResponseWriter,
	r *http.Request,
) {
	wire.WriteData(w, http.StatusOK, []NetworkDTO{})
}

func (a *API) handleNetworkShow(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")
	wire.WriteData(w, http.StatusOK, NetworkDTO{
		Name: name,
	})
}

func (a *API) handleNetworkAdd(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req AddNetworkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	wire.WriteData(w, http.StatusCreated, NetworkDTO{
		Name:       req.Name,
		Cidr:       req.Cidr,
		ExternalIP: req.ExternalIP,
		Port:       req.Port,
	})
}

func (a *API) handleNetworkDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")
	wire.WriteData(w, http.StatusOK, DeleteResponse{
		Status: "deleted",
		ID:     name,
	})
}

func (c *Client) ListNetworks(
	ctx context.Context,
) (
	[]NetworkDTO,
	error,
) {
	resp, err := c.t.Get(ctx, "/networks")
	if err != nil {
		return nil, err
	}
	return daemon.DecodeResponse[[]NetworkDTO](resp)
}

func (c *Client) ShowNetwork(
	ctx context.Context,
	name string,
) (
	NetworkDTO,
	error,
) {
	resp, err := c.t.Get(ctx, "/networks/"+name)
	if err != nil {
		return NetworkDTO{}, err
	}
	return daemon.DecodeResponse[NetworkDTO](resp)
}

func (c *Client) AddNetwork(
	ctx context.Context,
	req AddNetworkRequest,
) (
	NetworkDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks", req)
	if err != nil {
		return NetworkDTO{}, err
	}
	return daemon.DecodeResponse[NetworkDTO](resp)
}

func (c *Client) DeleteNetwork(
	ctx context.Context,
	name string,
) error {
	resp, err := c.t.Delete(ctx, "/networks/"+name)
	if err != nil {
		return err
	}
	_, err = daemon.DecodeResponse[struct{}](resp)
	return err
}
