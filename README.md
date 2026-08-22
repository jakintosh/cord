# cord

cord coordinates WireGuard networks. A server daemon owns the network
topology and reconciles WireGuard devices, while client daemons on
peers install networks from invites and keep them in sync.

## Requirements

- Go 1.26 or newer to build from source
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
