# Cord Design

This is a working design for Cord. It describes the current architecture
alongside ideas for where it could go.

Cord is a WireGuard network manager. It provides a server-authoritative control
plane for creating networks, inviting peers, distributing peer configuration,
and deciding which peers should be visible to each other. Once peers receive
their current view of the network, they use ordinary WireGuard interfaces to
communicate directly.

## Core Idea

A Cord network is a managed WireGuard network with one coordination server and
many peers.

The server is authoritative for:

- network membership
- invite creation and redemption
- peer addresses and public keys
- CIDR structure
- group assignment
- group associations
- peer endpoint observations
- the peer set each client should configure locally

Cord uses a hub-and-spoke control plane and a peer-to-peer data plane.

Peers ask the server for their current network view. The server responds with
the peers they should know about. Each client then reconciles its local
WireGuard interface to match that server-provided view. After that, peer traffic
is direct peer-to-peer WireGuard traffic.

Cord does not try to enforce a hard firewall against malicious peers that ignore
the server. It controls the configuration distributed to cooperative peers. A
well-behaved peer follows the server's view because the server is the source of
truth for safe membership and visibility.

## Design Principles

- The Cord server is the authority for network state.
- Clients reconcile local WireGuard state from server state.
- APIs are exposed over private transports: local Unix sockets, the invite
  WireGuard network, or the main WireGuard network.
- A peer's main-network identity is immutable after confirmation.
- Peer key rotation is not part of the design.
- CIDRs describe address structure.
- Groups describe logical membership.
- Group associations describe visibility.
- Group membership alone does not imply visibility.
- Visibility within a group requires an explicit self-association.
- Endpoint observations are dynamic reachability hints, not peer identity.
- Local administration is authorized by Unix socket access.
- Remote administration should use the same peer/IP identity model as the
  network API, rather than independent API keys.

## Daemon Model

Cord has both server and client daemon modes.

The CLI does not perform long-running network management directly. Instead,
the CLI talks to the relevant local daemon over an HTTP API exposed on a Unix
socket.

The server daemon manages:

- one or more Cord networks
- main and invite WireGuard interfaces for each network
- invite and main-network APIs
- local administrative APIs over a Unix socket
- peer endpoint observations reported over the main network
- authoritative private DNS over the main network

The client daemon manages:

- local WireGuard interface state
- invite redemption
- main-network confirmation
- periodic polling of the server for visible peers
- reconciliation of local WireGuard peers
- observation and reporting of active peer endpoints
- endpoint candidate retry state for degraded peers
- host resolver configuration for enabled permanent networks
- a local control API over a Unix socket for the CLI

The Unix socket APIs are local control surfaces. Remote network coordination
happens through WireGuard.

Both runtimes own mutable resource state in one goroutine. Commands and
maintenance execute sequentially, including work for different networks. A
blocked operation can therefore delay other networks and status commands.
Desired state is persisted separately from observed resource state. Failed
startup or cleanup remains visible in status and can be retried by maintenance.

Composition follows database -> service -> runtime -> APIs. Services own domain
policy over store contracts; SQLite adapters own transactions and durable
invariants; runtimes own WireGuard devices, listeners, activities, and cleanup.

## Network Planes

Each Cord network has two WireGuard planes.

### Invite Network

The invite network is a temporary onboarding network.

It exists so an invited machine can prove possession of invite credentials and
call the invite API without exposing that API publicly. The invite network has
its own CIDR and must not overlap the main network CIDR.

The server creates the invite peer identity. The invite file gives the recipient
enough information to bring up the invite WireGuard interface and reach the
server's invite API.

The invite file contains:

- the Cord network name
- the temporary private key and assigned host route
- the server's WireGuard public key and external invite endpoint
- the server's invite-network host route and API port
- the full invite subnet

The client derives a temporary interface name by appending `-i` to the network
name, and the API address from the server route and port. Redemption returns the
same file shape with main-network information and omits the private key.

The invite identity is temporary. It is not the peer's final main-network
identity.

### Main Network

The main network is the durable Cord network.

Confirmed peers join the main WireGuard network with their permanent peer keys
and immutable terminal CIDRs. The server's main-network API is reachable over
this network. Peers use that API to confirm onboarding and to fetch their
current visible peer set.

The server has a reserved `cord-server` peer record and terminal CIDR, but is
treated specially when distributing WireGuard configuration. Peers receive and
persist the server's main-network identity during redemption and keep it
configured unconditionally so they can confirm and poll for updates. The server
likewise keeps all enabled peers on its main WireGuard device regardless of
ordinary group visibility rules.

## Device Routing

The server and client apply different routing conventions for their WireGuard
devices. These conventions define the device-level `Route` and per-peer
`AllowedIPs` on each side.

### Server Device

The server configures its own WireGuard device route as the server's IP with the
full network mask (for example, `10.42.0.1/16`). The backend also installs a
host route-table entry for the full overlay CIDR so the server can reach every
peer through that interface. Per-peer `AllowedIPs` remain terminal prefixes.

### Server as Route Authority

When the server informs a peer about other peers, it always supplies
terminal-prefix routes (for example, `10.42.16.1/32`). The server is the
authoritative source for per-peer routing. The client trusts the server's routes
as host routes in peer `AllowedIPs`. The client parses and canonicalizes these
routes before applying them; it does not widen them to the overlay subnet.

### Client Device

The client configures its own WireGuard device with a terminal-prefix route (for
example, `<peer-ip>/32` for IPv4, `/128` for IPv6). Each peer entry on the
client device also uses terminal-prefix `AllowedIPs` as supplied by the server.

The interface address and host routing table have different scopes. The client
also supplies the full overlay `NetworkCIDR` to the backend, which installs a
route for that subnet through the tunnel. WireGuard selects configured peers
using their terminal `AllowedIPs`; a subnet route alone does not provide a peer
for every address in that subnet. For example:

```text
Interface address: 10.42.16.1/32
Host route:        10.42.0.0/16 -> Cord interface
Server AllowedIPs: 10.42.0.1/32
Peer AllowedIPs:   10.42.16.2/32
```

## Core Entities

### Cord Server

The coordination authority for one or more Cord networks.

The server owns the durable database state and is responsible for turning that
state into WireGuard configuration.

### Cord Network

A managed WireGuard network. A server can host multiple Cord networks.

Each Cord network has:

- a main WireGuard interface
- an invite WireGuard interface
- a main network CIDR
- an invite network CIDR
- peers
- invites
- cidrs
- groups
- group associations
- endpoints
- private DNS derived from network and peer names

### Peer

A confirmed or pending participant in the main network.

A peer has a durable WireGuard public key and is linked to a terminal CIDR. The
peer's identity is its permanent main-network key and immutable terminal
address.

Peers generate and persist their own permanent key pair before invite
redemption. The server stores the peer's permanent public key and uses it when
adding the peer to the main network.

### Endpoint Observation

An endpoint observation records that one peer recently saw another peer at a
specific WireGuard endpoint.

The database table is `endpoint`; sightings are submitted through
`POST /endpoints`. There is no endpoint administration resource today.

An endpoint record includes:

- the peer being observed
- the peer that observed it
- the endpoint itself
- the server-side timestamp when the observation was received

Endpoint records are unique within a network by observed peer and observing
peer. A report at least as recent as the stored timestamp replaces that
observer's previous endpoint and timestamp; older reports are ignored. If an
endpoint has gone stale for the observer, Cord assumes it is unlikely to remain
useful to other peers.

Endpoint observations are dynamic reachability state. They are useful hints for
connecting to a peer, not part of the peer's durable identity.

### Invite

A pending invitation to join a Cord network. Persisted invitations are called
`registration` records; the invite file is the out-of-band artifact delivered
to the recipient.

An invite has an expiration time and a temporary invite-network identity created
by the server. The client persists permanent main-network keys before redemption
and submits the permanent public key to the server when redeeming.

An invite can be:

- created
- redeemed
- confirmed
- expired
- revoked

Redeemed but unconfirmed peers are provisional. Unconfirmed peers are not
visible to other peers. Expiration and revocation remove the registration and
its provisional peer and terminal CIDR transactionally. Confirmed registrations
are retained and cannot be revoked; removing their peer also removes the
associated registration.

### CIDR

A CIDR represents address structure in a Cord network.

CIDRs may represent broad ranges, intermediate logical ranges, or terminal peer
addresses. CIDRs can be arranged hierarchically for address planning, but the
hierarchy is not the visibility model. Because they logically encompass their
children, non-terminal CIDRs are useful for grouping many peers together.

The schema marks terminal CIDRs with `terminal`. Their address prefixes are
immutable; names and group assignments can change under the store's validation
rules. Ordinary CIDR updates also currently change only the name, not the range.
Deletion rejects the CIDR named after the network, and a CIDR referenced by a
peer cannot be deleted directly. Peer deletion removes its terminal CIDR, so
that address can be reused.

### Group

A group is a first-class, network-scoped record in the `group` table. The
`cidr_assignment` join table attaches zero or more groups to each CIDR, and
`association` connects groups. Pending registrations have separate
`registration_assignment` rows; confirmation transfers those assignments to
the peer's terminal CIDR in the same transaction.

A peer does not need its own independent group list. A peer is linked to a
terminal CIDR, and all grouping flows through the CIDR layer.

### Group Association

A group association connects two groups and makes members of those groups
visible to each other.

Associations are explicit. Sharing a group does not make peers visible to other
members of that same group. To make members of a group visible to each other,
Cord must have a self-association for that group.

For example:

```text
engineers <-> engineers
```

allows engineers to see other engineers.

```text
users
```

by itself does not allow users to see other users.

## Peer Onboarding Lifecycle

Peer onboarding is the most important flow in Cord.

### 1. Create Invite

An administrator creates an invite for a future peer.

The server:

- stores the invite in its database
- reserves the requested terminal main-network route in a registration
- creates a temporary invite-network peer identity
- adds that temporary identity to the invite WireGuard interface
- returns an invite artifact that the CLI can write to a file for the recipient

The invite file contains the temporary invite identity and enough server
information to join the invite network and reach the invite API.

### 2. Join Invite Network

The invitee reads the invite file and brings up the temporary invite WireGuard
interface.

At this point, the invitee can reach only the invite API over the invite
network.

### 3. Redeem Invite

The client generates and persists its permanent main-network WireGuard key pair
when beginning the install, before contacting the invite API. It derives the
public key from that saved private key when redeeming.

The redemption request only includes the peer's permanent main-network public
key (`perm_pubkey`), because the WireGuard layer validates the peer's possession
of valid credentials from the invite, and the server can derive peer identity
from the calling IP address.

The server validates the invite. If redemption is allowed, the server:

- stores the peer's permanent public key
- links the peer to its terminal main-network CIDR
- provisionally adds the peer to the main WireGuard network
- returns the main-network information needed by the client

The peer is now provisionally admitted to the main network, but onboarding is
not complete.

`POST /redeem` accepts retries with the same permanent public key while the
registration remains unconfirmed and unexpired. It rejects a different public
key. The saved client install record retains both the permanent private key and
the returned assignment so retries can continue from the persisted phase.

### 4. Join Main Network

The client tears down the invite network and brings up the main WireGuard
interface using its permanent key pair.

The server has already added the peer to the main network, so the peer should be
able to reach the main-network API.

### 5. Confirm

The client calls the confirm endpoint over the main network.

This proves that the peer can actually reach the server through the
main-network identity it submitted during redemption.

Confirmation marks the peer and registration confirmed and transfers pending
group assignments to the terminal CIDR atomically. It does not change the
peer's enabled flag. Confirmed registrations are retained as lifecycle history,
not exposed through a separate audit API.

### Failure And Retry

Redemption is retryable within the limits above, and confirmation is idempotent.

If redeem succeeds but confirm never happens, the peer remains provisional. The
original registration expiration continues to govern cleanup. Maintenance prunes
expired unconfirmed registrations and their provisional peers and terminal CIDRs,
then reconciles the WireGuard peer sets from the remaining state.

## Addressing And CIDR Model

Cord's address model is CIDR-first.

All address structure is represented as CIDR rows. Some CIDRs are broad
organizational ranges. Some are narrower operational ranges. Some are terminal
prefixes that represent individual peers.

For IPv4, a terminal peer CIDR is a `/32`. For IPv6, it is a `/128`.

Peers are linked to terminal CIDRs rather than owning address fields directly.
That keeps grouping and containment on the CIDR layer:

```text
CIDR tree:
10.0.0.0/16          group: users
  10.0.1.0/24        group: engineering
    10.0.1.42/32     group: app:billing-user

Peer:
alice -> 10.0.1.42/32
```

Alice's effective groups are:

```text
users
engineering
app:billing-user
```

This keeps peer records focused on identity and lifecycle while CIDR records
handle address placement, inheritance, and grouping.

CIDR hierarchy remains useful for address planning. For example, a server range
may contain location-specific child ranges, and each child range may contain
terminal server peer CIDRs.

Visibility, however, is not based directly on the CIDR tree. Visibility is based
on effective groups and group associations.

## Groups And Visibility

Groups can be attached to CIDRs at any level.

A peer's effective groups are the union of:

- groups on its terminal CIDR
- groups on containing CIDRs

Inheritance is additive. The design does not currently include group overrides
or negative groups.

Group associations determine visibility. If a peer has group `A`, and group `A`
is associated with group `B`, then the peer can see peers whose effective groups
include `B`.

Associations are bidirectional.

### No Implicit Same-Group Visibility

Group membership does not imply that members of the same group can see one
another.

This is intentional. Large pools such as `users` or `customers` often should not
be internally visible. Those peers may need access to shared resources without
seeing each other.

To make members of a group visible to each other, create a self-association:

```text
engineers <-> engineers
```

Without this self-association:

```text
engineers
```

is only a label.

### Resource Access Example

Suppose Cord needs to grant selected users access to selected billing servers.

CIDRs or terminal peer CIDRs can be assigned groups:

```text
app:billing-user
app:billing-server
```

Then an association grants visibility:

```text
app:billing-user <-> app:billing-server
```

Those users can see those servers, and those servers can see those users.
Billing users do not see other billing users unless this also exists:

```text
app:billing-user <-> app:billing-user
```

This lets Cord model both broad topologies and targeted application access with
the same mechanism.

### Visibility Resolution

When a peer asks for its visible peers, the server computes the response from
authoritative state:

```text
request source IP
-> identify the calling peer
-> find the peer's terminal CIDR
-> compute the peer's effective groups
-> find groups associated with those effective groups
-> find terminal peer CIDRs with those associated groups
-> return the corresponding peer WireGuard configuration
```

The response excludes hidden, disabled, and unconfirmed peers, as well as the
calling peer itself. The service compiles network topology from a consistent
store read and caches the model and each subject's view. Topology mutations
invalidate that model; endpoint observations are loaded fresh for each snapshot.
The snapshot's peers and projected topology use the same compiled state.

Visibility resolution should remain efficient as the CIDR tree, group
assignments, and peer population grow. Compiled models and cached subject views
are the current approach.

The server's own main-network endpoint remains reachable because clients pin
the server configuration received during redemption. The server is filtered out
of the returned WireGuard peer set even if ordinary visibility resolution
includes it.

## Endpoint Discovery

Cord peers do not have a single fixed endpoint. A peer may be reachable at
different endpoints over time because of NAT, roaming, or ordinary network
changes.

Peers help the server discover useful endpoints by observing their local
WireGuard state. The client daemon periodically scans its configured peers,
filters for peers with recent handshakes, and reports the active endpoints it
sees back to the server.

The server stores those reports as endpoint observations. The reporting peer
does not send a timestamp; the server records when it received the observation.
The report handler attributes the witness to the authenticated caller, and the
store requires both peer keys to belong to the network. Returned snapshots
filter the peer set through the visibility model.

We want reports to be accepted only for confirmed, enabled targets visible to
the witness. That check is not implemented yet. Endpoint discovery should not
expand visibility beyond the group-association model.

The server also observes its own main WireGuard device, providing initial
endpoint candidates for confirmed, enabled peers before client gossip arrives.

When the server returns visible peers, it returns an ordered list of endpoints
for each peer rather than a single endpoint:

```text
peer
  public_key
  route
  endpoints
    endpoint, timestamp
    endpoint, timestamp
```

Endpoint lists are sorted from most recently seen to least recently seen. The
server omits sightings older than 24 hours and prunes them during maintenance.

The client persists candidate endpoints and their attempt times. A peer whose
handshake is at least three minutes old is stale; a peer with no handshake is
also stale. During scans, the client chooses the least-recently-attempted
candidate no more than once every 90 seconds per peer. Never-attempted candidates
come first, with catalog order breaking ties. It keeps rotating at that cadence
without exponential backoff, and synchronization merges new candidates into the
stored catalog.

Default activity intervals are two minutes for snapshot sync, 30 seconds for
handshake scans, and five minutes for endpoint reports. Runtime maintenance
defaults to 30 seconds. Client peers use a 25-second persistent keepalive.

Endpoint discovery is a best-effort connectivity aid. It helps peers find each
other across common endpoint changes, but it does not replace the server's
authoritative role in deciding which peers are visible.

## Main Network Peer API

The main network API is reachable only over the main WireGuard network.

Peer identity is inferred from the request source IP. A peer should not need to
tell the server who it is in ordinary peer API calls. The server maps the source
IP to the peer's terminal CIDR and peer record.

The core peer endpoints are:

```text
POST /confirm
GET  /snapshot
POST /endpoints
```

`POST /confirm` completes the invite redemption flow for a provisionally
admitted peer. It also accepts confirmed, enabled callers so a lost response
can be retried. Other peer endpoints require confirmed, enabled identity.

`GET /snapshot` returns `generated_at`, `peers`, and `topology` in the HTTP data
envelope. The topology is projected for the caller, rather than exposing the
complete administrative view. The client persists the snapshot and reconciles
its local WireGuard peer set. Each returned peer includes an array of recently
observed endpoints sorted from newest to oldest.

`POST /endpoints` accepts a JSON array of `{peer_key, endpoint}` sightings.
The handler records the receipt time and caller identity; the visibility
validation gap is described under Endpoint Discovery.

There is no `/v1` prefix in the current routers.

## Invite API

The invite API is reachable only over the invite WireGuard network.

The core invite endpoint is:

```text
POST /redeem
```

This endpoint validates the temporary invite identity and accepts the peer's
permanent main-network public key. On success, it provisionally creates or
updates the main-network peer state and returns the information the client needs
to join the main network.

## Private DNS

Each enabled server network serves authoritative DNS over UDP and TCP port 53
on its main Cord address. The zone is `<network>.internal`; peers resolve as
`<peer>.<network>.internal`, and the server as
`cord-server.<network>.internal`. Network and peer names are validated as
lowercase DNS labels; network names also satisfy the interface-name limit.

Only confirmed, enabled callers can query. A caller can resolve itself, the
server, and peers visible through the topology model. Hidden, disabled,
unconfirmed, and missing target names produce NXDOMAIN. Queries outside the
zone and unknown callers are refused. The server serves A or AAAA records for
the host's address family, plus SOA and NS at the zone apex; it does not recurse
or serve reverse DNS. Positive TTLs are one hour and negative TTLs five minutes.

Clients configure the host resolver for permanent networks: Linux uses
`resolvectl` with `systemd-resolved`, and macOS uses marked files in
`/etc/resolver`. Temporary enrollment tunnels do not configure DNS. Client
resolver failures leave tunnel synchronization running and are retried. Owned
resolver configuration is cleaned up before closing the permanent tunnel;
failed cleanup remains tracked. A server DNS bind failure during initial
startup rolls back that startup and maintenance retries it.

## Local Admin API

The server admin router is served only on the local Unix socket. It is not
mounted on the main WireGuard plane. The CLI talks to the matching daemon
through its socket.
There is no per-request authentication on the local socket: filesystem access
is the authorization boundary. The default mode is `0666`; deployments can
select a stricter mode such as `0660` or `0600`.

These are the current server administration routes. Identifiers in paths are
names. The client has a separate local API for installation, network state,
synchronization, peers, topology, and status.

### Status

```text
GET /status
```

Status includes the daemon version and resource tree with observed state,
intended state, health, errors, and activity results.

### Networks

```text
GET    /networks
POST   /networks
GET    /networks/{name}
GET    /networks/{name}/topology
POST   /networks/{name}/enable
POST   /networks/{name}/disable
DELETE /networks/{name}
```

Creation persists the network, root CIDR, and reserved server peer. Enable and
disable change persisted intent and reconcile resources. Deletion cascades
network-owned database records and attempts runtime cleanup; unfinished cleanup
remains visible and retryable.

### CIDRs

```text
GET    /networks/{name}/cidrs
POST   /networks/{name}/cidrs
PATCH  /networks/{name}/cidrs/{cidr}
DELETE /networks/{name}/cidrs/{cidr}
```

Creation validates containment in the main network and rejects duplicate names
or prefixes. PATCH renames a CIDR; it does not move or resize its range.

### Groups

```text
GET    /networks/{name}/groups
POST   /networks/{name}/groups
DELETE /networks/{name}/groups/{group}
```

Groups are a separate network-scoped catalog. Deletion cascades their
assignments and associations.

### CIDR Group Assignment

```text
GET    /networks/{name}/cidrs/{cidr}/groups
POST   /networks/{name}/cidrs/{cidr}/groups
DELETE /networks/{name}/cidrs/{cidr}/groups/{group}
```

Assignments use existing catalog groups. For unconfirmed peers, manage pending
assignments through their registration instead; direct CIDR assignment is
rejected until confirmation.

### Group Associations

```text
GET    /networks/{name}/associations
POST   /networks/{name}/associations
DELETE /networks/{name}/associations/{group1}/{group2}
```

Associations are symmetric, normalized pairs of group IDs in the database.
Self-associations use the same representation with equal IDs.

### Peers

```text
GET    /networks/{name}/peers
PATCH  /networks/{name}/peers/{peer}
DELETE /networks/{name}/peers/{peer}
```

Ordinary peers are created through redemption. PATCH supports name, `admin`,
and `enabled`; it does not change public key or address. Deletion removes the
peer, its terminal CIDR, associated registration, and endpoint observations.

### Registrations

```text
GET    /networks/{name}/registrations
POST   /networks/{name}/registrations
DELETE /networks/{name}/registrations/{registration}
GET    /networks/{name}/registrations/{registration}/groups
POST   /networks/{name}/registrations/{registration}/groups
DELETE /networks/{name}/registrations/{registration}/groups/{group}
```

POST creates a registration and returns the invite artifact. The default expiry
is 24 hours. DELETE revokes an unconfirmed registration and cleans up any
provisional peer state. Confirmed registrations cannot be revoked or have their
pending group assignments changed.

## Remote Admin API

Remote administration is still a plan. The idea is to serve an admin API over
the main WireGuard network alongside ordinary peer coordination, using Cord's
existing identity model:

```text
request source IP -> peer -> administrative authorization decision
```

The server would decide what the caller may administer from its Cord peer
identity, without separate API keys. Group visibility alone would not grant
administrative permission.

Requests should be scoped to the network on whose main interface they arrive.
Server-wide or cross-network administration would need explicit authorization.

One possible API root is `/v1/admin`; the exact routes can be settled later.
Local and remote administration should share handlers and domain operations,
with an authorization layer around the remote surface. Local Unix socket access
would continue to rely on socket permissions. Sharing handlers should not give
a remote caller access to every network the local API can manage.

### Administrative Grants

Grants could describe which peers may use remote admin endpoints. A grant could
refer directly to a peer, or to a group whose effective members receive that
permission. Administrators should be able to inspect, create, change, and revoke
grants.

Peers already have an `admin` flag. How grants relate to that flag, how broadly
they apply, and how group membership changes affect permission still need to be
worked out.

## Further Ideas

These are future possibilities, rather than features available today.

### Status And Events

Remote administration should provide daemon and network status, scoped to the
caller. A history of administrative and reconciliation events could help explain
what happened over time; current activity results only describe recent attempts.

### Endpoint Inspection And Cleanup

Admins should be able to inspect peer-reported observations, delete bad or stale
records, and request pruning according to the network's culling policy.

Observations should still come from peers, with the server recording the witness
and timestamp. Admins should not create or edit them directly. Automatic pruning
already exists; inspection and manual cleanup would make connectivity problems
easier to investigate.

### Audit Records

An audit history should record invite creation and revocation, peer deletion,
CIDR changes, group assignments, and group associations. Endpoint pruning could
also be included. Audit records would be read-only through the API.

Confirmed registrations already preserve onboarding history, but not a general
administrative audit trail. What to retain, how to attribute actions, and how
audit records relate to operational events still need thought.

### Resource Inspection And Updates

Administration could support individual resource reads and broader updates:
network configuration, group names or metadata, associations, and registration
metadata or expiration before redemption. These operations would need to
preserve confirmed registration history and immutable peer keys and addresses.

### Explicit Reconciliation

An administrator may need an explicit command to reapply persisted intent to
WireGuard devices and API listeners without editing that intent. Maintenance
already does this automatically. An explicit command should fit the existing
enable/disable model, which records whether the network should be running.

### CIDR Mutation And Deleted Addresses

Non-peer ranges could eventually support moving, narrowing, or widening.
Current updates only rename a CIDR. Broader changes would need rules for their
effect on contained peers and inherited groups, while keeping terminal peer
addresses immutable.

Deleting a peer currently deletes its terminal CIDR and allows address reuse.
We could keep that behavior, retain a tombstone, or archive the old allocation.
Keeping an address fixed during a peer's lifetime doesn't settle whether it
should be reused afterward. Retention would also need to account for network
deletion and expired or revoked provisional registrations.

### Timing And Retry Behavior

Polling intervals, endpoint expiry, and retry timing can evolve with operational
experience. The current fixed rotation cadence is described above; backoff was
an earlier possibility rather than the behavior we implemented.

### Runtime Progress And Failure Ownership

The DNS follow-up note preserves two other ideas: allowing networks to progress
independently, and treating runtime database corruption as a reason for an
orderly daemon exit.

Independent workers could keep a slow operation from blocking other networks
or status reads. Work for each network should still be sequential, and shutdown
should wait for owned work to finish. Corruption handling should preserve the
SQLite cause and release resources before closing the database. Both ideas
should fit the existing resource ownership and cleanup model.
