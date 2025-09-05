package wireguard

// Backend defines the set of operations for a WireGuard implementation.
// This abstraction allows the package to support both kernel-based WireGuard
// (on Linux) and userspace implementations (on macOS, Windows, and optionally Linux).
type Backend interface {
	// Up creates the network device if it doesn't exist, configures it with the
	// current state of the Interface object, and brings it up.
	// It also writes the native .conf file to the specified path.
	Up(iface *Interface, configPath string) error

	// Down brings the interface down and, optionally, deletes it.
	Down(iface *Interface, delete bool) error

	// Sync applies only the changes to the peer list to a live interface
	// without tearing it down. This is more efficient for updates.
	Sync(iface *Interface) error
}
