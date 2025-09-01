# Cord API Specification

## 1) Endpoint List

| METHOD | PATH                         | PURPOSE                    | AUTH         | IDEMPOTENT |
|--------|------------------------------|----------------------------|--------------|------------|
| GET    | /api/v1/peers                | List visible peers         | main_net     | yes        |
| POST   | /api/v1/endpoint             | Report endpoint sightings  | main_net     | no         |
| POST   | /api/v1/invite/confirm/{key} | Confirm peer presence      | main_net     | yes        |
| POST   | /api/v1/invite/redeem/{key}  | Redeem invite              | invite_net   | yes        |
| POST   | /api/v1/admin/peer           | Create peer invite         | admin        | no         |
| GET    | /api/v1/admin/peers          | List peers                 | admin        | yes        |
| GET    | /api/v1/admin/peer/{name}    | Get peer                   | admin        | yes        |
| PATCH  | /api/v1/admin/peer/{name}    | Rename/enable/disable peer | admin        | yes        |
| DELETE | /api/v1/admin/peer/{name}    | Delete peer                | admin        | yes        |
| POST   | /api/v1/admin/cidr           | Create CIDR                | admin        | no         |
| GET    | /api/v1/admin/cidrs          | List CIDRs                 | admin        | yes        |
| GET    | /api/v1/admin/cidr/{name}    | Get CIDR                   | admin        | yes        |
| PATCH  | /api/v1/admin/cidr/{name}    | Rename CIDR                | admin        | yes        |
| DELETE | /api/v1/admin/cidr/{name}    | Delete CIDR                | admin        | yes        |
| POST   | /api/v1/admin/association    | Create association         | admin        | yes        |
| GET    | /api/v1/admin/associations   | List associations          | admin        | yes        |
| DELETE | /api/v1/admin/association    | Delete association         | admin        | yes        |


## 2) Conventions

- Auth: IP-based authentication via WireGuard network membership
  - `invite_net`: Request must come from invite network IP range
  - `main_net`: Request must come from main network IP range
  - `admin`: Request must come from peer with `admin=1` flag
- JSON fields: camelCase to match Go struct tags
- Timestamps: Unix epoch integers


## 3) API Object Schema

### APIResponse
```json
{
  "error": "(APIError | null)",
  "data": "(array | object | null)"
}
```

### APIError
```json
{
  "code": "integer",
  "message": "string"
}
```


## 4) Endpoints

### GET /api/v1/peers

Description: Get list of peers visible to requesting peer based on CIDR associations. Excludes requesting peer, unconfirmed peers, and disabled peers.

Request: `null`

Response: `[PublicPeer]`
```json
{
  "name": "string",
  "cidr": "string",
  "publicKey": "string",
  "endpoints": {
    "witnessKey": "string",
    "endpoint": "string",
    "timestamp": "integer"
  }
}
```

Status:
| CODE | NOTE                                                          |
|------|---------------------------------------------------------------|
| 200  | OK                                                            |
| 401  | Request not from confirmed peer on main network               |

### POST /api/v1/endpoint

Description: Report endpoint sightings observed by peer. Used for endpoint gossip protocol.

Request: `[EndpointSighting]`
```json
{
  "peerKey": "string",
  "witnessKey": "string",
  "endpoint": "string",
  "timestamp": "integer"
}
```

Response: `null`

Status:
| CODE | NOTE                                                          |
|------|---------------------------------------------------------------|
| 200  | Sightings recorded successfully                               |
| 400  | Malformed request                                             |
| 401  | Request not from confirmed peer on main network               |

### POST /api/v1/invite/confirm/{key}

Description: Confirm peer presence on main network using public {key}. Finalizes redemption process and marks peer as operational.

Request: `null`

Response: `null`

Status:
| CODE | NOTE                                                          |
|------|---------------------------------------------------------------|
| 200  | Peer confirmed successfully                                   |
| 400  | Malformed request or invalid public key                       |
| 401  | Request not from valid main network IP                        |
| 404  | No peer found for public key                                  |

### POST /api/v1/invite/redeem/{key}

Description: Redeem invite with permanent public {key}. Called over invite network during peer redemption flow. Returns main network configuration for transition.

Request: `null`

Response: `Invite`
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

Status:
| CODE | NOTE                                                          |
|------|---------------------------------------------------------------|
| 200  | Invite redeemed successfully                                  |
| 400  | Malformed request or invalid public key                       |
| 401  | Request not from valid invite network IP                      |
| 404  | No redeemable invite for sender IP                            |
| 410  | Invite expired                                                |

### POST /api/v1/admin/peer

Description: Create peer invite. Generates temporary credentials and reserves IP on main network.

Request: `CreatePeerRequest`
```json
{
  "name": "string",
  "cidr": "string",
  "admin": "boolean"
}
```

Response: `AdminPeer`
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

Status:
| CODE | NOTE                                                          |
|------|---------------------------------------------------------------|
| 201  | Peer invite created successfully                              |
| 400  | Malformed request, invalid IP, or IP already assigned         |
| 401  | Request not from admin peer                                   |
| 409  | Peer name already exists                                      |

### GET /api/v1/admin/peers

Description: List all peers on the network. Includes admin/enabled/confirmed flags.

Request: `null`

Response: `[AdminPeer]`
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

Status:
| CODE | NOTE                          |
|------|-------------------------------|
| 200  | OK                            |
| 401  | Request not from admin peer   |

### GET /api/v1/admin/peer/{name}

Description: Get details for a single peer by name.

Request: `null`

Response: `AdminPeer`
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

Status:
| CODE | NOTE                          |
|------|-------------------------------|
| 200  | OK                            |
| 401  | Request not from admin peer   |
| 404  | Peer not found                |

### PATCH /api/v1/admin/peer/{name}

Description: Update peer properties including renaming, enabling, or disabling.

Request: `UpdatePeerRequest`
```json
{
  "name": "string, optional",
  "admin": "boolean, optional",
  "enabled": "boolean, optional"
}
```

Response: `AdminPeer`
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

Status:
| CODE | NOTE                                                          |
|------|---------------------------------------------------------------|
| 200  | Peer updated successfully                                     |
| 400  | Malformed request                                             |
| 401  | Request not from admin peer                                   |
| 404  | Peer not found                                                |
| 409  | New name conflicts with existing peer                         |

### DELETE /api/v1/admin/peer/{name}

Description: Delete peer and associated CIDR. Removes peer from network permanently.

Request: `null`

Response: `null`

Status:
| CODE | NOTE                                                          |
|------|---------------------------------------------------------------|
| 204  | Peer deleted successfully                                     |
| 401  | Request not from admin peer                                   |
| 404  | Peer not found                                                |

### POST /api/v1/admin/cidr

Description: Create child CIDR within network. Must fall within root CIDR range.

Request: `CreateCidrRequest`
```json
{
  "name": "string",
  "cidr": "string"
}
```

Response: `Cidr`
```json
{
  "name": "string",
  "cidr": "string",
  "length": "integer",
  "prefix": "integer"
}
```

Status:
| CODE | NOTE                                                          |
|------|---------------------------------------------------------------|
| 201  | CIDR created successfully                                     |
| 400  | Malformed request, invalid CIDR, or CIDR outside root range   |
| 401  | Request not from admin peer                                   |
| 409  | CIDR name or range already exists                             |

### GET /api/v1/admin/cidrs

Description: List all CIDRs configured on the network.

Request: `null`

Response: `[Cidr]`
```json
{
  "name": "string",
  "cidr": "string",
  "length": "integer",
  "prefix": "integer"
}
```

Status:
| CODE | NOTE                          |
|------|-------------------------------|
| 200  | OK                            |
| 401  | Request not from admin peer   |

### GET /api/v1/admin/cidr/{name}

Description: Get details for a single CIDR by name.

Request: `null`

Response: `Cidr`
```json
{
  "name": "string",
  "cidr": "string",
  "length": "integer",
  "prefix": "integer"
}
```

Status:
| CODE | NOTE                          |
|------|-------------------------------|
| 200  | OK                            |
| 401  | Request not from admin peer   |
| 404  | CIDR not found                |

### PATCH /api/v1/admin/cidr/{name}

Description: Rename existing CIDR.

Request: `RenameCidrRequest`
```json
{
  "name": "string"
}
```

Response: `Cidr`
```json
{
  "name": "string",
  "cidr": "string",
  "length": "integer",
  "prefix": "integer"
}
```

Status:
| CODE | NOTE                                                          |
|------|---------------------------------------------------------------|
| 200  | CIDR renamed successfully                                     |
| 400  | Malformed request                                             |
| 401  | Request not from admin peer                                   |
| 404  | CIDR not found                                                |
| 409  | New name conflicts with existing CIDR                         |

### DELETE /api/v1/admin/cidr/{name}

Description: Delete CIDR and all associations. Cannot delete root CIDR (id=1).

Request: `null`

Response: `null`

Status:
| CODE | NOTE                                                          |
|------|---------------------------------------------------------------|
| 204  | CIDR deleted successfully                                     |
| 401  | Request not from admin peer                                   |
| 404  | CIDR not found                                                |
| 409  | Cannot delete root CIDR                                       |

### POST /api/v1/admin/association

Description: Create association between two CIDRs, enabling peer communication across ranges.

Request: `Association`
```json
{
  "cidr1": "string",
  "cidr2": "string"
}
```

Response: `Association`
```json
{
  "cidr1": "string",
  "cidr2": "string"
}
```

Status:
| CODE | NOTE                                                          |
|------|---------------------------------------------------------------|
| 201  | Association created successfully                              |
| 400  | Malformed request, identical CIDRs, or CIDRs don't exist      |
| 401  | Request not from admin peer                                   |
| 409  | Association already exists                                    |

### GET /api/v1/admin/associations

Description: List all CIDR associations.

Request: `null`

Response: `[Association]`
```json
{
  "cidr1": "string",
  "cidr2": "string"
}
```

Status:
| CODE | NOTE                          |
|------|-------------------------------|
| 200  | OK                            |
| 401  | Request not from admin peer   |

### DELETE /api/v1/admin/association

Description: Delete association between CIDRs. Removes peer communication across ranges.

Request: `Association`
```json
{
  "cidr1": "string",
  "cidr2": "string"
}
```

Response: `null`

Status:
| CODE | NOTE                                                          |
|------|---------------------------------------------------------------|
| 204  | Association deleted successfully                              |
| 401  | Request not from admin peer                                   |
| 404  | Association not found                                         |


## 5) Authentication & Network Architecture

- **Dual Network Design:** The server operates two WireGuard interfaces:
  - **Invite Network** (e.g. 172.16.0.0/16:51821): Only exposes `/api/v1/invite/redeem`
  - **Main Network** (e.g. 10.0.0.0/8:51820): Exposes full API including admin endpoints
- **Authentication:** IP-based via WireGuard cryptographic identity
  - Server validates requesting peer exists in `peer` table for sender IP
  - Admin operations require `admin=1` flag on requesting peer
  - Invite network requests validated against `invite` table
- **Network Transitions:** Peers move from invite → main network during redemption
- **Peer Visibility:** Based on CIDR associations - peers only see others in same/associated ranges


## 6) Key Flows

- **Redemption:** `/api/v1/invite/redeem` (invite net) → `/api/v1/invite/confirm` (main net)
- **State Sync:** Periodic `/api/v1/peers` calls with WireGuard config updates
- **Endpoint Gossip:** `/api/v1/endpoint` submissions with peer endpoint observations
- **Administration:** Admin peers use `/api/v1/admin/*` endpoints for network management


## 7) Assumptions & Open Questions

- **Assumption:** All timestamps use Unix epoch integers for consistency with Go `time` package
- **Assumption:** Invite expiration handled server-side; expired invites return 410 status
- **Question:** Rate limiting needed for endpoint reporting to prevent gossip spam?
