package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type Assignment struct {
	CidrName  string `json:"cidr"`
	GroupName string `json:"group"`
}

type CreateAssignmentRequest struct {
	Cidr  string `json:"cidr"`
	Group string `json:"group"`
}

type RemoveAssignmentRequest struct {
	Cidr  string `json:"cidr"`
	Group string `json:"group"`
}

func assignmentFromService(
	a service.Assignment,
) Assignment {
	return Assignment{
		CidrName:  a.CidrName,
		GroupName: a.GroupName,
	}
}

func assignmentsFromService(
	assignments []*service.Assignment,
) []Assignment {
	if assignments == nil {
		return []Assignment{}
	}
	result := make([]Assignment, len(assignments))
	for i, a := range assignments {
		result[i] = assignmentFromService(*a)
	}
	return result
}

func (a *API) handleListAssignments(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	assignments, err := a.service.ListAssignments(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, assignmentsFromService(assignments))
}

func (a *API) handlePostAssignment(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req CreateAssignmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.service.AssignGroup(
		network,
		req.Cidr,
		req.Group,
	); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, nil)
}

func (a *API) handlePostAssignmentDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req RemoveAssignmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := a.service.RemoveGroup(
		network,
		req.Cidr,
		req.Group,
	); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (c *Client) ListAssignments(
	ctx context.Context,
	network string,
) (
	[]Assignment,
	error,
) {
	var result []Assignment
	return result, c.wire.Get(ctx, "/networks/"+network+"/assignments", &result)
}

func (c *Client) AssignGroup(
	ctx context.Context,
	network string,
	cidr string,
	group string,
) error {
	req := CreateAssignmentRequest{
		Cidr:  cidr,
		Group: group,
	}
	body, err := marshalJSON(req)
	if err != nil {
		return err
	}
	return c.wire.Post(ctx, "/networks/"+network+"/assignments", body, nil)
}

func (c *Client) RemoveGroup(
	ctx context.Context,
	network string,
	cidr string,
	group string,
) error {
	req := RemoveAssignmentRequest{
		Cidr:  cidr,
		Group: group,
	}
	body, err := marshalJSON(req)
	if err != nil {
		return err
	}
	return c.wire.Post(ctx, "/networks/"+network+"/assignments/delete", body, nil)
}
