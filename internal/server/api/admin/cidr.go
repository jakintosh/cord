package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
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

	if err := a.service.CreateCidr(network, req.Name, req.Cidr); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, nil)
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

	if err := a.service.UpdateCidr(network, cidr, req.Name); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (a *API) handleCidrDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	cidr := r.PathValue("cidr")

	if err := a.service.DeleteCidr(network, cidr); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (c *Client) ListCidrs(
	ctx context.Context,
	network string,
) (
	[]CidrDTO,
	error,
) {
	var result []CidrDTO
	return result, c.wire.Get(ctx, "/networks/"+network+"/cidrs", &result)
}

func (c *Client) AddCidr(
	ctx context.Context,
	network string,
	name string,
	cidr string,
) error {
	body, err := marshalJSON(AddCidrRequest{
		Name: name,
		Cidr: cidr,
	})
	if err != nil {
		return err
	}
	return c.wire.Post(ctx, "/networks/"+network+"/cidrs", body, nil)
}

func (c *Client) RenameCidr(
	ctx context.Context,
	network string,
	cidr string,
	newName string,
) error {
	req := RenameCidrRequest{Name: newName}
	body, err := marshalJSON(req)
	if err != nil {
		return err
	}
	return c.wire.Patch(ctx, "/networks/"+network+"/cidrs/"+cidr, body, nil)
}

func (c *Client) DeleteCidr(
	ctx context.Context,
	network string,
	cidr string,
) error {
	return c.wire.Delete(ctx, "/networks/"+network+"/cidrs/"+cidr, nil)
}
