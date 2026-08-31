// Package daemon provides the shared process boundary for cord daemons.
package daemon

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

// ErrAlreadyRunning reports that an active daemon already owns the socket.
var ErrAlreadyRunning = errors.New("daemon already running")

const (
	// DefaultSocketMode grants administration access to the daemon user and
	// members of the socket's owning group.
	DefaultSocketMode os.FileMode = 0660

	socketProbeTimeout = 250 * time.Millisecond
)

// ParseSocketMode parses one of cord's supported control-socket policies.
func ParseSocketMode(
	value string,
) (
	os.FileMode,
	error,
) {
	parsed, err := strconv.ParseUint(value, 8, 9)
	if err != nil {
		return 0, fmt.Errorf("invalid socket mode %q", value)
	}

	mode := os.FileMode(parsed)
	if !supportedSocketMode(mode) {
		return 0, fmt.Errorf("socket mode must be 0600, 0660, or 0666")
	}
	return mode, nil
}

// ListenUnix claims path as a local control socket. An active socket
// is never replaced; an unreachable stale socket is removed. Existing paths
// that are not Unix sockets are left untouched. A missing parent directory is
// created with protected runtime-directory permissions.
func ListenUnix(
	path string,
	mode os.FileMode,
) (
	net.Listener,
	error,
) {
	if path == "" {
		return nil, errors.New("Unix socket path required")
	}
	if mode == 0 {
		mode = DefaultSocketMode
	}
	if !supportedSocketMode(mode) {
		return nil, fmt.Errorf("socket mode must be 0600, 0660, or 0666")
	}
	if err := prepareSocketDirectory(path); err != nil {
		return nil, err
	}
	if err := prepareSocketPath(path); err != nil {
		return nil, err
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, fmt.Errorf("listen on Unix socket %q: %w", path, err)
	}

	if err := os.Chmod(path, mode); err != nil {
		_ = ln.Close()
		return nil, fmt.Errorf("set Unix socket permissions on %q: %w", path, err)
	}

	return ln, nil
}

func supportedSocketMode(
	mode os.FileMode,
) bool {
	return mode == 0600 || mode == 0660 || mode == 0666
}

func prepareSocketDirectory(
	path string,
) error {
	dir := filepath.Dir(path)
	_, err := os.Stat(dir)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("inspect Unix socket directory %q: %w", dir, err)
	}

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create Unix socket directory %q: %w", dir, err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		return fmt.Errorf("set Unix socket directory permissions on %q: %w", dir, err)
	}
	return nil
}

func prepareSocketPath(
	path string,
) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect Unix socket %q: %w", path, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("control socket path %q exists and is not a Unix socket", path)
	}

	conn, err := net.DialTimeout("unix", path, socketProbeTimeout)
	if err == nil {
		_ = conn.Close()
		return fmt.Errorf("%w at %q", ErrAlreadyRunning, path)
	}
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if !errors.Is(err, syscall.ECONNREFUSED) {
		return fmt.Errorf("check existing Unix socket %q: %w", path, err)
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale Unix socket %q: %w", path, err)
	}
	return nil
}
