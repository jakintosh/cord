//go:build linux

package wireguard

// KernelBackend implements the Backend interface for Linux systems using
// the kernel WireGuard implementation. This is the default implementation
// on Linux systems.
//
// It uses:
// - vishvananda/netlink library to create and manage the network device
// - golang.zx2c4.com/wireguard/wgctrl library to configure the device's
//   private key, listen port, and peers
type KernelBackend struct{}

// Up creates the network device if it doesn't exist, configures it with the
// current state of the Interface object, and brings it up.
// It also writes the native .conf file to the specified path.
func (b *KernelBackend) Up(iface *Interface, configPath string) error {
	// TODO: Implement kernel backend Up method
	return nil
}

// Down brings the interface down and, optionally, deletes it.
func (b *KernelBackend) Down(iface *Interface, delete bool) error {
	// TODO: Implement kernel backend Down method
	return nil
}

// Sync applies only the changes to the peer list to a live interface
// without tearing it down. This is more efficient for updates.
func (b *KernelBackend) Sync(iface *Interface) error {
	// TODO: Implement kernel backend Sync method
	return nil
}