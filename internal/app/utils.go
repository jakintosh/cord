package app

import (
	"fmt"
	"net"
	"slices"

	sqlite "modernc.org/sqlite"
	sqlite3 "modernc.org/sqlite/lib"
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

// a 'peer CIDR' is just a fully masked CIDR that represents one IP address,
// but in CIDR notation. in practice: x.x.x.x/32 or xxxx::/128
func peerCidr(ip net.IP) *net.IPNet {
	ipBits := len(ip) * 8
	fullMask := net.CIDRMask(ipBits, ipBits)
	return &net.IPNet{
		IP:   ip,
		Mask: fullMask,
	}
}

func checkSqliteErr(err error) error {
	if err == nil {
		return nil
	}

	if sqliteErr, ok := err.(*sqlite.Error); ok {
		if sqliteErr.Code() == sqlite3.SQLITE_CONSTRAINT_UNIQUE {
			return fmt.Errorf("Duplicate record")
		} else {
			return fmt.Errorf("SQLite error (%d): %s", sqliteErr.Code(), sqliteErr.Error())
		}
	} else {
		return fmt.Errorf("other database error: %w", err)
	}
}
