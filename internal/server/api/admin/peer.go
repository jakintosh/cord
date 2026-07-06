package admin

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

type PeerDTO struct {
	Name      string `json:"name"`
	PublicKey string `json:"public_key"`
	Route     string `json:"route"`
	Admin     bool   `json:"admin"`
	Enabled   bool   `json:"enabled"`
}

type CreateInviteRequest struct {
	Name  string  `json:"name"`
	Ip    *string `json:"ip,omitempty"`
	Admin bool    `json:"admin"`
}

type RenamePeerRequest struct {
	Name string `json:"name"`
}

func PeerDTOFromService(
	p service.Peer,
) PeerDTO {
	return PeerDTO{
		Name:      p.Name,
		PublicKey: p.PublicKey,
		Route:     p.Route,
		Admin:     p.Admin,
		Enabled:   p.Enabled,
	}
}

func PeerDTOsFromService(
	peers []*service.Peer,
) []PeerDTO {
	if peers == nil {
		return []PeerDTO{}
	}
	result := make([]PeerDTO, len(peers))
	for i, p := range peers {
		result[i] = PeerDTOFromService(*p)
	}
	return result
}

func (a *API) handlePeerList(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	peers, err := a.service.ListPeers(network)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, PeerDTOsFromService(peers))
}

func (a *API) handleInviteCreate(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")

	var req CreateInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	var ip *net.IP
	if req.Ip != nil && *req.Ip != "" {
		parsed := net.ParseIP(*req.Ip)
		if parsed == nil {
			wire.WriteError(w, http.StatusBadRequest, "invalid IP address")
			return
		}
		ip = &parsed
	}

	invitation, err := a.service.CreateRegistration(network, req.Name, ip, req.Admin, nil)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusCreated, invitation)
}

func (a *API) handlePeerRename(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	peer := r.PathValue("peer")

	var req RenamePeerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}

	if _, err := a.service.UpdatePeer(network, peer, &req.Name, nil, nil, nil); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (a *API) handlePeerDelete(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	peer := r.PathValue("peer")

	if err := a.service.RemovePeer(network, peer); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (a *API) handlePeerEnable(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	peer := r.PathValue("peer")

	if err := a.service.EnablePeer(network, peer); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (a *API) handlePeerDisable(
	w http.ResponseWriter,
	r *http.Request,
) {
	network := r.PathValue("name")
	peer := r.PathValue("peer")

	if err := a.service.DisablePeer(network, peer); err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, nil)
}

func (c *Client) ListPeers(
	ctx context.Context,
	network string,
) (
	[]PeerDTO,
	error,
) {
	var result []PeerDTO
	return result, c.wire.Get(ctx, "/networks/"+network+"/peers", &result)
}

func (c *Client) CreateInvite(
	ctx context.Context,
	network string,
	name string,
	ip *string,
	admin bool,
) (
	*protocol.Invitation,
	error,
) {
	req := CreateInviteRequest{Name: name, Ip: ip, Admin: admin}
	body, err := marshalJSON(req)
	if err != nil {
		return nil, err
	}
	var result *protocol.Invitation
	return result, c.wire.Post(ctx, "/networks/"+network+"/registrations", body, &result)
}

func (c *Client) RenamePeer(
	ctx context.Context,
	network string,
	peer string,
	newName string,
) error {
	req := RenamePeerRequest{Name: newName}
	body, err := marshalJSON(req)
	if err != nil {
		return err
	}
	return c.wire.Patch(ctx, "/networks/"+network+"/peers/"+peer, body, nil)
}

func (c *Client) DeletePeer(
	ctx context.Context,
	network string,
	peer string,
) error {
	return c.wire.Delete(ctx, "/networks/"+network+"/peers/"+peer, nil)
}

func (c *Client) EnablePeer(
	ctx context.Context,
	network string,
	peer string,
) error {
	return c.wire.Post(ctx, "/networks/"+network+"/peers/"+peer+"/enable", nil, nil)
}

func (c *Client) DisablePeer(
	ctx context.Context,
	network string,
	peer string,
) error {
	return c.wire.Post(ctx, "/networks/"+network+"/peers/"+peer+"/disable", nil, nil)
}
