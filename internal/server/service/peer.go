package service

import (
	"net"
	"time"
)

type Peer struct {
	Name      string
	Cidr      string
	PublicKey string
	Admin     bool
	Enabled   bool
	Confirmed bool
}

type PeerConfig struct {
	Name  string
	IP    string
	Admin bool
}

type UpdatePeerRequest struct {
	Name    *string
	Admin   *bool
	Enabled *bool
}

type VisiblePeer struct {
	Name      string
	Cidr      string
	PublicKey string
	Endpoints []EndpointWitness
}

type EndpointSighting struct {
	PeerKey   string
	Endpoint  string
	Timestamp time.Time
}

type EndpointWitness struct {
	Witness   string
	Endpoint  string
	Timestamp time.Time
}

func (s *Service) GetPeer(
	network string,
	name string,
) (
	*Peer,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) ListPeers(
	network string,
) (
	[]*Peer,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) ListVisiblePeers(
	network string,
	peerName string,
) (
	[]*VisiblePeer,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) AddPeer(
	network string,
	cfg PeerConfig,
) (
	*PeerInvite,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) RemovePeer(
	network string,
	name string,
) error {
	return ErrNotImplemented
}

func (s *Service) UpdatePeer(
	network string,
	name string,
	req UpdatePeerRequest,
) (
	*Peer,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) EnablePeer(
	network string,
	name string,
) error {
	return ErrNotImplemented
}

func (s *Service) DisablePeer(
	network string,
	name string,
) error {
	return ErrNotImplemented
}

func (s *Service) ConfirmPeer(
	network string,
	pubKey string,
	ip net.IP,
) error {
	return ErrNotImplemented
}

func (s *Service) ReportEndpoints(
	network string,
	sightings []EndpointSighting,
) error {
	return ErrNotImplemented
}
