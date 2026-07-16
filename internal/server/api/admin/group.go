package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type CreateGroupRequest struct {
	Name string `json:"name"`
}

type Group struct {
	Name string `json:"name"`
}

func groupFromService(
	g service.Group,
) Group {
	return Group{
		Name: g.Name,
	}
}

func groupsFromService(
	groups []*service.Group,
) []Group {
	if groups == nil {
		return []Group{}
	}
	result := make([]Group, len(groups))
	for i, g := range groups {
		result[i] = groupFromService(*g)
	}
	return result
}

func (a *API) handleListGroups(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	groups, err := a.service.ListGroups(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, groupsFromService(groups))
}

func (a *API) handlePostGroup(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req CreateGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := a.service.CreateGroup(
		network,
		req.Name,
	); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, nil)
}

func (a *API) handleDeleteGroup(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	group := r.PathValue("group")

	if err := a.service.DeleteGroup(
		network,
		group,
	); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (c *Client) ListGroups(
	ctx context.Context,
	network string,
) (
	[]Group,
	error,
) {
	var result []Group
	return result, c.wire.Get(ctx, "/networks/"+network+"/groups", &result)
}

func (c *Client) CreateGroup(
	ctx context.Context,
	network string,
	name string,
) error {
	req := CreateGroupRequest{
		Name: name,
	}
	body, err := marshalJSON(req)
	if err != nil {
		return err
	}
	return c.wire.Post(ctx, "/networks/"+network+"/groups", body, nil)
}

func (c *Client) DeleteGroup(
	ctx context.Context,
	network string,
	name string,
) error {
	return c.wire.Delete(ctx, "/networks/"+network+"/groups/"+name, nil)
}
