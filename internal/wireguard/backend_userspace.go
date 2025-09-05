package wireguard

// UserspaceBackend implements the Backend interface for non-Linux systems
// (macOS, Windows) and optionally for Linux. This implementation runs the
// WireGuard protocol in userspace.
//
// It uses:
//   - wireguard-go library to run the WireGuard protocol in userspace
//   - A helper library (like songgao/water or an internal equivalent) to create
//     the virtual TUN network interface
//   - OS-specific build tags for TUN device creation
type UserspaceBackend struct{}

// Up creates the network device if it doesn't exist, configures it with the
// current state of the Interface object, and brings it up.
// It also writes the native .conf file to the specified path.
func (b *UserspaceBackend) Up(iface *Interface, configPath string) error {
	// TODO: Implement userspace backend Up method
	return nil
}

// Down brings the interface down and, optionally, deletes it.
func (b *UserspaceBackend) Down(iface *Interface, delete bool) error {
	// TODO: Implement userspace backend Down method
	return nil
}

// Sync applies only the changes to the peer list to a live interface
// without tearing it down. This is more efficient for updates.
func (b *UserspaceBackend) Sync(iface *Interface) error {
	// TODO: Implement userspace backend Sync method
	return nil
}
