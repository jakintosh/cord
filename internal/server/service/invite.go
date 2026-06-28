package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

// External Invite types — these travel over the wire in JSON form.

// ServerInfo describes how to reach the coordination server. It travels
// in invite payloads and is stored in client-side network config.
type ServerInfo struct {
	PublicKey        string `json:"public_key"`
	ExternalEndpoint string `json:"external_endpoint"`
	InternalEndpoint string `json:"internal_endpoint"`
}

// InviteInterface describes the temporary WireGuard identity given to
// an invitee for connecting to the invite network.
type InviteInterface struct {
	NetworkName  string `json:"network_name"`
	PrivateKey   string `json:"private_key"`
	AssignedCidr string `json:"assigned_cidr"`
}

// PeerInvite is the opaque JSON payload delivered out-of-band to a
// prospective peer. It contains everything the client needs to connect
// to the invite network and redeem a permanent identity.
type PeerInvite struct {
	Interface InviteInterface `json:"interface"`
	Server    ServerInfo      `json:"server"`
}

// ParseInvite reads and validates a PeerInvite from a JSON reader.
func ParseInvite(
	r io.Reader,
) (
	*PeerInvite,
	error,
) {
	var inv PeerInvite
	if err := json.NewDecoder(r).Decode(&inv); err != nil {
		return nil, fmt.Errorf("%w: parse invite: %v", ErrInvalidInput, err)
	}
	return &inv, nil
}

// Write serializes the invite as JSON to the writer.
func (inv *PeerInvite) Write(
	w io.Writer,
) error {
	if err := json.NewEncoder(w).Encode(inv); err != nil {
		return fmt.Errorf("write invite: %w", err)
	}
	return nil
}

// Domain Invite types — these represent the server-side stored state.

// Invite is the server-side stored representation of an invite. It
// tracks the temporary key, assigned IPs, and redemption state.
type Invite struct {
	Name        string
	TempPubKey  string    // the temporary public key the invitee uses to redeem
	TempIP      net.IP    // the temporary IP on the invite network
	FinalIP     net.IP    // the permanent IP on the main network
	Admin       bool      // whether the invite grants admin privileges
	Redeemed    bool      // whether the invite has been redeemed
	RedeemedKey string    // the permanent public key after redemption
	ExpiresAt   time.Time // when the invite expires
	CreatedAt   time.Time // when the invite was created
}

// CreateInviteRequest is the input for generating a peer invite.
type CreateInviteRequest struct {
	Name      string        // the name for the new peer
	IP        string        // permanent IP; auto-assigned from root CIDR if empty
	Admin     bool          // whether the peer has admin privileges
	ExpiresIn time.Duration // zero means default 24-hour expiration
}

// RedeemResult is returned to a peer after successful invite
// redemption. It contains the permanent network identity.
type RedeemResult struct {
	NetworkName  string     `json:"network_name"`
	AssignedCidr string     `json:"assigned_cidr"`
	Server       ServerInfo `json:"server"`
}

// endpointTTL is how long an endpoint sighting is considered current.
const endpointTTL = 24 * time.Hour

// ResolveInviteIdentity looks up an unredeemed, unexpired invite by
// temporary IP within the invite network. Used by the identity middleware
// to authenticate incoming invite-redemption requests.
func (s *Service) ResolveInviteIdentity(network string, ip net.IP) (*Invite, error) {
	inv, err := s.store.GetInviteByIP(network, ip, s.clock())
	if err != nil {
		return nil, fmt.Errorf("resolve invite identity: %w", mapStoreError(err))
	}
	return inv, nil
}

// CreateInvite reserves an IP on the main network, allocates a
// temporary IP on the invite network, generates a temporary keypair,
// persists the invite record, and returns the PeerInvite payload to
// deliver to the invitee out-of-band.
func (s *Service) CreateInvite(
	network string,
	req CreateInviteRequest,
) (
	*PeerInvite,
	error,
) {
	nw, err := s.store.GetNetwork(network)
	if err != nil {
		return nil, fmt.Errorf("get network: %w", mapStoreError(err))
	}

	if req.Name == "" {
		return nil, fmt.Errorf("%w: invite name required", ErrInvalidInput)
	}

	var finalIP net.IP
	if req.IP != "" {
		ip := net.ParseIP(req.IP)
		if ip == nil {
			// Try parsing as CIDR (e.g. from AddPeer which passes "10.0.0.5/16")
			parsed, _, err := net.ParseCIDR(req.IP)
			if err != nil {
				return nil, fmt.Errorf("%w: invalid permanent IP %q", ErrInvalidInput, req.IP)
			}
			ip = parsed
		}
		finalIP = normalizeIP(ip)
	} else {
		freeIP, err := s.nextFreePeerIP(network, nw.MainCidr)
		if err != nil {
			return nil, fmt.Errorf("auto-assign permanent IP: %w", err)
		}
		ip, _, _ := net.ParseCIDR(freeIP)
		finalIP = normalizeIP(ip)
	}

	tempPrivKey, err := s.wg.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("generate temp key: %w", err)
	}

	tempPubKey, err := s.wg.PublicKey(tempPrivKey)
	if err != nil {
		return nil, fmt.Errorf("derive temp public key: %w", err)
	}

	tempIP, err := s.nextFreeInviteIP(network, nw.InviteCidr)
	if err != nil {
		return nil, fmt.Errorf("allocate invite IP: %w", err)
	}

	if req.ExpiresIn == 0 {
		req.ExpiresIn = 24 * time.Hour
	}

	now := s.clock()
	invite := &Invite{
		Name:       req.Name,
		TempPubKey: tempPubKey,
		TempIP:     tempIP,
		FinalIP:    finalIP,
		Admin:      req.Admin,
		ExpiresAt:  now.Add(req.ExpiresIn),
		CreatedAt:  now,
	}

	if err := s.store.InsertInvite(network, invite); err != nil {
		return nil, fmt.Errorf("insert invite: %w", mapStoreError(err))
	}
	s.reconcileOnce(network)

	_, inviteNet, _ := net.ParseCIDR(nw.InviteCidr)
	prefix, _ := inviteNet.Mask.Size()

	payload := &PeerInvite{
		Interface: InviteInterface{
			NetworkName:  nw.Name,
			PrivateKey:   tempPrivKey,
			AssignedCidr: fmt.Sprintf("%s/%d", tempIP.String(), prefix),
		},
		Server: ServerInfo{
			PublicKey:        nw.PublicKey,
			ExternalEndpoint: fmt.Sprintf("%s:%d", nw.ExternalIP, nw.InviteListenPort),
			InternalEndpoint: fmt.Sprintf("%s:%d", firstAssignableIP(inviteNet).String(), nw.ApiPort),
		},
	}

	return payload, nil
}

// RedeemInvite exchanges a temporary invite key for a permanent peer
// registration. Idempotent: redeeming an already-redeemed invite with
// the same permanent key returns the same result.
func (s *Service) RedeemInvite(
	network string,
	tempPubKey string,
	permPubKey string,
) (
	*RedeemResult,
	error,
) {
	nw, err := s.store.GetNetwork(network)
	if err != nil {
		return nil, fmt.Errorf("get network: %w", mapStoreError(err))
	}

	err = s.store.RedeemInvite(network, tempPubKey, permPubKey, s.clock())
	if err != nil {
		peer, lookupErr := s.store.GetPeerByKey(network, permPubKey)
		if lookupErr == nil && !peer.Confirmed {
			s.reconcileOnce(network)
			return s.buildRedeemResult(nw, peer)
		}
		return nil, fmt.Errorf("redeem invite: %w", mapStoreError(err))
	}
	s.reconcileOnce(network)

	peer, err := s.store.GetPeerByKey(network, permPubKey)
	if err != nil {
		return nil, fmt.Errorf("get redeemed peer: %w", mapStoreError(err))
	}

	return s.buildRedeemResult(nw, peer)
}

// ListInvites returns all invites for the given network (active,
// expired, and redeemed).
func (s *Service) ListInvites(
	network string,
) (
	[]*Invite,
	error,
) {
	invites, err := s.store.ListInvites(network)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}
	return invites, nil
}

// RevokeInvite deletes an invite by name, preventing it from being
// redeemed. Any associated temporary key and IP are released.
func (s *Service) RevokeInvite(
	network string,
	name string,
) error {
	if err := s.store.DeleteInvite(network, name); err != nil {
		return fmt.Errorf("delete invite %q: %w", name, mapStoreError(err))
	}
	s.reconcileOnce(network)
	return nil
}

// buildRedeemResult constructs the RedeemResult from a network and
// redeemed peer.
func (s *Service) buildRedeemResult(
	nw *Network,
	peer *Peer,
) (
	*RedeemResult,
	error,
) {
	_, rootNet, _ := net.ParseCIDR(nw.MainCidr)
	prefix, _ := rootNet.Mask.Size()

	peerIP, _, _ := net.ParseCIDR(peer.Cidr)

	return &RedeemResult{
		NetworkName:  nw.Name,
		AssignedCidr: fmt.Sprintf("%s/%d", peerIP.String(), prefix),
		Server: ServerInfo{
			PublicKey:        nw.PublicKey,
			ExternalEndpoint: fmt.Sprintf("%s:%d", nw.ExternalIP, nw.ListenPort),
			InternalEndpoint: fmt.Sprintf("%s:%d", firstAssignableIP(rootNet).String(), nw.ApiPort),
		},
	}, nil
}

// nextFreeInviteIP finds the lowest free address on the invite network,
// skipping the network address and the server's invite address.
func (s *Service) nextFreeInviteIP(
	network string,
	inviteCidr string,
) (
	net.IP,
	error,
) {
	_, ipNet, err := net.ParseCIDR(inviteCidr)
	if err != nil {
		return nil, fmt.Errorf("parse invite CIDR: %w", err)
	}

	invites, err := s.store.ListInvites(network)
	if err != nil {
		return nil, fmt.Errorf("list invites: %w", err)
	}

	used := map[string]bool{}
	for _, inv := range invites {
		if inv.TempIP != nil {
			used[normalizeIP(inv.TempIP).String()] = true
		}
	}

	first := firstAssignableIP(ipNet)
	_, last := cidrRange(ipNet)

	candidate := incrementIP(first)
	for ipNet.Contains(candidate) && !candidate.Equal(last) {
		if !used[normalizeIP(candidate).String()] {
			return normalizeIP(candidate), nil
		}
		candidate = incrementIP(candidate)
	}

	return nil, fmt.Errorf("%w: no free addresses in invite CIDR %s", ErrInvalidInput, inviteCidr)
}
