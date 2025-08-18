# Question Log

## Questions about the Network Initialization Flow:

1. **External IP and Port Usage**: The `CreateNetwork` function accepts `address` and `port` parameters (external IP and port), but I don't see where these are actually stored or used in the current implementation. Are these supposed to be persisted in the database? Are they for configuring the server's WireGuard endpoint?

Answer: Inside the CreateNetwork function, the address and port parameters are ultimately going to be used to define the public address:port that the server is listening on. This is how external nodes will initially find the server.

2. **Server Config File**: There's a TODO comment saying "also write out the server config file here" - what exactly should this config file contain? Is this supposed to be a WireGuard configuration file for the server itself to use?

Answer: The "server config" file is the WireGuard config file that the server itself uses for WireGuard. At first it will only have it's own interface definition, because there will be no other peers.

3. **WriteInvite Call**: At the end of `CreateNetwork`, there's a call to `peerCfg.WriteInvite(cfgFile, deviceCfg)`, but the `WriteInvite` function is just a stub. Given that this is for the server peer, what should actually be written to this file? It doesn't seem like it should be an "invite" since the server is creating itself.

Answer: I think you're right, and my comment ("what is this doing here") is also right. This might have been a temporary version of the "server config" we just talked about, and I think that the WireGuard config is all we need to get the server set up and ready to go.

4. **Server Peer Redemption**: The server peer is created and then immediately redeemed with the same public key. Is this the intended behavior? What's the purpose of the redemption step for the server itself?

Answer: I think that it was just to simplify the logic, since the server's peer creation is the only "special" one—every other peer will go through redemption, and this was the fastest way to to not build a custom path for creating the server.

5. **Database Initialization Timing**: Should the database schema initialization happen before or after validation steps? Currently it happens after name validation but before CIDR creation.

Answer: We init the database second only to getting a file handle for writing the final config. If we can't create a file handle for the network config, we don't want to init a database for it. But otherwise, the database needs to be initialized first before we call any of the other context functions, because they rely on the db being initialized.

6. **Error Handling**: If any step fails partway through (e.g., after database init but before peer creation), should there be cleanup/rollback logic?

Answer: There actually probably should be rollback logic, so that the network isn't left in a partially constructed state. This would also simplify the init order from #5, meaning that the db could just be initialized first, which would be the most obvious way to do it.


## Questions about the CIDR Management Flow:

1. **Admin Verification**: How does the server verify that the requester has admin privileges for CIDR operations? I see the `admin` field on peers, but don't see authentication logic in the CIDR functions.

Answer: For admin verification, we are relying on the admin field on peers as our authentication. Because the WireGuard network can only be accessed by someone who joined the network, and because the server is the source of truth, and because wireguard peers are cryptographically identified, there is no plausible way for a peer to spoof another peer, or to fake credentials—the server decides which peers are admins, and the server is the one they are admin-ing.

2. **Request Source**: Should CIDR management happen via:
   - Direct CLI commands on the server (current implementation)
   - HTTP API calls from remote clients (suggested by client placeholders)
   - Both methods?

Answer: CIDR management will ultimately spawn from either the CLI or the HTTP API.

3. **CIDR Overlap Prevention**: Beyond checking if a CIDR fits within the root CIDR, should the system prevent overlapping CIDRs at the same hierarchical level?

Answer: Technically, CIDRs at the same hierarchical level cannot overlap, because they "collapse" into their own separated buckets. While a CIDR covers a "range" of IPs, each CIDR is a discrete unit.

4. **Conflict Detection**: Should CIDR creation/modification check for conflicts with existing peer IP assignments?

Answer: I think no, CIDR creation shouldn't check for conflicts with peer assignments, and we should leave it up to the admin to know what they're doing.

5. **CIDR Deletion Impact**: When deleting a CIDR, what should happen to:
   - Peers currently assigned to that CIDR?
   - Associations involving that CIDR?
   - Any sub-CIDRs nested within it?

Answer: Peers and sub-CIDRs should not be deleted, but associations should be deleted.

6. **Peer Notification**: When CIDRs change, how do existing peers learn about the network topology changes? Does this trigger the state synchronization system?

Answer: Peer updates are handled separately from server topology changes, and we don't worry about that here.

7. **Transaction Management**: Should CIDR operations be wrapped in database transactions to handle partial failures (e.g., CIDR deleted but associations not cleaned up)?

Answer: Yes, any CIDR operations that make multiple database changes at once should be wrapped in a transaction.

8. **Concurrent Operations**: How should concurrent CIDR modifications be handled to prevent race conditions?

Answer: Concurrent CIDR modifications should not be allowed. The easiest way to do this would be to make sure any change to the CIDR table starts a transaction that immediately claims a write lock, and to rely on SQLite's serial execution, and to limit its connection pool to 1.


## Questions about the Peer Invitation Flow:

1. **Invite File Format & Contents**: The `WriteInvite` method is stubbed as a TODO. What exactly should be written to the invite file? I can see it receives both a `DeviceConfig` and `PeerConfig` - should the invite contain:
   - The temporary keypair generated during peer creation?
   - Server connection information (IP, port, endpoint)?
   - Network metadata (CIDR, network name)?
   - Some kind of redemption token?

Answer: The WriteInvite method writes out an Invite struct, which is composed of a DeviceConfig and a PeerConfig. Basically, the device config has enough information to set up the wireguard interface, and the peer config has enough information to bootstrap the "cord" peer. These sections will be both stored in a toml file that can be sent to a peer, who can then bootstrap themselves into the network with both a wg interface and the necessary cord information. To be more specific, the invite will include the private wireguard key that the server generated for the peer, the network name, and the peer's internal CIDR. For server information, it will have the external endpoint for reaching the server's wireguard listening port over the public internet, the wireguard public key for the server, and the internal endpoint for the server's wireguard listening port once the peer has connected. All of this information together will allow a peer to bootstrap enough information to create it's initial understanding of the network and then plug into the server.

2. **Server Connection Information**: How does the client learn how to contact the server for redemption? I see the network creation takes an external IP and port, but I don't see where this gets stored or how it's communicated to clients in invites.

Answer: The client learns how to contact the server from the invite. Since it's not finished yet, the network will eventually store this information, and insert it into the invite creation process.

3. **Redemption Protocol**: What's the actual technical mechanism for redemption? Is this:
   - An HTTP API call to the server?
   - A WireGuard-based communication?
   - Something else?

Answer: To redeem the invite, the peer will use the invite to bootstrap a wireguard network with the server peer. Once it's created and connected to the wireguard interface, then it uses the internal endpoint to make an HTTP API call to the server.

4. **Key Management**: I see `CreatePeer` generates a keypair, but `RedeemPeer` allows the client to provide a new key. Is the flow:
   - Server generates temporary keypair for invite
   - Client generates their own permanent keypair
   - Client uses temporary credentials to authenticate and provides permanent key?

Answer: To handle initial key creation, the server first manually creates a temporary key pair for the peer. Then, when the peer goes to redeem the invite, they use that key to connect to the server, and simultaneously pass along a new, permanent, public key to the server so that the private key is known only to them.

5. **Invite Delivery**: Is invite delivery just manual file transfer (admin creates file, sends it to user somehow), or is there an intended automated delivery mechanism?

Answer: There is no built in delivery mechanism. The program just writes out a .toml file, and the file is delivered to the peer out-of-band.

6. **Security & Validation**: Beyond expiration time, what prevents unauthorized use of intercepted invites? Are there any additional security measures I should include in the flow?

Answer: There are no additional security measures. The invite is the "golden ticket", and its up to the network administrator to make sure that invite gets to where it needs to go.

## Questions about the Peer Redemption Flow:

**1. Invite File Format & Contents**
The `WriteInvite` method is a TODO. What exactly does an invite file contain? I can see it should include `DeviceConfig` and server peer info, but what's the specific format and what are all the fields?

Answer: The WriteInvite method writes out an Invite struct, which is composed of a DeviceConfig and a PeerConfig. Basically, the device config has enough information to set up the wireguard interface, and the peer config has enough information to bootstrap the "cord" peer. These sections will be both stored in a toml file that can be sent to a peer, who can then bootstrap themselves into the network with both a wg interface and the necessary cord information. To be more specific, the invite will include the private wireguard key that the server generated for the peer, the network name, and the peer's internal CIDR. For server information, it will have the external endpoint for reaching the server's wireguard listening port over the public internet, the wireguard public key for the server, and the internal endpoint for the server's wireguard listening port once the peer has connected. All of this information together will allow a peer to bootstrap enough information to create it's initial understanding of the network and then plug into the server.

**2. Network Communication Method**
How does the client actually contact the server during redemption? The comments mention HTTP API, but I don't see the endpoint implementations. Is there a specific `/redeem` endpoint? What's the request/response format?

Answer: There is not an HTTP API implemented yet, but there will be. We have not yet fully designed the endpoint list yet, nor have we specified what the request/response format will be: those are both things that we're hoping *this process* will inform.

**3. Temporary vs Permanent Keypair Handoff**
The comments suggest the invite contains a temporary keypair, and the client generates a permanent one during redemption. How exactly does this work? Does the client use the temporary private key to authenticate the redemption request?

Answer: To handle initial key creation, the server first manually creates a temporary key pair for the peer. Then, when the peer goes to redeem the invite, they use that key to connect to the server, and simultaneously pass along a new, permanent, public key to the server so that the private key is known only to them.

**4. Network Connectivity for Redemption**
How does the client reach the server during redemption? Do they:
- Set up a temporary WireGuard connection using the invite's temporary keypair?
- Connect over regular internet to an HTTP endpoint?
- Something else?

Answer: To redeem the invite, the peer will use the invite to bootstrap a wireguard network with the server peer. Once it's created and connected to the wireguard interface, then it uses the internal endpoint to make an HTTP API call to the server.

**5. Invite Expiration Validation**
There's an `invite_expires` field in the database. Where and how is this checked during the redemption process?

Answer: This should be checked near the beginning of the redemption process. Once the server has enough info to begin procssing a specific invite, it should check to make sure that the current time is not beyond the expiration.

**6. Error Recovery**
What happens if the redemption process fails partway through (network issues, server restart, etc.)? Can it be retried? Is there cleanup needed?

Answer: If the error is due to some kind of recoverable issue, that we should make sure it can be retried, since the invites are pretty "heavy" in terms of their permanance. However, if it's non-recoverable (like the invite is expired), then that would need to be handled differently. I'm actually not sure what we should do in the case of an expired invite, or what other "non-recoverable errors" there might be.

**7. Post-Redemption State**
After successful redemption, what is the exact state of:
- The peer record in the server database?
- The client's local configuration/database?
- Any WireGuard interfaces?

Answer: The peer record should make sure to have its `redeemed` column set to true, and its `invite_expires` column set to `0`. The client's database should be initialized, and have the server registered as a peer. The peer should also have the cord's wireguard interface configured, listening on a reachable address/port, with the server peer in the configuration.

**1. HTTP API Request Details**
Since the HTTP API isn't designed yet, what should I assume about:
- The request format (JSON with the new public key?)
- How the server identifies which invite is being redeemed (does the client send the temporary public key as identification?)
- The response format on success/failure

Answer: Do not worry about the request format, just assume the data you need is in the request—later I'll use this flow to determine what data needs to be in the request. Similar with the response, just explain the possible failure and success states in the flow, and we'll use that later to transalte into error codes and responses. Finally, during API routing, the server looks at the sending IP address to determine which peer is sending the request by looking it up in the peer table. Additionally, the sending peer will put the destination server's wireguard public key in a custom header "X-Cord-Server-Public-Key", so that the server can verify that the peer is reaching its intended destination. HOWEVER: this API routing information is beyond the scope of the peer redemption flow. We do not need to describe the routing process on every flow that can originate from the API. Focus on the process that happens below the api routing.

**2. WireGuard Interface Transition**
After successful redemption, does the client:
- Keep the existing WireGuard interface but update its private key to the permanent one?
- Tear down the temporary interface and create a new one with the permanent key?
- Or something else?

Answer: Once the peer gets word of successful redemption from the server, it will tear down the temporary interface, then rebuild and put up the permanent one.

**3. Error Response Handling**
For non-recoverable errors (like expired invites), what should the client do? Should I assume:
- The client removes the temporary WireGuard interface and reports failure?
- There's some kind of error response that tells the client what went wrong?

Answer: For non-recoverable errors, yes, the client should remove the temporary Wireguard interface and report failure, and the error response from the API will give it the information it needs to know what happened.

**4. Redemption Completion Signal**
How does the client know the redemption was fully successful and they can proceed? Is it:
- A successful HTTP response from the server?
- Should the client verify something else (like trying to fetch network state)?

Answer: The client will receive a successful HTTP response from the server. Once the client receives the OK, it can continue to 

**5. Local Database Initialization Timing**
When exactly should the client initialize their local database - before attempting redemption, or only after successful redemption? This affects error cleanup.


