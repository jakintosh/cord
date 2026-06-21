package clientd

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

type NetworkDTO struct {
	Name      string `json:"name"`
	Installed bool   `json:"installed"`
	Enabled   bool   `json:"enabled"`
	Connected bool   `json:"connected"`
}

type InstallNetworkRequest struct {
	InvitePath string `json:"invite_path"`
}

func handleNetworkList(
	w http.ResponseWriter,
	r *http.Request,
) {
	wire.WriteData(w, http.StatusOK, []NetworkDTO{})
}

func handleNetworkShow(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")
	wire.WriteData(w, http.StatusOK, NetworkDTO{
		Name: name,
	})
}

func handleNetworkInstall(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req InstallNetworkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	wire.WriteData(w, http.StatusCreated, NetworkDTO{
		Name:      req.InvitePath,
		Installed: true,
	})
}

func handleNetworkUninstall(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")
	wire.WriteData(w, http.StatusOK, DeleteResponse{
		Status: "deleted",
		ID:     name,
	})
}

func handleNetworkEnable(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")
	wire.WriteData(w, http.StatusOK, NetworkDTO{
		Name:      name,
		Installed: true,
		Enabled:   true,
	})
}

func handleNetworkDisable(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")
	wire.WriteData(w, http.StatusOK, NetworkDTO{
		Name:      name,
		Installed: true,
		Enabled:   false,
	})
}

func handleNetworkUp(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")
	wire.WriteData(w, http.StatusOK, NetworkDTO{
		Name:      name,
		Installed: true,
		Enabled:   true,
		Connected: true,
	})
}

func handleNetworkDown(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")
	wire.WriteData(w, http.StatusOK, NetworkDTO{
		Name:      name,
		Installed: true,
		Enabled:   true,
		Connected: false,
	})
}

func handleNetworkFetch(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")
	wire.WriteData(w, http.StatusOK, NetworkDTO{
		Name:      name,
		Installed: true,
	})
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

func (c *Client) NetworkUp(
	ctx context.Context,
	name string,
) (
	NetworkDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/up", nil)
	if err != nil {
		return NetworkDTO{}, err
	}
	return daemon.DecodeResponse[NetworkDTO](resp)
}

func (c *Client) NetworkDown(
	ctx context.Context,
	name string,
) (
	NetworkDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+name+"/down", nil)
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
