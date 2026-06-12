package client

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path"
	"syscall"
	"time"

	"git.sr.ht/~jakintosh/cord/internal/server"
	wg "git.sr.ht/~jakintosh/cord/internal/wireguard"
)

const (
	syncInterval   = 25 * time.Second
	keepalive      = 25 * time.Second
	handshakeFresh = 3 * time.Minute
)

// Context bundles the identity and storage locations for one network.
type Context struct {
	Name      string
	ConfigDir string
	DataDir   string
	Backend   wg.BackendType
}

// NewContext prepares a client context for a network. The name may be
// empty for install, which learns it from the invite file.
func NewContext(
	networkName string,
	configDir string,
	dataDir string,
) (*Context, error) {
	return &Context{
		Name:      networkName,
		ConfigDir: configDir,
		DataDir:   dataDir,
		Backend:   wg.BackendAuto,
	}, nil
}

// Install consumes an invite to join a network: bring up a temporary
// interface on the invite network, redeem a permanent identity, prove
// presence on the main network, then tear everything down. The
// permanent keypair is persisted before redemption so the flow can be
// retried safely.
func (ctx *Context) Install(
	invitePath string,
) error {
	// parse the invite file
	file, err := os.Open(invitePath)
	if err != nil {
		return fmt.Errorf("failed to open invite: %w", err)
	}
	invite, err := server.ReadPeerInvite(file)
	file.Close()
	if err != nil {
		return err
	}
	if invite.Interface.NetworkName == "" {
		return fmt.Errorf("invite has no network name")
	}
	ctx.Name = invite.Interface.NetworkName

	// load or create the permanent identity; persisting it first
	// makes redemption retryable
	cfg, err := ctx.installIdentity()
	if err != nil {
		return err
	}

	// stand up the temporary invite interface
	inviteIface, err := ctx.buildInviteInterface(invite)
	if err != nil {
		return err
	}
	if err := inviteIface.Up(""); err != nil {
		return fmt.Errorf("failed to bring up invite interface: %w", err)
	}
	defer func() { _ = inviteIface.Down(true) }()
	fmt.Printf("invite interface up (%s), redeeming...\n", inviteIface.DeviceName())

	// redeem over the invite network
	inviteApi := newApiClient(invite.Server.InternalEndpoint)
	result, err := inviteApi.redeem(cfg.PublicKey)
	if err != nil {
		return fmt.Errorf("failed to redeem invite: %w", err)
	}

	// persist the permanent network assignment
	cfg.AssignedCidr = result.AssignedCidr
	cfg.Server = result.Server
	if err := ctx.SaveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("redeemed: assigned %s\n", cfg.AssignedCidr)

	// stand up the main interface and confirm presence
	mainIface, err := ctx.buildMainInterface(cfg, nil)
	if err != nil {
		return err
	}
	if err := mainIface.Up(""); err != nil {
		return fmt.Errorf("failed to bring up main interface: %w", err)
	}
	defer func() { _ = mainIface.Down(true) }()

	mainApi := newApiClient(cfg.Server.InternalEndpoint)
	if err := mainApi.confirm(cfg.PublicKey); err != nil {
		return fmt.Errorf("failed to confirm peer: %w", err)
	}
	fmt.Println("confirmed on main network")

	// seed the local peer database
	d, err := ctx.openDb()
	if err != nil {
		return err
	}
	defer d.Close()
	if peers, err := mainApi.peers(); err == nil {
		if err := reconcilePeers(d, peers); err != nil {
			return err
		}
		fmt.Printf("fetched %d peer(s)\n", len(peers))
	} else {
		fmt.Printf("warning: initial peer fetch failed: %v\n", err)
	}

	fmt.Printf("installed network '%s'; run 'cord up %s' to connect\n", ctx.Name, ctx.Name)
	return nil
}

// installIdentity loads the persisted keypair for this network or
// generates and persists a fresh one.
func (ctx *Context) installIdentity() (*ClientConfig, error) {
	if ctx.HasConfig() {
		cfg, err := ctx.LoadConfig()
		if err != nil {
			return nil, err
		}
		fmt.Println("reusing existing keypair from previous install attempt")
		return cfg, nil
	}

	key, err := wg.GeneratePrivateKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate keypair: %w", err)
	}

	cfg := &ClientConfig{
		NetworkName: ctx.Name,
		PrivateKey:  key.String(),
		PublicKey:   key.PublicKey().String(),
	}
	if err := ctx.SaveConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Uninstall removes all local state for a network: interface, config,
// and database. Idempotent.
func (ctx *Context) Uninstall() error {
	// best effort: tear down a lingering interface (kernel backend)
	if err := ctx.Down(); err != nil {
		fmt.Printf("warning: could not bring interface down: %v\n", err)
	}

	if err := ctx.DeleteConfig(); err != nil {
		return err
	}
	if err := ctx.deleteDb(); err != nil {
		return err
	}
	if err := os.Remove(ctx.confPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete wg config: %w", err)
	}

	fmt.Printf("uninstalled network '%s'\n", ctx.Name)
	return nil
}

// Show prints the local state for one network or, with no network
// selected, lists the installed networks.
func (ctx *Context) Show() error {
	if ctx.Name == "" {
		return ctx.showInstalled()
	}

	cfg, err := ctx.LoadConfig()
	if err != nil {
		return err
	}

	fmt.Printf("network:  %s\n", cfg.NetworkName)
	fmt.Printf("address:  %s\n", cfg.AssignedCidr)
	fmt.Printf("pubkey:   %s\n", cfg.PublicKey)
	fmt.Printf("server:   %s (api %s)\n", cfg.Server.ExternalEndpoint, cfg.Server.InternalEndpoint)

	d, err := ctx.openDb()
	if err != nil {
		return err
	}
	defer d.Close()

	peers, err := listLocalPeers(d)
	if err != nil {
		return err
	}
	fmt.Printf("peers:    %d\n", len(peers))
	for _, peer := range peers {
		line := fmt.Sprintf("  %-20s %-18s", peer.Name, peer.Cidr)
		if peer.Endpoint != "" {
			line += " last seen at " + peer.Endpoint
		}
		fmt.Println(line)
	}

	return nil
}

func (ctx *Context) showInstalled() error {
	entries, err := os.ReadDir(ctx.ConfigDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("no networks installed")
			return nil
		}
		return fmt.Errorf("failed to read config dir: %w", err)
	}

	found := false
	for _, entry := range entries {
		name := entry.Name()
		if path.Ext(name) == ".toml" {
			fmt.Println(name[:len(name)-len(".toml")])
			found = true
		}
	}
	if !found {
		fmt.Println("no networks installed")
	}
	return nil
}

// Fetch updates the local peer database from the server. It requires
// the network to be reachable (interface up in another process, or the
// server's API otherwise routable).
func (ctx *Context) Fetch() error {
	cfg, err := ctx.LoadConfig()
	if err != nil {
		return err
	}

	d, err := ctx.openDb()
	if err != nil {
		return err
	}
	defer d.Close()

	peers, err := newApiClient(cfg.Server.InternalEndpoint).peers()
	if err != nil {
		return fmt.Errorf("failed to fetch peers: %w", err)
	}
	if err := reconcilePeers(d, peers); err != nil {
		return err
	}

	fmt.Printf("fetched %d peer(s)\n", len(peers))
	return nil
}

// Up connects to the network and stays in the foreground: it keeps the
// local peer set in sync with the server and reports endpoint changes
// it witnesses (endpoint gossip) until interrupted.
func (ctx *Context) Up(noFetch bool) error {
	cfg, err := ctx.LoadConfig()
	if err != nil {
		return err
	}

	d, err := ctx.openDb()
	if err != nil {
		return err
	}
	defer d.Close()

	apiClient := newApiClient(cfg.Server.InternalEndpoint)

	peers, err := listLocalPeers(d)
	if err != nil {
		return err
	}

	iface, err := ctx.buildMainInterface(cfg, peers)
	if err != nil {
		return err
	}
	if err := iface.Up(ctx.confPath()); err != nil {
		return fmt.Errorf("failed to bring up interface: %w", err)
	}
	defer func() { _ = iface.Down(true) }()
	fmt.Printf("interface up: %s (%s)\n", iface.DeviceName(), cfg.AssignedCidr)

	// initial fetch now that the network is reachable
	if !noFetch {
		if err := ctx.syncOnce(cfg, d, apiClient, iface); err != nil {
			fmt.Printf("warning: sync failed: %v\n", err)
		}
	}

	sigCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()

	fmt.Println("connected; press ctrl-c to disconnect")
	for {
		select {
		case <-sigCtx.Done():
			fmt.Println("\ndisconnecting")
			return nil
		case <-ticker.C:
			if err := ctx.syncOnce(cfg, d, apiClient, iface); err != nil {
				fmt.Printf("warning: sync failed: %v\n", err)
			}
		}
	}
}

// syncOnce performs one round of the state synchronization flow:
// report witnessed endpoint changes, refresh the peer list, and apply
// any changes to the live interface.
func (ctx *Context) syncOnce(
	cfg *ClientConfig,
	d *sql.DB,
	apiClient *apiClient,
	iface *wg.Interface,
) error {
	// gossip: compare live endpoints against the local database
	if sightings := ctx.scanEndpoints(d, iface); len(sightings) > 0 {
		if err := apiClient.reportEndpoints(sightings); err != nil {
			fmt.Printf("warning: endpoint report failed: %v\n", err)
		}
	}

	// pull the server's view of the network
	peers, err := apiClient.peers()
	if err != nil {
		return err
	}
	if err := reconcilePeers(d, peers); err != nil {
		return err
	}

	// rebuild and apply the interface peer list
	localPeers, err := listLocalPeers(d)
	if err != nil {
		return err
	}
	wgPeers, err := ctx.buildPeers(cfg, localPeers)
	if err != nil {
		return err
	}
	iface.SetPeers(wgPeers)
	return iface.Sync()
}

// scanEndpoints inspects the live device for peers whose endpoint
// changed since we last saw them, records the changes locally, and
// returns sightings to report to the server.
func (ctx *Context) scanEndpoints(
	d *sql.DB,
	iface *wg.Interface,
) []server.EndpointSighting {
	status, err := iface.Status()
	if err != nil {
		return nil
	}

	known, err := listLocalPeers(d)
	if err != nil {
		return nil
	}
	knownByKey := map[string]LocalPeer{}
	for _, peer := range known {
		knownByKey[peer.PublicKey] = peer
	}

	now := time.Now()
	var sightings []server.EndpointSighting
	for _, peerStatus := range status.Peers {
		// only trust endpoints with a recent handshake
		if peerStatus.Endpoint == nil {
			continue
		}
		if now.Sub(peerStatus.LastHandshake) > handshakeFresh {
			continue
		}

		key := peerStatus.PublicKey.String()
		local, ok := knownByKey[key]
		if !ok || local.Endpoint == peerStatus.Endpoint.String() {
			continue
		}

		endpoint := peerStatus.Endpoint.String()
		if err := updateLocalEndpoint(d, key, endpoint, now.Unix()); err != nil {
			continue
		}
		sightings = append(sightings, server.EndpointSighting{
			PeerKey:   key,
			Endpoint:  endpoint,
			Timestamp: now.Unix(),
		})
	}

	return sightings
}

// Down tears down the network interface. On Linux this removes a
// kernel WireGuard device left by another process; with the userspace
// backend the interface lives and dies with its 'cord up' process.
func (ctx *Context) Down() error {
	cfg, err := ctx.LoadConfig()
	if err != nil {
		return err
	}

	iface, err := ctx.buildMainInterface(cfg, nil)
	if err != nil {
		return err
	}
	return iface.Down(true)
}

func (ctx *Context) confPath() string {
	return path.Join(ctx.DataDir, ctx.Name+".conf")
}

// buildInviteInterface constructs the temporary interface described by
// an invite: the server is the only peer, reachable at its public
// invite endpoint.
func (ctx *Context) buildInviteInterface(
	invite *server.PeerInvite,
) (
	*wg.Interface,
	error,
) {
	tempKey, err := wg.ParseKey(invite.Interface.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid invite private key: %w", err)
	}

	ip, inviteNet, err := net.ParseCIDR(invite.Interface.AssignedCidr)
	if err != nil {
		return nil, fmt.Errorf("invalid invite cidr: %w", err)
	}

	iface, err := wg.NewInterface(
		ctx.Name+"-invite",
		tempKey,
		net.IPNet{IP: ip, Mask: inviteNet.Mask},
		0,
		ctx.Backend,
	)
	if err != nil {
		return nil, err
	}

	serverPeer, err := buildServerPeer(
		invite.Server.PublicKey,
		invite.Server.ExternalEndpoint,
		inviteNet,
	)
	if err != nil {
		return nil, err
	}
	iface.AddPeer(*serverPeer)

	return iface, nil
}

// buildMainInterface constructs the permanent interface from the
// client config and the local peer database.
func (ctx *Context) buildMainInterface(
	cfg *ClientConfig,
	peers []LocalPeer,
) (
	*wg.Interface,
	error,
) {
	privKey, err := wg.ParseKey(cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	address, err := cfg.Address()
	if err != nil {
		return nil, err
	}

	iface, err := wg.NewInterface(ctx.Name, privKey, *address, 0, ctx.Backend)
	if err != nil {
		return nil, err
	}

	wgPeers, err := ctx.buildPeers(cfg, peers)
	if err != nil {
		return nil, err
	}
	iface.SetPeers(wgPeers)

	return iface, nil
}

// buildPeers converts the server peer plus local peer records into
// WireGuard peers. The server's allowed IPs cover the whole network so
// it can relay until direct peer connections are established; peers'
// narrower allowed IPs take precedence via longest-prefix match.
func (ctx *Context) buildPeers(
	cfg *ClientConfig,
	peers []LocalPeer,
) (
	[]wg.Peer,
	error,
) {
	network, err := cfg.NetworkCidr()
	if err != nil {
		return nil, err
	}

	serverPeer, err := buildServerPeer(
		cfg.Server.PublicKey,
		cfg.Server.ExternalEndpoint,
		network,
	)
	if err != nil {
		return nil, err
	}

	wgPeers := []wg.Peer{*serverPeer}
	for _, peer := range peers {
		if peer.PublicKey == cfg.Server.PublicKey {
			continue
		}

		key, err := wg.ParseKey(peer.PublicKey)
		if err != nil {
			fmt.Printf("warning: skipping peer '%s': invalid key\n", peer.Name)
			continue
		}
		_, allowed, err := net.ParseCIDR(peer.Cidr)
		if err != nil {
			fmt.Printf("warning: skipping peer '%s': invalid cidr\n", peer.Name)
			continue
		}

		wgPeer := wg.Peer{
			PublicKey:           key,
			AllowedIPs:          []net.IPNet{*allowed},
			PersistentKeepalive: keepalive,
		}
		if peer.Endpoint != "" {
			if addr, err := net.ResolveUDPAddr("udp", peer.Endpoint); err == nil {
				wgPeer.Endpoint = addr
			}
		}
		wgPeers = append(wgPeers, wgPeer)
	}

	return wgPeers, nil
}

func buildServerPeer(
	pubKey string,
	endpoint string,
	allowed *net.IPNet,
) (
	*wg.Peer,
	error,
) {
	key, err := wg.ParseKey(pubKey)
	if err != nil {
		return nil, fmt.Errorf("invalid server public key: %w", err)
	}

	addr, err := net.ResolveUDPAddr("udp", endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid server endpoint '%s': %w", endpoint, err)
	}

	return &wg.Peer{
		PublicKey:           key,
		AllowedIPs:          []net.IPNet{*allowed},
		Endpoint:            addr,
		PersistentKeepalive: keepalive,
	}, nil
}
