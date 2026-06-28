//go:build darwin

package wireguard

import (
	"fmt"
	"net"
	"os/exec"
)

func configureTunOS(
	name string,
	addr net.IPNet,
	mtu int,
	noRoutes bool,
) error {
	cmd := exec.Command("ifconfig", name, "inet", addr.IP.String(), addr.IP.String(), "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wireguard: set addr on %s: %w (%s)", name, err, out)
	}

	cmd = exec.Command("ifconfig", name, "mtu", fmt.Sprintf("%d", mtu))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wireguard: set mtu on %s: %w (%s)", name, err, out)
	}

	if !noRoutes {
		network := net.IPNet{
			IP:   addr.IP.Mask(addr.Mask),
			Mask: addr.Mask,
		}
		cmd = exec.Command("route", "-q", "add", "-net", network.String(), "-interface", name)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("wireguard: add route for %s: %w (%s)", network.String(), err, out)
		}
	}

	return nil
}
