package client

import (
	"database/sql"
	"fmt"
	"os"

	db "git.sr.ht/~jakintosh/cord/internal/database"
)

type Context struct {
	Db        *sql.DB
	Name      string
	ConfigDir string
	DataDir   string
}

// NewContext prepares a client context for a network. It ensures
// paths exist and opens the network-scoped SQLite database.
func NewContext(
	network string,
	configDir string,
	dataDir string,
) (*Context, error) {

	os.MkdirAll(configDir, 0755)
	database, err := db.Open(network, dataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	return &Context{
		Db:        database,
		Name:      network,
		ConfigDir: configDir,
		DataDir:   dataDir,
	}, nil
}

// Install consumes an invite to join a network. It follows the
// Peer Redemption flow: configure a temporary invite interface, make
// a permanent keypair, redeem, configure the main interface, fetch
// state, confirm with the server, then tear down the invite
// interface and persist local state.
func (ctx *Context) Install(
	invitePath string,
) error {

	// 1. Parse invite file: endpoint, invite keypair, assigned IP
	// 2. Initialize local database schema for new network
	// 3. Configure temporary WireGuard invite interface
	// 4. Generate permanent WireGuard keypair for this client
	// 5. POST /peer/redeem with permanent public key over invite net
	// 6. Persist returned main-network config and server pubkey
	// 7. Configure main WireGuard interface with permanent key/IP
	// 8. POST /peer/confirm over main network
	// 9. (On 200 OK) Tear down temporary invite interface
	// 10. Call `Fetch()` for initial network snapshot

	fmt.Printf(
		"Install\nInvite: %s\nNetwork: %s\nConfig: %s\nData: %s\n",
		invitePath, ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Uninstall removes all local client state for a network. It
// follows the Network Uninstallation flow: bring the interface
// down, remove generated configuration, delete the network's local
// SQLite database, and optionally clean directories. Idempotent.
func (ctx *Context) Uninstall() error {

	// 1. Attempt to bring the WireGuard interface down
	// 2. Remove any routes or OS-specific artifacts
	// 3. Delete generated WireGuard config files (if any)
	// 4. Close database handle if open
	// 5. Delete the network's SQLite database file
	// 6. Optionally remove empty config/data directories
	// 7. Return success even if components were already absent

	fmt.Printf(
		"Uninstall\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Show reports local client state for the network: wireguard
// interface information (including peers/keys), and recent endpoints.
// Read-only helper to inspect health.
func (ctx *Context) Show() error {

	// 1. Check existence and status of WireGuard interface
	// 2. Read local keys and assigned IP (if stored)
	// 3. Query local DB for peers and last endpoint sightings
	// 4. Print a concise report of the current local state

	fmt.Printf(
		"Show\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// ShowAll reports local state for all installed networks. It enumerates
// data/config directories, opens each network context, and prints the
// same information as Show() per network.
func ShowAll(configDir string, dataDir string) error {

	// 1. Enumerate networks from dataDir/configDir
	// 2. For each network, open a lightweight context
	// 3. Reuse Show() logic to collect local details
	// 4. Print aggregated results in a readable order

	fmt.Printf(
		"Show All\nData: %s\n",
		dataDir,
	)
	return nil
}

// Fetch updates local state from the server. It follows the Peer
// State Query flow: request visible peers and their latest endpoints,
// upsert into the local DB, delete any peers not returned, and prepare
// interface updates. Idempotent and safe to call before bringing the
// interface up.
func (ctx *Context) Fetch() error {

	// 1. Retrieve server endpoint
	// 2. GET peers for this network over the main interface
	// 3. Parse JSON list of confirmed, enabled peers
	// 4. Upsert peers into local DB (keys, names, IPs)
	// 5. Delete peers in local DB that were not returned
	// 6. Create wireguard peer with initial endpoint from server
	// 7. Compute add/remove/update sets for interface peers
	// 8. Return without mutating if nothing changed

	fmt.Printf(
		"Fetch\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Up constructs and applies the WireGuard configuration for this
// network. It aligns with the Interface Management flow: optionally
// fetch state, build device and peer configs from local DB, write the
// interface using OS APIs, set routes, and bring it up. Idempotent.
func (ctx *Context) Up(fetch bool) error {

	// 1. Optionally call Fetch() to refresh local peer state
	// 2. Load local keys, IP/CIDR, and listen port
	// 3. Build DeviceConfig structure for the interface
	// 4. Build PeerConfig entries from local DB peers
	// 5. Write configuration via wgctrl/netlink
	// 6. Ensure routing rules are present (unless disabled)
	// 7. Bring the interface up; handle already-up idempotently

	fmt.Printf(
		"Up\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Down deactivates the WireGuard interface. Part of the Interface
// Management flow: remove routes, bring the interface down, and clean
// ephemeral state. Safe to call if the interface is already down.
func (ctx *Context) Down() error {

	// 1. Locate the WireGuard interface for this network
	// 2. Remove routing rules associated with the interface
	// 3. Bring the interface down using OS APIs
	// 4. Optionally delete transient configuration artifacts
	// 5. Exit cleanly if the interface does not exist

	fmt.Printf(
		"Down\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Watch runs the Endpoint Discovery flow continuously. It periodically
// scans the interface, records endpoint changes, and syncs updates to
// the server. Long‑running and cancelable; intended to run after Up.
func (ctx *Context) Watch() error {

	// 1. Initialize a ticker with the configured interval
	// 2. On each tick, invoke Scan() to detect endpoint changes
	// 3. If changes or reporting TTL elapsed, call Sync()
	// 4. Handle shutdown via signal or context cancellation
	// 5. Backoff on transient errors; keep loop running

	fmt.Printf(
		"Down\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Scan inspects the WireGuard interface to observe current peer
// endpoints. It detects changes against the local database and stores
// updated endpoints with timestamps for later Sync().
func (ctx *Context) Scan() error {

	// 1. Query OS/WireGuard for current peer endpoint list
	// 2. Read last-known endpoints for peers from local DB
	// 3. Compare and identify changed endpoints; make sure wg endpoint is recent
	// 4. Upsert changed endpoints with current timestamp

	fmt.Printf(
		"Down\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Sync sends locally observed endpoint changes to the server. Part
// of the Endpoint Gossip flow: post only changed endpoints with
// timestamps, update local reporting state, and optionally refresh
// recent endpoints for peers.
func (ctx *Context) Sync() error {

	// 1. Gather endpoint changes since last report or TTL
	// 2. Build request payload of peer keys, endpoints, timestamps
	// 3. POST to server endpoint sighting API over main network
	// 4. On success, mark records as reported with time
	// 5. Optionally fetch refreshed endpoint set and upsert

	fmt.Printf(
		"Down\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// initNetworkDb initializes local client tables used by Install and
// endpoint gossip: peers and endpoint sightings. Enables foreign keys
// and creates tables if absent. No-op if already initialized.
func initNetworkDb(d *sql.DB) error {

	if err := db.EnableForeignKeys(d); err != nil {
		return err
	}

	if err := db.InitTable(d, "peer", `
		CREATE TABLE IF NOT EXISTS peer (
			id                INTEGER PRIMARY KEY,
			public_key        TEXT NOT NULL UNIQUE,
			name              TEXT NOT NULL UNIQUE,
			ip                INTEGER NOT NULL UNIQUE
		);
	`); err != nil {
		return err
	}

	if err := db.InitTable(d, "endpoint", `
		CREATE TABLE IF NOT EXISTS endpoint (
			id                INTEGER PRIMARY KEY,
			peer_key          TEXT NOT NULL,        -- target peer's public key
			witness_key       TEXT NOT NULL,        -- witnessing peer's public key
			endpoint          TEXT NOT NULL,        -- ip:port endpoint
			time              INTEGER NOT NULL,     -- witness timestamp
		);
	`); err != nil {
		return err
	}

	return nil
}
