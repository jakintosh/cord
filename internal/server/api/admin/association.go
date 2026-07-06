package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
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
	var result []AssociationDTO
	return result, c.wire.Get(ctx, "/networks/"+network+"/associations", &result)
}

func (c *Client) AddAssociation(
	ctx context.Context,
	network string,
	cidr1 string,
	cidr2 string,
) error {
	req := AddAssociationRequest{Cidr1: cidr1, Cidr2: cidr2}
	body, err := marshalJSON(req)
	if err != nil {
		return err
	}
	return c.wire.Post(ctx, "/networks/"+network+"/associations", body, nil)
}

func (c *Client) DeleteAssociation(
	ctx context.Context,
	network string,
	cidr1 string,
	cidr2 string,
) error {
	req := DeleteAssociationRequest{Cidr1: cidr1, Cidr2: cidr2}
	body, err := marshalJSON(req)
	if err != nil {
		return err
	}
	return c.wire.Post(ctx, "/networks/"+network+"/associations/delete", body, nil)
}
