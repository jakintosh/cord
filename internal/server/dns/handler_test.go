package dns_test

import (
	"net"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/server/testutil"
	"github.com/miekg/dns"
)

func TestServeDNS_WireResponses(
	t *testing.T,
) {
	for _, protocol := range []string{"udp", "tcp"} {
		t.Run(protocol, func(t *testing.T) {
			srv := startLoopbackDNS(t)
			for _, tt := range []struct {
				name      string
				qtype     uint16
				rcode     int
				answer    string
				authority string
			}{
				{"cord-server.looptest.internal.", dns.TypeA, dns.RcodeSuccess, "cord-server.looptest.internal.\t3600\tIN\tA\t127.0.0.1", ""},
				{"CORD-SERVER.LoopTest.Internal.", dns.TypeA, dns.RcodeSuccess, "cord-server.looptest.internal.\t3600\tIN\tA\t127.0.0.1", ""},
				{"cord-server.looptest.internal.", dns.TypeAAAA, dns.RcodeSuccess, "", ""},
				{"cord-server.looptest.internal.", dns.TypeTXT, dns.RcodeSuccess, "", ""},
				{"looptest.internal.", dns.TypeSOA, dns.RcodeSuccess, "looptest.internal.\t3600\tIN\tSOA\tcord-server.looptest.internal. hostmaster.looptest.internal. 1 7200 3600 1209600 300", ""},
				{"looptest.internal.", dns.TypeNS, dns.RcodeSuccess, "looptest.internal.\t3600\tIN\tNS\tcord-server.looptest.internal.", ""},
				{"looptest.internal.", dns.TypeA, dns.RcodeSuccess, "", ""},
				{"ghost.looptest.internal.", dns.TypeA, dns.RcodeNameError, "", "looptest.internal.\t300\tIN\tSOA\tcord-server.looptest.internal. hostmaster.looptest.internal. 1 7200 3600 1209600 300"},
				{"looptest.internal.evil.", dns.TypeA, dns.RcodeRefused, "", ""},
				{"other.internal.", dns.TypeA, dns.RcodeRefused, "", ""},
			} {
				t.Run(tt.name+dns.TypeToString[tt.qtype], func(t *testing.T) {
					query := new(dns.Msg).SetQuestion(tt.name, tt.qtype)
					response := exchangeDNS(t, protocol, "127.0.0.1", srv.Address(), query)
					if response.Rcode != tt.rcode {
						t.Fatalf("rcode = %d, want %d", response.Rcode, tt.rcode)
					}
					if !response.Response || response.RecursionAvailable || response.Authoritative != (tt.rcode != dns.RcodeRefused) {
						t.Fatalf("unexpected response flags: %+v", response.MsgHdr)
					}
					if response.Id != query.Id || len(response.Question) != 1 || response.Question[0] != query.Question[0] {
						t.Fatalf("request not echoed: %v", response)
					}
					assertRecords(t, response.Answer, tt.answer)
					assertRecords(t, response.Ns, tt.authority)
					if len(response.Extra) != 0 {
						t.Fatalf("unexpected additional records: %v", response.Extra)
					}
				})
			}
		})
	}
}

func TestServeDNS_IPv6AddressAndWrongFamily(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	// The bootstrap server peer is ::1, matching the client's loopback source.
	seedZone(t, env, "looptest", "::/120")
	srv := startDNS(t, env, "[::1]:0")
	for _, protocol := range []string{"udp", "tcp"} {
		for _, qtype := range []uint16{dns.TypeAAAA, dns.TypeA} {
			response := exchangeDNS(t, protocol, "::1", srv.Address(), new(dns.Msg).SetQuestion("cord-server.looptest.internal.", qtype))
			if response.Rcode != dns.RcodeSuccess || !response.Authoritative {
				t.Fatalf("response = %v", response)
			}
			want := ""
			if qtype == dns.TypeAAAA {
				want = "cord-server.looptest.internal.\t3600\tIN\tAAAA\t::1"
			}
			assertRecords(t, response.Answer, want)
		}
	}
}

func TestServeDNS_UnknownSourceRefused(
	t *testing.T,
) {
	env := testutil.SetupService(t)
	seedZone(t, env, "looptest", "10.0.0.0/24")
	srv := startDNS(t, env, "127.0.0.1:0")
	for _, protocol := range []string{"udp", "tcp"} {
		response := exchangeDNS(t, protocol, "127.0.0.1", srv.Address(), new(dns.Msg).SetQuestion("looptest.internal.", dns.TypeSOA))
		if response.Rcode != dns.RcodeRefused || response.Authoritative {
			t.Fatalf("response = %v", response)
		}
		if len(response.Answer)+len(response.Ns)+len(response.Extra) != 0 {
			t.Fatalf("refused response discloses records: %v", response)
		}
	}
}

func TestServeDNS_QuestionlessRequestRejected(
	t *testing.T,
) {
	srv := startLoopbackDNS(t)
	for _, protocol := range []string{"udp", "tcp"} {
		response := exchangeDNS(t, protocol, "127.0.0.1", srv.Address(), &dns.Msg{})
		if response.Rcode != dns.RcodeFormatError || !response.Response || len(response.Question) != 0 {
			t.Fatalf("response = %v", response)
		}
	}
}

func TestServeDNS_EnforcesVisibilityAndRefreshesAfterMutation(
	t *testing.T,
) {
	for _, protocol := range []string{"udp", "tcp"} {
		t.Run(protocol, func(t *testing.T) {
			env := testutil.SetupService(t)
			seedWireVisibility(t, env)

			srv := startDNS(t, env, "127.0.0.1:0")
			queryHost := func(
				host string,
			) *dns.Msg {
				question := new(dns.Msg)
				question.SetQuestion(host+".looptest.internal.", dns.TypeA)

				return exchangeDNS(
					t,
					protocol,
					"127.0.0.1",
					srv.Address(),
					question,
				)
			}

			// The ordinary requester can resolve itself, the server, and
			// the active target made visible by the group association.
			for _, tc := range []struct {
				host    string
				address string
			}{
				{"requester", "127.0.0.1"},
				{"cord-server", "126.0.0.1"},
				{"visible", "127.0.1.1"},
			} {
				response := queryHost(tc.host)
				if response.Rcode != dns.RcodeSuccess {
					t.Fatalf("%s response = %v", tc.host, response)
				}
				if !response.Authoritative || response.RecursionAvailable {
					t.Fatalf("unexpected response flags: %+v", response.MsgHdr)
				}
				wantAnswer := tc.host + ".looptest.internal.\t3600\tIN\tA\t" + tc.address
				assertRecords(t, response.Answer, wantAnswer)
			}

			// Unavailable targets are indistinguishable from missing names.
			for _, host := range []string{"hidden", "down", "pending", "missing"} {
				response := queryHost(host)
				assertNegativeDNS(t, response, "looptest.internal.")
			}

			// Invalidate the warmed view without restarting the listeners.
			err := env.Service.DeleteAssociation("looptest", "readers", "targets")
			if err != nil {
				t.Fatal(err)
			}
			response := queryHost("visible")
			assertNegativeDNS(t, response, "looptest.internal.")

			// Restoring the association makes the next lookup succeed.
			err = env.Service.CreateAssociation("looptest", "readers", "targets")
			if err != nil {
				t.Fatal(err)
			}
			response = queryHost("visible")
			if response.Rcode != dns.RcodeSuccess {
				t.Fatalf("restored visibility = %v", response)
			}
			wantAnswer := "visible.looptest.internal.\t3600\tIN\tA\t127.0.1.1"
			assertRecords(t, response.Answer, wantAnswer)

			// Source identity must also refresh, even for self/server queries.
			enabled := false
			diff := service.PeerDiff{
				Enabled: &enabled,
			}
			_, err = env.Service.UpdatePeer("looptest", "requester", diff)
			if err != nil {
				t.Fatal(err)
			}
			for _, host := range []string{"requester", "cord-server", "visible"} {
				response := queryHost(host)
				if response.Rcode != dns.RcodeRefused || response.Authoritative {
					t.Fatalf("disabled requester response = %v", response)
				}
				assertRecords(t, response.Answer, "")
				assertRecords(t, response.Ns, "")
				assertRecords(t, response.Extra, "")
			}
		})
	}
}

// seedWireVisibility makes the loopback source an ordinary non-admin peer.
// Targets inherit the associated group, while the hidden peer has no association.
func seedWireVisibility(
	t *testing.T,
	env *testutil.ServiceEnv,
) {
	t.Helper()

	// Bootstrap uses 126.0.0.1, leaving 127.0.0.1 for the requesting peer.
	seedZone(t, env, "looptest", "126.0.0.0/7")
	err := env.Service.CreateCidr("looptest", "targets", "127.0.1.0/24")
	if err != nil {
		t.Fatal(err)
	}

	peers := []struct {
		name      string
		route     string
		enabled   bool
		confirmed bool
	}{
		{"requester", "127.0.0.1/32", true, true},
		{"visible", "127.0.1.1/32", true, true},
		{"down", "127.0.1.2/32", false, true},
		{"pending", "127.0.1.3/32", true, false},
		{"hidden", "127.0.2.1/32", true, true},
	}
	for _, peer := range peers {
		testutil.SeedPeerDB(
			t,
			env.Database,
			"looptest",
			peer.name,
			peer.route,
			peer.name+"-key",
			false,
			peer.enabled,
			peer.confirmed,
		)
	}

	for _, group := range []string{"readers", "targets"} {
		if _, err := env.Service.CreateGroup("looptest", group); err != nil {
			t.Fatal(err)
		}
	}
	for cidr, group := range map[string]string{
		"requester": "readers",
		"targets":   "targets",
	} {
		if err := env.Service.AssignCidrGroup("looptest", cidr, group); err != nil {
			t.Fatal(err)
		}
	}
	err = env.Service.CreateAssociation("looptest", "readers", "targets")
	if err != nil {
		t.Fatal(err)
	}
}

func assertNegativeDNS(
	t *testing.T,
	response *dns.Msg,
	zone string,
) {
	t.Helper()
	if response.Rcode != dns.RcodeNameError {
		t.Fatalf("negative response = %v", response)
	}
	if !response.Authoritative || response.RecursionAvailable {
		t.Fatalf("unexpected response flags: %+v", response.MsgHdr)
	}

	assertRecords(t, response.Answer, "")
	assertRecords(t, response.Extra, "")

	wantAuthority := zone + "\t300\tIN\tSOA\tcord-server." + zone +
		" hostmaster." + zone + " 1 7200 3600 1209600 300"
	assertRecords(t, response.Ns, wantAuthority)
}

func exchangeDNS(
	t *testing.T,
	protocol, source, address string,
	query *dns.Msg,
) *dns.Msg {
	t.Helper()
	var local net.Addr = &net.UDPAddr{IP: net.ParseIP(source)}
	if protocol == "tcp" {
		local = &net.TCPAddr{IP: net.ParseIP(source)}
	}
	client := &dns.Client{
		Net:     protocol,
		Timeout: time.Second,
		Dialer: &net.Dialer{
			LocalAddr: local,
			Timeout:   time.Second,
		},
	}
	response, _, err := client.ExchangeContext(t.Context(), query, address)
	if err != nil {
		t.Fatalf("%s exchange: %v", protocol, err)
	}
	return response
}

func assertRecords(
	t *testing.T,
	records []dns.RR,
	want string,
) {
	t.Helper()
	if want == "" {
		if len(records) != 0 {
			t.Fatalf("records = %v, want none", records)
		}
		return
	}
	if len(records) != 1 || records[0].String() != want {
		t.Fatalf("records = %v, want %q", records, want)
	}
}
