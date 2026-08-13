package service

import "errors"

var (
	ErrNotFound             = errors.New("resource not found")
	ErrConflict             = errors.New("resource already exists")
	ErrInstallState         = errors.New("install state does not permit operation")
	ErrNetworkExists        = errors.New("network already installed")
	ErrNetworkNotEnabled    = errors.New("network not enabled")
	ErrInvalidInput         = errors.New("invalid input")
	ErrTopologyUnavailable  = errors.New("topology unavailable before first sync")
	ErrWireGuardUnavailable = errors.New("wireguard backend not configured")
)
