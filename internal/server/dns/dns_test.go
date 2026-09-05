package dns_test

import (
	"context"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"codeberg.org/miekg/dns"
	serverdns "git.studiopollinator.com/pollinator/cord/internal/server/dns"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
)

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

	srv, err := serverdns.New(serverdns.Options{
		Network: "looptest",
		Zone:    "looptest.internal",
		Address: "127.0.0.1:0",
		Service: env.Service,
	})
	if err != nil {
		t.Fatalf("new dns server: %v", err)
	}

	if err := srv.Start(t.Context()); err != nil {
		t.Fatalf("start dns server: %v", err)
	}
	t.Cleanup(func() {
		_ = srv.Stop(context.Background())
	})

	return srv
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

func TestServer_UDPRoundTrip(
	t *testing.T,
) {
	srv := startLoopbackDNS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := dns.Exchange(ctx, apexQuery("cord-server.looptest.internal."), "udp", srv.Address())
	if err != nil {
		t.Fatalf("udp exchange: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("rcode = %d, answers = %d, want one A answer", resp.Rcode, len(resp.Answer))
	}
	if a := resp.Answer[0].(*dns.A); a.A.Addr.String() != "127.0.0.1" {
		t.Fatalf("address = %s, want 127.0.0.1", a.A.Addr)
	}
}

func TestServer_TCPRoundTrip(
	t *testing.T,
) {
	srv := startLoopbackDNS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := dns.Exchange(ctx, apexQuery("cord-server.looptest.internal."), "tcp", srv.Address())
	if err != nil {
		t.Fatalf("tcp exchange: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("rcode = %d, answers = %d, want one A answer", resp.Rcode, len(resp.Answer))
	}
	if a := resp.Answer[0].(*dns.A); a.A.Addr.String() != "127.0.0.1" {
		t.Fatalf("address = %s, want 127.0.0.1", a.A.Addr)
	}
}

func TestServer_PartialBindRollback(
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

	startErr := srv.Start(t.Context())
	if startErr == nil || !strings.Contains(startErr.Error(), "listen tcp") {
		t.Fatalf("start error = %v, want a tcp bind failure", startErr)
	}

	status := srv.Status()
	if status.Serving {
		t.Fatalf("status after failed start = %+v, want not serving", status)
	}
	if status.Error == "" {
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
	if err := srv.Start(t.Context()); err != nil {
		t.Fatalf("restart after failed start: %v", err)
	}
	if !srv.Status().Serving {
		t.Fatal("restarted server should be serving")
	}
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("stop after restart: %v", err)
	}
}

func TestServer_BoundedShutdown(
	t *testing.T,
) {
	srv := startLoopbackDNS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil {
		t.Fatalf("stop: %v", err)
	}

	if srv.Status().Serving {
		t.Fatal("server still serving after stop")
	}

	// Stop is safe to call twice.
	if err := srv.Stop(ctx); err != nil {
		t.Fatalf("second stop: %v", err)
	}

	// The sockets are released and the server is restartable.
	if err := srv.Start(t.Context()); err != nil {
		t.Fatalf("start after stop: %v", err)
	}
	if !srv.Status().Serving {
		t.Fatal("restarted server should be serving")
	}
	if err := srv.Stop(context.Background()); err != nil {
		t.Fatalf("final stop: %v", err)
	}
}

func TestServer_StatusTracksAttemptAndSuccess(
	t *testing.T,
) {
	srv := startLoopbackDNS(t)

	status := srv.Status()
	if !status.Serving {
		t.Fatalf("status = %+v, want serving", status)
	}
	if status.LastAttemptAt.IsZero() || status.LastSuccessAt.IsZero() {
		t.Fatalf("status = %+v, want attempt and success times", status)
	}
	if status.Error != "" {
		t.Fatalf("status error = %q, want empty", status.Error)
	}
}

// apexQuery builds an A query for the running server's own name, which
// resolves because loopback queries originate from 127.0.0.1 — the
// network's server peer.
func apexQuery(
	name string,
) *dns.Msg {
	q := &dns.Msg{}
	q.Question = []dns.RR{&dns.A{Hdr: dns.Header{
		Name:  name,
		Class: dns.ClassINET,
	}}}

	return q
}
