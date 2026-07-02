package wireguard

import (
	"fmt"
	"strings"
)

const defaultMTU = 1420

// BackendType selects which WireGuard implementation drives a device.
type BackendType string

const (
	BackendAuto      BackendType = "auto"
	BackendKernel    BackendType = "kernel"
	BackendUserspace BackendType = "userspace"
)

// ParseBackendType converts a string to a BackendType, normalizing
// case and whitespace. An empty string returns BackendAuto. Returns
// an error for unrecognized values.
func ParseBackendType(
	s string,
) (
	BackendType,
	error,
) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "auto", "":
		return BackendAuto, nil
	case "kernel":
		return BackendKernel, nil
	case "userspace":
		return BackendUserspace, nil
	default:
		return "", fmt.Errorf("unknown wireguard backend %q; valid: auto, kernel, userspace", s)
	}
}

// Backend creates per-device handles. Implementations manage the
// platform-specific WireGuard resource lifecycle (kernel netlink or
// userspace wireguard-go).
type Backend interface {
	CreateDevice(cfg DeviceConfig) (WgDevice, error)
}

// WgDevice is a live WireGuard interface handle returned by Backend.
// Each WgDevice is owned by exactly one Device, which serializes access.
type WgDevice interface {
	Peers() ([]PeerStatus, error)
	ApplyPeers(ops []PeerOp) error
	Close() error
}
