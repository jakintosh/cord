//go:build darwin

package wireguard

import "fmt"

func newBackend(t BackendType) (Backend, error) {
	switch t {
	case BackendAuto, BackendUserspace:
		return &UserspaceBackend{}, nil
	case BackendKernel:
		return nil, fmt.Errorf("kernel backend is not available on macOS; use the userspace backend")
	default:
		return nil, fmt.Errorf("unknown backend type: %d", t)
	}
}

// tunName returns the name to request when creating a TUN device.
// macOS only allows utun devices, so the OS picks the next free utunN;
// the requested cord name is recorded separately for config files.
func tunName(requested string) string {
	return "utun"
}
