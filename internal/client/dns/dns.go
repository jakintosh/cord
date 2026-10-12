// Package dns manages split-DNS configuration for Cord-owned networks on Linux
// and macOS, the platforms supported by Cord.
// The caller owns interface lifetime and invokes maintenance; Manager starts no
// background work. Application failures may leave partial state, so callers must
// retain the attempted Config until Remove succeeds.
package dns

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnsupported   = errors.New("DNS integration unsupported")
	ErrConflict      = errors.New("DNS configuration ownership conflict")
	ErrInvalidConfig = errors.New("invalid DNS configuration")
)

const defaultTimeout = 5 * time.Second

// dnsLabelMaxBytes is the RFC 1035 limit for a single DNS label.
const dnsLabelMaxBytes = 63

// Config is derived from an installed network and its live interface. Interface
// must be the actual OS name, not the configured logical name on macOS.
type Config struct {
	Network    string
	Domain     string
	Nameserver net.IP
	Interface  string
}

// CommandRunner executes a program without a shell, honoring context cancellation.
// Output is included in diagnostics and used to recognize unavailable resolved.
type CommandRunner func(context.Context, string, ...string) ([]byte, error)

// InterfaceLookup returns the current OS index, or an error wrapping fs.ErrNotExist
// when the interface is gone. Linux uses the index to avoid reverting a replacement
// link that happens to have the same name.
type InterfaceLookup func(string) (int, error)

type Options struct {
	Run             CommandRunner
	LookupInterface InterfaceLookup
	ResolverDir     string        // Empty selects /etc/resolver on macOS.
	Timeout         time.Duration // Zero selects five seconds.
}

// Manager serializes platform operations. It retains Linux cleanup ownership for
// this process; macOS ownership is carried by an exact marker in resolver files.
type Manager struct {
	mu              sync.Mutex
	run             CommandRunner
	lookupInterface InterfaceLookup
	resolverDir     string
	timeout         time.Duration
	platform        platformState
}

// New validates options without running commands or accessing resolver files.
// Unavailable system integration is reported by Apply, not by construction.
func New(
	opts Options,
) (
	*Manager,
	error,
) {
	if opts.Timeout < 0 {
		return nil, fmt.Errorf("%w: negative operation timeout", ErrInvalidConfig)
	}

	if opts.Timeout == 0 {
		opts.Timeout = defaultTimeout
	}

	if opts.ResolverDir == "" {
		opts.ResolverDir = "/etc/resolver"
	} else if !filepath.IsAbs(opts.ResolverDir) {
		return nil, fmt.Errorf("%w: resolver directory must be absolute", ErrInvalidConfig)
	}

	if opts.Run == nil {
		opts.Run = runCommand
	}

	if opts.LookupInterface == nil {
		opts.LookupInterface = lookupInterface
	}

	return &Manager{
		run:             opts.Run,
		lookupInterface: opts.LookupInterface,
		resolverDir:     opts.ResolverDir,
		timeout:         opts.Timeout,
	}, nil
}

func (m *Manager) Apply(
	ctx context.Context,
	config Config,
) error {
	if err := validateConfig(config); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return err
	}

	return m.apply(ctx, config)
}

// Remove is idempotent. Disappeared or disowned resources are already released;
// failed removal of owned state remains retryable. Unrelated state is untouched.
func (m *Manager) Remove(
	ctx context.Context,
	config Config,
) error {
	if err := validateConfig(config); err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return err
	}

	return m.remove(ctx, config)
}

// CleanupStale removes marked macOS files for domains outside the caller's saved
// enabled-network set. Linux does not infer interface ownership from domain names;
// active links are repaired by Apply, and resolved forgets removed interfaces.
func (m *Manager) CleanupStale(
	ctx context.Context,
	activeDomains map[string]bool,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	if err := ctx.Err(); err != nil {
		return err
	}

	return m.cleanupStale(ctx, activeDomains)
}

func validateConfig(
	config Config,
) error {
	if err := validateNetworkName(config.Network); err != nil {
		return fmt.Errorf("%w: invalid network name: %v", ErrInvalidConfig, err)
	}

	if config.Domain != config.Network+".internal" {
		return fmt.Errorf("%w: domain must match network.internal", ErrInvalidConfig)
	}

	if config.Nameserver.To16() == nil {
		return fmt.Errorf("%w: nameserver must be an IP address", ErrInvalidConfig)
	}

	if config.Interface == "" || strings.ContainsAny(config.Interface, "\x00/\n\r\t ") {
		return fmt.Errorf("%w: actual interface name required", ErrInvalidConfig)
	}

	return nil
}

// validateNetworkName checks that name is one valid lowercase DNS label:
// ASCII lowercase letters, digits, and hyphens only, with the first
// and last characters being a letter or digit. Input is rejected
// rather than silently lowercased.
func validateNetworkName(
	name string,
) error {
	if name == "" {
		return fmt.Errorf("name is empty")
	}

	if len(name) > dnsLabelMaxBytes {
		return fmt.Errorf("name %q exceeds %d byte DNS label limit", name, dnsLabelMaxBytes)
	}

	if name[0] == '-' {
		return fmt.Errorf("name %q must not start with a hyphen", name)
	}

	if name[len(name)-1] == '-' {
		return fmt.Errorf("name %q must not end with a hyphen", name)
	}

	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z':
		case c >= '0' && c <= '9':
		case c == '-':
		default:
			return fmt.Errorf("name %q may contain only lowercase letters, digits, and hyphens", name)
		}
	}

	return nil
}

func runCommand(
	ctx context.Context,
	name string,
	args ...string,
) (
	[]byte,
	error,
) {
	command := exec.CommandContext(ctx, name, args...)
	command.WaitDelay = time.Second
	output, err := command.CombinedOutput()

	if ctx.Err() != nil {
		return output, ctx.Err()
	}

	return output, err
}

func lookupInterface(
	name string,
) (
	int,
	error,
) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return 0, err
	}

	for _, iface := range interfaces {
		if iface.Name == name {
			return iface.Index, nil
		}
	}

	return 0, fmt.Errorf("interface %q: %w", name, fs.ErrNotExist)
}
