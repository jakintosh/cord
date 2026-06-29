package identity

import (
	"fmt"
	"net"
	"net/http"
)

type Peer struct {
	PublicKey string
	Name      string
}

// Resolver identifies a confirmed peer by source IP. Used for
// ordinary peer API calls where the caller must be fully onboarded.
type Resolver interface {
	ResolveIdentity(sourceIP net.IP) (*Peer, error)
}

// ProvisionalResolver identifies a peer that has redeemed but not
// yet confirmed. Used for the /confirm endpoint, which is the only
// call a provisional peer is allowed to make.
type ProvisionalResolver interface {
	ResolveProvisionalIdentity(sourceIP net.IP) (*Peer, error)
}

func Resolve(
	r *http.Request,
	resolver Resolver,
) (
	*Peer,
	error,
) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil, fmt.Errorf("parse remote addr: %w", err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("invalid remote IP: %s", host)
	}

	return resolver.ResolveIdentity(ip)
}

func ResolveProvisional(
	r *http.Request,
	resolver ProvisionalResolver,
) (
	*Peer,
	error,
) {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return nil, fmt.Errorf("parse remote addr: %w", err)
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("invalid remote IP: %s", host)
	}

	return resolver.ResolveProvisionalIdentity(ip)
}
