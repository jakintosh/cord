// Package netaddr provides small, pure helpers for the IP and CIDR
// math used across cord. Centralizing these avoids re-deriving the
// same address arithmetic (and the same footguns, such as
// net.ParseCIDR silently masking host bits) in each package.
//
// All functions operate on the standard net.IP / net.IPNet types and
// normalize IPv4 addresses to their 4-byte form so that an address and
// its mask stay byte-length consistent.
package netaddr

import "net"

// Normalize returns the canonical representation of ip: a 4-byte slice
// for IPv4 (including IPv4-in-IPv6), or the original slice for IPv6.
func Normalize(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip
}

// Increment returns ip + 1, carrying across all octets.
func Increment(ip net.IP) net.IP {
	ip = Normalize(ip)
	next := make(net.IP, len(ip))
	copy(next, ip)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] > 0 {
			break
		}
	}
	return next
}

// FirstAssignable returns the first usable host address in n: the
// network address plus one, which is typically the gateway/server
// address.
func FirstAssignable(n *net.IPNet) net.IP {
	ip := make(net.IP, len(n.IP))
	copy(ip, n.IP)
	ip[len(ip)-1]++
	return Normalize(ip)
}

// Range returns the network (first) and broadcast (last) addresses of n.
func Range(n *net.IPNet) (first, last net.IP) {
	f := n.IP.Mask(n.Mask)
	l := make(net.IP, len(f))
	copy(l, f)
	for i := range l {
		l[i] |= ^n.Mask[i]
	}
	return Normalize(f), Normalize(l)
}

// TerminalPrefix returns the single-host route prefix length for ip:
// 32 for IPv4, 128 for IPv6.
func TerminalPrefix(ip net.IP) int {
	if ip.To4() != nil {
		return 32
	}
	return 128
}

// InterfaceAddress returns the address to assign to a device on network
// n: the first assignable host IP carrying n's prefix length (e.g.
// 10.0.0.1/16). This is deliberately not a masked network address — the
// host bits identify the device itself.
func InterfaceAddress(n *net.IPNet) net.IPNet {
	mask := make(net.IPMask, len(n.Mask))
	copy(mask, n.Mask)
	return net.IPNet{
		IP:   FirstAssignable(n),
		Mask: mask,
	}
}

// HostRoute returns a single-host route for ip (e.g. 10.0.0.5/32 or
// fd00::1/128).
func HostRoute(ip net.IP) net.IPNet {
	ip = Normalize(ip)
	return net.IPNet{
		IP:   ip,
		Mask: net.CIDRMask(TerminalPrefix(ip), len(ip)*8),
	}
}

// ParseInterface parses an interface address in CIDR notation (e.g.
// "10.0.0.1/16") while preserving the host bits. Unlike net.ParseCIDR —
// whose returned *net.IPNet is masked to the network address — the
// result retains the host portion, so it is suitable for assigning to a
// device. Returns an error if s is not valid CIDR.
func ParseInterface(s string) (net.IPNet, error) {
	ip, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		return net.IPNet{}, err
	}
	ipNet.IP = Normalize(ip)
	return *ipNet, nil
}
