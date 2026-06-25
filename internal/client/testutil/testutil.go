package testutil

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
)

const DefaultNetworkName = "testnet"

var defaultInvite = service.Invite{
	AssignedCidr:   "10.42.0.5/16",
	ServerPubkey:   "server-pub-key",
	ServerEndpoint: "1.2.3.4:51820",
	ServerApiAddr:  "10.42.0.1:8443",
}

func SeedNetwork(
	t *testing.T,
	svc *service.Service,
) *service.Network {
	return SeedNetworkWithName(t, svc, DefaultNetworkName)
}

func SeedNetworkWithName(
	t *testing.T,
	svc *service.Service,
	name string,
) *service.Network {
	t.Helper()

	invite := defaultInvite
	invite.NetworkName = name
	nw, err := svc.InstallNetwork(invite)
	if err != nil {
		t.Fatalf("seed network %q: %v", name, err)
	}
	return nw
}
