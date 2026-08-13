package service

import "time"

// Store is the persistence contract required by the client service.
// It uses domain vocabulary and domain types. The concrete SQLite
// implementation lives in internal/client/database.
type Store interface {
	// Install records (transient, consumed at confirm).

	GetInstall(name string) (*Install, error)
	ListInstalls() ([]*Install, error)

	// BeginInstall creates an invited install or returns its compatible
	// existing install.
	//
	// Persisted preconditions:
	//   - No completed network has the requested name.
	//   - An existing install with the name has the same invitation identity
	//     and local options.
	//
	// Atomic effects:
	//   - When absent, an invited install is created with the supplied
	//     permanent key.
	//
	// Retry:
	//   - A compatible retry returns the existing install unchanged, including
	//     its original permanent key and any later redeemed phase.
	//   - An incompatible retry fails with ErrConflict.
	//
	// Errors:
	//   - ErrNetworkExists when the name belongs to a completed network.
	//   - ErrConflict when an existing install is incompatible.
	BeginInstall(
		params BeginInstallParams,
	) (
		*Install,
		error,
	)

	// RedeemInstall records the main-network identity returned by redemption
	// and returns the persisted install.
	//
	// Persisted preconditions:
	//   - The install exists and is invited, or is already redeemed with the
	//     same result.
	//
	// Atomic effects:
	//   - The install becomes redeemed with its main route and server identity.
	//
	// Retry:
	//   - Repeating the same redemption succeeds unchanged.
	//   - A different redemption result fails with ErrConflict.
	//
	// Errors:
	//   - ErrNotFound when neither an install nor network exists.
	//   - ErrInstallState when the network is already completed.
	//   - ErrConflict for an incompatible redemption retry.
	RedeemInstall(
		name string,
		assignment NetworkAssignment,
	) (
		*Install,
		error,
	)

	// ConfirmInstall promotes a redeemed install and returns the persisted
	// network.
	//
	// Persisted preconditions:
	//   - The install is redeemed with the supplied permanent identity, or the
	//     network was already completed with that identity.
	//
	// Atomic effects:
	//   - A NetworkConfig is created from the authoritative install fields.
	//   - The transient install is deleted.
	//
	// Retry:
	//   - Repeating after completion with the same permanent identity returns
	//     the existing network unchanged.
	//
	// Errors:
	//   - ErrNotFound when neither record exists.
	//   - ErrInstallState when the install remains invited.
	//   - ErrConflict if an incompatible network record already exists.
	ConfirmInstall(
		name string,
		mainPrivateKey string,
		confirmedAt time.Time,
	) (
		*NetworkConfig,
		error,
	)

	// Network records (permanent membership).

	GetNetwork(
		name string,
	) (
		*NetworkConfig,
		error,
	)
	ListNetworkNames() (
		[]string,
		error,
	)
	ListNetworks() (
		[]*NetworkConfig,
		error,
	)

	// DeleteNetworkState removes all local state for a completed or in-progress
	// network.
	//
	// Persisted preconditions:
	//   - A completed network or install with the name exists.
	//
	// Atomic effects:
	//   - The network and install records are deleted.
	//   - Peer and endpoint state is deleted by foreign-key cascades.
	//
	// Retry:
	//   - Repeating after deletion fails with ErrNotFound.
	//
	// Errors:
	//   - ErrNotFound when neither record exists.
	DeleteNetworkState(
		name string,
	) error

	// SetNetworkEnabled updates a completed network's enabled intent.
	//
	// Persisted preconditions:
	//   - The completed network exists.
	//
	// Atomic effects:
	//   - The enabled intent is set to the requested value.
	//
	// Retry:
	//   - Repeating the same value succeeds unchanged.
	//
	// Errors:
	//   - ErrNotFound when the completed network does not exist.
	SetNetworkEnabled(
		name string,
		enabled bool,
	) error

	// UpdateNetwork applies independently editable local options.
	//
	// Persisted preconditions:
	//   - The completed network exists.
	//
	// Atomic effects:
	//   - Supplied options replace their persisted values.
	//
	// Retry:
	//   - Repeating the same update succeeds unchanged.
	//
	// Errors:
	//   - ErrNotFound when the completed network does not exist.
	UpdateNetwork(
		name string,
		update NetworkOptions,
	) error

	// Peer cache within a network.

	// ApplyNetworkReconciliation atomically reconciles cached peers and replaces
	// the cached topology with one complete server view.
	//
	// Persisted preconditions:
	//   - The completed network exists.
	//
	// Atomic effects:
	//   - Observed peers are upserted by public key.
	//   - Peers absent from the reconciliation are deleted.
	//   - Server endpoint observations are merged monotonically.
	//   - Local observations and endpoint-attempt history are retained.
	//   - Endpoints older than PruneBefore in both observation channels are
	//     deleted.
	//   - The prior topology projection is replaced in full.
	//
	// Retry:
	//   - Reapplying the same reconciliation succeeds unchanged.
	//
	// Errors:
	//   - ErrNotFound when the completed network does not exist.
	//   - ErrInvalidInput when the topology projection is malformed.
	//   - ErrConflict when the reconciliation contains duplicate peer public keys.
	//   - ErrConflict when peer identities conflict with durable uniqueness
	//     rules.
	ApplyNetworkReconciliation(
		network string,
		reconciliation NetworkReconciliation,
	) error

	// GetNetworkTopology returns the last complete projected topology and its
	// server generation and local synchronization times. It returns ErrNotFound
	// when the network is absent and ErrTopologyUnavailable before its first
	// successful synchronization.
	GetNetworkTopology(
		network string,
	) (
		*CachedTopology,
		error,
	)

	// ListPeers returns all cached peers for the named network,
	// ordered by name ascending. Each peer's Endpoint field is
	// populated with the best known endpoint from the endpoint
	// table (most recently observed locally, then by the server).
	// It returns ErrNotFound when the completed network does not exist.
	ListPeers(
		network string,
	) (
		[]*Peer,
		error,
	)

	// Endpoint catalog within a network.

	// RecordLocalEndpoint records a locally observed peer endpoint.
	//
	// Persisted preconditions:
	//   - The completed network and peer exist.
	//
	// Atomic effects:
	//   - The endpoint is created or its local observation time advances.
	//
	// Retry:
	//   - Repeating the same or an older observation succeeds unchanged.
	//
	// Errors:
	//   - ErrNotFound when the network or peer does not exist.
	RecordLocalEndpoint(
		network string,
		pubKey string,
		endpoint string,
		observedAt time.Time,
	) error

	// RecordEndpointAttempt records a successful device endpoint change.
	//
	// Persisted preconditions:
	//   - The completed network, peer, and endpoint exist.
	//
	// Atomic effects:
	//   - The endpoint's last-attempted time advances.
	//
	// Retry:
	//   - Repeating the same or an older attempt succeeds unchanged.
	//
	// Errors:
	//   - ErrNotFound when the network, peer, or endpoint does not exist.
	RecordEndpointAttempt(
		network string,
		pubKey string,
		endpoint string,
		attemptedAt time.Time,
	) error

	// ListPeerEndpoints returns all known endpoints for a peer,
	// ordered by local observation then server observation, newest first.
	ListPeerEndpoints(
		network string,
		pubKey string,
	) (
		[]PeerEndpoint,
		error,
	)

	// ListLocalEndpointsSince returns endpoints across all peers of
	// the named network with local_observed_at at or after since.
	ListLocalEndpointsSince(
		network string,
		since time.Time,
	) (
		[]EndpointSighting,
		error,
	)

	// Close releases the database connection.
	Close() error
}
