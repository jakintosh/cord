//go:build darwin

package wireguard

import (
	"fmt"
	"net"
	"os/exec"
)

// configureTunOS assigns the address, sets the MTU, and (optionally)
// routes the interface's network through the new utun device.
func configureTunOS(
	name string,
	addr net.IPNet,
	mtu int,
	noRoutes bool,
) error {
	// Set IP address (utun devices are point-to-point, so the host
	// address is used for both ends)
	cmd := exec.Command("ifconfig", name, "inet", addr.IP.String(), addr.IP.String(), "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set IP address on %s: %w (%s)", name, err, out)
	}

	// Set MTU
	cmd = exec.Command("ifconfig", name, "mtu", fmt.Sprintf("%d", mtu))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to set MTU on %s: %w (%s)", name, err, out)
	}

	// Add route for the network so traffic to other peers uses this device
	if !noRoutes {
		network := net.IPNet{IP: addr.IP.Mask(addr.Mask), Mask: addr.Mask}
		cmd = exec.Command("route", "-q", "add", "-net", network.String(), "-interface", name)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to add route for %s: %w (%s)", network.String(), err, out)
		}
	}

	return nil
}
