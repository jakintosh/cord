package service

import (
	"fmt"
	"strings"
)

// dnsLabelMaxBytes is the RFC 1035 limit for a single DNS label.
const dnsLabelMaxBytes = 63

// DNSHost is the transport-independent result of resolving one host
// name for one requesting peer. Route is the peer's existing Cord
// terminal host route, e.g. "10.0.0.5/32".
type DNSHost struct {
	Name  string
	Route string
}

// ResolveDNSHost resolves hostname within network for the requesting
// peer, enforcing Cord's visibility rules:
//
//   - The requesting peer can always resolve itself.
//   - The reserved "cord-server" peer is always addressable.
//   - Any other target must be confirmed, enabled, and visible to the
//     requester under the topology rules.
//   - Hidden, disabled, unconfirmed, and nonexistent names all return
//     ErrNotFound, as does an unknown network or requester.
//
// The hostname is matched case-insensitively; anything that is not a
// single stored peer name is ErrNotFound, never ErrInvalidInput. No
// DNS protocol types leak into this operation.
func (s *Service) ResolveDNSHost(
	network string,
	requestingPeer string,
	hostname string,
) (
	DNSHost,
	error,
) {
	host := strings.ToLower(hostname)

	model, err := s.getNetworkModel(network)
	if err != nil {
		return DNSHost{}, fmt.Errorf("resolve dns host: %w", err)
	}

	view, err := model.peerViews.view(model.activeTopology, requestingPeer)
	if err != nil {
		return DNSHost{}, fmt.Errorf("resolve dns host: %w", ErrNotFound)
	}

	// Self-resolution comes from the active index, not the visibility
	// list, because the subject is excluded from its own peers.
	if host == strings.ToLower(requestingPeer) {
		return dnsHostFromIndex(model, host)
	}

	if host == reservedPeerName {
		return dnsHostFromIndex(model, host)
	}

	if target, ok := view.hostsByName[host]; ok {
		return target, nil
	}

	return DNSHost{}, fmt.Errorf("resolve dns host %q: %w", hostname, ErrNotFound)
}

// dnsHostFromIndex looks a target up in the model's active peers by
// name. A confirmed peer can disappear between source resolution and
// this call, so a miss is ErrNotFound.
func dnsHostFromIndex(
	model *networkModel,
	name string,
) (
	DNSHost,
	error,
) {
	peer, ok := model.activeByName[name]
	if !ok {
		return DNSHost{}, fmt.Errorf("resolve dns host %q: %w", name, ErrNotFound)
	}

	return DNSHost{
		Name:  peer.Name,
		Route: peer.Route,
	}, nil
}

// validateDNSName checks that name is one valid lowercase DNS label:
// ASCII lowercase letters, digits, and hyphens only, with the first
// and last characters being a letter or digit. Input is rejected
// rather than silently lowercased.
func validateDNSName(
	name string,
) error {
	if name == "" {
		return fmt.Errorf("name is empty")
	}

	if len(name) > dnsLabelMaxBytes {
		return fmt.Errorf("name %q exceeds %d byte DNS label limit", name, dnsLabelMaxBytes)
	}

	if name[0] == '-' {
		return fmt.Errorf("name %q must not start with a hyphen", name)
	}

	if name[len(name)-1] == '-' {
		return fmt.Errorf("name %q must not end with a hyphen", name)
	}

	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-':
		default:
			return fmt.Errorf("name %q may contain only lowercase letters, digits, and hyphens", name)
		}
	}

	return nil
}
