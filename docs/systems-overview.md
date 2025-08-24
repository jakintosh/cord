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

    Using the structured config file approach, here's how WireGuard network information is persisted for discovery:

    **Server persistence:** During `CreateNetwork()`, the server writes a TOML config file via `Config.GetConfigWriter()` containing its interface parameters:
    ```toml
    [interface]
    private_key = "server_generated_key"
    internal_cidr = "10.0.0.1/16"
    listen_port = 51820

    [server]
    external_ip = "203.0.113.10"
    external_port = 51820

    [network]
    name = "example-network"
    root_cidr = "10.0.0.0/16"
    ```

    **Invite generation:** When `CreatePeer()` generates invites, it reads the server's config file to embed server discovery information in the invite file:
    ```toml
    [server_peer]
    public_key = "server_public_key"
    external_endpoint = "203.0.113.10:51820"
    internal_endpoint = "10.0.0.1:8080"
    allowed_ips = ["10.0.0.0/16"]

    [client_interface]
    private_key = "temporary_key_from_server"
    internal_cidr = "10.0.1.42/16"

    [network]
    name = "example-network"
    ```

    **Client persistence:** During `Install()`, the client reads the invite, generates its permanent keypair, redeems with the server, then writes its own config file combining invite data with the permanent keys:
    ```toml
    [interface]
    private_key = "client_generated_permanent_key"
    internal_cidr = "10.0.1.42/16"
    listen_port = 0

    [server_peer]
    public_key = "server_public_key"
    external_endpoint = "203.0.113.10:51820"
    internal_endpoint = "10.0.0.1:8080"
    allowed_ips = ["10.0.0.0/16"]

    [network]
    name = "example-network"
    ```

    **Ongoing operations:** Both server and client operations (`Up()`, `Fetch()`, etc.) read their respective config files to get WireGuard interface parameters and discovery information. The server reads its config to know how to present itself; clients read their config to know how to contact the server and configure their local interface.

    This unified approach uses the existing `Config` abstraction for both sides, handles multi-network scenarios through separate config files per network, and provides a single source of truth for WireGuard interface configuration that's separate from but complementary to the SQLite-stored network topology.

- Network initialization accepts one CIDR, but ADR‑001 requires both main and invite CIDRs. Should the initialization flow capture two ranges?

    The initialization should indeed capture two ranges; one for the real network interface, and one for the invite interface.

- The schema's `peer` table lacks a `name` column even though flows reference peer names and renaming. Should peer names live in `peer`, in `cidr`, or elsewhere?

    The canonical name for a `peer` is in its associated `cidr`. `cidr` has names, and `peer` is just supplementary data that expands a `cidr` into a peer.

- During CIDR deletion, what happens to peers and nested CIDRs bound to that range?

    Peers and nested CIDRs are unaltered; this allows flexibility for an admin to resegment peers later on, since peer IPs are immutable.

- What rollback or cleanup strategy is expected if network initialization or other server flows fail midway?

    If initial network initialization fails on the server, we should just clean it all up and surface an error to the admin.

- How will the HTTP API authenticate and identify administrators versus regular peers?

    The server will read the sending IP address from the request, and look up that peer in the database to check their `admin` flag. Since wireguard takes care of cryptographically validating IP, we can use that as our auth layer.

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

    The client needs to immediately persist its keypair, so that it maintain idempotency when retrying if it runs into network isses during the API calls in the redemption flow. However, this won't live in the database, but in a `.toml` file. Once the peer recieves a response from the "/redeem" endpoint, that is when we've gotten far enough along to be ready to actually begin building out persistence for the network data, including creating the database for that network. If the server has OK'd the redemption, there's not a categorical error that can happen in the confirmation stage, so it's safe to assume the network is ready for persistence.

- If installation fails after generating a permanent keypair and attempting redemption, can the invite be retried or should partial artifacts be preserved?

    The invite redemption can be retried, but it must use the same keypair since the server does not allow updating them. So, if you call "/redeem" and get some kind of network issue, we can't be certain that the server did *not* get the key pair, and so any further calls to "/redeem" must use the same public key. If the server returns some kind of *error* from "/redeem" though, the error may signify that the redemption is invalid (for example, if the invite is expired), and we might want to actually clean up the partial artifacts.

- What HTTP request/response formats should the client use for redeem, confirm, fetch and admin operations?

    We will worry about this later, when we develop the API. Right now, we can focus on building out the features for the CLI, which is the primary interface.

- What cadence should periodic state fetches and endpoint syncs follow?

    I think this is an open question that we can leave open as a configuration option. Different network needs may have different correct answers.

- During redemption, does the client tear down the temporary interface before or after configuring the permanent one, and how is failure handled in between?

    The client tears down the temporary interface only after the permanent configuration has been confirmed by the server. So, the flow would be: temporary interface up -> redeem -> permanent interface up -> confirm -> termporary interface down.

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

