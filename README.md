<!-- readme-kind: tool -->
# frostmarks

frostmarks is frostyard's bookmark manager: a daemon, a CLI and a stdio MCP
server that keep your bookmarks in folders, with full-text search. It is for
people who want their bookmarks on their own machine, and for the agents that
read and file them.

**Status:** pre-release. It is an incubating repository until 2027-03-31.

**Documentation:** [frostyard.github.io/frostmarks](https://frostyard.github.io/frostmarks/)

## What it does

- **Bookmarks in folders.** Each bookmark has a URL, a title, notes, tags and 1
  folder; a folder path starts with `/`, such as `/reading/2026`, and a search reads
  the title, the URL, the notes and the tags
  ([the CLI](https://frostyard.github.io/frostmarks/reference/commands/)).
- **A daemon on a private socket.** `frostmarksd` keeps the SQLite store and
  serves the app's API on a Unix socket that only your user reaches
  ([the design](docs/design/overview.md)).
- **A CLI that scripts.** `marksctl` sends each call over that socket and prints
  a table, or 1 JSON object with `-json`.
- **MCP tools for agents.** `marksctl mcp` serves the same calls as MCP tools
  over stdio, as the local user and with no token
  ([connect an MCP client](https://frostyard.github.io/frostmarks/guides/mcp-client/)).
- **Folders decide access.** A grant names the capabilities and the folders it
  covers, and each stored bookmark is checked against the folder it holds
  ([folders and permissions](https://frostyard.github.io/frostmarks/guides/folders-and-permissions/)).
- **No lodge needed.** The daemon, the CLI and the MCP server work on their own;
  a lodge app comes in a later phase
  ([the roadmap](docs/plans/0001-roadmap.md)).

## Install

frostmarks is pre-release, so it has no tagged release yet. Install both
commands from the module:

```sh
go install github.com/frostyard/frostmarks/cmd/frostmarksd@latest
go install github.com/frostyard/frostmarks/cmd/marksctl@latest
```

Go puts them in the directory that `go env GOBIN` names, or in
`$(go env GOPATH)/bin`. Keep that directory on `PATH`.

[Install](https://frostyard.github.io/frostmarks/getting-started/install/)
lists the requirements and how to pin a version.

## Use

Start the daemon, store a bookmark, and find it again:

```sh
frostmarksd &
marksctl create-folder /reading
marksctl create-bookmark -url https://example.com/ -title Example -folder /reading
marksctl list-bookmarks -recursive
marksctl search-bookmarks example
```

`frostmarksd` keeps its state in `$XDG_STATE_HOME/frostmarks` (else
`~/.local/state/frostmarks`) and listens on
`$XDG_RUNTIME_DIR/frostmarks.sock`; `-state-dir` and `-socket` change both.
`marksctl` exits 0 on success, 1 on an error and 2 on a wrong usage.

To give an MCP client the tools, add this entry to its configuration:

```json
{"mcpServers": {"frostmarks": {"command": "marksctl", "args": ["mcp"]}}}
```

[Use marksctl](https://frostyard.github.io/frostmarks/guides/marksctl/) walks
through each command.

## How it works

1 Protobuf service, `frostyard.frostmarks.v1.Marks`, is the whole contract: the
11 RPCs give the API, the permission checks, the MCP tools and the events, and
the code that the plugins write from it is committed. The daemon serves the API
over Connect on the private socket, `marksctl` is a client of it, and the
generated MCP tools run in `marksctl mcp`. Events go to an outbox in the
transaction of their change, so a bookmark that a subscriber sees is a bookmark
that is stored. [The design](docs/design/overview.md) shows how the parts fit.

## Develop

```sh
mise install   # the pinned tools: golangci-lint, buf, protoc-gen-go, svu
make ci        # what CI runs: verify, the generated code, race, cross-builds, the manifest
```

[Build from source](https://frostyard.github.io/frostmarks/contributing/build/)
explains each target.

## Where to go next

- [AGENTS.md](AGENTS.md): the rules for agents and contributors.
- [docs/README.md](docs/README.md): the decisions, designs, specs and plans.
- [SECURITY.md](SECURITY.md): report a vulnerability privately.
- [LICENSE](LICENSE): MIT.
