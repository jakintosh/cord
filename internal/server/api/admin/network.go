package admin

import (
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (a *API) handleListNetworks(
	w http.ResponseWriter,
	r *http.Request,
) {
	if networks, err := a.runtime.ListNetworks(
		r.Context(),
	); err != nil {
		writeServiceError(w, err)
	} else {
		names := make([]string, len(networks))
		for i, network := range networks {
			names[i] = network.Name
		}
		wire.WriteData(w, http.StatusOK, names)
	}
}

func (a *API) handleGetNetwork(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if network, err := a.runtime.GetNetwork(
		r.Context(),
		name,
	); err != nil {
		writeServiceError(w, err)
	} else {
		networkDTO := networkFromService(*network)
		wire.WriteData(w, http.StatusOK, networkDTO)
	}
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

	main := service.Plane{
		Cidr:          req.MainCidr,
		WireguardPort: ptrVal(req.MainWgPort, 0),
		ApiPort:       ptrVal(req.MainApiPort, 0),
	}
	if req.MainName != nil {
		main.Name = *req.MainName
	}

	invite := service.Plane{
		WireguardPort: ptrVal(req.InviteWgPort, 0),
		ApiPort:       ptrVal(req.InviteApiPort, 0),
	}
	if req.InviteName != nil {
		invite.Name = *req.InviteName
	}
	if req.InviteCidr != nil {
		invite.Cidr = *req.InviteCidr
	}

	if network, err := a.runtime.CreateNetwork(
		r.Context(),
		req.Name,
		req.ExternalIP,
		main,
		invite,
	); err != nil {
		writeServiceError(w, err)
	} else {
		networkDTO := networkFromService(*network)
		wire.WriteData(w, http.StatusCreated, networkDTO)
	}
}

func (a *API) handleDeleteNetwork(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.runtime.DeleteNetwork(
		r.Context(),
		name,
	); err != nil {
		writeServiceError(w, err)
	} else {
		wire.WriteData(w, http.StatusOK, nil)
	}
}

func (a *API) handlePostNetworkEnable(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if status, err := a.runtime.SetNetworkEnabled(
		r.Context(),
		name,
		true,
	); err != nil {
		writeServiceError(w, err)
	} else {
		statusDTO := networkStatusFromRuntime(status)
		wire.WriteData(w, http.StatusOK, statusDTO)
	}
}

func (a *API) handlePostNetworkDisable(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if status, err := a.runtime.SetNetworkEnabled(
		r.Context(),
		name,
		false,
	); err != nil {
		writeServiceError(w, err)
	} else {
		statusDTO := networkStatusFromRuntime(status)
		wire.WriteData(w, http.StatusOK, statusDTO)
	}
}

func networkFromService(
	n service.Network,
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

func ptrVal(p *uint16, def uint16) uint16 {
	if p == nil {
		return def
	}
	return *p
}
