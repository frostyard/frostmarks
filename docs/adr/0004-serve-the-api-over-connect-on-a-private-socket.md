# 0004 — Serve the API over Connect on a private socket

- **Status:** Accepted
- **Date:** 2026-11-16

## Context

frostmarks is built on hearth, frostyard's app toolkit
([frostyard/core ADR-0085](https://github.com/frostyard/core/blob/main/docs/adr/0085-build-new-first-party-apps-on-hearth-with-lodge-as-their-hub.md)).
Its API is 1 Protobuf service, `frostyard.frostmarks.v1.Marks`, whose RPCs carry
hearth's options: a capability, an effect, its exposures and an agent
description. hearth's 4 plugins generate the Connect server and client, the
permission checks, the MCP tools, the event publishers and the manifest from
that contract.

The app must work with no lodge. Phase 1 therefore has no lodge socket, no
broker and no web UI: it has a daemon, a CLI and a stdio MCP server, and its
tests run them together.

3 callers reach the API: the CLI, the MCP server, and later the web UI and
lodge. Something must own the database and the outbox, or 2 processes would
write the same SQLite file and race the relay.

## Decision

The daemon `frostmarksd` serves the app's API over Connect on 1 private Unix
socket, `$XDG_RUNTIME_DIR/frostmarks.sock` by default, and it is the 1 process
that opens the database and runs the outbox relay.

The Connect server and client are generated from the proto by
`protoc-gen-connect-go`, run with the `simple` option, so that 1 RPC method takes
and returns the request and the response messages, with no `ConnectRequest`
wrapper. `protoc-gen-connect-go` and hearth's 4 plugins are tools of the Go
module: `go.mod` names them in its `tool` block, `go.sum` pins them, and
`buf.gen.yaml` runs them with `go run`, so no plugin comes from a registry.
`protoc-gen-go` is pinned in `mise.toml` instead, because buf runs it as a local
binary. `make gen-check` regenerates into a temporary directory and fails on a
difference, so `gen/` is never edited by hand.

The private socket acts as the local user. The handler is hearth's
`lodge.PrivateHandler`: it deletes every `Lodge-*` header of the request and puts
the local user's caller in the context, so a forged `Lodge-Principal` or
`Lodge-Grant` header reaches no handler, and every generated permission check
passes. The socket's directory is 0700 and the socket itself is 0600, and hearth's
`lodge.Listener` closes a connection of another user before the server reads a
byte of it: the socket file exists a moment before its permission is set, so the
peer check, which the kernel makes, is what keeps another user out.

1 interceptor maps the errors of the app and of hearth to Connect codes:

| Error | Code | Text |
| --- | --- | --- |
| the app's `*connect.Error` | its own | its own |
| `*authz.DeniedError` | `permission_denied` | `permission denied: needs <cap> on <selector>=<path>` |
| `authz.ErrNoCaller` | `permission_denied` | `permission denied: no caller` |
| `*authz.PathError` | `invalid_argument` | `invalid folder path "<path>": <reason>` |
| `context.Canceled` | `canceled` | the error's text |
| any other error | `internal` | the error's text |

`marksctl mcp` calls the daemon over that socket instead of opening the
database. The MCP server is the local user, hearth's `authz.LocalCaller()`, and
the generated tools call the generated client, so 1 process owns the database and
the outbox, every change publishes its event from the 1 relay, and the MCP server
needs no lock and no second database connection.

The lodge socket comes with the lodge app, in phase 2: hearth's adapter trusts
the 2 headers only there, and only after it has checked the peer's user id.

## Consequences

- The CLI and the MCP server need a running daemon. A call fails with a dial
  error when `frostmarksd` is not there.
- 1 socket path must stay shorter than the platform's limit (100 bytes on
  Linux), which the tests account for with a short temporary directory.
- The socket's permission is the app's only authentication: any process of the
  local user acts with every capability. That is the local user's own access, not
  a boundary.
- Every RPC is unary. A later streaming RPC needs its own decision.
- A second process that needs the data must call the API. It must not open the
  database, because the relay's order and the migration would race.
- The generated code is committed, so a proto change and its `gen/` are 1
  reviewable change.

## Alternatives considered

- **hearth's own Connect route:** hearth's roadmap reaches a shared route in
  phase 4, and it is not built. The app carries its own server until then.
- **`marksctl mcp` opens the database itself:** rejected because 2 processes
  would race SQLite's writes and the outbox relay, and 1 change could be
  published twice or lost.
- **gRPC instead of Connect:** rejected because Connect serves HTTP/1.1 on a Unix
  socket with no proxy, its client is 1 `http.Client`, and hearth generates its
  checks for it.
- **A TCP port with a token:** rejected because a private socket needs no port,
  no token and no firewall rule, and its permission is the file system's.
- **The socket's permission alone:** rejected because the file is created before
  `chmod` sets its permission, and every connection in that moment would act as
  the local user. The peer's user id is checked before a byte is read.

## References

- Shapes: [design/overview.md](../design/overview.md), [specs/api.md](../specs/api.md), [specs/cli.md](../specs/cli.md)
- Storage and events: [ADR-0006](0006-store-bookmarks-in-sqlite-with-full-text-search.md)
- Builds on: [ADR-0001](0001-record-architecture-decisions.md), [ADR-0005](0005-run-the-repository-gate-in-go.md)
- Amended by: [ADR-0009](0009-serve-the-connect-route-with-hearths-connectserver.md), which takes the error table from hearth's `connectserver` and adds `deadline_exceeded`
- hearth: [frostyard/core ADR-0085](https://github.com/frostyard/core/blob/main/docs/adr/0085-build-new-first-party-apps-on-hearth-with-lodge-as-their-hub.md), [hearth docs/specs/permissions.md](https://github.com/frostyard/hearth/blob/main/docs/specs/permissions.md), [hearth docs/specs/mcp-tools.md](https://github.com/frostyard/hearth/blob/main/docs/specs/mcp-tools.md)
