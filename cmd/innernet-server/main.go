package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"strconv"

	"git.sr.ht/~jakintosh/command-go"
	"git.sr.ht/~jakintosh/innernet-go/internal/app"
)

const (
	BIN_NAME     = "innernet-server"
	AUTHOR       = "jakintosh"
	VERSION      = "0.1"
	DEFAULT_CFG  = "/etc/" + BIN_NAME
	DEFAULT_DATA = "/var/lib/" + BIN_NAME
)

var root = &command.Command{
	Name:    BIN_NAME,
	Author:  AUTHOR,
	Version: VERSION,
	Help:    "manage innernets",
	Subcommands: []*command.Command{
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
	},
	Operands: []command.Operand{},
	Options: []command.Option{
		{
			Short: 0,
			Long:  "config-dir",
			Type:  command.OptionTypeParameter,
			Help:  "directory for config files",
		},
		{
			Short: 0,
			Long:  "data-dir",
			Type:  command.OptionTypeParameter,
			Help:  "directory for program data",
		},
	},
}

var serve = &command.Command{
	Name:        "serve",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "serve an innernet coordination server",
	Subcommands: []*command.Command{},
	Operands: []command.Operand{
		{
			Name: "network",
			Help: "name of the innernet network the server coordinates",
		},
	},
	Options: []command.Option{
		{
			Short: 0,
			Long:  "no-routing",
			Type:  command.OptionTypeFlag,
			Help:  "tell Innernet not to handle routing",
		},
		{
			Short: 0,
			Long:  "mtu",
			Type:  command.OptionTypeParameter,
			Help:  "MTU for the WireGuard interface",
		},
		{
			Short: 0,
			Long:  "backend",
			Type:  command.OptionTypeParameter,
			Help:  "WireGuard backend to use ('kernel' or 'userspace')",
		},
	},
	Handler: func(i *command.Input) error {

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)
		noRouting := i.GetFlag("no-routing")
		mtu := i.GetIntParameterOr("mtu", 1280)
		backend := i.GetParameterOr("backend", "auto")

		// operands
		network := i.GetOperand("network")

		fmt.Printf("network: %s\n", network)
		fmt.Printf("configDir: %s\n", configDir)
		fmt.Printf("dataDir: %s\n", dataDir)
		fmt.Printf("noRouting: %t\n", noRouting)
		fmt.Printf("mtu: %d\n", mtu)
		fmt.Printf("backend: %s\n", backend)

		return nil
	},
}

var addNetwork = &command.Command{
	Name:        "add-network",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "create a new innernet",
	Subcommands: []*command.Command{},
	Operands: []command.Operand{
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
	Options: []command.Option{},
	Handler: func(i *command.Input) error {

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// operands
		network := i.GetOperand("network")
		if err := app.ValidateNetworkName(network); err != nil {
			return err
		}

		cidr, err := parseCidr(i, "cidr")
		if err != nil {
			return err
		}

		ip, err := parseIp(i, "external-ip")
		if err != nil {
			return err
		}

		port, err := parsePort(i, "external-port")
		if err != nil {
			return err
		}

		return app.CreateNetwork(
			configDir,
			dataDir,
			network,
			cidr,
			ip,
			port,
		)
	},
}

var deleteNetwork = &command.Command{
	Name:        "delete-network",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "delete an existing innernet",
	Subcommands: []*command.Command{},
	Operands: []command.Operand{
		{
			Name: "network",
			Help: "name of the network to delete",
		},
	},
	Options: []command.Option{},
	Handler: func(i *command.Input) error {

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// operands
		network := i.GetOperand("network")

		fmt.Printf("configDir: %s\n", configDir)
		fmt.Printf("dataDir: %s\n", dataDir)
		fmt.Printf("network: %s\n", network)

		return nil
	},
}

var addCidr = &command.Command{
	Name:        "add-cidr",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "add a child CIDR to a network",
	Subcommands: []*command.Command{},
	Operands: []command.Operand{
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
		{
			Name: "parent",
			Help: "name of the parent CIDR",
		},
	},
	Options: []command.Option{},
	Handler: func(i *command.Input) error {

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// operands
		network := i.GetOperand("network")
		name := i.GetOperand("name")
		cidr := i.GetOperand("cidr")
		parent := i.GetOperand("parent")

		fmt.Printf("configDir: %s\n", configDir)
		fmt.Printf("dataDir: %s\n", dataDir)
		fmt.Printf("network: %s\n", network)
		fmt.Printf("name: %s\n", name)
		fmt.Printf("cidr: %s\n", cidr)
		fmt.Printf("parent: %s\n", parent)

		return nil
	},
}

var renameCidr = &command.Command{
	Name:        "rename-cidr",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "rename an existing CIDR from a network",
	Subcommands: []*command.Command{},
	Operands: []command.Operand{
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
	Options: []command.Option{},
	Handler: func(i *command.Input) error {

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// operands
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")
		newName := i.GetOperand("new-name")

		fmt.Printf("configDir: %s\n", configDir)
		fmt.Printf("dataDir: %s\n", dataDir)
		fmt.Printf("network: %s\n", network)
		fmt.Printf("cidr: %s\n", cidr)
		fmt.Printf("new-name: %s\n", newName)

		return nil
	},
}

var deleteCidr = &command.Command{
	Name:        "delete-cidr",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "delete an existing CIDR from a network",
	Subcommands: []*command.Command{},
	Operands: []command.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "cidr",
			Help: "CIDR to delete",
		},
	},
	Options: []command.Option{},
	Handler: func(i *command.Input) error {

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// operands
		network := i.GetOperand("network")
		cidr := i.GetOperand("cidr")

		fmt.Printf("configDir: %s\n", configDir)
		fmt.Printf("dataDir: %s\n", dataDir)
		fmt.Printf("network: %s\n", network)
		fmt.Printf("cidr: %s\n", cidr)

		return nil
	},
}

var addPeer = &command.Command{
	Name:        "add-peer",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "create a new peer invite",
	Subcommands: []*command.Command{},
	Operands: []command.Operand{
		{
			Name: "network",
			Help: "network to add peer to",
		},
		{
			Name: "name",
			Help: "name of the peer",
		},
		{
			Name: "cidr",
			Help: "parent CIDR of the peer",
		},
	},
	Options: []command.Option{
		{
			Short: 'i',
			Long:  "ip",
			Type:  command.OptionTypeParameter,
			Help:  "IP of peer (within parent CIDR)",
		},
		{
			Short: 'a',
			Long:  "admin",
			Type:  command.OptionTypeFlag,
			Help:  "make new peer an admin?",
		},
		{
			Short: 0,
			Long:  "save-invite",
			Type:  command.OptionTypeParameter,
			Help:  "path to write the invite to",
		},
		{
			Short: 0,
			Long:  "invite-expires",
			Type:  command.OptionTypeParameter,
			Help:  "invite expiration period (eg. '30d', '7w', '2h', '1000s')",
		},
	},
	Handler: func(i *command.Input) error {

		// operands
		network := i.GetOperand("network")
		name := i.GetOperand("name")
		cidr := i.GetOperand("cidr")

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)
		ip := i.GetParameter("ip")
		admin := i.GetFlag("admin")
		savePath := i.GetParameterOr("save-invite", getPwd())
		inviteExpires := i.GetParameterOr("invite-expires", "7d")
		// TODO: write a parser for this time format

		fmt.Printf("configDir: %s\n", configDir)
		fmt.Printf("dataDir: %s\n", dataDir)
		fmt.Printf("network: %s\n", network)
		fmt.Printf("name: %s\n", name)
		fmt.Printf("cidr: %s\n", cidr)
		fmt.Printf("ip: %v\n", ip)
		fmt.Printf("admin: %t\n", admin)
		fmt.Printf("save-path: %s\n", savePath)
		fmt.Printf("invite-expires: %s\n", inviteExpires)

		return nil
	},
}

var renamePeer = &command.Command{
	Name:        "rename-peer",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "rename an existing peer",
	Subcommands: []*command.Command{},
	Operands: []command.Operand{
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
	Options: []command.Option{},
	Handler: func(i *command.Input) error {

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// operands
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")
		newName := i.GetOperand("new-name")

		fmt.Printf("configDir: %s\n", configDir)
		fmt.Printf("dataDir: %s\n", dataDir)
		fmt.Printf("network: %s\n", network)
		fmt.Printf("peer: %s\n", peer)
		fmt.Printf("new-name: %s\n", newName)

		return nil
	},
}

var enablePeer = &command.Command{
	Name:        "enable-peer",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "enable an existing peer",
	Subcommands: []*command.Command{},
	Operands: []command.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "peer",
			Help: "peer to rename",
		},
	},
	Options: []command.Option{},
	Handler: func(i *command.Input) error {

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// operands
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")

		fmt.Printf("configDir: %s\n", configDir)
		fmt.Printf("dataDir: %s\n", dataDir)
		fmt.Printf("network: %s\n", network)
		fmt.Printf("peer: %s\n", peer)

		return nil
	},
}

var disablePeer = &command.Command{
	Name:        "disable-peer",
	Author:      AUTHOR,
	Version:     VERSION,
	Help:        "disable an existing peer",
	Subcommands: []*command.Command{},
	Operands: []command.Operand{
		{
			Name: "network",
			Help: "network to be modified",
		},
		{
			Name: "peer",
			Help: "peer to rename",
		},
	},
	Options: []command.Option{},
	Handler: func(i *command.Input) error {

		// options
		configDir := i.GetParameterOr("config-dir", DEFAULT_CFG)
		dataDir := i.GetParameterOr("data-dir", DEFAULT_DATA)

		// operands
		network := i.GetOperand("network")
		peer := i.GetOperand("peer")

		fmt.Printf("configDir: %s\n", configDir)
		fmt.Printf("dataDir: %s\n", dataDir)
		fmt.Printf("network: %s\n", network)
		fmt.Printf("peer: %s\n", peer)

		return nil
	},
}

func main() {
	root.Parse()
}

// Subcommands
//  => serve				<iname>								no-routing?, mtu?, backend?
//  => add-network			<iname>, <cidr>, <external-ip>, <external-port>
//  => delete-network		<iname>
//  => add-cidr				<iname>, <name>, <cidr>				parent?
//  => rename-cidr			<iname>, <name>, <new-name>
//  => delete-cidr			<iname>, <name>
//  => add-peer				<iname>, <name>, <cidr>, <admin>	ip?, configPath?
//  => rename-peer			<iname>, <name>, <new-name>
//  => enable-peer			<iname>, <name>
//  => disable-peer			<iname>, <name>

// // init program directories
// os.MkdirAll(*configPath, 0755)
// os.MkdirAll(*dataPath, 0755)

// // init modules
// initDatabase(*configPath)

// config routing

// serve
// addr := fmt.Sprintf(":%d", port)
// log.Fatal(http.ListenAndServe(addr, nil))

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
// peer. from this, innernet can generate and stand up a wg interface.
//
// next, the admin creates an invite, which places a hold on an IP address
// and is ready to handle a /redeem request. when a client goes to redeem
// an invite, they'll use the invite to create a temporary network and add
// the server as their only peer, then contact the server over its internal
// address. the server validates the redemption, an registers the peer.
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

func parseCidr(input *command.Input, field string) (*net.IPNet, error) {
	value := input.GetOperand(field)
	_, cidr, err := net.ParseCIDR(value)
	if err != nil {
		err = fmt.Errorf("failed to parse cidr from operand '%s': %v", field, err)
	}
	return cidr, err
}

func parseIp(input *command.Input, field string) (net.IP, error) {
	value := input.GetOperand(field)
	ip := net.ParseIP(value)
	if ip == nil {
		return nil, fmt.Errorf("failed to parse ip from '%s'", value)
	}
	return ip, nil
}

func parsePort(input *command.Input, field string) (uint16, error) {
	value := input.GetOperand(field)
	port, err := strconv.ParseUint(value, 10, 16)
	if err != nil {
		return 0, fmt.Errorf("failed to parse port from '%s': %v", value, err)
	}
	return uint16(port), nil
}

func readEnvVar(name string) string {
	var present bool
	str, present := os.LookupEnv(name)
	if !present {
		log.Fatalf("missing required env var '%s'\n", name)
	}
	return str
}

func getPwd() string {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Printf("couldn't read pwd")
		os.Exit(1)
	}
	return dir
}
