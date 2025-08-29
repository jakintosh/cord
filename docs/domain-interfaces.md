# Server Domain Interfaces

The `internal/server` package defines the domain interfaces and all data types used by the API. The `internal/api` package imports these types and uses them directly for request deserialization and response serialization. There are no API-local DTOs or duplicate structs.

This document specifies those interfaces and structs, ensuring their JSON fields align with `docs/api-spec.md`.

## Principles

- Single source of truth: API uses server-exported types for all I/O (requests and responses).
- Clean boundaries: API never depends on DB rows or WireGuard internals; only on server interfaces and exported structs.
- Idempotency: where specified by the API, replays do not fail if state already matches.
- Input validation: return contextual errors (wrapped) for precise HTTP mapping.

---

## PublicDomain Interface

Public endpoints callable from peers over the main or invite WireGuard networks.

```go
package server

import "context"

type PublicDomain interface {

	// GetPeers returns the set of visible peers for the requester.
	// Excludes the requester, disabled peers, and unconfirmed peers.
	GetPeers(ctx context.Context) ([]PublicPeer, error)

	// ReportEndpoints records endpoint sightings (gossip) from a peer.
	// The reporting peer is derived from authentication context.
	ReportEndpoints(ctx context.Context, reports []EndpointSighting) error

	// ConfirmPeer marks a peer as present on the main network using {key}.
	// Idempotent: calling for an already-confirmed peer succeeds.
	ConfirmPeer(ctx context.Context, key string) error

	// RedeemInvite finalizes an invite using the permanent public key {key}.
	// The invite identity is inferred by the API auth layer (invite network).
	RedeemInvite(ctx context.Context, key string) (Invite, error)
}
```

Notes
- AuthZ/AuthN is enforced by middleware; request identity (invite vs main) is provided via `context`.
- Implementations look up the requester by derived identity to determine CIDR and associations.

---

## AdminDomain Interface

Administrative operations for managing peers, CIDRs, and associations. All calls require a peer with `admin=1` (enforced above the interface).

```go
import "context"

type AdminDomain interface {

	// Peer management
	CreatePeer(ctx context.Context, in CreatePeerRequest) (AdminPeer, error)
	ListPeers(ctx context.Context) ([]AdminPeer, error)
	GetPeer(ctx context.Context, name string) (AdminPeer, error)
	UpdatePeer(ctx context.Context, name string, in UpdatePeerRequest) (AdminPeer, error)
	DeletePeer(ctx context.Context, name string) error

	// CIDR management
	CreateCidr(ctx context.Context, in CreateCidrRequest) (Cidr, error)
	ListCidrs(ctx context.Context) ([]Cidr, error)
	GetCidr(ctx context.Context, name string) (Cidr, error)
	RenameCidr(ctx context.Context, name, newName string) (Cidr, error)
	DeleteCidr(ctx context.Context, name string) error

	// Association management (symmetric pairs)
	CreateAssociation(ctx context.Context, in Association) (Association, error)
	ListAssociations(ctx context.Context) ([]Association, error)
	DeleteAssociation(ctx context.Context, in Association) error
}
```

---

## Domain Models and Params

Server-exported types below are used directly by the API for JSON (de)serialization. Field tags align with `docs/api-spec.md`.

```go
type PublicPeer struct {
	Name      string             `json:"name"`
	Cidr      string             `json:"cidr"`
	PublicKey string             `json:"publicKey"`
	Endpoints []EndpointWitness  `json:"endpoints"`
}

type EndpointWitness struct {
	WitnessKey string `json:"witnessKey"`
	Endpoint   string `json:"endpoint"`
	Timestamp  int64  `json:"timestamp"`
}

type EndpointSighting struct {
	PeerKey    string `json:"peerKey"`
	WitnessKey string `json:"witnessKey"`
	Endpoint   string `json:"endpoint"`
	Timestamp  int64  `json:"timestamp"`
}

type Invite struct {
	Interface struct {
		NetworkName  string `json:"networkName"`
		PrivateKey   string `json:"privateKey"`
		AssignedCidr string `json:"assignedCidr"`
	} `json:"interface"`
	Server struct {
		PublicKey        string `json:"publicKey"`
		ExternalEndpoint string `json:"externalEndpoint"`
		InternalEndpoint string `json:"internalEndpoint"`
	} `json:"server"`
}

type AdminPeer struct {
	Name      string `json:"name"`
	Cidr      string `json:"cidr"`
	PublicKey string `json:"publicKey"`
	Admin     bool   `json:"admin"`
	Disabled  bool   `json:"disabled"`
	Confirmed bool   `json:"confirmed"`
}

type CreatePeerRequest struct {
	Name  string `json:"name"`
	Cidr  string `json:"cidr"`
	Admin bool   `json:"admin"`
}

type UpdatePeerRequest struct {
	Name    *string `json:"name,omitempty"`
	Admin   *bool   `json:"admin,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

type Cidr struct {
	Name   string `json:"name"`
	Cidr   string `json:"cidr"`
	Length int    `json:"length"`
	Prefix int    `json:"prefix"`
}

type CreateCidrRequest struct {
	Name string `json:"name"`
	Cidr string `json:"cidr"`
}

type Association struct {
	Cidr1 string `json:"cidr1"`
	Cidr2 string `json:"cidr2"`
}
```

---

## Error Semantics (mapping hints)

- Validation errors: invalid names, CIDR formats, or missing fields → `400`.
- AuthZ failures (checked above interfaces) → `401/403` (middleware responsibility).
- Not found (peer/CIDR/association) → `404`.
- Conflicts (duplicate names, ranges, existing association, rename collisions) → `409`.
- Idempotent success: confirm on already-confirmed peer, redeem already-redeemed invite → `200` with no change.

---

## Handler-to-Domain Mapping

- GET `/api/v1/peers` → `PublicDomain.GetPeers(ctx)` returns `[]PublicPeer`
- POST `/api/v1/report` → `PublicDomain.ReportEndpoints(ctx, []EndpointSighting)`
- POST `/api/v1/confirm/{key}` → `PublicDomain.ConfirmPeer(ctx, key)`
- POST `/api/v1/redeem/{key}` → `PublicDomain.RedeemInvite(ctx, key)` returns `Invite`
- POST `/api/v1/admin/peer` → `AdminDomain.CreatePeer(ctx, CreatePeerRequest)` returns `AdminPeer`
- GET `/api/v1/admin/peers` → `AdminDomain.ListPeers(ctx)` returns `[]AdminPeer`
- GET `/api/v1/admin/peer/{name}` → `AdminDomain.GetPeer(ctx, name)` returns `AdminPeer`
- PUT `/api/v1/admin/peer/{name}` → `AdminDomain.UpdatePeer(ctx, name, UpdatePeerRequest)` returns `AdminPeer`
- DELETE `/api/v1/admin/peer/{name}` → `AdminDomain.DeletePeer(ctx, name)`
- POST `/api/v1/admin/cidr` → `AdminDomain.CreateCidr(ctx, CreateCidrRequest)` returns `Cidr`
- GET `/api/v1/admin/cidrs` → `AdminDomain.ListCidrs(ctx)` returns `[]Cidr`
- GET `/api/v1/admin/cidr/{name}` → `AdminDomain.GetCidr(ctx, name)` returns `Cidr`
- PUT `/api/v1/admin/cidr/{name}` → `AdminDomain.RenameCidr(ctx, name, newName)` returns `Cidr`
- DELETE `/api/v1/admin/cidr/{name}` → `AdminDomain.DeleteCidr(ctx, name)`
- POST `/api/v1/admin/association` → `AdminDomain.CreateAssociation(ctx, Association)` returns `Association`
- GET `/api/v1/admin/associations` → `AdminDomain.ListAssociations(ctx)` returns `[]Association`
- DELETE `/api/v1/admin/association` → `AdminDomain.DeleteAssociation(ctx, Association)`
