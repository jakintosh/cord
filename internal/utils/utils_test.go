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
