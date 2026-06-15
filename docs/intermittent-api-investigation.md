# Intermittent API Connectivity Investigation

## Resolution

The intermittent API failure was caused by destructive server-side WireGuard
peer synchronization.

The server rebuilt both peer lists every 10 seconds and each backend replaced
the complete live WireGuard peer set. Replacing unchanged server peers removed
their dynamically learned endpoints and active session state. ICMP and HTTP
traffic then failed until clients re-established handshakes.

Experiments isolated the cause:

1. Disabling periodic peer application on the macOS client did not change the
   packet-loss pattern.
2. Disabling only periodic peer application on the server produced 100%
   packet delivery, regardless of client behavior.

The fix replaces full peer-list replacement with live-state reconciliation:

- Cord queries the active WireGuard device.
- A shared planner compares desired Cord peer configuration with observed
  WireGuard peer state.
- Backends apply targeted add, update, and remove operations.
- Unchanged peers are not touched, preserving learned endpoints, handshakes,
  and other runtime state.

The architecture and behavior are described in
[ADR-002](adrs/002-live-wireguard-peer-reconciliation.md). Deferred related
work is tracked in [reconciliation follow-ups](reconciliation-follow-ups.md).
