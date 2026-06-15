package wireguard

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// EndpointPolicy controls how Cord manages a peer endpoint.
type EndpointPolicy int

const (
	// EndpointDynamic leaves endpoint selection entirely to WireGuard.
	EndpointDynamic EndpointPolicy = iota
	// EndpointBootstrap supplies an endpoint when adding a peer, then allows roaming.
	EndpointBootstrap
	// EndpointFixed continuously enforces the configured endpoint.
	EndpointFixed
)

// PeerOperationType identifies one targeted change to a live WireGuard peer.
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

// PeerOperation is one targeted operation needed to reconcile a live device.
type PeerOperation struct {
	Type PeerOperationType
	Peer Peer

	UpdateAllowedIPs bool
	UpdateEndpoint   bool
	UpdateKeepalive  bool
}

// ReconcilePlan is the deterministic set of operations needed to make live
// WireGuard peer configuration match Cord's desired peer configuration.
type ReconcilePlan struct {
	Operations []PeerOperation
}

// OperationCounts returns the number of adds, updates, and removes in the plan.
func (p ReconcilePlan) OperationCounts() (adds, updates, removes int) {
	for _, operation := range p.Operations {
		switch operation.Type {
		case PeerAdd:
			adds++
		case PeerUpdate:
			updates++
		case PeerRemove:
			removes++
		}
	}
	return adds, updates, removes
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

// ReconcileError records a failed application while preserving the plan for
// later retry. Reconciliation always re-plans against live state before retry.
type ReconcileError struct {
	Operation string
	PublicKey string
	Message   string
}

// ReconcileStatus is the most recent reconciliation state for an interface.
type ReconcileStatus struct {
	LastAttempt time.Time
	LastSuccess time.Time
	Desired     int
	Observed    int
	Pending     []PeerOperation
	Errors      []ReconcileError
}

// PlanPeerReconciliation compares desired Cord peers with observed WireGuard
// peers. Runtime-only state such as learned endpoints and handshakes is ignored.
func PlanPeerReconciliation(desired []DesiredPeer, observed []ObservedPeer) ReconcilePlan {
	desiredByKey := make(map[wgtypes.Key]DesiredPeer, len(desired))
	observedByKey := make(map[wgtypes.Key]ObservedPeer, len(observed))
	keySet := make(map[wgtypes.Key]struct{}, len(desired)+len(observed))

	for _, peer := range desired {
		peer.AllowedIPs = normalizeAllowedIPs(peer.AllowedIPs)
		desiredByKey[peer.PublicKey] = peer
		keySet[peer.PublicKey] = struct{}{}
	}
	for _, peer := range observed {
		peer.AllowedIPs = normalizeAllowedIPs(peer.AllowedIPs)
		observedByKey[peer.PublicKey] = peer
		keySet[peer.PublicKey] = struct{}{}
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
				Peer: Peer{PublicKey: key},
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

func normalizeAllowedIPs(ips []net.IPNet) []net.IPNet {
	normalized := append([]net.IPNet(nil), ips...)
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i].String() < normalized[j].String()
	})
	return normalized
}

func allowedIPsEqual(left, right []net.IPNet) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].String() != right[index].String() {
			return false
		}
	}
	return true
}

func endpointsEqual(left, right *net.UDPAddr) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.String() == right.String()
}

func reconciliationError(plan ReconcilePlan, err error) []ReconcileError {
	errors := make([]ReconcileError, 0, len(plan.Operations))
	for _, op := range plan.Operations {
		errors = append(errors, ReconcileError{
			Operation: op.Type.String(),
			PublicKey: shortKey(op.Peer.PublicKey),
			Message:   err.Error(),
		})
	}
	return errors
}

func shortKey(key wgtypes.Key) string {
	value := key.String()
	if len(value) <= 8 {
		return value
	}
	return fmt.Sprintf("%s...", value[:8])
}

func wgPeerConfig(operation PeerOperation) wgtypes.PeerConfig {
	peer := operation.Peer
	config := wgtypes.PeerConfig{PublicKey: peer.PublicKey}
	switch operation.Type {
	case PeerRemove:
		config.Remove = true
	case PeerAdd:
		config.ReplaceAllowedIPs = true
		config.AllowedIPs = peer.AllowedIPs
		if peer.EndpointPolicy != EndpointDynamic {
			config.Endpoint = peer.Endpoint
		}
		keepalive := peer.PersistentKeepalive
		config.PersistentKeepaliveInterval = &keepalive
	case PeerUpdate:
		config.UpdateOnly = true
		if operation.UpdateAllowedIPs {
			config.ReplaceAllowedIPs = true
			config.AllowedIPs = peer.AllowedIPs
		}
		if operation.UpdateEndpoint {
			config.Endpoint = peer.Endpoint
		}
		if operation.UpdateKeepalive {
			keepalive := peer.PersistentKeepalive
			config.PersistentKeepaliveInterval = &keepalive
		}
	}
	return config
}
