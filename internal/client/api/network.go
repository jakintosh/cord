package api

import (
	"context"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
)

type NetworkDTO struct {
	Name           string `json:"name"`
	State          string `json:"state"`
	Enabled        bool   `json:"enabled"`
	Connected      bool   `json:"connected"`
	Address        string `json:"address,omitempty"`
	Interface      string `json:"interface,omitempty"`
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
	inv, err := protocol.Parse(r.Body)
	if err != nil {
		wire.WriteError(w, http.StatusBadRequest, "malformed invitation")
		return
	}

	network, err := a.service.InstallNetwork(*inv)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	networkDTO := networkDTOFromConfig(*network, a.service.IsNetworkRunning(network.Name), a.peerCount(network.Name))
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

// InstallNetwork sends a raw invitation payload to the daemon, which
// parses and validates it. Passing the bytes verbatim means a malformed
// invitation surfaces as a clean 400 from the daemon rather than a
// client-side parse error.
func (c *Client) InstallNetwork(
	ctx context.Context,
	payload []byte,
) (
	NetworkDTO,
	error,
) {
	resp, err := c.t.PostRaw(ctx, "/networks", payload)
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
) error {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/confirm", nil)
	if err != nil {
		return err
	}
	return daemon.DecodeStatus(resp)
}

func (c *Client) UninstallNetwork(
	ctx context.Context,
	name string,
) error {
	resp, err := c.t.Delete(ctx, "/networks/"+name)
	if err != nil {
		return err
	}
	return daemon.DecodeStatus(resp)
}

func (c *Client) EnableNetwork(
	ctx context.Context,
	name string,
) error {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/enable", nil)
	if err != nil {
		return err
	}
	return daemon.DecodeStatus(resp)
}

func (c *Client) DisableNetwork(
	ctx context.Context,
	name string,
) error {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/disable", nil)
	if err != nil {
		return err
	}
	return daemon.DecodeStatus(resp)
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
