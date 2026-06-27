package wireguard

import (
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// testBackend implements Backend for unit testing the wgDevice.
type testBackend struct {
	mu              sync.Mutex
	statusCalls     int
	statusResponses []*DeviceStatus
	statusErrors    []error
	operations      []PeerOperation
	applyErr        error
	upErr           error
	downErr         error
	deleteErr       error

	upCalls     int
	downCalls   int
	deleteCalls int
}

func (b *testBackend) Up(cfg DeviceConfig) error {
	b.mu.Lock()
	b.upCalls++
	b.mu.Unlock()
	if b.upErr != nil {
		return b.upErr
	}
	return nil
}

func (b *testBackend) Down(name string) error {
	b.mu.Lock()
	b.downCalls++
	b.mu.Unlock()
	if b.downErr != nil {
		return b.downErr
	}
	return nil
}

func (b *testBackend) Delete(name string) error {
	b.mu.Lock()
	b.deleteCalls++
	b.mu.Unlock()
	if b.deleteErr != nil {
		return b.deleteErr
	}
	return nil
}

func (b *testBackend) Status(name string) (*DeviceStatus, error) {
	b.mu.Lock()
	idx := b.statusCalls
	b.statusCalls++
	if idx < len(b.statusErrors) && b.statusErrors[idx] != nil {
		err := b.statusErrors[idx]
		b.mu.Unlock()
		return nil, err
	}
	var resp *DeviceStatus
	if idx < len(b.statusResponses) {
		resp = b.statusResponses[idx]
	}
	b.mu.Unlock()
	if resp == nil {
		return &DeviceStatus{Name: name}, nil
	}
	return resp, nil
}

func (b *testBackend) ApplyPeerOperations(name string, ops []PeerOperation) error {
	b.mu.Lock()
	b.operations = append(b.operations, ops...)
	b.mu.Unlock()
	if b.applyErr != nil {
		return b.applyErr
	}
	return nil
}

func (b *testBackend) lastOperations() []PeerOperation {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.operations
}

func (b *testBackend) reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.statusCalls = 0
	b.statusResponses = nil
	b.statusErrors = nil
	b.operations = nil
	b.applyErr = nil
	b.upErr = nil
	b.downErr = nil
	b.deleteErr = nil
	b.upCalls = 0
	b.downCalls = 0
	b.deleteCalls = 0
}

func TestBuildDesiredPeers_Valid(t *testing.T) {
	k := mustGenerateKey(t)
	peers := []WGPeer{
		{
			PublicKey:           k.String(),
			AllowedIPs:          []string{"10.0.0.1/32"},
			Endpoint:            "1.2.3.4:51820",
			PersistentKeepalive: 25,
			EndpointPolicy:      EndpointFixed,
		},
	}

	desired, err := buildDesiredPeers(peers)
	if err != nil {
		t.Fatalf("buildDesiredPeers: %v", err)
	}

	dp, ok := desired[k]
	if !ok {
		t.Fatal("expected peer with matching public key")
	}

	if dp.PublicKey != k {
		t.Errorf("public key mismatch")
	}
	if dp.PersistentKeepalive != 25*time.Second {
		t.Errorf("keepalive = %v, want 25s", dp.PersistentKeepalive)
	}
	if dp.EndpointPolicy != EndpointFixed {
		t.Errorf("policy = %v, want EndpointFixed", dp.EndpointPolicy)
	}
	if dp.Endpoint == nil || dp.Endpoint.String() != "1.2.3.4:51820" {
		t.Errorf("endpoint = %v, want 1.2.3.4:51820", dp.Endpoint)
	}
	if len(dp.AllowedIPs) != 1 || dp.AllowedIPs[0].String() != "10.0.0.1/32" {
		t.Errorf("allowed IPs = %v, want [10.0.0.1/32]", dp.AllowedIPs)
	}
}

func TestBuildDesiredPeers_InvalidPublicKey(t *testing.T) {
	peers := []WGPeer{
		{PublicKey: "not-a-valid-key"},
	}
	_, err := buildDesiredPeers(peers)
	if err == nil {
		t.Error("expected error for invalid public key")
	}
}

func TestBuildDesiredPeers_InvalidAllowedIP(t *testing.T) {
	k := mustGenerateKey(t)
	peers := []WGPeer{
		{PublicKey: k.String(), AllowedIPs: []string{"not-an-ip"}},
	}
	_, err := buildDesiredPeers(peers)
	if err == nil {
		t.Error("expected error for invalid AllowedIP")
	}
}

func TestBuildDesiredPeers_InvalidEndpoint(t *testing.T) {
	k := mustGenerateKey(t)
	peers := []WGPeer{
		{PublicKey: k.String(), Endpoint: "not-a-valid-endpoint"},
	}
	_, err := buildDesiredPeers(peers)
	if err == nil {
		t.Error("expected error for invalid endpoint")
	}
}

func TestBuildDesiredPeers_EmptyEndpoint(t *testing.T) {
	k := mustGenerateKey(t)
	peers := []WGPeer{
		{PublicKey: k.String(), Endpoint: ""},
	}
	desired, err := buildDesiredPeers(peers)
	if err != nil {
		t.Fatalf("buildDesiredPeers: %v", err)
	}
	dp := desired[k]
	if dp.Endpoint != nil {
		t.Error("expected nil endpoint for empty string")
	}
}

func TestBuildDesiredPeers_MultiplePeers(t *testing.T) {
	k1, k2 := mustGenerateKey(t), mustGenerateKey(t)
	peers := []WGPeer{
		{PublicKey: k1.String(), AllowedIPs: []string{"10.0.0.1/32"}},
		{PublicKey: k2.String(), AllowedIPs: []string{"10.0.0.2/32"}},
	}
	desired, err := buildDesiredPeers(peers)
	if err != nil {
		t.Fatalf("buildDesiredPeers: %v", err)
	}
	if len(desired) != 2 {
		t.Errorf("expected 2 peers, got %d", len(desired))
	}
}

func TestDevice_ApplyPeers_AddsNewPeers(t *testing.T) {
	backend := &testBackend{}
	d := newTestDevice(t, backend)

	k := mustGenerateKey(t)
	peers := []WGPeer{
		{PublicKey: k.String(), AllowedIPs: []string{"10.0.0.1/32"}},
	}

	if err := d.ApplyPeers(peers); err != nil {
		t.Fatalf("ApplyPeers: %v", err)
	}

	ops := backend.lastOperations()
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].Type != PeerAdd {
		t.Errorf("expected add, got %s", ops[0].Type)
	}
}

func TestDevice_ApplyPeers_NoChangeOnMatch(t *testing.T) {
	k := mustGenerateKey(t)
	backend := &testBackend{
		statusResponses: []*DeviceStatus{
			{
				Peers: []ObservedPeer{
					{PublicKey: k, AllowedIPs: []net.IPNet{mustParseCIDR(t, "10.0.0.1/32")}},
				},
			},
		},
	}
	d := newTestDevice(t, backend)

	peers := []WGPeer{
		{PublicKey: k.String(), AllowedIPs: []string{"10.0.0.1/32"}},
	}

	if err := d.ApplyPeers(peers); err != nil {
		t.Fatalf("ApplyPeers: %v", err)
	}

	ops := backend.lastOperations()
	if len(ops) != 0 {
		t.Errorf("expected 0 operations, got %d", len(ops))
	}
}

func TestDevice_ApplyPeers_RemovesStalePeer(t *testing.T) {
	kLive := mustGenerateKey(t)
	backend := &testBackend{
		statusResponses: []*DeviceStatus{
			{
				Peers: []ObservedPeer{
					{PublicKey: kLive, AllowedIPs: []net.IPNet{mustParseCIDR(t, "10.0.0.1/32")}},
				},
			},
		},
	}
	d := newTestDevice(t, backend)

	// Desired peers is empty — live peer should be removed
	if err := d.ApplyPeers(nil); err != nil {
		t.Fatalf("ApplyPeers: %v", err)
	}

	ops := backend.lastOperations()
	if len(ops) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(ops))
	}
	if ops[0].Type != PeerRemove {
		t.Errorf("expected remove, got %s", ops[0].Type)
	}
}

func TestDevice_ApplyPeers_ObserveError(t *testing.T) {
	backend := &testBackend{
		statusErrors: []error{errors.New("status failed")},
	}
	d := newTestDevice(t, backend)

	err := d.ApplyPeers(nil)
	if err == nil {
		t.Fatal("expected error from observe failure")
	}

	st := d.ReconcileStatus()
	if st.Error == nil {
		t.Fatal("expected reconcile error")
	}
	if st.Error.Stage != StageObserve {
		t.Errorf("error stage = %v, want StageObserve", st.Error.Stage)
	}
}

func TestDevice_ApplyPeers_ApplyError(t *testing.T) {
	backend := &testBackend{
		applyErr: errors.New("apply failed"),
	}
	d := newTestDevice(t, backend)

	k := mustGenerateKey(t)
	peers := []WGPeer{
		{PublicKey: k.String(), AllowedIPs: []string{"10.0.0.1/32"}},
	}

	err := d.ApplyPeers(peers)
	if err == nil {
		t.Fatal("expected error from apply failure")
	}

	st := d.ReconcileStatus()
	if st.Error == nil {
		t.Fatal("expected reconcile error")
	}
	if st.Error.Stage != StageApply {
		t.Errorf("error stage = %v, want StageApply", st.Error.Stage)
	}
	if st.Degraded() != true {
		t.Error("expected Degraded=true after apply failure")
	}
}

func TestDevice_ReconcileStatus_RecordsSuccess(t *testing.T) {
	backend := &testBackend{}
	d := newTestDevice(t, backend)

	k := mustGenerateKey(t)
	peers := []WGPeer{
		{PublicKey: k.String(), AllowedIPs: []string{"10.0.0.1/32"}},
	}

	if err := d.ApplyPeers(peers); err != nil {
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
		t.Errorf("Observed = %d, want 0 (no peers on device)", st.Observed)
	}
}

func TestDevice_Up(t *testing.T) {
	backend := &testBackend{}
	d := newTestDevice(t, backend)

	if err := d.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}

	if backend.upCalls != 1 {
		t.Errorf("upCalls = %d, want 1", backend.upCalls)
	}
}

func TestDevice_Up_Error(t *testing.T) {
	backend := &testBackend{upErr: errors.New("up failed")}
	d := newTestDevice(t, backend)

	err := d.Up()
	if err == nil {
		t.Fatal("expected error from Up")
	}
}

func TestDevice_Down(t *testing.T) {
	backend := &testBackend{}
	d := newTestDevice(t, backend)

	if err := d.Down(); err != nil {
		t.Fatalf("Down: %v", err)
	}

	if backend.downCalls != 1 {
		t.Errorf("downCalls = %d, want 1", backend.downCalls)
	}
}

func TestDevice_DeviceName_Default(t *testing.T) {
	backend := &testBackend{}
	d := newTestDevice(t, backend)

	if d.DeviceName() != "test" {
		t.Errorf("DeviceName = %q, want test", d.DeviceName())
	}
}

func TestDevice_DeviceName_AfterSetRealName(t *testing.T) {
	backend := &testBackend{}
	d := newTestDevice(t, backend)

	d.setRealName("utun4")
	if d.DeviceName() != "utun4" {
		t.Errorf("DeviceName = %q, want utun4", d.DeviceName())
	}
}

func TestDevice_SetLogger(t *testing.T) {
	backend := &testBackend{}
	d := newTestDevice(t, backend)

	var logged string
	d.SetLogger(func(format string, args ...any) {
		logged = format
	})

	// trigger a log via reconcile
	d.ApplyPeers(nil)

	if logged == "" {
		t.Error("expected logger to be called during reconcile")
	}
}

func TestDevice_WaitForHandshake_Immediate(t *testing.T) {
	k := mustGenerateKey(t)
	backend := &testBackend{
		statusResponses: []*DeviceStatus{
			{
				Peers: []ObservedPeer{
					{PublicKey: k, LastHandshake: time.Now()},
				},
			},
		},
	}
	d := newTestDevice(t, backend)

	err := d.WaitForHandshake(k.String(), time.Second, nil)
	if err != nil {
		t.Fatalf("WaitForHandshake: %v", err)
	}
}

func TestDevice_WaitForHandshake_Timeout(t *testing.T) {
	backend := &testBackend{
		statusResponses: []*DeviceStatus{
			{Peers: nil},
		},
	}
	d := newTestDevice(t, backend)

	err := d.WaitForHandshake(mustGenerateKey(t).String(), 50*time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestDevice_WaitForHandshake_BackendError(t *testing.T) {
	backend := &testBackend{
		statusErrors: []error{errors.New("status failed")},
	}
	d := newTestDevice(t, backend)

	err := d.WaitForHandshake(mustGenerateKey(t).String(), 50*time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDevice_WaitForHandshake_OnStatusCallback(t *testing.T) {
	k := mustGenerateKey(t)
	backend := &testBackend{
		statusResponses: []*DeviceStatus{
			{
				Peers: []ObservedPeer{
					{PublicKey: k, LastHandshake: time.Now()},
				},
			},
		},
	}
	d := newTestDevice(t, backend)

	var called bool
	err := d.WaitForHandshake(k.String(), time.Second, func(ps PeerStatus) {
		called = true
		if ps.PublicKey != k.String() {
			t.Errorf("callback key = %q, want %q", ps.PublicKey, k.String())
		}
	})
	if err != nil {
		t.Fatalf("WaitForHandshake: %v", err)
	}
	if !called {
		t.Error("expected onStatus callback to be called")
	}
}

func newTestDevice(t *testing.T, backend Backend) *wgDevice {
	t.Helper()
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return newDevice("test", key, net.IPNet{}, 0, 0, false, backend)
}
