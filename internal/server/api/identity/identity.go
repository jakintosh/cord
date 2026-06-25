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

type Resolver interface {
	ResolveIdentity(sourceIP net.IP) (*Peer, error)
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
