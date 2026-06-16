//go:build integration

package wireguard_test

import (
	"net"
	"os"
	"path"
	"runtime"
	"strings"
	"testing"
	"time"

	"git.sr.ht/~jakintosh/cord/internal/wireguard"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// These tests create real WireGuard interfaces and require root:
//
//	sudo make test-integration
//
// They run on both macOS (userspace/utun) and Linux (kernel WireGuard,
// falling back to userspace when the kernel module is unavailable).

func requireRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("integration tests require root; run with sudo")
	}
}

// upTestInterface creates and brings up an interface, registering
// cleanup. On Linux it prefers the kernel backend but falls back to
// userspace when kernel WireGuard is unavailable.
func upTestInterface(
	t *testing.T,
	name string,
	address string,
	listenPort int,
	noRoutes bool,
) *wireguard.Interface {
	t.Helper()

	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	ip, network, err := net.ParseCIDR(address)
	if err != nil {
		t.Fatalf("failed to parse address %s: %v", address, err)
	}
	addr := net.IPNet{IP: ip, Mask: network.Mask}

	iface, err := wireguard.NewInterface(name, key, addr, listenPort, wireguard.BackendAuto)
	if err != nil {
		t.Fatalf("failed to create interface: %v", err)
	}
	iface.NoRoutes = noRoutes

	if err := iface.Up(""); err != nil {
		if runtime.GOOS == "linux" {
			// kernel WireGuard may be unavailable; retry in userspace
			iface, err = wireguard.NewInterface(name, key, addr, listenPort, wireguard.BackendUserspace)
			if err != nil {
				t.Fatalf("failed to create userspace interface: %v", err)
			}
			iface.NoRoutes = noRoutes
			if err := iface.Up(""); err != nil {
				t.Fatalf("failed to bring up interface (userspace fallback): %v", err)
			}
		} else {
			t.Fatalf("failed to bring up interface: %v", err)
		}
	}

	t.Cleanup(func() { _ = iface.Down(true) })
	return iface
}

// TestIntegration_InterfaceLifecycle proves an interface can be
// created, observed by the OS, synced, and destroyed.
func TestIntegration_InterfaceLifecycle(t *testing.T) {
	requireRoot(t)

	iface := upTestInterface(t, "cord-it0", "10.99.10.1/24", 51998, false)

	// the OS should know about the device
	deviceName := iface.DeviceName()
	osIface, err := net.InterfaceByName(deviceName)
	if err != nil {
		t.Fatalf("OS does not report device %s: %v", deviceName, err)
	}

	// the device should carry our address
	addrs, err := osIface.Addrs()
	if err != nil {
		t.Fatalf("failed to list device addresses: %v", err)
	}
	found := false
	for _, addr := range addrs {
		if strings.HasPrefix(addr.String(), "10.99.10.1") {
			found = true
		}
	}
	if !found {
		t.Errorf("device %s does not carry 10.99.10.1 (has %v)", deviceName, addrs)
	}

	// the live device should report our listen port and no peers
	status, err := iface.Status()
	if err != nil {
		t.Fatalf("failed to query device status: %v", err)
	}
	if status.ListenPort != 51998 {
		t.Errorf("device listen port = %d, want 51998", status.ListenPort)
	}
	if len(status.Peers) != 0 {
		t.Errorf("expected no peers on fresh device, got %d", len(status.Peers))
	}

	// adding a peer and syncing should be visible in device state
	peerKey, _ := wgtypes.GeneratePrivateKey()
	_, allowed, _ := net.ParseCIDR("10.99.10.2/32")
	iface.AddPeer(wireguard.Peer{
		PublicKey:  peerKey.PublicKey(),
		AllowedIPs: []net.IPNet{*allowed},
	})
	if err := iface.Reconcile(); err != nil {
		t.Fatalf("failed to sync peer: %v", err)
	}
	status, err = iface.Status()
	if err != nil {
		t.Fatalf("failed to query device status after sync: %v", err)
	}
	if len(status.Peers) != 1 || status.Peers[0].PublicKey != peerKey.PublicKey() {
		t.Fatalf("expected synced peer on device, got %v", status.Peers)
	}

	// removing the peer should be visible too
	iface.SetPeers(nil)
	if err := iface.Reconcile(); err != nil {
		t.Fatalf("failed to sync peer removal: %v", err)
	}
	status, _ = iface.Status()
	if len(status.Peers) != 0 {
		t.Errorf("expected no peers after removal, got %d", len(status.Peers))
	}

	// tearing down should remove the device from the OS
	if err := iface.Down(true); err != nil {
		t.Fatalf("failed to bring interface down: %v", err)
	}
	if _, err := net.InterfaceByName(deviceName); err == nil {
		t.Errorf("device %s still exists after Down", deviceName)
	}
}

// TestIntegration_WritesConfigFile proves Up writes a wg-quick style
// config file when asked.
func TestIntegration_WritesConfigFile(t *testing.T) {
	requireRoot(t)

	key, _ := wgtypes.GeneratePrivateKey()
	_, network, _ := net.ParseCIDR("10.99.11.0/24")
	addr := net.IPNet{IP: net.IPv4(10, 99, 11, 1), Mask: network.Mask}

	iface, err := wireguard.NewInterface("cord-it1", key, addr, 0, wireguard.BackendAuto)
	if err != nil {
		t.Fatalf("failed to create interface: %v", err)
	}

	confPath := path.Join(t.TempDir(), "cord-it1.conf")
	if err := iface.Up(confPath); err != nil {
		t.Fatalf("failed to bring up interface: %v", err)
	}
	defer iface.Down(true)

	payload, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("config file was not written: %v", err)
	}
	if !strings.Contains(string(payload), "PrivateKey = "+key.String()) {
		t.Errorf("config file missing private key")
	}
}

// TestIntegration_TwoInterfacesHandshake proves that two interfaces
// managed by this package establish a real WireGuard session over
// loopback: keepalives from the initiator must produce a completed
// handshake on both devices.
func TestIntegration_TwoInterfacesHandshake(t *testing.T) {
	requireRoot(t)

	listenerKey, _ := wgtypes.GeneratePrivateKey()
	dialerKey, _ := wgtypes.GeneratePrivateKey()

	// listener: a "server" with a known port, no endpoint for its peer
	_, network, _ := net.ParseCIDR("10.99.12.0/24")
	listener, err := wireguard.NewInterface(
		"cord-it2",
		listenerKey,
		net.IPNet{IP: net.IPv4(10, 99, 12, 1), Mask: network.Mask},
		52000,
		wireguard.BackendAuto,
	)
	if err != nil {
		t.Fatalf("failed to create listener interface: %v", err)
	}
	_, dialerAllowed, _ := net.ParseCIDR("10.99.12.2/32")
	listener.AddPeer(wireguard.Peer{
		PublicKey:  dialerKey.PublicKey(),
		AllowedIPs: []net.IPNet{*dialerAllowed},
	})
	if err := listener.Up(""); err != nil {
		t.Fatalf("failed to bring up listener: %v", err)
	}
	defer listener.Down(true)

	// dialer: connects to the listener over loopback with keepalives
	dialer, err := wireguard.NewInterface(
		"cord-it3",
		dialerKey,
		net.IPNet{IP: net.IPv4(10, 99, 12, 2), Mask: network.Mask},
		0,
		wireguard.BackendAuto,
	)
	if err != nil {
		t.Fatalf("failed to create dialer interface: %v", err)
	}
	dialer.NoRoutes = true // listener already routes 10.99.12.0/24
	endpoint, _ := net.ResolveUDPAddr("udp", "127.0.0.1:52000")
	_, listenerAllowed, _ := net.ParseCIDR("10.99.12.1/32")
	dialer.AddPeer(wireguard.Peer{
		PublicKey:           listenerKey.PublicKey(),
		AllowedIPs:          []net.IPNet{*listenerAllowed},
		Endpoint:            endpoint,
		PersistentKeepalive: time.Second,
	})
	if err := dialer.Up(""); err != nil {
		t.Fatalf("failed to bring up dialer: %v", err)
	}
	defer dialer.Down(true)

	// wait for both devices to report a completed handshake
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		listenerStatus, err1 := listener.Status()
		dialerStatus, err2 := dialer.Status()
		if err1 == nil && err2 == nil &&
			handshakeComplete(listenerStatus) && handshakeComplete(dialerStatus) {
			before := listenerStatus.Peers[0].LastHandshake
			if err := listener.Reconcile(); err != nil {
				t.Fatalf("no-op listener reconcile failed: %v", err)
			}
			after, err := listener.Status()
			if err != nil {
				t.Fatalf("listener status after reconcile: %v", err)
			}
			if len(after.Peers) != 1 || after.Peers[0].LastHandshake.Before(before) {
				t.Fatalf(
					"no-op reconcile disturbed handshake: before=%s after=%v",
					before, after.Peers,
				)
			}
			return // success
		}
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatal("no WireGuard handshake completed within 20s")
}

func handshakeComplete(status *wireguard.DeviceStatus) bool {
	for _, peer := range status.Peers {
		if !peer.LastHandshake.IsZero() && peer.LastHandshake.Unix() > 0 {
			return true
		}
	}
	return false
}
