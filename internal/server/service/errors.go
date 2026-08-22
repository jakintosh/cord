package service

import (
	"errors"
	"strings"
)

var (
	ErrNotFound                     = errors.New("resource not found")
	ErrConflict                     = errors.New("resource already exists")
	ErrNetworkExists                = errors.New("network already exists")
	ErrInvalidInput                 = errors.New("invalid input")
	ErrCIDROverlap                  = errors.New("CIDR overlap with existing range")
	ErrRegistrationAddressExhausted = errors.New("no invite addresses available for registration")
	ErrRegistrationExpired          = errors.New("registration has expired")
	ErrRegistrationRedeemed         = errors.New("registration already redeemed")
	ErrPeerNotConfirmed             = errors.New("peer not yet confirmed")
	ErrNetworkEnabled               = errors.New("network is enabled; disable it first")
	ErrNotImplemented               = errors.New("not implemented")
)

// mapStoreError translates store-level sentinel errors into
// service-level sentinel errors. It also normalizes conflict errors
// into ErrNetworkExists when the context mentions "network".
func mapStoreError(
	err error,
) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, ErrNotFound) {
		return ErrNotFound
	}

	if errors.Is(err, ErrConflict) {
		if strings.Contains(err.Error(), "network") {
			return ErrNetworkExists
		}
		return ErrConflict
	}

	// The database adapter wraps SQLite unique constraint violations
	// as a fmt.Errorf chain that doesn't use errors.Is with ErrConflict.
	// Check the error message for unique constraint patterns.
	errStr := err.Error()
	if strings.Contains(errStr, "UNIQUE constraint failed") {
		if strings.Contains(errStr, "network.name") {
			return ErrNetworkExists
		}
		return ErrConflict
	}

	return err
}
