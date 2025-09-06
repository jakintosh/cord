//go:build linux

package wireguard

import (
	"fmt"
	"net"
	"os/exec"
)

// configureTunOS configures the TUN interface with its IP address and MTU, and brings it up on Linux
func configureTunOS(
	name string,
	addr net.IPNet,
	mtu int,
) error {
	// Set IP address
	cmd := exec.Command("ip", "addr", "add", addr.String(), "dev", name)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set IP address on %s: %w", name, err)
	}

	// Set MTU
	cmd = exec.Command("ip", "link", "set", name, "mtu", fmt.Sprintf("%d", mtu))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set MTU on %s: %w", name, err)
	}

	// Bring interface up
	cmd = exec.Command("ip", "link", "set", name, "up")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to bring interface %s up: %w", name, err)
	}

	return nil
}
