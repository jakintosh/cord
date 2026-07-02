package wireguard_test

import (
	"errors"
	"net"
	"testing"
	"time"

	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
	"git.studiopollinator.com/pollinator/cord/internal/wireguard/wireguardtest"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func mustGenerateKey(t *testing.T) wgtypes.Key {
	t.Helper()
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func mustGeneratePrivateKey(t *testing.T) string {
	t.Helper()
	return mustGenerateKey(t).String()
}

func mustParseCIDR(t *testing.T, s string) net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("parse CIDR %q: %v", s, err)
	}
	return *n
}

func mustResolveUDP(t *testing.T, s string) *net.UDPAddr {
	t.Helper()
	addr, err := net.ResolveUDPAddr("udp", s)
	if err != nil {
		t.Fatalf("resolve UDP %q: %v", s, err)
	}
	return addr
}

func peer(
	key wgtypes.Key,
	allowedIPs []string,
	endpoint string,
	policy wireguard.EndpointPolicy,
	keepalive time.Duration,
) wireguard.Peer {
	var ep *net.UDPAddr
	if endpoint != "" {
		addr, err := net.ResolveUDPAddr("udp", endpoint)
		if err != nil {
			panic(err)
		}
		ep = addr
	}

	ips := make([]net.IPNet, 0, len(allowedIPs))
	for _, cidr := range allowedIPs {
		_, n, err := net.ParseCIDR(cidr)
		if err != nil {
			panic(err)
		}
		ips = append(ips, *n)
	}

	return wireguard.Peer{
		PublicKey:           key,
		AllowedIPs:          ips,
		Endpoint:            ep,
		EndpointPolicy:      policy,
		PersistentKeepalive: keepalive,
	}
}

func newStartedTestDevice(
	t *testing.T,
	name string,
	backend wireguard.Backend,
) *wireguard.Device {
	t.Helper()

	mgr := wireguard.NewManagerWithBackend(backend)
	d := newStoppedDeviceFromManager(t, mgr, name, 0)
	if err := d.Up(); err != nil {
		t.Fatalf("Up: %v", err)
	}
	return d
}

func newStoppedTestDevice(
	t *testing.T,
	name string,
	backend wireguard.Backend,
) *wireguard.Device {
	t.Helper()
	mgr := wireguard.NewManagerWithBackend(backend)
	return newStoppedDeviceFromManager(t, mgr, name, 0)
}

func newStoppedDeviceFromManager(
	t *testing.T,
	mgr *wireguard.Manager,
	name string,
	port uint16,
) *wireguard.Device {
	t.Helper()
	d, err := mgr.NewDevice(name, mustGeneratePrivateKey(t), mustParseCIDR(t, "10.0.0.1/32"), port)
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	return d
}

func TestParseBackendType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    wireguard.BackendType
		wantErr bool
	}{
		{name: "empty", input: "", want: wireguard.BackendAuto},
		{name: "auto", input: "auto", want: wireguard.BackendAuto},
		{name: "trim and case", input: " Kernel ", want: wireguard.BackendKernel},
		{name: "userspace", input: "userspace", want: wireguard.BackendUserspace},
		{name: "unknown", input: "bogus", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := wireguard.ParseBackendType(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseBackendType: %v", err)
			}
			if got != tt.want {
				t.Errorf("ParseBackendType(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestValidateDeviceName(t *testing.T) {
	if err := wireguard.ValidateDeviceName("123456789012345"); err != nil {
		t.Errorf("15-byte name should succeed: %v", err)
	}
	if err := wireguard.ValidateDeviceName("1234567890123456"); err == nil {
		t.Error("16-byte name should fail")
	}
}

func TestNewManager_ReturnsManager(t *testing.T) {
	mgr, err := wireguard.NewManager(wireguard.Options{Backend: wireguard.BackendUserspace})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if mgr == nil {
		t.Fatal("expected non-nil Manager")
	}
}

func TestNewDevice_ValidatesNameLength(t *testing.T) {
	mgr := wireguard.NewManagerWithBackend(wireguardtest.NewMockBackend())
	key := mustGeneratePrivateKey(t)

	_, err := mgr.NewDevice("123456789012345", key, mustParseCIDR(t, "10.0.0.1/32"), 0)
	if err != nil {
		t.Errorf("15-byte name should succeed: %v", err)
	}

	_, err = mgr.NewDevice("1234567890123456", key, mustParseCIDR(t, "10.0.0.1/32"), 0)
	if err == nil {
		t.Error("16-byte name should fail")
	}
}

func TestNewDevice_InvalidPrivateKey(t *testing.T) {
	mgr := wireguard.NewManagerWithBackend(wireguardtest.NewMockBackend())

	_, err := mgr.NewDevice("test", "not-a-key", mustParseCIDR(t, "10.0.0.1/32"), 0)
	if err == nil {
		t.Error("expected error for invalid private key")
	}
}

func TestNewDevice_Valid(t *testing.T) {
	mgr := wireguard.NewManagerWithBackend(wireguardtest.NewMockBackend())

	dev, err := mgr.NewDevice("test", mustGeneratePrivateKey(t), mustParseCIDR(t, "10.0.0.1/32"), 51820)
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	if dev == nil {
		t.Fatal("expected non-nil Device")
	}
	if dev.Name() != "test" {
		t.Errorf("Name = %q, want test", dev.Name())
	}
}

func TestRemoveDevice_Existing(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)

	_, err := mgr.NewDevice("test", mustGeneratePrivateKey(t), mustParseCIDR(t, "10.0.0.1/32"), 0)
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}

	if err := mgr.RemoveDevice("test"); err != nil {
		t.Errorf("RemoveDevice: %v", err)
	}
	if got := backend.DeleteCount("test"); got != 1 {
		t.Errorf("delete count = %d, want 1", got)
	}
}

func TestRemoveDevice_BackendError(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	backend.DeleteErr = errors.New("delete failed")
	mgr := wireguard.NewManagerWithBackend(backend)

	_, err := mgr.NewDevice("test", mustGeneratePrivateKey(t), mustParseCIDR(t, "10.0.0.1/32"), 0)
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}

	if err := mgr.RemoveDevice("test"); err == nil {
		t.Error("expected backend delete error")
	}
}

func TestRemoveDevice_NotFound(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)

	err := mgr.RemoveDevice("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent device")
	}
	if got := backend.DeleteCount("nonexistent"); got != 0 {
		t.Errorf("delete count = %d, want 0", got)
	}
}

func TestNewPeer(t *testing.T) {
	key := mustGenerateKey(t)
	p, err := wireguard.NewPeer(
		key.String(),
		[]string{"10.0.0.5/32", "10.0.1.0/24"},
		"1.2.3.4:51820",
		25,
		wireguard.EndpointFixed,
	)
	if err != nil {
		t.Fatalf("NewPeer: %v", err)
	}
	if p.PublicKey != key {
		t.Error("public key mismatch")
	}
	if len(p.AllowedIPs) != 2 {
		t.Fatalf("allowed IPs = %d, want 2", len(p.AllowedIPs))
	}
	if p.Endpoint == nil || p.Endpoint.String() != "1.2.3.4:51820" {
		t.Errorf("endpoint = %v, want 1.2.3.4:51820", p.Endpoint)
	}
	if p.EndpointPolicy != wireguard.EndpointFixed {
		t.Errorf("endpoint policy = %v, want fixed", p.EndpointPolicy)
	}
}

func TestNewPeer_InvalidInputs(t *testing.T) {
	key := mustGenerateKey(t).String()

	tests := []struct {
		name       string
		publicKey  string
		allowedIPs []string
		endpoint   string
	}{
		{name: "public key", publicKey: "not-a-key"},
		{name: "allowed IP", publicKey: key, allowedIPs: []string{"not-a-cidr"}},
		{name: "endpoint", publicKey: key, endpoint: "not an endpoint"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := wireguard.NewPeer(
				tt.publicKey,
				tt.allowedIPs,
				tt.endpoint,
				0,
				wireguard.EndpointDynamic,
			)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
