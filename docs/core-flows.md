# Server Flows

## Network Initialization Flow

1. Admin executes `cord-server add-network <name> <cidr> <external-ip> <port>`
2. Server validates network name format (alphanumeric, hyphen, period only)
3. Server creates config file writer for `<name>.conf` to ensure write permissions
4. Server begins database transaction and initializes schema (cidr, association, peer, endpoint tables)
5. Server creates root CIDR entry (id=1) with provided network CIDR range
6. Server calculates first assignable IP from root CIDR for server peer
7. Server generates WireGuard keypair for server peer
8. Server creates server peer entry with name "cord-server", admin=true, and generated public key
9. Server immediately redeems server peer (marks redeemed=true) with same public key
10. Server creates WireGuard device configuration with server's private key, assigned IP/CIDR, and listening port
11. Server stores external endpoint information (external-ip:port) for client discovery
12. Server writes WireGuard interface configuration to `<name>.conf` file
13. Server commits database transaction
14. Server confirms network creation success
15. If any step fails: rollback database transaction, cleanup partial config files, return error

**Key Outputs:**
- SQLite database with initialized schema and server peer
- WireGuard configuration file for server to use
- Network ready to accept peer invitations

**Rollback Conditions:**
- Invalid network name format
- Cannot create config file
- Database operation failures
- File write failures

## CIDR Management Flow

1. Admin issues CIDR command via CLI or HTTP API (create/rename/delete)
2. Server identifies requesting peer through WireGuard cryptographic identity
3. Server queries database to verify peer has `admin=1` flag
4. Server validates CIDR notation (for create), names, and required parameters
5. Server begins sqlite transaction that acquires write lock on CIDR table
6. **Operation-Specific Logic**

   **For CIDR Creation**:
   - Verify new CIDR fits within root CIDR bounds
   - Insert new CIDR record with calculated prefix, length, base, and last values
   
   **For CIDR Rename**:
   - Verify target CIDR exists by name
   - Update CIDR name field in database
   
   **For CIDR Deletion**:
   - Verify target CIDR exists and is not the root CIDR (id=1)
   - Delete all associations where `cidr1` or `cidr2` references target CIDR
   - Delete the CIDR record itself

7. If all operations succeed, commit transaction
8. Return confirmation to requesting admin
9. On any failure, rollback transaction and return specific error message

**Concurrent Protection**: SQLite's single-connection pool and transaction-level write locks ensure serial execution of CIDR modifications, preventing race conditions between multiple admin operations.

## Peer Redemption Flow

1. Admin creates invite via `cord-server add-peer`, generating cryptographically secure token
2. Server inserts invite record with token, peer_name, assigned CIDR, admin flag, and expiration
3. Server writes invite file containing only: `https://server/api/v1/public/redeem?token=<token>`
4. Admin delivers invite URL to client through out-of-band channel
5. Client generates permanent WireGuard keypair locally
6. Client calls `POST https://server/api/v1/public/redeem?token=<token>` with permanent public key in body
7. Server validates token exists with `redeemed=0` and expiration not exceeded
8. Server begins database transaction
9. Server checks if peer already exists for this invite:
   - If peer exists with same public key: return existing configuration (idempotent)
   - If peer exists with different public key: return error and abort
   - If no peer exists: continue to step 10
10. Server creates peer record with permanent public key, copying CIDR and admin flag from invite, setting `confirmed=0`
11. Server sets `redeemed=1` on invite record
12. Server adds new peer to WireGuard interface configuration
13. Server commits transaction and returns complete WireGuard configuration:
    - Client's assigned IP address and netmask
    - Server's WireGuard endpoint and public key
    - Allowed IPs and routing rules
    - Listen port and MTU settings
14. Client configures local WireGuard interface with received configuration
15. Client brings up WireGuard interface and establishes connection
16. Client calls `POST /api/v1/peer/confirm` via WireGuard network, including public key in body
17. Server validates public key matches peer record for source IP with `confirmed=0`
18. Server begins transaction: sets `confirmed=1` on peer record, deletes invite record
19. Server commits transaction and returns `200 OK`
20. Client receives confirmation and marks setup complete
21. Peer now appears in results for `GetPeersOfPeerNamed()` queries (filters on `confirmed=1, disabled=0`)

**Key States:**
- Invite with `redeemed=0`: Unused token, no peer created
- Invite with `redeemed=1`, peer with `confirmed=0`: Peer created but not yet verified on network
- Peer with `confirmed=1`, invite deleted: Fully operational peer

**Idempotency:**
- `/redeem` with same public key: Returns identical configuration
- `/redeem` with different public key: Returns error (prevent key replacement attacks)
- `/confirm`: Returns success if already confirmed

**Failure Recovery:**
- If `/redeem` response lost: Client retries with same key, receives same configuration
- If `/confirm` fails: Client retries until success
- Network partition after redemption: Client has full config, retries confirmation when connected

**Rollback Conditions:**
- Invalid or expired token
- Attempted public key change on redeemed invite
- Database transaction failure
- WireGuard configuration update failure (rollback database changes)

## Association Management Flow

1. **Admin initiates association command.** A network administrator uses the CLI (`cord-server add-association`) or the future HTTP API to create or remove an association between two named CIDRs. The server uses the administrator’s credentials to authenticate the request and parses the provided CIDR names.
2. **Validate admin and CIDRs.** The server looks up the requesting peer in the database and verifies that it is marked as an admin (`admin=1`). It then confirms that both CIDR names exist in the `cidr` table and that the names are distinct; otherwise it rejects the request.
3. **Begin transaction and acquire lock.** To prevent concurrent modifications, the server begins a SQLite transaction on the `association` table. SQLite’s single‑connection pool and transaction‑level write locks ensure serial execution of association changes.
4. **Operation‑specific logic.**

   **Create association:** The server inserts a row into the `association` table mapping the two CIDR IDs. Because associations are symmetric, only one row is inserted to represent both directions.

   **Delete association:** The server deletes any rows where `cidr1`/`cidr2` match either ordering of the supplied CIDR names.

5. **Commit transaction and update state.** On success, the transaction commits. The server recalculates which peers can communicate across the affected CIDRs by resolving associated ranges. Any connected peers will be notified in future state fetches that new peers are now reachable (or unreachable after deletion). Test cases demonstrate that adding an association causes peers in previously disjoint CIDRs to see each other, while deleting it removes visibility.
6. **Return confirmation.** The server returns a success response to the administrator. If a database error occurs, the server rolls back the transaction and returns a descriptive error.

**Key Impacts:** Associations enable or restrict traffic between subnets. Creating an association immediately expands the peer list for affected CIDRs, while deleting one prunes that list. These changes are visible in subsequent peer state queries and client fetches.

## Peer State Query Flow

1. **Peer requests state.** A peer (or administrator) calls `cord-server get-peers` or sends an HTTP request like `GET /api/v1/peers`.
2. **Server looks up requester.** The server opens a context for the network and verifies that the requesting peer exists. (If API, uses IP address. If CLI, implicitly assumes server admin.) It determines whether the peer is confirmed and enabled; if not, the request is rejected.
3. **Resolve parent and associated CIDRs.** The server finds the peer’s own CIDR and any parent CIDR. It then calls `GetAssociatedCidrIdsForCidrId()` to fetch all CIDRs associated with that parent. The requesting peer may see peers in its own CIDR, parent CIDR and any associated CIDRs.
4. **Select eligible peers.** The server queries the `peer` table for all rows where `confirmed=1` and `disabled=0` within the resolved CIDR set. The requesting peer itself is excluded from the results. Each returned record includes the peer ID, name, public key, CIDR.
5. **Return peer list.** The server serializes the list as JSON and returns it to the requester. The flow is idempotent: repeated queries return identical results unless the network state (peers, associations or CIDRs) has changed.
6. **Client updates local state.** Clients use this list to determine which peers to add to or remove from their WireGuard configuration. Disabled peers disappear from the list immediately.

**Key Points:** Peer state queries are read‑only operations and can be served concurrently. They hide unconfirmed (`confirmed=0`) or disabled (`disabled=1`) peers and only reveal those peers that the requester is allowed to communicate with according to CIDR associations and the admin’s configuration.

## Peer Administration Flow

1. **Admin issues command.** The administrator uses CLI commands such as `cord-server rename-peer`, `cord-server enable-peer` or `cord-server disable-peer` (or their future HTTP API equivalents) to modify an existing peer. Each command identifies the target peer by name and, for renames, supplies the new name.
2. **Authenticate and validate.** The server identifies the requesting administrator using the context and checks the `admin` flag in the `peer` table. It verifies that the target peer exists.
3. **Operation‑specific logic:**

   **Rename:** Since the `peer` table doesn't track names, it is the peer's underlying CIDR that is actually being renamed.

   **Enable/Disable:** The server flips the `disabled` column for the peer. Setting `disabled=1` immediately removes the peer from other peers’ allowed lists; setting it back to `0` makes it visible again.

6. **Commit changes and propagate.** The server commits the update. Future state queries and client fetches will reflect the new name or enabled status.
7. **Return result.** A success message is returned. If an error occurs (e.g., peer does not exist, name conflict), the server returns an error and no changes take effect.

**Idempotency and Concurrency:** Renaming a peer to its existing name or enabling an already‑enabled peer are no‑ops. SQLite’s transaction isolation prevents concurrent administrators from producing conflicting updates.


# Client Flows

## Network Installation Flow

1. **Receive invite.** A prospective peer obtains an invite file from an administrator, which includes a network name and an invite redemption URL (e.g., `https://server/api/v1/public/redeem/{token}`).
2. **Invoke install command.** The user runs `cord install` with the invite URL. The client creates a `Context` pointing at the desired configuration and data directories.
3. **Initialize local state.** The client creates a new local SQLite database and configuration directory for the network using the network name. If a network with the same name exists, the installation fails.
4. **Generate permanent keypair.** The client generates a permanent WireGuard keypair on the local machine and stores it in the network's configuration directory. This keypair will identify the peer within the network. The private key is never shared, and the public key is shared with the server, but never sent in plaintext.
5. **Redeem the invite.** Using the redemption URL, the client sends an HTTP `POST` request containing the permanent public key. The server validates the token and returns two chunks of information: the WireGuard connection details for the "server peer"—the server endpoint and allowed CIDRs—and the peer details for client itself—including its name and IP. This step is idempotent; resending the same public key yields the same configuration. (Sending a different public key is an error.)
6. **Configure network interface.** The client uses the new server peer information to create a WireGuard interface using the server configuration returned from the server, in addition to its locally generated private key.
7. **Confirm with server.** The client then sends a `POST /api/v1/peer/confirm` request over the WireGuard tunnel with the client's public key in the payload. The server matches the public key to the peer record, sets `confirmed=1` and deletes the invite. This confirmation ensures that only reachable peers become operational.
8. **Initial state fetch.** Once confirmed, the client performs an initial fetch of the peer list (see State Synchronization Flow) to populate its local database with other peers in the network.
9. **Handle failures.** If any step fails—invalid token, network unreachable or configuration write error—the client tears down the temporary interface, deletes the partial database and invites the user to retry. TODO: deleting the network config during a partial failure after creating the key pair and calling the redeem endpoint will make that invite impossible to retry: the server will not accept a second set of keys. Is this an acceptable design?

**Key Outputs:** Successful installation yields a local database containing the network definition, a WireGuard configuration file for the new peer and a running interface ready to exchange traffic.

## State Synchronization Flow

1. **Trigger fetch.** Clients periodically (or on demand via `cord fetch`) call the server to update their view of the network. The fetch command obtains the network name and context directories, then invokes `Fetch()`.
2. **Send state request.** The client contacts the server, identifying itself by its IP address.
3. **Server computes visibility.** The server performs the Peer State Query Flow to gather all confirmed and enabled peers within the requester’s CIDR and associated CIDRs.
4. **Receive peer list and revisions.** The client receives a list of peers (and their endpoints).
5. **Reconcile local database.** The client compares the returned list with its local `peer` table. New peers are inserted; peers no longer present (deleted or disabled) are removed; existing peers have their names or endpoint information updated.
6. **Update WireGuard configuration.** Based on the updated peer list, the client regenerates its WireGuard interface and updates the interface in place.
7. **Persist state.** The client writes the updated peer and endpoint information to its local database.
8. **Error handling.** If the server is unreachable, the client continues operating with its existing state and retries later. Fetch is idempotent: repeated calls with no changes produce no side effects.

## WireGuard Interface Management Flow

1. **Bring interface up.** When the user runs `cord up` or installation completes, the client invokes the `Up()` function. The client first attempts a fetch to ensure it has the latest peer state but proceeds even if the server is unavailable.
2. **Construct configuration.** The client assembles a `DeviceConfig` containing its private key, internal CIDR and listen port. For each peer in the local database, it creates a `PeerConfig` with that peer’s public key, allowed IPs and most recent endpoint.
3. **Write interface.** Using OS‑specific APIs (e.g., `wgctrl`/`netlink`), the client writes the configuration to the WireGuard interface. It sets appropriate routing rules unless the user specifies `--no-routing` when invoking the server. The interface is then brought up, making the peer reachable on the network.
4. **Idempotent operation.** If the interface already exists and is up, writing the same configuration results in no changes. Bringing the interface up repeatedly is safe.
5. **Apply updates.** After each fetch, the client updates the interface in place: new peers are added, removed peers are deleted and existing peers may have their endpoints updated. These operations avoid tearing down the entire interface.
6. **Bring interface down.** Running `cord down` calls `Down()`, which brings the interface down and removes routes. This operation is idempotent; calling down on an already‑down interface has no effect.

**Key Outputs:** The interface management flow ensures that the local WireGuard device always reflects the current network state and that routing is configured appropriately. It abstracts away OS‑specific complexity for the user.

## Endpoint Discovery and Gossip Flow

1. **Start watching.** After bringing up the interface, the client may call `Watch()`, which launches a long‑running task.
2. **Scan interface.** At regular intervals, the watcher invokes `Scan()` to examine the local WireGuard interface and retrieve the current endpoint for each peer. It compares these endpoints with the values stored in the local `endpoint` table.
3. **Record changes.** If a peer’s endpoint has changed (for example, the peer roamed behind a new NAT), the client records the new endpoint and timestamp in its local database.
4. **Sync with server.** When changes are detected (or enough time has passed since the client reported a sighting), the client invokes `Sync()` to send its locally observed endpoint updates to the server. The request contains only changed endpoints and associated timestamps.
5. **Server stores sightings.** The server writes each reported endpoint sighting into its `endpoint` table including the endpoint itself, the sighting peer's key, the sighted peer’s key, and a timestamp. Historical sightings may be kept to aid in debugging or to support advanced gossip algorithms.
6. **Disseminate updates.** When other peers fetch state, the server includes the most recent endpoint for each peer. Clients update their local WireGuard configurations accordingly, ensuring they can reach peers at their new addresses. Clients that fail to connect to a peer may request a full list of endpoint candidates for a given peer—assuming that peer is visible to them.
7. **Iterate and decay.** The process repeats; clients continuously monitor endpoints and share updates through the server. After a predetermined period of time, endpoint sightings expire on the server. This expiration time should be longer than the time clients wait to re-send their sightings.
8. **Security considerations.** Only confirmed and enabled peers can send or receive endpoint updates.

**Key Mechanisms:** Endpoint gossip allows a peer to remain reachable even when its external address changes. By centralizing endpoint sightings on the server, the design avoids flooding the network with peer‑to‑peer endpoint announcements while still providing timely updates.

## Network Uninstallation Flow

1. **User initiates uninstall.** The user runs `cord uninstall`, specifying the network name. The client creates a context pointing at the network’s configuration and data directories.
2. **Bring interface down.** The client calls `Down()` to deactivate the WireGuard interface if it exists. This removes routes and ensures no lingering network state remains.
3. **Delete configuration.** Any WireGuard configuration files under the network’s config directory are deleted. Because the client generates configurations at runtime, this may simply involve cleaning up leftover files.
4. **Remove local database.** The client deletes the SQLite database for the network, erasing all stored peer and endpoint information.
5. **Clean up directories.** If there are no other networks using the same directories, the client may optionally remove the directories themselves. The operation is idempotent; attempting to uninstall an already removed network does nothing and returns success.
6. **Return confirmation.** The client prints a success message and exits. If any removal fails (e.g., insufficient permissions), an error is returned and the user may need to run the command with elevated privileges.

**Key Effects:** After uninstallation, the system no longer has any local state for the network. No WireGuard interfaces remain and no state will be fetched until the network is installed again.

## Administrative Remote Management Flow

1. **Select management command.** Administrators use the `cord server` subcommands on their own machines to manage remote networks without SSHing into the server. Subcommands include `peer add`, `peer rename`, `peer enable`, `peer disable`, `cidr add`, `cidr rename`, `cidr delete`, `association add` and `association delete`.
2. **Construct HTTP request.** Each command corresponds to an HTTP request against the server’s administrative API. For example, `peer add` issues `POST /api/v1/admin/peer`, `peer rename` uses `PUT`, and `association delete` uses `DELETE`.
3. **Authenticate administrator.** The server authenticates the request by only exposing the admin API over WireGuard, and verifying the client via sending IP. Only peers marked as admins can perform these operations.
4. **Execute server‑side flow.** Depending on the endpoint, the server invokes the appropriate flow.
5. **Return results.** The server returns a success or error message. The CLI prints the response to the user.
6. **Idempotency and errors.** Repeating the same request should have the same idempotency rules as invoking the functionality localy via the `cord-server` CLI. Errors such as invalid names, nonexistent resources or insufficient permissions are relayed back to the administrator.

**Key Functions:** This flow enables administrators to manage multiple networks from a single workstation. It leverages the same server flows defined elsewhere and provides a convenient UX layer over the raw HTTP API.
