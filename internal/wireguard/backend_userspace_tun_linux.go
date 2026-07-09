//go:build linux

package wireguard

import (
	"fmt"
	"net"
	"os/exec"
)

// tunRequestName maps a logical device name to the name passed to
// tun.CreateTUN. On Linux the userspace backend uses the logical name
// directly as the interface name.
func tunRequestName(
	name string,
) string {
	return name
}

func configureTunOS(
	name string,
	addr net.IPNet,
	networkCIDR net.IPNet,
	mtu int,
) error {
	cmd := exec.Command("ip", "addr", "replace", addr.String(), "dev", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wireguard: set addr on %s: %w (%s)", name, err, out)
	}

	cmd = exec.Command("ip", "link", "set", name, "mtu", fmt.Sprintf("%d", mtu))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wireguard: set mtu on %s: %w (%s)", name, err, out)
	}

	cmd = exec.Command("ip", "link", "set", name, "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wireguard: bring %s up: %w (%s)", name, err, out)
	}

	cmd = exec.Command("ip", "route", "replace", networkCIDR.String(), "dev", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wireguard: add route for %s: %w (%s)", networkCIDR.String(), err, out)
	}

	return nil
}
