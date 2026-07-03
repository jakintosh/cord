package wireguard

import (
	"fmt"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// GenerateKey produces a new WireGuard private key and returns it as
// a base64-encoded string.
func GenerateKey() (
	string,
	error,
) {
	key, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return "", fmt.Errorf("wireguard: generate key: %w", err)
	}
	return key.String(), nil
}

// PublicKey derives the public key from a base64-encoded private key
// and returns it as a base64-encoded string.
func PublicKey(
	privateKey string,
) (
	string,
	error,
) {
	key, err := wgtypes.ParseKey(privateKey)
	if err != nil {
		return "", fmt.Errorf("wireguard: parse private key: %w", err)
	}
	return key.PublicKey().String(), nil
}

// parseKey converts a base64-encoded key string to a wgtypes.Key.
func parseKey(
	keyStr string,
) (
	wgtypes.Key,
	error,
) {
	key, err := wgtypes.ParseKey(keyStr)
	if err != nil {
		return wgtypes.Key{}, fmt.Errorf("wireguard: parse key: %w", err)
	}
	return key, nil
}
