package peer

import (
	"encoding/json"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/identity"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (a *API) handleVisiblePeers(
	w http.ResponseWriter,
	r *http.Request,
) {
	caller, err := identity.Resolve(r, a.resolver)
	if err != nil {
		wire.WriteError(w, http.StatusForbidden, "identity unknown")
		return
	}

	peers, err := a.service.ListVisiblePeers(a.network, caller.Name)
	if err != nil {
		wire.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	dtos := make([]VisiblePeerDTO, len(peers))
	for i, p := range peers {
		dtos[i] = toVisiblePeerDTO(p)
	}

	wire.WriteData(w, http.StatusOK, dtos)
}

func (a *API) handleReportEndpoints(
	w http.ResponseWriter,
	r *http.Request,
) {
	caller, err := identity.Resolve(r, a.resolver)
	if err != nil {
		wire.WriteError(w, http.StatusForbidden, "identity unknown")
		return
	}

	var sightings []EndpointSightingDTO
	if err := json.NewDecoder(r.Body).Decode(&sightings); err != nil {
		wire.WriteError(w, http.StatusBadRequest, "invalid JSON")
		return
	}

	svcSightings := make([]service.EndpointSighting, len(sightings))
	for i, s := range sightings {
		svcSightings[i] = service.EndpointSighting{
			WitnessKey: caller.PublicKey,
			PeerKey:    s.PeerKey,
			Endpoint:   s.Endpoint,
			Timestamp:  time.Now(),
		}
	}

	if err := a.service.ReportEndpoints(a.network, svcSightings); err != nil {
		wire.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	wire.WriteData(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleConfirmPeer(
	w http.ResponseWriter,
	r *http.Request,
) {
	caller, err := identity.ResolveProvisional(r, a)
	if err != nil {
		wire.WriteError(w, http.StatusForbidden, "identity unknown")
		return
	}

	if err := a.service.ConfirmPeer(a.network, caller.Name); err != nil {
		wire.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}

	wire.WriteData(w, http.StatusOK, map[string]string{"status": "confirmed"})
}
