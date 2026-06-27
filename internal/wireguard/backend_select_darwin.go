//go:build darwin

package wireguard

import "fmt"

func newBackend(t BackendType) (Backend, error) {
	switch t {
	case BackendAuto, BackendUserspace:
		return &UserspaceBackend{}, nil
	case BackendKernel:
		return nil, fmt.Errorf("wireguard: kernel backend not available on macOS")
	default:
		return nil, fmt.Errorf("wireguard: unknown backend type %q; valid: auto, kernel, userspace", t)
	}
}

func tunName(_ string) string {
	return "utun"
}
