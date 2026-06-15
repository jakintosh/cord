package wireguard

import (
	"errors"
	"net"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type reconcileBackend struct {
	observed   []ObservedPeer
	applied    [][]PeerOperation
	applyError error
}

func (b *reconcileBackend) Up(*Interface, string) error {
	return nil
}

func (b *reconcileBackend) Down(*Interface, bool) error {
	return nil
}

func (b *reconcileBackend) Status(*Interface) (*DeviceStatus, error) {
	return &DeviceStatus{Peers: b.observed}, nil
}

func (b *reconcileBackend) ApplyPeerOperations(_ *Interface, operations []PeerOperation) error {
	b.applied = append(b.applied, operations)
	return b.applyError
}

func TestInterface_ReconcileNoOpDoesNotApply(t *testing.T) {
	key := wgtypes.Key{1}
	_, allowed, _ := net.ParseCIDR("10.0.0.2/32")
	backend := &reconcileBackend{
		observed: []ObservedPeer{{PublicKey: key, AllowedIPs: []net.IPNet{*allowed}}},
	}
	iface := &Interface{
		Peers:   []Peer{{PublicKey: key, AllowedIPs: []net.IPNet{*allowed}}},
		backend: backend,
	}

	if err := iface.Reconcile(); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	if len(backend.applied) != 0 {
		t.Fatalf("apply calls = %d, want 0", len(backend.applied))
	}
	if iface.ReconcileStatus().LastSuccess.IsZero() {
		t.Fatal("last success not recorded")
	}
}

func TestInterface_ReconcileFailureTracksPendingAndReplans(t *testing.T) {
	key := wgtypes.Key{1}
	_, allowed, _ := net.ParseCIDR("10.0.0.2/32")
	backend := &reconcileBackend{applyError: errors.New("device unavailable")}
	iface := &Interface{
		Peers:   []Peer{{PublicKey: key, AllowedIPs: []net.IPNet{*allowed}}},
		backend: backend,
	}

	if err := iface.Reconcile(); err == nil {
		t.Fatal("expected reconcile failure")
	}
	status := iface.ReconcileStatus()
	if len(status.Pending) != 1 || len(status.Errors) != 1 {
		t.Fatalf("unexpected failed status: %+v", status)
	}

	backend.applyError = nil
	backend.observed = []ObservedPeer{{PublicKey: key, AllowedIPs: []net.IPNet{*allowed}}}
	if err := iface.Reconcile(); err != nil {
		t.Fatalf("retry reconcile: %v", err)
	}
	status = iface.ReconcileStatus()
	if len(status.Pending) != 0 || len(status.Errors) != 0 {
		t.Fatalf("unexpected recovered status: %+v", status)
	}
	if len(backend.applied) != 1 {
		t.Fatalf("apply calls = %d, stale plan was replayed", len(backend.applied))
	}
}
