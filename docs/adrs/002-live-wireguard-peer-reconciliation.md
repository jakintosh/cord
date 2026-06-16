# ADR-002: Reconcile Cord Peer Configuration Against Live WireGuard State

**Status:** Accepted

## Context

Cord manages WireGuard peer configuration for server and client interfaces.
Previously, every sync rebuilt the complete desired peer list and instructed
the active backend to replace all peers. The server did this every 10 seconds
and clients every 25 seconds.

WireGuard peers contain operational state Cord does not own, including learned
endpoints, active handshake/session state, replay protection, and counters.
Replacing unchanged peers destroyed this state. On the server, ordinary peers
have no configured endpoint because WireGuard learns their endpoints from
authenticated traffic. Periodic replacement therefore caused repeated packet
loss until clients re-established handshakes.

## Decision

Cord is authoritative for durable peer configuration, while WireGuard owns
live operational state. Cord reconciles the former against the latter.

Each reconciliation:

1. Builds desired peer configuration from Cord state.
2. Queries the active WireGuard implementation for observed peer state.
3. Computes a deterministic plan of targeted add, update, and remove
   operations.
4. Applies only those operations.

Peers are matched by public key. Live peers absent from desired Cord state are
removed. Desired peers absent from the live device are added. Existing peers
are updated only when Cord-owned configuration differs. Unchanged peers are
never submitted to the backend.

Cord-owned configuration includes:

- Peer presence
- Allowed IPs
- Persistent keepalive
- Endpoint according to endpoint policy

WireGuard-owned operational state includes:

- Dynamically learned or roamed endpoints
- Latest handshake
- Transfer counters
- Session and replay state

Endpoint policies are:

- `dynamic`: WireGuard owns the endpoint. Used by the server for ordinary
  peers.
- `bootstrap`: Cord supplies an endpoint when adding the peer, then allows
  WireGuard to roam it. Used by clients for direct peers.
- `fixed`: Cord continuously enforces the endpoint. Used by clients for the
  server's public endpoint.

Both kernel and userspace backends expose normalized observed peer state and
translate standardized operations into backend-specific commands. Normal
reconciliation never uses full peer-list replacement.

Failed application does not roll back desired Cord state. The interface stores
structured reconciliation status containing the last attempt, last success,
pending plan, and errors. Later periodic reconciliation observes current live
state and creates a fresh plan before retrying.

## Consequences

- Periodic reconciliation is safe when desired state has not changed.
- Server-learned peer endpoints and active sessions survive reconciliation.
- Server and client modes use the same peer-management pipeline.
- Cord can remove unauthorized or stale live peers without disturbing others.
- Startup can reconcile against an existing kernel device after an unclean
  process exit.
- Backend observation must expose durable peer configuration in addition to
  operational status.
- Endpoint candidates and endpoint selection remain Cord-level concerns above
  the single endpoint currently active in WireGuard.
