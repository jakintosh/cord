package service_test

import (
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

type testEnv struct {
	svc *service.Service
	db  *testutil.Env
	wg  *wireguardtest.MockWG
}

func setupTestEnv(t *testing.T) *testEnv {
	t.Helper()
	wg := wireguardtest.NewMockWG()
	env := testutil.SetupService(t, wg)
	return &testEnv{svc: env.Service, db: env, wg: wg}
}

var fixedTime = time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)

func seedNetwork(t *testing.T, svc *service.Service) *service.Network {
	t.Helper()
	net, err := svc.CreateNetwork(service.Network{
		Name:             "testnet",
		RootCidr:         "10.0.0.0/16",
		InviteCidr:       "10.1.0.0/24",
		ExternalIP:       "192.168.1.1",
		ListenPort:       51820,
		InviteListenPort: 51821,
		ApiPort:          8080,
	})
	if err != nil {
		t.Fatalf("seed network: %v", err)
	}
	return net
}

func seedSubCidr(t *testing.T, svc *service.Service, network, name, cidr string) {
	t.Helper()
	if err := svc.AddCidr(network, service.CreateCidrRequest{
		Name: name,
		Cidr: cidr,
	}); err != nil {
		t.Fatalf("seed cidr %s: %v", name, err)
	}
}

func seedPeer(t *testing.T, svc *service.Service, network, name, ip string) {
	t.Helper()
	_, err := svc.AddPeer(network, service.PeerConfig{
		Name:  name,
		IP:    ip,
		Admin: false,
	})
	if err != nil {
		t.Fatalf("seed peer %s: %v", name, err)
	}
}

func lastTempKey(t *testing.T, svc *service.Service, network string) string {
	t.Helper()
	invites, err := svc.ListInvites(network)
	if err != nil {
		t.Fatalf("list invites: %v", err)
	}
	if len(invites) == 0 {
		t.Fatal("no invites found")
	}
	return invites[len(invites)-1].TempPubKey
}
