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
	Name          string `json:"name"`
	ExternalIP    string `json:"external_ip"`
	MainName      string `json:"main_name"`
	MainCidr      string `json:"main_cidr"`
	MainWgPort    uint16 `json:"main_wg_port"`
	MainApiPort   uint16 `json:"main_api_port"`
	InviteName    string `json:"invite_name"`
	InviteCidr    string `json:"invite_cidr"`
	InviteWgPort  uint16 `json:"invite_wg_port"`
	InviteApiPort uint16 `json:"invite_api_port"`
	Enabled       bool   `json:"enabled"`
}

type AddNetworkRequest struct {
	Name          string  `json:"name"`
	ExternalIP    string  `json:"external_ip"`
	MainName      *string `json:"main_name,omitempty"`
	MainCidr      string  `json:"main_cidr"`
	MainWgPort    *uint16 `json:"main_wg_port,omitempty"`
	MainApiPort   *uint16 `json:"main_api_port,omitempty"`
	InviteName    *string `json:"invite_name,omitempty"`
	InviteCidr    *string `json:"invite_cidr,omitempty"`
	InviteWgPort  *uint16 `json:"invite_wg_port,omitempty"`
	InviteApiPort *uint16 `json:"invite_api_port,omitempty"`
}

func NetworkDTOFromService(
	n service.Network,
) NetworkDTO {
	return NetworkDTO{
		Name:          n.Name,
		ExternalIP:    n.ExternalIP,
		MainName:      n.MainName,
		MainCidr:      n.MainCidr,
		MainWgPort:    n.MainWireguardPort,
		MainApiPort:   n.MainApiPort,
		InviteName:    n.InviteName,
		InviteCidr:    n.InviteCidr,
		InviteWgPort:  n.InviteWireguardPort,
		InviteApiPort: n.InviteApiPort,
		Enabled:       n.Enabled,
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

	network, err := a.service.GetNetwork(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, NetworkDTOFromService(*network))
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

	network, err := a.service.CreateNetwork(
		req.Name,
		req.ExternalIP,
		req.MainCidr,
		req.MainName,
		req.MainWgPort,
		req.MainApiPort,
		req.InviteName,
		req.InviteCidr,
		req.InviteWgPort,
		req.InviteApiPort,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, NetworkDTOFromService(*network))
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

	wire.WriteData(w, http.StatusOK, nil)
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

	wire.WriteData(w, http.StatusOK, nil)
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
