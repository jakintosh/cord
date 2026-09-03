package service

import "fmt"

// dnsLabelMaxBytes is the RFC 1035 limit for a single DNS label.
const dnsLabelMaxBytes = 63

// validateDNSName checks that name is one valid lowercase DNS label:
// ASCII lowercase letters, digits, and hyphens only, with the first
// and last characters being a letter or digit. Input is rejected
// rather than silently lowercased.
func validateDNSName(
	name string,
) error {
	if name == "" {
		return fmt.Errorf("name is empty")
	}

	if len(name) > dnsLabelMaxBytes {
		return fmt.Errorf("name %q exceeds %d byte DNS label limit", name, dnsLabelMaxBytes)
	}

	if name[0] == '-' {
		return fmt.Errorf("name %q must not start with a hyphen", name)
	}

	if name[len(name)-1] == '-' {
		return fmt.Errorf("name %q must not end with a hyphen", name)
	}

	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-':
		default:
			return fmt.Errorf("name %q may contain only lowercase letters, digits, and hyphens", name)
		}
	}

	return nil
}
