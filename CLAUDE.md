# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

Build commands:
- `make all` (or `make cord`) - builds `./bin/cord`
- `make test` or `go test ./...` - runs all unit tests
- `sudo make test-integration` - creates real WireGuard interfaces (root required)

Test specific packages: `go test ./internal/server -v`

## Architecture Overview

Cord is a WireGuard configuration manager shipped as a single `cord` binary with two top-level subcommands:

- **cord server**: coordination server that manages network state in SQLite and exposes administration commands (defaults to `/etc/cord-server` and `/var/lib/cord-server`)
- **cord client**: client/administrative tool for peer management and WireGuard interface control (defaults to `/etc/cord` and `/var/lib/cord`)

### Key Internal Packages

- `internal/server` - core network management logic, CIDR handling, peer lifecycle
- `internal/database` - SQLite persistence layer with schema for cidrs, peers, invites, associations, endpoints
- `internal/wireguard` - WireGuard key generation, device/peer configuration, OS integration
- `internal/api` - HTTP API endpoints for peer redemption, confirmation, and admin operations
- `internal/client` - client-side flows: install/up/down/show/fetch/uninstall, local peer DB, API client, remote admin
- `internal/utils` - IP/CIDR manipulation utilities

### Core Concepts

- **Networks** start with a root CIDR, sub-CIDRs can be created and associated
- **Dual networks** (ADR-001): the server runs a main interface and an invite-only interface; the invite network exposes only the redeem endpoint
- **Peers** join via invite redemption: temporary WG interface → redeem (client-generated permanent key) → permanent configuration → confirmation
- **Context pattern**: operations use a Context bundling network name, config store, and DB-backed ServerStore
- **Storage abstractions**: FsConfig/MemConfig for configuration, SQLite on disk or in-memory for data (enables in-memory testing)
- **File formats**: all cord files on disk are TOML via BurntSushi/toml (server network config, invite files, client config); the HTTP API speaks JSON
- **Platforms**: Linux (kernel WireGuard via netlink/wgctrl) and macOS (userspace wireguard-go, devices named utunN); no Windows support

## Database Schema

SQLite tables managed by server:
- `cidr` - named CIDR blocks with numeric ranges
- `association` - symmetric communication permissions between CIDRs
- `invite` - temporary peer invitations with temp keys, assignments, expiration
- `peer` - peers with public keys and admin/enabled/confirmed flags (unconfirmed = redeemed but not yet confirmed)
- `endpoint` - historical peer endpoint sightings for gossip

## Development Notes

- Uses Go 1.24+ with tabs for indentation
- Documentation can be found in the `docs/` folder; if implementation changes are made that conflict with the docs, make sure to update the relevant documentation at the same time.
- Server operations override paths with `--config-dir` and `--data-dir`
- Tests use in-memory storage, production uses filesystem
- `cord client up` and `cord server serve` run in the foreground; on macOS the userspace interface lives and dies with the process
- **Security**: Never commit WireGuard keys, invite payloads, or real endpoints

## Test Design Philosophy

- **Readable, focused tests**: Each test function should test exactly one behavior/scenario with a clear name that immediately indicates what failed
- **Reusable building blocks**: Common test setup, assertion helpers, and test data should be extracted to shared functions (see `database_test.go` pattern)
- **Single assertions per test**: Avoid sub-tests with anonymous structs testing multiple cases - create separate test functions instead
- **Helper functions**: Mark setup/assertion functions with `t.Helper()` for better error reporting
