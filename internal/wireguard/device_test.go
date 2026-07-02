package wireguard_test

import (
	"errors"
	"sync"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestDevice_ApplyPeers_AddsNewPeers(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStartedTestDevice(t, "test", backend)

	k := mustGenerateKey(t)
	if err := d.SetPeers(peer(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0)); err != nil {
		t.Fatalf("ApplyPeers: %v", err)
	}

	ops := backend.LastAppliedOpsFor("test")
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].Type != wireguard.PeerAdd {
		t.Errorf("expected add, got %s", ops[0].Type)
	}
}

func TestDevice_ApplyPeers_NoChangeOnMatch(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStartedTestDevice(t, "test", backend)

	k := mustGenerateKey(t)
	p := peer(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0)
	backend.SetPeers("test", p)

	if err := d.SetPeers(p); err != nil {
		t.Fatalf("ApplyPeers: %v", err)
	}

	ops := backend.LastAppliedOpsFor("test")
	if len(ops) != 0 {
		t.Errorf("expected 0 operations, got %d", len(ops))
	}
}

func TestDevice_ApplyPeers_RemovesStalePeer(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStartedTestDevice(t, "test", backend)

	kLive := mustGenerateKey(t)
	backend.SetPeers("test", peer(kLive, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0))

	if err := d.SetPeers(); err != nil {
		t.Fatalf("ApplyPeers: %v", err)
	}

	ops := backend.LastAppliedOpsFor("test")
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].Type != wireguard.PeerRemove {
		t.Errorf("expected remove, got %s", ops[0].Type)
	}
}

func TestDevice_ApplyPeers_ObserveError(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStartedTestDevice(t, "test", backend)
	backend.PeersErr = errors.New("status failed")

	err := d.SetPeers()
	if err == nil {
		t.Fatal("expected error from observe failure")
	}

	st := d.ReconcileStatus()
	if st.Error == nil {
		t.Fatal("expected reconcile error")
	}
	if st.Error.Stage != wireguard.StageObserve {
		t.Errorf("error stage = %v, want StageObserve", st.Error.Stage)
	}
}

func TestDevice_ApplyPeers_ApplyError(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStartedTestDevice(t, "test", backend)
	backend.ApplyErr = errors.New("apply failed")

	k := mustGenerateKey(t)
	err := d.SetPeers(peer(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0))
	if err == nil {
		t.Fatal("expected error from apply failure")
	}

	st := d.ReconcileStatus()
	if st.Error == nil {
		t.Fatal("expected reconcile error")
	}
	if st.Error.Stage != wireguard.StageApply {
		t.Errorf("error stage = %v, want StageApply", st.Error.Stage)
	}
	if !st.Degraded() {
		t.Error("expected Degraded=true after apply failure")
	}
}

func TestDevice_ReconcileStatus_RecordsSuccess(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStartedTestDevice(t, "test", backend)

	k := mustGenerateKey(t)
	if err := d.SetPeers(peer(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0)); err != nil {
		t.Fatalf("ApplyPeers: %v", err)
	}

	st := d.ReconcileStatus()
	if st.Degraded() {
		t.Error("expected Degraded=false after success")
	}
	if st.Desired != 1 {
		t.Errorf("Desired = %d, want 1", st.Desired)
	}
	if st.Observed != 0 {
		t.Errorf("Observed = %d, want 0", st.Observed)
	}
}

func TestDevice_Up(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStoppedTestDevice(t, "test", backend)

	if err := d.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}

	if got := backend.UpCount("test"); got != 1 {
		t.Errorf("up count = %d, want 1", got)
	}
}

func TestDevice_Up_Error(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	backend.UpErr = errors.New("up failed")
	d := newStoppedTestDevice(t, "test", backend)

	err := d.Up()
	if err == nil {
		t.Fatal("expected error from Up")
	}
}

func TestDevice_Down(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStartedTestDevice(t, "test", backend)

	if err := d.Down(); err != nil {
		t.Fatalf("Down: %v", err)
	}

	if got := backend.DownCount("test"); got != 1 {
		t.Errorf("down count = %d, want 1", got)
	}
}

func TestDevice_Name(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStoppedTestDevice(t, "test", backend)

	if d.Name() != "test" {
		t.Errorf("Name = %q, want test", d.Name())
	}
}

func TestDevice_SetLogger(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStoppedTestDevice(t, "test", backend)

	var logged string
	d.SetLogger(func(format string, args ...any) {
		logged = format
	})

	if err := d.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}

	if logged == "" {
		t.Error("expected logger to be called during reconcile")
	}
}

func TestDevice_ApplyPeers_NotUp(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStoppedTestDevice(t, "test", backend)

	k := mustGenerateKey(t)
	err := d.SetPeers(peer(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0))
	if !errors.Is(err, wireguard.ErrDeviceNotUp) {
		t.Fatalf("expected ErrDeviceNotUp, got %v", err)
	}
	if got := backend.ApplyCountFor("test"); got != 0 {
		t.Errorf("apply count = %d, want 0", got)
	}
}

func TestDevice_Up_ReconcilesStoredPeers(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStoppedTestDevice(t, "test", backend)

	k := mustGenerateKey(t)
	err := d.SetPeers(peer(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0))
	if !errors.Is(err, wireguard.ErrDeviceNotUp) {
		t.Fatalf("expected ErrDeviceNotUp, got %v", err)
	}
	if got := backend.ApplyCountFor("test"); got != 0 {
		t.Errorf("apply count before Up = %d, want 0", got)
	}

	if err := d.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}

	ops := backend.LastAppliedOpsFor("test")
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation after Up, got %d", len(ops))
	}
	if ops[0].Type != wireguard.PeerAdd {
		t.Errorf("expected add, got %s", ops[0].Type)
	}
}

func TestDevice_Peers_NotUp(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStoppedTestDevice(t, "test", backend)

	_, err := d.GetPeers()
	if !errors.Is(err, wireguard.ErrDeviceNotUp) {
		t.Fatalf("expected ErrDeviceNotUp, got %v", err)
	}
}

func TestDevice_UpdateEndpoint_NotUp(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStoppedTestDevice(t, "test", backend)

	err := d.UpdateEndpoint(mustGenerateKey(t).String(), "1.2.3.4:51820")
	if !errors.Is(err, wireguard.ErrDeviceNotUp) {
		t.Fatalf("expected ErrDeviceNotUp, got %v", err)
	}
}

func TestDevice_UpdateEndpoint_AppliesTargetedOperation(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStartedTestDevice(t, "test", backend)

	k := mustGenerateKey(t)
	p := peer(k, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0)
	if err := d.SetPeers(p); err != nil {
		t.Fatalf("ApplyPeers: %v", err)
	}

	if err := d.UpdateEndpoint(k.String(), "1.2.3.4:51820"); err != nil {
		t.Fatalf("UpdateEndpoint: %v", err)
	}

	ops := backend.LastAppliedOpsFor("test")
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].Type != wireguard.PeerUpdate {
		t.Errorf("operation type = %s, want update", ops[0].Type)
	}
	if !ops[0].UpdateEndpoint {
		t.Error("expected UpdateEndpoint=true")
	}
	if ops[0].Peer.Endpoint == nil || ops[0].Peer.Endpoint.String() != "1.2.3.4:51820" {
		t.Errorf("endpoint = %v, want 1.2.3.4:51820", ops[0].Peer.Endpoint)
	}
}

func TestDevice_TwoDeviceIndependence(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)

	d1 := newStoppedDeviceFromManager(t, mgr, "dev1", 51820)
	d2 := newStoppedDeviceFromManager(t, mgr, "dev2", 51821)
	if err := d1.Up(); err != nil {
		t.Fatalf("d1.Up: %v", err)
	}
	if err := d2.Up(); err != nil {
		t.Fatalf("d2.Up: %v", err)
	}

	peer1 := mustGenerateKey(t)
	peer2 := mustGenerateKey(t)

	if err := d1.SetPeers(peer(peer1, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0)); err != nil {
		t.Fatalf("d1.ApplyPeers: %v", err)
	}
	if err := d2.SetPeers(peer(peer2, []string{"10.0.1.1/32"}, "", wireguard.EndpointDynamic, 0)); err != nil {
		t.Fatalf("d2.ApplyPeers: %v", err)
	}

	ops1 := backend.AppliedOpsFor("dev1")
	ops2 := backend.AppliedOpsFor("dev2")
	if len(ops1) != 1 {
		t.Fatalf("dev1: expected 1 operation, got %d", len(ops1))
	}
	if ops1[0].Peer.PublicKey != peer1 {
		t.Errorf("dev1: expected peer %s, got %s", peer1, ops1[0].Peer.PublicKey)
	}
	if len(ops2) != 1 {
		t.Fatalf("dev2: expected 1 operation, got %d", len(ops2))
	}
	if ops2[0].Peer.PublicKey != peer2 {
		t.Errorf("dev2: expected peer %s, got %s", peer2, ops2[0].Peer.PublicKey)
	}

	if err := d1.Down(); err != nil {
		t.Fatalf("d1.Down: %v", err)
	}
	if err := d2.SetPeers(
		peer(peer2, []string{"10.0.1.1/32"}, "", wireguard.EndpointDynamic, 0),
		peer(mustGenerateKey(t), []string{"10.0.1.2/32"}, "", wireguard.EndpointDynamic, 0),
	); err != nil {
		t.Fatalf("d2.ApplyPeers after d1 down: %v", err)
	}

	ops2 = backend.AppliedOpsFor("dev2")
	if len(ops2) != 2 {
		t.Fatalf("dev2: expected 2 operations after second apply, got %d", len(ops2))
	}
	if got := backend.ApplyCountFor("dev1"); got != 1 {
		t.Errorf("dev1 apply count = %d, want 1", got)
	}
}

func TestDevice_ConcurrentReconcile(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	d := newStartedTestDevice(t, "test", backend)

	k1 := mustGenerateKey(t)
	k2 := mustGenerateKey(t)

	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		<-start
		for range 50 {
			_ = d.SetPeers(peer(k1, []string{"10.0.0.1/32"}, "", wireguard.EndpointDynamic, 0))
		}
	}()

	go func() {
		defer wg.Done()
		<-start
		for range 50 {
			_ = d.SetPeers(peer(k2, []string{"10.0.1.1/32"}, "", wireguard.EndpointDynamic, 0))
		}
	}()

	close(start)
	wg.Wait()

	st := d.ReconcileStatus()
	if st.Degraded() {
		t.Error("expected no degradation after concurrent reconciles")
	}
	if st.Error != nil {
		t.Errorf("expected no error, got %v", st.Error)
	}
}
