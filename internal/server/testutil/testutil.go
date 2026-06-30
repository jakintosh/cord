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
) *service.Network {
	t.Helper()

	mainWgPort := uint16(51820)
	mainAPIPort := uint16(80)
	inviteWgPort := uint16(51821)
	inviteAPIPort := uint16(80)
	inviteCidr := "10.1.0.0/24"

	nw, err := svc.CreateNetwork(
		"testnet",
		"192.168.1.1",
		"10.0.0.0/16",
		nil,
		&mainWgPort,
		&mainAPIPort,
		nil,
		&inviteCidr,
		&inviteWgPort,
		&inviteAPIPort,
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
