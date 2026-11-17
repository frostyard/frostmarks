# 0009 — Serve the Connect route with hearth's connectserver

- **Status:** Accepted
- **Date:** 2026-11-17

## Context

[ADR-0004](0004-serve-the-api-over-connect-on-a-private-socket.md) serves the
API over Connect on the private socket, and
[ADR-0007](0007-run-on-lodge-from-a-hand-installed-deb.md) serves it on lodge's
app socket too. `internal/rpc` wrote both routes by hand: the error
interceptor, the mux of each socket, the check of `Lodge-Caller`, the
`http.Server` with its stop, and the client's Unix transport.

hearth v0.2.0 adds the package `connectserver`
([hearth docs/specs/connect.md](https://github.com/frostyard/hearth/blob/main/docs/specs/connect.md),
[hearth ADR-0010](https://github.com/frostyard/hearth/blob/main/docs/adr/0010-serve-the-connect-route-with-a-go-mod-pinned-generator.md)).
It holds the same 5 parts for every app on hearth: `HandlerOptions` (the error
mapping), `Private`, `Lodge`, `Serve` and `UnixClient`. Its error table is
ADR-0004's table, with 1 more row.

## Decision

`internal/rpc` takes the Connect route from hearth's `connectserver`:

- the private socket serves `connectserver.Private`, and lodge's app socket
  serves `connectserver.Lodge` under `/connect/`;
- the generated handler gets `connectserver.HandlerOptions()`, so the error
  table is hearth's;
- both sockets run `connectserver.Serve`, and the CLI and the MCP server dial
  with `connectserver.UnixClient`.

frostmarks keeps what is its own: the socket files and their modes
(`Listen`, `ListenLodge`, the socket that systemd passes), `/health`, and the
403 of a path outside `/connect/` and `/health`.

## Consequences

- The error table of ADR-0004 gets 1 row: `context.DeadlineExceeded` is
  `deadline_exceeded`, not `internal`. Every other row is the same.
- A refused `Lodge-Caller` on `/connect/` is still 403 with 1 line, and the
  line is hearth's: `forbidden: the Lodge-Caller header is missing or is not
  principal`.
- A wrapped refusal keeps its outer text. The app's hooks and hearth's
  generated checks return their refusals unwrapped, so a client reads the same
  text as before.
- A server that fails closes the connections it accepted, so no call starts
  after `Serve` returns.
- The private socket's server now marks each connection with
  `lodge.ConnContext`. The private handler reads no mark, so nothing changes
  there.
- A change of the route's contract comes with a hearth upgrade, and the
  upgrade's pull request runs this repository's tests of both sockets.

## Alternatives considered

- **Keep the hand-written route:** rejected, because it repeats hearth's
  contract in each app, and a fix in hearth (such as the close of the
  connections after a failure) would not reach frostmarks.
- **Keep frostmarks' own error interceptor beside hearth's server:** rejected,
  because 2 tables would drift, and the only difference is 1 code that no
  client of frostmarks reads.

## References

- Shapes: [design/overview.md](../design/overview.md), [specs/api.md](../specs/api.md#the-errors), [specs/lodge-app.md](../specs/lodge-app.md#the-app-socket)
- Builds on: [ADR-0004](0004-serve-the-api-over-connect-on-a-private-socket.md), [ADR-0007](0007-run-on-lodge-from-a-hand-installed-deb.md)
