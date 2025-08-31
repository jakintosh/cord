# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Development Commands

Build commands:
- `make client` - builds `./bin/cord` (client CLI)
- `make server` - builds `./bin/cord-server` (coordination server)
- `make all` - builds both binaries
- `make test` or `go test ./...` - runs all unit tests

Test specific packages: `go test ./internal/server -v`

## Architecture Overview

Cord is a WireGuard configuration manager with two main binaries:

- **cord-server**: coordination server that manages network state in SQLite and exposes administration commands
- **cord**: client/administrative tool for peer management and WireGuard interface control

### Key Internal Packages

- `internal/server` - core network management logic, CIDR handling, peer lifecycle
- `internal/database` - SQLite persistence layer with schema for cidrs, peers, invites, associations, endpoints
- `internal/wireguard` - WireGuard key generation, device/peer configuration, OS integration
- `internal/api` - HTTP API endpoints for peer redemption, confirmation, and admin operations
- `internal/client` - client-side operations (largely aspirational/stubbed)
- `internal/utils` - IP/CIDR manipulation utilities

### Core Concepts

- **Networks** start with a root CIDR, sub-CIDRs can be created and associated
- **Peers** join via invite redemption: temporary WG interface → key exchange → permanent configuration → confirmation
- **Context pattern**: operations use a Context bundling network name, DB handle, config writer, data location
- **Storage abstractions**: FsConfig/MemConfig for configuration, FsData/MemData for data (enables in-memory testing)

## Database Schema

SQLite tables managed by server:
- `cidr` - named CIDR blocks with numeric ranges
- `association` - symmetric communication permissions between CIDRs
- `invite` - temporary peer invitations with temp keys, assignments, expiration
- `peer` - confirmed peers with public keys, admin/enabled flags
- `endpoint` - historical peer endpoint sightings for gossip

## Development Notes

- Uses Go 1.24+ with tabs for indentation
- Many client functions are stubs with TODO comments
- Server operations override paths with `--config-dir` and `--data-dir`
- Tests use in-memory storage, production uses filesystem
- **Security**: Never commit WireGuard keys, invite payloads, or real endpoints
