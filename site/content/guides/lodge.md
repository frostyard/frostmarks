# Run on lodge

On a host that runs lodge, frostyard's tailnet web shell, frostmarks is a lodge
app: lodge shows its web UI at `/app/marks/`, opens its save page for a browser
bookmarklet, and gives an agent its bookmark tools through lodge's MCP gateway,
after you approve the agent's access. The daemon, the CLI and the MCP server
keep working on their own; lodge only adds these doors.

## Requirements

- A Debian or Ubuntu host on amd64 that runs lodge v0.5.0 or later, with its
  event broker on (the default).
- `sudo` on that host.

## Install the package

Each release attaches 1 package, `frostyard-frostmarks_<version>_amd64.deb`, to
its page on [GitHub](https://github.com/frostyard/frostmarks/releases), with a
build attestation. Check the attestation, then install it:

```sh
gh attestation verify frostyard-frostmarks_0.1.0_amd64.deb --repo frostyard/frostmarks
sudo apt install ./frostyard-frostmarks_0.1.0_amd64.deb
```

The package writes only under `/usr`: the 2 commands, lodge's app directory
`/usr/share/lodge/apps/marks/`, the systemd units and the `sysusers.d` entry of
user `frostmarks`. It runs no script and starts nothing.

## Start it

```sh
sudo systemd-sysusers
sudo systemctl daemon-reload
sudo systemctl enable --now frostmarks.socket
sudo lodgectl reload
```

`frostmarks.socket` makes lodge's app socket, `/run/lodge/apps/marks.sock`, and
the first call starts `frostmarks.service`. The service keeps its store in
`/var/lib/frostmarks`.

!!! note "Check the install"

    `sudo lodgectl apps` lists `marks` as enabled, and `https://LODGE/marks`
    shows the web UI, where `LODGE` is your lodge's name, such as
    `lodge.example.com`.

## Use it on the host

Only user `frostmarks` reaches the daemon's private socket on the host:

```sh
sudo -u frostmarks marksctl -socket /run/frostmarks/private.sock list-folders
```

The service's log is `journalctl -u frostmarks`. A broker that is down shows
there as a failed drain every 5 s; the daemon keeps each event until the broker
takes it.

## Update

Install the newer package, then restart the service:

```sh
sudo apt install ./frostyard-frostmarks_<version>_amd64.deb
sudo systemctl restart frostmarks.service
sudo lodgectl reload
```

The store's schema moves forward with the daemon, and it does not move back.

## Next

[Save from your browser](save-from-browser.md) adds the bookmarklet.
