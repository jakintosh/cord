package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

type NetworkDTO struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
}

type InstallNetworkRequest struct {
	NetworkName    string `json:"network_name"`
	TempPrivKey    string `json:"temp_private_key"`
	TempCidr       string `json:"temp_cidr"`
	ServerPubkey   string `json:"server_pubkey"`
	ServerEndpoint string `json:"server_endpoint"`
	TempApiAddr    string `json:"temp_api_addr"`
}

func installRequestToInvite(req InstallNetworkRequest) service.Invite {
	return service.Invite{
		NetworkName:    req.NetworkName,
		TempPrivKey:    req.TempPrivKey,
		TempCidr:       req.TempCidr,
		ServerPubkey:   req.ServerPubkey,
		ServerEndpoint: req.ServerEndpoint,
		TempApiAddr:    req.TempApiAddr,
	}
}

func NetworkDTOFromService(
	nw service.Network,
	connected bool,
) NetworkDTO {
	return NetworkDTO{
		Name:      nw.Name,
		Installed: true,
		Enabled:   nw.Enabled,
		Connected: connected,
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

	statuses, err := a.service.Status()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	connected := make(map[string]bool, len(statuses))
	for _, st := range statuses {
		connected[st.Name] = st.Running
	}

	dtos := make([]NetworkDTO, 0, len(names))
	for _, name := range names {
		nw, err := a.service.GetNetwork(name)
		if err != nil {
			continue
		}
		dtos = append(dtos, NetworkDTOFromService(*nw, connected[name]))
	}

	wire.WriteData(w, http.StatusOK, dtos)
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

	statuses, err := a.service.Status()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	connected := false
	for _, st := range statuses {
		if st.Name == name {
			connected = st.Running
			break
		}
	}

	wire.WriteData(w, http.StatusOK, NetworkDTOFromService(*nw, connected))
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
	if req.NetworkName == "" {
		wire.WriteError(w, http.StatusBadRequest, "network_name is required")
		return
	}

	invite := installRequestToInvite(req)
	nw, err := a.service.InstallNetwork(invite)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, NetworkDTOFromService(*nw, false))
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

	if err := a.service.EnableNetwork(r.Context(), name); err != nil {
		writeServiceError(w, err)
		return
	}

	nw, err := a.service.GetNetwork(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, NetworkDTOFromService(*nw, true))
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

	nw, err := a.service.GetNetwork(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, NetworkDTOFromService(*nw, false))
}

func (a *API) handleNetworkFetch(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.FetchNetwork(name); err != nil {
		writeServiceError(w, err)
		return
	}

	nw, err := a.service.GetNetwork(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	statuses, err := a.service.Status()
	if err != nil {
		writeServiceError(w, err)
		return
	}
	connected := false
	for _, st := range statuses {
		if st.Name == name {
			connected = st.Running
			break
		}
	}

	wire.WriteData(w, http.StatusOK, NetworkDTOFromService(*nw, connected))
}

func writeServiceError(
	w http.ResponseWriter,
	err error,
) {
	switch {
	case errors.Is(err, service.ErrNotFound):
		wire.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrNetworkNotInstalled):
		wire.WriteError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, service.ErrConflict):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNetworkExists):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNetworkEnabled):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrNetworkNotEnabled):
		wire.WriteError(w, http.StatusConflict, err.Error())
	case errors.Is(err, service.ErrInvalidInput):
		wire.WriteError(w, http.StatusBadRequest, err.Error())
	default:
		wire.WriteError(w, http.StatusInternalServerError, err.Error())
	}
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

func (c *Client) FetchNetwork(
	ctx context.Context,
	name string,
) (
	NetworkDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/fetch", nil)
	if err != nil {
		return NetworkDTO{}, err
	}
	return daemon.DecodeResponse[NetworkDTO](resp)
}
