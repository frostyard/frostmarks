# Plan: Roadmap

This plan delivers frostmarks: frostyard's bookmark manager, the first app on
hearth ([frostyard/core ADR-0085](https://github.com/frostyard/core/blob/main/docs/adr/0085-build-new-first-party-apps-on-hearth-with-lodge-as-their-hub.md)).
Phase 1 builds the app with no lodge: a daemon `frostmarksd`, a CLI `marksctl`
and a stdio MCP server. Phase 2 adds the lodge app.

frostmarks is an incubating repository of frostyard/core
[Plan 0002](https://github.com/frostyard/core/blob/main/docs/plans/0002-org-portfolio-roadmap.md#incubating-repositories).
Its decision date is 2027-03-31: on that date the organization keeps the
repository as a product, or retires the work. Its exit criteria are the pilot's
"Done when" lines, under phase 2.

## Phase 1 — Onboarding and the standalone core (built 2026-11-16)

- The repository baseline of frostyard/core ADR-0071, with a Go gate
  ([ADR-0005](../adr/0005-run-the-repository-gate-in-go.md)) and the docs site.
- 1 Protobuf service, `frostyard.frostmarks.v1.Marks`, with 11 RPCs, the
  hearth options of each, and the generated code and manifest committed
  ([ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md),
  [ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md),
  [design/overview.md](../design/overview.md)).
- `marks/`: the SQLite store, the service, the rules and the 4 permission
  hooks of `marks/authorize.go`.
- `internal/rpc/`: the Connect server on the app's private socket, its client,
  `internal/paths/` for the state directory and the socket path, and
  `internal/e2e/` for the tests of the whole app.
- `cmd/frostmarksd` and `cmd/marksctl`: the daemon and the CLI, with its
  `mcp` command (the stdio MCP server of the local user).
- The user documentation site under `site/`, built by `make site`, and
  `README.md`.
- **Done when:** the standalone tests pass, and `make ci` is green.

## Phase 2 — The lodge app and the pilot

- The web UI on frostui at `/app/marks/`, served on the app's lodge socket.
- The save page `/app/marks/save?url=&title=`, which a browser bookmarklet
  opens, so that 1 click stores a bookmark.
- The lodge socket and the broker transport: hearth's adapter trusts
  `Lodge-Principal` and `Lodge-Grant` only on the app's lodge socket, after the
  peer check, and the app publishes its events to lodge's broker socket.
  `internal/repocheck` still refuses a lodge import in `marks/`, `cmd/` and
  `internal/rpc/`: the adapter, not the app, carries the lodge headers.
- An attested `frostyard-frostmarks` `.deb` with a systemd service, a
  `sysusers.d` user `frostmarks`, and `StateDirectory=frostmarks`.
- **Done when (the pilot, which is also the repository's exit criteria):**
  - Brian tells Chani to get bookmark access, lodge's consent page shows the
    requested scopes, Brian narrows and approves, and Chani's bookmark tools
    appear in her 1 gateway entry with no config change.
  - A bookmark saved from the bookmarklet reaches Chani as an event, and with a
    "now" subscription it wakes her and she tags the bookmark.
  - A grant that reads 1 folder receives only that folder's events, in an
    automated test.
  - Revoking the grant removes the tools and ends the event subscription
    together.
  - A second, standard MCP client connects through the same OAuth flow.
  - The bookmark manager works with no lodge: its daemon, CLI and stdio MCP
    server.
  - A developer outside frostyard can build a small example app on hearth from
    its README alone.

## Later / ideas

- A browser extension, as an alternative to the bookmarklet.
- Import from other bookmark managers.
- A tagged release series, which needs its own release workflow
  (`.github/workflows/release.yml`), `.goreleaser.yaml` and `packaging/`.

## Open questions

- **Question:** does the `.deb` ship the daemon only, or the CLI too, and where
  does the unit enable it? Phase 2 decides, and records it as an ADR.
- **Question:** does the web UI need the store's full-text search, or a plain
  folder list? Phase 2 decides from the pilot's use of it.

## References

- Implements: [design/overview.md](../design/overview.md),
  [specs/api.md](../specs/api.md), [specs/cli.md](../specs/cli.md)
- Rationale: [ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md),
  [ADR-0005](../adr/0005-run-the-repository-gate-in-go.md),
  [ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md)
- Builds on: [frostyard/core ADR-0085](https://github.com/frostyard/core/blob/main/docs/adr/0085-build-new-first-party-apps-on-hearth-with-lodge-as-their-hub.md),
  [frostyard/core Plan 0002](https://github.com/frostyard/core/blob/main/docs/plans/0002-org-portfolio-roadmap.md#incubating-repositories)
