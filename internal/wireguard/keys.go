package wireguard

import (
	"fmt"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// ParseKey parses a base64-encoded WireGuard key string.
func ParseKey(keyStr string) (wgtypes.Key, error) {
	key, err := wgtypes.ParseKey(keyStr)
	if err != nil {
		return wgtypes.Key{}, fmt.Errorf("failed to parse key: %w", err)
	}
	return key, nil
}

// GeneratePrivateKey generates a new WireGuard private key.
func GeneratePrivateKey() (wgtypes.Key, error) {
	private, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return wgtypes.Key{}, fmt.Errorf("failed to generate private key: %w", err)
	}
	return private, nil
}

// GenerateKeypair generates a new WireGuard private/public key pair.
func GenerateKeypair() (wgtypes.Key, wgtypes.Key, error) {
	private, err := GeneratePrivateKey()
	if err != nil {
		return wgtypes.Key{}, wgtypes.Key{}, err
	}
	public := private.PublicKey()
	return private, public, nil
}

// Legacy wrapper types for backward compatibility
type PrivateKey wgtypes.Key
type PublicKey wgtypes.Key

func (k *PublicKey) String() string {
	return wgtypes.Key(*k).String()
}

func (k *PrivateKey) String() string {
	return wgtypes.Key(*k).String()
}

func ParsePubKey(key string) (PublicKey, error) {
	pubKey, err := ParseKey(key)
	if err != nil {
		return PublicKey{}, err
	}
	return PublicKey(pubKey), nil
}
