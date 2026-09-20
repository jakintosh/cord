package runtime_test

import (
	"errors"
	"net"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/runtime"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestCreateRegistration_ResultSurvivesApplicationFailure(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	rt, _, backend, _ := newHTTPRuntime(t, env.Service, time.Second, nil)
	status, err := rt.SetNetworkEnabled(t.Context(), "httptest", true)
	if err != nil || status.Status == nil {
		t.Fatalf("enable = %+v, %v", status, err)
	}
	device := backend.Device("httptest-i")
	failure := errors.New("cannot update invite device")
	device.PeersErr = failure
	invitation, err := rt.CreateRegistration(t.Context(), "httptest", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("127.0.0.5")})
	if err != nil || invitation == nil || invitation.Peer.PrivateKey == "" {
		t.Fatalf("lost saved invitation: %+v, %v", invitation, err)
	}
	publicKey, err := wireguard.PublicKey(invitation.Peer.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	registrations, err := rt.ListRegistrations(t.Context(), "httptest")
	if err != nil || len(registrations) != 1 || registrations[0].InvitePublicKey != publicKey {
		t.Fatalf("saved registration does not match invitation: %+v, %v", registrations, err)
	}
	overview := networkStatus(t, rt, "httptest")
	if overview.Reason == "" || overview.Health != runtime.HealthDegraded {
		t.Fatalf("application failure missing from status: %+v", overview)
	}
	if overview.Status.Main.HTTP.State != resource.Running {
		t.Fatal("registration command stopped the HTTP listener")
	}
	if backend.Device("httptest-i") != device {
		t.Fatal("registration command replaced infrastructure")
	}
	device.PeersErr = nil
	if err := rt.RevokeRegistration(t.Context(), "httptest", "alice"); err != nil {
		t.Fatal(err)
	}
	if status := networkStatus(t, rt, "httptest"); status.Reason != "" || status.Health != runtime.HealthHealthy {
		t.Fatalf("recovery = %+v", status)
	}
}

func TestRegistrationCommands_ApplyPeersBeforeReturning(
	t *testing.T,
) {
	env := testutil.SetupRuntime(t)
	testutil.SeedNetwork(t, env.Service)
	if status, err := env.Runtime.SetNetworkEnabled(t.Context(), "testnet", true); err != nil || !status.Running {
		t.Fatalf("enable = %+v, %v", status, err)
	}
	main, invite := env.Backend.Device("testnet"), env.Backend.Device("testnet-i")
	invitation, err := env.Runtime.CreateRegistration(t.Context(), "testnet", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")})
	if err != nil {
		t.Fatal(err)
	}
	tempKey, err := wireguard.PublicKey(invitation.Peer.PrivateKey)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDevicePeer(t, invite, tempKey) {
		t.Fatal("creation returned before adding the invite peer")
	}
	privateKey, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := wireguard.PublicKey(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.Runtime.RedeemRegistration(t.Context(), "testnet", tempKey, publicKey); err != nil {
		t.Fatal(err)
	}
	if !hasDevicePeer(t, main, publicKey) {
		t.Fatal("redemption returned before adding the permanent peer")
	}
	if env.Backend.Device("testnet") != main || env.Backend.Device("testnet-i") != invite || len(env.Backend.CreateCalls) != 2 {
		t.Fatal("registration commands rebuilt the network devices")
	}
}
