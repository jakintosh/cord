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
		{"10.27.0.5/16", "10.27.0.1"},       // unmasked input
		{"192.168.1.100/24", "192.168.1.1"}, // unmasked input
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

func TestFirstAssignable_UnmaskedManually(t *testing.T) {
	// Construct a net.IPNet with host bits set (simulating a caller
	// that didn't go through net.ParseCIDR).
	ip := net.ParseIP("10.0.0.5")
	mask := net.CIDRMask(16, 32)
	n := &net.IPNet{IP: ip, Mask: mask}
	got := FirstAssignable(n)
	if got.String() != "10.0.0.1" {
		t.Fatalf("FirstAssignable(unmasked 10.0.0.5/16) = %s, want 10.0.0.1", got)
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

func TestEndpoint(t *testing.T) {
	tests := []struct {
		name string
		ip   net.IP
		port uint16
		want string
	}{
		{"v4", net.ParseIP("10.0.0.1"), 8080, "10.0.0.1:8080"},
		{"v4-4byte", net.IP{10, 0, 0, 1}, 51820, "10.0.0.1:51820"},
		{"v6", net.ParseIP("fd00::1"), 8080, "[fd00::1]:8080"},
		{"v6-loopback", net.ParseIP("::1"), 9090, "[::1]:9090"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Endpoint(tt.ip, tt.port)
			if got != tt.want {
				t.Fatalf("Endpoint(%v, %d) = %q, want %q", tt.ip, tt.port, got, tt.want)
			}
		})
	}
}

func TestOverlaps(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want bool
	}{
		{"identical", "10.0.0.0/16", "10.0.0.0/16", true},
		{"full containment", "10.0.0.0/16", "10.0.1.0/24", true},
		{"reverse containment", "10.0.1.0/24", "10.0.0.0/16", true},
		{"partial overlap", "10.0.0.0/24", "10.0.0.128/25", true},
		{"adjacent", "10.0.0.0/25", "10.0.0.128/25", false},
		{"disjoint-v4", "10.0.0.0/24", "10.0.1.0/24", false},
		{"host-in-network", "10.0.0.0/16", "10.0.0.5/32", true},
		{"disjoint-host", "10.0.0.5/32", "10.0.0.6/32", false},
		{"identical-host", "10.0.0.5/32", "10.0.0.5/32", true},
		{"v6-identical", "fd00::/64", "fd00::/64", true},
		{"v6-disjoint", "fd00::/64", "fd01::/64", false},
		{"v6-host-in-network", "fd00::/64", "fd00::1/128", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Overlaps(mustCIDR(t, tt.a), mustCIDR(t, tt.b))
			if got != tt.want {
				t.Fatalf("Overlaps(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestOverlaps_Symmetric(t *testing.T) {
	a := mustCIDR(t, "10.0.0.0/16")
	b := mustCIDR(t, "10.0.1.0/24")
	if Overlaps(a, b) != Overlaps(b, a) {
		t.Fatal("Overlaps is not symmetric")
	}
	// disjoint
	c := mustCIDR(t, "10.0.0.0/25")
	d := mustCIDR(t, "10.0.0.128/25")
	if Overlaps(c, d) != Overlaps(d, c) {
		t.Fatal("Overlaps is not symmetric for disjoint networks")
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name  string
		outer string
		inner string
		want  bool
	}{
		{"identical", "10.0.0.0/16", "10.0.0.0/16", true},
		{"contains-subnet", "10.0.0.0/16", "10.0.1.0/24", true},
		{"contains-host", "10.0.0.0/16", "10.0.0.5/32", true},
		{"subnet-not-contain-parent", "10.0.1.0/24", "10.0.0.0/16", false},
		{"smaller-not-contain-larger", "10.0.0.128/25", "10.0.0.0/24", false},
		{"adjacent-not-contain", "10.0.0.0/25", "10.0.0.128/25", false},
		{"disjoint", "10.0.0.0/24", "10.0.1.0/24", false},
		{"v6-contains-host", "fd00::/64", "fd00::1/128", true},
		{"v6-disjoint", "fd00::/64", "fd01::/64", false},
		{"/32-equal", "10.0.0.5/32", "10.0.0.5/32", true},
		{"/32-disjoint", "10.0.0.5/32", "10.0.0.6/32", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Contains(mustCIDR(t, tt.outer), mustCIDR(t, tt.inner))
			if got != tt.want {
				t.Fatalf("Contains(%q, %q) = %v, want %v", tt.outer, tt.inner, got, tt.want)
			}
		})
	}
}

func TestContains_ManualNet(t *testing.T) {
	// Manual net.IPNet with host bits set in inner.IP — validates
	// that Contains still correctly uses Range (which masks) for
	// determining boundaries.
	outer := &net.IPNet{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(24, 32)}
	inner := &net.IPNet{IP: net.ParseIP("10.0.0.200"), Mask: net.CIDRMask(25, 32)}
	if !Contains(outer, inner) {
		t.Fatal("Contains(10.0.0.0/24, manually-constructed 10.0.0.200/25) = false, want true")
	}
}

func TestHostRouteFromCidr(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"10.0.0.5/32", "10.0.0.5/32"},
		{"10.42.0.5/16", "10.42.0.5/32"}, // wider prefix narrowed
		{"10.0.0.0/16", "10.0.0.0/32"},   // network address as /32
		{"172.16.10.1/24", "172.16.10.1/32"},
		{"fd00::1/64", "fd00::1/128"},
		{"fd00::/64", "fd00::/128"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := HostRouteFromCidr(tt.in)
			if err != nil {
				t.Fatalf("HostRouteFromCidr(%q): %v", tt.in, err)
			}
			if got.String() != tt.want {
				t.Fatalf("HostRouteFromCidr(%q) = %q, want %q", tt.in, got.String(), tt.want)
			}
		})
	}
}

func TestHostRouteFromCidr_Invalid(t *testing.T) {
	if _, err := HostRouteFromCidr("not-a-cidr"); err == nil {
		t.Fatal("expected error for invalid CIDR")
	}
}

func TestHostRouteFromCidr_AllHostBitsPreserved(t *testing.T) {
	// HostRouteFromCidr must preserve the host bits of the input IP
	// and just change the prefix length.
	got, err := HostRouteFromCidr("10.27.0.1/16")
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "10.27.0.1/32" {
		t.Fatalf("HostRouteFromCidr(10.27.0.1/16) = %q, want 10.27.0.1/32", got.String())
	}
}
