package serverd

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
)

type CidrDTO struct {
	Name string `json:"name"`
	Cidr string `json:"cidr"`
}

type AddCidrRequest struct {
	Name string `json:"name"`
	Cidr string `json:"cidr"`
}

type RenameCidrRequest struct {
	Name string `json:"name"`
}

func handleCidrList(
	w http.ResponseWriter,
	r *http.Request,
) {
	wire.WriteData(w, http.StatusOK, []CidrDTO{})
}

func handleCidrAdd(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req AddCidrRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	wire.WriteData(w, http.StatusCreated, CidrDTO{
		Name: req.Name,
		Cidr: req.Cidr,
	})
}

func handleCidrRename(
	w http.ResponseWriter,
	r *http.Request,
) {
	cidr := r.PathValue("cidr")

	var req RenameCidrRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	wire.WriteData(w, http.StatusOK, CidrDTO{
		Name: req.Name,
		Cidr: cidr,
	})
}

func handleCidrDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	cidr := r.PathValue("cidr")
	wire.WriteData(w, http.StatusOK, DeleteResponse{
		Status: "deleted",
		ID:     cidr,
	})
}

func (c *Client) ListCidrs(
	ctx context.Context,
	network string,
) (
	[]CidrDTO,
	error,
) {
	resp, err := c.t.Get(ctx, "/networks/"+network+"/cidrs")
	if err != nil {
		return nil, err
	}
	return daemon.DecodeResponse[[]CidrDTO](resp)
}

func (c *Client) AddCidr(
	ctx context.Context,
	network string,
	req AddCidrRequest,
) (
	CidrDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+network+"/cidrs", req)
	if err != nil {
		return CidrDTO{}, err
	}
	return daemon.DecodeResponse[CidrDTO](resp)
}

func (c *Client) RenameCidr(
	ctx context.Context,
	network string,
	cidr string,
	newName string,
) (
	CidrDTO,
	error,
) {
	req := RenameCidrRequest{Name: newName}
	resp, err := c.t.Patch(ctx, "/networks/"+network+"/cidrs/"+cidr, req)
	if err != nil {
		return CidrDTO{}, err
	}
	return daemon.DecodeResponse[CidrDTO](resp)
}

func (c *Client) DeleteCidr(
	ctx context.Context,
	network string,
	cidr string,
) error {
	resp, err := c.t.Delete(ctx, "/networks/"+network+"/cidrs/"+cidr)
	if err != nil {
		return err
	}
	_, err = daemon.DecodeResponse[struct{}](resp)
	return err
}
