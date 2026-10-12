package dns

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os/exec"
	"strconv"
	"strings"
)

type linkOwnership struct {
	config Config
	index  int
}

type platformState struct {
	links map[string]linkOwnership
}

func (m *Manager) apply(
	ctx context.Context,
	config Config,
) error {
	index, err := m.lookupInterface(config.Interface)
	if err != nil {
		return fmt.Errorf("DNS interface: %w", err)
	}

	if index <= 0 {
		return fmt.Errorf("DNS interface: invalid index %d", index)
	}

	if m.platform.links == nil {
		m.platform.links = make(map[string]linkOwnership)
	}

	prior, owned := m.platform.links[config.Domain]
	if owned && !sameConfig(prior.config, config) {
		return fmt.Errorf("%w: remove previous configuration for %s first", ErrConflict, config.Domain)
	}

	for domain, link := range m.platform.links {
		if domain != config.Domain && link.index == index {
			return fmt.Errorf("%w: interface already belongs to %s", ErrConflict, domain)
		}
	}

	// Retain even an uncertain/partial command outcome for removal. A new
	// index means the old link is gone; resolved has already discarded it.
	config.Nameserver = append([]byte(nil), config.Nameserver...)
	m.platform.links[config.Domain] = linkOwnership{
		config: config,
		index:  index,
	}

	link := strconv.Itoa(index)
	commands := [][]string{
		{"default-route", link, "no"},
		{"domain", link, config.Domain},
		{"dns", link, config.Nameserver.String()},
	}
	for i, args := range commands {
		if err := m.resolvectl(ctx, args...); err != nil {
			// An unavailable integration before any command took effect owns
			// nothing. Existing and partial configurations still need cleanup.
			if i == 0 && !owned && errors.Is(err, ErrUnsupported) {
				delete(m.platform.links, config.Domain)
			}
			return err
		}
	}

	return nil
}

func (m *Manager) remove(
	ctx context.Context,
	config Config,
) error {
	owned, ok := m.platform.links[config.Domain]
	if !ok {
		return nil
	}

	if !sameConfig(owned.config, config) {
		return fmt.Errorf("%w: configuration differs from owned link", ErrConflict)
	}

	index, err := m.lookupInterface(config.Interface)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		delete(m.platform.links, config.Domain)
		return nil

	case err != nil:
		return fmt.Errorf("DNS interface: %w", err)

	case index != owned.index:
		delete(m.platform.links, config.Domain)
		return nil
	}

	if err := m.resolvectl(ctx, "revert", strconv.Itoa(index)); err != nil {
		return err
	}

	delete(m.platform.links, config.Domain)

	return nil
}

func (m *Manager) cleanupStale(
	ctx context.Context,
	activeDomains map[string]bool,
) error {
	// An active-domain set does not establish ownership of surviving kernel
	// interfaces from another process. Do not scan/revert arbitrary links.
	return ctx.Err()
}

func (m *Manager) resolvectl(
	ctx context.Context,
	args ...string,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	output, err := m.run(ctx, "resolvectl", args...)
	message := strings.TrimSpace(string(output))

	switch {
	case ctx.Err() != nil:
		return ctx.Err()

	case err == nil:
		return nil

	case errors.Is(err, exec.ErrNotFound),
		strings.Contains(message, "org.freedesktop.resolve1 was not provided"),
		strings.Contains(message, "Unit dbus-org.freedesktop.resolve1.service not found"):
		err = errors.Join(ErrUnsupported, err)
	}

	return fmt.Errorf("resolvectl %s: %s: %w", args[0], message, err)
}

func sameConfig(
	a Config,
	b Config,
) bool {
	return a.Network == b.Network && a.Domain == b.Domain &&
		a.Interface == b.Interface && a.Nameserver.Equal(b.Nameserver)
}
