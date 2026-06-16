package client

import (
	"context"
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
	syncInterval           = 25 * time.Second
	keepalive              = 25 * time.Second
	handshakeFresh         = 3 * time.Minute
	inviteHandshakeTimeout = 20 * time.Second
)

// Client manages one network on this machine: its identity, its
// WireGuard interface, and its local peer cache.
type Client struct {
	Network   string
	ConfigDir string
	DataDir   string
	Backend   wg.BackendType
	Verbose   bool

	store PeerStore
}

// Options configures a Client. The store is built by the caller (the
// composition root) and passed in ready for use.
type Options struct {
	Network   string
	ConfigDir string
	DataDir   string
	Store     PeerStore
	Verbose   bool
}

// New prepares a client for a network.
func New(opts Options) (*Client, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("client requires a peer store")
	}
	return &Client{
		Network:   opts.Network,
		ConfigDir: opts.ConfigDir,
		DataDir:   opts.DataDir,
		Backend:   wg.BackendAuto,
		Verbose:   opts.Verbose,
		store:     opts.Store,
	}, nil
}

// Install consumes an invite to join a network: bring up a temporary
// interface on the invite network, redeem a permanent identity, prove
// presence on the main network, then tear everything down. The
// permanent keypair is persisted before redemption so the flow can be
// retried safely.
func (c *Client) Install(
	invite *server.PeerInvite,
) error {
	// load or create the permanent identity; persisting it first
	// makes redemption retryable
	cfg, err := c.installIdentity()
	if err != nil {
		return err
	}

	// stand up the temporary invite interface
	inviteIface, err := c.buildInviteInterface(invite)
	if err != nil {
		return err
	}
	if err := inviteIface.Up(""); err != nil {
		return fmt.Errorf("failed to bring up invite interface: %w", err)
	}
	defer func() { _ = inviteIface.Down(true) }()
	fmt.Printf("invite interface up (%s), checking WireGuard handshake...\n", inviteIface.DeviceName())
	c.verbosef("invite address: %s", invite.Interface.AssignedCidr)
	c.verbosef("invite WireGuard endpoint: %s", invite.Server.ExternalEndpoint)
	c.verbosef("invite API endpoint through tunnel: %s", invite.Server.InternalEndpoint)
	started := time.Now()
	if err := c.probeInviteTunnel(inviteIface, invite); err != nil {
		return err
	}
	c.verbosef("invite handshake completed in %s", time.Since(started).Round(time.Millisecond))
	fmt.Println("invite WireGuard handshake complete; redeeming...")

	// redeem over the invite network
	inviteApi := newApiClient(invite.Server.InternalEndpoint)
	result, err := inviteApi.redeem(cfg.PublicKey)
	if err != nil {
		return fmt.Errorf(
			"invite WireGuard handshake succeeded, but API redemption at %s failed: %w",
			invite.Server.InternalEndpoint,
			err,
		)
	}

	// persist the permanent network assignment
	cfg.AssignedCidr = result.AssignedCidr
	cfg.Server = result.Server
	if err := c.SaveConfig(cfg); err != nil {
		return err
	}
	fmt.Printf("redeemed: assigned %s\n", cfg.AssignedCidr)

	// stand up the main interface and confirm presence
	mainIface, err := c.buildMainInterface(cfg, nil)
	if err != nil {
		return err
	}
	if err := mainIface.Up(""); err != nil {
		return fmt.Errorf("failed to bring up main interface: %w", err)
	}
	defer func() { _ = mainIface.Down(true) }()
	fmt.Printf("main interface up (%s), checking WireGuard handshake...\n", mainIface.DeviceName())
	c.verbosef("main address: %s", cfg.AssignedCidr)
	c.verbosef("main WireGuard endpoint: %s", cfg.Server.ExternalEndpoint)
	c.verbosef("main API endpoint through tunnel: %s", cfg.Server.InternalEndpoint)
	started = time.Now()
	if err := c.probeServerTunnel(
		mainIface,
		cfg.Server,
		"main",
		"check UDP forwarding/firewall for the main WireGuard port, that the server main interface is running, and that the main CIDR does not conflict with a local network",
	); err != nil {
		return err
	}
	c.verbosef("main handshake completed in %s", time.Since(started).Round(time.Millisecond))
	fmt.Println("main WireGuard handshake complete; confirming...")

	mainApi := newApiClient(cfg.Server.InternalEndpoint)
	if err := mainApi.confirm(cfg.PublicKey); err != nil {
		return fmt.Errorf(
			"main WireGuard handshake succeeded, but API confirmation at %s failed: %w",
			cfg.Server.InternalEndpoint,
			err,
		)
	}
	fmt.Println("confirmed on main network")

	// seed the local peer cache
	if peers, err := mainApi.peers(); err == nil {
		if err := c.store.ReconcilePeers(peers); err != nil {
			return err
		}
		fmt.Printf("fetched %d peer(s)\n", len(peers))
	} else {
		fmt.Printf("warning: initial peer fetch failed: %v\n", err)
	}

	fmt.Printf("installed network '%s'; run 'cord client up %s' to connect\n", c.Network, c.Network)
	return nil
}

func (c *Client) verbosef(format string, args ...any) {
	if c.Verbose {
		fmt.Printf("verbose: "+format+"\n", args...)
	}
}

// installIdentity loads the persisted keypair for this network or
// generates and persists a fresh one.
func (c *Client) installIdentity() (
	*ClientConfig,
	error,
) {
	if c.HasConfig() {
		cfg, err := c.LoadConfig()
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
		NetworkName: c.Network,
		PrivateKey:  key.String(),
		PublicKey:   key.PublicKey().String(),
	}
	if err := c.SaveConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Uninstall removes all local state for a network: interface, config,
// and peer cache. Idempotent.
func (c *Client) Uninstall() error {
	// best effort: tear down a lingering interface (kernel backend)
	if err := c.Down(); err != nil {
		fmt.Printf("warning: could not bring interface down: %v\n", err)
	}

	if err := c.DeleteConfig(); err != nil {
		return err
	}
	if err := c.store.Delete(); err != nil {
		return err
	}
	if err := os.Remove(c.confPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete wg config: %w", err)
	}

	fmt.Printf("uninstalled network '%s'\n", c.Network)
	return nil
}

// Show prints the local state for this network.
func (c *Client) Show() error {
	cfg, err := c.LoadConfig()
	if err != nil {
		return err
	}

	fmt.Printf("network:  %s\n", cfg.NetworkName)
	fmt.Printf("address:  %s\n", cfg.AssignedCidr)
	fmt.Printf("pubkey:   %s\n", cfg.PublicKey)
	fmt.Printf("server:   %s (api %s)\n", cfg.Server.ExternalEndpoint, cfg.Server.InternalEndpoint)

	peers, err := c.store.ListPeers()
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

// Fetch updates the local peer cache from the server. It requires the
// network to be reachable (interface up in another process, or the
// server's API otherwise routable).
func (c *Client) Fetch() error {
	cfg, err := c.LoadConfig()
	if err != nil {
		return err
	}

	peers, err := newApiClient(cfg.Server.InternalEndpoint).peers()
	if err != nil {
		return fmt.Errorf("failed to fetch peers: %w", err)
	}
	if err := c.store.ReconcilePeers(peers); err != nil {
		return err
	}

	fmt.Printf("fetched %d peer(s)\n", len(peers))
	return nil
}

// Up connects to the network and stays in the foreground: it keeps the
// local peer set in sync with the server and reports endpoint changes
// it witnesses (endpoint gossip) until interrupted.
func (c *Client) Up(noFetch bool) error {
	cfg, err := c.LoadConfig()
	if err != nil {
		return err
	}

	apiClient := newApiClient(cfg.Server.InternalEndpoint)

	peers, err := c.store.ListPeers()
	if err != nil {
		return err
	}

	iface, err := c.buildMainInterface(cfg, peers)
	if err != nil {
		return err
	}
	if err := iface.Up(c.confPath()); err != nil {
		return fmt.Errorf("failed to bring up interface: %w", err)
	}
	defer func() { _ = iface.Down(true) }()
	fmt.Printf("interface up: %s (%s)\n", iface.DeviceName(), cfg.AssignedCidr)

	// initial fetch now that the network is reachable
	if !noFetch {
		if err := c.syncOnce(cfg, apiClient, iface); err != nil {
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
			if err := c.syncOnce(cfg, apiClient, iface); err != nil {
				fmt.Printf("warning: sync failed: %v\n", err)
			}
		}
	}
}

// Down tears down the network interface. On Linux this removes a
// kernel WireGuard device left by another process; with the userspace
// backend the interface lives and dies with its 'cord client up'
// process.
func (c *Client) Down() error {
	cfg, err := c.LoadConfig()
	if err != nil {
		return err
	}

	iface, err := c.buildMainInterface(cfg, nil)
	if err != nil {
		return err
	}
	return iface.Down(true)
}

// syncOnce performs one round of the state synchronization flow:
// report witnessed endpoint changes, refresh the peer list, and apply
// any changes to the live interface.
func (c *Client) syncOnce(
	cfg *ClientConfig,
	apiClient *apiClient,
	iface *wg.Interface,
) error {
	// gossip: compare live endpoints against the local peer cache
	if sightings := c.scanEndpoints(iface); len(sightings) > 0 {
		if err := apiClient.reportEndpoints(sightings); err != nil {
			fmt.Printf("warning: endpoint report failed: %v\n", err)
		}
	}

	// pull the server's view of the network
	peers, err := apiClient.peers()
	if err != nil {
		return err
	}
	if err := c.store.ReconcilePeers(peers); err != nil {
		return err
	}
	// rebuild and apply the interface peer list
	localPeers, err := c.store.ListPeers()
	if err != nil {
		return err
	}
	wgPeers, err := c.buildPeers(cfg, localPeers)
	if err != nil {
		return err
	}
	iface.SetPeers(wgPeers)
	return iface.Reconcile()
}

// scanEndpoints inspects the live device for peers whose endpoint
// changed since we last saw them, records the changes locally, and
// returns sightings to report to the server.
func (c *Client) scanEndpoints(
	iface *wg.Interface,
) []server.EndpointSighting {
	status, err := iface.Status()
	if err != nil {
		return nil
	}

	known, err := c.store.ListPeers()
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
		if err := c.store.UpdateEndpoint(key, endpoint, now.Unix()); err != nil {
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

func (c *Client) confPath() string {
	return path.Join(c.DataDir, c.Network+".conf")
}

// buildInviteInterface constructs the temporary interface described by
// an invite: the server is the only peer, reachable at its public
// invite endpoint.
func (c *Client) buildInviteInterface(
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

	_, inviteName, err := wg.NetworkInterfaceNames(c.Network)
	if err != nil {
		return nil, fmt.Errorf("invalid network interface names: %w", err)
	}

	iface, err := wg.NewInterface(
		inviteName,
		tempKey,
		net.IPNet{IP: ip, Mask: inviteNet.Mask},
		0,
		c.Backend,
	)
	if err != nil {
		return nil, err
	}
	iface.SetReconcileLogger(c.verbosef)

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
// client config and the local peer cache.
func (c *Client) buildMainInterface(
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

	mainName, _, err := wg.NetworkInterfaceNames(c.Network)
	if err != nil {
		return nil, fmt.Errorf("invalid network interface names: %w", err)
	}

	iface, err := wg.NewInterface(mainName, privKey, *address, 0, c.Backend)
	if err != nil {
		return nil, err
	}
	iface.SetReconcileLogger(c.verbosef)

	wgPeers, err := c.buildPeers(cfg, peers)
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
func (c *Client) buildPeers(
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
			EndpointPolicy:      wg.EndpointBootstrap,
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

// LoadInvite reads and validates a peer invite file. Callers use the
// invite's network name to construct the Client (and its store) before
// running Install.
func LoadInvite(
	invitePath string,
) (
	*server.PeerInvite,
	error,
) {
	file, err := os.Open(invitePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open invite: %w", err)
	}
	defer file.Close()

	invite, err := server.ReadPeerInvite(file)
	if err != nil {
		return nil, err
	}
	if invite.Interface.NetworkName == "" {
		return nil, fmt.Errorf("invite has no network name")
	}
	if err := server.ValidateNetworkName(invite.Interface.NetworkName); err != nil {
		return nil, fmt.Errorf("invite has invalid network name: %w", err)
	}
	return invite, nil
}

// probeInviteTunnel sends traffic toward the private invite API address
// to trigger WireGuard's lazy handshake, then verifies that the server
// peer answered before redemption starts.
func (c *Client) probeInviteTunnel(
	iface *wg.Interface,
	invite *server.PeerInvite,
) error {
	return c.probeServerTunnel(
		iface,
		invite.Server,
		"invite",
		"check UDP forwarding/firewall for the invite port, that the server invite interface is running, that this invite is active, and that the invite CIDR does not conflict with a local network",
	)
}

func (c *Client) probeServerTunnel(
	iface *wg.Interface,
	serverInfo server.ServerInfo,
	networkRole string,
	failureAdvice string,
) error {
	serverKey, err := wg.ParseKey(serverInfo.PublicKey)
	if err != nil {
		return fmt.Errorf("invalid server public key: %w", err)
	}

	internalAddr, err := net.ResolveUDPAddr("udp", serverInfo.InternalEndpoint)
	if err != nil {
		return fmt.Errorf(
			"invalid %s API endpoint '%s': %w",
			networkRole,
			serverInfo.InternalEndpoint,
			err,
		)
	}

	conn, err := net.DialUDP("udp", nil, internalAddr)
	if err != nil {
		return fmt.Errorf(
			"failed to route %s probe to %s: %w",
			networkRole,
			serverInfo.InternalEndpoint,
			err,
		)
	}
	c.verbosef("sending probe through %s tunnel to %s", networkRole, serverInfo.InternalEndpoint)
	_, writeErr := conn.Write([]byte{0})
	closeErr := conn.Close()
	if writeErr != nil {
		return fmt.Errorf(
			"failed to send %s probe to %s: %w",
			networkRole,
			serverInfo.InternalEndpoint,
			writeErr,
		)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close %s probe: %w", networkRole, closeErr)
	}

	lastLog := time.Time{}
	onStatus := func(status wg.PeerStatus) {
		if !c.Verbose || (!lastLog.IsZero() && time.Since(lastLog) < time.Second) {
			return
		}
		lastLog = time.Now()
		endpoint := serverInfo.ExternalEndpoint
		if status.Endpoint != nil {
			endpoint = status.Endpoint.String()
		}
		handshake := "not completed"
		if !status.LastHandshake.IsZero() {
			handshake = status.LastHandshake.Format(time.RFC3339)
		}
		c.verbosef("%s peer status: endpoint=%s, last handshake=%s", networkRole, endpoint, handshake)
	}

	if err := iface.WaitForHandshake(serverKey, inviteHandshakeTimeout, onStatus); err != nil {
		return fmt.Errorf(
			"%s WireGuard handshake with public endpoint %s failed: %w; %s",
			networkRole,
			serverInfo.ExternalEndpoint,
			err,
			failureAdvice,
		)
	}
	return nil
}

// ShowInstalled lists the networks installed under a config directory.
func ShowInstalled(
	configDir string,
) error {
	entries, err := os.ReadDir(configDir)
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
		EndpointPolicy:      wg.EndpointFixed,
		PersistentKeepalive: keepalive,
	}, nil
}
