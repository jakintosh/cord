# System Overview (Revised)

## Context & Goals / Non-Goals

Cord manages WireGuard networks through a coordination server that tracks network topology (CIDRs, associations, peers) and distributes configuration to clients. Success means peers can join via invites, discover allowed peers based on CIDR associations, and maintain connectivity through endpoint gossip. The system explicitly does not handle certificate management, provides no data plane functionality beyond WireGuard itself, and leaves invite file delivery to administrators.

## Updated System Sketch (Text)

The system consists of two binaries: `cord-server` manages network state in SQLite and exposes CLI/HTTP administration, while `cord` handles client operations and remote administration. Both share internal packages for server logic (`internal/server`), client operations (`internal/client`), database wrappers (`internal/database`), WireGuard helpers (`internal/wireguard`), and utilities (`internal/utils`). The server maintains two WireGuard interfaces per network (main on port 51820, invite on port 51821) with configuration stored in TOML files under `$CONFIG_DIR/<network>/` and topology in SQLite at `$DATA_DIR/<network>.db`. Clients connect first to the invite network for redemption, then transition to the main network where they access the full API authenticated by WireGuard-validated source IPs.

## Components

### Server Application (`cord-server`)

Manages network state and coordinates peers through CLI commands and a future HTTP API. Initializes networks with two CIDR ranges (main and invite), creates WireGuard interfaces for each, and persists configuration in TOML files while storing topology in SQLite. The server writes its own WireGuard parameters and external endpoints to `$CONFIG_DIR/<network>/server.toml` during initialization, making this information available for invite generation. When creating invites, it assigns IPs from the invite network pool, generates temporary keypairs, and writes invite files containing both networks' connection details. The redemption flow transitions peers from invite to main network atomically using database transactions. Failures during network initialization trigger full rollback including config file cleanup; redemption failures preserve the invite for retry unless expired.

**Invariants:** Root CIDR always has id=1; only confirmed peers appear in state queries.

### Client Application (`cord`)

Installs networks from invites, maintains local peer state, and manages WireGuard interfaces. During installation, generates a permanent keypair immediately persisted to `$CONFIG_DIR/<network>/client.toml` for idempotency, connects to the invite network with temporary credentials, redeems the invite with the permanent public key, transitions to the main network, and confirms presence before tearing down the invite interface. Network state fetches update the local SQLite database and regenerate WireGuard configuration in place. Remote administration commands will eventually proxy to the server's HTTP API using the peer's WireGuard-authenticated identity. Network failures during redemption preserve the permanent keypair for retry; server errors indicating invalid invites trigger cleanup of partial state.

**Invariants:** Permanent keypair never changes once generated; invite interface exists only during redemption.

### Persistence Layer

Abstracts storage through `Context` objects supporting both filesystem (`FsConfig`/`FsData`) and in-memory (`MemConfig`/`MemData`) implementations. Server persists WireGuard parameters in TOML config files: `server.toml` contains private key, listen ports, and external endpoints; invite files embed temporary credentials and server discovery information; clients write their permanent configuration to `client.toml`. Network topology lives in SQLite with `cidr` (named ranges with numeric bounds), `association` (symmetric CIDR pairs), `invite` (temporary records with both network CIDRs), `peer` (confirmed peers tied to CIDRs), and `endpoint` (witnessed peer endpoints with timestamps). Database operations use transactions for atomicity; config file writes verify permissions before database changes.

**Invariants:** Network name determines both config subdirectory and database filename; peer names canonically stored in CIDR table.

### WireGuard Configuration Layer

Generates device and peer configurations, manages keypairs, and interfaces with the OS via `wgctrl`/`netlink`. `DeviceConfig` describes local interfaces (private key, internal CIDR, listen port) while `PeerConfig` represents remote peers. During network initialization, generates the server's keypair and writes full interface configuration. For invites, creates temporary keypairs and packages connection details into TOML files with server endpoint, temporary private key, and assigned IPs for both networks. Configuration can be regenerated from persisted TOML files and database state without data loss.

**Invariants:** Each peer has exactly one CIDR; configurations are idempotent to write.

### Endpoint Gossip

Tracks peer endpoint changes through periodic interface scanning and server synchronization. Clients detect endpoint changes by comparing WireGuard interface state against local database, record witnessed endpoints with timestamps, and sync observations to the server. The server stores sightings in the `endpoint` table with witness and target peer IDs, includes most recent endpoints in state responses, and expires sightings after a configurable period (default 24 hours). Peers unable to connect may request full endpoint history for specific peers. Multiple conflicting endpoints are expected and stored as different network paths may observe different addresses.

**Invariants:** Only confirmed peers can report or receive endpoint updates; timestamps always increase monotonically.

## Data Model (Concise)

The server maintains network topology in SQLite with TOML config files for WireGuard parameters. Primary keys are auto-incrementing integers except for special cases (root CIDR has id=1). Critical uniqueness constraints exist on CIDR names/ranges, peer public keys, and the base/prefix combination for CIDRs. IP addresses stored as blobs to support both IPv4 and IPv6.

| Table | Primary Key | Unique Constraints | Key Indexes |
|-------|-------------|-------------------|-------------|
| cidr | id (root=1) | name, cidr, (base,prefix) | prefix, base, last |
| peer | id | cidr, public_key | confirmed, enabled |
| invite | id | public_key, temp_cidr, final_cidr | redeemed, expiration |
| association | id | - | cidr1, cidr2 |
| endpoint | id | - | peer, witness, time |

## Operations Basics

WireGuard operations use 30-second timeouts with exponential backoff starting at 1 second. Idempotency achieved through: database uniqueness constraints for creates, version-checking for updates, and permanent keypair persistence for invite redemption. The server logs all administrative actions with requesting peer identity; clients log state changes and endpoint discoveries. Configuration applied by reading TOML files on startup; schema migrations run automatically on database open. No distributed locking needed as SQLite provides serialization through its single-writer model.

## Open Items (Short)

- HTTP API request/response formats for redemption and state endpoints - define JSON schemas during API implementation
- Periodic fetch and endpoint sync intervals - expose as CLI flags with sensible defaults
- OS-specific routing implementation - create interface abstraction when expanding beyond Linux

## Assumptions

None.

## Notes

The two-network design (main + invite) provides complete isolation during the redemption flow, preventing untrusted invites from accessing production endpoints. Peer names being stored in the CIDR table rather than peer table reflects that a peer is fundamentally an extension of a CIDR with additional WireGuard metadata. Authentication via WireGuard-validated source IPs eliminates need for separate auth tokens as WireGuard already provides cryptographic peer identity.
