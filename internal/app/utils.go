package app

import (
	"fmt"
	"net"
	"slices"
)

func ValidateNetworkName(name string) error {
	for _, c := range name {
		if !((c >= 0x30 && c <= 0x39) ||
			(c >= 0x41 && c <= 0x5A) ||
			(c >= 0x61 && c <= 0x7A) ||
			c == 0x2D) {
			return fmt.Errorf("invalid network name: must only contain alphanumeric or hyphen characters")
		}
	}
	return nil
}

func firstAssignableIp(cidr *net.IPNet) net.IP {
	ip := cidr.IP
	ip[len(ip)-1] += 1
	return ip
}

func rangeFromCidr(cidr *net.IPNet) (net.IP, net.IP) {
	start := slices.Clone(cidr.IP)
	end := slices.Clone(cidr.Mask)
	for i, octet := range end {
		end[i] = start[i] + ^octet
	}
	return start, net.IP(end)
}
