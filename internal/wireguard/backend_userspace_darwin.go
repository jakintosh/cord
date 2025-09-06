//go:build darwin

package wireguard

import (
	"fmt"
	"net"
	"os/exec"
)

// configureTunOS configures the TUN interface with its IP address and MTU, and brings it up on macOS
func configureTunOS(
	name string,
	addr net.IPNet,
	mtu int,
) error {
	// Set IP address
	cmd := exec.Command("ifconfig", name, "inet", addr.IP.String(), addr.IP.String(), "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set IP address on %s: %w", name, err)
	}

	// Set MTU
	cmd = exec.Command("ifconfig", name, "mtu", fmt.Sprintf("%d", mtu))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set MTU on %s: %w", name, err)
	}

	// Add route for the network
	cmd = exec.Command("route", "add", "-net", addr.String(), "-interface", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to add route for %s: %w", addr.String(), err)
	}

	return nil
}
