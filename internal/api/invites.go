package api

import (
	"net/http"

	"git.sr.ht/~jakintosh/command-go/pkg/wire"

	"git.sr.ht/~jakintosh/cord/internal/server"
)

// KeyRequest carries a peer's permanent public key. WireGuard keys are
// base64 (and may contain '/'), so they travel in the body, not the path.
type KeyRequest struct {
	PublicKey string `json:"publicKey"`
}

// ServerInfoDTO describes how a peer reaches the coordination server.
type ServerInfoDTO struct {
	PublicKey        string `json:"publicKey"`
	ExternalEndpoint string `json:"externalEndpoint"`
	InternalEndpoint string `json:"internalEndpoint"`
}

// PeerInviteDTO is the invite payload returned by the admin API; it is
// the JSON shape of an invite file's contents.
type PeerInviteDTO struct {
	Interface InviteInterfaceDTO `json:"interface"`
	Server    ServerInfoDTO      `json:"server"`
}

// InviteInterfaceDTO is the temporary identity an invitee uses on the
// invite network.
type InviteInterfaceDTO struct {
	NetworkName  string `json:"networkName"`
	PrivateKey   string `json:"privateKey"`
	AssignedCidr string `json:"assignedCidr"`
}

// RedeemResultDTO is what a redeeming peer receives: its assignment on
// the main network and how to reach the server there.
type RedeemResultDTO struct {
	NetworkName  string        `json:"networkName"`
	AssignedCidr string        `json:"assignedCidr"`
	Server       ServerInfoDTO `json:"server"`
}

func PeerInviteDTOFromServer(
	i server.PeerInvite,
) PeerInviteDTO {
	return PeerInviteDTO{
		Interface: InviteInterfaceDTO(i.Interface),
		Server:    ServerInfoDTO(i.Server),
	}
}

func (i PeerInviteDTO) ToServer() server.PeerInvite {
	return server.PeerInvite{
		Interface: server.InviteInterface(i.Interface),
		Server:    server.ServerInfo(i.Server),
	}
}

func RedeemResultDTOFromServer(
	r server.RedeemResult,
) RedeemResultDTO {
	return RedeemResultDTO{
		NetworkName:  r.NetworkName,
		AssignedCidr: r.AssignedCidr,
		Server:       ServerInfoDTO(r.Server),
	}
}

func (r RedeemResultDTO) ToServer() server.RedeemResult {
	return server.RedeemResult{
		NetworkName:  r.NetworkName,
		AssignedCidr: r.AssignedCidr,
		Server:       server.ServerInfo(r.Server),
	}
}

// handleRedeemInvite trades the caller's invite for a permanent peer
// registration. The body carries the permanent public key the client
// generated; the invite itself is identified by the caller's source IP
// on the invite network.
func (a *API) handleRedeemInvite(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req KeyRequest
	if err := decodeJSON(r, &req); err != nil || req.PublicKey == "" {
		wire.WriteError(w, http.StatusBadRequest, "publicKey is required")
		return
	}

	invite, ok := authedInvite(r)
	if !ok {
		wire.WriteError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	result, err := a.service.RedeemInvite(invite, req.PublicKey)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	a.mutated()
	wire.WriteData(w, http.StatusOK, RedeemResultDTOFromServer(*result))
}

// handleConfirmInvite finalizes redemption. It is called over the main
// network from the peer's assigned IP; the body must carry the peer's
// permanent public key. Auth is intentionally looser than withMainAuth:
// the peer is not confirmed yet.
func (a *API) handleConfirmInvite(
	w http.ResponseWriter,
	r *http.Request,
) {
	var req KeyRequest
	if err := decodeJSON(r, &req); err != nil || req.PublicKey == "" {
		wire.WriteError(w, http.StatusBadRequest, "publicKey is required")
		return
	}

	ip := clientIP(r)
	if ip == nil {
		wire.WriteError(w, http.StatusBadRequest, "unable to determine client IP")
		return
	}

	if err := a.service.ConfirmPeer(req.PublicKey, ip); err != nil {
		writeServiceError(w, err)
		return
	}

	a.mutated()
	wire.WriteData(w, http.StatusOK, nil)
}
