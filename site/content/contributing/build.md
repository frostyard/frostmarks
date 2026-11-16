# Build from source

The gate needs Go, `make` and the 4 tools that `mise.toml` pins. It needs no
package manager beyond that, and no network access in its tests.

## Build and check

```sh
mise install   # golangci-lint, buf, protoc-gen-go, svu
make build     # frostmarksd and marksctl into build/
make check     # format, then verify
make ci        # what CI runs: verify, the generated code, race, cross-builds, the docs
```

| Target | What it runs |
| --- | --- |
| `make build` | `go build` of `./cmd/...` into `build/`, with the version in the code |
| `make verify` | `go mod tidy -diff`, `go vet`, a gofmt check, golangci-lint, `buf lint`, `buf format --diff`, `go test ./...` |
| `make generate` | `buf generate`: the Go code, the Connect client, and hearth's manifest, permission checks, MCP tools and events |
| `make gen-check` | regenerate into a temporary directory, then fail when the committed `gen/` differs |
| `make race` | the tests under the race detector |
| `make cross` | build every package for linux/amd64 and arm64, and darwin/arm64. No windows build: the daemon serves a Unix socket |
| `make corrections-base` | compare the corrections inbox with `BASE` |
| `make manifest-check` | diff `gen/manifest.json` against the default branch, so a breaking change is caught in review |
| `make site` | build this site into `site/dist`, in strict mode |

## The generated code

`gen/` is committed: the messages, the Connect server and client, and hearth's
4 outputs. A change of `proto/frostyard/frostmarks/v1/marks.proto`, of
`third_party/` or of a plugin means `make generate`, and then a commit of `gen/`
beside the change. `make gen-check` fails when the 2 drift, so the review sees
the generated difference.

The plugins run with `go run` of the versions that `go.mod` requires, so
`go.sum` pins them and no plugin comes from a registry.

## The docs

The maintainer contracts live in `docs/`: an ADR for a decision, a design for
how the parts fit, a spec for an exact contract, and a plan for the order of
work. `make verify` fails on a relative link whose file or heading is missing,
and on a doc without a line in `docs/README.md`.

## The documentation site

```sh
make site        # strict build into site/dist
make site-serve  # preview at http://127.0.0.1:8000/frostmarks/
```

The site needs the Python lock in `site/requirements.txt` and writes its
virtual environment to `build/site-venv`. `make ci` does not build the site, so
a change that does not touch `site/` needs no Python toolchain.

## Write for users

Pages live in `site/content/`. A page explains what a user does, not how the
code works: the maintainer contracts stay in `docs/`. A new command or option
updates its page here in the same pull request.
