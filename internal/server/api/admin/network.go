package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server/api"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type NetworkDTO struct {
	Name       string `json:"name"`
	Cidr       string `json:"cidr"`
	ExternalIP string `json:"external_ip"`
	Port       uint16 `json:"port"`
	InviteCidr string `json:"invite_cidr,omitempty"`
	Enabled    bool   `json:"enabled"`
}

type AddNetworkRequest struct {
	Name       string `json:"name"`
	Cidr       string `json:"cidr"`
	InviteCidr string `json:"invite_cidr"`
	ExternalIP string `json:"external_ip"`
	Port       uint16 `json:"port"`
	InvitePort uint16 `json:"invite_port"`
	ApiPort    uint16 `json:"api_port"`
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
		Enabled:    n.Enabled,
	}
}

func (a *API) handleNetworkList(
	w http.ResponseWriter,
	r *http.Request,
) {
	names, err := a.service.ListNetworks()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	if names == nil {
		names = []string{}
	}

	wire.WriteData(w, http.StatusOK, names)
}

func (a *API) handleNetworkShow(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	nw, err := a.service.GetNetwork(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, NetworkDTOFromService(*nw))
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

	cfg := service.Network{
		Name:             req.Name,
		RootCidr:         req.Cidr,
		InviteCidr:       req.InviteCidr,
		ExternalIP:       req.ExternalIP,
		ListenPort:       req.Port,
		InviteListenPort: req.InvitePort,
		ApiPort:          req.ApiPort,
	}

	nw, err := a.service.CreateNetwork(cfg)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, NetworkDTOFromService(*nw))
}

func (a *API) handleNetworkDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.DeleteNetwork(name); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, api.DeleteResponse{
		Status: "deleted",
		ID:     name,
	})
}

func (a *API) handleNetworkEnable(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.EnableNetwork(r.Context(), name); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, api.StatusResponse{Status: "enabled"})
}

func (a *API) handleNetworkDisable(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.DisableNetwork(name); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, api.StatusResponse{Status: "disabled"})
}

func (c *Client) ListNetworks(
	ctx context.Context,
) (
	[]string,
	error,
) {
	resp, err := c.t.Get(ctx, "/networks")
	if err != nil {
		return nil, err
	}
	return daemon.DecodeResponse[[]string](resp)
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

func (c *Client) EnableNetwork(
	ctx context.Context,
	name string,
) error {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/enable", nil)
	if err != nil {
		return err
	}
	_, err = daemon.DecodeResponse[api.StatusResponse](resp)
	return err
}

func (c *Client) DisableNetwork(
	ctx context.Context,
	name string,
) error {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/disable", nil)
	if err != nil {
		return err
	}
	_, err = daemon.DecodeResponse[api.StatusResponse](resp)
	return err
}
