package wireguard_test

import (
	"testing"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"git.studiopollinator.com/pollinator/cord/internal/wireguard"
)

func TestGenerateKey_ProducesValidBase64(t *testing.T) {
	key, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	if len(key) != 44 {
		t.Errorf("key length = %d, want 44", len(key))
	}

	if _, err := wgtypes.ParseKey(key); err != nil {
		t.Errorf("key %q is not valid wgtypes base64: %v", key, err)
	}
}

func TestGenerateKey_ProducesUniqueKeys(t *testing.T) {
	seen := make(map[string]bool)
	for range 10 {
		key, err := wireguard.GenerateKey()
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
	priv, err := wireguard.GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}

	pub, err := wireguard.PublicKey(priv)
	if err != nil {
		t.Fatalf("PublicKey: %v", err)
	}

	if pub == priv {
		t.Error("public key should differ from private key")
	}
	if len(pub) != 44 {
		t.Errorf("public key length = %d, want 44", len(pub))
	}

	pub2, err := wireguard.PublicKey(priv)
	if err != nil {
		t.Fatalf("second PublicKey: %v", err)
	}
	if pub != pub2 {
		t.Error("PublicKey not deterministic on same input")
	}
}

func TestPublicKey_InvalidInput(t *testing.T) {
	_, err := wireguard.PublicKey("not-a-valid-key")
	if err == nil {
		t.Error("expected error for invalid key")
	}
}
