package invite

import (
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/identity"
)

func (a *API) handleRedeemInvite(
	w http.ResponseWriter,
	r *http.Request,
) {
	_, err := identity.Resolve(r, a.resolver)
	if err != nil {
		wire.WriteError(w, http.StatusForbidden, "identity unknown")
		return
	}

	var req RedeemInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	result, err := a.service.RedeemInvite(a.network, req.TempPubKey, req.PermPubKey)
	if err != nil {
		wire.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := RedeemResultDTO{
		NetworkName:  result.NetworkName,
		AssignedCidr: result.AssignedCidr,
		Server: ServerInfoDTO{
			PublicKey:        result.Server.PublicKey,
			ExternalEndpoint: result.Server.ExternalEndpoint,
			InternalEndpoint: result.Server.InternalEndpoint,
		},
	}

	wire.WriteData(w, http.StatusOK, response)
}
