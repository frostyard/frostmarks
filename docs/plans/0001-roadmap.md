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

- **Built (2026-11-16):** the web UI at `/app/marks/`, React with lodge's UI
  package and a Connect client generated from the proto, which lodge serves
  from disk ([ADR-0008](../adr/0008-build-the-web-ui-with-react-and-lodge-ui.md)).
- **Built:** the save page `/app/marks/save?url=&title=`, which the bookmarklet
  opens through the shell's canonical URL `/marks/save`, so that 1 click stores
  a bookmark ([specs/lodge-app.md](../specs/lodge-app.md#the-save-page)).
- **Built:** `frostmarksd -lodge`: lodge's app socket, on which hearth's
  adapter trusts `Lodge-Principal` and `Lodge-Grant` only after the peer check,
  and the broker transport over `/run/lodge/nats.sock`
  ([ADR-0007](../adr/0007-run-on-lodge-from-a-hand-installed-deb.md)).
  `internal/repocheck` still refuses a lodge import: hearth's adapter, not the
  app, carries the lodge headers.
- **Built:** an attested `frostyard-frostmarks` `.deb` from a `vX.Y.Z` tag, with
  the systemd socket and service, a `sysusers.d` user `frostmarks`, and
  `StateDirectory=frostmarks` ([design/overview.md](../design/overview.md)).
- **Built (2026-11-16):** `marksctl import`, which reads the Netscape bookmark
  file of Firefox, Chrome, Edge, Safari and Pinboard, and adds its folders and
  bookmarks through the existing RPCs; a second import of the file adds nothing
  ([specs/cli.md](../specs/cli.md#the-import)). It gives the pilot real data
  from the first day.
- **Next:** Brian tags `v0.1.0` and installs the `.deb` on lodge's host; then
  the pilot runs its acceptance from lodge's runbook.
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
- An arm64 `.deb`, a sysext and a Flatpak.

## Open questions

- **Answered (2026-11-16):** the `.deb` ships the daemon and the CLI, and it
  enables no unit: the administrator enables `frostmarks.socket`
  ([ADR-0007](../adr/0007-run-on-lodge-from-a-hand-installed-deb.md)).
- **Answered (2026-11-16):** the web UI uses the store's full-text search, as
  the CLI and the agents do: 1 search field over the current folder and below.

## References

- Implements: [design/overview.md](../design/overview.md),
  [specs/api.md](../specs/api.md), [specs/cli.md](../specs/cli.md),
  [specs/lodge-app.md](../specs/lodge-app.md)
- Rationale: [ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md),
  [ADR-0005](../adr/0005-run-the-repository-gate-in-go.md),
  [ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md),
  [ADR-0007](../adr/0007-run-on-lodge-from-a-hand-installed-deb.md),
  [ADR-0008](../adr/0008-build-the-web-ui-with-react-and-lodge-ui.md)
- Builds on: [frostyard/core ADR-0085](https://github.com/frostyard/core/blob/main/docs/adr/0085-build-new-first-party-apps-on-hearth-with-lodge-as-their-hub.md),
  [frostyard/core Plan 0002](https://github.com/frostyard/core/blob/main/docs/plans/0002-org-portfolio-roadmap.md#incubating-repositories)
