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

1. Client reads invite file containing temporary private key, temporary IP, permanent IP, server endpoint, and server public key
2. Client configures WireGuard interface with temporary credentials and temporary IP from invite CIDR
3. Client connects to server using temporary IP and generates permanent WireGuard keypair
4. Client calls `POST /peer/redeem` via temporary IP connection, sending permanent public key in request body
5. Server queries `invite` table matching temporary public key and source IP and `redeemed=0`
6. Server validates redemption request against `expiration` timestamp in invite record
7. Server begins database transaction
  - Server sets `redeemed=1` on invite record
  - Server creates new peer record with permanent public key, permanent IP from invite, copying `admin` flag, setting `confirmed=0`
  - Server adds permanent peer to WireGuard configuration with permanent IP alongside temporary peer
8. Server commits database transaction and returns `200 OK` to client
9. Client receives `200 OK` and updates local WireGuard interface to use permanent keypair and permanent IP
10. Client calls `POST /peer/confirm` via permanent IP connection, including public key in request body
11. Server validates public key matches expected permanent key for source permanent IP in `peer` table
12. Server begins transaction: sets `confirmed=1` on peer record, clears `invite_id` from `invite_ip` record
13. Server deletes invite record and removes temporary peer from WireGuard configuration
14. Server commits transaction and returns `200 OK` to client
15. Client removes temporary credentials and temporary IP configuration from local storage
16. Peer now appears in results for `GetPeersOfPeerNamed()` queries (filters on `confirmed=1, disabled=0`)
17. Temporary IP becomes available for reuse in future invites

**Key States:**
- Invite exists with `redeemed=0`: Unused invite with temporary IP allocated
- Invite with `redeemed=1`, peer with `confirmed=0`: Mid-transition, both IPs active in WireGuard
- Peer with `confirmed=1`, invite deleted: Fully transitioned on permanent IP, temporary IP freed

**IP Management:**
- Temporary IPs allocated from dedicated invite CIDR on invite creation
- Permanent IPs allocated from target CIDR on invite creation
- Both IPs reserved until confirmation or expiration
- Temporary IP freed and returned to pool after confirmation

**Idempotency:**
- `/peer/redeem`: Returns success if peer already exists with matching permanent key
- `/peer/confirm`: Returns success if peer already confirmed (no-op on subsequent calls)

**Failure Recovery:**
- If `/peer/redeem` response lost: Client retries with same permanent key, server returns success
- If `/peer/confirm` fails: Client retries until success or explicit rejection
- Network partition during transition: Client remains on permanent IP, retries confirmation

**Rollback Conditions:**
- Expired invite timestamp
- Invalid or missing invite record
- Mismatched temporary IP
- Duplicate permanent public key
- Database transaction failure
- WireGuard configuration update failure (rollback database changes)
