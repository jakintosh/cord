package utils

import (
	"fmt"
	"net"
	"slices"
)

func ValidateHostName(name string) error {
	for _, c := range name {
		if !((c >= 0x30 && c <= 0x39) || // numbers
			(c >= 0x41 && c <= 0x5A) || // uppercase letters
			(c >= 0x61 && c <= 0x7A) || // lowercase letters
			c == 0x2D || c == 0x2E) { // hyphen, period
			return fmt.Errorf("invalid network name: must only contain alphanumeric or hyphen characters")
		}
	}
	return nil
}

func GetFirstAssignableIpFromCidr(cidr *net.IPNet) net.IP {

	// clone the cidr.IP
	ip := append(net.IP(nil), cidr.IP...)

	// if the mask is full, the network IP is the only IP, so we
	// assume that this is an "assignable cidr" itself
	prefix, length := cidr.Mask.Size()
	if prefix != length {
		// if the mask is not full, we assume the base IP is the
		// network IP, and we should increment to the next address
		// to be used as the first assignable IP
		ip[len(ip)-1] += 1
	}

	// normalize ip to 4-byte slice if necessary
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}

	return ip
}

// get the smallest and largest IP address possible based on the given
// cidr (*net.IPNet)
func GetIpRangeFromCidr(cidr *net.IPNet) (net.IP, net.IP) {
	// Normalize IPv4 to 4-byte form to ensure consistent DB storage/comparisons
	if v4 := cidr.IP.To4(); v4 != nil {
		start := slices.Clone(v4)
		mask := cidr.Mask
		// mask for IPv4 should be 4 bytes; compute broadcast/end
		end := make([]byte, 4)
		for i := range 4 {
			end[i] = start[i] + ^mask[i]
		}
		return net.IP(start), net.IP(end)
	}

	// IPv6 path: operate on full 16-byte address
	start := slices.Clone(cidr.IP)
	mask := cidr.Mask
	end := make([]byte, len(start))
	for i := range start {
		end[i] = start[i] + ^mask[i]
	}
	return net.IP(start), net.IP(end)
}

// a 'peer CIDR' is just a fully masked CIDR that represents one IP
// address, but in CIDR notation. in practice: x.x.x.x/32 or xxxx::/128
func GetPeerCidrFromIp(ip net.IP) *net.IPNet {
	if v4 := ip.To4(); v4 != nil {
		return &net.IPNet{
			IP:   v4,
			Mask: net.CIDRMask(32, 32),
		}
	} else {
		return &net.IPNet{
			IP:   ip,
			Mask: net.CIDRMask(256, 256),
		}
	}
}
