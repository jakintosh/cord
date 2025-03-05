package main

func main() {
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
}
