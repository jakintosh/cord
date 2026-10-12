package service_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/client/testutil"
	"git.studiopollinator.com/pollinator/cord/internal/protocol"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

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

func TestBeginInstall_PersistsPermanentKey(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	invite := testutil.Invitation("keytest", "10.43.0.1:8443")
	inst, err := env.Service.BeginInstall(invite, service.NetworkOptions{})
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
	if inst.MainIfaceName != "keytest" || inst.InviteIfaceName != "keytest-i" ||
		inst.InvitePrivateKey != invite.Peer.PrivateKey || inst.InviteAssignedRoute != invite.Peer.Route ||
		!inst.CreatedAt.Equal(testutil.FixedTime) {
		t.Fatalf("install identity = %#v", inst)
	}
	wantServer := service.ServerInfo{
		PublicKey: invite.Network.PublicKey, Endpoint: invite.Network.Endpoint,
		Route: invite.Network.ServerRoute, NetworkCidr: invite.Network.NetworkCidr, APIPort: invite.Network.APIPort,
	}
	if inst.InviteServer != wantServer {
		t.Errorf("invite server = %#v, want %#v", inst.InviteServer, wantServer)
	}
	persisted, err := env.Service.GetInstall(inst.Name)
	if err != nil {
		t.Fatal(err)
	}
	persisted.CreatedAt = persisted.CreatedAt.UTC()
	inst.CreatedAt = inst.CreatedAt.UTC()
	if !reflect.DeepEqual(persisted, inst) {
		t.Fatalf("persisted install = %#v, want %#v", persisted, inst)
	}
}

func TestBeginInstall_ListenPortOptions(
	t *testing.T,
) {
	zero, chosen, maximum := uint16(0), uint16(51820), uint16(65535)
	for _, tc := range []struct {
		name string
		port *uint16
		want uint16
	}{
		{"default", nil, 0},
		{"ephemeral", &zero, 0},
		{"chosen", &chosen, 51820},
		{"maximum", &maximum, 65535},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testutil.SetupService(t)
			install, err := env.Service.BeginInstall(
				testutil.Invitation("testnet", "10.43.0.1:8443"),
				service.NetworkOptions{ListenPort: tc.port},
			)
			if err != nil {
				t.Fatal(err)
			}
			if install.ListenPort != tc.want {
				t.Fatalf("listen port = %d, want %d", install.ListenPort, tc.want)
			}
		})
	}
}

func TestBeginInstall_RetryPreservesPermanentIdentity(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	invite := testutil.Invitation("idempotent", "10.43.0.1:8443")

	inst1, err := env.Service.BeginInstall(invite, service.NetworkOptions{})
	if err != nil {
		t.Fatalf("first begin: %v", err)
	}

	inst2, err := env.Service.BeginInstall(invite, service.NetworkOptions{})
	if err != nil {
		t.Fatalf("second begin: %v", err)
	}

	if inst1.MainPrivateKey != inst2.MainPrivateKey {
		t.Errorf("private key changed: %q vs %q", inst1.MainPrivateKey, inst2.MainPrivateKey)
	}
}

func TestBeginInstall_IncompatibleRetryConflicts(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	invite := testutil.Invitation("incompatible", "10.43.0.1:8443")

	if _, err := env.Service.BeginInstall(
		invite,
		service.NetworkOptions{},
	); err != nil {
		t.Fatalf("first begin: %v", err)
	}
	before, err := env.Service.GetInstall("incompatible")
	if err != nil {
		t.Fatal(err)
	}

	invite.Peer.Route = "10.43.0.3/24"
	_, err = env.Service.BeginInstall(invite, service.NetworkOptions{})
	if !errors.Is(err, service.ErrConflict) {
		t.Fatalf("err = %v, want ErrConflict", err)
	}
	after, err := env.Service.GetInstall("incompatible")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected retry changed install: %#v", after)
	}
}

func TestBeginInstall_ExistingConfirmedNetwork(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "already-here")

	_, err := env.Service.BeginInstall(testutil.Invitation("already-here", "10.43.0.1:8443"), service.NetworkOptions{})
	if !errors.Is(err, service.ErrNetworkExists) {
		t.Fatalf("error = %v, want ErrNetworkExists", err)
	}
}

func TestBeginInstall_RejectsIncompleteInvitation(
	t *testing.T,
) {
	tests := []struct {
		name   string
		mutate func(*protocol.Invitation)
	}{
		{"network name", func(i *protocol.Invitation) { i.Network.Name = "" }},
		{"peer private key", func(i *protocol.Invitation) { i.Peer.PrivateKey = "" }},
		{"peer route", func(i *protocol.Invitation) { i.Peer.Route = "" }},
		{"server public key", func(i *protocol.Invitation) { i.Network.PublicKey = "" }},
		{"server endpoint", func(i *protocol.Invitation) { i.Network.Endpoint = "" }},
		{"server route", func(i *protocol.Invitation) { i.Network.ServerRoute = "" }},
		{"network cidr", func(i *protocol.Invitation) { i.Network.NetworkCidr = "" }},
		{"api port", func(i *protocol.Invitation) { i.Network.APIPort = 0 }},
		{"main interface too long", func(i *protocol.Invitation) { i.Network.Name = "abcdefghijklmnop" }},
		{"invite suffix exceeds limit", func(i *protocol.Invitation) { i.Network.Name = "abcdefghijklmn" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := testutil.SetupService(t)

			invite := testutil.Invitation("incomplete", "10.43.0.1:8443")
			tt.mutate(&invite)

			_, err := env.Service.BeginInstall(invite, service.NetworkOptions{})
			if !errors.Is(err, service.ErrInvalidInput) {
				t.Errorf("err = %v, want ErrInvalidInput", err)
			}
			installs, err := env.Service.ListInstalls()
			if err != nil {
				t.Fatal(err)
			}
			if len(installs) != 0 {
				t.Fatalf("invalid invitation created installs: %#v", installs)
			}
		})
	}
}

func TestRedeemInstall_PersistsAssignment(
	t *testing.T,
) {
	env := testutil.SetupService(t)

	testutil.SeedInstall(t, env.Service, "redeem-me")

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

func TestRedeemInstall_RejectsIncompleteResultWithoutChangingInstall(
	t *testing.T,
) {
	for _, tc := range []struct {
		name   string
		mutate func(*protocol.Invitation)
	}{
		{"network name", func(i *protocol.Invitation) { i.Network.Name = "" }},
		{"peer route", func(i *protocol.Invitation) { i.Peer.Route = "" }},
		{"server public key", func(i *protocol.Invitation) { i.Network.PublicKey = "" }},
		{"server endpoint", func(i *protocol.Invitation) { i.Network.Endpoint = "" }},
		{"server route", func(i *protocol.Invitation) { i.Network.ServerRoute = "" }},
		{"network cidr", func(i *protocol.Invitation) { i.Network.NetworkCidr = "" }},
		{"api port", func(i *protocol.Invitation) { i.Network.APIPort = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testutil.SetupService(t)
			testutil.SeedInstall(t, env.Service, "testnet")
			before, err := env.Service.GetInstall("testnet")
			if err != nil {
				t.Fatal(err)
			}
			result := assignment("testnet")
			tc.mutate(&result)

			_, err = env.Service.RedeemInstall("testnet", result)
			if !errors.Is(err, service.ErrInvalidInput) {
				t.Fatalf("error = %v, want ErrInvalidInput", err)
			}
			after, err := env.Service.GetInstall("testnet")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("install changed after rejected assignment: %#v", after)
			}
		})
	}
}

func TestConfirmInstall_CreatesEnabledNetwork(
	t *testing.T,
) {
	now := testutil.FixedTime
	env := testutil.SetupServiceWithClock(t, func() time.Time { return now })
	invited := testutil.SeedInstall(t, env.Service, "confirm-me")
	if _, err := env.Service.RedeemInstall("confirm-me", assignment("confirm-me")); err != nil {
		t.Fatalf("redeem install: %v", err)
	}

	now = now.Add(time.Hour)
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
	if network.PrivateKey != invited.MainPrivateKey || network.InterfaceName != invited.MainIfaceName ||
		!network.CreatedAt.Equal(now) {
		t.Fatalf("confirmed identity/time = %#v", network)
	}
	persisted, err := env.Service.GetNetwork("confirm-me")
	if err != nil {
		t.Fatal(err)
	}
	persisted.CreatedAt = persisted.CreatedAt.UTC()
	network.CreatedAt = network.CreatedAt.UTC()
	if !reflect.DeepEqual(persisted, network) {
		t.Fatalf("persisted network = %#v, want %#v", persisted, network)
	}

	if _, err := env.Service.GetInstall("confirm-me"); !errors.Is(err, service.ErrNotFound) {
		t.Errorf("install after confirm: err = %v, want ErrNotFound", err)
	}
}

func TestConfirmInstall_RequiresRedeemedPhase(
	t *testing.T,
) {
	env := testutil.SetupService(t)

	testutil.SeedInstall(t, env.Service, "still-invited")

	_, err := env.Service.ConfirmInstall("still-invited")
	if !errors.Is(err, service.ErrInstallState) {
		t.Errorf("err = %v, want ErrInstallState", err)
	}
	install, err := env.Service.GetInstall("still-invited")
	if err != nil {
		t.Fatal(err)
	}
	if install.Phase != service.PhaseInvited {
		t.Fatalf("rejected confirmation changed phase to %q", install.Phase)
	}
	if _, err := env.Service.GetNetwork("still-invited"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("rejected confirmation created network: %v", err)
	}
}

func TestUninstallNetwork_RemovesMembership(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetworkDirect(t, env.Database, "to-delete")

	if err := env.Service.UninstallNetwork("to-delete"); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	_, err := env.Service.GetNetwork("to-delete")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("after uninstall: err = %v, want ErrNotFound", err)
	}
}

func TestUninstallNetwork_NotFound(
	t *testing.T,
) {
	env := testutil.SetupService(t)

	err := env.Service.UninstallNetwork("ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListInstalls_ReturnsPendingInstallsInNameOrder(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	installs, err := env.Service.ListInstalls()
	if err != nil || len(installs) != 0 {
		t.Fatalf("empty installs = %#v, error %v", installs, err)
	}
	testutil.SeedInstall(t, env.Service, "beta")
	testutil.SeedInstall(t, env.Service, "alpha")
	if _, err := env.Service.RedeemInstall("alpha", assignment("alpha")); err != nil {
		t.Fatal(err)
	}
	installs, err = env.Service.ListInstalls()
	if err != nil {
		t.Fatal(err)
	}
	if len(installs) != 2 || installs[0].Name != "alpha" || installs[1].Name != "beta" ||
		installs[0].Phase != service.PhaseRedeemed || installs[1].Phase != service.PhaseInvited {
		t.Fatalf("pending installs = %#v, want redeemed alpha then invited beta", installs)
	}
}

func TestUninstallNetwork_RemovesPendingInstall(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedInstall(t, env.Service, "testnet")
	if err := env.Service.UninstallNetwork("testnet"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Service.GetInstall("testnet"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("install after uninstall = %v, want ErrNotFound", err)
	}
}

func TestRedeemInstall_MissingInstall(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	_, err := env.Service.RedeemInstall("missing", assignment("missing"))
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}

func TestConfirmInstall_MissingInstall(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	_, err := env.Service.ConfirmInstall("missing")
	if !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("error = %v, want ErrNotFound", err)
	}
}
