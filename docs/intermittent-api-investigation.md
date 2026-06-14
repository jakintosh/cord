# Intermittent API Connectivity Investigation

## Status

The cause is not yet proven. This document records the current hypothesis,
the smallest client-only experiment, and the server instrumentation to add if
the experiment is inconclusive.

Observed behavior:

- The macOS client has one stable route for the Cord network through one
  `utun` interface.
- Requests to the server's internal API alternate between a valid HTTP
  response and a TCP connection timeout.
- A valid HTTP `401 Unauthorized` proves that the route, tunnel, API listener,
  and TCP/UDP port sharing all work for that request. The authorization result
  is a separate issue from the timeout.
- Direct traffic to other Cord peers may continue working while traffic to the
  server fails.

## Current Hypothesis

Cord periodically rebuilds and reapplies complete WireGuard peer lists:

- The server syncs both interfaces every 10 seconds.
- The client syncs its interface every 25 seconds.
- Both WireGuard backends implement sync by replacing the complete peer list
  (`ReplacePeers: true` for kernel WireGuard and `replace_peers=true` for
  wireguard-go).

The hypothesis is that replacing an unchanged peer list disturbs live
WireGuard peer/session state long enough to cause intermittent API timeouts.
This is plausible but unproven. In particular, we have not yet correlated a
failed request with a client or server sync.

## First Experiment: Disable Client Peer Sync

The smallest experiment changes only the macOS client. Run:

```sh
sudo ./bin/cord -v client up --no-peer-sync pollinator
```

This diagnostic mode still:

- Builds and starts the WireGuard interface from the existing client config
  and local peer cache.
- Reports endpoint sightings.
- Fetches peer state from the server every 25 seconds.
- Reconciles the fetched state into the local peer cache.

It skips only the final step that reapplies the complete peer list to the live
WireGuard interface. With `-v`, every completed fetch reports that peer
application was skipped, making it clear that the diagnostic mode remains
active while requests are tested.

While it runs, repeatedly request the API:

```sh
while true; do
  date
  curl -sv --connect-timeout 3 \
    http://10.33.0.1:51820/api/v1/admin/peers \
    -o /dev/null
  sleep 1
done
```

Interpretation:

- If the timeouts stop, client-side periodic peer replacement is implicated.
- If the timeouts continue, client-side periodic peer replacement is not the
  sole cause. Server-side replacement remains a candidate because the server
  still syncs every 10 seconds.
- If peer endpoints or membership change during the experiment, those changes
  are recorded locally but are not applied until the client is restarted
  without `--no-peer-sync`.

## Second Experiment: Disable Server Periodic Peer Sync

The client-only experiment did not stop the failures. A 100-second ping to the
server's WireGuard IP showed long bursts of packet loss beginning roughly
every 20 seconds. That rules out HTTP and makes client-side periodic peer
replacement insufficient to explain the issue.

The next experiment changes the server:

```sh
sudo ./bin/cord server serve --no-periodic-peer-sync pollinator
```

This diagnostic mode still:

- Applies the complete peer lists when the server starts.
- Applies peer-list changes immediately after successful API mutations.
- Runs periodic invite and endpoint maintenance.
- Serves both HTTP APIs normally.

It skips only peer-list application triggered by the 10-second timer. Repeat
the ping and curl tests while the server runs in this mode.

Interpretation:

- If the packet-loss bursts stop, periodic server-side peer replacement is
  strongly implicated.
- If the packet-loss bursts continue, the cause is outside Cord's periodic
  peer sync loop and the server instrumentation below becomes the next step.
- Database changes made outside the API are not reflected in the live
  WireGuard interfaces until an API mutation triggers a sync or the server is
  restarted without `--no-periodic-peer-sync`.

## Server Instrumentation Follow-up

If a server update is needed, it should collect enough information to
correlate a timeout with the server's view before changing sync behavior.
Diagnostics should be opt-in, structured, and include a timestamp and network
name on every event.

For every periodic or mutation-triggered sync, record:

- Trigger: periodic tick or API mutation.
- Main and invite desired peer counts.
- Desired peer configuration fingerprint.
- Added, removed, and changed peer public-key prefixes.
- Changed fields: allowed IPs, configured endpoint, and keepalive.
- Sync start time, finish time, duration, and result.
- Whether the desired configuration is identical to the previous desired
  configuration.

Immediately before and after applying each interface, record live device
status for every peer:

- Public-key prefix.
- Current endpoint.
- Latest handshake timestamp and age.
- Receive/transmit byte counters, when exposed by the backend.

For API requests, record:

- Request method and path.
- Remote source IP.
- Authentication result: unknown IP, disabled/unconfirmed peer, non-admin
  peer, or authorized peer.
- Request start, duration, response status, and completion.

Useful correlations:

- Does a timeout begin immediately after a server sync?
- Does the server's peer endpoint or handshake timestamp change during sync?
- Is the desired peer configuration actually changing in the wild?
- Does the server receive a TCP connection for failed client requests?
- Do authorized requests sometimes arrive from a different source IP?

The first server diagnostic implementation should avoid logging full public
keys, private keys, invite payloads, or request bodies.
