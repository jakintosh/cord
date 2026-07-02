package wireguard

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"strings"
)

// uapiWriter builds a WireGuard UAPI key=value configuration string.
type uapiWriter struct {
	sb strings.Builder
}

func (w *uapiWriter) Set(
	key string,
	value string,
) {
	fmt.Fprintf(&w.sb, "%s=%s\n", key, value)
}

func (w *uapiWriter) SetHex(
	key string,
	b []byte,
) {
	fmt.Fprintf(&w.sb, "%s=%s\n", key, hex.EncodeToString(b))
}

func (w *uapiWriter) SetInt(
	key string,
	v int,
) {
	fmt.Fprintf(&w.sb, "%s=%d\n", key, v)
}

func (w *uapiWriter) String() string {
	return w.sb.String()
}

// uapiParser iterates over WireGuard UAPI key=value lines.
type uapiParser struct {
	scanner *bufio.Scanner
	key     string
	value   string
	err     error
}

func newUAPIParser(
	raw string,
) *uapiParser {
	return &uapiParser{
		scanner: bufio.NewScanner(strings.NewReader(raw)),
	}
}

// Next advances to the next key=value line. Lines without '=' are
// skipped. Returns false when input is exhausted or a scan error
// occurs; call Err() to check for errors.
func (p *uapiParser) Next() bool {
	for p.scanner.Scan() {
		key, value, ok := strings.Cut(p.scanner.Text(), "=")
		if !ok {
			continue
		}
		p.key = key
		p.value = value
		return true
	}
	if err := p.scanner.Err(); err != nil && p.err == nil {
		p.err = err
	}
	return false
}

func (p *uapiParser) Key() string   { return p.key }
func (p *uapiParser) Value() string { return p.value }
func (p *uapiParser) Err() error    { return p.err }

// DecodeHex decodes the current value as a hex string into dst.
func (p *uapiParser) DecodeHex(
	dst *[]byte,
) error {
	b, err := hex.DecodeString(p.value)
	if err != nil {
		return fmt.Errorf("invalid hex %q: %w", p.value, err)
	}
	*dst = b
	return nil
}

// DecodeInt decodes the current value as a decimal int.
func (p *uapiParser) DecodeInt(
	dst *int,
) error {
	v, err := strconv.Atoi(p.value)
	if err != nil {
		return fmt.Errorf("invalid int %q: %w", p.value, err)
	}
	*dst = v
	return nil
}

// DecodeInt64 decodes the current value as a decimal int64.
func (p *uapiParser) DecodeInt64(
	dst *int64,
) error {
	v, err := strconv.ParseInt(p.value, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid int64 %q: %w", p.value, err)
	}
	*dst = v
	return nil
}

// DecodeUDPAddr parses the current value as a UDP address in
// "host:port" form.
func (p *uapiParser) DecodeUDPAddr(
	dst **net.UDPAddr,
) error {
	addr, err := net.ResolveUDPAddr("udp", p.value)
	if err != nil {
		return fmt.Errorf("invalid udp addr %q: %w", p.value, err)
	}
	*dst = addr
	return nil
}

// DecodeCIDR parses the current value as a CIDR network.
func (p *uapiParser) DecodeCIDR(
	dst *net.IPNet,
) error {
	_, ipNet, err := net.ParseCIDR(p.value)
	if err != nil {
		return fmt.Errorf("invalid cidr %q: %w", p.value, err)
	}
	*dst = *ipNet
	return nil
}
