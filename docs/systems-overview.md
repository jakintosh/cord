# Systems Overview

This document infers the major systems for Cord based on existing project documentation. Each section outlines responsibilities and surfaces open questions or inconsistencies that need clarification before implementation.

## Server Application (`cord-server`)

The server manages network state and coordinates peers. It exposes administrative commands through the CLI and a future HTTP API.

**Responsibilities**
- Initialize networks and configure two WireGuard interfaces (main and invite).
- Persist CIDRs, peers, invites and associations in SQLite via a `Context` that also manages config and data paths.
- Provide flows for CIDR and association management, peer invitation and redemption, peer state queries and administrative updates.

**Open Questions**
- How are the server's external IP and port persisted so invites and clients can discover them?
- Network initialization accepts one CIDR, but ADR‑001 requires both main and invite CIDRs. Should the initialization flow capture two ranges?
- The schema's `peer` table lacks a `name` column even though flows reference peer names and renaming. Should peer names live in `peer`, in `cidr`, or elsewhere?
- During CIDR deletion, what happens to peers and nested CIDRs bound to that range?
- What rollback or cleanup strategy is expected if network initialization or other server flows fail midway?
- How will the HTTP API authenticate and identify administrators versus regular peers?

## Client Application (`cord`)

The client installs networks, maintains local state and manages WireGuard interfaces for peers and administrators.

**Responsibilities**
- Install a network from an invite: initialize local database, generate keypair, redeem invite and confirm with the server.
- Periodically fetch network state, reconcile the local database and update the WireGuard interface.
- Manage interface lifecycle (`up`, `down`, updates) and optional endpoint watching.
- Uninstall networks by tearing down interfaces and deleting local state.
- Offer remote administrative commands that call the server's HTTP API.

**Open Questions**
- When should the client initialize its local database relative to invite redemption to avoid unrecoverable partial states?
- If installation fails after generating a permanent keypair and attempting redemption, can the invite be retried or should partial artifacts be preserved?
- What HTTP request/response formats should the client use for redeem, confirm, fetch and admin operations?
- What cadence should periodic state fetches and endpoint syncs follow?
- During redemption, does the client tear down the temporary interface before or after configuring the permanent one, and how is failure handled in between?

## Persistence Layer

Both server and client rely on SQLite databases to track network state.

**Responsibilities**
- Server schema defines `cidr`, `association`, `invite`, `peer` and `endpoint` tables.
- Client schema tracks known peers and endpoint sightings locally.
- A `Context` object abstracts filesystem vs. in‑memory storage for configuration and data.

**Open Questions**
- Where are network‑level settings such as external endpoints, listen ports or invite network CIDR stored? No `network` table currently exists.
- The client schema stores peer IPs as integers while the endpoint table uses blobs; should these representations be unified?
- How are invite expirations enforced and expired records cleaned up on the server?
- Are additional tables needed to track server configuration or multiple networks?

## WireGuard Configuration Layer

WireGuard helpers construct device and peer configurations for both server and client.

**Responsibilities**
- Generate key pairs and create `DeviceConfig` and `PeerConfig` structures.
- Write server and client WireGuard configuration files and update interfaces idempotently.
- `WriteInvite` is intended to serialize device and peer configuration into a file delivered to new peers.

**Open Questions**
- What exact format and fields should `WriteInvite` produce, and how are secrets protected during transit?
- How should OS‑specific routing rules be represented and controlled through this layer?
- What mechanism will persist the server's WireGuard configuration so it can be rewritten or restored?

## Endpoint Gossip

Clients share observed peer endpoints so the network can adapt to changing external addresses.

**Responsibilities**
- Watch the local interface for endpoint changes and record sightings in the client's database.
- Sync changed endpoints to the server, which stores sightings and disseminates the newest values during state fetches.

**Open Questions**
- What expiration policy governs endpoint sightings on the server?
- Should clients request historical endpoint candidates when a peer is unreachable, and how is this exposed via the API?
- How are conflicting or malicious endpoint reports mitigated?

