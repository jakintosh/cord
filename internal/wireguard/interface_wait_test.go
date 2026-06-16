package wireguard

import (
	"fmt"
	"testing"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type handshakeStatusBackend struct {
	publicKey     wgtypes.Key
	completeAfter int
	statusCalls   int
	statusErr     error
}

func (b *handshakeStatusBackend) Up(*Interface, string) error {
	return nil
}

func (b *handshakeStatusBackend) Down(*Interface, bool) error {
	return nil
}

func (b *handshakeStatusBackend) ApplyPeerOperations(*Interface, []PeerOperation) error {
	return nil
}

func (b *handshakeStatusBackend) Status(*Interface) (*DeviceStatus, error) {
	b.statusCalls++
	if b.statusErr != nil {
		return nil, b.statusErr
	}

	peer := PeerStatus{PublicKey: b.publicKey}
	if b.statusCalls >= b.completeAfter {
		peer.LastHandshake = time.Now()
	}
	return &DeviceStatus{Peers: []PeerStatus{peer}}, nil
}

func TestInterface_WaitForHandshakeCompletes(t *testing.T) {
	key := wgtypes.Key{1}
	backend := &handshakeStatusBackend{publicKey: key, completeAfter: 2}
	iface := &Interface{backend: backend}
	statusUpdates := 0

	if err := iface.WaitForHandshake(key, time.Second, func(PeerStatus) {
		statusUpdates++
	}); err != nil {
		t.Fatalf("wait for handshake failed: %v", err)
	}
	if backend.statusCalls < 2 {
		t.Fatalf("status calls = %d, want at least 2", backend.statusCalls)
	}
	if statusUpdates < 2 {
		t.Fatalf("status updates = %d, want at least 2", statusUpdates)
	}
}

func TestInterface_WaitForHandshakeTimesOut(t *testing.T) {
	key := wgtypes.Key{1}
	backend := &handshakeStatusBackend{publicKey: key, completeAfter: 100}
	iface := &Interface{backend: backend}

	err := iface.WaitForHandshake(key, time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected handshake timeout")
	}
}

func TestInterface_WaitForHandshakeReportsStatusError(t *testing.T) {
	key := wgtypes.Key{1}
	backend := &handshakeStatusBackend{statusErr: fmt.Errorf("status unavailable")}
	iface := &Interface{backend: backend}

	err := iface.WaitForHandshake(key, time.Millisecond, nil)
	if err == nil {
		t.Fatal("expected status error")
	}
}
