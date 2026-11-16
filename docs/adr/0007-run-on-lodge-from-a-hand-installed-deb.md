# 0007 — Run on lodge from a hand-installed .deb

- **Status:** Accepted
- **Date:** 2026-11-16

## Context

Phase 2 of the [roadmap](../plans/0001-roadmap.md) makes frostmarks a lodge app.
lodge is frostyard's tailnet web shell. It is the hub of the apps on hearth: it
issues the agents' grants, it serves the 1 MCP gateway, and it embeds the event
broker
([frostyard/core ADR-0085](https://github.com/frostyard/core/blob/main/docs/adr/0085-build-new-first-party-apps-on-hearth-with-lodge-as-their-hub.md)).
lodge's [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md)
says what an app installs and serves:

- `app.json` in `/usr/share/lodge/apps/<id>/`, beside the app's web bundle. An
  app on hearth also names its hearth manifest (`hearth`) and its tool catalog
  (`tools`) there.
- An app socket, `/run/lodge/apps/<id>.sock`, owned by the app's user and group
  `lodge`, 0660, that serves `GET /health` and, for an app on hearth,
  `POST /connect/<service>/<method>` with the caller kind `principal`. The app
  closes a connection whose peer is not lodge before it reads a byte.
- The broker socket, `/run/lodge/nats.sock`, which admits an app by its peer's
  user id, and lets it publish only `<id>.>`.
- Its own system user, in group `lodge-apps` and never in group `lodge`, and a
  `sysusers.d` entry that also declares group `lodge` and user `lodge`.
- An install that writes only `/usr`
  ([frostyard/core ADR-0058](https://github.com/frostyard/core/blob/main/docs/adr/0058-ship-suite-desktop-apps-as-flatpaks-and-services-as-sysexts.md)).

The roadmap left 1 question open: does the package ship the daemon only, or the
CLI too, and where is the unit enabled? The app must also keep working with no
lodge ([ADR-0004](0004-serve-the-api-over-connect-on-a-private-socket.md)).

## Decision

**1 flag selects lodge.** `frostmarksd -lodge` serves lodge's app socket beside
the private socket, and it publishes its events to lodge's broker. Without the
flag, nothing changes: 1 private socket, and the events go to the process's own
bus. The 2 modes build from the same code, and the app's Go code imports no
lodge code: hearth's `lodge` package is the adapter.

**systemd owns the app socket.** The unit `frostmarks.socket` makes
`/run/lodge/apps/marks.sock` (user `frostmarks`, group `lodge`, 0660) and passes
it to the daemon by its name, `marks`. `/run/lodge/apps` belongs to root, so the
app's user cannot make the socket itself. Under lodge's development root
(`LODGE_ROOT`), the daemon makes the socket itself, as lodge's contract allows.

**hearth's adapter is the trust boundary.** The app socket's listener is
hearth's `lodge.Listener`, which closes a connection whose peer is not user
`lodge` (under `LODGE_ROOT`, the uid in `LODGE_UID`, else the daemon's own). The
socket serves `GET /health` outside the adapter, and `/connect/` only with
`Lodge-Caller: principal`, through hearth's `lodge.Handler`, which reads
`Lodge-Principal` and `Lodge-Grant` and puts the caller in the request's
context. Every other path is 403. The generated checks and the app's hooks then
test the grant on each call, as on the private socket.

**The broker is a transport of the outbox.** The relay publishes each event with
hearth's `events/broker` transport over `/run/lodge/nats.sock`. The transport
dials on the first publish, and again after a failed dial, so the daemon serves
its API while lodge or its broker is down: the outbox keeps each event until the
broker acknowledges it.

**The tool catalog comes from the generated tools.** The install writes
`tools.json` from `MarksTools` at build time, so the catalog and the code that
serves the tools cannot drift. `manifest.json` is a copy of `gen/manifest.json`.

**1 .deb, installed by hand.** A `vX.Y.Z` tag builds `frostyard-frostmarks`, an
amd64 `.deb` with every file under `/usr`, no maintainer script and no file in
`/etc`, and attests it with GitHub's build provenance. It ships both binaries:
`frostmarksd`, and `marksctl` for the host's own user of the store
(`sudo -u frostmarks marksctl -socket /run/frostmarks/private.sock`). The
package makes no user and enables nothing. After the install, the
administrator runs `systemd-sysusers` (the socket unit needs user
`frostmarks`), `systemctl daemon-reload`, `systemctl enable --now
frostmarks.socket` and `lodgectl reload`, 1 time, and lodge's first call starts
the service. A Go program, `internal/cmd/debcheck`, checks the built package
in the release, so the check needs no Python and no `dpkg-deb`.

## Consequences

- The app's private socket on lodge's host is `/run/frostmarks/private.sock`
  (`RuntimeDirectory=frostmarks`), and its store is `/var/lib/frostmarks`
  (`StateDirectory=frostmarks`). Only user `frostmarks` reaches the private
  socket.
- A package that runs no script cannot make its user, enable a unit or reload
  lodge. The install is 4 commands after `apt install`, in this order:
  `systemd-sysusers`, `systemctl daemon-reload`, enable the socket, then
  `lodgectl reload` ([lodge-app.md](../specs/lodge-app.md#the-units)).
- A broker that stays down fills the outbox. The relay logs each failed drain,
  so the journal shows it.
- `app.json`'s `version` is the release's version, and `manifest.json`'s is the
  API's. A release that changes no operation keeps the manifest's version.
- The `.deb` is for amd64 only, as lodge's is.
- No Flatpak and no sysext yet: the plan that built the pilot keeps packaging
  for later. A sysext can carry this package as it is, because every file is
  under `/usr`.

## Alternatives considered

- **The daemon makes its app socket itself:** rejected because
  `/run/lodge/apps` belongs to root, and a socket that the app makes needs a
  group and a mode that a system unit gives for free.
- **A second daemon for lodge:** rejected because 2 processes would open 1
  database and race the relay ([ADR-0004](0004-serve-the-api-over-connect-on-a-private-socket.md)).
- **Dial the broker at start, and fail without it:** rejected because the app
  would be down whenever the broker is, and the outbox already keeps every event
  until a transport holds it.
- **A catalog file in the tree:** rejected because it would be a second copy of
  the generated tools, and a test would have to keep the 2 equal.
- **A postinst that enables the unit:** rejected because a sysext runs no script
  (frostyard/core ADR-0058), and lodge's own package has none.

## References

- Shapes: [specs/lodge-app.md](../specs/lodge-app.md), [specs/cli.md](../specs/cli.md), [design/overview.md](../design/overview.md)
- Builds on: [ADR-0004](0004-serve-the-api-over-connect-on-a-private-socket.md), [ADR-0006](0006-store-bookmarks-in-sqlite-with-full-text-search.md)
- Built in: [plan 0001, phase 2](../plans/0001-roadmap.md#phase-2--the-lodge-app-and-the-pilot)
- lodge: [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md), [lodge ADR-0022](https://github.com/frostyard/lodge/blob/main/docs/adr/0022-lodge-embeds-a-nats-broker-for-app-events.md), [lodge ADR-0024](https://github.com/frostyard/lodge/blob/main/docs/adr/0024-agents-reach-apps-on-hearth-through-one-mcp-gateway.md)
