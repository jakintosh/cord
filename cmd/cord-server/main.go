package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path"
	"strconv"
	"time"

	cmd "git.sr.ht/~jakintosh/command-go"
	"git.sr.ht/~jakintosh/cord/internal/database"
	"git.sr.ht/~jakintosh/cord/internal/server"
	"git.sr.ht/~jakintosh/cord/internal/wireguard"
)

const (
	BIN_NAME     = "cord-server"
	AUTHOR       = "jakintosh"
	VERSION      = "0.1"
	DEFAULT_CFG  = "/etc/" + BIN_NAME
	DEFAULT_DATA = "/var/lib/" + BIN_NAME
)

func main() {
	root.Parse()
}

var root = &cmd.Command{
	Name:    BIN_NAME,
	Author:  AUTHOR,
	Version: VERSION,
	Help:    "manage cords",
	Subcommands: []*cmd.Command{
		serve,
		addNetwork,
		deleteNetwork,
		addCidr,
		renameCidr,
		deleteCidr,
		addPeer,
		renamePeer,
		enablePeer,
		disablePeer,
		getPeers,
		addAssociation,
		deleteAssociation,
	},
	Operands: []cmd.Operand{},
	Options: []cmd.Option{
		{
			Short: 0,
			Long:  "config-dir",
			Type:  cmd.OptionTypeParameter,
			Help:  "directory for config files",
		},
		{
			Short: 0,
			Long:  "data-dir",
			Type:  cmd.OptionTypeParameter,
			Help:  "directory for program data",
		},
	},
}

var getPeers = &cmd.Command{
	Name:        "get-peers",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "get peer list for a given peer",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "name of the cord network the server coordinates",
		},
		{
			Name: "peer",
			Help: "the name of the requesting peer",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		//operands
		network := i.GetOperand("network")
		peerName := i.GetOperand("peer")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		peers, err := ctx.GetPeersOfPeerNamed(peerName)
		if err != nil {
			return fmt.Errorf("failed to get peers for '%s': %w", peerName, err)
		}

		jsonBytes, err := json.Marshal(peers)
		if err != nil {
			return fmt.Errorf("failed to marshal []server.Peer to json: %w", err)
		}

		fmt.Println(string(jsonBytes))

		return nil
	},
}

var serve = &cmd.Command{
	Name:        "serve",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "serve a cord coordination server",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "name of the cord network the server coordinates",
		},
	},
	Options: []cmd.Option{
		{
			Short: 0,
			Long:  "no-routing",
			Type:  cmd.OptionTypeFlag,
			Help:  "tell cord not to handle routing",
		},
		{
			Short: 0,
			Long:  "mtu",
			Type:  cmd.OptionTypeParameter,
			Help:  "MTU for the WireGuard interface",
		},
		{
			Short: 0,
			Long:  "backend",
			Type:  cmd.OptionTypeParameter,
			Help:  "WireGuard backend to use ('kernel' or 'userspace')",
		},
	},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)
		noRouting := i.GetFlag("no-routing")
		mtu := i.GetIntParameterOr("mtu", 1280)
		backendValue := i.GetParameterOr("backend", "kernel")

		// parse
		backend, err := parseBackend(backendValue)
		if err != nil {
			return fmt.Errorf("failed to parse backend: %w", err)
		}

		// create app context
		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		err = ctx.Serve(noRouting, mtu, backend)
		if err != nil {
			return fmt.Errorf("failed to serve network '%s': %w", network, err)
		}

		return nil
	},
}

var addNetwork = &cmd.Command{
	Name:        "add-network",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "create a new cord",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "name for the new network",
		},
		{
			Name: "cidr",
			Help: "root CIDR for the new network",
		},
		{
			Name: "external-ip",
			Help: "external IP the coordination server can be reached at",
		},
		{
			Name: "external-port",
			Help: "external port the coordination server listens on",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")
		cidrValue := i.GetOperand("cidr")
		ipValue := i.GetOperand("external-ip")
		portValue := i.GetOperand("external-port")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// parse
		cidr, err := parseCidr(cidrValue)
		if err != nil {
			return fmt.Errorf("failed to parse cidr: %w", err)
		}

		ip, err := parseIp(ipValue)
		if err != nil {
			return fmt.Errorf("failed to parse ip: %w", err)
		}

		port, err := parsePort(portValue)
		if err != nil {
			return fmt.Errorf("failed to parse port: %w", err)
		}

		// create app context
		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		err = ctx.CreateNetwork(cidr, ip, port)
		if err != nil {
			return fmt.Errorf("failed to create network: %w", err)
		}

		return nil
	},
}

var deleteNetwork = &cmd.Command{
	Name:        "delete-network",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "delete an existing cord",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "name of the network to delete",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		err = ctx.DeleteNetwork()
		if err != nil {
			return fmt.Errorf("failed to delete network: %w", err)
		}

		return nil
	},
}

var addCidr = &cmd.Command{
	Name:        "add-cidr",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "add a child CIDR to a network",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "network to add a CIDR to",
		},
		{
			Name: "name",
			Help: "name of the CIDR",
		},
		{
			Name: "cidr",
			Help: "address range in CIDR notation (i.e. 10.0.0.0/8)",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")
		name := i.GetOperand("name")
		cidr := i.GetOperand("cidr")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		// create request
		req := server.CreateCidrRequest{
			Name: name,
			Cidr: cidr,
		}

		// execute command
		err = ctx.CreateCidr(req)
		if err != nil {
			return fmt.Errorf("failed to create cidr: %w", err)
		}

		return nil
	},
}

var renameCidr = &cmd.Command{
	Name:        "rename-cidr",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "rename an existing CIDR from a network",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "cidr",
			Help: "CIDR to rename",
		},
		{
			Name: "new-name",
			Help: "new name for CIDR",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")
		newName := i.GetOperand("new-name")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		// create request
		req := server.UpdateCidrRequest{
			Name: newName,
		}

		// execute command
		err = ctx.UpdateCidr(cidr, req)
		if err != nil {
			return fmt.Errorf("failed to rename cidr: %w", err)
		}

		return nil
	},
}

var deleteCidr = &cmd.Command{
	Name:        "delete-cidr",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "delete an existing CIDR from a network",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "cidr",
			Help: "CIDR to delete",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		err = ctx.DeleteCidr(cidr)
		if err != nil {
			return fmt.Errorf("failed to delete cidr: %w", err)
		}

		return nil
	},
}

var addPeer = &cmd.Command{
	Name:        "add-peer",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "create a new peer invite",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "network to add peer to",
		},
		{
			Name: "name",
			Help: "name of the peer",
		},
		{
			Name: "ip",
			Help: "IP of peer (immutable once created)",
		},
	},
	Options: []cmd.Option{
		{
			Short: 'a',
			Long:  "admin",
			Type:  cmd.OptionTypeFlag,
			Help:  "make new peer an admin?",
		},
		{
			Short: 0,
			Long:  "save-invite",
			Type:  cmd.OptionTypeParameter,
			Help:  "path to write the invite to",
		},
		{
			Short: 0,
			Long:  "invite-expires",
			Type:  cmd.OptionTypeParameter,
			Help:  "invite expiration period (eg. '30d', '7w', '2h', '1000s')",
		},
	},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")
		name := i.GetOperand("name")
		ipValue := i.GetOperand("ip")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)
		admin := i.GetFlag("admin")
		savePath := i.GetParameterOr("save-invite", getPwd())
		inviteValue := i.GetParameterOr("invite-expires", "7d")

		// parse
		ip, err := parseIp(ipValue)
		if err != nil {
			return fmt.Errorf("failed to parse ip: %w", err)
		}

		expiration, err := parseExpiration(inviteValue)
		if err != nil {
			return fmt.Errorf("failed to parse expiration: %w", err)
		}

		// command logic

		// make sure we have file handle before db logic
		fileName := name + ".toml"
		savePath = path.Join(savePath, fileName)
		inviteFile, err := os.Create(savePath)
		if err != nil {
			return fmt.Errorf("failed to open file '%s': %w", savePath, err)
		}

		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		req := server.CreateInviteRequest{
			Name:       name,
			IP:         ip,
			Admin:      admin,
			Expiration: expiration,
		}
		peerInterface, _, err := ctx.CreateInvite(req)
		if err != nil {
			return fmt.Errorf("failed to create peer: %w", err)
		}

		invite := wireguard.Invite{
			PeerInterface: peerInterface,
		}
		err = invite.Write(inviteFile)
		if err != nil {
			return fmt.Errorf("failed to write invite: %w", err)
		}

		return nil
	},
}

var renamePeer = &cmd.Command{
	Name:        "rename-peer",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "rename an existing peer",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "peer",
			Help: "peer to rename",
		},
		{
			Name: "new-name",
			Help: "new name for peer",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")
		oldName := i.GetOperand("peer")
		newName := i.GetOperand("new-name")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		req := server.UpdatePeerRequest{
			Name: &newName,
		}
		_, err = ctx.UpdatePeer(oldName, req)
		if err != nil {
			return fmt.Errorf("failed to rename peer: %w", err)
		}

		return nil
	},
}

var enablePeer = &cmd.Command{
	Name:        "enable-peer",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "enable an existing peer",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "peer",
			Help: "peer to enable",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")
		peerName := i.GetOperand("peer")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		req := server.UpdatePeerRequest{
			Enabled: boolPtr(true),
		}
		_, err = ctx.UpdatePeer(peerName, req)
		if err != nil {
			return fmt.Errorf("failed to enable peer: %w", err)
		}

		return nil
	},
}

var disablePeer = &cmd.Command{
	Name:        "disable-peer",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "disable an existing peer",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "peer",
			Help: "peer to rename",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")
		peerName := i.GetOperand("peer")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		req := server.UpdatePeerRequest{
			Enabled: boolPtr(false),
		}
		_, err = ctx.UpdatePeer(peerName, req)
		if err != nil {
			return fmt.Errorf("failed to enable peer: %w", err)
		}

		return nil
	},
}

var addAssociation = &cmd.Command{
	Name:        "add-association",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "create an association between two CIDRs",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "network to add an association to",
		},
		{
			Name: "cidr1",
			Help: "name of the first CIDR",
		},
		{
			Name: "cidr2",
			Help: "name of the second CIDR",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")
		cidr1 := i.GetOperand("cidr1")
		cidr2 := i.GetOperand("cidr2")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		err = ctx.CreateAssociation(cidr1, cidr2)
		if err != nil {
			return fmt.Errorf("failed to create association: %w", err)
		}

		return nil
	},
}

var deleteAssociation = &cmd.Command{
	Name:        "delete-association",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "delete an association between two CIDRs",
	Subcommands: []*cmd.Command{},
	Operands: []cmd.Operand{
		{
			Name: "network",
			Help: "network to delete an association from",
		},
		{
			Name: "cidr1",
			Help: "name of the first CIDR",
		},
		{
			Name: "cidr2",
			Help: "name of the second CIDR",
		},
	},
	Options: []cmd.Option{},
	Handler: func(i *cmd.Input) error {

		// operands
		network := i.GetOperand("network")
		cidr1 := i.GetOperand("cidr1")
		cidr2 := i.GetOperand("cidr2")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// create app context
		ctx, err := initContext(network, configDir, dataDir)
		if err != nil {
			return fmt.Errorf("failed to create context: %w", err)
		}

		err = ctx.DeleteAssociation(cidr1, cidr2)
		if err != nil {
			return fmt.Errorf("failed to delete association: %w", err)
		}

		return nil
	},
}

// // init program directories
// os.MkdirAll(*configPath, 0755)
// os.MkdirAll(*dataPath, 0755)
//
// what does (server) program do
//
// * create/delete cidrs
// * create/delete associations
// * create/delete peers
//   * this uses an invite system
// * tell peers about network changes
//
// so, in short, there's a small CRUD component that tracks the state
// of the network, and an api endpoint for clients to send and receive
// information about their view of the network. there's also a need
// for servers to accept admin commands, and a way to verify who is
// sending those commands.
//
// network management
// api endpoints
//
// is that it?
//
// server is created. admin sets up a network by creating a new wg interface
// with a name and cidr mask. implicitly, the server is added as a peer to
// this network. this creates the abstract idea of a network in the server.
// in order for this network to actually exist, this data model of a server
// needs to be translated into a wireguard configuration. the server needs
// to be able to return all relevant network/peer information for a given
// peer. from this, cord can generate and stand up a wg interface.
//
// next, the admin creates an invite, which places a hold on an IP address
// and is ready to handle a /redeem request. when a client goes to redeem
// an invite, they'll use the invite to create a temporary network and add
// the server as their only peer, then contact the server over its internal
// address. the server validates the redemption, and registers the peer.
// to register the peer, the server already has the IP and other metadata,
// but needs the peer to provide it's self generated public key. once it
// finishes, the newly added peer can use the /state endpoint to get a new
// network snapshot and build its own wg interface.
//
// The /state endpoint checks the asker and then queries a list of peers
// for that peer (using CIDRs and associations) and gives them back. (can
// we store IPs as 32bit ints and filter CIDRs with comparison operators
// in SQL?)
//
// additionally, the server can field /admin/ requests, verifying that the
// peer asking has admin credentials. these are the standard CRUD operations
// for CIDR, Peers, and Associations.
//
// finally, most importantly: how will my endpoint gossiping work? the goal
// is for any peer on the network to notice that one of its peer endpoints
// changed, and then to communicate that back to the server. perhaps a
// client can read its wg state, check it against its last known state, if
// it sees a new endpoint, it can look at the last handshake timestamp and
// then share back that it saw that endpoint at that time. perhaps clients
// could do some other periodic checking/reporting of endpoints as well.
// on the server side, the server would maintain some kind of rolling
// endpoint history for each peer, allowing it to see if peers are holding
// on to multiple simultaneous IPs, or have definitively switched. when
// peers request state, they'll get all recently seen endpoints, sorted by
// most recent. perhaps after some time (24h) endpoint sightings expire.
// could we also push these endpoint sightings to peers on the network?
// perhaps even in a p2p fashion, so that server downtime doesn't disrupt
// the ability for peers to communicate? technically, each client could
// also be listening on their internal network for some gossip info, and
// new peer sightings could move through the network. this could maybe be
// added later, and not be part of the initial design. so: A is connected
// to B via endpoint 1. suddenly, A connects to B via endpoint 2, and wg
// changes the endpoint. B does a periodic check and sees that A has
// changed, and sends an update to the server with A's ID, new endpoint,
// and timestamp. the server adds this sighting to its database, and the
// next time peers call /state, this new endpoint is part of A's endpoint
// candidate list for them to use.
//
// Server
//
// API
//   => /user/state
//   => /user/redeem
//   => /user/{endpoint sighting}
//   => /admin/associations
//   => /admin/cidrs
//   => /admin/peers
//
// The server maintains a sqlite database of "network rules", but at the
// end of the day these just get turned into a list of wireguard peers
// that each client can add to their wg interface. That means that the
// server should really not be concerned with wireguard interfaces at
// all. The only networking role it has is to serve the HTTP API on an
// internal address, and to initialize the server's "peer" address and
// listening port.
//
// On network init, the server initializes its "peer" info, which is
// its external endpoint and internal endpoint. OG cord treats the
// server as almost a "static peer", in that it doesn't follow the
// convention of a normal peer, and is basically "shipped" with the
// network. Do I want to keep this? Actually, yes, until I reconsider a
// p2p version. So server needs to share public key, allowed ip, and
// endpoint. There's the wireguard endpoint, which is public ip and
// public UDP listening port. This is for other peers to join wg network.
// The internal endpoint is for peers to talk to the API once on the
// cord. The server also needs information for creating the wg
// interface, which is net name, peer CIDR, and private key. So in total,
// the server gets created, chooses a name, a stable external endpoint,
// a stable internal endpoint, and generates a public and private key.
//
// Client
//
// The client, on the other hand, has a different job. It just polls the
// server for peer information, updates its list of valid peers, and
// updates its wireguard interface to match. It also phones home to the
// server to report the external endpoint it sees for peers. Perhaps for
// peers that are not connected, the client will poll somewhat more
// frequently just for that peer's endpoint.
//
// So I think the client should maintain its own sqlite db for peer info,
// and then be able to generate a full wg interface from that db. It
// should have a .conf file that describes the client peer and server
// peer, and then generates the rest of the network peers via the sqlite
// db. /etc/cord/{interface}.conf and /var/lib/cord/{interface}.db
// are what the client binary manages.
//
// There's also the server as a unique case for the "client" to manage.
// Should the server binary handle wg manipulation itself, or expect the
// user to just run the normal client binary for network manipulation?
// It would be great, logically, to keep those separate. But why might
// that be a bad idea? Are there special things that the server peer
// shouldn't be able to do that the client binary would let other peers
// do? Things like setting a specific endpoint? I don't really see any
// issues off hand. So, the server binary should also be responsible for
// creating the /etc/cord/{interface}.conf file in lieu of the client
// managing the "install" of that peer? Perhaps it can call into the
// client.Install() function, we'll see. Maybe the client binary can have
// come kind of server-aware set up?
//
//
// Wireguard
//
// A wireguard interface consists of a "device" definition, which is the
// local machine's private key, listening port, and "internal" ip/netmask.
//
// A wireguard peer consists of the peer's public key, external endpoint,
// and "allowed ips" (cidrs).

func initContext(
	network string,
	configDir string,
	dataDir string,
) (*server.Context, error) {
	config := server.NewFsConfig(configDir)
	store, err := database.Init(network, dataDir, true)
	if err != nil {
		return nil, err
	}
	return server.NewContext(network, config, store)
}

func parseCidr(
	value string,
) (
	*net.IPNet,
	error,
) {
	_, cidr, err := net.ParseCIDR(value)
	if err != nil {
		err = fmt.Errorf("failed to parse cidr from operand '%s': %v", value, err)
	}
	return cidr, err
}

func parseIp(
	value string,
) (
	net.IP,
	error,
) {
	ip := net.ParseIP(value)
	if ip == nil {
		return nil, fmt.Errorf("failed to parse ip from '%s'", value)
	}

	if ip4 := ip.To4(); ip4 != nil {
		return ip4, nil
	} else {
		return ip, nil
	}
}

func parsePort(
	value string,
) (
	uint16,
	error,
) {
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("failed to parse port from '%s': %v", value, err)
	}
	return uint16(port), nil
}

func parseBackend(
	value string,
) (
	server.BackendType,
	error,
) {
	switch value {
	case "kernel":
		return server.KernelBackend, nil
	case "userspace":
		return server.UserspaceBackend, nil
	default:
		return server.UndefinedBackend, fmt.Errorf("unexpected backend value: %s", value)
	}
}

func parseExpiration(
	value string,
) (
	time.Time,
	error,
) {
	last := len(value) - 1
	number, err := strconv.ParseInt(value[0:last], 10, 64)
	if err != nil {
		return time.Unix(0, 0), err
	}

	var multiplier int64
	suffix := value[last]
	switch suffix {
	case 's':
		multiplier = 1
	case 'm':
		multiplier = 60
	case 'h':
		multiplier = 60 * 60
	case 'd':
		multiplier = 60 * 60 * 24
	case 'w':
		multiplier = 60 * 60 * 24 * 7
	}

	return time.Now().Add(time.Duration(number * multiplier)), nil
}

func getPwd() string {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Printf("couldn't read pwd")
		os.Exit(1)
	}
	return dir
}

func boolPtr(b bool) *bool { return &b }
