package runtime

import (
	"context"
	"net"

	"git.studiopollinator.com/pollinator/cord/internal/server/service"
)

// ResolveRegistrationIdentity looks up an unconfirmed, unexpired
// registration by temporary IP within the invite network.
func (r *Runtime) ResolveRegistrationIdentity(
	ctx context.Context,
	networkName string,
	ip net.IP,
) (
	*service.Registration,
	error,
) {
	return executeValue(r, ctx, func() (*service.Registration, error) {
		return r.service.ResolveRegistrationIdentity(networkName, ip)
	})
}

// ResolvePeerIdentity looks up a confirmed, enabled peer by IP address
// within the network.
func (r *Runtime) ResolvePeerIdentity(
	ctx context.Context,
	network string,
	ip net.IP,
) (
	*service.Peer,
	error,
) {
	return executeValue(r, ctx, func() (*service.Peer, error) {
		return r.service.ResolvePeerIdentity(network, ip)
	})
}

// ResolveProvisionalIdentity looks up an unconfirmed, enabled peer by
// IP address within the network.
func (r *Runtime) ResolveProvisionalIdentity(
	ctx context.Context,
	network string,
	ip net.IP,
) (
	*service.Peer,
	error,
) {
	return executeValue(r, ctx, func() (*service.Peer, error) {
		return r.service.ResolveProvisionalIdentity(network, ip)
	})
}
