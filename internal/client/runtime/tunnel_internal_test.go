package runtime

import (
	"context"
	"errors"
	"testing"

	"git.studiopollinator.com/pollinator/cord/internal/client/service"
	"git.studiopollinator.com/pollinator/cord/internal/resource"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
)

func TestTunnelConstructionDoesNotStartDevice(t *testing.T) {
	tunnel, backend := newTestTunnel(t)
	if tunnel.apiAddr != "10.42.0.1:8080" {
		t.Fatalf("API address = %q", tunnel.apiAddr)
	}
	if status := tunnel.status(); status.State != resource.Stopped {
		t.Fatalf("initial status = %+v", status)
	}
	if len(backend.CreateCalls) != 0 {
		t.Fatal("construction or status created an interface")
	}

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := tunnel.reconcile(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled reconcile = %v", err)
	}
	if len(backend.CreateCalls) != 0 {
		t.Fatal("canceled reconciliation created an interface")
	}
}

func TestTunnelReconcilePreservesNetworkPeers(t *testing.T) {
	tunnel, backend := newTestTunnel(t)
	if err := tunnel.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	peers, err := tunnel.device.GetPeerStatuses()
	if err != nil {
		t.Fatal(err)
	}
	if len(peers) != 1 || peers[0].PublicKey.String() != tunnel.serverPeer.PublicKey {
		t.Fatalf("initial peers = %+v", peers)
	}

	key, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := wireguard.PublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if err := tunnel.device.SetPeers(tunnel.serverPeer, wireguard.PeerConfig{
		PublicKey:  publicKey,
		AllowedIPs: []string{"10.42.0.3/32"},
	}); err != nil {
		t.Fatal(err)
	}

	for _, failed := range []bool{false, true} {
		if failed {
			backend.Device("test").CheckErr = errors.New("interface lost")
			if status := tunnel.status(); status.State != resource.Failed {
				t.Fatalf("failed status = %+v", status)
			}
			if len(backend.CreateCalls) != 1 {
				t.Fatal("status repaired the interface")
			}
		}
		if err := tunnel.reconcile(t.Context()); err != nil {
			t.Fatal(err)
		}
		peers, err := tunnel.device.GetPeerStatuses()
		if err != nil {
			t.Fatal(err)
		}
		if len(peers) != 2 {
			t.Fatalf("peers after reconcile (failed=%v) = %+v", failed, peers)
		}
	}
}

func TestTunnelCloseRetainsDeviceForRetryAndRestart(t *testing.T) {
	tunnel, backend := newTestTunnel(t)
	if err := tunnel.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	wrapper := tunnel.device
	device := backend.Device("test")
	closeErr := errors.New("close failed")
	device.CloseErr = closeErr
	if err := tunnel.close(); !errors.Is(err, closeErr) {
		t.Fatalf("close = %v", err)
	}
	if status := tunnel.status(); status.State != resource.Failed {
		t.Fatalf("cleanup status = %+v", status)
	}

	device.CloseErr = nil
	if err := tunnel.close(); err != nil {
		t.Fatal(err)
	}
	if device.CloseCalls != 2 || tunnel.device != wrapper {
		t.Fatal("cleanup did not retry the retained device")
	}
	if err := tunnel.reconcile(t.Context()); err != nil {
		t.Fatal(err)
	}
	if tunnel.device != wrapper || len(backend.CreateCalls) != 2 {
		t.Fatal("restart did not reuse the configured wrapper")
	}
	if status := tunnel.status(); status.State != resource.Running {
		t.Fatalf("restarted status = %+v", status)
	}
}

func newTestTunnel(
	t *testing.T,
) (
	*Tunnel,
	*wireguardtest.MockBackend,
) {
	t.Helper()
	key, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	publicKey, err := wireguard.PublicKey(key)
	if err != nil {
		t.Fatal(err)
	}
	backend := wireguardtest.NewMockBackend()
	tunnel, err := newTunnel(tunnelOptions{
		name:       "test",
		privateKey: key,
		route:      "10.42.0.2/32",
		server: service.ServerInfo{
			PublicKey:   publicKey,
			Route:       "10.42.0.1/32",
			NetworkCidr: "10.42.0.0/24",
			Endpoint:    "127.0.0.1:51820",
			APIPort:     8080,
		},
	}, wireguard.NewManagerWithBackend(backend))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tunnel.close(); err != nil {
			t.Error(err)
		}
	})
	return tunnel, backend
}
