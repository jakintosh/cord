package testutil

import (
	"net"
	"net/http"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/api/identity"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

func SeedNetwork(
	t *testing.T,
	svc *service.Service,
) *service.NetworkConfig {
	t.Helper()

	nw, err := svc.CreateNetwork(
		"testnet",
		"192.168.1.1",
		service.PlaneConfig{
			Cidr:          "10.0.0.0/16",
			WireguardPort: 51820,
			ApiPort:       80,
		},
		service.PlaneConfig{
			Cidr:          "10.1.0.0/24",
			WireguardPort: 51821,
			ApiPort:       80,
		},
	)
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
