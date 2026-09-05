package dns

import (
	"context"
	"io"
	"net"
	"net/netip"
	"time"

	"codeberg.org/miekg/dns"
	"codeberg.org/miekg/dns/dnsutil"
	"codeberg.org/miekg/dns/rdata"
	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
)

const (
	// positiveTTL is the record TTL for successful answers. Peer
	// addresses are immutable and renames are rare, so cached
	// answers stay valid for an hour.
	positiveTTL = time.Hour

	// negativeTTL bounds negative caching through the zone's SOA, so
	// a typo or a lookup just before a peer is created or renamed
	// does not stay unresolvable for the positive TTL.
	negativeTTL = 5 * time.Minute

	// soaSerial is the zone serial. The zone is small and rebuilt
	// from the service's compiled model, so a fixed serial suffices.
	soaSerial = 1

	// soaRefresh, soaRetry, and soaExpire are the RFC 1035 timing
	// fields secondaries would use.
	soaRefresh = 2 * time.Hour
	soaRetry   = time.Hour
	soaExpire  = 14 * 24 * time.Hour
)

// serveDNS implements dns.Handler. It is the only I/O-bearing step of
// a request: it extracts the source address, delegates the response
// logic to the pure respond function, and writes the result.
func (s *Server) serveDNS(
	ctx context.Context,
	w dns.ResponseWriter,
	query *dns.Msg,
) {
	ip := remoteIP(w.RemoteAddr())
	resp := s.respond(ip, query)

	s.log.Debug(
		"dns request",
		"source",
		w.RemoteAddr().String(),
		"name",
		questionName(query),
		"rcode",
		resp.Rcode,
	)

	if _, err := io.Copy(w, resp); err != nil {
		s.log.Debug(
			"dns write failed",
			"err",
			err,
		)
	}
}

// respond builds the authoritative response for one query from one
// source address. It performs no I/O of its own: every behavior test
// drives it directly with a constructed query and source IP.
//
// Sources that are not confirmed, enabled peers of the network and
// queries outside the zone are REFUSED. In-zone queries are answered
// authoritatively from the service: address records for known hosts,
// SOA and NS at the apex, NODATA where no record of the requested type
// exists, and NXDOMAIN with a negative-caching SOA for unknown or
// invisible names.
func (s *Server) respond(
	source net.IP,
	query *dns.Msg,
) *dns.Msg {
	if query == nil || len(query.Question) != 1 {
		return malformed()
	}

	qname := dnsutil.Canonical(questionName(query))
	if source == nil || !dnsutil.IsBelow(s.zone, qname) {
		return refused(query)
	}

	peer, err := s.service.ResolvePeerIdentity(s.network, source)
	if err != nil {
		return refused(query)
	}

	_, qtype := dnsutil.Question(query)
	reply := &dns.Msg{}
	dnsutil.SetReply(reply, query)

	if dns.EqualName(qname, s.zone) {
		return s.apexReply(reply, qtype)
	}

	host, err := s.service.ResolveDNSHost(
		s.network,
		peer.Name,
		dnsutil.Trim(qname, s.zone),
	)
	if err != nil {
		return nxdomain(reply, s.soaRecord(negativeTTL))
	}

	return s.hostReply(reply, qname, qtype, host.Route)
}

// apexReply answers a query at the zone apex: SOA and NS for their own
// types, NODATA (NOERROR without answers) for everything else.
func (s *Server) apexReply(
	reply *dns.Msg,
	qtype uint16,
) *dns.Msg {
	reply.Authoritative = true

	switch qtype {
	case dns.TypeSOA:
		reply.Answer = []dns.RR{s.soaRecord(positiveTTL)}
	case dns.TypeNS:
		reply.Answer = []dns.RR{&dns.NS{
			Hdr: dns.Header{
				Name:  s.zone,
				TTL:   seconds(positiveTTL),
				Class: dns.ClassINET,
			},
			NS: rdata.NS{
				Ns: "cord-server." + s.zone,
			},
		}}
	}

	return reply
}

// hostReply answers a host query from the peer's Cord route: the
// address record matching the route's family, NODATA for the wrong
// family and for unsupported types.
func (s *Server) hostReply(
	reply *dns.Msg,
	qname string,
	qtype uint16,
	route string,
) *dns.Msg {
	reply.Authoritative = true

	r, err := netaddr.ParseRoute(route)
	if err != nil {
		return reply
	}

	switch qtype {
	case dns.TypeA:
		if v4 := r.IP.To4(); v4 != nil {
			if addr, ok := netip.AddrFromSlice(v4); ok {
				reply.Answer = []dns.RR{&dns.A{
					Hdr: dns.Header{
						Name:  qname,
						TTL:   seconds(positiveTTL),
						Class: dns.ClassINET,
					},
					A: rdata.A{
						Addr: addr,
					},
				}}
			}
		}
	case dns.TypeAAAA:
		if r.IP.To4() == nil {
			if addr, ok := netip.AddrFromSlice(r.IP.To16()); ok {
				reply.Answer = []dns.RR{&dns.AAAA{
					Hdr: dns.Header{
						Name:  qname,
						TTL:   seconds(positiveTTL),
						Class: dns.ClassINET,
					},
					AAAA: rdata.AAAA{
						Addr: addr,
					},
				}}
			}
		}
	}

	return reply
}

// soaRecord builds the zone's SOA record with the nameserver name,
// hostmaster contact, fixed serial, standard timing fields, and the
// negative TTL as MINIMUM. The record's own TTL varies by use.
func (s *Server) soaRecord(
	ttl time.Duration,
) *dns.SOA {
	return &dns.SOA{
		Hdr: dns.Header{
			Name:  s.zone,
			TTL:   seconds(ttl),
			Class: dns.ClassINET,
		},
		SOA: rdata.SOA{
			Ns:      "cord-server." + s.zone,
			Mbox:    "hostmaster." + s.zone,
			Serial:  soaSerial,
			Refresh: seconds(soaRefresh),
			Retry:   seconds(soaRetry),
			Expire:  seconds(soaExpire),
			Minttl:  seconds(negativeTTL),
		},
	}
}

// malformed answers FORMERR without echoing anything, so a request the
// server cannot interpret exposes no Cord information.
func malformed() *dns.Msg {
	reply := &dns.Msg{}
	reply.Response = true
	reply.Rcode = dns.RcodeFormatError

	return reply
}

// refused answers REFUSED, non-authoritatively: the server makes no
// statement about names it does not serve or sources it does not know.
func refused(
	query *dns.Msg,
) *dns.Msg {
	reply := &dns.Msg{}
	dnsutil.SetReply(reply, query)
	reply.Rcode = dns.RcodeRefused

	return reply
}

// nxdomain answers authoritatively that the name does not exist,
// attaching the negative-caching SOA to the authority section.
func nxdomain(
	reply *dns.Msg,
	soa *dns.SOA,
) *dns.Msg {
	reply.Rcode = dns.RcodeNameError
	reply.Authoritative = true
	reply.Ns = []dns.RR{soa}

	return reply
}

// questionName returns the query's owner name, or the empty string
// when the question cannot be read.
func questionName(
	query *dns.Msg,
) string {
	if query == nil || len(query.Question) != 1 {
		return ""
	}

	name, _ := dnsutil.Question(query)
	return name
}

// remoteIP extracts the query's source IP. Unknown address types yield
// nil, which respond treats as an unknown source.
func remoteIP(
	addr net.Addr,
) net.IP {
	switch addr := addr.(type) {
	case *net.UDPAddr:
		return addr.IP
	case *net.TCPAddr:
		return addr.IP
	default:
		return nil
	}
}

// seconds converts a duration to whole seconds for DNS TTL fields.
func seconds(
	d time.Duration,
) uint32 {
	return uint32(d.Seconds())
}
