package api

import (
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

func (a *API) handleListNetworks(
	w http.ResponseWriter,
	r *http.Request,
) {
	networks, err := a.service.ListNetworks()
	if err != nil {
		writeServiceError(w, err)
		return
	}

	names := make([]string, len(networks))
	for i, network := range networks {
		names[i] = network.Name
	}

	wire.WriteData(w, http.StatusOK, names)
}

func (a *API) handleGetNetwork(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	network, err := a.service.GetNetwork(name)
	if err == nil {
		wire.WriteData(w, http.StatusOK, networkFromService(*network))
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

	network, err := a.runtime.Install(
		r.Context(),
		request.Invitation,
		service.NetworkOptions{
			ListenPort: request.ListenPort,
		},
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	networkDTO := networkFromService(*network)
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

	network, err := a.runtime.UpdateNetwork(
		name,
		service.NetworkOptions{
			ListenPort: request.ListenPort,
		},
	)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	networkDTO := networkFromService(*network)
	wire.WriteData(w, http.StatusOK, networkDTO)
}

func (a *API) handlePostNetworkRedeem(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	inst, err := a.runtime.RedeemInstall(r.Context(), name)
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

	if _, err := a.runtime.ConfirmInstall(r.Context(), name); err != nil {
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

	if err := a.runtime.UninstallNetwork(name); err != nil {
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

	status, err := a.runtime.SetNetworkEnabled(name, true)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	statusDTO := networkStatusFromRuntime(status)
	wire.WriteData(w, http.StatusOK, statusDTO)
}

func (a *API) handlePostNetworkDisable(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	status, err := a.runtime.SetNetworkEnabled(name, false)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	statusDTO := networkStatusFromRuntime(status)
	wire.WriteData(w, http.StatusOK, statusDTO)
}

func (a *API) handPostNetworkSync(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	network, err := a.runtime.Sync(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	networkDTO := networkFromService(*network)
	wire.WriteData(w, http.StatusOK, networkDTO)
}

func networkFromService(
	n service.Network,
) Network {
	return Network{
		Name:           n.Name,
		State:          "installed",
		Enabled:        n.Enabled,
		Address:        n.AssignedRoute,
		Interface:      n.InterfaceName,
		ListenPort:     n.ListenPort,
		ServerEndpoint: n.Server.Endpoint,
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
