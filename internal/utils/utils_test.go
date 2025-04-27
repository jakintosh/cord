package utils

import (
	"net"
	"testing"
)

func TestCIDRRange(t *testing.T) {

	_, cidr, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal("invalid cidr")
	}

	expectedStart := net.ParseIP("10.0.0.0")
	start, end := GetIpRangeFromCidr(cidr)
	if !start.Equal(expectedStart) {
		t.Fatalf("invalid start: %v, expected %v", start, expectedStart)
	}
	expectedEnd := net.ParseIP("10.255.255.255")
	if !end.Equal(expectedEnd) {
		t.Fatalf("invalid end: %v, expected %v", end, expectedEnd)
	}
}

func TestFirstAssignableIPLarge(t *testing.T) {

	_, cidr, err := net.ParseCIDR("10.0.0.0/8")
	if err != nil {
		t.Fatal("invalid cidr")
	}

	expectedIp := net.ParseIP("10.0.0.1")
	ip := GetFirstAssignableIpFromCidr(cidr)
	if !ip.Equal(expectedIp) {
		t.Fatalf("large range, unexpected ip: %v, expected %v", ip, expectedIp)
	}
}

func TestFirstAssignableIPSmall(t *testing.T) {

	_, cidr, err := net.ParseCIDR("10.0.0.0/32")
	if err != nil {
		t.Fatal("invalid cidr")
	}

	expectedIp := net.ParseIP("10.0.0.0")
	ip := GetFirstAssignableIpFromCidr(cidr)
	if !ip.Equal(expectedIp) {
		t.Fatalf("smallest range, unexpected ip: %v, expected %v", ip, expectedIp)
	}
}

func TestPeerCidrIPv4(t *testing.T) {

	ip := net.ParseIP("10.0.0.0")
	cidr := GetPeerCidrFromIp(ip)
	expectedCidr := net.IPNet{
		IP:   net.IPv4(10, 0, 0, 0),
		Mask: net.CIDRMask(32, 32),
	}
	if !cidr.IP.Equal(expectedCidr.IP) {
		t.Fatalf("IPv4 unexpected ip: %v, expected %v", cidr.IP, expectedCidr.IP)
	}

	ones, bits := cidr.Mask.Size()
	expectedOnes, expectedBits := expectedCidr.Mask.Size()
	if ones != expectedOnes {
		t.Fatalf("IPv4 unexpected mask prefix: %d, expected %d", ones, expectedOnes)
	}
	if bits != expectedBits {
		t.Fatalf("IPv4 unexpected mask length: %d, expected %d", bits, expectedBits)
	}
}

func TestPeerCidrIPv6(t *testing.T) {

	ip := net.ParseIP("ffee::")
	cidr := GetPeerCidrFromIp(ip)
	expectedCidr := net.IPNet{
		IP:   net.ParseIP("ffee::"),
		Mask: net.CIDRMask(256, 256),
	}
	if !cidr.IP.Equal(expectedCidr.IP) {
		t.Fatalf("IPv6 unexpected ip: %v, expected %v", cidr.IP, expectedCidr.IP)
	}

	ones, bits := cidr.Mask.Size()
	expectedOnes, expectedBits := expectedCidr.Mask.Size()
	if ones != expectedOnes {
		t.Fatalf("IPv6 unexpected mask prefix: %d, expected %d", ones, expectedOnes)
	}
	if bits != expectedBits {
		t.Fatalf("IPv6 unexpected mask length: %d, expected %d", bits, expectedBits)
	}

}
