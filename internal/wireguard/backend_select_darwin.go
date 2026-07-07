//go:build darwin

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
	case BackendAuto, BackendUserspace:
		return &UserspaceBackend{log: log}, nil
	case BackendKernel:
		return nil, fmt.Errorf("wireguard: kernel backend not available on macOS")
	default:
		return nil, fmt.Errorf("wireguard: unknown backend type %q; valid: auto, kernel, userspace", t)
	}
}
