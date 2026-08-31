// Package server provides a typed client for the Cord server daemon's local
// administration API.
//
// The API is served over a Unix socket. The daemon creates that socket with
// mode 0660 by default, so access is authorized through its owner and group.
// Non-successful HTTP responses retain command-go wire error semantics.
package server
