# Server Flows

## Network Initialization

1. Admin runs `cord server network add <name> <cidr> <external-ip> <port>`
   (optionally `--invite-cidr`, `--invite-port`, `--api-port`).
2. The name is validated (alphanumeric, hyphen, period) before any state
   is created: invalid names fail without leaving directories or database
   files behind. Network names are limited to 13 bytes and must not end
   in the reserved `-i` suffix so the main (`<name>`) and invite
   (`<name>-i`) Linux interface names remain distinct and fit the
   kernel's 15-byte limit. Creating a network whose config file already
   exists fails with a conflict; the server also validates that the
   invite CIDR does not overlap the root CIDR.
3. Server verifies it can write the config file before touching the database.
4. Server generates its WireGuard keypair.
5. In one transaction: schema is migrated, the root CIDR is inserted as
   row `id=1`, and the server peer (`cord-server`, first assignable IP,
   admin/enabled/confirmed) is created.
6. Server writes `<config-dir>/<name>.toml`: private/public key, root and
   invite CIDRs, external IP, WireGuard ports, API port.

**Outputs:** SQLite database at `<data-dir>/<name>.db`, network config
file, network ready for invites.

## Network Deletion

`cord server network delete <name>` deletes the network database and its
sidecars, then deletes `<config-dir>/<name>.toml`. After removing those
files, the config and data directories are also removed if they are empty;
shared directories containing other networks or files are left intact.

## Serving

`cord server serve <name>` loads the network config and runs until
interrupted:

1. Brings up the **main interface** (`<name>`) with the server's key and
   address, peers = all enabled permanent peers, including
   redeemed-but-unconfirmed peers (allowed-ips = their /32).
2. Brings up the **invite interface** (`<name>-i`), peers = all
   active invites' temporary keys (allowed-ips = their invite /32).
3. Starts two HTTP listeners: the full API on the main internal address,
   and a redeem-only API on the invite internal address.
4. Sync loop: every 10s — and immediately after any API mutation — both
   interfaces' peer lists are rebuilt from the database and synced in
   place. Periodically, expired invites and endpoint sightings older
   than 24h are pruned.
5. On SIGINT/SIGTERM the HTTP servers shut down and both interfaces are
   destroyed.

## Peer Redemption

1. Admin creates an invite (`cord server peer add` or `POST /admin/peer`):
   the requested main-network IP is reserved, a temporary keypair and the
   lowest free invite-network IP are assigned, and the invite record is
   stored with an expiration.
2. The invite file (TOML) is delivered to the client out-of-band. It
   contains the temporary private key, the assigned invite-network CIDR,
   and the server's public key + invite endpoints.
3. Client (`cord client install`) generates and persists a permanent keypair
   *first* (so the flow is retryable), brings up a temporary interface on
   the invite network, and `POST /invite/redeem`s with the permanent
   public key in the body.
4. Server validates the invite by source IP, then atomically marks it
   redeemed and creates the peer record — enabled but **unconfirmed** —
   and returns the main-network assignment (assigned CIDR, server public
   key and endpoints). Redemption is the authorization boundary for
   main-network membership.
5. The serve loop adds the new peer to the main interface.
6. Client brings up the main interface and `POST /invite/confirm`s from
   its assigned IP with its key in the body.
7. Server sets `confirmed=1`, marking the installation operational, and
   deletes the invite; the serve loop drops the temporary peer from the
   invite interface.
8. Client tears down the invite interface and fetches an initial peer list.

**Idempotency:** both redeem and confirm can be retried; a redeemed
invite returns the same configuration for the same key, and confirming a
confirmed peer succeeds.

**Key states:** invite created → peer exists only on invite network;
redeemed → permanent peer is authorized on the main WireGuard network but
normal Cord APIs and peer discovery remain unavailable; confirmed → main
network only, fully operational in Cord.

**Trust boundaries:** `/invite/redeem` authorizes the permanent key and
grants main-network packet access. `/invite/confirm` is a retryable
operational acknowledgment proving that the client received the response and
successfully configured the main tunnel. Confirmation gates normal Cord API
access, administration, peer discovery, and endpoint gossip; it is not a
packet-level firewall boundary. CIDR associations likewise control discovery,
not packet forwarding.

## Peer Visibility

`GET /peers` (or `cord server peer visible`) resolves the requesting peer's
most specific CIDR, expands it with associated CIDRs, and returns all
confirmed, enabled peers in that set (excluding the requester), each with
its endpoint sightings from the last 24h, newest first. Associations are
symmetric; creating one immediately widens visibility for both sides,
deleting one narrows it.

## Peer Administration

Rename, enable/disable, grant/revoke admin (`PATCH`), and delete
(`DELETE`) work locally via `cord server` or remotely via the admin API.
Read-only inspection is available the same two ways: locally via
`cord server network|cidr|peer|association|invite list` (plus
`network show`), remotely via the admin `GET` endpoints
(`cord client admin ... list`). Every server command except
`network add` requires the network's database to already exist and
fails with "not found" otherwise — a mistyped network name never
creates directories or database files.
Disabling a peer removes it from other peers' lists and interfaces on
the next sync (immediately, when done over the API) and revokes its API
access. Deleting a peer also removes its endpoint history and any invite
reserving its IP.

# Client Flows

## Installation

`cord client install <invite-file>`: parse invite → persist permanent keypair →
invite interface up → verify invite handshake → redeem → persist assignment →
main interface up → verify main handshake → confirm → seed local peer
database → tear both interfaces down. The client config lands
in `<config-dir>/<network>.toml` (mode 0600 — it holds the private key);
the local database in `<data-dir>/<network>.db`.

If anything fails mid-flow, rerunning install reuses the persisted
keypair and the server's idempotent redeem/confirm semantics.

The invite's network name is validated when the invite file is parsed,
so a malformed invite is rejected before any local state is created.
Every other client command (`uninstall`, `show -n`, `fetch`, `up`,
`down`, `admin`) requires `<config-dir>/<network>.toml` to exist and
fails with "not installed" otherwise — a mistyped network name never
creates directories or database files. The local peer database is a
recreatable cache and is rebuilt as needed for installed networks.

## Connecting

`cord client up <network>` runs in the foreground:

1. Builds the interface from the client config and local peer database.
   The server peer's allowed-ips span the whole network (relay fallback);
   other peers get their /32s, which win by longest-prefix match when a
   direct path exists.
2. Writes `<data-dir>/<network>.conf` (wg-quick format, for reference).
3. Every 25s: report witnessed endpoint changes, fetch the peer list,
   reconcile the local database, and sync the interface in place.
4. Ctrl-C tears the interface down and exits.

On Linux the device is kernel WireGuard; on macOS it is a userspace
(wireguard-go) device that lives inside the `cord client up` process.
`cord client down` removes a kernel device left behind by a crashed
process.

## Endpoint Gossip

While connected, the client compares each live session's endpoint
(handshake within 3 minutes) against its database. Changed endpoints are
recorded locally and reported via `POST /endpoint`. The server stores
sightings keyed by witness and peer, serves the recent ones with peer
listings, and expires them after 24h. Only confirmed, enabled peers can
report or receive sightings.

## Uninstallation

`cord client uninstall <network>` brings the interface down (best effort) and
deletes the client config, local database, and generated wg config.
Idempotent: uninstalling an absent network succeeds.
