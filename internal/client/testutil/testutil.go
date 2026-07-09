package testutil

import (
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

const DefaultNetworkName = "testnet"

var defaultInvite = protocol.Invitation{
	Network: protocol.NetworkInfo{
		PublicKey:   "server-pub-key",
		Endpoint:    "1.2.3.4:51820",
		ServerRoute: "10.42.0.1/32",
		NetworkCidr: "10.42.0.0/16",
		APIPort:     8443,
	},
	Peer: protocol.PeerIdentity{
		Route:      "10.42.0.5/32",
		PrivateKey: mustGenerateKey(),
	},
}

func mustGenerateKey() string {
	key, err := wireguard.GenerateKey()
	if err != nil {
		panic("generate key: " + err.Error())
	}
	return key
}

func SeedNetwork(
	t *testing.T,
	svc *service.Service,
) *service.NetworkConfig {
	return SeedNetworkWithName(t, svc, DefaultNetworkName)
}

func SeedNetworkWithName(
	t *testing.T,
	svc *service.Service,
	name string,
) *service.NetworkConfig {
	t.Helper()

	invite := defaultInvite
	invite.Network.Name = name
	nc, err := svc.InstallNetwork(invite)
	if err != nil {
		t.Fatalf("seed network %q: %v", name, err)
	}
	return nc
}

func SeedNetworkDirect(
	t *testing.T,
	svc *service.Service,
	name string,
) *service.NetworkConfig {
	t.Helper()

	privKey, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("seed network %q: generate key: %v", name, err)
	}

	nc := &service.NetworkConfig{
		Name:          name,
		PrivateKey:    privKey,
		InterfaceName: name,
		AssignedRoute: "10.42.0.5/32",
		Server: service.ServerInfo{
			PublicKey:   mustGenerateKey(),
			Endpoint:    "1.2.3.4:51820",
			Route:       "10.42.0.1/32",
			NetworkCidr: "10.42.0.0/16",
			APIPort:     8443,
		},
		Enabled:   false,
		CreatedAt: FixedTime,
	}
	if err := svc.InsertNetworkDirect(nc); err != nil {
		t.Fatalf("seed network %q: %v", name, err)
	}
	return nc
}
