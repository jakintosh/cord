package api

import (
	"net"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/server"
	"git.sr.ht/~jakintosh/cord/internal/utils"
)

// PeerDTO is the admin view of a peer record.
type PeerDTO struct {
	Name      string `json:"name"`
	Cidr      string `json:"cidr"`
	PublicKey string `json:"publicKey"`
	Admin     bool   `json:"admin"`
	Enabled   bool   `json:"enabled"`
	Confirmed bool   `json:"confirmed"`
}

// CreatePeerRequest asks the server to mint an invite for a new peer.
// ExpiresIn is in seconds; zero means the server default (24h).
type CreatePeerRequest struct {
	Name      string `json:"name"`
	IP        string `json:"ip"`
	Admin     bool   `json:"admin"`
	ExpiresIn int64  `json:"expiresIn,omitempty"`
}

// UpdatePeerRequest renames, enables/disables, or grants/revokes admin
// on a peer. All fields are optional.
type UpdatePeerRequest struct {
	Name    *string `json:"name,omitempty"`
	Admin   *bool   `json:"admin,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

func PeerDTOFromServer(
	p server.Peer,
) PeerDTO {
	return PeerDTO(p)
}

func (p PeerDTO) ToServer() server.Peer {
	return server.Peer(p)
}

func (r CreatePeerRequest) ToServer() (
	server.CreateInviteRequest,
	error,
) {
	ip := net.ParseIP(r.IP)
	if ip == nil {
		return server.CreateInviteRequest{}, server.ErrInvalid
	}

	req := server.CreateInviteRequest{
		Name:  r.Name,
		IP:    utils.NormalizeIP(ip),
		Admin: r.Admin,
	}
	if r.ExpiresIn > 0 {
		req.Expiration = time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)
	}
	return req, nil
}

func (r UpdatePeerRequest) ToServer() server.UpdatePeerRequest {
	return server.UpdatePeerRequest(r)
}

func (a *API) addAdminPeerRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /peers", a.handleListAdminPeers)
	mux.HandleFunc("POST /peer", a.handleCreatePeer)
	mux.HandleFunc("GET /peer/{name}", a.handleGetPeer)
	mux.HandleFunc("PATCH /peer/{name}", a.handleUpdatePeer)
	mux.HandleFunc("DELETE /peer/{name}", a.handleDeletePeer)
}

// handleCreatePeer mints a peer invite and returns the invite payload
// so a remote admin can deliver it out-of-band.
func (a *API) handleCreatePeer(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req CreatePeerRequest
	if err := decodeJSON(r, &req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "malformed json")
		return
	}
	if req.Name == "" || req.IP == "" {
		wire.WriteError(w, http.StatusBadRequest, "name and ip are required")
		return
	}

	inviteReq, err := req.ToServer()
	if err != nil {
		wire.WriteError(w, http.StatusBadRequest, "invalid ip")
		return
	}

	invite, err := a.service.CreateInvite(inviteReq)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	a.mutated()
	wire.WriteData(w, http.StatusCreated, PeerInviteDTOFromServer(*invite))
}

func (a *API) handleListAdminPeers(
	w http.ResponseWriter,
	r *http.Request,
) {
	peers, err := a.service.ListPeers()
	if err != nil {
		writeServiceError(w, err)
		return
	}

	dtos := make([]PeerDTO, 0, len(peers))
	for _, peer := range peers {
		dtos = append(dtos, PeerDTOFromServer(*peer))
	}

	wire.WriteData(w, http.StatusOK, dtos)
}

func (a *API) handleGetPeer(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	peer, err := a.service.GetPeer(name)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	wire.WriteData(w, http.StatusOK, PeerDTOFromServer(*peer))
}

func (a *API) handleUpdatePeer(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	var req UpdatePeerRequest
	if err := decodeJSON(r, &req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "malformed json")
		return
	}

	peer, err := a.service.UpdatePeer(name, req.ToServer())
	if err != nil {
		writeServiceError(w, err)
		return
	}

	a.mutated()
	wire.WriteData(w, http.StatusOK, PeerDTOFromServer(*peer))
}

func (a *API) handleDeletePeer(
	w http.ResponseWriter,
	r *http.Request,
) {
	name := r.PathValue("name")

	if err := a.service.DeletePeer(name); err != nil {
		writeServiceError(w, err)
		return
	}

	a.mutated()
	wire.WriteData(w, http.StatusNoContent, nil)
}
