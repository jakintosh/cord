package service

import (
	"crypto/rand"
	"encoding/base64"
)

// WG is the WireGuard abstraction the server service depends on.
// A production implementation will live in internal/wireguard.
type WG interface {
	// GenerateKey produces a new WireGuard private key.
	GenerateKey() (string, error)

	// PublicKey derives the public key from a private key.
	PublicKey(privateKey string) (string, error)

	// NewDevice creates a new WireGuard device (interface) for a
	// server network. The device is configured with the given
	// private key, address, and listen port.
	NewDevice(name, privateKey, address string, port uint16) (WGDevice, error)

	// RemoveDevice destroys a WireGuard device by name.
	RemoveDevice(name string) error
}

// stubWG is the default WG used when none is provided. It generates
// random base64 keys (not real WireGuard keys) so that domain flows
// like CreateNetwork and CreateInvite can proceed. Device operations
// return ErrNotImplemented.
type stubWG struct{}

func (stubWG) GenerateKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func (stubWG) PublicKey(privateKey string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

func (stubWG) NewDevice(string, string, string, uint16) (WGDevice, error) {
	return nil, ErrNotImplemented
}

func (stubWG) RemoveDevice(string) error {
	return ErrNotImplemented
}

// WGDevice is a live WireGuard network device for one network.
// ApplyPeers replaces the entire peer configuration and reconciles
// against the live device in one call.
type WGDevice interface {
	// ApplyPeers replaces the entire peer set on this device and
	// reconciles against the live WireGuard state.
	ApplyPeers(peers []WGPeer) error

	// Up brings the device into the running state.
	Up() error

	// Down brings the device down. When remove is true the device
	// is destroyed; when false it is only administratively down.
	Down(remove bool) error

	// DeviceName returns the WireGuard interface name.
	DeviceName() string
}

// WGPeer is a single peer entry in a WireGuard device configuration.
// It is intentionally narrower than the domain Peer type — it carries
// only the fields WireGuard needs for configuration.
type WGPeer struct {
	PublicKey  string
	AllowedIPs []string
	Endpoint   string // "host:port"; empty means dynamic endpoint
}
