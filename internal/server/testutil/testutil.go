package testutil

import (
	"net"
	"net/http"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/api/identity"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

var DefaultNetwork = service.Network{
	Name:             "testnet",
	MainCidr:         "10.0.0.0/16",
	InviteCidr:       "10.1.0.0/24",
	ExternalIP:       "192.168.1.1",
	ListenPort:       51820,
	InviteListenPort: 51821,
	ApiPort:          8080,
}

func SeedNetwork(
	t *testing.T,
	svc *service.Service,
) *service.Network {
	t.Helper()

	nw, err := svc.CreateNetwork(DefaultNetwork)
	if err != nil {
		t.Fatalf("seed network: %v", err)
	}
	return nw
}

type FailResolver struct{}

func (f *FailResolver) ResolveIdentity(
	sourceIP net.IP,
) (*identity.Peer, error) {
	return nil, http.ErrNoLocation
}
