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
	ip := slices.Clone(cidr.IP)

	// if the mask is full, the network IP is the only IP, so we
	// assume that this is an "assignable cidr" itself
	prefix, length := cidr.Mask.Size()
	if prefix != length {
		// if the mask is not full, we assume the base IP is the
		// network IP, and we should increment to the next address
		// to be used as the first assignable IP
		ip[len(ip)-1] += 1
	}

	return NormalizeIP(ip)
}

// get the smallest and largest IP address possible based on the given
// cidr (*net.IPNet)
func GetIpRangeFromCidr(cidr *net.IPNet) (net.IP, net.IP) {
	start := slices.Clone(cidr.IP)
	mask := cidr.Mask
	end := make([]byte, len(start))
	for i := range start {
		end[i] = start[i] + ^mask[i]
	}
	return NormalizeIP(net.IP(start)), NormalizeIP(net.IP(end))
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

func NormalizeIP(ip net.IP) net.IP {
	if v4 := ip.To4(); v4 != nil {
		return v4
	}
	return ip
}
