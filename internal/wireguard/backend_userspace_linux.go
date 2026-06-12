//go:build linux

package wireguard

import (
	"fmt"
	"net"
	"os/exec"
)

// configureTunOS assigns the address, sets the MTU, and brings the
// device up. On Linux the connected route is created automatically
// from the address's prefix, so noRoutes has no extra work to skip.
func configureTunOS(
	name string,
	addr net.IPNet,
	mtu int,
	noRoutes bool,
) error {
	// Set IP address
	cmd := exec.Command("ip", "addr", "replace", addr.String(), "dev", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set IP address on %s: %w (%s)", name, err, out)
	}

	// Set MTU
	cmd = exec.Command("ip", "link", "set", name, "mtu", fmt.Sprintf("%d", mtu))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set MTU on %s: %w (%s)", name, err, out)
	}

	// Bring interface up
	cmd = exec.Command("ip", "link", "set", name, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to bring interface %s up: %w (%s)", name, err, out)
	}

	return nil
}
