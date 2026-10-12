package admin

import (
	"encoding/json"
	"errors"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (a *API) handleListGroups(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	if groups, err := a.runtime.ListGroups(
		r.Context(),
		network,
	); err != nil {
		a.writeServiceError(w, r, err)
	} else {
		groupsDTO := groupsFromService(groups)
		wire.WriteData(w, http.StatusOK, groupsDTO)
	}
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

	if _, err := a.runtime.CreateGroup(
		r.Context(),
		network,
		req.Name,
	); err != nil {
		if errors.Is(err, service.ErrNotFound) {
			// This mutation's missing parent has a stable public message.
			err = service.ErrNotFound
		}
		a.writeServiceError(w, r, err)
	} else {
		wire.WriteData(w, http.StatusCreated, nil)
	}
}

func (a *API) handleDeleteGroup(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	group := r.PathValue("group")

	if err := a.runtime.DeleteGroup(
		r.Context(),
		network,
		group,
	); err != nil {
		a.writeServiceError(w, r, err)
	} else {
		wire.WriteData(w, http.StatusOK, nil)
	}
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
