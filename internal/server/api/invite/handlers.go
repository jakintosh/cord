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
	peer, err := identity.Resolve(r, a.resolver)
	if err != nil {
		wire.WriteError(w, http.StatusForbidden, "identity unknown")
		return
	}

	var req RedeemInviteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	result, err := a.service.RedeemRegistration(a.network, peer.PublicKey, req.PermPubKey)
	if err != nil {
		wire.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := InvitationDTO{
		Network: NetworkInfoDTO{
			Name:        result.Network.Name,
			PublicKey:   result.Network.PublicKey,
			Endpoint:    result.Network.Endpoint,
			APIEndpoint: result.Network.APIEndpoint,
		},
		Peer: PeerIdentityDTO{
			CIDR: result.Peer.CIDR,
		},
	}

	wire.WriteData(w, http.StatusOK, response)
}
