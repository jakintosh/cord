package api

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
)

// InstallRequest is the payload accepted by the client control API to install
// a server-issued invitation with local network settings.
type InstallRequest struct {
	Invitation protocol.Invitation `json:"invitation"`
	ListenPort *uint16             `json:"listen_port"`
}

type UpdateNetworkRequest struct {
	ListenPort *uint16 `json:"listen_port,omitempty"`
}

type Network struct {
	Name           string `json:"name"`
	State          string `json:"state"`
	Enabled        bool   `json:"enabled"`
	Address        string `json:"address,omitempty"`
	Interface      string `json:"interface,omitempty"`
	ListenPort     uint16 `json:"listen_port,omitempty"`
	ServerEndpoint string `json:"server_endpoint,omitempty"`
}

func networkFromConfig(
	nc service.NetworkConfig,
) Network {
	return Network{
		Name:           nc.Name,
		State:          "installed",
		Enabled:        nc.Enabled,
		Address:        nc.AssignedRoute,
		Interface:      nc.InterfaceName,
		ListenPort:     nc.ListenPort,
		ServerEndpoint: nc.Server.Endpoint,
	}
}

func networkFromInstall(
	inst service.Install,
) Network {
	return Network{
		Name:    inst.Name,
		State:   inst.Phase,
		Address: inst.MainAssignedRoute,
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

	nc, err := a.service.GetNetwork(name)
	if err == nil {
		wire.WriteData(w, http.StatusOK, networkFromConfig(*nc))
		return
	}

	inst, err := a.service.GetInstall(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, networkFromInstall(*inst))
}

func (a *API) handlePostNetwork(
	w http.ResponseWriter,
	r *http.Request,
) {
	var request InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "malformed invitation")
		return
	}

	network, err := a.service.InstallNetwork(request.Invitation, service.NetworkOptions{
		ListenPort: request.ListenPort,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}

	networkDTO := networkFromConfig(*network)
	wire.WriteData(w, http.StatusCreated, networkDTO)
}

func (a *API) handlePatchNetwork(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	var request UpdateNetworkRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "invalid network update request")
		return
	}

	if err := a.service.UpdateNetwork(name, service.NetworkOptions{
		ListenPort: request.ListenPort,
	}); err != nil {
		writeServiceError(w, err)
		return
	}

	network, err := a.service.GetNetwork(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, networkFromConfig(*network))
}

func (a *API) handlePostNetworkRedeem(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	inst, err := a.service.RedeemInstall(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, networkFromInstall(*inst))
}

func (a *API) handlePostNetworkConfirm(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.ConfirmInstall(name); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (a *API) handleDeleteNetwork(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.UninstallNetwork(name); err != nil {
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

func (a *API) handPostNetworkSync(
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

	wire.WriteData(w, http.StatusOK, networkFromConfig(*nc))
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

func (c *Client) GetNetwork(
	ctx context.Context,
	name string,
) (
	Network,
	error,
) {
	var result Network
	return result, c.wire.Get(ctx, "/networks/"+name, &result)
}

func (c *Client) InstallNetwork(
	ctx context.Context,
	invitation protocol.Invitation,
	listenPort *uint16,
) (
	Network,
	error,
) {
	req := InstallRequest{
		Invitation: invitation,
		ListenPort: listenPort,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Network{}, err
	}
	var result Network
	return result, c.wire.Post(ctx, "/networks", body, &result)
}

// UpdateNetwork updates a network's local configuration.
func (c *Client) UpdateNetwork(
	ctx context.Context,
	name string,
	listenPort *uint16,
) (
	Network,
	error,
) {
	req := UpdateNetworkRequest{
		ListenPort: listenPort,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Network{}, err
	}
	var result Network
	return result, c.wire.Patch(ctx, "/networks/"+name, body, &result)
}

func (c *Client) RedeemNetwork(
	ctx context.Context,
	name string,
) (
	Network,
	error,
) {
	var result Network
	return result, c.wire.Post(ctx, "/networks/"+name+"/redeem", nil, &result)
}

func (c *Client) ConfirmNetwork(
	ctx context.Context,
	name string,
) error {
	return c.wire.Post(ctx, "/networks/"+name+"/confirm", nil, nil)
}

func (c *Client) UninstallNetwork(
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

func (c *Client) SyncNetwork(
	ctx context.Context,
	name string,
) (
	Network,
	error,
) {
	var result Network
	return result, c.wire.Post(ctx, "/networks/"+name+"/sync", nil, &result)
}
