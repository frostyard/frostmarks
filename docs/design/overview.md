# frostmarks

Living document. Rationale: [ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md),
[ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md),
[ADR-0007](../adr/0007-run-on-lodge-from-a-hand-installed-deb.md),
[ADR-0008](../adr/0008-build-the-web-ui-with-react-and-lodge-ui.md).
Contracts: [specs/api.md](../specs/api.md), [specs/cli.md](../specs/cli.md),
[specs/lodge-app.md](../specs/lodge-app.md).

## Overview

frostmarks is frostyard's bookmark manager: 1 Protobuf service of 11 RPCs, 1
daemon, 1 CLI and 1 stdio MCP server, stored in SQLite, and on lodge a web UI
and a save page. It is the first app on
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
   ├── internal/rpc/   Connect on the private socket and on lodge's app socket, and its client
   ├── cmd/frostmarksd the daemon: the sockets, the database, the outbox relay
   ├── cmd/marksctl    the CLI, and its stdio MCP server
   └── web/            the web UI on lodge (web/src/gen: the TypeScript code, from protoc-gen-es)
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
| `internal/rpc/` | the Connect route on the private socket (hearth's `connectserver.Private`, with its error interceptor), the handler of lodge's app socket (`/health`, and `/connect/` through hearth's `connectserver.Lodge`), both servers (`connectserver.Serve`), the socket that systemd passes, the listener's rules, and the client ([ADR-0009](../adr/0009-serve-the-connect-route-with-hearths-connectserver.md)) |
| `internal/paths/` | the default state directory and the default socket path, from the XDG variables |
| `cmd/frostmarksd` | the daemon: 1 process that opens the database, runs the relay and serves the private socket, and with `-lodge` lodge's app socket |
| `cmd/marksctl` | the CLI: 1 command for each RPC, `import`, and `mcp`, the stdio MCP server of the local user |
| `internal/netscape`, `internal/importer` | the parser of a browser's Netscape bookmark file, and the import that plans and writes it through the client's RPCs ([specs/cli.md](../specs/cli.md#the-import)) |
| `internal/catalog` | the tool catalog `tools.json` that lodge reads, from the generated tools |
| `internal/cmd/stage`, `internal/cmd/debcheck` | the install tree under `/usr`, and the check of the built `.deb` |
| `packaging/` | `app.json`, the systemd units, the `sysusers.d` entry and the nFPM config of `frostyard-frostmarks` |
| `web/` | the web UI: React with `@frostyard/lodge-ui` and `@frostyard/lodge-client`, the generated Connect client, and the save page |
| `internal/e2e/` | the tests that build both binaries: every command and every tool on the private socket, and the lodge mode with a test broker |

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
inproc.Bus                    standalone: logs the event at debug level
broker.Transport (-lodge)     on lodge: /run/lodge/nats.sock, acknowledged by JetStream
```

On lodge, the same flow starts at lodge's app socket: lodge sends the caller
in `Lodge-Principal` and `Lodge-Grant`, and hearth's `lodge.Handler` puts it in
the context in place of the local user, so the generated check and the hooks
test that grant.

A refused check rolls the transaction back, so neither the change nor its event
stays. A change that wrote no event wakes no relay work.

### The store

1 SQLite database, `<state-dir>/frostmarks.db`, through `modernc.org/sqlite` with
FTS5 and no CGO. 1 connection, WAL, foreign keys on, the schema at
`PRAGMA user_version` 1, and hearth's outbox table. The tables are `folders`,
`bookmarks`, `bookmark_tags` and `bookmarks_fts`. The rules are in
[ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md).

### The 2 modes

**Standalone.** The daemon serves the private socket, the local user holds
every capability, the events go to the process's own bus, and the MCP server is
the local user's.

**On lodge (`-lodge`, [ADR-0007](../adr/0007-run-on-lodge-from-a-hand-installed-deb.md)).**
The daemon also serves lodge's app socket, which systemd's `frostmarks.socket`
passes, and on which hearth's adapter trusts `Lodge-Principal` and `Lodge-Grant`
after the peer check. Its relay publishes to lodge's broker, so that lodge
delivers each event to the agents whose grants cover it. The relay's transport
dials on its first publish, so the API stays up while the broker is down, and the
outbox keeps the events. lodge serves the web UI from
`/usr/share/lodge/apps/marks/web/`, and the agents reach the tools of
`tools.json` through lodge's MCP gateway. The app's Go code imports no lodge,
frostyard app or Pocket code, so both modes build from the same tree.

```
browser ──https──▶ lodge ── /app/marks/…            web/ from disk (the UI and the save page)
                     ├──── /app/marks/connect/…     ─┐
agent ───OAuth────▶  └──── /gateway/mcp (marks_*)   ─┴─▶ /run/lodge/apps/marks.sock ─▶ frostmarksd
                                                                    frostmarksd ─▶ /run/lodge/nats.sock
```

### The web UI

`web/` is a React app with lodge's UI package and frame protocol
([ADR-0008](../adr/0008-build-the-web-ui-with-react-and-lodge-ui.md)). It calls
the API with a Connect client that `protoc-gen-es` generates into `web/src/gen/`,
over `/app/marks/connect/`, where lodge gives the signed-in person every
capability on the root folder. Its routes are `/` (a folder's bookmarks, and a
search with `?q=`) and `/save` (the save page,
[lodge-app.md](../specs/lodge-app.md#the-save-page)). It moves only through
lodge-client's history, so the shell keeps the address bar and Back. The
bookmarklet opens the shell's canonical URL `/marks/save`, which loads the save
page in the app's frame.

### The tests

`marks/` tests each rule of the app in process, with 1 injected clock and 1
injected id source, and 1 in-process bus with the relay's `Drain`, so that a test
reads each event without a timer. `internal/rpc/` tests the handler, the error
table, the forged header and the listener. `internal/paths/` tests the 2 tables of
the default paths. `internal/e2e/` builds `frostmarksd` and `marksctl`, serves a
real socket in a short temporary directory, calls the 11 commands and the 11 MCP
tools, and stops the daemon with SIGTERM; its import test imports a browser's
file with `-dry-run`, then for real, then again, and the second import adds
nothing; `internal/netscape` tests the parser on 1 fixture of each browser's
export, and `internal/importer` tests the plan against a fake client; a second test runs the daemon with
`-lodge` under `LODGE_ROOT`, calls it as lodge does, and reads its event from a
test broker. `internal/rpc/` tests the app socket's paths and caller kinds, and
the socket that systemd passes. `internal/catalog`, `internal/cmd/stage` and
`internal/cmd/debcheck` test lodge's catalog rules, the install tree and the
package's rules. `web/` has its own tests (Vitest, in the nsl machine): the
routes, the bookmarklet, the list, the editor and the save page, against a fake
API.

## Operational notes

- The socket's directory is 0700 and the socket is 0600, and the listener closes
  a peer of another user before the server reads a byte, so a connection of
  another user gets no response. A socket whose dial is refused is stale and is
  removed; a live 1, and any other dial error, is a refusal that names the path.
- The database holds 1 connection, so 1 long transaction holds the relay. 1
  change touches 1 to a few rows.
- A migration runs on the next start, in 1 transaction. A database of a later
  schema version is refused, so a downgrade fails loudly.
- The outbox holds an event until its transport acknowledged it, and the relay
  re-publishes a row after a restart. The bus drops an id it already delivered.
- An absent `$XDG_RUNTIME_DIR` is no socket: the daemon says so, and the
  packaging passes `-socket`.
- On lodge's host, the store is `/var/lib/frostmarks/frostmarks.db`, and only
  user `frostmarks` reaches the private socket:
  `sudo -u frostmarks marksctl -socket /run/frostmarks/private.sock list-folders`.
- A broker that is down fills the outbox, and the relay logs each failed drain
  every 5 s in the journal (`journalctl -u frostmarks`).
- `marksctl mcp` needs a running `frostmarksd`; it holds no database handle.
- `marksctl import` writes through the same RPCs as the other commands, 1 call
  for each folder and each bookmark. Each bookmark writes its own `created`
  event; a folder writes none, because folders publish no events in version 1
  ([ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md)).
  It holds no transaction across the file: a failed import leaves what it wrote,
  and the next import of the file skips it.

## References

- Rationale: [ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md), [ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md), [ADR-0007](../adr/0007-run-on-lodge-from-a-hand-installed-deb.md), [ADR-0008](../adr/0008-build-the-web-ui-with-react-and-lodge-ui.md), [ADR-0009](../adr/0009-serve-the-connect-route-with-hearths-connectserver.md)
- Contracts: [specs/api.md](../specs/api.md), [specs/cli.md](../specs/cli.md), [specs/lodge-app.md](../specs/lodge-app.md)
- Built in: [plan 0001, phase 1 and phase 2](../plans/0001-roadmap.md)
- The toolkit: [frostyard/core ADR-0085](https://github.com/frostyard/core/blob/main/docs/adr/0085-build-new-first-party-apps-on-hearth-with-lodge-as-their-hub.md), [hearth](https://github.com/frostyard/hearth)
