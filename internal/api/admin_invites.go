package api

import (
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

// InviteDTO is the admin view of an invite. The temporary invite key
// and invite-network address are deliberately omitted.
type InviteDTO struct {
	Name        string `json:"name"`
	NetworkCidr string `json:"networkCidr"`
	Admin       bool   `json:"admin"`
	Redeemed    bool   `json:"redeemed"`
	Expiration  int64  `json:"expiration"`
}

func InviteDTOFromServer(
	i server.InviteStatus,
) InviteDTO {
	return InviteDTO{
		Name:        i.Name,
		NetworkCidr: i.NetworkCidr,
		Admin:       i.Admin,
		Redeemed:    i.Redeemed,
		Expiration:  i.Expiration.Unix(),
	}
}

func (i InviteDTO) ToServer() server.InviteStatus {
	return server.InviteStatus{
		Name:        i.Name,
		NetworkCidr: i.NetworkCidr,
		Admin:       i.Admin,
		Redeemed:    i.Redeemed,
		Expiration:  time.Unix(i.Expiration, 0),
	}
}

func (a *API) addAdminInviteRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /invites", a.handleListInvites)
}

func (a *API) handleListInvites(
	w http.ResponseWriter,
	r *http.Request,
) {
	invites, err := a.service.ListInvites()
	if err != nil {
		writeServiceError(w, err)
		return
	}

	dtos := make([]InviteDTO, 0, len(invites))
	for _, invite := range invites {
		dtos = append(dtos, InviteDTOFromServer(invite))
	}

	wire.WriteData(w, http.StatusOK, dtos)
}
