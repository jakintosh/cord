package service

import "errors"

var (
	ErrNotFound             = errors.New("resource not found")
	ErrConflict             = errors.New("resource already exists")
	ErrNetworkExists        = errors.New("network already installed")
	ErrNetworkNotInstalled  = errors.New("network not installed")
	ErrNetworkEnabled       = errors.New("network already enabled")
	ErrNetworkNotEnabled    = errors.New("network not enabled")
	ErrInvalidInput         = errors.New("invalid input")
	ErrNotImplemented       = errors.New("not implemented")
	ErrWireGuardUnavailable = errors.New("wireguard backend not configured")
)
