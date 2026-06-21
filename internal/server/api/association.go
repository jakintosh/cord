package api

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/daemon"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type AssociationDTO struct {
	Cidr1 string `json:"cidr1"`
	Cidr2 string `json:"cidr2"`
}

type AddAssociationRequest struct {
	Cidr1 string `json:"cidr1"`
	Cidr2 string `json:"cidr2"`
}

type DeleteAssociationRequest struct {
	Cidr1 string `json:"cidr1"`
	Cidr2 string `json:"cidr2"`
}

func AssociationDTOFromService(
	a service.Association,
) AssociationDTO {
	return AssociationDTO{
		Cidr1: a.Cidr1,
		Cidr2: a.Cidr2,
	}
}

func AssociationDTOsFromService(
	assocs []*service.Association,
) []AssociationDTO {
	if assocs == nil {
		return nil
	}
	result := make([]AssociationDTO, len(assocs))
	for i, a := range assocs {
		result[i] = AssociationDTOFromService(*a)
	}
	return result
}

func (a *API) handleAssociationList(
	w http.ResponseWriter,
	r *http.Request,
) {
	wire.WriteData(w, http.StatusOK, []AssociationDTO{})
}

func (a *API) handleAssociationAdd(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req AddAssociationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	wire.WriteData(w, http.StatusCreated, AssociationDTO{
		Cidr1: req.Cidr1,
		Cidr2: req.Cidr2,
	})
}

func (a *API) handleAssociationDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req DeleteAssociationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	wire.WriteData(w, http.StatusOK, DeleteResponse{
		Status: "deleted",
		ID:     req.Cidr1 + "/" + req.Cidr2,
	})
}

func (c *Client) ListAssociations(
	ctx context.Context,
	network string,
) (
	[]AssociationDTO,
	error,
) {
	resp, err := c.t.Get(ctx, "/networks/"+network+"/associations")
	if err != nil {
		return nil, err
	}
	return daemon.DecodeResponse[[]AssociationDTO](resp)
}

func (c *Client) AddAssociation(
	ctx context.Context,
	network string,
	req AddAssociationRequest,
) (
	AssociationDTO,
	error,
) {
	resp, err := c.t.Post(ctx, "/networks/"+network+"/associations", req)
	if err != nil {
		return AssociationDTO{}, err
	}
	return daemon.DecodeResponse[AssociationDTO](resp)
}

func (c *Client) DeleteAssociation(
	ctx context.Context,
	network string,
	req DeleteAssociationRequest,
) error {
	resp, err := c.t.Post(ctx, "/networks/"+network+"/associations/delete", req)
	if err != nil {
		return err
	}
	_, err = daemon.DecodeResponse[struct{}](resp)
	return err
}
