# Cord API Specification

## 1) Endpoint List

| METHOD | PATH                       | PURPOSE                    | AUTH         | IDEMPOTENT |
|--------|----------------------------|----------------------------|--------------|------------|
| GET    | /api/v1/peers              | List visible peers         | main_net     | yes        |
| POST   | /api/v1/endpoint           | Report endpoint sightings  | main_net     | no         |
| POST   | /api/v1/invite/confirm     | Confirm peer presence      | assigned IP  | yes        |
| POST   | /api/v1/invite/redeem      | Redeem invite              | invite_net   | yes        |
| POST   | /api/v1/admin/peer         | Create peer invite         | admin        | no         |
| GET    | /api/v1/admin/peers        | List peers                 | admin        | yes        |
| GET    | /api/v1/admin/peer/{name}  | Get peer                   | admin        | yes        |
| PATCH  | /api/v1/admin/peer/{name}  | Rename/enable/disable peer | admin        | yes        |
| DELETE | /api/v1/admin/peer/{name}  | Delete peer                | admin        | yes        |
| POST   | /api/v1/admin/cidr         | Create CIDR                | admin        | no         |
| GET    | /api/v1/admin/cidrs        | List CIDRs                 | admin        | yes        |
| GET    | /api/v1/admin/cidr/{name}  | Get CIDR                   | admin        | yes        |
| PATCH  | /api/v1/admin/cidr/{name}  | Rename CIDR                | admin        | yes        |
| DELETE | /api/v1/admin/cidr/{name}  | Delete CIDR                | admin        | yes        |
| POST   | /api/v1/admin/association  | Create association         | admin        | yes        |
| GET    | /api/v1/admin/associations | List associations          | admin        | yes        |
| DELETE | /api/v1/admin/association/{cidr1}/{cidr2} | Delete association | admin  | yes        |
| GET    | /api/v1/admin/invites      | List invites               | admin        | yes        |

The server runs two HTTP listeners, one per WireGuard network. The
**invite network listener serves only `POST /api/v1/invite/redeem`**;
everything else lives on the main network listener (see ADR-001).
Successful redemption authorizes the permanent peer for main WireGuard
membership. Confirmation records that installation completed and gates the
remaining Cord API; it is not a second network-membership authorization.

## 2) Conventions

- Auth: IP-based via WireGuard network membership. The API is only
  reachable over the tunnel, so the source IP is cryptographically tied
  to a peer key. Forwarding headers are ignored.
  - `invite_net`: request must come from an unexpired invite's assigned IP
  - `main_net`: request must come from a confirmed, enabled peer's IP
  - `assigned IP`: confirm is callable by a not-yet-confirmed peer, but
    only from the IP its invite assigned, with its matching key in the body
  - `admin`: `main_net`, plus the peer's `admin=1` flag
- Public keys travel in request bodies, never in URL paths (WireGuard
  keys are base64 and may contain `/`).
- JSON fields: camelCase. Timestamps: Unix epoch integers.

## 3) API Object Schema

Every JSON response uses the `command-go/pkg/wire` envelope. Success:

```json
{
  "data": "(array | object)"
}
```

Failure:

```json
{
  "error": {
    "message": "string"
  }
}
```

Status-only responses (e.g. `204`) have no body.

## 4) Endpoints

### GET /api/v1/peers

Get the peers visible to the requesting peer based on CIDR
associations, with recently witnessed endpoints (newest first).
Excludes the requesting peer, unconfirmed peers, and disabled peers.

Response: `[PublicPeer]`
```json
{
  "name": "string",
  "cidr": "string",
  "publicKey": "string",
  "endpoints": [
    {
      "witnessKey": "string",
      "endpoint": "string",
      "timestamp": "integer"
    }
  ]
}
```

Status: `200` OK; `401` not a confirmed main-network peer.

### POST /api/v1/endpoint

Report endpoint sightings observed by the calling peer. The witness is
always the authenticated caller; any witness key in the body is ignored.

Request: `[EndpointSighting]`
```json
{
  "peerKey": "string",
  "endpoint": "string",
  "timestamp": "integer"
}
```

Response: `null`

Status: `200` recorded; `400` malformed; `401` unauthorized.

### POST /api/v1/invite/redeem

Redeem the caller's invite (identified by source IP on the invite
network) for a permanent peer registration. Idempotent: repeating the
call with the same key returns the same configuration, so clients can
retry after network failures. A successful response authorizes the permanent
key for main-network membership and triggers an immediate server WireGuard
peer resync. The created peer is enabled but remains unconfirmed.

Request: `{ "publicKey": "string" }` — the permanent key the client generated.

Response: `RedeemResult`
```json
{
  "networkName": "string",
  "assignedCidr": "string",
  "server": {
    "publicKey": "string",
    "externalEndpoint": "string",
    "internalEndpoint": "string"
  }
}
```

Status: `200` OK; `400` malformed; `401` no active invite for source IP;
`404` invite not redeemable.

### POST /api/v1/invite/confirm

Finalize installation from the peer's assigned main-network IP. The peer
already has main WireGuard membership from redemption. This call proves the
client received that assignment and successfully configured the main tunnel,
then marks the peer operational for normal Cord APIs and deletes the invite.
Idempotent.

Request: `{ "publicKey": "string" }`

Response: `null`

Status: `200` confirmed; `400` malformed; `404` no peer matching key + source IP.

### POST /api/v1/admin/peer

Create a peer invite. Returns the invite payload (the contents of an
invite file) so a remote admin can deliver it out-of-band. `expiresIn`
is in seconds; omit for the server default (24h).

Request: `CreatePeerRequest`
```json
{
  "name": "string",
  "ip": "string",
  "admin": "boolean",
  "expiresIn": "integer, optional"
}
```

Response: `PeerInvite`
```json
{
  "interface": {
    "networkName": "string",
    "privateKey": "string",
    "assignedCidr": "string"
  },
  "server": {
    "publicKey": "string",
    "externalEndpoint": "string",
    "internalEndpoint": "string"
  }
}
```

Status: `201` created; `400` malformed; `401` not admin; `409` name or IP taken.

### GET /api/v1/admin/peers, GET /api/v1/admin/peer/{name}

List all peers / get one peer.

Response: `[Peer]` / `Peer`
```json
{
  "name": "string",
  "cidr": "string",
  "publicKey": "string",
  "admin": "boolean",
  "enabled": "boolean",
  "confirmed": "boolean"
}
```

Status: `200` OK; `401` not admin; `404` (single) not found.

### PATCH /api/v1/admin/peer/{name}

Rename, enable/disable, or grant/revoke admin. All fields optional.

Request: `{ "name": "string?", "admin": "boolean?", "enabled": "boolean?" }`

Response: the updated `Peer`.

Status: `200` OK; `400` malformed; `401` not admin; `404` not found.

### DELETE /api/v1/admin/peer/{name}

Delete a peer, its endpoint history, and any invite holding its IP.

Status: `204` deleted; `401` not admin; `404` not found.

### POST /api/v1/admin/cidr

Create a child CIDR; must fall within the root range.

Request: `{ "name": "string", "cidr": "string" }`

Response: `Cidr`
```json
{
  "name": "string",
  "cidr": "string",
  "length": "integer",
  "prefix": "integer"
}
```

Status: `201` created; `400` malformed or outside the root range; `401` not admin; `409` name or range conflict.

### GET /api/v1/admin/cidrs, GET /api/v1/admin/cidr/{name}

List all CIDRs / get one CIDR. Response: `[Cidr]` / `Cidr`.

Status: `200` OK; `401` not admin; `404` (single) not found.

### PATCH /api/v1/admin/cidr/{name}

Rename a CIDR. Request: `{ "name": "string" }`. Response: the renamed `Cidr`.

Status: `200` OK; `400` malformed; `401` not admin; `404` not found.

### DELETE /api/v1/admin/cidr/{name}

Delete a CIDR and its associations.

Status: `204` deleted; `401` not admin; `404` not found; `409` cannot delete the root CIDR.

### POST /api/v1/admin/association

Associate two CIDRs, enabling peer visibility across them.

Request/Response: `{ "cidr1": "string", "cidr2": "string" }`

Status: `201` created; `400` malformed; `401` not admin; `409` exists/invalid.

### GET /api/v1/admin/associations

List associations. Response: `[Association]`.

### DELETE /api/v1/admin/association/{cidr1}/{cidr2}

Delete an association by its CIDR names (order-independent).
Idempotent: deleting an absent association succeeds.

Status: `204` deleted; `401` not admin.

### GET /api/v1/admin/invites

List all invites: active, redeemed-but-unconfirmed, and expired.
Confirmation deletes an invite, so confirmed peers never appear here.
The temporary invite key and invite-network address are not exposed.

Response: `[Invite]`
```json
{
  "name": "string",
  "networkCidr": "string",
  "admin": "boolean",
  "redeemed": "boolean",
  "expiration": "integer"
}
```

Status: `200` OK; `401` not admin.

## 5) Key Flows

- **Redemption:** `POST /invite/redeem` (invite net) → `POST /invite/confirm` (main net, assigned IP)
- **Trust boundary:** redeem authorizes main-network membership; confirm
  records operational readiness and unlocks normal Cord API access
- **State sync:** periodic `GET /peers` with WireGuard config updates
- **Endpoint gossip:** `POST /endpoint` submissions with observed endpoints
- **Administration:** admin peers use `/admin/*`; every mutation triggers
  an immediate WireGuard interface resync on the server
