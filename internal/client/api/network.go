package api

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

type SetListenPortRequest struct {
	ListenPort uint16 `json:"listen_port"`
}

type NetworkDTO struct {
	Name           string `json:"name"`
	State          string `json:"state"`
	Enabled        bool   `json:"enabled"`
	Connected      bool   `json:"connected"`
	Address        string `json:"address,omitempty"`
	Interface      string `json:"interface,omitempty"`
	ListenPort     uint16 `json:"listen_port,omitempty"`
	ServerEndpoint string `json:"server_endpoint,omitempty"`
	PeerCount      int    `json:"peer_count,omitempty"`
}

func networkDTOFromConfig(
	nc service.NetworkConfig,
	connected bool,
	peerCount int,
) NetworkDTO {
	return NetworkDTO{
		Name:           nc.Name,
		State:          "installed",
		Enabled:        nc.Enabled,
		Connected:      connected,
		Address:        nc.AssignedRoute,
		Interface:      nc.InterfaceName,
		ListenPort:     nc.ListenPort,
		ServerEndpoint: nc.Server.Endpoint,
		PeerCount:      peerCount,
	}
}

func networkDTOFromInstall(
	inst service.Install,
) NetworkDTO {
	return NetworkDTO{
		Name:    inst.Name,
		State:   inst.Phase,
		Address: inst.MainAssignedRoute,
	}
}

// peerCount returns the number of cached peers for network, or 0 if the
// count can't be determined.
func (a *API) peerCount(
	network string,
) int {
	peers, err := a.service.ListPeers(network)
	if err != nil {
		return 0
	}
	return len(peers)
}

// listNetworkDTOs composes the full network list: installed networks
// enriched with live/cached state, plus networks still mid-install.
// Shared by the network list handler and the status handler.
func (a *API) listNetworkDTOs() (
	[]NetworkDTO,
	error,
) {
	names, err := a.service.ListNetworks()
	if err != nil {
		return nil, err
	}

	dtos := make([]NetworkDTO, 0)
	for _, name := range names {
		nc, err := a.service.GetNetwork(name)
		if err != nil {
			continue
		}
		dtos = append(dtos, networkDTOFromConfig(*nc, a.service.IsNetworkRunning(name), a.peerCount(name)))
	}

	installs, err := a.service.ListInstalls()
	if err != nil {
		return nil, err
	}
	for _, inst := range installs {
		dtos = append(dtos, networkDTOFromInstall(*inst))
	}

	return dtos, nil
}

func (a *API) handleNetworkList(
	w http.ResponseWriter,
	r *http.Request,
) {
	dtos, err := a.listNetworkDTOs()
	if err != nil {
		writeServiceError(w, err)
		return
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
		wire.WriteData(w, http.StatusOK, networkDTOFromConfig(*nc, a.service.IsNetworkRunning(name), a.peerCount(name)))
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
	var request service.InstallRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "malformed invitation")
		return
	}

	network, err := a.service.InstallNetwork(request)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	networkDTO := networkDTOFromConfig(*network, a.service.IsNetworkRunning(network.Name), a.peerCount(network.Name))
	wire.WriteData(w, http.StatusCreated, networkDTO)
}

func (a *API) handleNetworkListenPort(
	w http.ResponseWriter,
	r *http.Request,
) {
	var request SetListenPortRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "invalid listen port request")
		return
	}
	if err := a.service.SetNetworkListenPort(r.PathValue("name"), request.ListenPort); err != nil {
		writeServiceError(w, err)
		return
	}
	wire.WriteData(w, http.StatusOK, nil)
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

	wire.WriteData(w, http.StatusOK, nil)
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

	wire.WriteData(w, http.StatusOK, nil)
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

	wire.WriteData(w, http.StatusOK, networkDTOFromConfig(*nc, a.service.IsNetworkRunning(name), a.peerCount(name)))
}

func (c *Client) ListNetworks(
	ctx context.Context,
) (
	[]NetworkDTO,
	error,
) {
	var result []NetworkDTO
	return result, c.wire.Get(ctx, "/networks", &result)
}

func (c *Client) GetNetwork(
	ctx context.Context,
	name string,
) (
	NetworkDTO,
	error,
) {
	var result NetworkDTO
	return result, c.wire.Get(ctx, "/networks/"+name, &result)
}

func (c *Client) InstallNetwork(
	ctx context.Context,
	request service.InstallRequest,
) (
	NetworkDTO,
	error,
) {
	body, err := json.Marshal(request)
	if err != nil {
		return NetworkDTO{}, err
	}
	var result NetworkDTO
	return result, c.wire.Post(ctx, "/networks", body, &result)
}

// SetNetworkListenPort persists a network's local WireGuard UDP port and
// restarts the network if it is currently enabled.
func (c *Client) SetNetworkListenPort(
	ctx context.Context,
	name string,
	listenPort uint16,
) error {
	body, err := json.Marshal(SetListenPortRequest{ListenPort: listenPort})
	if err != nil {
		return err
	}
	return c.wire.Post(ctx, "/networks/"+name+"/listen-port", body, nil)
}

func (c *Client) RedeemNetwork(
	ctx context.Context,
	name string,
) (
	NetworkDTO,
	error,
) {
	var result NetworkDTO
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
	NetworkDTO,
	error,
) {
	var result NetworkDTO
	return result, c.wire.Post(ctx, "/networks/"+name+"/sync", nil, &result)
}
