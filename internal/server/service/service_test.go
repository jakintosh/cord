package service_test

import (
	"errors"
	"net"
	"strings"
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

func TestResolvePeerIdentity_ExcludesDisabled(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.5")
	if _, err := env.Service.CreateRegistration("testnet", "toggle", service.RegistrationOptions{PeerIP: ip}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", lastTempKey(t, env.Service, "testnet"), mustGenKey(t)); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", "toggle"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	disabled := false
	if _, err := env.Service.UpdatePeer("testnet", "toggle", service.PeerDiff{Enabled: &disabled}); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if _, err := env.Service.ResolvePeerIdentity("testnet", ip); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("resolve disabled peer: err = %v, want ErrNotFound", err)
	}

	enabled := true
	if _, err := env.Service.UpdatePeer("testnet", "toggle", service.PeerDiff{Enabled: &enabled}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if _, err := env.Service.ResolvePeerIdentity("testnet", ip); err != nil {
		t.Fatalf("resolve re-enabled peer: %v", err)
	}
}

func TestResolvePeerIdentity_NormalizesSourceIP(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	ip := net.ParseIP("10.0.0.5")
	if _, err := env.Service.CreateRegistration("testnet", "mapped", service.RegistrationOptions{PeerIP: ip}); err != nil {
		t.Fatalf("create registration: %v", err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", lastTempKey(t, env.Service, "testnet"), mustGenKey(t)); err != nil {
		t.Fatalf("redeem: %v", err)
	}
	if err := env.Service.ConfirmPeer("testnet", "mapped"); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	mapped := net.ParseIP("::ffff:10.0.0.5")
	peer, err := env.Service.ResolvePeerIdentity("testnet", mapped)
	if err != nil {
		t.Fatalf("resolve IPv4-mapped source: %v", err)
	}
	if peer.Name != "mapped" {
		t.Fatalf("identity = %q, want mapped", peer.Name)
	}
}

// topologyPeerByCidr returns the compiled full topology view's CIDR to
// peer-name mapping for the network.
func topologyPeerByCidr(
	t *testing.T,
	svc *service.Service,
	network string,
) map[string]string {
	t.Helper()

	view, err := svc.GetNetworkTopology(network)
	if err != nil {
		t.Fatalf("get topology: %v", err)
	}
	peerByCidr := make(map[string]string, len(view.Nodes))
	for _, node := range view.Nodes {
		peerByCidr[node.Cidr.Name] = node.PeerName
	}
	return peerByCidr
}

// wantInvalidInput asserts that err rejects the operation as invalid
// input and that the rejection names the rule the caller expects.
func wantInvalidInput(
	t *testing.T,
	err error,
	fragment string,
) {
	t.Helper()

	if !errors.Is(err, service.ErrInvalidInput) {
		t.Fatalf("err = %v, want ErrInvalidInput", err)
	}
	if !strings.Contains(err.Error(), fragment) {
		t.Fatalf("err = %q, want it to contain %q", err, fragment)
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
