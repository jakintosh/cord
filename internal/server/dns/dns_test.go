package dns_test

import (
	"context"
	"errors"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/resource"
	serverdns "git.studiopollinator.com/pollinator/cord/internal/server/dns"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"github.com/miekg/dns"
)

func TestNew_RejectsInvalidOptions(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	for _, tt := range []struct {
		name, network, zone, address string
		missingService               bool
	}{
		{"missing network", "", "x.internal", "127.0.0.1:0", false},
		{"missing zone", "looptest", "", "127.0.0.1:0", false},
		{"malformed zone", "looptest", "bad..internal", "127.0.0.1:0", false},
		{"missing port", "looptest", "x.internal", "127.0.0.1", false},
		{"hostname", "looptest", "x.internal", "localhost:53", false},
		{"large port", "looptest", "x.internal", "127.0.0.1:70000", false},
		{"non-numeric port", "looptest", "x.internal", "127.0.0.1:dns", false},
		{"missing service", "looptest", "x.internal", "127.0.0.1:0", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			opts := serverdns.Options{
				Network: tt.network,
				Zone:    tt.zone,
				Address: tt.address,
				Service: env.Service,
			}
			if tt.missingService {
				opts.Service = nil
			}
			if srv, err := serverdns.New(opts); err == nil || srv != nil {
				t.Fatalf("New = %v, %v, want nil server and error", srv, err)
			}
		})
	}
}

func TestReconcile_CancelledStartRecovers(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	seedZone(t, env, "looptest", "127.0.0.0/24")
	srv, err := serverdns.New(serverdns.Options{
		Network: "looptest",
		Zone:    "looptest.internal",
		Address: "127.0.0.1:0",
		Service: env.Service,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Close(context.Background()) })
	if srv.Address() != "127.0.0.1:0" || srv.Status().State != resource.Stopped {
		t.Fatalf("new server = %s, %+v", srv.Address(), srv.Status())
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := srv.Reconcile(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("Reconcile = %v", err)
	}
	if status := srv.Status(); status.State != resource.Failed || !errors.Is(status.Error, context.Canceled) {
		t.Fatalf("status = %+v", status)
	}
	if srv.Address() != "127.0.0.1:0" {
		t.Fatalf("cancelled startup bound address %s", srv.Address())
	}
	if err := srv.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if srv.Address() == "127.0.0.1:0" || srv.Status().State != resource.Running || srv.Status().Error != nil {
		t.Fatalf("recovered server = %s, %+v", srv.Address(), srv.Status())
	}
}

// startLoopbackDNS starts a real server for a loopback network whose
// server peer is 127.0.0.1, so client exchanges from loopback resolve.
// All socket tests use an OS-assigned ephemeral port read back from
// the bound server; nothing here needs privileges or a fixed port.
func startLoopbackDNS(
	t *testing.T,
) *serverdns.Server {
	t.Helper()

	env := testutil.SetupService(t)
	seedZone(t, env, "looptest", "127.0.0.0/24")

	return startDNS(t, env, "127.0.0.1:0")
}

func startDNS(
	t *testing.T,
	env *testutil.ServiceEnv,
	address string,
) *serverdns.Server {
	t.Helper()
	return startNetworkDNS(t, env, "looptest", address)
}

func startNetworkDNS(
	t *testing.T,
	env *testutil.ServiceEnv,
	network string,
	address string,
) *serverdns.Server {
	t.Helper()
	srv, err := serverdns.New(serverdns.Options{
		Network: network,
		Zone:    network + ".Internal",
		Address: address,
		Service: env.Service,
	})
	if err != nil {
		t.Fatalf("new dns server: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := srv.Close(ctx); err != nil {
			t.Errorf("cleanup DNS: %v", err)
		}
	})
	if err := srv.Reconcile(t.Context()); err != nil {
		t.Fatalf("start dns server: %v", err)
	}

	return srv
}

func TestReconcile_IsolatesNetworksSharingPort(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	seedZone(t, env, "alpha", "127.0.0.0/24")
	seedZone(t, env, "beta", "::/120")
	testutil.SeedPeerDB(
		t,
		env.Database,
		"alpha",
		"api",
		"127.0.0.2/32",
		"alpha-api",
		false,
		true,
		true,
	)
	testutil.SeedPeerDB(
		t,
		env.Database,
		"beta",
		"api",
		"::2/128",
		"beta-api",
		false,
		true,
		true,
	)

	// Each server can see its own network's peer named api.
	for _, network := range []string{"alpha", "beta"} {
		if _, err := env.Service.CreateGroup(network, "shared"); err != nil {
			t.Fatal(err)
		}
		for _, cidr := range []string{"cord-server", "api"} {
			if err := env.Service.AssignCidrGroup(network, cidr, "shared"); err != nil {
				t.Fatal(err)
			}
		}
		if err := env.Service.CreateAssociation(network, "shared", "shared"); err != nil {
			t.Fatal(err)
		}
	}

	// These addresses are available without interface aliases on macOS and
	// Linux. Share one ephemeral port across two distinct address families.
	alpha := startNetworkDNS(t, env, "alpha", "127.0.0.1:0")
	_, port, err := net.SplitHostPort(alpha.Address())
	if err != nil {
		t.Fatal(err)
	}
	betaAddress := net.JoinHostPort("::1", port)
	beta := startNetworkDNS(t, env, "beta", betaAddress)

	checkAlpha := func() {
		t.Helper()
		assertDNSZoneIsolation(
			t,
			alpha,
			"127.0.0.1",
			"alpha.internal.",
			"beta.internal.",
			dns.TypeA,
			"api.alpha.internal.\t3600\tIN\tA\t127.0.0.2",
		)
	}
	checkBeta := func() {
		t.Helper()
		assertDNSZoneIsolation(
			t,
			beta,
			"::1",
			"beta.internal.",
			"alpha.internal.",
			dns.TypeAAAA,
			"api.beta.internal.\t3600\tIN\tAAAA\t::2",
		)
	}

	// Both networks answer their own zone and refuse the other zone.
	checkAlpha()
	checkBeta()

	// Closing beta must leave alpha answering on its original address.
	alphaAddress := alpha.Address()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := beta.Close(ctx); err != nil {
		t.Fatal(err)
	}
	betaStatus := beta.Status()
	if betaStatus.State != resource.Stopped {
		t.Fatalf("beta after close = %+v", betaStatus)
	}
	checkAlpha()

	// Restart beta on the shared port and check both networks again.
	if err := beta.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	checkBeta()
	checkAlpha()
	if address := alpha.Address(); address != alphaAddress {
		t.Fatalf("alpha address = %s, want %s", address, alphaAddress)
	}
	alphaStatus := alpha.Status()
	if alphaStatus.State != resource.Running {
		t.Fatalf("alpha after beta restart = %+v", alphaStatus)
	}
}

func assertDNSZoneIsolation(
	t *testing.T,
	srv *serverdns.Server,
	source string,
	zone string,
	otherZone string,
	qtype uint16,
	wantAnswer string,
) {
	t.Helper()

	for _, protocol := range []string{"udp", "tcp"} {
		question := new(dns.Msg)
		question.SetQuestion("api."+zone, qtype)
		response := exchangeDNS(
			t,
			protocol,
			source,
			srv.Address(),
			question,
		)
		if response.Rcode != dns.RcodeSuccess {
			t.Fatalf("%s %s response = %v", zone, protocol, response)
		}
		if !response.Authoritative || response.RecursionAvailable {
			t.Fatalf("unexpected response flags: %+v", response.MsgHdr)
		}
		assertRecords(t, response.Answer, wantAnswer)

		otherQuestion := new(dns.Msg)
		otherQuestion.SetQuestion("api."+otherZone, qtype)
		response = exchangeDNS(
			t,
			protocol,
			source,
			srv.Address(),
			otherQuestion,
		)
		if response.Rcode != dns.RcodeRefused || response.Authoritative {
			t.Fatalf("%s answered for the other zone: %v", zone, response)
		}
		assertRecords(t, response.Answer, "")
		assertRecords(t, response.Ns, "")
		assertRecords(t, response.Extra, "")
	}
}

// seedZone creates a one-peer-less network; the bootstrap server peer
// on the loopback CIDR is the only identity the socket tests need.
func seedZone(
	t *testing.T,
	env *testutil.ServiceEnv,
	name string,
	cidr string,
) {
	t.Helper()

	if _, err := env.Service.CreateNetwork(
		name,
		"192.168.1.1",
		service.Plane{Cidr: cidr, WireguardPort: 51820, ApiPort: 8080},
		service.Plane{Cidr: "172.16.10.0/24", WireguardPort: 51821, ApiPort: 8080},
	); err != nil {
		t.Fatalf("seed network %s: %v", name, err)
	}
}

func TestReconcile_PartialBindRollback(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	seedZone(t, env, "looptest", "127.50.0.1/32")

	// The test itself holds the TCP port, so the server's TCP bind
	// fails after its UDP bind succeeded.
	blocker, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("hold tcp port: %v", err)
	}
	defer blocker.Close()
	port := blocker.Addr().(*net.TCPAddr).Port

	srv, err := serverdns.New(serverdns.Options{
		Network: "looptest",
		Zone:    "looptest.internal",
		Address: net.JoinHostPort("127.0.0.1", strconv.Itoa(port)),
		Service: env.Service,
	})
	if err != nil {
		t.Fatalf("new dns server: %v", err)
	}

	t.Cleanup(func() { _ = srv.Close(context.Background()) })

	startErr := srv.Reconcile(t.Context())
	if startErr == nil || !strings.Contains(startErr.Error(), "listen tcp") {
		t.Fatalf("start error = %v, want a tcp bind failure", startErr)
	}

	status := srv.Status()
	if status.State != resource.Failed {
		t.Fatalf("status after failed start = %+v, want not serving", status)
	}
	if status.Error == nil {
		t.Fatal("status after failed start should record the error")
	}

	// The rolled-back UDP socket must be released.
	released, err := net.ListenPacket("udp", blocker.Addr().String())
	if err != nil {
		t.Fatalf("udp socket was not released on rollback: %v", err)
	}
	_ = released.Close()

	// A failed start leaves the server restartable.
	if err := blocker.Close(); err != nil {
		t.Fatalf("release tcp port: %v", err)
	}
	if err := srv.Reconcile(t.Context()); err != nil {
		t.Fatalf("restart after failed start: %v", err)
	}
	if srv.Status().State != resource.Running {
		t.Fatal("restarted server should be serving")
	}
	if err := srv.Close(context.Background()); err != nil {
		t.Fatalf("stop after restart: %v", err)
	}
}

func TestClose_StopsAndPermitsRestart(
	t *testing.T,
) {
	srv := startLoopbackDNS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Close(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if srv.Status().State != resource.Stopped {
		t.Fatal("server should be stopped after Close")
	}

	// Close is safe to call twice.
	if err := srv.Close(ctx); err != nil {
		t.Fatalf("second stop: %v", err)
	}

	// The sockets are released and the server is restartable.
	if err := srv.Reconcile(t.Context()); err != nil {
		t.Fatalf("start after stop: %v", err)
	}
	if srv.Status().State != resource.Running {
		t.Fatal("restarted server should be serving")
	}
	if err := srv.Close(context.Background()); err != nil {
		t.Fatalf("final stop: %v", err)
	}
}

func TestStatus_ReportsRunning(
	t *testing.T,
) {
	srv := startLoopbackDNS(t)

	status := srv.Status()
	if status.State != resource.Running {
		t.Fatalf("status = %+v, want serving", status)
	}

	if status.Error != nil {
		t.Fatalf("status error = %q, want empty", status.Error)
	}
}

func TestReconcile_ServesQueriesAfterRestart(
	t *testing.T,
) {
	srv := startLoopbackDNS(t)
	if err := srv.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := srv.Reconcile(t.Context()); err != nil {
		t.Fatalf("reconcile after stop: %v", err)
	}
	address := srv.Address()
	if err := srv.Reconcile(t.Context()); err != nil {
		t.Fatalf("reconcile healthy server: %v", err)
	}
	if srv.Address() != address {
		t.Fatal("healthy reconciliation changed the listening address")
	}
	for _, network := range []string{"udp", "tcp"} {
		client := &dns.Client{Net: network, Timeout: time.Second}
		query := new(dns.Msg).SetQuestion("looptest.internal.", dns.TypeSOA)
		response, _, err := client.Exchange(query, srv.Address())
		if err != nil {
			t.Fatalf("%s query after restart: %v", network, err)
		}
		if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
			t.Fatalf("%s response after restart: %v", network, response)
		}
	}
}

func TestReconcile_ConcurrentLifecycle(
	t *testing.T,
) {
	srv := startLoopbackDNS(t)
	if err := srv.Close(t.Context()); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})
	for range 8 {
		wg.Go(func() {
			<-start
			if err := srv.Reconcile(t.Context()); err != nil {
				t.Errorf("concurrent reconcile: %v", err)
			}
		})
	}
	close(start)
	wg.Wait()

	start = make(chan struct{})
	for range 4 {
		wg.Go(func() {
			<-start
			for range 10 {
				if err := srv.Reconcile(t.Context()); err != nil {
					t.Errorf("reconcile during shutdown: %v", err)
				}
			}
		})
		wg.Go(func() {
			<-start
			for range 10 {
				if err := srv.Close(t.Context()); err != nil {
					t.Errorf("concurrent stop: %v", err)
				}
			}
		})
		wg.Go(func() {
			<-start
			for range 10 {
				status := srv.Status()
				if status.Error != nil || (status.State != resource.Running && status.State != resource.Stopped) {
					t.Errorf("concurrent status: %+v", status)
				}
				_ = srv.Address()
			}
		})
	}
	close(start)
	wg.Wait()

	if err := srv.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, network := range []string{"udp", "tcp"} {
		client := &dns.Client{Net: network, Timeout: time.Second}
		query := new(dns.Msg).SetQuestion("looptest.internal.", dns.TypeSOA)
		response, _, err := client.Exchange(query, srv.Address())
		if err != nil {
			t.Fatalf("%s query after concurrent lifecycle calls: %v", network, err)
		}
		if response.Rcode != dns.RcodeSuccess || len(response.Answer) != 1 {
			t.Fatalf("%s response = %v", network, response)
		}
	}
}
