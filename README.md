# Cord

Cord is a wireguard configuration manager and virtual network orchestrator. It aims to provide a small coordination server and complementary CLI for managing WireGuard networks built around the concept of "cords". A cord describes a network's address ranges, the peers that may join, and how those peers may communicate.

## Project status

The code base is a work in progress. Many commands and functions exist only as stubs or contain TODOs. The repository is useful for understanding the intended design and for running the existing unit tests, but it is **not yet a complete implementation**.

## Architecture

The system is split into two binaries:

* **`cord-server`** – manages network state, stores it in SQLite, and exposes administration commands. The server tracks CIDR ranges, peer identities, and associations between ranges. It is designed to run on a coordination host.
* **`cord`** – client/administrative tool. It installs peers from invites, fetches updated state, and can control WireGuard interfaces on local machines. Many subcommands are placeholders intended to eventually call the server's HTTP API.

Both programs share internal packages under `internal/`:

* `internal/server` – core network management logic used by the server CLI.
* `internal/client` – placeholder logic for client operations.
* `internal/database` – wrappers around SQLite for creating and manipulating the persistence layer.
* `internal/wireguard` – helpers for generating WireGuard device and peer configs and interacting with the OS' WireGuard implementation.
* `internal/utils` – miscellaneous helpers for IP/CIDR manipulation and validation.

### Contexts, Config and Data

Server operations run through a `Context` that bundles the network name, database handle, configuration writer, and data location. Two implementations exist for both configuration storage (`FsConfig` for filesystem and `MemConfig` for in-memory) and data storage (`FsData` for files and `MemData` for in-memory), allowing tests to use memory while the CLI uses disk storage.

### Database schema

The coordination server stores state in SQLite. `initNetworkDb` creates five primary tables:

* **`cidr`** – Named CIDR blocks belonging to the network. Each row stores the textual CIDR, prefix/length, and the numeric range.
* **`association`** – Pairs of CIDR IDs that are allowed to communicate. Associations are symmetric.
* **`invite`** – Pending peer invitations containing a temporary public key, temporary (invite network) cidr, assigned permanent cidr, peer name, admin flag, redemption status, and expiration timestampe. Invites are temporary records deleted after successful peer confirmation.
* **`peer`** – Peers tied to a single CIDR with their public key and flags for admin, confirmed, and enabled status.
* **`endpoint`** – Historical peer endpoint sightings with timestamps and witness. Used for future endpoint gossip and detection of peer changes.

### CIDR management

Networks start with a *root CIDR* (row `id=1` in the `cidr` table). Sub-CIDRs may be created, renamed or removed. `CreateCidr` ensures new CIDRs fall within the root range. CIDRs may be associated to permit communication across ranges.

### Peer lifecycle

Peers join the network through a temporary wireguard interface. `CreatePeer` generates a an invite record with the peer's assigned temporary and permanent IP, name, and permissions. It is also given a temporary wireguard key pair. The invite data as packaged into a file and delivered out-of-band to the client.

The client begins the redemption process using the invite file to connect to the server's "invite" wireguard network, then generates their own WireGuard keypair locally and redeems the invite by POSTing their public key to the redemption endpoint. Upon successful redemption, the server creates a peer record (marked as unconfirmed), returns the complete WireGuard configuration, and marks the invite as redeemed. 

The client then configures their permanent WireGuard interface and establishes a connection to confirm their presence on the network via the `/api/v1/peer/confirm` endpoint. Only after this confirmation step is the peer marked as operational and the invite record deleted.

Peers can be renamed (through CIDR renaming) or enabled/disabled. The server computes peer visibility by resolving associated CIDR ranges and collecting all confirmed (`confirmed=1`) and enabled (`disabled=0`) peers within them.

### WireGuard configuration

`internal/wireguard` defines types for representing devices and peers. `DeviceConfig` describes the local interface (private key, internal CIDR, listen port) and can be written to an `io.Writer`. `PeerConfig` will eventually write out full invite files combining device and server information (not yet implemented). Helper functions exist for key generation and interacting with the OS via `wgctrl`/`netlink`.

### Client operations

The client package is largely aspirational. Functions like `Install`, `Fetch`, `Up`, and `Sync` outline how a peer should consume an invite, generate a key pair, redeem it with the server, maintain a local database of peer state, and configure a WireGuard interface. Extensive comments describe plans for a state log and endpoint gossip mechanism.

## Command line usage

The Makefile builds both binaries:

```bash
make client   # builds ./bin/cord
make server   # builds ./bin/cord-server
```

Some example server commands (many rely on stubbed functionality):

```bash
cord-server add-network <name> <cidr> <external-ip> <port>
cord-server add-cidr <network> <cidr-name> <cidr>
cord-server add-peer <network> <peer-name> <ip>
cord-server get-peers <network> <peer-name>
```

The `cord` binary is intended for clients and administrators. Planned subcommands include installing from an invite, fetching state, and bringing interfaces up or down.

Both binaries accept `--config-dir` and `--data-dir` to override the default paths (`/etc/cord*` and `/var/lib/cord*`).

## Testing

Run the existing Go tests with:

```bash
go test ./...
```

Only the utility and server packages currently contain tests.

## Roadmap / TODOs

The code includes many TODOs and design notes outlining future work. Highlights include:

* Implement `PeerConfig.WriteInvite` to emit full invite files.
* Have `CreateNetwork` write the server's WireGuard config.
* Notify peers when others are redeemed or disabled.
* Flesh out the client-side workflow for redeeming invites, polling state, managing WireGuard interfaces, and syncing endpoint sightings.
* Add an HTTP API for server operations so the `cord` client can manage networks remotely.
* Develop a state log and endpoint gossip mechanism for tracking peer endpoints and sharing updates efficiently.
