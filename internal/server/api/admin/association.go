package admin

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
		return []AssociationDTO{}
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
	network := r.PathValue("name")

	assocs, err := a.service.ListAssociations(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, AssociationDTOsFromService(assocs))
}

func (a *API) handleAssociationAdd(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req AddAssociationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.service.CreateAssociation(network, req.Cidr1, req.Cidr2); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, nil)
}

func (a *API) handleAssociationDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req DeleteAssociationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.service.DeleteAssociation(network, req.Cidr1, req.Cidr2); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
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
) error {
	resp, err := c.t.Post(ctx, "/networks/"+network+"/associations", req)
	if err != nil {
		return err
	}
	return daemon.DecodeStatus(resp)
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
	return daemon.DecodeStatus(resp)
}
