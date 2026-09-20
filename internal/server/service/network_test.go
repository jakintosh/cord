package service_test

import (
	"errors"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"net"
	"reflect"
	"strings"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

func TestCreateNetwork_Success(
	t *testing.T,
) {
	env := testutil.SetupService(t)

	net, err := env.Service.CreateNetwork(
		"mynet",
		"1.2.3.4",
		service.Plane{
			Cidr:          "10.0.0.0/16",
			WireguardPort: 51820,
			ApiPort:       8080,
		},
		service.Plane{
			Cidr:          "10.1.0.0/24",
			WireguardPort: 51821,
			ApiPort:       8080,
		},
	)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}

	if net.Name != "mynet" {
		t.Errorf("name = %q, want mynet", net.Name)
	}
	if net.PrivateKey == "" {
		t.Error("private_key should not be empty")
	}
	if net.PublicKey == "" {
		t.Error("public_key should not be empty")
	}
	if net.Main.Cidr != "10.0.0.0/16" {
		t.Errorf("main.cidr = %q, want 10.0.0.0/16", net.Main.Cidr)
	}
	if net.Invite.Cidr != "10.1.0.0/24" {
		t.Errorf("invite.cidr = %q, want 10.1.0.0/24", net.Invite.Cidr)
	}
	if net.ExternalIP != "1.2.3.4" {
		t.Errorf("external_ip = %q, want 1.2.3.4", net.ExternalIP)
	}
	if net.Main.WireguardPort != 51820 {
		t.Errorf("main.wg_port = %d, want 51820", net.Main.WireguardPort)
	}
	if net.Invite.WireguardPort != 51821 {
		t.Errorf("invite.wg_port = %d, want 51821", net.Invite.WireguardPort)
	}
	if net.Main.ApiPort != 8080 {
		t.Errorf("main.api_port = %d, want 8080", net.Main.ApiPort)
	}
	if net.Invite.ApiPort != 8080 {
		t.Errorf("invite.api_port = %d, want 8080", net.Invite.ApiPort)
	}
	if !net.CreatedAt.Equal(testutil.FixedTime) {
		t.Fatalf("created at = %v", net.CreatedAt)
	}
	publicKey, err := wireguard.PublicKey(net.PrivateKey)
	if err != nil || publicKey != net.PublicKey {
		t.Fatalf("derived key = %q, %v", publicKey, err)
	}
	stored, err := env.Service.GetNetwork("mynet")
	if err != nil {
		t.Fatal(err)
	}
	if !stored.CreatedAt.Equal(net.CreatedAt) {
		t.Fatalf("stored timestamp = %v, want %v", stored.CreatedAt, net.CreatedAt)
	}
	stored.CreatedAt = net.CreatedAt
	if !reflect.DeepEqual(stored, net) {
		t.Fatalf("stored network = %+v, want %+v", stored, net)
	}
}

func TestCreateNetwork_PersistsDefaults(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	nw, err := env.Service.CreateNetwork("defaults", "192.0.2.1", service.Plane{Cidr: "10.0.0.0/16"}, service.Plane{})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := env.Service.GetNetwork("defaults")
	if err != nil {
		t.Fatal(err)
	}
	if !stored.CreatedAt.Equal(nw.CreatedAt) {
		t.Fatalf("stored timestamp = %v, want %v", stored.CreatedAt, nw.CreatedAt)
	}
	stored.CreatedAt = nw.CreatedAt
	if !reflect.DeepEqual(stored, nw) {
		t.Fatalf("stored network = %+v, want %+v", stored, nw)
	}
	if nw.Invite.Cidr != "172.16.10.0/24" || nw.Main.WireguardPort != 51820 || nw.Invite.WireguardPort != 51821 || nw.Main.ApiPort != 8080 || nw.Invite.ApiPort != 8080 {
		t.Fatalf("defaults = %+v", nw)
	}
}

func TestCreateNetwork_RejectsInvalidConfigurationWithoutWrites(
	t *testing.T,
) {
	for _, tc := range []struct {
		name, ip, main, invite string
		want                   error
	}{
		{"main syntax", "192.0.2.1", "invalid", "10.1.0.0/24", service.ErrInvalidInput},
		{"main host bits", "192.0.2.1", "10.0.99.0/16", "10.1.0.0/24", service.ErrInvalidInput},
		{"invite syntax", "192.0.2.1", "10.0.0.0/16", "invalid", service.ErrInvalidInput},
		{"invite host bits", "192.0.2.1", "10.0.0.0/16", "10.1.0.99/24", service.ErrInvalidInput},
		{"missing external IP", "", "10.0.0.0/16", "10.1.0.0/24", service.ErrInvalidInput},
		{"overlap", "192.0.2.1", "10.0.0.0/16", "10.0.1.0/24", service.ErrCIDROverlap},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := testutil.SetupService(t)
			if _, err := env.Service.CreateNetwork("invalid", tc.ip, service.Plane{Cidr: tc.main}, service.Plane{Cidr: tc.invite}); !errors.Is(err, tc.want) {
				t.Fatalf("create: %v, want %v", err, tc.want)
			}
			networks, err := env.Service.ListNetworks()
			if err != nil || len(networks) != 0 {
				t.Fatalf("networks after rejection = %+v, %v", networks, err)
			}
			cidrs, err := env.Service.ListCidrs("invalid")
			if err != nil || len(cidrs) != 0 {
				t.Fatalf("CIDRs after rejection = %+v, %v", cidrs, err)
			}
		})
	}
}

func TestCreateNetwork_DuplicateName(
	t *testing.T,
) {
	env := testutil.SetupService(t)

	_, err := env.Service.CreateNetwork(
		"dup",
		"1.2.3.4",
		service.Plane{Cidr: "10.0.0.0/16", WireguardPort: 51820, ApiPort: 8080},
		service.Plane{Cidr: "10.1.0.0/24", WireguardPort: 51821, ApiPort: 8080},
	)
	if err != nil {
		t.Fatalf("first create: %v", err)
	}
	before, err := env.Service.GetNetwork("dup")
	if err != nil {
		t.Fatal(err)
	}

	_, err = env.Service.CreateNetwork(
		"dup",
		"1.2.3.4",
		service.Plane{Cidr: "10.0.0.0/16", WireguardPort: 51820, ApiPort: 8080},
		service.Plane{Cidr: "10.1.0.0/24", WireguardPort: 51821, ApiPort: 8080},
	)
	if !errors.Is(err, service.ErrNetworkExists) {
		t.Errorf("err = %v, want ErrNetworkExists", err)
	}
	after, err := env.Service.GetNetwork("dup")
	if err != nil || !reflect.DeepEqual(after, before) {
		t.Fatalf("network after duplicate = %+v, %v", after, err)
	}
}

func TestCreateNetwork_NameValidation(
	t *testing.T,
) {
	cases := []struct {
		name    string
		wantErr string
	}{
		{"mynet", ""},
		{"a", ""},
		{"0net", ""},
		{"my-net-2", ""},
		{"", "invalid network name"},
		{"Mynet", "only lowercase"},
		{"my.net", "only lowercase"},
		{"my_net", "only lowercase"},
		{"my net", "only lowercase"},
		{"naïve", "only lowercase"},
		{"-net", "start with a hyphen"},
		{"net-", "end with a hyphen"},
		{"sixteencharname1", "device name"},
		{strings.Repeat("a", 63), "device name"},
		{strings.Repeat("a", 64), "DNS label"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := testutil.SetupService(t)

			network, err := env.Service.CreateNetwork(
				tc.name,
				"1.2.3.4",
				service.Plane{Cidr: "10.0.0.0/16"},
				service.Plane{Cidr: "10.1.0.0/24"},
			)
			if tc.wantErr != "" {
				wantInvalidInput(t, err, tc.wantErr)
				if _, err := env.Service.GetNetwork(tc.name); !errors.Is(err, service.ErrNotFound) {
					t.Fatalf("network %q should not persist after rejection: %v", tc.name, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("create network %q: %v", tc.name, err)
			}
			if network.Name != tc.name {
				t.Errorf("name = %q, want %q", network.Name, tc.name)
			}
		})
	}
}

func TestCreateNetwork_CanonicalizesStoredCIDRs(
	t *testing.T,
) {
	env := testutil.SetupService(t)

	nw, err := env.Service.CreateNetwork(
		"canonical",
		"1.2.3.4",
		service.Plane{Cidr: "FD00:0:0:0::/64"},
		service.Plane{Cidr: "FD01:0:0:0::/64"},
	)
	if err != nil {
		t.Fatalf("create network: %v", err)
	}
	if nw.Main.Cidr != "fd00::/64" || nw.Invite.Cidr != "fd01::/64" {
		t.Fatalf("created CIDRs = %q, %q; want canonical IPv6", nw.Main.Cidr, nw.Invite.Cidr)
	}

	stored, err := env.Service.GetNetwork("canonical")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if stored.Main.Cidr != "fd00::/64" || stored.Invite.Cidr != "fd01::/64" {
		t.Fatalf("stored CIDRs = %q, %q; want canonical IPv6", stored.Main.Cidr, stored.Invite.Cidr)
	}

	cidrs, err := env.Service.ListCidrs("canonical")
	if err != nil {
		t.Fatalf("list CIDRs: %v", err)
	}
	foundRoot := false
	for _, cidr := range cidrs {
		if cidr.Name == "canonical" {
			foundRoot = true
			if cidr.Cidr != "fd00::/64" {
				t.Fatalf("stored root CIDR = %q, want fd00::/64", cidr.Cidr)
			}
		}
	}
	if !foundRoot {
		t.Fatal("stored root CIDR not found")
	}
}

func TestGetNetwork_NotFound(
	t *testing.T,
) {
	env := testutil.SetupService(t)

	_, err := env.Service.GetNetwork("nonexistent")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestListNetworks_Success(
	t *testing.T,
) {
	env := testutil.SetupService(t)

	networks, err := env.Service.ListNetworks()
	if err != nil {
		t.Fatalf("list empty: %v", err)
	}
	if len(networks) != 0 {
		t.Fatalf("expected 0 networks, got %d", len(networks))
	}

	testutil.SeedNetwork(t, env.Service)
	if _, err := env.Service.CreateNetwork("alpha", "192.0.2.2", service.Plane{Cidr: "10.2.0.0/16"}, service.Plane{Cidr: "10.3.0.0/24"}); err != nil {
		t.Fatal(err)
	}
	networks, err = env.Service.ListNetworks()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(networks) != 2 || networks[0].Name != "alpha" || networks[1].Name != "testnet" {
		t.Errorf("networks = %v, want [alpha testnet]", networks)
	}
}

func TestSetNetworkEnabled_PersistsIntent(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	network, err := env.Service.GetNetwork("testnet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if !network.Enabled {
		t.Fatal("network should be enabled")
	}

	if err := env.Service.SetNetworkEnabled("testnet", false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	network, err = env.Service.GetNetwork("testnet")
	if err != nil {
		t.Fatalf("get network: %v", err)
	}
	if network.Enabled {
		t.Fatal("network should be disabled")
	}
}

func TestSetNetworkEnabled_NotFound(
	t *testing.T,
) {
	env := testutil.SetupService(t)

	if err := env.Service.SetNetworkEnabled("ghost", true); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteNetwork_RefusesEnabled(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if err := env.Service.SetNetworkEnabled("testnet", true); err != nil {
		t.Fatalf("enable: %v", err)
	}

	if err := env.Service.DeleteNetwork("testnet"); !errors.Is(err, service.ErrNetworkEnabled) {
		t.Fatalf("err = %v, want ErrNetworkEnabled", err)
	}
	if _, err := env.Service.GetNetwork("testnet"); err != nil {
		t.Fatalf("network should survive a refused delete: %v", err)
	}
}

func TestDeleteNetwork_RemovesResourcesBeforeRecreation(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)
	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Service.RedeemRegistration("testnet", lastTempKey(t, env.Service, "testnet"), mustGenKey(t)); err != nil {
		t.Fatal(err)
	}
	if _, err := env.Service.CreateGroup("testnet", "staff"); err != nil {
		t.Fatal(err)
	}
	if err := env.Service.AssignRegistrationGroup("testnet", "alice", "staff"); err != nil {
		t.Fatal(err)
	}
	if err := env.Service.CreateAssociation("testnet", "staff", "staff"); err != nil {
		t.Fatal(err)
	}

	if err := env.Service.DeleteNetwork("testnet"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	_, err := env.Service.GetNetwork("testnet")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("after delete: err = %v, want ErrNotFound", err)
	}
	// Recreating the namespace must not resurrect the old resources.
	testutil.SeedNetwork(t, env.Service)
	registrations, err := env.Service.ListRegistrations("testnet")
	if err != nil || len(registrations) != 0 {
		t.Fatalf("registrations after recreation = %+v, %v", registrations, err)
	}
	groups, err := env.Service.ListGroups("testnet")
	if err != nil || len(groups) != 0 {
		t.Fatalf("groups after recreation = %+v, %v", groups, err)
	}
	associations, err := env.Service.ListAssociations("testnet")
	if err != nil || len(associations) != 0 {
		t.Fatalf("associations after recreation = %+v, %v", associations, err)
	}
	peers, err := env.Service.ListPeers("testnet")
	if err != nil || len(peers) != 1 || peers[0].Name != "cord-server" {
		t.Fatalf("peers after recreation = %+v, %v", peers, err)
	}
	if _, err := env.Service.GetCidr("testnet", "alice"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("old peer CIDR after recreation: %v", err)
	}
	if _, err := env.Service.CreateRegistration("testnet", "alice", service.RegistrationOptions{PeerIP: net.ParseIP("10.0.0.5")}); err != nil {
		t.Fatalf("reuse registration name and address: %v", err)
	}
}

func TestDeleteNetwork_NotFound(
	t *testing.T,
) {
	env := testutil.SetupService(t)

	err := env.Service.DeleteNetwork("ghost")
	if !errors.Is(err, service.ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteNetwork_InvalidatesCompiledModel(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	testutil.SeedNetwork(t, env.Service)

	if _, err := env.Service.GetNetworkTopology("testnet"); err != nil {
		t.Fatalf("get topology before delete: %v", err)
	}
	if err := env.Service.DeleteNetwork("testnet"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := env.Service.GetNetworkTopology("testnet"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("topology after delete: err = %v, want ErrNotFound", err)
	}
}
