# Install frostmarks

frostmarks ships 2 commands: `frostmarksd`, the daemon that keeps the store and
serves the API on a Unix socket, and `marksctl`, the CLI with its `mcp`
command. Both are Go programs, and both need no service beside them.

frostmarks is pre-release: it has no tagged release yet, so the install takes
the newest code of `main`.

## Requirements

- Go 1.27 or later, to install with `go install`.
- A Linux or another Unix host: the daemon serves a Unix socket.
- A writable state directory. `$XDG_STATE_HOME/frostmarks` (else
  `~/.local/state/frostmarks`) is the default.

## Installation

Install both commands from the module:

```sh
go install github.com/frostyard/frostmarks/cmd/frostmarksd@latest
go install github.com/frostyard/frostmarks/cmd/marksctl@latest
```

Go puts them in the directory that `go env GOBIN` names, or in
`$(go env GOPATH)/bin`. Keep that directory on `PATH`.

Start the daemon:

```sh
frostmarksd
```

It prints nothing when it is ready, and it serves
`$XDG_RUNTIME_DIR/frostmarks.sock`. Run it in the background, or under your
service manager of choice.

!!! note "Check the install"

    Run `marksctl list-folders`. An empty table means that the daemon answers
    and the store is new.

### Pin a version

A release is a tag in the `vX.Y.Z` series of
[frostyard/frostmarks](https://github.com/frostyard/frostmarks/tags). To
install a version that a script pins:

```sh
go install github.com/frostyard/frostmarks/cmd/frostmarksd@v0.1.0
go install github.com/frostyard/frostmarks/cmd/marksctl@v0.1.0
```

## Updates

To move to the newest code, install both commands again with `@latest`, then
restart `frostmarksd`:

```sh
go install github.com/frostyard/frostmarks/cmd/frostmarksd@latest
go install github.com/frostyard/frostmarks/cmd/marksctl@latest
```

The daemon and the CLI talk over the socket, so upgrade both together: a newer
`marksctl` may send a call that an older daemon does not know. The store's
schema moves forward with the daemon, and it does not move back.

A tagged release carries its notes on the
[releases page](https://github.com/frostyard/frostmarks/releases). Until the
first tag, the change log is the commit history of `main`.

## Next

Continue with [Use marksctl](../guides/marksctl.md) to store the first
bookmark.
