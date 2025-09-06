//go:build windows

package wireguard

import (
	"fmt"
	"net"
	"os/exec"
)

// configureTunOS configures the TUN interface with its IP address and MTU, and brings it up on Windows
func configureTunOS(
	name string,
	addr net.IPNet,
	mtu int,
) error {
	// Set IP address using netsh
	cmd := exec.Command("netsh", "interface", "ip", "set", "address", name, "static", addr.IP.String(), addr.Mask.String())
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set IP address on %s: %w", name, err)
	}

	// Set MTU using netsh
	cmd = exec.Command("netsh", "interface", "ipv4", "set", "subinterface", name, "mtu="+fmt.Sprintf("%d", mtu))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to set MTU on %s: %w", name, err)
	}

	// Enable the interface
	cmd = exec.Command("netsh", "interface", "set", "interface", name, "admin=enabled")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to enable interface %s: %w", name, err)
	}

	return nil
}
