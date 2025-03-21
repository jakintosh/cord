package app

import "fmt"

func ValidateNetworkName(name string) error {
	for _, c := range name {
		if !((c >= 0x30 && c <= 0x39) ||
			(c >= 0x41 && c <= 0x5A) ||
			(c >= 0x61 && c <= 0x7A) ||
			c == 0x2D) {
			return fmt.Errorf("invalid network name: must only contain alphanumeric or hyphen characters")
		}
	}
	return nil
}
