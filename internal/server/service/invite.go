package service

import (
	"io"
	"net"
	"time"
)

// External Invite

type ServerInfo struct {
	PublicKey        string `json:"public_key"`
	ExternalEndpoint string `json:"external_endpoint"`
	InternalEndpoint string `json:"internal_endpoint"`
}

type InviteInterface struct {
	NetworkName  string `json:"network_name"`
	PrivateKey   string `json:"private_key"`
	AssignedCidr string `json:"assigned_cidr"`
}

type PeerInvite struct {
	Interface InviteInterface `json:"interface"`
	Server    ServerInfo      `json:"server"`
}

func ParseInvite(
	r io.Reader,
) (
	*PeerInvite,
	error,
) {
	return nil, ErrNotImplemented
}

func (inv *PeerInvite) Write(
	w io.Writer,
) error {
	return ErrNotImplemented
}

// Domain Invite

type Invite struct {
	Name        string
	TempPubKey  string
	TempIP      net.IP
	FinalIP     net.IP
	Admin       bool
	Redeemed    bool
	RedeemedKey string
	ExpiresAt   time.Time
	CreatedAt   time.Time
}

type CreateInviteRequest struct {
	Name      string
	IP        string
	Admin     bool
	ExpiresIn time.Duration
}

type RedeemResult struct {
	NetworkName  string     `json:"network_name"`
	AssignedCidr string     `json:"assigned_cidr"`
	Server       ServerInfo `json:"server"`
}

func (s *Service) CreateInvite(
	network string,
	req CreateInviteRequest,
) (
	*PeerInvite,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) RedeemInvite(
	network string,
	tempPubKey string,
	permPubKey string,
) (
	*RedeemResult,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) ListInvites(
	network string,
) (
	[]*Invite,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) RevokeInvite(
	network string,
	name string,
) error {
	return ErrNotImplemented
}
