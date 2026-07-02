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

func peerConfig(
	key wgtypes.Key,
	allowedIPs []string,
	endpoint string,
	policy wireguard.EndpointPolicy,
	keepalive time.Duration,
) wireguard.PeerConfig {
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

	return wireguard.PeerConfig{
		PublicKey:           key,
		AllowedIPs:          ips,
		Endpoint:            ep,
		EndpointPolicy:      policy,
		PersistentKeepalive: keepalive,
	}
}

func peerStatus(
	key wgtypes.Key,
	allowedIPs []string,
	endpoint string,
	keepalive time.Duration,
) wireguard.PeerStatus {
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

	return wireguard.PeerStatus{
		PublicKey:           key,
		AllowedIPs:          ips,
		Endpoint:            ep,
		PersistentKeepalive: keepalive,
	}
}

func createTestDevice(t *testing.T, name string, backend *wireguardtest.MockBackend) *wireguard.Device {
	t.Helper()
	mgr := wireguard.NewManagerWithBackend(backend)
	cfg := wireguard.DeviceConfig{
		Name:       name,
		PrivateKey: mustGeneratePrivateKey(t),
		Address:    mustParseCIDR(t, "10.0.0.1/32"),
	}
	dev, err := mgr.CreateDevice(cfg)
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	return dev
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

func TestCreateDevice_ValidatesNameLength(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)
	key := mustGeneratePrivateKey(t)

	_, err := mgr.CreateDevice(wireguard.DeviceConfig{
		Name:       "123456789012345",
		PrivateKey: key,
		Address:    mustParseCIDR(t, "10.0.0.1/32"),
	})
	if err != nil {
		t.Errorf("15-byte name should succeed: %v", err)
	}

	_, err = mgr.CreateDevice(wireguard.DeviceConfig{
		Name:       "1234567890123456",
		PrivateKey: key,
		Address:    mustParseCIDR(t, "10.0.0.1/32"),
	})
	if err == nil {
		t.Error("16-byte name should fail")
	}
}

func TestCreateDevice_InvalidPrivateKey(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)

	_, err := mgr.CreateDevice(wireguard.DeviceConfig{
		Name:       "test",
		PrivateKey: "not-a-key",
		Address:    mustParseCIDR(t, "10.0.0.1/32"),
	})
	if err == nil {
		t.Error("expected error for invalid private key")
	}
}

func TestCreateDevice_Valid(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	mgr := wireguard.NewManagerWithBackend(backend)

	dev, err := mgr.CreateDevice(wireguard.DeviceConfig{
		Name:       "test",
		PrivateKey: mustGeneratePrivateKey(t),
		Address:    mustParseCIDR(t, "10.0.0.1/32"),
		ListenPort: 51820,
	})
	if err != nil {
		t.Fatalf("CreateDevice: %v", err)
	}
	if dev == nil {
		t.Fatal("expected non-nil Device")
	}
	if dev.Name() != "test" {
		t.Errorf("Name = %q, want test", dev.Name())
	}
}

func TestCreateDevice_BackendError(t *testing.T) {
	backend := wireguardtest.NewMockBackend()
	backend.CreateErr = errors.New("create failed")
	mgr := wireguard.NewManagerWithBackend(backend)

	_, err := mgr.CreateDevice(wireguard.DeviceConfig{
		Name:       "test",
		PrivateKey: mustGeneratePrivateKey(t),
		Address:    mustParseCIDR(t, "10.0.0.1/32"),
	})
	if err == nil {
		t.Error("expected backend create error")
	}
}

func TestNewPeerConfig(t *testing.T) {
	key := mustGenerateKey(t)
	p, err := wireguard.NewPeerConfig(
		key.String(),
		[]string{"10.0.0.5/32", "10.0.1.0/24"},
		"1.2.3.4:51820",
		25,
		wireguard.EndpointFixed,
	)
	if err != nil {
		t.Fatalf("NewPeerConfig: %v", err)
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

func TestNewPeerConfig_InvalidInputs(t *testing.T) {
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
			_, err := wireguard.NewPeerConfig(
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
