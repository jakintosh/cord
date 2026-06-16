# Reconciliation Architecture Follow-ups

The live-state peer reconciler fixes destructive synchronization while
establishing boundaries for several larger architectural changes. These items
are intentionally out of scope for the initial reconciliation implementation.

## Server-Exclusive Database Writes

The running server should eventually become the exclusive writer of its
database. Local `cord server` administration commands currently edit SQLite
from separate processes, which is why the server periodically checks the
database for changes.

Moving local administration through the running server would:

- Centralize mutation validation and invariants.
- Guarantee immediate reconciliation after every mutation.
- Let periodic reconciliation focus on failed-operation retries and live
  WireGuard drift.
- Make degraded reconciliation state easier to associate with the mutation
  that caused it.

The new reconciler supports either model because every pass derives a fresh
plan from desired database state and live WireGuard state.

## Multiple Endpoint Candidates

The server should collect multiple observed endpoints for each peer and share
those candidates with other peers. Candidates may come from server
observations, endpoint gossip, and direct peer-to-peer communication.

WireGuard has one active endpoint per peer, so Cord will need a policy for:

- Ranking and expiring candidates.
- Trying alternatives after handshake failure.
- Distinguishing a bootstrap candidate from a live roamed endpoint.
- Updating an existing peer when a newer candidate should deliberately be
  tried.

The reconciler's endpoint policies prevent routine reconciliation from
overwriting dynamic WireGuard endpoint discovery while leaving room for an
explicit candidate-selection operation.

## Reconciliation Status API And CLI

Interfaces now retain structured reconciliation status, but it is not yet
exposed to operators. Future server and client commands should report:

- Last attempt and last successful reconciliation.
- Desired and observed peer counts.
- Pending add, update, and remove operations.
- Active errors and affected peer key prefixes.
- Whether the network is degraded.

API mutations that update durable state but fail to reconcile could return an
accepted-but-degraded result rather than appearing fully successful.

## Retry Scheduling And Drift Audits

Periodic loops currently provide retry and drift correction. A future runtime
could use:

- Immediate event-driven reconciliation after mutations.
- Backoff-driven retries while degraded.
- Less frequent audits of live WireGuard drift.
- Wakeups when endpoint candidates or other desired state change.

Every retry must continue to observe and re-plan rather than replaying a stale
operation queue.

## Device Lifecycle

Cord treats WireGuard devices as an implementation layer of a running Cord
network. Normal shutdown removes devices for every backend. A kernel device
may remain after a crash, so startup reconciliation must safely adopt and
repair it before normal operation.

No persistent unmanaged-device mode is planned.
