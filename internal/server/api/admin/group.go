package admin

import (
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

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
