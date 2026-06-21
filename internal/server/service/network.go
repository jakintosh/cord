package service

import "time"

type Network struct {
	Name             string
	PrivateKey       string
	PublicKey        string
	RootCidr         string
	InviteCidr       string
	ExternalIP       string
	ListenPort       uint16
	InviteListenPort uint16
	ApiPort          uint16
	CreatedAt        time.Time
}

func (s *Service) GetNetwork(
	name string,
) (
	*Network,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) ListNetworks() (
	[]string,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) CreateNetwork(
	cfg Network,
) (
	*Network,
	error,
) {
	return nil, ErrNotImplemented
}

func (s *Service) DeleteNetwork(
	name string,
) error {
	return ErrNotImplemented
}
