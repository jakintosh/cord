package server

import (
	"time"
)

// NetworkSummary is the one-line listing view of a network, built from
// its config file alone (no database access).
type NetworkSummary struct {
	Name             string `json:"name"`
	RootCidr         string `json:"rootCidr"`
	ExternalEndpoint string `json:"externalEndpoint"`
}

// NetworkOverview is the detail view of a network: its public config
// fields plus counts of the resources it holds.
type NetworkOverview struct {
	Name             string `json:"name"`
	PublicKey        string `json:"publicKey"`
	RootCidr         string `json:"rootCidr"`
	InviteCidr       string `json:"inviteCidr"`
	ExternalIP       string `json:"externalIp"`
	ListenPort       uint16 `json:"listenPort"`
	InviteListenPort uint16 `json:"inviteListenPort"`
	ApiPort          uint16 `json:"apiPort"`
	CidrCount        int    `json:"cidrCount"`
	PeerCount        int    `json:"peerCount"`
	ActiveInvites    int    `json:"activeInvites"`
	AssociationCount int    `json:"associationCount"`
}

// CidrDetail is a CIDR along with the names of its associated CIDRs.
type CidrDetail struct {
	Name         string   `json:"name"`
	Cidr         string   `json:"cidr"`
	Associations []string `json:"associations"`
}

// PeerStatus is the admin listing view of a peer: its flags plus the
// most recently witnessed endpoint, if any sighting is still current.
type PeerStatus struct {
	Name         string `json:"name"`
	Cidr         string `json:"cidr"`
	PublicKey    string `json:"publicKey"`
	Admin        bool   `json:"admin"`
	Enabled      bool   `json:"enabled"`
	Confirmed    bool   `json:"confirmed"`
	LastEndpoint string `json:"lastEndpoint,omitempty"`
	LastSeen     int64  `json:"lastSeen,omitempty"`
}

// InviteStatus is the admin listing view of an invite. The temporary
// invite key and invite-network address are deliberately omitted.
type InviteStatus struct {
	Name        string    `json:"name"`
	NetworkCidr string    `json:"networkCidr"`
	Admin       bool      `json:"admin"`
	Redeemed    bool      `json:"redeemed"`
	Expiration  time.Time `json:"expiration"`
}

// ListNetworks summarizes every network with a config in the store.
func ListNetworks(cfg Config) ([]NetworkSummary, error) {
	names, err := cfg.ListConfigs()
	if err != nil {
		return nil, err
	}

	summaries := make([]NetworkSummary, 0, len(names))
	for _, name := range names {
		netCfg, err := LoadNetwork(cfg, name)
		if err != nil {
			// a stray or malformed toml shouldn't break the listing
			continue
		}
		summaries = append(summaries, NetworkSummary{
			Name:             netCfg.Name,
			RootCidr:         netCfg.RootCidr,
			ExternalEndpoint: netCfg.ExternalEndpoint(),
		})
	}
	return summaries, nil
}

// GetNetworkOverview reports the network's public config and resource counts.
func (srv *Server) GetNetworkOverview() (*NetworkOverview, error) {
	cfg, err := srv.LoadNetwork()
	if err != nil {
		return nil, err
	}

	cidrs, err := srv.Store.CidrList()
	if err != nil {
		return nil, err
	}
	peers, err := srv.Store.PeerList()
	if err != nil {
		return nil, err
	}
	invites, err := srv.Store.InviteListActive()
	if err != nil {
		return nil, err
	}
	associations, err := srv.Store.AssociationList()
	if err != nil {
		return nil, err
	}

	return &NetworkOverview{
		Name:             cfg.Name,
		PublicKey:        cfg.PublicKey,
		RootCidr:         cfg.RootCidr,
		InviteCidr:       cfg.InviteCidr,
		ExternalIP:       cfg.ExternalIP,
		ListenPort:       cfg.ListenPort,
		InviteListenPort: cfg.InviteListenPort,
		ApiPort:          cfg.ApiPort,
		CidrCount:        len(cidrs),
		PeerCount:        len(peers),
		ActiveInvites:    len(invites),
		AssociationCount: len(associations),
	}, nil
}

// ListCidrDetails returns every CIDR with the names of its associations.
func (srv *Server) ListCidrDetails() ([]CidrDetail, error) {
	cidrs, err := srv.Store.CidrList()
	if err != nil {
		return nil, err
	}
	associations, err := srv.Store.AssociationList()
	if err != nil {
		return nil, err
	}

	partners := map[string][]string{}
	for _, a := range associations {
		partners[a.Cidr1] = append(partners[a.Cidr1], a.Cidr2)
		partners[a.Cidr2] = append(partners[a.Cidr2], a.Cidr1)
	}

	details := make([]CidrDetail, 0, len(cidrs))
	for _, cidr := range cidrs {
		associated := partners[cidr.Name]
		if associated == nil {
			associated = []string{}
		}
		details = append(details, CidrDetail{
			Name:         cidr.Name,
			Cidr:         cidr.Cidr,
			Associations: associated,
		})
	}
	return details, nil
}

// ListPeerStatuses returns every peer with its flags and the most
// recently witnessed endpoint still inside the sighting TTL.
func (srv *Server) ListPeerStatuses() ([]PeerStatus, error) {
	peers, err := srv.Store.PeerList()
	if err != nil {
		return nil, err
	}

	since := time.Now().Add(-endpointTTL).Unix()
	endpoints, err := srv.Store.EndpointsRecent(since)
	if err != nil {
		return nil, err
	}

	statuses := make([]PeerStatus, 0, len(peers))
	for _, peer := range peers {
		status := PeerStatus{
			Name:      peer.Name,
			Cidr:      peer.Cidr,
			PublicKey: peer.PublicKey,
			Admin:     peer.Admin,
			Enabled:   peer.Enabled,
			Confirmed: peer.Confirmed,
		}
		// witnesses arrive newest first
		if witnesses := endpoints[peer.PublicKey]; len(witnesses) > 0 {
			status.LastEndpoint = witnesses[0].Endpoint
			status.LastSeen = witnesses[0].Timestamp
		}
		statuses = append(statuses, status)
	}
	return statuses, nil
}

// ListInvites returns every invite the network still holds; invites are
// deleted on peer confirmation, so confirmed peers never appear.
func (srv *Server) ListInvites() ([]InviteStatus, error) {
	invites, err := srv.Store.InviteList()
	if err != nil {
		return nil, err
	}

	statuses := make([]InviteStatus, 0, len(invites))
	for _, invite := range invites {
		statuses = append(statuses, InviteStatus{
			Name:        invite.Name,
			NetworkCidr: invite.NetworkCidr,
			Admin:       invite.Admin,
			Redeemed:    invite.Redeemed,
			Expiration:  invite.Expiration,
		})
	}
	return statuses, nil
}
