package dns

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
)

const ownershipMarker = "# Managed by Cord DNS resolver v1.\n" +
	"# Cord may overwrite or remove this file while this marker is present.\n" +
	"# Remove or modify this marker to make Cord disown the file.\n"

type platformState struct{}

func (m *Manager) apply(
	ctx context.Context,
	config Config,
) error {
	if err := os.MkdirAll(m.resolverDir, 0755); err != nil {
		return fmt.Errorf("create resolver directory: %w", err)
	}

	root, err := os.OpenRoot(m.resolverDir)
	if err != nil {
		return err
	}
	defer root.Close()

	content := resolverContent(config)
	existing, err := readResolver(root, config.Domain)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		break // No existing resolver needs an ownership check.

	case err == nil:
		if !strings.HasPrefix(existing, ownershipMarker) {
			return fmt.Errorf("%w: %s", ErrConflict, config.Domain)
		}
		if existing == content {
			return nil
		}

	default:
		return err
	}

	// The replacement is created in the same directory as its destination.
	temporary := ".cord-" + rand.Text()
	file, err := root.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer root.Remove(temporary)

	_, writeErr := io.WriteString(file, content)
	closeErr := file.Close()

	if err := errors.Join(writeErr, closeErr); err != nil {
		return err
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	// Recheck ownership immediately before replacement so an intervening
	// user edit that disowns the destination is respected.
	current, err := readResolver(root, config.Domain)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		break // The destination is still available for creation.

	case err == nil:
		if !strings.HasPrefix(current, ownershipMarker) {
			return fmt.Errorf("%w: %s", ErrConflict, config.Domain)
		}

	default:
		return err
	}
	return root.Rename(temporary, config.Domain)
}

func (m *Manager) remove(
	ctx context.Context,
	config Config,
) error {
	root, err := os.OpenRoot(m.resolverDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil

	case err != nil:
		return err
	}
	defer root.Close()

	return removeOwnedResolver(ctx, root, config.Domain)
}

func (m *Manager) cleanupStale(
	ctx context.Context,
	activeDomains map[string]bool,
) error {
	root, err := os.OpenRoot(m.resolverDir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil

	case err != nil:
		return err
	}
	defer root.Close()

	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return err
	}

	var errs []error
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}

		name := entry.Name()
		if activeDomains[name] {
			continue
		}

		if err := removeOwnedResolver(ctx, root, name); err != nil {
			errs = append(errs, fmt.Errorf("remove stale resolver %s: %w", name, err))
		}
	}
	return errors.Join(errs...)
}

func resolverContent(
	config Config,
) string {
	return ownershipMarker +
		"nameserver " + config.Nameserver.String() + "\n" +
		"port 53\n" +
		"domain " + config.Domain + "\n" +
		"search " + config.Domain + "\n"
}

func readResolver(
	root *os.Root,
	name string,
) (
	string,
	error,
) {
	info, err := root.Lstat(name)
	if err != nil {
		return "", err
	}

	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%w: %s is not a regular file", ErrConflict, name)
	}

	file, err := root.Open(name)
	if err != nil {
		return "", err
	}
	defer file.Close()

	// Only the marker and a small canonical file matter. Do not read an
	// arbitrarily large unrelated file into memory during startup cleanup.
	content, err := io.ReadAll(io.LimitReader(file, 64*1024))

	return string(content), err
}

func removeOwnedResolver(
	ctx context.Context,
	root *os.Root,
	name string,
) error {
	content, err := readResolver(root, name)
	switch {
	case errors.Is(err, fs.ErrNotExist),
		errors.Is(err, ErrConflict):
		return nil

	case err != nil:
		return err
	}

	if !strings.HasPrefix(content, ownershipMarker) {
		return nil
	}

	if err := ctx.Err(); err != nil {
		return err
	}

	err = root.Remove(name)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil

	default:
		return err
	}
}
