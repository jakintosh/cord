//go:build darwin

package wireguard

import (
	"fmt"
	"net"
	"os/exec"
)

// tunRequestName maps a logical device name to the name passed to
// tun.CreateTUN. On macOS the TUN name must match utun[0-9]*; passing
// the bare "utun" lets the kernel assign the next free unit, and the
// real name is read back via tunDev.Name().
func tunRequestName(
	_ string,
) string {
	return "utun"
}

func configureTunOS(
	name string,
	addr net.IPNet,
	networkCIDR net.IPNet,
	mtu int,
) error {
	cmd := exec.Command("ifconfig", name, "inet", addr.IP.String(), addr.IP.String(), "up")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wireguard: set addr on %s: %w (%s)", name, err, out)
	}

	cmd = exec.Command("ifconfig", name, "mtu", fmt.Sprintf("%d", mtu))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wireguard: set mtu on %s: %w (%s)", name, err, out)
	}

	cmd = exec.Command("route", "-q", "add", "-net", networkCIDR.String(), "-interface", name)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("wireguard: add route for %s: %w (%s)", networkCIDR.String(), err, out)
	}

	return nil
}
