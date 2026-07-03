package wireguard

import (
	"fmt"
	"net"
	"sort"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"git.studiopollinator.com/pollinator/cord/internal/netaddr"
)

// EndpointPolicy controls how Cord manages a peer's endpoint.
type EndpointPolicy int

const (
	EndpointDynamic   EndpointPolicy = iota // cord never touches endpoint; learned from handshakes (default)
	EndpointBootstrap                       // set only on initial add
	EndpointFixed                           // always reconciled
)

// PeerConfig is an external-facing peer configuration with string-based
// fields. It is parsed and validated into a Peer when passed to
// Device.SetPeers.
type PeerConfig struct {
	PublicKey           string
	AllowedIPs          []string
	Endpoint            string
	EndpointPolicy      EndpointPolicy
	PersistentKeepalive int // seconds; 0 means no keepalive
}

// Parse validates the PeerConfig and returns a parsed Peer.
func (cfg PeerConfig) Parse() (
	Peer,
	error,
) {
	key, err := parseKey(cfg.PublicKey)
	if err != nil {
		return Peer{}, fmt.Errorf("public key %q: %w", cfg.PublicKey, err)
	}

	var ips []net.IPNet
	for _, cidr := range cfg.AllowedIPs {
		ipNet, err := netaddr.ParseRoute(cidr)
		if err != nil {
			return Peer{}, fmt.Errorf("allowed-ip %q: %w", cidr, err)
		}
		ips = append(ips, ipNet)
	}

	var ep *net.UDPAddr
	if cfg.Endpoint != "" {
		ep, err = net.ResolveUDPAddr("udp", cfg.Endpoint)
		if err != nil {
			return Peer{}, fmt.Errorf("endpoint %q: %w", cfg.Endpoint, err)
		}
	}

	return Peer{
		PublicKey:           key,
		AllowedIPs:          ips,
		Endpoint:            ep,
		EndpointPolicy:      cfg.EndpointPolicy,
		PersistentKeepalive: time.Duration(cfg.PersistentKeepalive) * time.Second,
	}, nil
}

// Peer is a desired WireGuard peer. It carries only parsed
// configuration fields — runtime state like handshake time and byte
// counters lives in PeerStatus.
type Peer struct {
	PublicKey           wgtypes.Key
	AllowedIPs          []net.IPNet
	Endpoint            *net.UDPAddr
	EndpointPolicy      EndpointPolicy
	PersistentKeepalive time.Duration
}

// PeerStatus is the observed live state of a WireGuard peer returned
// by the backend. It includes runtime fields that PeerConfig does not.
type PeerStatus struct {
	PublicKey           wgtypes.Key
	AllowedIPs          []net.IPNet
	Endpoint            *net.UDPAddr
	PersistentKeepalive time.Duration
	LastHandshake       time.Time
	ReceiveBytes        int64
	TransmitBytes       int64
}

// PeerOp is a single operation that makes a live peer match a desired
// state. When Remove is true the peer identified by Target.PublicKey
// is removed and the rest of Target is ignored. When Remove is false,
// the backend makes the peer look like Target — AllowedIPs and
// PersistentKeepalive are always applied, and Endpoint is set only
// when non-nil.
//
// The planner (planPeerReconciliation) is the only place that
// understands EndpointPolicy. By the time an op reaches the backend,
// policy has been resolved: the Target.Endpoint field is either nil
// ("don't touch") or the concrete address to set.
type PeerOp struct {
	Remove bool
	Target Peer
}

// planPeerReconciliation compares desired peers with live peer state
// and produces a targeted set of PeerOps. Operations are ordered:
// removes first, then adds, then updates. This ordering prevents
// temporary conflicts (e.g. replacing a peer's allowed-IPs by
// removing and re-adding).
//
// The reason for observe→plan→apply instead of replace-all is that
// removing and re-adding a peer destroys its session and roamed
// endpoint; targeted ops leave established tunnels undisturbed, and
// Dynamic/Bootstrap endpoints are never rewritten on update so
// WireGuard roaming can do its job.
func planPeerReconciliation(
	desired []Peer,
	observed []PeerStatus,
) []PeerOp {
	desiredByKey := make(map[wgtypes.Key]Peer, len(desired))
	observedByKey := make(map[wgtypes.Key]PeerStatus, len(observed))
	keySet := make(map[wgtypes.Key]struct{}, len(desired)+len(observed))

	for _, p := range desired {
		p.AllowedIPs = normalizeAllowedIPs(p.AllowedIPs)
		desiredByKey[p.PublicKey] = p
		keySet[p.PublicKey] = struct{}{}
	}
	for _, p := range observed {
		p.AllowedIPs = normalizeAllowedIPs(p.AllowedIPs)
		observedByKey[p.PublicKey] = p
		keySet[p.PublicKey] = struct{}{}
	}

	keys := make([]wgtypes.Key, 0, len(keySet))
	for key := range keySet {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].String() < keys[j].String()
	})

	var removes, adds, updates []PeerOp
	for _, key := range keys {
		want, wanted := desiredByKey[key]
		have, exists := observedByKey[key]
		switch {
		case !wanted && exists:
			removes = append(removes, PeerOp{
				Remove: true,
				Target: Peer{PublicKey: key},
			})
		case wanted && !exists:
			op := PeerOp{Target: want}
			// Strip endpoint for Dynamic peers — WireGuard will learn
			// it from incoming handshakes.
			if want.EndpointPolicy == EndpointDynamic {
				op.Target.Endpoint = nil
			}
			adds = append(adds, op)
		case wanted && exists:
			// Emit an update only when something actually changed.
			// AllowedIPs and Keepalive are always included when we emit
			// (idempotent, cheap). Endpoint is included only for Fixed
			// peers whose live endpoint drifted.
			needUpdate := !allowedIPsEqual(want.AllowedIPs, have.AllowedIPs) ||
				want.PersistentKeepalive != have.PersistentKeepalive ||
				(want.EndpointPolicy == EndpointFixed && !endpointsEqual(want.Endpoint, have.Endpoint))
			if !needUpdate {
				continue
			}
			cfg := want
			if cfg.EndpointPolicy != EndpointFixed || endpointsEqual(cfg.Endpoint, have.Endpoint) {
				cfg.Endpoint = nil
			}
			updates = append(updates, PeerOp{Target: cfg})
		}
	}

	out := make([]PeerOp, 0, len(removes)+len(adds)+len(updates))
	out = append(out, removes...)
	out = append(out, adds...)
	out = append(out, updates...)
	return out
}

func normalizeAllowedIPs(
	ips []net.IPNet,
) []net.IPNet {
	normalized := make([]net.IPNet, len(ips))
	for i, ip := range ips {
		// Allowed IPs are routing prefixes, so the host bits are
		// meaningless. Mask them off so that a desired entry carrying
		// host bits (e.g. from netaddr.ParseRoute) compares equal
		// to the network-form value backends report. Without this,
		// every reconcile cycle would emit a spurious update.
		normalized[i] = net.IPNet{
			IP:   ip.IP.Mask(ip.Mask),
			Mask: append(net.IPMask(nil), ip.Mask...),
		}
	}
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].String() < normalized[j].String()
	})
	return normalized
}

func allowedIPsEqual(
	left []net.IPNet,
	right []net.IPNet,
) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].String() != right[i].String() {
			return false
		}
	}
	return true
}

func endpointsEqual(
	left *net.UDPAddr,
	right *net.UDPAddr,
) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.String() == right.String()
}
