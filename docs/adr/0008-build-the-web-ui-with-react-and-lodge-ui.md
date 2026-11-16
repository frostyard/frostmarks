# 0008 — Build the web UI with React and lodge-ui

- **Status:** Accepted
- **Date:** 2026-11-16

## Context

On lodge, frostmarks needs a web UI at `/app/marks/` that lists, searches,
adds, edits, moves and deletes bookmarks, and a save page that a browser
bookmarklet opens ([plan 0001, phase 2](../plans/0001-roadmap.md#phase-2--the-lodge-app-and-the-pilot)).
The page runs in a frame of lodge's shell, which loads its scripts and styles
only from the bundle's `assets/`
([lodge-http.md](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md#headers)).
People open it in a desktop browser and on a phone.

2 UI kits exist in the suite:

- **frostui** is a Go kit for desktop windows on MyGo. It compiles for js/wasm,
  but its
  [ADR-0005](https://github.com/frostyard/frostui/blob/main/docs/adr/0005-build-everywhere-with-freedesktop-parts-behind-build-tags.md)
  says that "js/wasm is a compile check for reuse on the web; it is not a
  supported runtime". In a browser, MyGo draws on a canvas: about 10 MB of
  WebAssembly, no accessibility tree, and typing through a hidden text field.
  lodge's app policy has no `'wasm-unsafe-eval'`, so a browser refuses to
  compile the module at all.
- **`@frostyard/lodge-ui`** is lodge's UI package for web apps: the colour
  tokens, the Tailwind theme, the type roles and a few React parts (lodge
  ADR-0018). frostnotes' web app uses it with React and
  `@frostyard/lodge-client`, the frame protocol.

hearth's goal is 1 contract for the Go and the TypeScript clients
([frostyard/core ADR-0085](https://github.com/frostyard/core/blob/main/docs/adr/0085-build-new-first-party-apps-on-hearth-with-lodge-as-their-hub.md)).
The browser reaches the app's Connect API through lodge's app calls route,
`/app/marks/connect/<service>/<method>`.

## Decision

**The web UI is React with `@frostyard/lodge-ui` and `@frostyard/lodge-client`**,
in `web/`, as frostnotes' web app is. Both packages are pinned to lodge's release
tags (`ui/vX.Y.Z`, `client/vX.Y.Z`) as pnpm git dependencies.

**The client is generated from the proto.** `protoc-gen-es`, which
`web/package.json` pins, writes the messages and the service descriptor into
`web/src/gen/` from the repository's buf workspace, and
`@connectrpc/connect-web` calls them with the Connect protocol's JSON form, the
1 that lodge's route admits. The generated files are committed, and
`make web-check` regenerates them and fails on a difference, as `make
gen-check` does for Go.

**The tools run in the nsl machine.** Node, pnpm, Biome, TypeScript, Vitest and
Vite run only there (`make web-check`, `make web-build`, `make web-generate`);
CI and the release, whose runners have Node, run the same targets with
`IN_NSL='sh -c'`. `make ci` runs `web-check`.

**The save page is a route of the same bundle**, `/save?url=&title=`, and the
bookmarklet opens its canonical URL, `https://LODGE/marks/save?...`, in a new
window ([lodge-app.md](../specs/lodge-app.md#the-save-page)). lodge refuses a
cross-site request to an app route, and admits a top-level navigation to a
shell page from any site, so the shell loads the page and the page sends no
request to another origin.

## Consequences

- The web UI and a later desktop app on frostui are 2 UIs of 1 API. The
  desktop app is no part of this decision.
- A change of the proto changes `web/src/gen/` too. `make generate` writes the
  Go code on the host, and `make web-generate` writes the TypeScript code in
  the nsl machine, so a proto change needs both.
- `make ci` needs the nsl machine, as lodge's and frostnotes' do.
- The bundle depends on lodge's release tags for its look and its frame
  protocol: a new lodge-ui version is a deliberate update of `web/package.json`.
- React 19 refuses a `javascript:` URL in `href`, so the UI sets the
  bookmarklet's link with the DOM, and a click on it in the page does nothing.

## Alternatives considered

- **frostui on js/wasm:** rejected because frostui does not support the web as
  a runtime (its ADR-0005), lodge's policy would need `'wasm-unsafe-eval'` and a
  lodge release first, and a canvas UI is heavy and weak for a phone and a
  screen reader.
- **A hand-written fetch client:** rejected because the generated client keeps
  the field names, the types and the service path of the proto, and costs 1
  generator that `package.json` pins.
- **The bookmarklet opens `/app/marks/save` directly:** rejected because lodge
  refuses that cross-site request.

## References

- Shapes: [specs/lodge-app.md](../specs/lodge-app.md), [design/overview.md](../design/overview.md)
- Builds on: [ADR-0004](0004-serve-the-api-over-connect-on-a-private-socket.md), [ADR-0007](0007-run-on-lodge-from-a-hand-installed-deb.md)
- Built in: [plan 0001, phase 2](../plans/0001-roadmap.md#phase-2--the-lodge-app-and-the-pilot)
- lodge: [ui-package.md](https://github.com/frostyard/lodge/blob/main/docs/specs/ui-package.md), [lodge-client.md](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-client.md)
