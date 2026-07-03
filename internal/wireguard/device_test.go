package wireguard_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestDevice_ApplyPeers_AddsNewPeers(t *testing.T) {
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

func TestDevice_ApplyPeers_NoChangeOnMatch(t *testing.T) {
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

func TestDevice_ApplyPeers_RemovesStalePeer(t *testing.T) {
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

func TestDevice_ApplyPeers_ObserveError(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)
	mdev := backend.Device("test")
	mdev.PeersErr = errors.New("status failed")

	err := d.SetPeers()
	if err == nil {
		t.Fatal("expected error from observe failure")
	}
}

func TestDevice_ApplyPeers_ApplyError(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)
	mdev := backend.Device("test")
	mdev.ApplyPeersErr = errors.New("apply failed")

	k := mustGenerateKey(t)
	err := d.SetPeers(peerConfig(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0))
	if err == nil {
		t.Fatal("expected error from apply failure")
	}
}

func TestDevice_Close(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)

	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	mdev := backend.Device("test")
	if mdev.CloseCalls != 1 {
		t.Errorf("close count = %d, want 1", mdev.CloseCalls)
	}
}

func TestDevice_Name(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := createTestDevice(t, "test", backend)

	if d.Name() != "test" {
		t.Errorf("Name = %q, want test", d.Name())
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
	if ops[0].Config.Endpoint == nil || ops[0].Config.Endpoint.String() != "1.2.3.4:51820" {
		t.Errorf("endpoint = %v, want 1.2.3.4:51820", ops[0].Config.Endpoint)
	}
	// AllowedIPs and Keepalive must be preserved from the desired entry.
	if len(ops[0].Config.AllowedIPs) != 2 {
		t.Errorf("allowed IPs = %d, want 2", len(ops[0].Config.AllowedIPs))
	}
	if ops[0].Config.PersistentKeepalive != 25*time.Second {
		t.Errorf("keepalive = %v, want 25s", ops[0].Config.PersistentKeepalive)
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
	if mdev1.AppliedOps()[0].Config.PublicKey != peer1 {
		t.Errorf("dev1: expected peer %s, got %s", peer1, mdev1.AppliedOps()[0].Config.PublicKey)
	}
	if len(mdev2.AppliedOps()) != 1 {
		t.Fatalf("dev2: expected 1 operation, got %d", len(mdev2.AppliedOps()))
	}
	if mdev2.AppliedOps()[0].Config.PublicKey != peer2 {
		t.Errorf("dev2: expected peer %s, got %s", peer2, mdev2.AppliedOps()[0].Config.PublicKey)
	}

	if err := d1.Close(); err != nil {
		t.Fatalf("d1.Close: %v", err)
	}
	if err := d2.SetPeers(
		peerConfig(peer2, []string{"10.0.1.1/32"}, "", wireguard.EndpointDynamic, 0),
		peerConfig(mustGenerateKey(t), []string{"10.0.1.2/32"}, "", wireguard.EndpointDynamic, 0),
	); err != nil {
		t.Fatalf("d2.SetPeers after d1 close: %v", err)
	}

	if len(mdev2.AppliedOps()) != 2 {
		t.Fatalf("dev2: expected 2 operations after second apply, got %d", len(mdev2.AppliedOps()))
	}
	if mdev1.CloseCalls != 1 {
		t.Errorf("dev1 close count = %d, want 1", mdev1.CloseCalls)
	}
}

func TestDevice_ConcurrentReconcile(t *testing.T) {
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

	got, err := d.Peers()
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

	if _, err := d.Peers(); err == nil {
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
