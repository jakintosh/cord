package client

import (
	"database/sql"
	"fmt"
	"os"

	db "git.sr.ht/~jakintosh/innernet-go/internal/database"
)

type Context struct {
	Db        *sql.DB
	Name      string
	ConfigDir string
	DataDir   string
}

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

// Install consumes an invite and "creates" a new innernet on this
// machine. The invite tells this machine the basic info for the
// innernet definition, and also provides the peer info for the
// server-peer. Installing effectively creates a new network overview
// database with a single server peer in it. From there, this node
// can "fetch" the rest of the state via that server-peer node.
func (ctx *Context) Install() error {

	fmt.Printf(
		"Install\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Uninstall really just deletes a database file, and also puts
// "down" the existing wg device if it's "up", and deletes any config
// file that may exist on disk? (Maybe nothing, since the config is
// generated in code and directly puts up a wg interface.)
func (ctx *Context) Uninstall() error {

	fmt.Printf(
		"Uninstall\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Show gives a report of the given innernet, or of all innernets
// if no network is given. This means that maybe it can't take
// a default context? This is not always an operation on a single
// innernet. How to handle? Maybe a ShowAll from the command itself
func (ctx *Context) Show() error {

	fmt.Printf(
		"Show\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// After writing the last one, I decided that this needs to exist
// as its own distinct function. It will probably call Show() in
// a loop itself for all innernets.
func ShowAll(configDir string, dataDir string) error {

	fmt.Printf(
		"Show All\nData: %s\n",
		dataDir,
	)
	return nil
}

// Polls the server for peer state, and updates the local view of
// the network. This is really just fetching a list of peers which
// each have a list of recently seen endpoints, so that we can add
// (or remove) peers, and then try to contact them if we don't
// have a connection right now. Probably if we have any current
// state that the server does not have, we should send that back
// to the server.
func (ctx *Context) Fetch() error {

	fmt.Printf(
		"Fetch\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Generate a wireguard interface based on the local state stored
// for that innernet and then enable it. Should also try to fetch
// updated state from the server at the beginning, but should
// probably continue pretty quickly if the server is down.
func (ctx *Context) Up() error {

	fmt.Printf(
		"Up\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Take down a wireguard interface
func (ctx *Context) Down() error {

	fmt.Printf(
		"Down\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// what's missing?
//
// how is the client scheduling its own local work? it needs to
// be able to take a look at its wireguard config periodically
// to detect if peers have new endpoints, and then send that to
// the server if found. it might also want to be periodically
// fetching updates from the server. maybe these should both
// just be some kind of "sync" function? maybe not though,
// because sending an endpoint has a much smaller expectation
// for the server, but always asking for a full state dump from
// the server. I should figure out some kind of way to sync
// state between the clients and server without sending tons of
// json, especially if there might be *large* networks. i'm
// worried that now I'm thinking of adding more functionality,
// like marking certain machines as static and transient....
// maybe later.....
//
// anyway, maybe a Scan() function that checks the wg config
// for new endpoints, and then a Sync() function that sends
// that data to the server?
