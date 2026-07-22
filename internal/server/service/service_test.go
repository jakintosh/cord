package service_test

import (
	"errors"
	"net"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func lastTempKey(
	t *testing.T,
	svc *service.Service,
	network string,
) string {
	t.Helper()
	regs, err := svc.ListRegistrations(network)
	if err != nil {
		t.Fatalf("list registrations: %v", err)
	}
	if len(regs) == 0 {
		t.Fatal("no registrations found")
	}
	return regs[len(regs)-1].InvitePublicKey
}

func TestResolvePeerIdentity_TransitionsOnConfirmation(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	ip := net.ParseIP("10.0.0.5")

	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: ip}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", lastTempKey(t, env.Service, "testnet"), mustGenKey(t)); err != nil {
		t.Fatalf("redeem registration: %v", err)
	}

	provisional, err := env.Service.ResolveProvisionalIdentity("testnet", ip)
	if err != nil {
		t.Fatalf("resolve provisional identity: %v", err)
	}
	if provisional.Name != "alice" {
		t.Fatalf("provisional identity = %q, want alice", provisional.Name)
	}
	if _, err := env.Service.ResolvePeerIdentity("testnet", ip); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("resolve confirmed identity before confirmation: err = %v, want ErrNotFound", err)
	}

	if err := env.Service.ConfirmPeer("testnet", "alice"); err != nil {
		t.Fatalf("confirm peer: %v", err)
	}
	confirmed, err := env.Service.ResolvePeerIdentity("testnet", ip)
	if err != nil {
		t.Fatalf("resolve confirmed identity: %v", err)
	}
	if confirmed.Name != "alice" {
		t.Fatalf("confirmed identity = %q, want alice", confirmed.Name)
	}
	if _, err := env.Service.ResolveProvisionalIdentity("testnet", ip); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("resolve provisional identity after confirmation: err = %v, want ErrNotFound", err)
	}
}

func mustGenKey(t *testing.T) string {
	t.Helper()
	k, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	pub, err := wireguard.PublicKey(k)
	if err != nil {
		t.Fatalf("derive public key: %v", err)
	}
	return pub
}

func hasPeerOp(ops []wireguard.PeerOp, pubKey string) bool {
	for _, op := range ops {
		if !op.Remove && op.Target.PublicKey.String() == pubKey {
			return true
		}
	}
	return false
}
