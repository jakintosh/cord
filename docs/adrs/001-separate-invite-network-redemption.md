## ADR-001: Separate WireGuard Networks for Peer Redemption

**Context:** During the invite redemption process, peers need to transition from temporary credentials to permanent ones. WireGuard's constraint that no two peers can share the same Allowed IP makes it impossible to have both temporary and permanent configurations active simultaneously on the same IP address. We need a clean way to handle this transition without breaking the HTTP communication between client and server during the redemption flow.

**Decision:** Implement two separate WireGuard networks - an "invite network" (e.g. 172.16.0.0/16 on port 51821) exclusively for redemption, and the main network (e.g. 10.0.0.0/8 on port 51820) for operational peers. The invite network only exposes the `/peer/redeem` endpoint, while the main network hosts the full API. Peers connect to the invite network with temporary credentials, receive their permanent configuration, then transition to the main network. Both of these root network CIDRs will be provided by the admin at network initialization.

**Status:** Accepted

**Consequences:**
- Positive: Complete isolation between untrusted invite peers and the production network, natural security boundary, simple IP management, clean peer lifecycle with no intermediate states, atomic cleanup (just remove from invite network), reuses existing WireGuard management code
- Negative: Server must manage two WireGuard interfaces, two UDP ports to track in configuration, slightly more complex server initialization

**Alternatives considered:**
- Reserved IP pool within main network: Rejected due to complex IP recycling logic and reduced address space for production peers
- Atomic key replacement with scheduled transition: Rejected due to timing coordination complexity and brief connectivity loss
- HTTP-only redemption over public internet: Rejected due to operational complexity of TLS certificates, reverse proxies, and public endpoint exposure
- Dual peer configuration on same IP: Rejected because WireGuard fundamentally doesn't support multiple peers with same Allowed IP

**Implementation notes:**
- Invite network binds to separate UDP port
- HTTP API on invite network only serves `/peer/redeem` endpoint
- Sequential IP assignment on invite network (e.g. 172.16.0.2, .3, .4...)
- Server identifies network by source IP range when routing requests

**Risks & mitigations:**
- Risk: Invite network could accumulate stale entries if cleanup fails
- Mitigation: Periodic garbage collection of expired invites, monitoring of invite network size
- Risk: Confusion about which network a peer is on during debugging
- Mitigation: Clear logging with network identification, separate interface names (`network` vs `network-i`)

**Security benefits:** Invite peers cannot access admin endpoints, peer discovery endpoints, or any production network resources. Even compromised invites can only reach a single redemption endpoint, providing defense in depth.
