# frostyard/frostmarks

frostmarks is frostyard's bookmark manager: a daemon `frostmarksd`, a CLI
`marksctl` and a stdio MCP server (`marksctl mcp`), built on the toolkit hearth
(frostyard/core
[ADR-0085](https://github.com/frostyard/core/blob/main/docs/adr/0085-build-new-first-party-apps-on-hearth-with-lodge-as-their-hub.md)).
It works with no lodge, and it is a lodge app too: a web UI, a save page and
lodge's app socket. Start at
[docs/README.md](docs/README.md). The design is in
[docs/design/overview.md](docs/design/overview.md), the API contract in
[docs/specs/api.md](docs/specs/api.md), and the CLI contract in
[docs/specs/cli.md](docs/specs/cli.md).

This file (`AGENTS.md`) is the canonical agent instructions. `CLAUDE.md`,
`GEMINI.md`, `.cursorrules` and `.github/copilot-instructions.md` are symlinks
to it, and `.claude/skills` symlinks to `.agents/skills/`
([ADR-0002](docs/adr/0002-agent-portable-instruction-surface.md)). Edit only the
canonical paths; keep content tool-agnostic.

## Status

Incubating and pre-release. The repository is a frostyard/core
[Plan 0002](https://github.com/frostyard/core/blob/main/docs/plans/0002-org-portfolio-roadmap.md#incubating-repositories)
repository: its decision date is 2027-03-31. Phase 1 is built: the API, the
store, the daemon, the CLI, the stdio MCP server, the Go gate and the docs
site. Phase 2 of the [roadmap](docs/plans/0001-roadmap.md) builds the lodge
app: `frostmarksd -lodge`, the web UI, the save page and the `.deb`.

## Map

- `proto/frostyard/frostmarks/v1/marks.proto`: the 1 service,
  `frostyard.frostmarks.v1.Marks`, with 11 RPCs. Each RPC carries `hearth_rpc`
  (a permission, an effect and its exposure), its bookmarks and folders carry
  `hearth_selector` on the `folder` field, and the changes declare
  `hearth_event`.
- `third_party/frostyard/hearth/v1/options.proto`: the copy of hearth's options
  file, at the version that `go.mod` requires. A test of `internal/repocheck`
  keeps the 2 files equal.
- `gen/go/`: the generated Go code: the messages, the Connect server and
  client, the permission checks, the MCP tools and the event publishers.
  Committed; never edit it by hand.
- `gen/manifest.json`: the app's manifest, generated from the options, with app
  id `marks` and version `0.1.0`. Committed.
- `marks/`: the SQLite store with its full-text search
  ([ADR-0006](docs/adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md)),
  the service that implements the generated interface, and the rules of a
  folder path.
- `marks/authorize.go`: the 4 permission hooks, and the grant filters of the
  list and search calls.
- `internal/rpc/`: the Connect server on the app's private socket, and the
  client that `marksctl` uses ([ADR-0004](docs/adr/0004-serve-the-api-over-connect-on-a-private-socket.md));
  the handler of lodge's app socket, and the socket that systemd passes
  ([ADR-0007](docs/adr/0007-run-on-lodge-from-a-hand-installed-deb.md)).
- `internal/eventbus/`: the transport to lodge's broker, which dials on the
  first event.
- `internal/catalog/`: the tool catalog `tools.json` that lodge reads, from the
  generated tools.
- `internal/cmd/stage` and `internal/cmd/debcheck`: the install tree under
  `/usr`, and the check of the built `.deb`
  ([docs/specs/lodge-app.md](docs/specs/lodge-app.md)).
- `packaging/`: `app.json`, the systemd units, the `sysusers.d` entry and the
  nFPM config of `frostyard-frostmarks`.
- `web/`: the web UI on lodge: React with `@frostyard/lodge-ui` and
  `@frostyard/lodge-client`, and the Connect client that `protoc-gen-es` writes
  into `web/src/gen/` ([ADR-0008](docs/adr/0008-build-the-web-ui-with-react-and-lodge-ui.md)).
- `internal/paths/`: the state directory and the socket path.
- `internal/e2e/`: the tests of the whole app, over a real socket in a
  temporary directory.
- `cmd/frostmarksd` and `cmd/marksctl`: the daemon (with `-lodge`, also lodge's
  app socket and broker, in `cmd/frostmarksd/lodge.go`), and the CLI with its
  `mcp` command.
- `internal/workflowcheck` and `internal/repocheck`: the repository's own rules
  as Go tests: the workflows, the risk-path list, the corrections inbox, the
  Dependabot entries, the buf workspace, the permission hooks, the docs links
  and index, the private-host boundary, the import boundary and the copy of
  hearth's options file. The Markdown-link, private-host and import walks of
  `internal/repocheck` skip each direct child of `.agents/skills/` that holds
  a `.synced-from-core` file: frostyard/core's skill sync owns it (core
  ADR-0026), a change to it goes to core, and core's gate checks it. A marker
  anywhere else exempts nothing.
- `internal/cmd/corrections-base`: the check behind `make corrections-base`.
- `buf.yaml` and `buf.gen.yaml`: 2 Protobuf modules (`proto` and
  `third_party`), and the plugins that write `gen/`.
- `site/`: the user documentation site (ProperDocs and MaterialX, core
  ADR-0070), built by `make site`.

## Gates

- **`make ci` is the command to run before a push** (frostyard/core ADR-0083):
  `verify gen-check race cross corrections-base manifest-check web-check`.
  `web-check` runs in the nsl machine `lodge` (`NSL_MACHINE`), and CI runs it
  with `IN_NSL='sh -c'`.
- **Node, pnpm, Biome, TypeScript, Vitest and Vite run only in the nsl
  machine**, never on the host: `make web-check`, `make web-build`,
  `make web-generate`, `make web-fmt`. A change of the proto runs both
  `make generate` (host) and `make web-generate` (nsl), and commits `gen/` and
  `web/src/gen/`.
- **A pocketd agent session cannot run `nsl`**: pocketd's sandbox hides the
  user systemd bus that nsl drives its machines through
  ([frostyard/pocketgo#96](https://github.com/frostyard/pocketgo/issues/96),
  pocketgo's
  [ADR-0012](https://github.com/frostyard/pocketgo/blob/main/docs/adr/0012-run-tools-without-approval-in-a-sandbox-that-hides-pocketds-files.md)).
  In such a session, run `make ci` with `web-check` skipped: the Makefile has
  no variable for that, so run the others by name, `make verify gen-check race
  cross corrections-base manifest-check`. Never set `IN_NSL='sh -c'` there,
  which runs Node and pnpm on the host. The report names `web-check` as
  skipped; it then runs only in CI. A change that needs `make web-generate`
  cannot write `web/src/gen/` there, so the report says it is still owed.
- `make check` formats first, then verifies. `mise install` provisions golangci-lint, buf,
  protoc-gen-go and svu; Go comes from `go.mod`.
- `make verify` runs `go mod tidy -diff`, `go vet ./...`, a gofmt check, the
  pinned golangci-lint, `buf lint`, `buf format --diff --exit-code` and
  `go test ./...`.
- `make generate` writes `gen/`, the manifest included; `make gen-check`
  regenerates into a temporary directory, compares that directory with the
  tree, and fails on a changed, a missing or an extra file, then sweeps the
  directory away. Run `make generate` after a change of the proto, of the
  options or of the plugin, and commit `gen/` with the change.
- `make manifest-check` compares `gen/manifest.json` with the 1 at `BASE`, and
  `make corrections-base` compares the corrections inbox with `BASE`. CI sets
  `BASE` to the pull request's base commit.
- Tests run unfiltered (`go test ./...`, frostyard/core ADR-0038). A test uses
  a temp dir, and never sleeps, uses the network or the wall clock.
- `make vulncheck` needs the network, so it is in no gate; CI runs it as its
  own job. Fix a lint finding rather than loosening `.golangci.yml`.
- **A gate recipe fails closed.** Only an absent manifest path at an existing
  base skips the comparison: check the base commit first
  (`git rev-parse --verify`), then look the path up (`git ls-tree`: only a
  lookup that succeeds and prints no entry means absent), and never treat a
  `git ls-tree` or `git show` failure as an absent file. A shallow clone has
  no base commit, and that stops the gate with exit 2.

## Invariants

- **The app works with no lodge**, and **frostmarks imports no lodge, frostyard
  app or Pocket code**, in Go or in Protobuf (core ADR-0085, ADR-0062).
  `github.com/frostyard/hearth` and frostmarks' own module are the only
  frostyard modules a file may import; `internal/repocheck` fails on any other.
- **Every permission check of a stored bookmark reads the stored folder in the
  hook**, in `marks/authorize.go`: the hook reads the row's folder inside the
  transaction, and no request field decides it. A list or search call filters
  every row by the caller's grant, and `internal/rpc/` never checks a
  permission itself: the generated wrapper does.
- **An event goes only through the outbox**, in the transaction of its change,
  and the relay deletes a row only after its transport's acknowledgement.
- **`third_party/` is hearth's options file**, at the version that `go.mod`
  requires. Change neither by hand; a hearth upgrade copies the file and
  regenerates `gen/` in 1 pull request.
- **Generated code is never edited.** `gen/` is in the tree, and `make
  gen-check` fails on a difference without reading git.
- **No remote buf plugin runs.** Every plugin is a local binary that
  `mise.lock` pins, or `go run` of a module that `go.sum` pins.
- **Tools are pinned in `mise.toml` and `mise.lock`**, with a checksum for each
  platform. No workflow downloads a tool any other way.
- **No private hosts.** Docs write `example.com`, and never a tailnet, device or
  home-network name or a real address.
- **The private socket acts as the local user**, who holds every capability.
  `internal/rpc/` closes every connection whose peer uid is not the daemon's
  own before it reads a byte (hearth's `lodge.Listener`), and it sets the
  socket's mode to `0600`.
- The 8 `.memory/corrections/*.json` files of the template are permanent.

## Releases and dependencies

- **Who tags: Brian.** Agents never tag, push tags or release. A due release
  goes to Brian as an ask.
- `make bump` is the only way to push a `vX.Y.Z` tag. It runs on `main` when the
  tree is clean and equal to `origin/main`, runs `make check`, takes the version
  from `svu next`, and pushes an annotated tag. svu reads only `vX.Y.Z` tags
  (`.svu.yml`), so the first release is `v0.1.0`.
- The series is `vX.Y.Z`, from semver on the changes since the last tag.
- A `vX.Y.Z` tag runs `.github/workflows/release.yml`: `make web-build`, then
  `make deb`, which stages the tree (`internal/cmd/stage`), builds
  `frostyard-frostmarks_<version>_amd64.deb` with the nFPM that `mise.toml`
  pins, and checks it (`internal/cmd/debcheck`); the release job attests the
  package and attaches it to the GitHub release. The `go install` lines of
  `README.md` use `@latest`, the newest tag.
- The tags are the record of each release, so this file names no latest
  version. The newest 1 is the first line of
  `git ls-remote --tags --refs --sort=-v:refname origin 'refs/tags/v[0-9]*'`,
  or GitHub > frostyard/frostmarks > Tags.
- Internal dependencies: `github.com/frostyard/hearth` in `go.mod`, as a
  `require` and as the `tool`s of its CLI and its 4 plugins;
  `@frostyard/lodge-client` and `@frostyard/lodge-ui` in `web/package.json`,
  each a frostyard/lodge tag (`client/vX.Y.Z`, `ui/vX.Y.Z`);
  `.github/workflows/risk-paths.yml` pins frostyard/core's
  `.github/workflows/pr-triage.yml` at a commit, since core has no release
  tags.
- Who uses this repository: no frostyard repository depends on it in code.
  lodge hosts it as the app `marks` (lodge's `docs/runbook-pilot.md`).

## Repository baseline

The repository follows core's baseline (frostyard/core ADR-0071, its spec
`docs/specs/repository-baseline.md`, rows B1 to B19;
[docs/org-adrs.md](docs/org-adrs.md)). `internal/workflowcheck` and
`internal/repocheck` fail `make verify` when one of these breaks:

- The instruction symlinks above (B1). Edit `AGENTS.md`, never a link.
- `.memory/corrections/` (B2, core ADR-0081): 1 file for each correction record,
  named `<date>-<slug>.json` (from a pull request:
  `<date>-pr-<N>-<words>.json`), with exactly `date`, `scope`, `correction`,
  `evidence` and `promoted_to`, in canonical form: the text of
  `JSON.stringify(record, null, 2)` and 1 final newline, escapes and lone
  surrogates included. A record is permanent: never remove or rename a file,
  and change only an empty `promoted_to`. A fix of a blocking review point adds
  its guard and 1 record file.
- `.github/risk-paths.json` (B6, core ADR-0068, ADR-0078): schema version 2,
  each glob at `L1` to `L4`, the default `L3`, and each entry at L1 or L2 names
  a protected boundary of `policies/agent-governance.json` before its colon.
  The proto, `third_party/`, `gen/`, `marks/authorize.go` and `internal/rpc/`
  are at L1 or L2: they decide a permission, an effect, the generated checks or
  the trust boundary of the socket. A glob that matches no file yet is allowed,
  because a later task adds those paths. A release workflow, install script,
  unit or credential store joins the list in the pull request that adds it.
- `.github/dependabot.yml` (B13, core ADR-0089): exactly 1 entry for each
  ecosystem: `github-actions` at `/`, `gomod` at `/`, and `npm` at `/web`.
  Each schedule has only `interval: monthly`. A new manifest joins its
  ecosystem's entry in `directories`. A new ecosystem gets an entry.
  Each entry has exactly 1 `patterns: ["*"]` group, no `group-by`, and no
  `applies-to`.
- Every `uses:` is a 40-character commit SHA with a version comment, every
  workflow has `permissions: {}` at the top, and every checkout sets
  `persist-credentials: false` (B5).
- `README.md` (B17, B9): the gate checks the kind marker, the `# frostmarks`
  title, the `**Status:**` line, a `**Documentation:**` line that links
  <https://frostyard.github.io/frostmarks/>, the sections (`What it does`,
  `Install`, `Use`, an optional `How it works`, then `Develop` and
  `Where to go next`) and the 1000-word limit. The full rules R1 to R12 of
  core's README spec are core's `check-readme.mjs`, which the weekly sweep runs;
  no target of this repository runs it.
- No `CODEOWNERS` file at the root, in `.github/` or in `docs/` (B15), and no
  other agent framework's tree: no `docs/superpowers/`, `.planning/`,
  `.cursor/rules/`, `.gaai/`, `yeti/` or `cairn/`, and `.claude/` holds only
  `settings.json`, `skills` and `session-summary.md` (B16).

## Documentation rules (enforced)

Docs live in `docs/` in 4 categories (frostyard/core ADR-0025). Every new doc
starts from its category's `TEMPLATE.md` and follows its structure:

- `docs/adr/`: why we decided. Immutable once Accepted; a reversal is a new ADR
  that marks the old one Superseded. While the repository is pre-release, a
  Proposed ADR may still be edited in place.
- `docs/design/`: how it fits together. Living; updated in place to match
  reality.
- `docs/specs/`: exact contracts. Change only alongside the implementing code.
- `docs/plans/`: order of work. Phases with "Done when" outcomes.

### Cross-linking is mandatory

A doc without its required links is incomplete; do not finish a docs change
until they exist, in both directions:

- **ADR** links every design doc and spec it shapes, and the prior ADRs it
  builds on.
- **Design doc** links the ADR(s) providing its rationale, the spec(s) pinning
  its contracts, and the roadmap phase that builds it.
- **Spec** links its motivating ADR(s) and the design doc showing where it fits.
- **Plan** links every design doc and spec that a phase implements.

Use relative paths; `make verify` fails on a relative link whose file or heading
is missing.

### Housekeeping

- New doc: add a line to the index in [docs/README.md](docs/README.md).
- New significant decision: new ADR first, then the affected design docs and
  specs in the same change.
- Convert relative dates ("next weekend") to absolute dates in all docs.
- A change to user-visible behavior updates the affected page in `site/content/`
  in the same pull request.
- A new command or option updates the affected page under `site/content/`, and
  a change of the proto updates `docs/specs/api.md`, in the same pull request.
