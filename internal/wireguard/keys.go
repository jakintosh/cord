package wireguard

import (
	"fmt"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type PrivateKey wgtypes.Key
type PublicKey wgtypes.Key

func (k *PublicKey) String() string {
	return wgtypes.Key(*k).String()
}

func (k *PrivateKey) String() string {
	return wgtypes.Key(*k).String()
}

func ParsePubKey(key string) (PublicKey, error) {
	pubKey, err := wgtypes.ParseKey(key)
	if err != nil {
		return PublicKey{}, err
	}
	return PublicKey(pubKey), nil
}

func GenerateKeypair() (PrivateKey, PublicKey, error) {
	private, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return PrivateKey{}, PublicKey{},
			fmt.Errorf("failed to generate wg keys: %w", err)
	}
	public := private.PublicKey()

	return PrivateKey(private), PublicKey(public), nil
}
