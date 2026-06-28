package netaddr

import (
	"net"
	"testing"
)

func mustCIDR(t *testing.T, s string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		t.Fatalf("ParseCIDR(%q): %v", s, err)
	}
	return n
}

func TestNormalize(t *testing.T) {
	tests := []struct {
		name   string
		in     net.IP
		want   string
		wantV4 bool
	}{
		{"v4-from-parse", net.ParseIP("10.0.0.1"), "10.0.0.1", true},
		{"v4-4byte", net.IP{10, 0, 0, 1}, "10.0.0.1", true},
		{"v6", net.ParseIP("fd00::1"), "fd00::1", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Normalize(tt.in)
			if got.String() != tt.want {
				t.Fatalf("Normalize(%v) = %v, want %v", tt.in, got, tt.want)
			}
			if tt.wantV4 && len(got) != net.IPv4len {
				t.Fatalf("Normalize(%v) len = %d, want %d (4-byte v4)", tt.in, len(got), net.IPv4len)
			}
			if !tt.wantV4 && len(got) != net.IPv6len {
				t.Fatalf("Normalize(%v) len = %d, want %d (16-byte v6)", tt.in, len(got), net.IPv6len)
			}
		})
	}
}

func TestIncrement(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"10.0.0.1", "10.0.0.2"},
		{"10.0.0.255", "10.0.1.0"},     // carry across octet
		{"10.0.255.255", "10.1.0.0"},   // carry across two octets
		{"255.255.255.255", "0.0.0.0"}, // full wrap
		{"fd00::1", "fd00::2"},
		{"fd00::ffff", "fd00::1:0"}, // v6 carry
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := Increment(net.ParseIP(tt.in))
			if got.String() != tt.want {
				t.Fatalf("Increment(%s) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestIncrement_DoesNotMutateInput(t *testing.T) {
	in := net.ParseIP("10.0.0.1")
	before := in.String()
	_ = Increment(in)
	if in.String() != before {
		t.Fatalf("Increment mutated input: %s != %s", in, before)
	}
}

func TestFirstAssignable(t *testing.T) {
	tests := []struct {
		cidr string
		want string
	}{
		{"10.0.0.0/16", "10.0.0.1"},
		{"192.168.1.0/24", "192.168.1.1"},
		{"10.27.0.0/16", "10.27.0.1"},
		{"172.16.10.0/24", "172.16.10.1"},
		{"fd00::/64", "fd00::1"},
	}
	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			got := FirstAssignable(mustCIDR(t, tt.cidr))
			if got.String() != tt.want {
				t.Fatalf("FirstAssignable(%s) = %s, want %s", tt.cidr, got, tt.want)
			}
		})
	}
}

func TestRange(t *testing.T) {
	tests := []struct {
		cidr  string
		first string
		last  string
	}{
		{"10.0.0.0/16", "10.0.0.0", "10.0.255.255"},
		{"192.168.1.0/24", "192.168.1.0", "192.168.1.255"},
		{"10.0.0.0/30", "10.0.0.0", "10.0.0.3"},
		{"10.0.0.0/32", "10.0.0.0", "10.0.0.0"},
		{"fd00::/64", "fd00::", "fd00::ffff:ffff:ffff:ffff"},
	}
	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			first, last := Range(mustCIDR(t, tt.cidr))
			if first.String() != tt.first {
				t.Errorf("Range(%s) first = %s, want %s", tt.cidr, first, tt.first)
			}
			if last.String() != tt.last {
				t.Errorf("Range(%s) last = %s, want %s", tt.cidr, last, tt.last)
			}
		})
	}
}

func TestTerminalPrefix(t *testing.T) {
	if got := TerminalPrefix(net.ParseIP("10.0.0.1")); got != 32 {
		t.Errorf("TerminalPrefix(v4) = %d, want 32", got)
	}
	if got := TerminalPrefix(net.ParseIP("fd00::1")); got != 128 {
		t.Errorf("TerminalPrefix(v6) = %d, want 128", got)
	}
}

func TestInterfaceAddress(t *testing.T) {
	tests := []struct {
		cidr string
		want string
	}{
		{"10.0.0.0/16", "10.0.0.1/16"},
		{"172.16.10.0/24", "172.16.10.1/24"},
		{"10.27.0.0/16", "10.27.0.1/16"},
		{"fd00::/64", "fd00::1/64"},
	}
	for _, tt := range tests {
		t.Run(tt.cidr, func(t *testing.T) {
			got := InterfaceAddress(mustCIDR(t, tt.cidr))
			if got.String() != tt.want {
				t.Fatalf("InterfaceAddress(%s) = %s, want %s", tt.cidr, got.String(), tt.want)
			}
		})
	}
}

func TestInterfaceAddress_DoesNotAliasInputMask(t *testing.T) {
	n := mustCIDR(t, "10.0.0.0/16")
	addr := InterfaceAddress(n)
	// Mutating the returned mask must not corrupt the source network.
	for i := range addr.Mask {
		addr.Mask[i] = 0
	}
	if n.Mask.String() != net.CIDRMask(16, 32).String() {
		t.Fatalf("InterfaceAddress aliased input mask: source mask now %v", n.Mask)
	}
}

func TestHostRoute(t *testing.T) {
	tests := []struct {
		ip   string
		want string
	}{
		{"10.0.0.5", "10.0.0.5/32"},
		{"10.27.0.1", "10.27.0.1/32"},
		{"fd00::1", "fd00::1/128"},
	}
	for _, tt := range tests {
		t.Run(tt.ip, func(t *testing.T) {
			got := HostRoute(net.ParseIP(tt.ip))
			if got.String() != tt.want {
				t.Fatalf("HostRoute(%s) = %s, want %s", tt.ip, got.String(), tt.want)
			}
		})
	}
}

func TestParseInterface_PreservesHostBits(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"10.0.0.1/16", "10.0.0.1/16"}, // host bits preserved
		{"172.16.10.1/24", "172.16.10.1/24"},
		{"10.0.0.0/16", "10.0.0.0/16"}, // network address stays as-is
		{"10.0.0.5/32", "10.0.0.5/32"},
		{"fd00::1/64", "fd00::1/64"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseInterface(tt.in)
			if err != nil {
				t.Fatalf("ParseInterface(%q): %v", tt.in, err)
			}
			if got.String() != tt.want {
				t.Fatalf("ParseInterface(%q) = %q, want %q", tt.in, got.String(), tt.want)
			}
		})
	}
}

// TestParseInterface_DiffersFromParseCIDR documents the exact footgun
// this helper exists to avoid: net.ParseCIDR masks the host bits.
func TestParseInterface_DiffersFromParseCIDR(t *testing.T) {
	_, masked, err := net.ParseCIDR("10.27.0.1/16")
	if err != nil {
		t.Fatal(err)
	}
	if masked.String() != "10.27.0.0/16" {
		t.Fatalf("precondition: net.ParseCIDR returned %q, expected masked network", masked.String())
	}
	got, err := ParseInterface("10.27.0.1/16")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "10.27.0.1/16" {
		t.Fatalf("ParseInterface dropped host bits: %q", got.String())
	}
}

func TestParseInterface_ConsistentByteLengths(t *testing.T) {
	got, err := ParseInterface("10.0.0.1/16")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.IP) != net.IPv4len {
		t.Errorf("IP len = %d, want %d", len(got.IP), net.IPv4len)
	}
	if len(got.Mask) != net.IPv4len {
		t.Errorf("Mask len = %d, want %d", len(got.Mask), net.IPv4len)
	}
}

func TestParseInterface_Invalid(t *testing.T) {
	if _, err := ParseInterface("not-a-cidr"); err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}
