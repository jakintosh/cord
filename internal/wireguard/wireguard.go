package wireguard

import (
	"fmt"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// GeneratePrivateKey generates a new WireGuard private key.
func GeneratePrivateKey() (wgtypes.Key, error) {
	return wgtypes.GeneratePrivateKey()
}

// ParseKey parses a base64-encoded WireGuard key string.
func ParseKey(keyStr string) (wgtypes.Key, error) {
	key, err := wgtypes.ParseKey(keyStr)
	if err != nil {
		return wgtypes.Key{}, fmt.Errorf("failed to parse key: %w", err)
	}
	return key, nil
}
