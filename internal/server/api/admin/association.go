package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type Association struct {
	Group1 string `json:"group1"`
	Group2 string `json:"group2"`
}

type CreateAssociationRequest struct {
	Group1 string `json:"group1"`
	Group2 string `json:"group2"`
}

type DeleteAssociationRequest struct {
	Group1 string `json:"group1"`
	Group2 string `json:"group2"`
}

func associationFromService(
	a service.Association,
) Association {
	return Association{
		Group1: a.Group1,
		Group2: a.Group2,
	}
}

func associationsFromService(
	assocs []*service.Association,
) []Association {
	if assocs == nil {
		return []Association{}
	}
	result := make([]Association, len(assocs))
	for i, a := range assocs {
		result[i] = associationFromService(*a)
	}
	return result
}

func (a *API) handleListAssociations(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	assocs, err := a.service.ListAssociations(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, associationsFromService(assocs))
}

func (a *API) handlePostAssociation(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req CreateAssociationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.service.CreateAssociation(
		network,
		req.Group1,
		req.Group2,
	); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, nil)
}

func (a *API) handlePostAssociationDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req DeleteAssociationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.service.DeleteAssociation(
		network,
		req.Group1,
		req.Group2,
	); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (c *Client) ListAssociations(
	ctx context.Context,
	network string,
) (
	[]Association,
	error,
) {
	var result []Association
	return result, c.wire.Get(ctx, "/networks/"+network+"/associations", &result)
}

func (c *Client) AddAssociation(
	ctx context.Context,
	network string,
	group1 string,
	group2 string,
) error {
	req := CreateAssociationRequest{
		Group1: group1,
		Group2: group2,
	}
	body, err := marshalJSON(req)
	if err != nil {
		return err
	}
	return c.wire.Post(ctx, "/networks/"+network+"/associations", body, nil)
}

func (c *Client) DeleteAssociation(
	ctx context.Context,
	network string,
	group1 string,
	group2 string,
) error {
	req := DeleteAssociationRequest{
		Group1: group1,
		Group2: group2,
	}
	body, err := marshalJSON(req)
	if err != nil {
		return err
	}
	return c.wire.Post(ctx, "/networks/"+network+"/associations/delete", body, nil)
}
