//go:build linux

package wireguard

import (
	"fmt"
	"log/slog"
)

func newBackend(
	t BackendType,
	log *slog.Logger,
) (
	Backend,
	error,
) {
	switch t {
	case BackendAuto, BackendKernel:
		return &KernelBackend{}, nil
	case BackendUserspace:
		return &UserspaceBackend{log: log}, nil
	default:
		return nil, fmt.Errorf("wireguard: unknown backend type %q; valid: auto, kernel, userspace", t)
	}
}
