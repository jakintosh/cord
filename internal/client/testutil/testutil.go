package testutil

import (
	"net"
	"strconv"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/database"
	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/topology"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

const DefaultNetworkName = "testnet"

// Invitation builds a complete invite whose invite network is reachable
// at apiAddr, so the tunnel a redemption brings up actually reaches the
// test server listening there.
func Invitation(
	name string,
	apiAddr string,
) protocol.Invitation {
	return protocol.Invitation{
		Network: servedNetwork(name, apiAddr),
		Peer: protocol.PeerIdentity{
			Route:      "10.43.0.2/24",
			PrivateKey: mustGenerateKey(),
		},
	}
}

// servedNetwork describes a network whose API answers at apiAddr.
func servedNetwork(
	name string,
	apiAddr string,
) protocol.NetworkInfo {
	host, portStr, err := net.SplitHostPort(apiAddr)
	if err != nil {
		panic("split test server address: " + err.Error())
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		panic("parse test server port: " + err.Error())
	}
	return protocol.NetworkInfo{
		Name:        name,
		PublicKey:   mustGenerateKey(),
		Endpoint:    "1.2.3.4:51820",
		ServerRoute: host + "/32",
		NetworkCidr: "10.0.0.0/16",
		APIPort:     uint16(port),
	}
}

func NetworkSnapshot(
	peers ...protocol.VisiblePeer,
) protocol.VisibleNetworkSnapshot {
	return protocol.VisibleNetworkSnapshot{
		GeneratedAt: FixedTime,
		Peers:       peers,
		Topology: protocol.TopologyView{
			SubjectPeer: "self",
			Nodes: []protocol.TopologyNode{
				{
					Name:     "self",
					CIDR:     "10.42.0.5/32",
					Terminal: true,
					PeerName: "self",
					Subject:  true,
				},
			},
		},
	}
}

func NetworkReconciliation(
	peers ...service.PeerObservation,
) service.NetworkReconciliation {
	self, err := topology.CidrFromString("self", "10.42.0.5/32", true)
	if err != nil {
		panic("construct test topology: " + err.Error())
	}
	return service.NetworkReconciliation{
		Peers: peers,
		Topology: topology.View{
			SubjectPeer: "self",
			Nodes: []topology.ViewNode{{
				Cidr:     self,
				PeerName: "self",
				Subject:  true,
			}},
		},
		GeneratedAt: FixedTime,
		ReceivedAt:  FixedTime,
		PruneBefore: FixedTime.Add(-service.EndpointTTL),
	}
}

func mustGenerateKey() string {
	key, err := wireguard.GenerateKey()
	if err != nil {
		panic("generate key: " + err.Error())
	}
	return key
}

// SeedInstall creates an in-progress install record at phase "invited"
// without redeeming or confirming it.
func SeedInstall(
	t *testing.T,
	svc *service.Service,
	name string,
) *service.Install {
	t.Helper()

	inst, err := svc.BeginInstall(
		Invitation(name, "10.43.0.1:8443"),
		service.NetworkOptions{},
	)
	if err != nil {
		t.Fatalf("seed install %q: %v", name, err)
	}
	return inst
}

func SeedNetworkDirect(
	t *testing.T,
	db *database.DB,
	name string,
) *service.Network {
	t.Helper()

	privKey, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("seed network %q: generate key: %v", name, err)
	}

	_, err = db.BeginInstall(service.BeginInstallParams{
		Name:                name,
		InviteIfaceName:     name + "-i",
		InvitePrivateKey:    mustGenerateKey(),
		InviteAssignedRoute: "10.43.0.5/32",
		InviteServer: service.ServerInfo{
			PublicKey:   mustGenerateKey(),
			Endpoint:    "1.2.3.4:51821",
			Route:       "10.43.0.1/32",
			NetworkCidr: "10.43.0.0/24",
			APIPort:     8443,
		},
		MainIfaceName:  name,
		MainPrivateKey: privKey,
		CreatedAt:      FixedTime,
	})
	if err != nil {
		t.Fatalf("seed network %q: begin install: %v", name, err)
	}

	_, err = db.RedeemInstall(name, service.NetworkAssignment{
		AssignedRoute: "10.42.0.5/32",
		Server: service.ServerInfo{
			PublicKey:   mustGenerateKey(),
			Endpoint:    "1.2.3.4:51820",
			Route:       "10.42.0.1/32",
			NetworkCidr: "10.42.0.0/16",
			APIPort:     8443,
		},
	})
	if err != nil {
		t.Fatalf("seed network %q: redeem install: %v", name, err)
	}

	nc, err := db.ConfirmInstall(name, privKey, FixedTime)
	if err != nil {
		t.Fatalf("seed network %q: confirm install: %v", name, err)
	}
	if err := db.SetNetworkEnabled(name, false); err != nil {
		t.Fatalf("seed network %q: disable: %v", name, err)
	}
	nc.Enabled = false
	return nc
}

func SeedPeers(
	t *testing.T,
	db *database.DB,
	network string,
	peers ...service.Peer,
) {
	t.Helper()

	observations := make([]service.PeerObservation, len(peers))
	for i, peer := range peers {
		observations[i] = service.PeerObservation{Peer: peer}
	}
	if err := db.ApplyNetworkReconciliation(
		network,
		NetworkReconciliation(observations...),
	); err != nil {
		t.Fatalf("seed peers: %v", err)
	}
}
