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
		return nil, fmt.Errorf("wireguard: unknown backend type %q; valid: auto, kernel, userspace", t)
	}
}

func tunName(requested string) string {
	return requested
}
