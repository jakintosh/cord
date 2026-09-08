package wireguard_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestDevice_SetPeers_AddsNewPeers(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)

	k := mustGenerateKey(t)
	if err := d.SetPeers(peerConfig(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0)); err != nil {
		t.Fatalf("SetPeers: %v", err)
	}

	mdev := backend.Device("test")
	if mdev == nil {
		t.Fatal("device not found in backend")
	}
	ops := mdev.AppliedOps()
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].Remove {
		t.Error("expected add, got remove")
	}
}

func TestDevice_SetPeers_NoChangeOnMatch(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)

	k := mustGenerateKey(t)
	p := peerConfig(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0)
	// Pre-seed the backend to match desired state.
	mdev := backend.Device("test")
	mdev.SetPeers(peerStatus(k, []string{"10.0.0.1/32"}, "", 0))

	if err := d.SetPeers(p); err != nil {
		t.Fatalf("SetPeers: %v", err)
	}

	if len(mdev.AppliedOps()) != 0 {
		t.Errorf("expected 0 operations, got %d", len(mdev.AppliedOps()))
	}
}

func TestDevice_SetPeers_RemovesStalePeer(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)

	kLive := mustGenerateKey(t)
	mdev := backend.Device("test")
	mdev.SetPeers(peerStatus(kLive, []string{"10.0.0.1/32"}, "", 0))

	if err := d.SetPeers(); err != nil {
		t.Fatalf("SetPeers: %v", err)
	}

	ops := mdev.AppliedOps()
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if !ops[0].Remove {
		t.Errorf("expected remove, got add")
	}
}

func TestDevice_SetPeers_ObserveError(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)
	mdev := backend.Device("test")
	mdev.PeersErr = errors.New("status failed")

	err := d.SetPeers()
	if err == nil {
		t.Fatal("expected error from observe failure")
	}
}

func TestDevice_SetPeers_ApplyError(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)
	mdev := backend.Device("test")
	mdev.ApplyPeerOpsErr = errors.New("apply failed")

	k := mustGenerateKey(t)
	err := d.SetPeers(peerConfig(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0))
	if err == nil {
		t.Fatal("expected error from apply failure")
	}
}

func TestDevice_Stop(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)

	if err := d.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	mdev := backend.Device("test")
	if mdev.CloseCalls != 1 {
		t.Errorf("close count = %d, want 1", mdev.CloseCalls)
	}
}

func TestDevice_UpdateEndpoint_AppliesTargetedOperation(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)

	k := mustGenerateKey(t)
	p := peerConfig(k, []string{"10.0.0.1/32", "10.0.0.0/24"}, "5.6.7.8:51821", wireguard.EndpointDynamic, 25*time.Second)
	if err := d.SetPeers(p); err != nil {
		t.Fatalf("SetPeers: %v", err)
	}

	mdev := backend.Device("test")
	startLen := len(mdev.AppliedOps())

	if err := d.SetPeerEndpoint(k.String(), "1.2.3.4:51820"); err != nil {
		t.Fatalf("UpdateEndpoint: %v", err)
	}

	ops := mdev.AppliedOps()[startLen:]
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].Remove {
		t.Errorf("operation is remove, want update")
	}
	// Endpoint must be the new value.
	if ops[0].Target.Endpoint == nil || ops[0].Target.Endpoint.String() != "1.2.3.4:51820" {
		t.Errorf("endpoint = %v, want 1.2.3.4:51820", ops[0].Target.Endpoint)
	}
	// AllowedIPs and Keepalive must be preserved from the desired entry.
	if len(ops[0].Target.AllowedIPs) != 2 {
		t.Errorf("allowed IPs = %d, want 2", len(ops[0].Target.AllowedIPs))
	}
	if ops[0].Target.PersistentKeepalive != 25*time.Second {
		t.Errorf("keepalive = %v, want 25s", ops[0].Target.PersistentKeepalive)
	}
}

func TestDevice_UpdateEndpoint_UnknownPeer(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)

	err := d.SetPeerEndpoint(mustGenerateKey(t).String(), "1.2.3.4:51820")
	if err == nil {
		t.Fatal("expected error for unknown peer")
	}
}

func TestDevice_TwoDeviceIndependence(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)

	cfg1 := wireguard.DeviceConfig{
		Name:       "dev1",
		PrivateKey: mustGeneratePrivateKey(t),
		Route:      mustParseCIDR(t, "10.0.0.1/32"),
		ListenPort: 51820,
	}
	d1, err := mgr.CreateDevice(cfg1)
	if err != nil {
		t.Fatalf("CreateDevice dev1: %v", err)
	}

	cfg2 := wireguard.DeviceConfig{
		Name:       "dev2",
		PrivateKey: mustGeneratePrivateKey(t),
		Route:      mustParseCIDR(t, "10.0.1.1/32"),
		ListenPort: 51821,
	}
	d2, err := mgr.CreateDevice(cfg2)
	if err != nil {
		t.Fatalf("CreateDevice dev2: %v", err)
	}

	peer1 := mustGenerateKey(t)
	peer2 := mustGenerateKey(t)

	if err := d1.SetPeers(peerConfig(peer1, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0)); err != nil {
		t.Fatalf("d1.SetPeers: %v", err)
	}
	if err := d2.SetPeers(peerConfig(peer2, []string{"10.0.1.1/32"}, "", wireguard.EndpointDynamic, 0)); err != nil {
		t.Fatalf("d2.SetPeers: %v", err)
	}

	mdev1 := backend.Device("dev1")
	mdev2 := backend.Device("dev2")

	if len(mdev1.AppliedOps()) != 1 {
		t.Fatalf("dev1: expected 1 operation, got %d", len(mdev1.AppliedOps()))
	}
	if mdev1.AppliedOps()[0].Target.PublicKey != peer1 {
		t.Errorf("dev1: expected peer %s, got %s", peer1, mdev1.AppliedOps()[0].Target.PublicKey)
	}
	if len(mdev2.AppliedOps()) != 1 {
		t.Fatalf("dev2: expected 1 operation, got %d", len(mdev2.AppliedOps()))
	}
	if mdev2.AppliedOps()[0].Target.PublicKey != peer2 {
		t.Errorf("dev2: expected peer %s, got %s", peer2, mdev2.AppliedOps()[0].Target.PublicKey)
	}

	if err := d1.Stop(); err != nil {
		t.Fatalf("d1.Stop: %v", err)
	}
	if err := d2.SetPeers(
		peerConfig(peer2, []string{"10.0.1.1/32"}, "", wireguard.EndpointDynamic, 0),
		peerConfig(mustGenerateKey(t), []string{"10.0.1.2/32"}, "", wireguard.EndpointDynamic, 0),
	); err != nil {
		t.Fatalf("d2.SetPeers after d1 stop: %v", err)
	}

	if len(mdev2.AppliedOps()) != 2 {
		t.Fatalf("dev2: expected 2 operations after second apply, got %d", len(mdev2.AppliedOps()))
	}
	if mdev1.CloseCalls != 1 {
		t.Errorf("dev1 close count = %d, want 1", mdev1.CloseCalls)
	}
}

func TestDevice_ConcurrentSetPeers(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)

	k1 := mustGenerateKey(t)
	k2 := mustGenerateKey(t)

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		<-start
		for range 50 {
			_ = d.SetPeers(peerConfig(k1, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0))
		}
	}()

	go func() {
		defer wg.Done()
		<-start
		for range 50 {
			_ = d.SetPeers(peerConfig(k2, []string{"10.0.1.1/32"}, "", wireguard.EndpointDynamic, 0))
		}
	}()

	close(start)
	wg.Wait()

	// The race detector (enabled via `make test`) is what actually
	// validates concurrent safety here; absence of panics alone is
	// not sufficient.
}

func TestDevice_Peers_ReturnsLiveState(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)
	mdev := backend.Device("test")

	k := mustGenerateKey(t)
	mdev.SetPeers(peerStatus(k, []string{"10.0.0.1/32"}, "1.2.3.4:51820", 0))

	got, err := d.GetPeerStatuses()
	if err != nil {
		t.Fatalf("Peers: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 peer, got %d", len(got))
	}
	if got[0].PublicKey != k {
		t.Errorf("public key = %v, want %v", got[0].PublicKey, k)
	}
	if got[0].Endpoint == nil || got[0].Endpoint.String() != "1.2.3.4:51820" {
		t.Errorf("endpoint = %v, want 1.2.3.4:51820", got[0].Endpoint)
	}
	if mdev.PeersCalls != 1 {
		t.Errorf("PeersCalls = %d, want 1", mdev.PeersCalls)
	}
}

func TestDevice_Peers_BackendError(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)
	mdev := backend.Device("test")
	mdev.PeersErr = errors.New("status failed")

	if _, err := d.GetPeerStatuses(); err == nil {
		t.Fatal("expected error from observe failure")
	}
}

func TestDevice_UpdateEndpoint_InvalidKey(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)

	if err := d.SetPeerEndpoint("not-a-key", "1.2.3.4:51820"); err == nil {
		t.Fatal("expected error for invalid public key")
	}
}

func TestDevice_UpdateEndpoint_InvalidEndpoint(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)

	// Valid key but malformed endpoint string.
	key := mustGenerateKey(t).String()
	if err := d.SetPeerEndpoint(key, "not an endpoint"); err == nil {
		t.Fatal("expected error for invalid endpoint")
	}
}

func TestDevice_ReconcileRetainsPeersAcrossRecovery(
	t *testing.T,
) {
	backend := wireguardtest.NewMockBackend()
	manager := wireguard.NewManagerWithBackend(backend)
	d, err := manager.NewDevice(wireguard.DeviceConfig{
		Name:       "test",
		PrivateKey: mustGeneratePrivateKey(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Stop() })

	if status := d.Status(); status.State != resource.Stopped || status.Error != nil {
		t.Fatalf("initial status = %+v", status)
	}
	if backend.Device("test") != nil {
		t.Fatal("construction created an interface")
	}
	if _, err := d.GetPeerStatuses(); err == nil {
		t.Fatal("stopped device should not report live peers")
	}
	if err := d.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}

	key := mustGenerateKey(t)
	if err := d.SetPeers(peerConfig(key, []string{"10.0.0.1/32"}, "1.2.3.4:51820", wireguard.EndpointBootstrap, 25*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := d.SetPeerEndpoint(key.String(), "5.6.7.8:51820"); err != nil {
		t.Fatal(err)
	}
	original := backend.Device("test")
	if err := d.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if backend.Device("test") != original || len(original.ApplyCalls) != 2 {
		t.Fatal("healthy reconciliation replaced or reconfigured the interface")
	}

	// A backend fault is observable without repairing or consuming it.
	if err := original.Close(); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if status := d.Status(); status.State != resource.Failed || status.Error == nil {
			t.Fatalf("failed status = %+v", status)
		}
	}
	if backend.Device("test") != original {
		t.Fatal("status repaired the interface")
	}
	if err := d.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if backend.Device("test") == original {
		t.Fatal("failed interface was not replaced")
	}
	if status := d.Status(); status.State != resource.Running || status.Error != nil {
		t.Fatalf("recovered status = %+v", status)
	}
	peers, err := d.GetPeerStatuses()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].PublicKey != key ||
		peers[0].Endpoint.String() != "5.6.7.8:51820" ||
		len(peers[0].AllowedIPs) != 1 || peers[0].AllowedIPs[0].String() != "10.0.0.1/32" ||
		peers[0].PersistentKeepalive != 25*time.Second {
		t.Fatalf("restored peers = %+v", peers)
	}

	if err := d.Stop(); err != nil {
		t.Fatal(err)
	}
	if err := d.Stop(); err != nil {
		t.Fatal(err)
	}
	if status := d.Status(); status.State != resource.Stopped || status.Error != nil {
		t.Fatalf("stopped status = %+v", status)
	}
	if err := d.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if peers, err := d.GetPeerStatuses(); err != nil || len(peers) != 1 {
		t.Fatalf("peers after intentional restart = %+v, %v", peers, err)
	}
}

func TestDevice_ReconcileRetriesCreationAndCleanup(
	t *testing.T,
) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)
	t.Cleanup(func() { _ = d.Stop() })
	original := backend.Device("test")
	failed := errors.New("interface failed")
	closeErr := errors.New("close failed")
	createErr := errors.New("create failed")
	original.CheckErr = failed
	original.CloseErr = closeErr

	if err := d.Reconcile(t.Context()); !errors.Is(err, closeErr) {
		t.Fatalf("cleanup error = %v", err)
	}
	if backend.Device("test") != original || len(backend.CreateCalls) != 1 {
		t.Fatal("created a replacement before cleanup succeeded")
	}

	original.CloseErr = nil
	backend.CreateErr = createErr
	if err := d.Reconcile(t.Context()); !errors.Is(err, createErr) {
		t.Fatalf("creation error = %v", err)
	}
	if err := d.Stop(); err != nil {
		t.Fatal(err)
	}
	if status := d.Status(); status.State != resource.Failed || !errors.Is(status.Error, createErr) {
		t.Fatalf("cleanup discarded startup failure: %+v", status)
	}

	backend.CreateErr = nil
	if err := d.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if status := d.Status(); status.State != resource.Running || status.Error != nil {
		t.Fatalf("recovered status = %+v", status)
	}
	if original.CloseCalls != 2 {
		t.Fatalf("close attempts = %d, want 2", original.CloseCalls)
	}
}

func TestDevice_ReconcileCancellationDoesNotCreate(
	t *testing.T,
) {
	backend := wireguardtest.NewMockBackend()
	manager := wireguard.NewManagerWithBackend(backend)
	d, err := manager.NewDevice(wireguard.DeviceConfig{
		Name:       "test",
		PrivateKey: mustGeneratePrivateKey(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := d.Reconcile(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("reconcile = %v", err)
	}
	if backend.Device("test") != nil {
		t.Fatal("canceled reconciliation created an interface")
	}
}

func TestDevice_ReconcileRetainsPartialHandle(
	t *testing.T,
) {
	backend := &partialBackend{
		MockBackend: wireguardtest.NewMockBackend(),
		createErr:   errors.New("partial creation"),
		closeErr:    errors.New("cleanup failed"),
	}
	d, err := wireguard.NewManagerWithBackend(backend).NewDevice(wireguard.DeviceConfig{
		Name:       "test",
		PrivateKey: mustGeneratePrivateKey(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Stop() })

	if err := d.Reconcile(t.Context()); !errors.Is(err, backend.createErr) || !errors.Is(err, backend.closeErr) {
		t.Fatalf("partial creation = %v", err)
	}
	partial := backend.Device("test")
	if partial.CloseCalls != 1 {
		t.Fatalf("cleanup attempts = %d, want only the backend's attempt", partial.CloseCalls)
	}
	if err := d.Reconcile(t.Context()); !errors.Is(err, backend.closeErr) {
		t.Fatalf("retry cleanup = %v", err)
	}
	if len(backend.CreateCalls) != 1 {
		t.Fatal("lost partial handle and created another interface")
	}

	partial.CloseErr = nil
	backend.createErr, backend.closeErr = nil, nil
	if err := d.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if backend.Device("test") == partial || d.Status().State != resource.Running {
		t.Fatal("did not recover after partial cleanup succeeded")
	}
}

func TestDevice_ReconcileRetriesPeerRestoration(
	t *testing.T,
) {
	backend := &partialBackend{MockBackend: wireguardtest.NewMockBackend()}
	d, err := wireguard.NewManagerWithBackend(backend).CreateDevice(wireguard.DeviceConfig{
		Name:       "test",
		PrivateKey: mustGeneratePrivateKey(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Stop() })
	key := mustGenerateKey(t)
	if err := d.SetPeers(peerConfig(key, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0)); err != nil {
		t.Fatal(err)
	}
	if err := d.Stop(); err != nil {
		t.Fatal(err)
	}

	backend.applyErr = errors.New("restore failed")
	if err := d.Reconcile(t.Context()); !errors.Is(err, backend.applyErr) {
		t.Fatalf("restore = %v", err)
	}
	if d.Status().State != resource.Failed || backend.Device("test").CloseCalls != 1 {
		t.Fatal("failed restoration was not reported and cleaned up")
	}

	backend.applyErr = nil
	if err := d.Reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	peers, err := d.GetPeerStatuses()
	if err != nil || len(peers) != 1 || peers[0].PublicKey != key {
		t.Fatalf("restored peers = %+v, %v", peers, err)
	}
}

func TestCreateDevice_ReturnsHandleWhenCleanupFails(
	t *testing.T,
) {
	backend := &partialBackend{
		MockBackend: wireguardtest.NewMockBackend(),
		createErr:   errors.New("partial creation"),
		closeErr:    errors.New("cleanup failed"),
	}
	d, err := wireguard.NewManagerWithBackend(backend).CreateDevice(wireguard.DeviceConfig{
		Name:       "test",
		PrivateKey: mustGeneratePrivateKey(t),
	})
	if d == nil || !errors.Is(err, backend.closeErr) {
		t.Fatalf("create = %v, %v; want retained cleanup handle", d, err)
	}

	backend.Device("test").CloseErr = nil
	if err := d.Stop(); err != nil {
		t.Fatalf("retry cleanup: %v", err)
	}
}

// partialBackend models real backend failures after allocating an interface.
type partialBackend struct {
	*wireguardtest.MockBackend
	createErr error
	closeErr  error
	applyErr  error
}

func (b *partialBackend) CreateDevice(
	cfg wireguard.DeviceConfig,
) (
	wireguard.WgDevice,
	error,
) {
	handle, err := b.MockBackend.CreateDevice(cfg)
	if err != nil {
		return nil, err
	}
	device := b.Device(cfg.Name)
	device.CloseErr = b.closeErr
	device.ApplyPeerOpsErr = b.applyErr
	if b.createErr != nil {
		if closeErr := handle.Close(); closeErr != nil {
			return handle, errors.Join(b.createErr, closeErr)
		}
		return nil, b.createErr
	}
	return handle, nil
}
