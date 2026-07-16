package topology

import "net"

func mustParseCIDR(
	s string,
) (
	net.IP,
	net.IP,
	int,
	int,
) {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	pref, bits := n.Mask.Size()
	first, last := rangeFromNet(n)
	return first, last, pref, bits
}

func rangeFromNet(
	n *net.IPNet,
) (
	net.IP,
	net.IP,
) {
	f := n.IP.Mask(n.Mask)
	l := make(net.IP, len(f))
	copy(l, f)
	for i := range l {
		l[i] |= ^n.Mask[i]
	}
	return f, l
}

func makeCidr(
	name string,
	cidr string,
	terminal bool,
) Cidr {
	base, last, pref, bits := mustParseCIDR(cidr)
	return Cidr{
		Name:     name,
		Cidr:     cidr,
		Base:     base,
		Last:     last,
		Prefix:   pref,
		Bits:     bits,
		Terminal: terminal,
	}
}
