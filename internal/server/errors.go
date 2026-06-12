package server

import "errors"

// Sentinel errors for the service surface. Store implementations wrap
// these so callers (the API in particular) can translate outcomes
// without inspecting error strings.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("conflict")
	ErrInvalid  = errors.New("invalid")
)
