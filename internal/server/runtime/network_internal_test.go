package runtime

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/logging"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/server/database"
	"git.studiopollinator.com/pollinator/cord/internal/server/service"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

// The constructor is private; keep direct Network lifecycle tests here.
func TestNetworkLifecycle(t *testing.T) {
	n, backend := newTestNetwork(t)
	main, invite := n.main, n.invite
	if n.ctx != nil || n.timer != nil || len(backend.CreateCalls) != 0 {
		t.Fatal("construction started resources or scheduled work")
	}
	initial := n.Status()
	if initial.Main.WireGuard.State != resource.Stopped || initial.Invite.WireGuard.State != resource.Stopped ||
		initial.Main.DNS.State != resource.Disabled || initial.Main.HTTP.State != resource.Disabled ||
		!initial.Reconcile.LastAttemptAt.IsZero() {
		t.Fatalf("initial status = %+v", initial)
	}
	if err := n.Stop(); err != nil {
		t.Fatal(err)
	}

	parent, cancel := context.WithCancel(t.Context())
	defer cancel()
	if err := n.Reconcile(parent); err != nil {
		t.Fatal(err)
	}
	oldCtx := n.ctx
	first := backend.Device("testnet")
	if err := n.Reconcile(parent); err != nil {
		t.Fatal(err)
	}
	if backend.Device("testnet") != first || len(backend.CreateCalls) != 2 {
		t.Fatal("healthy reconciliation recreated a device")
	}

	cancel()
	before := n.Status()
	n.reconcileScheduled(oldCtx)
	if n.Status() != before {
		t.Fatal("canceled lifetime performed scheduled maintenance")
	}
	if err := n.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if n.ctx == oldCtx || backend.Device("testnet") == first || first.CloseCalls != 1 {
		t.Fatal("reconciliation did not replace the canceled lifetime")
	}

	oldCtx = n.ctx
	if err := n.Stop(); err != nil {
		t.Fatal(err)
	}
	before = n.Status()
	if before.Main.WireGuard.State != resource.Stopped || before.Invite.WireGuard.State != resource.Stopped ||
		n.ctx != nil || n.timer != nil {
		t.Fatalf("stopped status = %+v", before)
	}
	creates := len(backend.CreateCalls)
	// Simulate an old callback that was already queued when Stop ran.
	n.reconcileScheduled(oldCtx)
	if n.Status() != before || n.timer != nil || len(backend.CreateCalls) != creates {
		t.Fatal("stale callback restarted the stopped network")
	}

	if err := n.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	before = n.Status()
	timer := n.timer
	reads := backend.Device("testnet").PeersCalls
	n.reconcileScheduled(oldCtx)
	if n.Status() != before || n.timer != timer || backend.Device("testnet").PeersCalls != reads {
		t.Fatal("old callback performed maintenance in the new lifetime")
	}
	if n.main != main || n.invite != invite {
		t.Fatal("restart replaced the persistent planes")
	}
}

func TestNetworkConcurrentLifecycle(t *testing.T) {
	n, _ := newTestNetwork(t)
	var callers sync.WaitGroup
	for range 4 {
		callers.Go(func() {
			for range 10 {
				if err := n.Reconcile(t.Context()); err != nil {
					t.Error(err)
				}
				status := n.Status()
				if status.Main.WireGuard.State != status.Invite.WireGuard.State {
					t.Errorf("snapshot captured partial transition: %+v", status)
				}
				if err := n.Stop(); err != nil {
					t.Error(err)
				}
			}
		})
	}
	callers.Wait()
}

func TestNetworkScheduledReconcileRecoversBothPlanes(t *testing.T) {
	n, backend := newTestNetwork(t)
	if err := n.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	main, invite := backend.Device("testnet"), backend.Device("testnet-i")
	if err := main.Close(); err != nil {
		t.Fatal(err)
	}
	if err := invite.Close(); err != nil {
		t.Fatal(err)
	}
	backend.CreateErr = errors.New("device unavailable")
	n.reconcileScheduled(n.ctx)
	failed := n.Status()
	if failed.Main.WireGuard.State != resource.Failed || failed.Invite.WireGuard.State != resource.Failed || failed.Reconcile.Error == "" {
		t.Fatalf("scheduled failure = %+v", failed)
	}
	backend.CreateErr = nil
	n.reconcileScheduled(n.ctx)
	recovered := n.Status()
	if recovered.Main.WireGuard.State != resource.Running || recovered.Invite.WireGuard.State != resource.Running || recovered.Reconcile.Error != "" {
		t.Fatalf("scheduled recovery = %+v", recovered)
	}
}

func newTestNetwork(
	t *testing.T,
) (
	*Network,
	*wireguardtest.MockBackend,
) {
	t.Helper()

	db, err := database.Open(database.Options{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	svc, err := service.New(service.Options{Store: db})
	if err != nil {
		t.Fatal(err)
	}
	record, err := svc.CreateNetwork(
		"testnet", "192.168.1.1",
		service.Plane{Cidr: "10.0.0.0/16", WireguardPort: 51820, ApiPort: 8080},
		service.Plane{Cidr: "10.1.0.0/24", WireguardPort: 51821, ApiPort: 8080},
	)
	if err != nil {
		t.Fatal(err)
	}
	backend := wireguardtest.NewMockBackend()
	n, err := newNetwork(networkOptions{
		record:      record,
		service:     svc,
		wireguard:   wireguard.NewManagerWithBackend(backend),
		dnsDisabled: true,
		clock:       time.Now,
		log:         logging.Discard(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := n.Stop(); err != nil {
			t.Error(err)
		}
	})
	return n, backend
}
