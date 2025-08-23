# Core Flows

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
