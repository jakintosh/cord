# Cord

Cord is a WireGuard configuration manager and virtual network orchestrator. A single `cord` binary serves both roles: `cord server` runs a small coordination server that tracks a network's address ranges, peers, and communication rules in SQLite; `cord client` joins networks from invite files and keeps a local WireGuard interface in sync with the network. A "cord" is one such network: its CIDRs, the peers that may join, and which subnets may talk to each other.

Cord began as a Go rewrite and evolution of [tonarino/innernet](https://github.com/tonarino/innernet).

Linux (kernel WireGuard) and macOS (userspace via wireguard-go) are supported.

## How it works

- **Networks** are created with a root CIDR. Sub-CIDRs can be carved out and *associated* with each other; peers may only see peers in their own (most specific) CIDR and any associated CIDRs.
- **The server runs two WireGuard interfaces** ([ADR-001](docs/adrs/001-separate-invite-network-redemption.md)): the main network, and a separate *invite network* used only for redeeming invites. Untrusted invitees never touch the main network.
- **Peers join via invites.** An admin mints an invite file (a temporary keypair and IP on the invite network). The client connects to the invite network, generates its own permanent keypair, and redeems the invite for its main-network assignment. It then connects to the main network and *confirms*, proving the transition worked, at which point the invite is destroyed and the peer is operational.
- **Authentication is WireGuard itself.** The HTTP API is only reachable over the tunnel; the server maps the source IP — cryptographically bound to a peer key by WireGuard — to a peer record. Admin endpoints additionally require the peer's `admin` flag.
- **Endpoint gossip.** Connected clients watch their live WireGuard sessions; when a peer's endpoint changes (e.g. it roamed networks), they report the sighting. The server folds recent sightings into peer listings so everyone can find roaming peers again. Sightings expire after 24h.

## Building

```bash
make all      # builds ./bin/cord
make test     # unit tests (run anywhere, no privileges needed)
sudo make test-integration   # creates real WireGuard interfaces
```

## Running a network

On the coordination host:

```bash
# create a network: name, root CIDR, public IP, WireGuard port
cord server add-network homenet 10.42.0.0/16 198.51.100.7 51820

# mint an invite for a peer (writes ./alice.invite.toml)
cord server add-peer homenet alice 10.42.0.10

# serve (foreground): brings up both WireGuard interfaces + the API
cord server serve homenet
```

`add-network` options: `--invite-cidr` (default `172.16.10.0/24`), `--invite-port` (default WireGuard port + 1), `--api-port` (TCP, default same number as the WireGuard port). The network's identity lands in `/etc/cord-server/<name>.toml`; state lives in `/var/lib/cord-server/<name>.db`.

Deliver the invite file out-of-band. On the joining machine:

```bash
cord client install alice.invite.toml   # redeem + confirm, then exits
cord client up homenet                  # connect (foreground; ctrl-c disconnects)
```

`cord client up` stays in the foreground: it periodically fetches peer state, applies changes to the live interface, and reports endpoint sightings. On Linux the interface uses kernel WireGuard; on macOS it is a userspace device that lives and dies with the `cord client up` process. Other commands: `cord client show`, `cord client fetch <net>`, `cord client down <net>`, `cord client uninstall <net>`.

### Managing the network

Locally on the server host, `cord server` offers the full set: `add-cidr`, `rename-cidr`, `delete-cidr`, `add-peer`, `rename-peer`, `enable-peer`, `disable-peer`, `delete-peer`, `add-association`, `delete-association`, `get-peers`.

Remotely, any *admin* peer can do the same over the API via `cord client admin`:

```bash
cord client admin peer add homenet bob 10.42.0.11 --save-invite ~/invites
cord client admin peer disable homenet bob
cord client admin cidr add homenet infra 10.42.1.0/24
cord client admin association add homenet infra fleet
```

(Create an admin peer with `cord server add-peer ... --admin`.)

## Architecture

The `cord` binary is a thin CLI over packages in `internal/`:

- `internal/server` — network/CIDR/peer/invite logic, the network config file, and the serve `Runtime` (interfaces + sync loop).
- `internal/api` — HTTP handlers; the main network serves the full API, the invite network serves *only* the redeem endpoint.
- `internal/client` — install/up/down/show/fetch/uninstall flows, local peer cache, API client.
- `internal/database` — SQLite adapters with versioned migrations: the server store (cidr, association, invite, peer, endpoint tables) and the client peer cache.
- `internal/wireguard` — interface management with two backends: Linux kernel (netlink/wgctrl) and userspace (embedded wireguard-go). On macOS the OS names devices `utunN`; cord tracks the mapping.
- `internal/utils` — IP/CIDR helpers.

The API surface is described in [docs/api-spec.md](docs/api-spec.md), and the main flows in [docs/core-flows.md](docs/core-flows.md).

### Notes & limitations

- All cord files on disk are TOML: server network config, invite files, and client config (`/etc/cord/<network>.toml`, which contains the private key). The HTTP API speaks JSON.
- Peer-to-peer traffic that has no direct path is routed through the server (its allowed-IPs cover the whole network from a client's view); this requires IP forwarding to be enabled on the server host.
- IPv4 is the well-tested path; IPv6 plumbing exists in the database layer but hasn't been exercised end-to-end.

## Testing

Unit tests use in-memory SQLite and in-memory config stores; they run unprivileged on macOS and Linux (`make test`). The integration tests (`sudo make test-integration`) create real interfaces: device lifecycle, OS visibility, peer sync, config file output, and a live two-interface WireGuard handshake over loopback.
