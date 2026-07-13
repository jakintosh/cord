package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type Network struct {
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

type CreateNetworkRequest struct {
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

func networkFromService(
	n service.NetworkConfig,
) Network {
	return Network{
		Name:          n.Name,
		ExternalIP:    n.ExternalIP,
		MainName:      n.Main.Name,
		MainCidr:      n.Main.Cidr,
		MainWgPort:    n.Main.WireguardPort,
		MainApiPort:   n.Main.ApiPort,
		InviteName:    n.Invite.Name,
		InviteCidr:    n.Invite.Cidr,
		InviteWgPort:  n.Invite.WireguardPort,
		InviteApiPort: n.Invite.ApiPort,
		Enabled:       n.Enabled,
	}
}

func (a *API) handleListNetworks(
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

func (a *API) handleGetNetwork(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	network, err := a.service.GetNetwork(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, networkFromService(*network))
}

func (a *API) handlePostNetwork(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req CreateNetworkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	main := service.PlaneConfig{
		Cidr:          req.MainCidr,
		WireguardPort: ptrVal(req.MainWgPort, 0),
		ApiPort:       ptrVal(req.MainApiPort, 0),
	}
	if req.MainName != nil {
		main.Name = *req.MainName
	}

	invite := service.PlaneConfig{
		WireguardPort: ptrVal(req.InviteWgPort, 0),
		ApiPort:       ptrVal(req.InviteApiPort, 0),
	}
	if req.InviteName != nil {
		invite.Name = *req.InviteName
	}
	if req.InviteCidr != nil {
		invite.Cidr = *req.InviteCidr
	}

	network, err := a.service.CreateNetwork(
		req.Name,
		req.ExternalIP,
		main,
		invite,
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, networkFromService(*network))
}

func ptrVal(p *uint16, def uint16) uint16 {
	if p == nil {
		return def
	}
	return *p
}

func (a *API) handleDeleteNetwork(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.DeleteNetwork(name); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (a *API) handlePostNetworkEnable(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.EnableNetwork(name); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (a *API) handlePostNetworkDisable(
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
	var result []string
	return result, c.wire.Get(ctx, "/networks", &result)
}

func (c *Client) ShowNetwork(
	ctx context.Context,
	name string,
) (
	Network,
	error,
) {
	var result Network
	return result, c.wire.Get(ctx, "/networks/"+name, &result)
}

func (c *Client) AddNetwork(
	ctx context.Context,
	req CreateNetworkRequest,
) (
	Network,
	error,
) {
	body, err := marshalJSON(req)
	if err != nil {
		return Network{}, err
	}
	var result Network
	return result, c.wire.Post(ctx, "/networks", body, &result)
}

func (c *Client) DeleteNetwork(
	ctx context.Context,
	name string,
) error {
	return c.wire.Delete(ctx, "/networks/"+name, nil)
}

func (c *Client) EnableNetwork(
	ctx context.Context,
	name string,
) error {
	return c.wire.Post(ctx, "/networks/"+name+"/enable", nil, nil)
}

func (c *Client) DisableNetwork(
	ctx context.Context,
	name string,
) error {
	return c.wire.Post(ctx, "/networks/"+name+"/disable", nil, nil)
}
