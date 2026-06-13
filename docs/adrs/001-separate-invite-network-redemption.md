## ADR-001: Separate WireGuard Networks for Peer Redemption

**Context:** During the invite redemption process, peers need to transition from temporary credentials to permanent ones. WireGuard's constraint that no two peers can share the same Allowed IP makes it impossible to have both temporary and permanent configurations active simultaneously on the same IP address. We need a clean way to handle this transition without breaking the HTTP communication between client and server during the redemption flow.

**Decision:** Implement two separate WireGuard networks - an "invite network" (e.g. 172.16.0.0/16 on port 51821) exclusively for redemption, and the main network (e.g. 10.0.0.0/8 on port 51820) for redeemed peers. The invite network only exposes the `/invite/redeem` endpoint, while the main network hosts confirmation and the full API. Peers connect to the invite network with temporary credentials and present a permanent public key. Successful redemption is the authorization boundary: the server assigns that permanent key a main-network address and adds it to the main WireGuard interface. The peer then calls `/invite/confirm` over the main network to acknowledge that installation succeeded. Confirmation is an operational-readiness/accounting boundary, not a second security authorization decision. Both root network CIDRs are provided by the admin at network initialization.

**Status:** Accepted

**Consequences:**
- Positive: Temporary invite credentials remain isolated from the main network; successful redemption cleanly replaces them with a permanent main-network identity; interrupted installation remains retryable; reuses existing WireGuard management code
- Negative: Server must manage two WireGuard interfaces, two UDP ports to track in configuration, slightly more complex server initialization
- Negative: A redeemed-but-unconfirmed permanent peer has main-network packet access before Cord considers it operational. Confirmation gates normal Cord API access and peer discovery, but is not a packet firewall.

**Alternatives considered:**
- Reserved IP pool within main network: Rejected due to complex IP recycling logic and reduced address space for production peers
- Atomic key replacement with scheduled transition: Rejected due to timing coordination complexity and brief connectivity loss
- HTTP-only redemption over public internet: Rejected due to operational complexity of TLS certificates, reverse proxies, and public endpoint exposure
- Dual peer configuration on same IP: Rejected because WireGuard fundamentally doesn't support multiple peers with same Allowed IP

**Implementation notes:**
- Invite network binds to separate UDP port
- HTTP API on invite network only serves `/invite/redeem`
- Successful redemption creates an enabled, unconfirmed permanent peer and adds it to the main WireGuard interface
- Main-network `/invite/confirm` is callable by the assigned permanent peer before confirmation
- Other main-network and admin API routes require a confirmed, enabled peer
- Sequential IP assignment on invite network (e.g. 172.16.0.2, .3, .4...)
- Server identifies network by source IP range when routing requests

**Risks & mitigations:**
- Risk: Invite network could accumulate stale entries if cleanup fails
- Mitigation: Periodic garbage collection of expired invites, monitoring of invite network size
- Risk: Confusion about which network a peer is on during debugging
- Mitigation: Clear logging with network identification, separate interface names (`network` vs `network-i`)
- Risk: Treating `confirmed` or CIDR associations as packet-level access controls
- Mitigation: Document that redemption grants main-network membership, confirmation records readiness, and associations control discovery rather than host firewall policy

**Security boundary:** Before redemption, temporary invite credentials can reach only the invite network and its redemption endpoint. Successful redemption authorizes the presented permanent key for main-network membership. Until confirmation, Cord rejects that peer from normal API routes, admin routes, peer discovery, and endpoint gossip; operators requiring packet-level segmentation must enforce it separately with host/network firewall policy.
