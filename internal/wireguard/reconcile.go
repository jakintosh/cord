package wireguard

import (
	"net"
	"sort"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// PeerOperationType identifies one targeted change to a live
// WireGuard peer.
type PeerOperationType int

const (
	PeerAdd PeerOperationType = iota
	PeerUpdate
	PeerRemove
)

func (t PeerOperationType) String() string {
	switch t {
	case PeerAdd:
		return "add"
	case PeerUpdate:
		return "update"
	case PeerRemove:
		return "remove"
	default:
		return "unknown"
	}
}

// PeerOperation is one targeted operation needed to reconcile a
// live device's peer set against the desired configuration.
type PeerOperation struct {
	Type PeerOperationType
	Peer desiredPeer

	UpdateAllowedIPs bool
	UpdateEndpoint   bool
	UpdateKeepalive  bool
}

// desiredPeer is the cord-authored configuration for one peer.
type desiredPeer struct {
	PublicKey           wgtypes.Key
	AllowedIPs          []net.IPNet
	Endpoint            *net.UDPAddr
	EndpointPolicy      EndpointPolicy
	PersistentKeepalive time.Duration
}

// ReconcilePlan is the deterministic set of operations needed to
// make live WireGuard peer configuration match the desired set.
type ReconcilePlan struct {
	Operations []PeerOperation
}

// OperationCounts returns the number of adds, updates, and removes
// in the plan.
func (p ReconcilePlan) OperationCounts() (adds, updates, removes int) {
	for _, op := range p.Operations {
		switch op.Type {
		case PeerAdd:
			adds++
		case PeerUpdate:
			updates++
		case PeerRemove:
			removes++
		}
	}
	return
}

// Fields summarizes the durable peer fields affected by an operation.
func (o PeerOperation) Fields() string {
	switch o.Type {
	case PeerAdd:
		fields := []string{"allowed-ips", "keepalive"}
		if o.Peer.EndpointPolicy != EndpointDynamic && o.Peer.Endpoint != nil {
			fields = append(fields, "endpoint")
		}
		return strings.Join(fields, ",")
	case PeerRemove:
		return "peer"
	case PeerUpdate:
		var fields []string
		if o.UpdateAllowedIPs {
			fields = append(fields, "allowed-ips")
		}
		if o.UpdateEndpoint {
			fields = append(fields, "endpoint")
		}
		if o.UpdateKeepalive {
			fields = append(fields, "keepalive")
		}
		return strings.Join(fields, ",")
	default:
		return "unknown"
	}
}

// ReconciliationStage identifies which phase of reconciliation failed.
type ReconciliationStage int

const (
	StageObserve ReconciliationStage = iota
	StageApply
)

// ReconcileError records the stage that failed and why.
type ReconcileError struct {
	Stage   ReconciliationStage
	Message string
}

// ReconcileStatus is the most recent reconciliation state.
type ReconcileStatus struct {
	LastAttempt time.Time
	LastSuccess time.Time
	Desired     int
	Observed    int
	Error       *ReconcileError
}

// Degraded reports whether the last reconciliation attempt failed.
func (s ReconcileStatus) Degraded() bool {
	return s.LastSuccess.IsZero() || s.LastSuccess.Before(s.LastAttempt)
}

// PlanPeerReconciliation compares desired peers with live peer state
// and produces a targeted reconciliation plan.
func PlanPeerReconciliation(
	desired []desiredPeer,
	observed []ObservedPeer,
) ReconcilePlan {
	desiredByKey := make(map[wgtypes.Key]desiredPeer, len(desired))
	observedByKey := make(map[wgtypes.Key]ObservedPeer, len(observed))
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

	var removes, adds, updates []PeerOperation
	for _, key := range keys {
		want, wanted := desiredByKey[key]
		have, exists := observedByKey[key]
		switch {
		case !wanted && exists:
			removes = append(removes, PeerOperation{
				Type: PeerRemove,
				Peer: desiredPeer{PublicKey: key},
			})
		case wanted && !exists:
			adds = append(adds, PeerOperation{Type: PeerAdd, Peer: want})
		case wanted && exists:
			op := PeerOperation{Type: PeerUpdate, Peer: want}
			op.UpdateAllowedIPs = !allowedIPsEqual(want.AllowedIPs, have.AllowedIPs)
			op.UpdateKeepalive = want.PersistentKeepalive != have.PersistentKeepalive
			op.UpdateEndpoint = want.EndpointPolicy == EndpointFixed &&
				!endpointsEqual(want.Endpoint, have.Endpoint)
			if op.UpdateAllowedIPs || op.UpdateKeepalive || op.UpdateEndpoint {
				updates = append(updates, op)
			}
		}
	}

	return ReconcilePlan{
		Operations: append(append(removes, adds...), updates...),
	}
}

func normalizeAllowedIPs(
	ips []net.IPNet,
) []net.IPNet {
	normalized := append([]net.IPNet(nil), ips...)
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

func shortKey(
	key wgtypes.Key,
) string {
	s := key.String()
	if len(s) <= 8 {
		return s
	}
	return s[:8] + "..."
}

func wgPeerConfig(
	op PeerOperation,
) wgtypes.PeerConfig {
	peer := op.Peer
	cfg := wgtypes.PeerConfig{PublicKey: peer.PublicKey}
	switch op.Type {
	case PeerRemove:
		cfg.Remove = true
	case PeerAdd:
		cfg.ReplaceAllowedIPs = true
		cfg.AllowedIPs = peer.AllowedIPs
		switch peer.EndpointPolicy {
		case EndpointBootstrap, EndpointFixed:
			cfg.Endpoint = peer.Endpoint
		}
		keepalive := peer.PersistentKeepalive
		cfg.PersistentKeepaliveInterval = &keepalive
	case PeerUpdate:
		cfg.UpdateOnly = true
		if op.UpdateAllowedIPs {
			cfg.ReplaceAllowedIPs = true
			cfg.AllowedIPs = peer.AllowedIPs
		}
		if op.UpdateEndpoint {
			cfg.Endpoint = peer.Endpoint
		}
		if op.UpdateKeepalive {
			keepalive := peer.PersistentKeepalive
			cfg.PersistentKeepaliveInterval = &keepalive
		}
	}
	return cfg
}
