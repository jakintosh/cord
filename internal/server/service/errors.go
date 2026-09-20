package service

import "errors"

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
// service-level sentinel errors. Operation-specific classifications belong
// to the owning service method, never to diagnostic text.
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
		return ErrConflict
	}

	return err
}
