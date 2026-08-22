package service_test

import (
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func installOptions() service.NetworkOptions {
	return service.NetworkOptions{}
}

// invitation returns a complete invite for the named network.
func invitation(
	name string,
) protocol.Invitation {
	return protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        name,
			PublicKey:   "srv-pub",
			Endpoint:    "1.2.3.4:51821",
			ServerRoute: "10.43.0.1/32",
			NetworkCidr: "10.0.0.0/16",
			APIPort:     8443,
		},
		Peer: protocol.PeerIdentity{
			Route:      "10.43.0.2/24",
			PrivateKey: "temp-key",
		},
	}
}

// assignment returns the invitation a redemption answers with: the main
// network's identity, without a peer private key.
func assignment(
	name string,
) protocol.Invitation {
	return protocol.Invitation{
		Network: protocol.NetworkInfo{
			Name:        name,
			PublicKey:   "main-srv-pub",
			Endpoint:    "1.2.3.4:51820",
			ServerRoute: "10.42.0.1/32",
			NetworkCidr: "10.42.0.0/16",
			APIPort:     8443,
		},
		Peer: protocol.PeerIdentity{
			Route: "10.42.0.5/32",
		},
	}
}

// TestBeginInstall_PersistsPermanentKey verifies that BeginInstall
// generates a permanent keypair and persists it in the install record.
func TestBeginInstall_PersistsPermanentKey(t *testing.T) {
	env := testutil.SetupService(t)

	inst, err := env.Service.BeginInstall(invitation("keytest"), installOptions())
	if err != nil {
		t.Fatalf("begin install: %v", err)
	}
	if inst.Phase != service.PhaseInvited {
		t.Errorf("phase = %q, want %q", inst.Phase, service.PhaseInvited)
	}
	if inst.MainPrivateKey == "" {
		t.Error("private key should not be empty")
	}
	pubKey, err := wireguard.PublicKey(inst.MainPrivateKey)
	if err != nil {
		t.Fatalf("derive public key: %v", err)
	}
	if pubKey == "" {
		t.Error("public key should not be empty")
	}
	if inst.MainAssignedRoute != "" {
		t.Errorf("assigned_cidr = %q, want empty before redeem", inst.MainAssignedRoute)
	}
}

func TestBeginInstall_ListenPortOptions(t *testing.T) {
	env := testutil.SetupService(t)

	inst, err := env.Service.BeginInstall(invitation("listen-port"), service.NetworkOptions{})
	if err != nil {
		t.Fatalf("begin install with default options: %v", err)
	}
	if inst.ListenPort != 0 {
		t.Errorf("default listen port = %d, want 0", inst.ListenPort)
	}

	port := uint16(51820)
	inst, err = env.Service.BeginInstall(
		invitation("port-explicit"),
		service.NetworkOptions{ListenPort: &port},
	)
	if err != nil {
		t.Fatalf("begin install with explicit port: %v", err)
	}
	if inst.ListenPort != port {
		t.Errorf("explicit listen port = %d, want %d", inst.ListenPort, port)
	}
}

// TestBeginInstall_Idempotent verifies that calling BeginInstall twice
// with the same invite returns the same install record with the same
// permanent key — the key is never regenerated.
func TestBeginInstall_Idempotent(t *testing.T) {
	env := testutil.SetupService(t)

	inst1, err := env.Service.BeginInstall(invitation("idempotent"), installOptions())
	if err != nil {
		t.Fatalf("first begin: %v", err)
	}

	inst2, err := env.Service.BeginInstall(invitation("idempotent"), installOptions())
	if err != nil {
		t.Fatalf("second begin: %v", err)
	}

	if inst1.MainPrivateKey != inst2.MainPrivateKey {
		t.Errorf("private key changed: %q vs %q", inst1.MainPrivateKey, inst2.MainPrivateKey)
	}
}

func TestBeginInstall_IncompatibleRetryConflicts(t *testing.T) {
	env := testutil.SetupService(t)

	if _, err := env.Service.BeginInstall(
		invitation("incompatible"),
		installOptions(),
	); err != nil {
		t.Fatalf("first begin: %v", err)
	}

	invite := invitation("incompatible")
	invite.Peer.Route = "10.43.0.3/24"
	_, err := env.Service.BeginInstall(invite, installOptions())
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
}

// TestBeginInstall_ExistingConfirmedNetwork verifies that BeginInstall
// refuses if a completed network already exists.
func TestBeginInstall_ExistingConfirmedNetwork(t *testing.T) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "already-here")

	_, err := env.Service.BeginInstall(invitation("already-here"), installOptions())
	if err == nil {
		t.Fatal("expected error for existing network")
	}
}

// TestBeginInstall_RejectsIncompleteInvitation verifies that every field
// a peer needs is required before an install record is created.
func TestBeginInstall_RejectsIncompleteInvitation(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*protocol.Invitation)
		missing string
	}{
		{"network name", func(i *protocol.Invitation) { i.Network.Name = "" }, "name"},
		{"peer private key", func(i *protocol.Invitation) { i.Peer.PrivateKey = "" }, "private key"},
		{"peer route", func(i *protocol.Invitation) { i.Peer.Route = "" }, "route"},
		{"server public key", func(i *protocol.Invitation) { i.Network.PublicKey = "" }, "public key"},
		{"server endpoint", func(i *protocol.Invitation) { i.Network.Endpoint = "" }, "endpoint"},
		{"server route", func(i *protocol.Invitation) { i.Network.ServerRoute = "" }, "server route"},
		{"network cidr", func(i *protocol.Invitation) { i.Network.NetworkCidr = "" }, "cidr"},
		{"api port", func(i *protocol.Invitation) { i.Network.APIPort = 0 }, "api port"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.SetupService(t)

			invite := invitation("incomplete")
			tt.mutate(&invite)

			_, err := env.Service.BeginInstall(invite, installOptions())
			if !errors.Is(err, service.ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
		})
	}
}

// TestRedeemInstall_PersistsAssignment verifies that the main-network
// identity a redemption returns is what moves the install forward.
func TestRedeemInstall_PersistsAssignment(t *testing.T) {
	env := testutil.SetupService(t)

	if _, err := env.Service.BeginInstall(invitation("redeem-me"), installOptions()); err != nil {
		t.Fatalf("begin install: %v", err)
	}

	inst, err := env.Service.RedeemInstall("redeem-me", assignment("redeem-me"))
	if err != nil {
		t.Fatalf("redeem install: %v", err)
	}
	if inst.Phase != service.PhaseRedeemed {
		t.Errorf("phase = %q, want %q", inst.Phase, service.PhaseRedeemed)
	}
	if inst.MainAssignedRoute != "10.42.0.5/32" {
		t.Errorf("assigned route = %q, want 10.42.0.5/32", inst.MainAssignedRoute)
	}
	if inst.MainServer.Endpoint != "1.2.3.4:51820" {
		t.Errorf("server endpoint = %q, want 1.2.3.4:51820", inst.MainServer.Endpoint)
	}
}

func TestRedeemInstall_RejectsIncompleteResult(t *testing.T) {
	env := testutil.SetupService(t)

	if _, err := env.Service.BeginInstall(invitation("bad-redeem"), installOptions()); err != nil {
		t.Fatalf("begin install: %v", err)
	}

	result := assignment("bad-redeem")
	result.Network.ServerRoute = ""

	_, err := env.Service.RedeemInstall("bad-redeem", result)
	if !errors.Is(err, service.ErrInvalidInput) {
		t.Errorf("err = %v, want ErrInvalidInput", err)
	}
}

// TestConfirmInstall_CreatesEnabledNetwork verifies that confirmation
// consumes the install into a membership the runtime is told to bring up.
func TestConfirmInstall_CreatesEnabledNetwork(t *testing.T) {
	env := testutil.SetupService(t)

	if _, err := env.Service.BeginInstall(invitation("confirm-me"), installOptions()); err != nil {
		t.Fatalf("begin install: %v", err)
	}
	if _, err := env.Service.RedeemInstall("confirm-me", assignment("confirm-me")); err != nil {
		t.Fatalf("redeem install: %v", err)
	}

	network, err := env.Service.ConfirmInstall("confirm-me")
	if err != nil {
		t.Fatalf("confirm install: %v", err)
	}
	if !network.Enabled {
		t.Error("a confirmed network should be enabled")
	}
	if network.AssignedRoute != "10.42.0.5/32" {
		t.Errorf("assigned route = %q, want 10.42.0.5/32", network.AssignedRoute)
	}
	if network.Server.PublicKey == "" {
		t.Error("server public key should not be empty after confirm")
	}
	expectWake(t, env, "confirm-me")

	if _, err := env.Service.GetInstall("confirm-me"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("install after confirm: err = %v, want ErrNotFound", err)
	}
}

func TestConfirmInstall_RequiresRedeemedPhase(t *testing.T) {
	env := testutil.SetupService(t)

	if _, err := env.Service.BeginInstall(invitation("still-invited"), installOptions()); err != nil {
		t.Fatalf("begin install: %v", err)
	}

	_, err := env.Service.ConfirmInstall("still-invited")
	if !errors.Is(err, service.ErrInstallState) {
		t.Errorf("err = %v, want ErrInstallState", err)
	}
}
