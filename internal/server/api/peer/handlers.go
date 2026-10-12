package peer

import (
	"encoding/json"
	"net/http"
	"time"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/server/api/identity"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func (a *API) handleVisibleSnapshot(
	network string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller := identity.Caller(r.Context())

		if snapshot, err := a.runtime.GetVisibleNetworkSnapshot(
			r.Context(),
			network,
			caller.Name,
		); err != nil {
			a.writeServiceError(w, r, network, "snapshot", err)
		} else {
			snapshotDTO := toVisibleNetworkSnapshot(snapshot)
			wire.WriteData(w, http.StatusOK, snapshotDTO)
		}
	}
}

func (a *API) handleReportEndpoints(
	network string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller := identity.Caller(r.Context())

		var protocolSightings []protocol.EndpointSighting
		if err := json.NewDecoder(r.Body).Decode(&protocolSightings); err != nil {
			wire.WriteError(w, http.StatusBadRequest, "invalid JSON")
			return
		}

		// Witness attribution is server-internal: the caller reports which
		// peer it saw and where, and the server stamps its own identity
		// (resolved from the tunnel source IP) and a receipt timestamp.
		svcSightings := serviceSightingsFromProtocol(
			caller.PublicKey,
			protocolSightings,
			time.Now(),
		)

		if err := a.runtime.ReportEndpoints(
			r.Context(),
			network,
			svcSightings,
		); err != nil {
			a.writeServiceError(w, r, network, "report endpoints", err)
		} else {
			wire.WriteData(w, http.StatusOK, protocol.StatusResponse{
				Status: "ok",
			})
		}
	}
}

func (a *API) handleConfirmPeer(
	network string,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		caller := identity.Caller(r.Context())

		if err := a.runtime.ConfirmPeer(
			r.Context(),
			network,
			caller.Name,
		); err != nil {
			a.writeServiceError(w, r, network, "confirm peer", err)
		} else {
			wire.WriteData(w, http.StatusOK, protocol.StatusResponse{
				Status: "confirmed",
			})
		}
	}
}

// serviceSightingsFromProtocol converts wire endpoint sightings reported by
// one peer into domain sightings. Witness attribution is server-internal:
// the witness is the authenticated caller, and the server stamps the
// receipt time.
func serviceSightingsFromProtocol(
	witnessKey string,
	sightings []protocol.EndpointSighting,
	receivedAt time.Time,
) []service.EndpointSighting {
	svcSightings := make([]service.EndpointSighting, len(sightings))
	for i, s := range sightings {
		svcSightings[i] = service.EndpointSighting{
			WitnessKey: witnessKey,
			PeerKey:    s.PeerKey,
			Endpoint:   s.Endpoint,
			Timestamp:  receivedAt,
		}
	}
	return svcSightings
}
