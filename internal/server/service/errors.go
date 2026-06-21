package service

import "errors"

var (
	ErrNotFound          = errors.New("resource not found")
	ErrConflict          = errors.New("resource already exists")
	ErrNetworkExists     = errors.New("network already exists")
	ErrInvalidInput      = errors.New("invalid input")
	ErrCIDROverlap       = errors.New("CIDR overlap with existing range")
	ErrInviteExpired     = errors.New("invite has expired")
	ErrInviteRedeemed    = errors.New("invite already redeemed")
	ErrPeerNotConfirmed  = errors.New("peer not yet confirmed")
	ErrNetworkRunning    = errors.New("network is already running")
	ErrNetworkNotRunning = errors.New("network is not running")
	ErrNotImplemented    = errors.New("not implemented")
)
