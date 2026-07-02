package testutil

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

const DefaultNetworkName = "testnet"

var defaultInvite = service.Invite{
	TempPeerPrivKey:      mustGenerateKey(),
	TempPeerAssignedCidr: "10.42.0.5/16",
	InviteServerPubkey:   "server-pub-key",
	InviteServerEndpoint: "1.2.3.4:51820",
	InviteServerAddr:     "10.42.0.1:8443",
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
	nw, err := svc.Install(invite)
	if err != nil {
		t.Fatalf("seed network %q: %v", name, err)
	}
	return nw
}

func SeedNetworkDirect(
	t *testing.T,
	svc *service.Service,
	name string,
) *service.Network {
	t.Helper()

	privKey, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("seed network %q: generate key: %v", name, err)
	}
	pubKey, err := wireguard.PublicKey(privKey)
	if err != nil {
		t.Fatalf("seed network %q: derive public key: %v", name, err)
	}

	nw := &service.Network{
		Name:                name,
		State:               service.StateConfirmed,
		PrivateKey:          privKey,
		PublicKey:           pubKey,
		MainInterfaceName:   name,
		InviteInterfaceName: name + "-i",
		AssignedCidr:        "10.42.0.5/16",
		ServerPubkey:        mustGenerateKey(),
		ServerEndpoint:      "1.2.3.4:51820",
		ServerApiAddr:       "10.42.0.1:8443",
		Enabled:             false,
		CreatedAt:           time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC),
	}
	if err := svc.InsertNetworkDirect(nw); err != nil {
		t.Fatalf("seed network %q: %v", name, err)
	}
	return nw
}
