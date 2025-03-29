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
			c == 0x2D) { // hyphen
			return fmt.Errorf("invalid network name: must only contain alphanumeric or hyphen characters")
		}
	}
	return nil
}

func GetFirstAssignableIpFromCidr(cidr *net.IPNet) net.IP {
	prefix, length := cidr.Mask.Size()
	if prefix == length {
		// if the mask is full, the network IP is the only IP, so we
		// assume that this is an "assignable cidr" and return the
		// base/network IP
		return cidr.IP
	} else {
		// if the mask is not full, we assume the base IP is the
		// network IP, and we should increment to the next address
		// to be used as the first assignable IP
		ip := cidr.IP
		ip[len(ip)-1] += 1
		return ip
	}
}

// get the smallest and largest IP address possible based on the given
// cidr (*net.IPNet)
func GetIpRangeFromCidr(cidr *net.IPNet) (net.IP, net.IP) {
	start := slices.Clone(cidr.IP)
	end := slices.Clone(cidr.Mask)
	for i, octet := range end {
		end[i] = start[i] + ^octet
	}
	return start, net.IP(end)
}

// a 'peer CIDR' is just a fully masked CIDR that represents one IP
// address, but in CIDR notation. in practice: x.x.x.x/32 or xxxx::/128
func GetPeerCidrFromIp(ip net.IP) *net.IPNet {
	ipBits := len(ip) * 8
	fullMask := net.CIDRMask(ipBits, ipBits)
	return &net.IPNet{
		IP:   ip,
		Mask: fullMask,
	}
}
