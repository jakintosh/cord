package api

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

type NetworkDTO struct {
	Name      string `json:"name"`
	State     string `json:"state"`
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
}

func networkDTOFromConfig(
	nc service.NetworkConfig,
	connected bool,
) NetworkDTO {
	return NetworkDTO{
		Name:      nc.Name,
		State:     "installed",
		Enabled:   nc.Enabled,
		Connected: connected,
	}
}

func networkDTOFromInstall(
	inst service.Install,
) NetworkDTO {
	return NetworkDTO{
		Name:  inst.Name,
		State: inst.Phase,
	}
}

type ServerInfoDTO struct {
	PublicKey string `json:"public_key"`
	Endpoint  string `json:"endpoint"`
	Route     string `json:"server_route"`
	APIPort   uint16 `json:"api_port"`
}

type InstallNetworkRequest struct {
	NetworkName   string        `json:"network_name"`
	PrivateKey    string        `json:"private_key"`
	AssignedRoute string        `json:"route"`
	Server        ServerInfoDTO `json:"server"`
}

func installRequestToInvite(
	req InstallNetworkRequest,
) service.Invite {
	return service.Invite{
		NetworkName:   req.NetworkName,
		PrivateKey:    req.PrivateKey,
		AssignedRoute: req.AssignedRoute,
		Server: service.ServerInfo{
			PublicKey: req.Server.PublicKey,
			Endpoint:  req.Server.Endpoint,
			Route:     req.Server.Route,
			APIPort:   req.Server.APIPort,
		},
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

	dtos := make([]NetworkDTO, 0)
	for _, name := range names {
		nc, err := a.service.GetNetwork(name)
		if err != nil {
			continue
		}
		dtos = append(dtos, networkDTOFromConfig(*nc, a.service.IsNetworkRunning(name)))
	}

	installs, err := a.service.ListInstalls()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	for _, inst := range installs {
		dtos = append(dtos, networkDTOFromInstall(*inst))
	}

	wire.WriteData(w, http.StatusOK, dtos)
}

func (a *API) handleNetworkShow(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	nc, err := a.service.GetNetwork(name)
	if err == nil {
		wire.WriteData(w, http.StatusOK, networkDTOFromConfig(*nc, a.service.IsNetworkRunning(name)))
		return
	}

	inst, err := a.service.GetInstall(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, networkDTOFromInstall(*inst))
}

func (a *API) handleNetworkInstall(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req InstallNetworkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "malformed json")
		return
	}
	invite := installRequestToInvite(req)

	network, err := a.service.InstallNetwork(invite)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	networkDTO := networkDTOFromConfig(*network, a.service.IsNetworkRunning(network.Name))
	wire.WriteData(w, http.StatusCreated, networkDTO)
}

func (a *API) handleNetworkRedeem(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if _, err := a.service.Redeem(name); err != nil {
		writeServiceError(w, err)
		return
	}

	inst, err := a.service.GetInstall(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, networkDTOFromInstall(*inst))
}

func (a *API) handleNetworkConfirm(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.Confirm(name); err != nil {
		writeServiceError(w, err)
		return
	}

	nc, err := a.service.GetNetwork(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, networkDTOFromConfig(*nc, a.service.IsNetworkRunning(name)))
}

func (a *API) handleNetworkUninstall(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.UninstallNetwork(name); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, DeleteResponse{
		Status: "deleted",
		ID:     name,
	})
}

func (a *API) handleNetworkEnable(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.EnableNetwork(name); err != nil {
		writeServiceError(w, err)
		return
	}

	nc, err := a.service.GetNetwork(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, networkDTOFromConfig(*nc, a.service.IsNetworkRunning(name)))
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

	nc, err := a.service.GetNetwork(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, networkDTOFromConfig(*nc, a.service.IsNetworkRunning(name)))
}

func (a *API) handleNetworkSync(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.SyncNetwork(name); err != nil {
		writeServiceError(w, err)
		return
	}

	nc, err := a.service.GetNetwork(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, networkDTOFromConfig(*nc, a.service.IsNetworkRunning(name)))
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

func (c *Client) GetNetwork(
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

func (c *Client) InstallNetwork(
	ctx context.Context,
	req InstallNetworkRequest,
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

func (c *Client) RedeemNetwork(
	ctx context.Context,
	name string,
) (
	NetworkDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/redeem", nil)
	if err != nil {
		return NetworkDTO{}, err
	}
	return daemon.DecodeResponse[NetworkDTO](resp)
}

func (c *Client) ConfirmNetwork(
	ctx context.Context,
	name string,
) (
	NetworkDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/confirm", nil)
	if err != nil {
		return NetworkDTO{}, err
	}
	return daemon.DecodeResponse[NetworkDTO](resp)
}

func (c *Client) UninstallNetwork(
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
) (
	NetworkDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/enable", nil)
	if err != nil {
		return NetworkDTO{}, err
	}
	return daemon.DecodeResponse[NetworkDTO](resp)
}

func (c *Client) DisableNetwork(
	ctx context.Context,
	name string,
) (
	NetworkDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/disable", nil)
	if err != nil {
		return NetworkDTO{}, err
	}
	return daemon.DecodeResponse[NetworkDTO](resp)
}

func (c *Client) SyncNetwork(
	ctx context.Context,
	name string,
) (
	NetworkDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/sync", nil)
	if err != nil {
		return NetworkDTO{}, err
	}
	return daemon.DecodeResponse[NetworkDTO](resp)
}
