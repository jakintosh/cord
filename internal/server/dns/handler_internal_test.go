package dns

import (
	"net"
	"testing"

	"codeberg.org/miekg/dns"
	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

// The signed-off TTL policy, asserted as literals so accidental drift
// in the package constants fails these tests.
const (
	positiveTTLSeconds = uint32(3600)
	negativeTTLSeconds = uint32(300)
)

// respondEnv is a service over an in-memory database for the pure
// response-matrix tests. It cannot use the shared testutil helpers:
// they transitively import the runtime, which imports this package.
type respondEnv struct {
	Service *service.Service
	DB      *database.DB
}

func setupRespondEnv(
	t *testing.T,
) *respondEnv {
	t.Helper()

	db, err := database.Open(database.Options{Path: ":memory:"})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	svc, err := service.New(service.Options{
		Store:  db,
		Logger: logging.Discard(),
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}

	return &respondEnv{Service: svc, DB: db}
}

// seedZoneNetwork creates a standalone network so tests can prove zone
// and network isolation between servers.
func seedZoneNetwork(
	t *testing.T,
	env *respondEnv,
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

// seedPeer inserts a confirmed-or-not peer on its own terminal CIDR.
func seedPeer(
	t *testing.T,
	env *respondEnv,
	network string,
	name string,
	cidr string,
	enabled bool,
	confirmed bool,
) {
	t.Helper()

	if err := env.DB.CreateCidr(network, &service.Cidr{
		Name:     name,
		Cidr:     cidr,
		Terminal: true,
	}); err != nil {
		t.Fatalf("seed cidr %s: %v", name, err)
	}

	if err := env.DB.InsertPeer(network, &service.Peer{
		Name:      name,
		CidrName:  name,
		PublicKey: name + "-key",
		Enabled:   enabled,
		Confirmed: confirmed,
	}); err != nil {
		t.Fatalf("seed peer %s: %v", name, err)
	}
}

// seedTestnetVisibility seeds the testnet visibility web used by the
// matrix tests: frank (ops) sees api-1 (api) through the app:api ->
// team:ops association, while lonely, down, and pending have no
// visibility path.
func seedTestnetVisibility(
	t *testing.T,
	env *respondEnv,
) {
	t.Helper()

	seedZoneNetwork(t, env, "testnet", "10.0.0.0/16")

	for _, cidr := range []struct {
		name string
		cidr string
	}{
		{name: "api", cidr: "10.0.1.0/24"},
		{name: "ops", cidr: "10.0.32.0/24"},
	} {
		if err := env.Service.CreateCidr("testnet", cidr.name, cidr.cidr); err != nil {
			t.Fatalf("create cidr %s: %v", cidr.name, err)
		}
	}

	seedPeer(t, env, "testnet", "api-1", "10.0.1.1/32", true, true)
	seedPeer(t, env, "testnet", "frank", "10.0.32.5/32", true, true)
	seedPeer(t, env, "testnet", "lonely", "10.0.64.5/32", true, true)
	seedPeer(t, env, "testnet", "down", "10.0.64.6/32", false, true)
	seedPeer(t, env, "testnet", "pending", "10.0.64.7/32", true, false)

	seedGroupWeb(t, env, "testnet", "api", "ops")
}

// seedNet6Visibility seeds the same visibility web on an IPv6 network,
// for the AAAA record behavior.
func seedNet6Visibility(
	t *testing.T,
	env *respondEnv,
) {
	t.Helper()

	seedZoneNetwork(t, env, "net6", "fd00::/48")

	for _, cidr := range []struct {
		name string
		cidr string
	}{
		{name: "api6", cidr: "fd00:0:0:1::/64"},
		{name: "ops6", cidr: "fd00:0:0:20::/64"},
	} {
		if err := env.Service.CreateCidr("net6", cidr.name, cidr.cidr); err != nil {
			t.Fatalf("create cidr %s: %v", cidr.name, err)
		}
	}

	seedPeer(t, env, "net6", "api6-1", "fd00:0:0:1::1/128", true, true)
	seedPeer(t, env, "net6", "frank6", "fd00:0:0:20::5/128", true, true)

	seedGroupWeb(t, env, "net6", "api6", "ops6")
}

// seedGroupWeb associates a source CIDR with a target CIDR through
// one group each.
func seedGroupWeb(
	t *testing.T,
	env *respondEnv,
	network string,
	source string,
	target string,
) {
	t.Helper()

	sourceGroup := "app:" + source
	targetGroup := "team:" + target

	for _, group := range []string{sourceGroup, targetGroup} {
		if _, err := env.Service.CreateGroup(network, group); err != nil {
			t.Fatalf("create group %s: %v", group, err)
		}
	}
	if err := env.Service.AssignCidrGroup(network, source, sourceGroup); err != nil {
		t.Fatalf("assign %s: %v", source, err)
	}
	if err := env.Service.AssignCidrGroup(network, target, targetGroup); err != nil {
		t.Fatalf("assign %s: %v", target, err)
	}
	if err := env.Service.CreateAssociation(network, sourceGroup, targetGroup); err != nil {
		t.Fatalf("create association: %v", err)
	}
}

func TestRespond_ARecordForVisiblePeer(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedTestnetVisibility(t, env)
	srv := newTestServer(t, env)

	resp := srv.respond(net.ParseIP("10.0.32.5"), query("api-1.testnet.internal.", dns.TypeA))

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %d, want NOERROR", resp.Rcode)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
	a, ok := resp.Answer[0].(*dns.A)
	if !ok {
		t.Fatalf("answer type = %T, want *dns.A", resp.Answer[0])
	}
	if a.A.Addr.String() != "10.0.1.1" {
		t.Fatalf("address = %s, want 10.0.1.1", a.A.Addr)
	}
	if a.Hdr.TTL != positiveTTLSeconds {
		t.Fatalf("ttl = %d, want %d", a.Hdr.TTL, positiveTTLSeconds)
	}
	if a.Hdr.Name != "api-1.testnet.internal." {
		t.Fatalf("owner = %q, want the canonical query name", a.Hdr.Name)
	}
	assertAuthoritative(t, resp)
}

func TestRespond_AAAAForIPv6Peer(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedNet6Visibility(t, env)
	srv := newTestServerFor(t, env, "net6", "net6.internal")

	resp := srv.respond(net.ParseIP("fd00:0:0:20::5"), query("api6-1.net6.internal.", dns.TypeAAAA))

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %d, want NOERROR", resp.Rcode)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
	aaaa, ok := resp.Answer[0].(*dns.AAAA)
	if !ok {
		t.Fatalf("answer type = %T, want *dns.AAAA", resp.Answer[0])
	}
	if aaaa.AAAA.Addr.String() != "fd00:0:0:1::1" {
		t.Fatalf("address = %s, want fd00:0:0:1::1", aaaa.AAAA.Addr)
	}
	if aaaa.Hdr.TTL != positiveTTLSeconds {
		t.Fatalf("ttl = %d, want %d", aaaa.Hdr.TTL, positiveTTLSeconds)
	}
	assertAuthoritative(t, resp)
}

func TestRespond_WrongFamilyIsNODATA(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedTestnetVisibility(t, env)
	srv := newTestServer(t, env)
	env6 := setupRespondEnv(t)
	seedNet6Visibility(t, env6)
	srv6 := newTestServerFor(t, env6, "net6", "net6.internal")

	for _, tt := range []struct {
		name   string
		server *Server
		source net.IP
		qname  string
		qtype  uint16
	}{
		{name: "aaaa for ipv4 peer", server: srv, source: net.ParseIP("10.0.32.5"), qname: "api-1.testnet.internal.", qtype: dns.TypeAAAA},
		{name: "a for ipv6 peer", server: srv6, source: net.ParseIP("fd00:0:0:20::5"), qname: "api6-1.net6.internal.", qtype: dns.TypeA},
		{name: "txt for known host", server: srv, source: net.ParseIP("10.0.32.5"), qname: "api-1.testnet.internal.", qtype: dns.TypeTXT},
	} {
		resp := tt.server.respond(tt.source, query(tt.qname, tt.qtype))
		if resp.Rcode != dns.RcodeSuccess {
			t.Fatalf("%s: rcode = %d, want NOERROR", tt.name, resp.Rcode)
		}
		if len(resp.Answer) != 0 {
			t.Fatalf("%s: answers = %d, want 0", tt.name, len(resp.Answer))
		}
		assertAuthoritative(t, resp)
	}
}

func TestRespond_ApexSOA(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedZoneNetwork(t, env, "testnet", "10.0.0.0/16")
	srv := newTestServer(t, env)

	resp := srv.respond(net.ParseIP("10.0.0.1"), query("testnet.internal.", dns.TypeSOA))

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %d, want NOERROR", resp.Rcode)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
	soa, ok := resp.Answer[0].(*dns.SOA)
	if !ok {
		t.Fatalf("answer type = %T, want *dns.SOA", resp.Answer[0])
	}
	if soa.Hdr.Name != "testnet.internal." {
		t.Fatalf("owner = %q, want the zone apex", soa.Hdr.Name)
	}
	if soa.Hdr.TTL != positiveTTLSeconds {
		t.Fatalf("ttl = %d, want %d", soa.Hdr.TTL, positiveTTLSeconds)
	}
	if soa.SOA.Ns != "cord-server.testnet.internal." {
		t.Fatalf("mname = %q, want cord-server.testnet.internal.", soa.SOA.Ns)
	}
	if soa.SOA.Mbox != "hostmaster.testnet.internal." {
		t.Fatalf("rname = %q, want hostmaster.testnet.internal.", soa.SOA.Mbox)
	}
	if soa.SOA.Serial != 1 {
		t.Fatalf("serial = %d, want 1", soa.SOA.Serial)
	}
	if soa.SOA.Minttl != negativeTTLSeconds {
		t.Fatalf("minimum = %d, want %d", soa.SOA.Minttl, negativeTTLSeconds)
	}
	assertAuthoritative(t, resp)
}

func TestRespond_ApexNS(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedZoneNetwork(t, env, "testnet", "10.0.0.0/16")
	srv := newTestServer(t, env)

	resp := srv.respond(net.ParseIP("10.0.0.1"), query("testnet.internal.", dns.TypeNS))

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %d, want NOERROR", resp.Rcode)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
	ns, ok := resp.Answer[0].(*dns.NS)
	if !ok {
		t.Fatalf("answer type = %T, want *dns.NS", resp.Answer[0])
	}
	if ns.NS.Ns != "cord-server.testnet.internal." {
		t.Fatalf("nameserver = %q, want cord-server.testnet.internal.", ns.NS.Ns)
	}
	if ns.Hdr.TTL != positiveTTLSeconds {
		t.Fatalf("ttl = %d, want %d", ns.Hdr.TTL, positiveTTLSeconds)
	}
	assertAuthoritative(t, resp)
}

func TestRespond_ApexOtherTypesNODATA(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedZoneNetwork(t, env, "testnet", "10.0.0.0/16")
	srv := newTestServer(t, env)

	for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA, dns.TypeTXT} {
		resp := srv.respond(net.ParseIP("10.0.0.1"), query("testnet.internal.", qtype))
		if resp.Rcode != dns.RcodeSuccess {
			t.Fatalf("qtype %d: rcode = %d, want NOERROR", qtype, resp.Rcode)
		}
		if len(resp.Answer) != 0 {
			t.Fatalf("qtype %d: answers = %d, want 0", qtype, len(resp.Answer))
		}
		assertAuthoritative(t, resp)
	}
}

func TestRespond_NXDOMAINCarriesNegativeSOA(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedTestnetVisibility(t, env)
	srv := newTestServer(t, env)

	resp := srv.respond(net.ParseIP("10.0.32.5"), query("ghost.testnet.internal.", dns.TypeA))

	if resp.Rcode != dns.RcodeNameError {
		t.Fatalf("rcode = %d, want NXDOMAIN", resp.Rcode)
	}
	if len(resp.Answer) != 0 {
		t.Fatalf("answers = %d, want 0", len(resp.Answer))
	}
	if len(resp.Ns) != 1 {
		t.Fatalf("authority = %d, want 1 SOA", len(resp.Ns))
	}
	soa, ok := resp.Ns[0].(*dns.SOA)
	if !ok {
		t.Fatalf("authority type = %T, want *dns.SOA", resp.Ns[0])
	}
	if soa.Hdr.TTL != negativeTTLSeconds {
		t.Fatalf("soa ttl = %d, want %d", soa.Hdr.TTL, negativeTTLSeconds)
	}
	if soa.SOA.Minttl != negativeTTLSeconds {
		t.Fatalf("soa minimum = %d, want %d", soa.SOA.Minttl, negativeTTLSeconds)
	}
	assertAuthoritative(t, resp)
}

func TestRespond_HiddenHostsAreNotDisclosed(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedTestnetVisibility(t, env)
	srv := newTestServer(t, env)

	// lonely has no visibility path, down is disabled, pending is
	// unconfirmed. All must be indistinguishable from nonexistent.
	for _, name := range []string{"lonely", "down", "pending", "ghost"} {
		resp := srv.respond(
			net.ParseIP("10.0.32.5"),
			query(name+".testnet.internal.", dns.TypeA),
		)
		if resp.Rcode != dns.RcodeNameError {
			t.Fatalf("%s: rcode = %d, want NXDOMAIN", name, resp.Rcode)
		}
	}
}

func TestRespond_CordServerAlwaysAddressable(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedTestnetVisibility(t, env)
	srv := newTestServer(t, env)

	resp := srv.respond(net.ParseIP("10.0.32.5"), query("cord-server.testnet.internal.", dns.TypeA))

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %d, want NOERROR", resp.Rcode)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
	a := resp.Answer[0].(*dns.A)
	if a.A.Addr.String() != "10.0.0.1" {
		t.Fatalf("address = %s, want 10.0.0.1", a.A.Addr)
	}
}

func TestRespond_SelfResolution(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedTestnetVisibility(t, env)
	srv := newTestServer(t, env)

	resp := srv.respond(net.ParseIP("10.0.32.5"), query("frank.testnet.internal.", dns.TypeA))

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %d, want NOERROR", resp.Rcode)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
	a := resp.Answer[0].(*dns.A)
	if a.A.Addr.String() != "10.0.32.5" {
		t.Fatalf("address = %s, want 10.0.32.5", a.A.Addr)
	}
}

func TestRespond_UnknownSourceRefused(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedTestnetVisibility(t, env)
	srv := newTestServer(t, env)

	for _, source := range []net.IP{net.ParseIP("10.0.99.99"), nil} {
		resp := srv.respond(source, query("api-1.testnet.internal.", dns.TypeA))
		if resp.Rcode != dns.RcodeRefused {
			t.Fatalf("source %v: rcode = %d, want REFUSED", source, resp.Rcode)
		}
		if resp.Authoritative {
			t.Fatalf("source %v: REFUSED must be non-authoritative", source)
		}
	}
}

func TestRespond_OutOfZoneRefused(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedTestnetVisibility(t, env)
	srv := newTestServer(t, env)

	for _, name := range []string{
		"example.com.",
		"other.internal.",
		"testnet.other.internal.",
		"sub.testnet.internal.evil.",
		"internal.",
	} {
		resp := srv.respond(net.ParseIP("10.0.32.5"), query(name, dns.TypeA))
		if resp.Rcode != dns.RcodeRefused {
			t.Fatalf("%s: rcode = %d, want REFUSED", name, resp.Rcode)
		}
		if resp.Authoritative {
			t.Fatalf("%s: REFUSED must be non-authoritative", name)
		}
	}
}

func TestRespond_CaseInsensitive(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedTestnetVisibility(t, env)
	srv := newTestServer(t, env)

	resp := srv.respond(
		net.ParseIP("10.0.32.5"),
		query("API-1.TestNet.Internal.", dns.TypeA),
	)

	if resp.Rcode != dns.RcodeSuccess {
		t.Fatalf("rcode = %d, want NOERROR", resp.Rcode)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
	a := resp.Answer[0].(*dns.A)
	if a.A.Addr.String() != "10.0.1.1" {
		t.Fatalf("address = %s, want 10.0.1.1", a.A.Addr)
	}
}

func TestRespond_MalformedRequest(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedZoneNetwork(t, env, "testnet", "10.0.0.0/16")
	srv := newTestServer(t, env)

	resp := srv.respond(net.ParseIP("10.0.32.5"), &dns.Msg{})
	if resp.Rcode != dns.RcodeFormatError {
		t.Fatalf("rcode = %d, want FORMERR", resp.Rcode)
	}
	if len(resp.Question) != 0 {
		t.Fatal("malformed response must not echo a question")
	}

	resp = srv.respond(net.ParseIP("10.0.32.5"), nil)
	if resp.Rcode != dns.RcodeFormatError {
		t.Fatalf("nil query rcode = %d, want FORMERR", resp.Rcode)
	}
}

func TestRespond_TwoServersServeOnlyTheirOwnZone(
	t *testing.T,
) {
	redEnv := setupRespondEnv(t)
	seedZoneNetwork(t, redEnv, "red", "127.10.0.0/24")
	seedPeer(t, redEnv, "red", "alpha", "127.10.0.5/32", true, true)
	blueEnv := setupRespondEnv(t)
	seedZoneNetwork(t, blueEnv, "blue", "127.20.0.0/24")
	seedPeer(t, blueEnv, "blue", "beta", "127.20.0.5/32", true, true)

	red, err := New(Options{
		Network: "red",
		Zone:    "red.internal",
		Address: "127.0.0.1:53",
		Service: redEnv.Service,
	})
	if err != nil {
		t.Fatalf("new red server: %v", err)
	}
	blue, err := New(Options{
		Network: "blue",
		Zone:    "blue.internal",
		Address: "127.0.0.1:53",
		Service: blueEnv.Service,
	})
	if err != nil {
		t.Fatalf("new blue server: %v", err)
	}

	resp := red.respond(net.ParseIP("127.10.0.5"), query("beta.blue.internal.", dns.TypeA))
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("red server rcode for blue name = %d, want REFUSED", resp.Rcode)
	}

	resp = blue.respond(net.ParseIP("127.20.0.5"), query("alpha.red.internal.", dns.TypeA))
	if resp.Rcode != dns.RcodeRefused {
		t.Fatalf("blue server rcode for red name = %d, want REFUSED", resp.Rcode)
	}

	resp = blue.respond(net.ParseIP("127.20.0.5"), query("beta.blue.internal.", dns.TypeA))
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("blue server for own host: rcode = %d, answers = %d", resp.Rcode, len(resp.Answer))
	}
}

func TestNew_RejectsInvalidOptions(
	t *testing.T,
) {
	env := setupRespondEnv(t)

	for _, tt := range []struct {
		name    string
		network string
		zone    string
		address string
	}{
		{name: "missing network", network: "", zone: "x.internal", address: "127.0.0.1:53"},
		{name: "missing zone", network: "testnet", zone: "", address: "127.0.0.1:53"},
		{name: "missing port", network: "testnet", zone: "x.internal", address: "127.0.0.1"},
		{name: "non-IP host", network: "testnet", zone: "x.internal", address: "localhost:53"},
		{name: "port out of range", network: "testnet", zone: "x.internal", address: "127.0.0.1:70000"},
	} {
		_, err := New(Options{
			Network: tt.network,
			Zone:    tt.zone,
			Address: tt.address,
			Service: env.Service,
		})
		if err == nil {
			t.Fatalf("%s: expected an error", tt.name)
		}
	}

	if _, err := New(Options{Network: "testnet", Zone: "x.internal", Address: "127.0.0.1:53"}); err == nil {
		t.Fatal("missing service: expected an error")
	}
}

func TestNew_NormalizesZone(
	t *testing.T,
) {
	env := setupRespondEnv(t)
	seedZoneNetwork(t, env, "testnet", "10.0.0.0/16")

	srv, err := New(Options{
		Network: "testnet",
		Zone:    "TestNet.Internal",
		Address: "127.0.0.1:53",
		Service: env.Service,
	})
	if err != nil {
		t.Fatalf("new dns server: %v", err)
	}

	resp := srv.respond(net.ParseIP("10.0.0.1"), query("testnet.internal.", dns.TypeSOA))
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 1 {
		t.Fatalf("normalized zone not served: rcode = %d, answers = %d", resp.Rcode, len(resp.Answer))
	}
}

// newTestServer returns a server for the testnet zone. Its address is
// never bound by the pure tests, so port 53 is safe unprivileged.
func newTestServer(
	t *testing.T,
	env *respondEnv,
) *Server {
	t.Helper()

	return newTestServerFor(t, env, "testnet", "testnet.internal")
}

// newTestServerFor returns a server for an arbitrary network and zone.
func newTestServerFor(
	t *testing.T,
	env *respondEnv,
	network string,
	zone string,
) *Server {
	t.Helper()

	srv, err := New(Options{
		Network: network,
		Zone:    zone,
		Address: "127.0.0.1:53",
		Service: env.Service,
	})
	if err != nil {
		t.Fatalf("new dns server: %v", err)
	}

	return srv
}

// query builds a query message for name and qtype. The library encodes
// the qtype as the type of the question RR.
func query(
	name string,
	qtype uint16,
) *dns.Msg {
	q := &dns.Msg{}
	var rr dns.RR
	switch qtype {
	case dns.TypeA:
		rr = &dns.A{}
	case dns.TypeAAAA:
		rr = &dns.AAAA{}
	case dns.TypeSOA:
		rr = &dns.SOA{}
	case dns.TypeNS:
		rr = &dns.NS{}
	default:
		rr = &dns.TXT{}
	}
	rr.Header().Name = name
	rr.Header().Class = dns.ClassINET
	q.Question = []dns.RR{rr}

	return q
}

// assertAuthoritative asserts the flags every in-zone success or
// NXDOMAIN response must carry.
func assertAuthoritative(
	t *testing.T,
	resp *dns.Msg,
) {
	t.Helper()

	if !resp.Authoritative {
		t.Error("response must set the authoritative flag")
	}
	if resp.RecursionAvailable {
		t.Error("response must not advertise recursion")
	}
	if !resp.Response {
		t.Error("response must set the QR flag")
	}
}
