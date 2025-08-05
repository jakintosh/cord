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

// Install consumes an invite and "creates" a new cord on this
// machine. The invite tells this machine the basic info for the
// cord definition, and also provides the peer info for the
// server-peer. Installing effectively creates a new network overview
// database with a single server peer in it. From there, this node
// can "fetch" the rest of the state via that server-peer node.
func (ctx *Context) Install(
	invitePath string,
) error {

	// create a database

	// insert the basic wg interface information

	// insert the server's peer information

	// generate a new key pair

	// redeem invite with server using new public key

	// update interface with new public key

	// 1. create a temporary interface with info in the invite
	// 2. generate new key pair and redeem it on temp iface
	// 3. then create permanent interface and fetch state

	fmt.Printf(
		"Install\nInvite: %s\nNetwork: %s\nConfig: %s\nData: %s\n",
		invitePath, ctx.Name, ctx.ConfigDir, ctx.DataDir,
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

// Show gives a report of the given cord, or of all cords
// if no network is given. This means that maybe it can't take
// a default context? This is not always an operation on a single
// cord. How to handle? Maybe a ShowAll from the command itself
func (ctx *Context) Show() error {

	fmt.Printf(
		"Show\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// After writing the last one, I decided that this needs to exist
// as its own distinct function. It will probably call Show() in
// a loop itself for all cords.
func ShowAll(configDir string, dataDir string) error {

	fmt.Printf(
		"Show All\nData: %s\n",
		dataDir,
	)
	return nil
}

// Polls the server for peer state, and updates the local view
// of the network. This is really just fetching a list of peers
// which each have a list of recently seen endpoints, so that
// we can add (or remove) peers, and then try to contact them
// if we don't have a connection right now. Probably if we have
// any current state that the server does not have, we should
// send that back to the server.
//
// Bigger question, how can we keep a very large peer list in
// sync? What's the smallest way I can send state? The current
// state of a peer is its id and endpoint. If we compress, this
// can be a u64 and a u256, for a total of 40B. Maybe I can
// keep a canonical set of changes, which is like a "log" of
// all the events that have occurred, and then peers can fetch
// all events past their most recent event and then play them
// back to get to the latest state?
//
// This means you could fetch a full state, which would be the
// current state of the network plus the current revision, and
// then to update you'd say "send me changes since this rev"
// and then apply them and update your local revision. This
// seems kind of cool. Only the server holds the revision list,
// and the client just applies them. Maybe there's also a
// version id and if there's a new version that's when you
// could pull an entirely fresh state.
//
// This also means that the server can collapse the event log
// to a simpler state if relevant, like multiple reports of
// the same new endpoint.
//
// What are the other state events that exist? From the
// perspective of a peer, basically nothing? We only care
// about peers having seen endpoints, or being deleted. From a
// security perspective, we actually don't really want each
// peer to fully construct the event log, because a malicious
// peer could get way more info than it needs.
//
// *Really*, the best case scenario would be one where a peer
// is able to say here's my reference point, tell me the
// minimal changes to update my state". A legal peer will
// follow state change instructions, and a malicious peer will
// get the bare minimum information to misuse. The question
// becomes: how can the server efficiently process the delta
// between two network states for any given peer? The key info
// is that both CIDRs and Associations can change, so the
// valid peers at one snapshot can change significantly over
// time. What if CIDRs and Associations (oh, and peer enable?)
// had an "effective" index, which was an int of the state
// where it went into effect, and then we grab "highest index
// for given resource under limit"?
//
// Okay what would this look like: I'm a peer with a network
// state index of 100. I query the server for my new state by
// sending along that index. The server looks in the database
// using that index to find the peer and parent cidrs for the
// peer. What's important here is that we can't just "delete"
// things from the database now, we'd need to have a physical
// entry in the CIDR table that says whether that CIDR is still
// real, because otherwise we'll pick up old state from when it
// was valid. So the state database would now be growing
// forever, when cidr/assoc/peer are added or changed. Sure.
// So, we use this index to filter the last valid states at
// that point in time, and can construct a state table from
// that. Then we can do the same for the very latest state, and
// calculate a delta between the two tables.
//
// What can I do to make this easier? It would be great to be
// able to do all of the work inside SQL as easily as possible,
// so if there's anything I can do to make "deltas" easier to
// calculate that would be optimal. Again, all I *really* care
// about is which peers are valid for me. I just need to know
// if I should forget a peer I used to know, or learn about a
// new peer entirely. Given the index, I should get back a list
// of "added, deleted" peers. So when doing all the SQL, I
// don't actually care about figuring out deltas of anything
// but peers. The delta would be a FULL JOIN on the pre/post
// state table, ONLY B means added, if A + B exist we grab
// anything where state has changed which would be 'disabled'
// and 'name'. In that case, we just send over the B value.
//
// Okay so where did we end up? A new model for making network
// changes using a 'state_index' value that allows me to grab
// the last state for a resource at a certain point in time.
// Then I use that index to resolve a peer list at the index
// and now, and figure out the delta of those peer lists. The
// possible things that can happen is a totally new peer is
// now existing, an existing peer had a state change, or an
// existing peer is deleted.
func (ctx *Context) Fetch() error {

	fmt.Printf(
		"Fetch\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Generate a wireguard interface based on the local state stored
// for that cord and then enable it. Should also try to fetch
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

// Begin a long running task that scans the wireguard interface
// at some interval
func (ctx *Context) Watch() error {

	fmt.Printf(
		"Down\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Scan the wireguard interface to detect any changes to peer
// endpoints. Needs to keep track of the local state to compare
// against.
func (ctx *Context) Scan() error {

	fmt.Printf(
		"Down\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// Send any locally known peer endpoint changes to the server.
// Send a timestamp, and then get changes seen since the
// timestamp? Some kind of way to not repeatedly get back
// "renewed" sightings of stable endpoints.
func (ctx *Context) Sync() error {

	fmt.Printf(
		"Down\nNetwork: %s\nConfig: %s\nData: %s\n",
		ctx.Name, ctx.ConfigDir, ctx.DataDir,
	)
	return nil
}

// When we install a new cordwork, we need a database to
// keep track of our locally known peer state, which is a log
// of the peers we know about and the endpoints we've seen
func initNetworkDb(d *sql.DB) error {

	if err := db.EnableForeignKeys(d); err != nil {
		return err
	}

	if err := db.InitTable(d, "peer", `
		CREATE TABLE IF NOT EXISTS peer (
			id					INTEGER PRIMARY KEY,
			cidr				INTEGER NOT NULL,
			public_key			TEXT NOT NULL UNIQUE,
			admin				INTEGER DEFAULT 0 NOT NULL,
			disabled			INTEGER DEFAULT 0 NOT NULL,
			redeemed			INTEGER DEFAULT 0 NOT NULL,
			invite_expires 		INTEGER,
			FOREIGN KEY (cidr)
				REFERENCES cidr (id)
		);
	`); err != nil {
		return err
	}

	if err := db.InitTable(d, "endpoint", `
		CREATE TABLE IF NOT EXISTS endpoint (
			id					INTEGER PRIMARY KEY,
			peer_ip				BLOB NOT NULL,
			peer_key			TEXT NOT NULL,
			endpoint			TEXT NOT NULL,
			time				INTEGER NOT NULL
		);
	`); err != nil {
		return err
	}

	return nil
}
