package wireguard

import (
	"strings"
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestGenerateKey_ProducesValidBase64(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	if len(key) != 44 {
		t.Errorf("key length = %d, want 44", len(key))
	}

	// Must be valid WireGuard base64
	_, err = wgtypes.ParseKey(key)
	if err != nil {
		t.Errorf("key %q is not valid wgtypes base64: %v", key, err)
	}
}

func TestGenerateKey_ProducesUniqueKeys(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 10; i++ {
		key, err := GenerateKey()
		if err != nil {
			t.Fatalf("GenerateKey: %v", err)
		}
		if seen[key] {
			t.Errorf("duplicate key generated: %q", key)
		}
		seen[key] = true
	}
}

func TestPublicKey_DerivesFromRealKey(t *testing.T) {
	priv, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	pub, err := PublicKey(priv)
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}

	if pub == priv {
		t.Error("public key should differ from private key")
	}

	if len(pub) != 44 {
		t.Errorf("public key length = %d, want 44", len(pub))
	}

	// Derive again — must be deterministic
	pub2, err := PublicKey(priv)
	if err != nil {
		t.Fatalf("second PublicKey: %v", err)
	}
	if pub != pub2 {
		t.Error("PublicKey not deterministic on same input")
	}
}

func TestPublicKey_InvalidInput(t *testing.T) {
	_, err := PublicKey("not-a-valid-key")
	if err == nil {
		t.Error("expected error for invalid key")
	}
}

func TestParseKey_RoundTrip(t *testing.T) {
	priv, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	wgKey, err := parseKey(priv)
	if err != nil {
		t.Fatalf("parseKey: %v", err)
	}

	if wgKey.String() != priv {
		t.Errorf("round-trip mismatch: got %q, want %q", wgKey.String(), priv)
	}
}

func TestParseKey_InvalidInput(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"empty", ""},
		{"garbage", "not-a-key"},
		{"wrong length", strings.Repeat("a", 43)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseKey(tt.key)
			if err == nil {
				t.Errorf("expected error for %q", tt.key)
			}
		})
	}
}
