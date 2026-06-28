package wireguard

import (
	"net"
	"testing"
)

func cidr(t *testing.T, s string) net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("ParseCIDR(%q): %v", s, err)
	}
	return *n
}

func TestNew_ReturnsManager(t *testing.T) {
	wg, err := New(Options{Backend: BackendUserspace})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if wg == nil {
		t.Fatal("expected non-nil WG")
	}
}

func TestNewDevice_ValidatesNameLength(t *testing.T) {
	wg, err := New(Options{Backend: BackendUserspace})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	// Name exactly 15 bytes should succeed
	_, err = wg.NewDevice("123456789012345", key, cidr(t, "10.0.0.1/32"), 0)
	if err != nil {
		t.Errorf("15-byte name should succeed: %v", err)
	}

	// 16 bytes should fail
	_, err = wg.NewDevice("1234567890123456", key, cidr(t, "10.0.0.1/32"), 0)
	if err == nil {
		t.Error("16-byte name should fail")
	}
}

func TestNewDevice_InvalidPrivateKey(t *testing.T) {
	wg, err := New(Options{Backend: BackendUserspace})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	_, err = wg.NewDevice("test", "not-a-key", cidr(t, "10.0.0.1/32"), 0)
	if err == nil {
		t.Error("expected error for invalid private key")
	}
}

func TestNewDevice_Valid(t *testing.T) {
	wg, err := New(Options{Backend: BackendUserspace})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	dev, err := wg.NewDevice("test", key, cidr(t, "10.0.0.1/32"), 51820)
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}
	if dev == nil {
		t.Fatal("expected non-nil WGDevice")
	}
	if dev.DeviceName() != "test" {
		t.Errorf("DeviceName = %q, want test", dev.DeviceName())
	}
}

func TestRemoveDevice_Existing(t *testing.T) {
	wg, err := New(Options{Backend: BackendUserspace})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	_, err = wg.NewDevice("test", key, cidr(t, "10.0.0.1/32"), 0)
	if err != nil {
		t.Fatalf("NewDevice: %v", err)
	}

	if err := wg.RemoveDevice("test"); err != nil {
		t.Errorf("RemoveDevice: %v", err)
	}
}

func TestRemoveDevice_NotFound(t *testing.T) {
	wg, err := New(Options{Backend: BackendUserspace})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	err = wg.RemoveDevice("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent device")
	}
}

func TestManager_GenerateKeyDelegates(t *testing.T) {
	wg, err := New(Options{Backend: BackendUserspace})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	key, err := wg.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if len(key) != 44 {
		t.Errorf("key length = %d, want 44", len(key))
	}
}

func TestManager_PublicKeyDelegates(t *testing.T) {
	wg, err := New(Options{Backend: BackendUserspace})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	priv, err := wg.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	pub, err := wg.PublicKey(priv)
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}
	if pub == priv {
		t.Error("public key should differ from private key")
	}
}
