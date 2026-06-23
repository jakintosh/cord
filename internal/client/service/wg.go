package service

// WG is the WireGuard abstraction the client service depends on. It
// mirrors the server-side WG interface. A production implementation
// will live in internal/wireguard.
type WG interface {
	// GenerateKey produces a new WireGuard private key.
	GenerateKey() (string, error)

	// PublicKey derives the public key from a private key.
	PublicKey(privateKey string) (string, error)

	// NewDevice creates a new WireGuard device (interface) with the
	// given name, private key, address, and listen port. The client
	// uses a fixed port of 0 (OS-assigned) for its interfaces.
	NewDevice(name, privateKey, address string, port uint16) (WGDevice, error)

	// RemoveDevice destroys a WireGuard device by name.
	RemoveDevice(name string) error
}

// WGDevice is a live WireGuard network device for one client network.
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
