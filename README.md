# cord

cord coordinates WireGuard networks. A server daemon owns the network
topology and reconciles WireGuard devices, while client daemons on
peers install networks from invites and keep them in sync.

## Requirements

- Go 1.25 or newer to build from source
- A host with WireGuard interface support (the `wireguard` kernel
  module, or the userspace backend)

## Building

```sh
make build   # builds bin/cord
make install # installs cord to /usr/local/bin
make test    # runs the test suite
```

## Server

Start the daemon on Linux or macOS:

```sh
sudo cord server daemon
```

The daemon creates its protected runtime directory when needed and listens on
the Unix socket `/var/run/cord/server.sock`. By default, the socket trusts
local users, so subsequent management commands do not need `sudo`:

```sh
cord server status
cord server network add <name> <main-cidr> <external-ip>
cord server network list
```

The daemon stores its state in a SQLite database at `data/server.db`, relative
to the current working directory. `--backend` selects the WireGuard
implementation (`auto`, `kernel`, or `userspace`) and `--debug`
enables verbose logging.

## Client

```sh
sudo cord client daemon
cord client network install invite.json
```

The client daemon listens on `/var/run/cord/client.sock` and stores its
state in a SQLite database at `data/client.db`, relative to the
current working directory.

## Private DNS

Enabled networks automatically configure DNS on Linux and macOS clients. Use
`<peer>.<network>.internal`, for example `nas.office.internal`; the coordination
server is `cord-server.office.internal`. Answers follow the requesting peer's
network visibility. Cord serves forward address records, not recursive Internet
DNS. Prefer fully qualified names: short-name search depends on the host resolver
and can be ambiguous with multiple networks. Concurrent networks need distinct
names and nonconflicting address ranges. Applications using their own DNS provider
may bypass the system resolver.

Linux clients require `resolvectl` and a running `systemd-resolved` used by the
host's resolver path. Cord configures DNS on its tunnel link and does not edit
`/etc/resolv.conf`. macOS clients manage marked files under `/etc/resolver` and
need permission to write there. The server binds UDP and TCP port 53 on each
network's main Cord address. The daemon needs permission to bind these ports,
they must be accessible to the network's peers, and other DNS listeners must not
conflict with them. Temporary enrollment tunnels do not configure DNS.

On macOS, Cord may overwrite or remove `/etc/resolver/<network>.internal` while
its ownership marker is intact. Removing or modifying the marker makes Cord
leave the file alone. Existing unmarked files are preserved and reported as
conflicts; move a conflicting file aside if you want Cord to manage that domain.

Use `cord client status` or `cord server status` to inspect the resource tree.
Each resource reports health and lifecycle state; activity rows (`↻`) report
their last result and last successful attempt. State arrows show observed versus
intended state, such as `failed → stopped` for unfinished cleanup. Errors and
reasons appear below the tree. Add `--verbose` (`-v`) for all activities and their
attempt times and schedules, or `--json` for the complete status response.

Administrative commands with `--json` emit their response without confirmation
text. Commands with no response write nothing on success; use the exit status
to check the outcome. Empty lists are `[]`. When registration creation uses
`--output`, the invitation is written to that file and JSON mode leaves stdout
empty. Daemon logging remains text on stderr.

Client DNS appears as a `dns-resolver` resource. An `unsupported` reason usually
means missing `resolvectl` or an unavailable `systemd-resolved`; other failures
may indicate permissions or resolver-file conflicts.

Client DNS failures leave the tunnel and synchronization running while maintenance
retries. Disable, uninstall, and shutdown attempt to remove owned DNS configuration
before closing the tunnel. Failed cleanup stays visible and retryable, even after
uninstall. On restart, active networks reapply DNS; macOS also removes abandoned
marked resolver files for inactive networks. A server DNS bind failure during
initial network startup rolls back that startup and is retried by maintenance.

DNS caches may delay changes after a rename or topology update: positive answers
have a one-hour TTL and negative answers a five-minute TTL.

## Running under systemd

Example units for the [server](deploy/cord-server.service) and
[client](deploy/cord-client.service) daemons live in `deploy/`. They start
the daemons at boot, restart them if they exit with a failure, and pin
the working directory to `/var/lib/cord` (systemd creates it), so the
SQLite databases live under `/var/lib/cord/data/` and survive reboots.
The units run as root because the daemons must create WireGuard interfaces.
The CLI uses the same default socket path.

Create the administrative group, add the users who should manage cord, and
install the desired hardened example units (omit either service if that host
only has one role):

```sh
sudo groupadd --system cord
sudo usermod --append --groups cord <admin-user>
sudo cp deploy/cord-server.service /etc/systemd/system/
sudo cp deploy/cord-client.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now cord-server cord-client
```

Check status and follow the logs:

```sh
systemctl status cord-server
journalctl -u cord-server -f
cord server status

systemctl status cord-client
journalctl -u cord-client -f
cord client status
```

Start a new login session after changing group membership.

## Socket permissions

Daemon sockets default to `0666`, intentionally trusting local user accounts.
Use `--socket-mode` to select a stricter policy:

```sh
sudo cord server daemon --socket-mode 0600 # root only
sudo -g cord cord server daemon --socket-mode 0660 # root and cord group
```

The example systemd units use `0660` and run the daemons with primary group
`cord`. Launchd deployments can apply the same policy by assigning each daemon
an administrative group.
