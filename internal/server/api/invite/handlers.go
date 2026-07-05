package invite

import (
	"encoding/json"
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
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

	var req protocol.RedeemRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	// The redeem response IS an Invitation with the PrivateKey omitted;
	// the service returns it with PrivateKey empty and the field is
	// omitempty, so it can be written straight to the wire.
	invitation, err := a.service.RedeemRegistration(a.network, peer.PublicKey, req.PermPubKey)
	if err != nil {
		wire.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	wire.WriteData(w, http.StatusOK, invitation)
}
