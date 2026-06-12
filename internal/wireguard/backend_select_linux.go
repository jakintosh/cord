//go:build linux

package wireguard

import "fmt"

func newBackend(t BackendType) (Backend, error) {
	switch t {
	case BackendAuto, BackendKernel:
		return &KernelBackend{}, nil
	case BackendUserspace:
		return &UserspaceBackend{}, nil
	default:
		return nil, fmt.Errorf("unknown backend type: %d", t)
	}
}

// tunName returns the name to request when creating a TUN device.
// Linux honors the requested name directly.
func tunName(requested string) string {
	return requested
}
