# frostmarks

Living document. Rationale: [ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md),
[ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md).
Contracts: [specs/api.md](../specs/api.md), [specs/cli.md](../specs/cli.md).

## Overview

frostmarks is frostyard's bookmark manager: 1 Protobuf service of 11 RPCs, 1
daemon, 1 CLI and 1 stdio MCP server, stored in SQLite. It is the first app on
hearth, frostyard's app toolkit
([frostyard/core ADR-0085](https://github.com/frostyard/core/blob/main/docs/adr/0085-build-new-first-party-apps-on-hearth-with-lodge-as-their-hub.md)):
hearth's plugins write the app's Connect server and client, its permission
checks, its MCP tools, its event publishers and its manifest from 1 proto file.

```
proto/frostyard/frostmarks/v1/marks.proto
   │  buf generate (protoc-gen-go, protoc-gen-connect-go, and hearth's 4 plugins)
   ▼
gen/go/frostyard/frostmarks/v1 …  gen/manifest.json
   │
   ├── marks/          the app: the store, the rules, the 11 RPCs, the 4 hooks
   ├── internal/rpc/   Connect on the private socket, and its client
   ├── cmd/frostmarksd the daemon: the socket, the database, the outbox relay
   └── cmd/marksctl    the CLI, and its stdio MCP server
```

## Design

### The parts

| Path | What it is |
| --- | --- |
| `proto/` | the contract: 1 service, `Marks`, 11 RPCs, 5 event messages, and hearth's options on each |
| `third_party/frostyard/hearth/v1/options.proto` | the copy of hearth's options file, at the version that `go.mod` requires |
| `gen/` | the generated Go code and the manifest, committed, and never edited by hand |
| `marks/` | the app core: `Open` (the schema and the outbox table), the rules of a URL, a tag, a folder and a text, the 7 bookmark RPCs, the 4 folder RPCs, the 4 permission hooks and the grant filter of the lists, and the app's errors |
| `marks/authorize.go` | the app's whole permission code: the 4 hooks read the stored bookmark's folder inside the call's transaction, and the lists and the search keep only the rows that the caller's grant covers |
| `internal/rpc/` | the Connect handler on the private socket (hearth's `lodge.PrivateHandler`, the error interceptor), the listener's rules, and the client |
| `internal/paths/` | the default state directory and the default socket path, from the XDG variables |
| `cmd/frostmarksd` | the daemon: 1 process that opens the database, runs the relay and serves the socket |
| `cmd/marksctl` | the CLI: 1 command for each RPC, and `mcp`, the stdio MCP server of the local user |
| `internal/e2e/` | the test that builds both binaries and calls every command and every tool |

### The data flow of 1 change

```
marksctl create-bookmark
   │ Connect over the Unix socket
   ▼
lodge.PrivateHandler          deletes every Lodge-* header, sets the local user
   ▼
generated permission check    marks:write on the request's folder
   ▼
marks.Service.inTx            BEGIN
   ├── the hook, for an RPC on an id: marks:read or marks:write on the stored folder
   ├── the change               1 row, its tags and its search row
   └── Emit…(outbox.Tx(tx))     1 event in the same transaction
   ▼                          COMMIT, then Notify
outbox.Relay                  publishes the event, then deletes its row
   ▼
inproc.Bus (phase 1)          logs the event at debug level, or lodge's broker (phase 2)
```

A refused check rolls the transaction back, so neither the change nor its event
stays. A change that wrote no event wakes no relay work.

### The store

1 SQLite database, `<state-dir>/frostmarks.db`, through `modernc.org/sqlite` with
FTS5 and no CGO. 1 connection, WAL, foreign keys on, the schema at
`PRAGMA user_version` 1, and hearth's outbox table. The tables are `folders`,
`bookmarks`, `bookmark_tags` and `bookmarks_fts`. The rules are in
[ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md).

### The 2 modes

**Standalone, now (phase 1).** The daemon serves the private socket, the local
user holds every capability, the events go to the process's own bus, and the MCP
server is the local user's. This is what the repository's tests run.

**On lodge, later (phase 2).** The app adds a lodge socket, on which hearth's
adapter trusts `Lodge-Principal` and `Lodge-Grant` after the peer check; a web UI
on frostui at `/app/marks/`; the save page `/app/marks/save?url=&title=`, which a
bookmarklet opens; and the broker transport, so that lodge delivers the app's
events to the agents whose grants cover them. The `.deb` with its systemd unit
passes `-state-dir /var/lib/frostmarks -socket /run/frostmarks/private.sock`. The
app's Go code imports no lodge, frostyard app or Pocket code, so both modes build
from the same tree.

### The tests

`marks/` tests each rule of the app in process, with 1 injected clock and 1
injected id source, and 1 in-process bus with the relay's `Drain`, so that a test
reads each event without a timer. `internal/rpc/` tests the handler, the error
table, the forged header and the listener. `internal/paths/` tests the 2 tables of
the default paths. `internal/e2e/` builds `frostmarksd` and `marksctl`, serves a
real socket in a short temporary directory, calls the 11 commands and the 11 MCP
tools, and stops the daemon with SIGTERM.

## Operational notes

- The socket's directory is 0700 and the socket is 0600: the local user only. A
  stale socket is removed; a live 1 is a refusal that names the path.
- The database holds 1 connection, so 1 long transaction holds the relay. 1
  change touches 1 to a few rows.
- A migration runs on the next start, in 1 transaction. A database of a later
  schema version is refused, so a downgrade fails loudly.
- The outbox holds an event until its transport acknowledged it, and the relay
  re-publishes a row after a restart. The bus drops an id it already delivered.
- An absent `$XDG_RUNTIME_DIR` is no socket: the daemon says so, and the
  packaging passes `-socket`.
- `marksctl mcp` needs a running `frostmarksd`; it holds no database handle.

## References

- Rationale: [ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md), [ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md)
- Contracts: [specs/api.md](../specs/api.md), [specs/cli.md](../specs/cli.md)
- Built in: [plan 0001, phase 1](../plans/0001-roadmap.md)
- The toolkit: [frostyard/core ADR-0085](https://github.com/frostyard/core/blob/main/docs/adr/0085-build-new-first-party-apps-on-hearth-with-lodge-as-their-hub.md), [hearth](https://github.com/frostyard/hearth)
