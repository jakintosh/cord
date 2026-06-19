package server_test

import (
	"encoding/json"
	"net"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/cord/internal/database"
	"git.sr.ht/~jakintosh/cord/internal/server"
)

func TestFsConfigListConfigsMissingDirReturnsEmpty(t *testing.T) {
	cfg := server.NewFsConfig(path.Join(t.TempDir(), "absent"))

	names, err := cfg.ListConfigs()
	if err != nil {
		t.Fatalf("expected no error for missing directory, got: %v", err)
	}
	if len(names) != 0 {
		t.Fatalf("expected no configs, got %v", names)
	}
}

func TestFsConfigListConfigsFiltersAndSortsTomlFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "beta.toml")
	writeFile(t, dir, "alpha.toml")
	writeFile(t, dir, "junk.txt")

	cfg := server.NewFsConfig(dir)
	names, err := cfg.ListConfigs()
	if err != nil {
		t.Fatalf("failed to list configs: %v", err)
	}

	expectNames(t, names, []string{"alpha", "beta"})
}

func TestMemConfigListConfigsReturnsSortedNames(t *testing.T) {
	cfg := server.NewMemConfig()
	for _, name := range []string{"beta.toml", "alpha.toml"} {
		if _, err := cfg.GetConfigWriter(name); err != nil {
			t.Fatalf("failed to create config '%s': %v", name, err)
		}
	}

	names, err := cfg.ListConfigs()
	if err != nil {
		t.Fatalf("failed to list configs: %v", err)
	}

	expectNames(t, names, []string{"alpha", "beta"})
}

func TestListNetworksReturnsSummaries(t *testing.T) {
	cfg := server.NewMemConfig()
	createNetworkOn(t, cfg, NetworkDesc{
		Name: "net-a",
		Cidr: "10.1.0.0/16",
		Ip:   net.IPv4(1, 1, 1, 1),
		Port: 10000,
	})
	createNetworkOn(t, cfg, NetworkDesc{
		Name: "net-b",
		Cidr: "10.2.0.0/16",
		Ip:   net.IPv4(2, 2, 2, 2),
		Port: 20000,
	})

	summaries, err := server.ListNetworks(cfg)
	if err != nil {
		t.Fatalf("failed to list networks: %v", err)
	}

	if len(summaries) != 2 {
		t.Fatalf("expected 2 networks, got %d", len(summaries))
	}
	if summaries[0].Name != "net-a" || summaries[1].Name != "net-b" {
		t.Fatalf("unexpected network names: %v", summaries)
	}
	if summaries[0].RootCidr != "10.1.0.0/16" {
		t.Errorf("expected root cidr 10.1.0.0/16, got %s", summaries[0].RootCidr)
	}
	if summaries[0].ExternalEndpoint != "1.1.1.1:10000" {
		t.Errorf("expected endpoint 1.1.1.1:10000, got %s", summaries[0].ExternalEndpoint)
	}
}

func TestListNetworksEmptyConfigReturnsEmpty(t *testing.T) {
	summaries, err := server.ListNetworks(server.NewMemConfig())
	if err != nil {
		t.Fatalf("failed to list networks: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("expected no networks, got %v", summaries)
	}
}

func TestListNetworksOmitsPrivateKey(t *testing.T) {
	cfg := server.NewMemConfig()
	srv := createNetworkOn(t, cfg, testNetwork)

	netCfg, err := srv.LoadNetwork()
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	summaries, err := server.ListNetworks(cfg)
	if err != nil {
		t.Fatalf("failed to list networks: %v", err)
	}

	marshaled, err := json.Marshal(summaries)
	if err != nil {
		t.Fatalf("failed to marshal summaries: %v", err)
	}
	if strings.Contains(string(marshaled), netCfg.PrivateKey) {
		t.Fatal("network summary leaked the private key")
	}
}

func TestGetNetworkOverviewReportsConfigFields(t *testing.T) {
	srv := mustCreateBaseNetwork(t)

	overview, err := srv.GetNetworkOverview()
	if err != nil {
		t.Fatalf("failed to get overview: %v", err)
	}

	if overview.Name != testNetwork.Name {
		t.Errorf("expected name %s, got %s", testNetwork.Name, overview.Name)
	}
	if overview.RootCidr != testNetwork.Cidr {
		t.Errorf("expected root cidr %s, got %s", testNetwork.Cidr, overview.RootCidr)
	}
	if overview.InviteCidr != testInviteCidr {
		t.Errorf("expected invite cidr %s, got %s", testInviteCidr, overview.InviteCidr)
	}
	if overview.PublicKey == "" {
		t.Error("expected a public key in the overview")
	}
}

func TestGetNetworkOverviewCountsResources(t *testing.T) {
	srv := mustCreateBaseNetwork(t)
	mustAddAndRedeemPeer(t, srv, testServer)
	mustAddPeer(t, srv, testUser) // active invite, not yet a confirmed peer
	mustAssociate(t, srv, infraCidr.Name, fleetCidr.Name)

	overview, err := srv.GetNetworkOverview()
	if err != nil {
		t.Fatalf("failed to get overview: %v", err)
	}

	// root + infra + fleet
	if overview.CidrCount != 3 {
		t.Errorf("expected 3 cidrs, got %d", overview.CidrCount)
	}
	// cord-server peer + redeemed test-server; test-user is only invited
	if overview.PeerCount != 2 {
		t.Errorf("expected 2 peers, got %d", overview.PeerCount)
	}
	if overview.ActiveInvites != 1 {
		t.Errorf("expected 1 active invite, got %d", overview.ActiveInvites)
	}
	if overview.AssociationCount != 1 {
		t.Errorf("expected 1 association, got %d", overview.AssociationCount)
	}
}

func TestListCidrDetailsIncludesAssociations(t *testing.T) {
	srv := mustCreateBaseNetwork(t)
	mustAssociate(t, srv, infraCidr.Name, fleetCidr.Name)

	details, err := srv.ListCidrDetails()
	if err != nil {
		t.Fatalf("failed to list cidr details: %v", err)
	}

	expectCidrAssociations(t, details, infraCidr.Name, []string{fleetCidr.Name})
	expectCidrAssociations(t, details, fleetCidr.Name, []string{infraCidr.Name})
}

func TestListCidrDetailsUnassociatedCidrHasEmptyList(t *testing.T) {
	srv := mustCreateBaseNetwork(t)

	details, err := srv.ListCidrDetails()
	if err != nil {
		t.Fatalf("failed to list cidr details: %v", err)
	}

	expectCidrAssociations(t, details, infraCidr.Name, []string{})
}

func TestListPeerStatusesReportsFlags(t *testing.T) {
	srv := mustCreateBaseNetwork(t)
	admin := PeerDesc{Name: "admin-peer", Ip: net.IPv4(10, 0, 64, 10), Admin: true}
	mustAddAndRedeemPeer(t, srv, admin)

	status := findPeerStatus(t, srv, admin.Name)

	if !status.Admin {
		t.Error("expected peer to be admin")
	}
	if !status.Enabled {
		t.Error("expected peer to be enabled")
	}
	if !status.Confirmed {
		t.Error("expected peer to be confirmed")
	}
}

func TestListPeerStatusesIncludesNewestEndpoint(t *testing.T) {
	srv := mustCreateBaseNetwork(t)
	mustAddAndRedeemPeer(t, srv, testServer)
	key := peerKey(t, srv, testServer.Name)
	witness := peerKey(t, srv, cordServerPeer.Name)

	now := time.Now().Unix()
	mustReportEndpoint(t, srv, key, witness, "203.0.113.5:1000", now-60)
	mustReportEndpoint(t, srv, key, witness, "203.0.113.5:2000", now)

	status := findPeerStatus(t, srv, testServer.Name)

	if status.LastEndpoint != "203.0.113.5:2000" {
		t.Errorf("expected newest endpoint 203.0.113.5:2000, got %s", status.LastEndpoint)
	}
	if status.LastSeen != now {
		t.Errorf("expected last seen %d, got %d", now, status.LastSeen)
	}
}

func TestListPeerStatusesExcludesStaleEndpoints(t *testing.T) {
	srv := mustCreateBaseNetwork(t)
	mustAddAndRedeemPeer(t, srv, testServer)
	key := peerKey(t, srv, testServer.Name)
	witness := peerKey(t, srv, cordServerPeer.Name)

	stale := time.Now().Add(-25 * time.Hour).Unix()
	mustReportEndpoint(t, srv, key, witness, "203.0.113.5:1000", stale)

	status := findPeerStatus(t, srv, testServer.Name)

	if status.LastEndpoint != "" {
		t.Errorf("expected no endpoint for stale sighting, got %s", status.LastEndpoint)
	}
}

func TestListInvitesReportsActiveAndRedeemed(t *testing.T) {
	srv := mustCreateBaseNetwork(t)
	mustAddPeer(t, srv, testUser)
	tempCidr := mustAddPeerReturningCidr(t, srv, testUser2)
	mustRedeemWithoutConfirm(t, srv, tempCidr)

	invites, err := srv.ListInvites()
	if err != nil {
		t.Fatalf("failed to list invites: %v", err)
	}

	if len(invites) != 2 {
		t.Fatalf("expected 2 invites, got %d", len(invites))
	}
	if findInvite(t, invites, testUser.Name).Redeemed {
		t.Error("expected unredeemed invite for test-user")
	}
	if !findInvite(t, invites, testUser2.Name).Redeemed {
		t.Error("expected redeemed invite for test-user-2")
	}
}

func TestListInvitesExcludesConfirmedPeers(t *testing.T) {
	srv := mustCreateBaseNetwork(t)
	mustAddAndRedeemPeer(t, srv, testUser)

	invites, err := srv.ListInvites()
	if err != nil {
		t.Fatalf("failed to list invites: %v", err)
	}

	if len(invites) != 0 {
		t.Fatalf("expected no invites after confirmation, got %v", invites)
	}
}

// helpers

func writeFile(t *testing.T, dir string, name string) {
	t.Helper()
	if err := os.WriteFile(path.Join(dir, name), []byte("name = \"x\"\n"), 0600); err != nil {
		t.Fatalf("failed to write file '%s': %v", name, err)
	}
}

func expectNames(t *testing.T, got []string, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected names %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected names %v, got %v", want, got)
		}
	}
}

func mustCreateBaseNetwork(t *testing.T) *server.Server {
	t.Helper()
	srv, err := createBaseNetwork()
	if err != nil {
		t.Fatalf("failed to create base network: %v", err)
	}
	return srv
}

func createNetworkOn(t *testing.T, cfg server.Config, desc NetworkDesc) *server.Server {
	t.Helper()
	store, err := database.OpenServer(database.Options{
		Name: desc.Name,
		Dir:  ":memory:",
	})
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	srv, err := server.New(server.Options{
		Network: desc.Name,
		Config:  cfg,
		Store:   store,
	})
	if err != nil {
		t.Fatalf("failed to init test server: %v", err)
	}
	if err := addNetwork(srv, desc); err != nil {
		t.Fatalf("failed to add network '%s': %v", desc.Name, err)
	}
	return srv
}

func mustAddPeer(t *testing.T, srv *server.Server, desc PeerDesc) {
	t.Helper()
	mustAddPeerReturningCidr(t, srv, desc)
}

func mustAddPeerReturningCidr(t *testing.T, srv *server.Server, desc PeerDesc) string {
	t.Helper()
	cidr, err := addPeer(srv, desc)
	if err != nil {
		t.Fatalf("failed to add peer '%s': %v", desc.Name, err)
	}
	return cidr
}

func mustAddAndRedeemPeer(t *testing.T, srv *server.Server, desc PeerDesc) {
	t.Helper()
	if err := addAndRedeemPeer(srv, desc); err != nil {
		t.Fatalf("failed to add and redeem peer '%s': %v", desc.Name, err)
	}
}

func mustRedeemWithoutConfirm(t *testing.T, srv *server.Server, tempCidr string) {
	t.Helper()
	ip, _, err := net.ParseCIDR(tempCidr)
	if err != nil {
		t.Fatalf("failed to parse temp cidr '%s': %v", tempCidr, err)
	}
	invite, err := srv.Store.InviteGetByIP(ip)
	if err != nil {
		t.Fatalf("failed to get invite for ip %v: %v", ip, err)
	}
	if _, err := srv.RedeemInvite(invite, invite.PublicKey); err != nil {
		t.Fatalf("failed to redeem invite: %v", err)
	}
}

func mustAssociate(t *testing.T, srv *server.Server, cidr1 string, cidr2 string) {
	t.Helper()
	if err := srv.CreateAssociation(cidr1, cidr2); err != nil {
		t.Fatalf("failed to associate '%s' and '%s': %v", cidr1, cidr2, err)
	}
}

func mustReportEndpoint(
	t *testing.T,
	srv *server.Server,
	peerKey string,
	witnessKey string,
	endpoint string,
	timestamp int64,
) {
	t.Helper()
	err := srv.ReportEndpoints([]server.EndpointSighting{{
		PeerKey:    peerKey,
		WitnessKey: witnessKey,
		Endpoint:   endpoint,
		Timestamp:  timestamp,
	}})
	if err != nil {
		t.Fatalf("failed to report endpoint: %v", err)
	}
}

func peerKey(t *testing.T, srv *server.Server, name string) string {
	t.Helper()
	peer, err := srv.GetPeer(name)
	if err != nil {
		t.Fatalf("failed to get peer '%s': %v", name, err)
	}
	return peer.PublicKey
}

func findPeerStatus(t *testing.T, srv *server.Server, name string) server.PeerStatus {
	t.Helper()
	statuses, err := srv.ListPeerStatuses()
	if err != nil {
		t.Fatalf("failed to list peer statuses: %v", err)
	}
	for _, status := range statuses {
		if status.Name == name {
			return status
		}
	}
	t.Fatalf("no peer status for '%s' in %v", name, statuses)
	return server.PeerStatus{}
}

func findInvite(t *testing.T, invites []server.InviteStatus, name string) server.InviteStatus {
	t.Helper()
	for _, invite := range invites {
		if invite.Name == name {
			return invite
		}
	}
	t.Fatalf("no invite for '%s' in %v", name, invites)
	return server.InviteStatus{}
}

func expectCidrAssociations(
	t *testing.T,
	details []server.CidrDetail,
	name string,
	want []string,
) {
	t.Helper()
	for _, detail := range details {
		if detail.Name != name {
			continue
		}
		expectNames(t, detail.Associations, want)
		return
	}
	t.Fatalf("no cidr detail for '%s' in %v", name, details)
}
