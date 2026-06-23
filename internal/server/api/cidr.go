package api

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
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

func CidrDTOFromService(
	c service.Cidr,
) CidrDTO {
	return CidrDTO{
		Name: c.Name,
		Cidr: c.Cidr,
	}
}

func CidrDTOsFromService(
	cidrs []*service.Cidr,
) []CidrDTO {
	if cidrs == nil {
		return []CidrDTO{}
	}
	result := make([]CidrDTO, len(cidrs))
	for i, c := range cidrs {
		result[i] = CidrDTOFromService(*c)
	}
	return result
}

func (a *API) handleCidrList(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	cidrs, err := a.service.ListCidrs(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, CidrDTOsFromService(cidrs))
}

func (a *API) handleCidrAdd(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req AddCidrRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.service.AddCidr(network, service.CreateCidrRequest{
		Name: req.Name,
		Cidr: req.Cidr,
	}); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, CidrDTO{
		Name: req.Name,
		Cidr: req.Cidr,
	})
}

func (a *API) handleCidrRename(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	cidr := r.PathValue("cidr")

	var req RenameCidrRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.service.UpdateCidr(network, cidr, service.UpdateCidrRequest{
		Name: req.Name,
	}); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, CidrDTO{
		Name: req.Name,
	})
}

func (a *API) handleCidrDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	cidr := r.PathValue("cidr")

	if err := a.service.RemoveCidr(network, cidr); err != nil {
		writeServiceError(w, err)
		return
	}

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
