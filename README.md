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

Start the daemon:

```sh
cord server daemon
```

The daemon listens on the Unix socket `/tmp/cord-server.sock` and
stores its state in a SQLite database at `data/server.db`, relative
to the current working directory. `--backend` selects the WireGuard
implementation (`auto`, `kernel`, or `userspace`) and `--debug`
enables verbose logging.

Manage the running daemon over its socket:

```sh
cord server status
cord server network add <name> <main-cidr> <external-ip>
cord server network list
```

## Client

```sh
cord client daemon
cord client network install invite.json
```

The client daemon listens on `/tmp/cord-client.sock` and stores its
state in a SQLite database at `data/client.db`, relative to the
current working directory.

## Running under systemd

An example unit for the server daemon lives in
[deploy/cord-server.service](deploy/cord-server.service). It starts
the daemon at boot, restarts it if it exits with a failure, and pins
the working directory to `/var/lib/cord` (systemd creates it), so the
SQLite database lives at `/var/lib/cord/data/server.db` and survives
reboots. The unit runs as root because the daemon must create
WireGuard interfaces. Because it keeps the default socket path, the
regular CLI works unchanged.

Install it with:

```sh
sudo cp deploy/cord-server.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now cord-server
```

Check status and follow the logs:

```sh
systemctl status cord-server
journalctl -u cord-server -f
```

To run the client daemon under systemd instead, copy the same unit
and change `ExecStart` to `/usr/local/bin/cord client daemon`; the
client's database is `data/client.db` relative to the working
directory.
