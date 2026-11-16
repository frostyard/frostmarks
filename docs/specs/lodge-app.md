# Spec: frostmarks as a lodge app

This contract governs what frostmarks installs on lodge's host and what it
serves to lodge: the package's files, `app.json`, the app socket, the broker
connection, and the save page that a browser bookmarklet opens. Its consumers
are lodge (which reads the files and calls the socket), the package's build and
its check, the administrator who installs it, and the web UI. It applies
[lodge's app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md),
contract major 1, and its section "Apps on hearth"; where this spec is silent,
that contract holds. The key words MUST, MUST NOT, SHOULD and MAY are used as in
RFC 2119.

## Interface

### The package

`frostyard-frostmarks_<version>_amd64.deb`, where `<version>` is the tag
without its `v`. Every file is under `/usr`, owned by root, and no file is
group- or world-writable.

| Path | Mode | What it is |
| --- | --- | --- |
| `/usr/bin/frostmarksd` | 0755 | the daemon |
| `/usr/bin/marksctl` | 0755 | the CLI, and its `mcp` command |
| `/usr/share/lodge/apps/marks/app.json` | 0644 | lodge's manifest of the app |
| `/usr/share/lodge/apps/marks/manifest.json` | 0644 | the hearth manifest, a copy of `gen/manifest.json` |
| `/usr/share/lodge/apps/marks/tools.json` | 0644 | the tool catalog, written from the generated tools |
| `/usr/share/lodge/apps/marks/web/` | 0755, files 0644 | the web UI: `index.html`, `icon.svg`, and its scripts and styles under `assets/` |
| `/usr/lib/systemd/system/frostmarks.socket` | 0644 | the app socket's unit |
| `/usr/lib/systemd/system/frostmarks.service` | 0644 | the daemon's unit |
| `/usr/lib/sysusers.d/frostmarks.conf` | 0644 | user `frostmarks`, its group `lodge-apps`, and group and user `lodge` |

The control fields are `Package: frostyard-frostmarks`, `Version: <version>`,
`Architecture: amd64` and `Depends: systemd`. The control archive holds only
`control` and `md5sums`: no maintainer script and no conffile.

### `app.json`

```json
{
  "contract": 1,
  "id": "marks",
  "name": "Bookmarks",
  "icon": "icon.svg",
  "version": "<version>",
  "user": "frostmarks",
  "key": "b",
  "hearth": "manifest.json",
  "tools": "tools.json"
}
```

`packaging/lodge/app.json` holds this object with `"version": "dev"`, and the
build sets the version.

### The tool catalog

`tools.json` is `{"tools": [...]}`, 1 object for each operation of
`manifest.json` whose `expose` holds `agent`, with its `name`, `description`,
`inputSchema` and `outputSchema`, from `MarksTools` of the generated code. It
meets lodge's rules: at most 128 tools and 1 MiB; each name matches
`^[a-z][a-z0-9_]*$` and has at most 47 bytes; each description has at most
4096 bytes; each schema is an object of `"type": "object"`.

### The units

| Unit | Lines that matter |
| --- | --- |
| `frostmarks.socket` | `ListenStream=/run/lodge/apps/marks.sock`, `FileDescriptorName=marks`, `SocketUser=frostmarks`, `SocketGroup=lodge`, `SocketMode=0660`, `WantedBy=sockets.target` |
| `frostmarks.service` | `Requires=` and `After=frostmarks.socket`, `User=` and `Group=frostmarks`, `SupplementaryGroups=lodge-apps`, `StateDirectory=frostmarks` (0700), `RuntimeDirectory=frostmarks` (0700), `RestrictAddressFamilies=AF_UNIX`, and `ExecStart=/usr/bin/frostmarksd -lodge -state-dir /var/lib/frostmarks -socket /run/frostmarks/private.sock` |

The package enables no unit. The administrator runs
`sudo systemctl enable --now frostmarks.socket` and `sudo lodgectl reload`.

### `frostmarksd -lodge`

With `-lodge`, the daemon serves lodge's app socket beside its private socket,
and it publishes its events to lodge's broker. `LODGE_ROOT` is lodge's
development root: when it is set, every lodge path below is under it.

| What | Without `LODGE_ROOT` | With `LODGE_ROOT=DIR` |
| --- | --- | --- |
| the app socket | the socket that systemd passes with the name `marks`; with none, the daemon fails | the passed socket, else `DIR/run/lodge/apps/marks.sock`, which the daemon makes (0660) |
| lodge's uid | the uid of user `lodge`; no such user fails the start | `LODGE_UID`, else the daemon's own uid |
| the broker | `/run/lodge/nats.sock` | `DIR/run/lodge/nats.sock` |

### The app socket

HTTP/1.1. hearth's `lodge.Listener` closes every connection whose peer uid is
not lodge's, before the server reads a byte.

| Request | `Lodge-Caller` | Answer |
| --- | --- | --- |
| `GET`, `HEAD /health` | any, or none | 200, `application/json`: `{"ok": true, "version": "<version>", "protocol": 1}`, or `{"ok": false, "reason": "<1 line>"}` when the database does not answer |
| another method on `/health` | | 405 |
| `POST /connect/frostyard.frostmarks.v1.Marks/<Method>` | `principal` | 1 RPC with the Connect unary protocol, under the caller of `Lodge-Principal` and `Lodge-Grant` |
| `/connect/...` with no `Lodge-Caller`, or another kind | | 403 |
| any other path | | 403 |

A refusal is `text/plain; charset=utf-8`, 1 line. The RPCs, their errors and
their permissions are those of [api.md](api.md); the error codes are those of
[ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md).
hearth's `lodge.Handler` answers 403 when `Lodge-Principal` is malformed or
`Lodge-Grant` does not parse.

### The events

The outbox relay publishes each event of [api.md](api.md#the-events) to the broker
socket with hearth's `events/broker` transport: the event's topic is the
subject, its id is the message id, and its envelope is the data. The
connection's `user` field is `marks`, which lodge reads only under its
development root. The transport dials on its first publish, and again on the
next publish after a failed dial.

### The save page

| URL | What it is |
| --- | --- |
| `https://LODGE/marks/save?url=<url>&title=<title>` | the canonical URL: lodge's shell opens the app at the route `/save` |
| `https://LODGE/app/marks/save?url=<url>&title=<title>` | the frame's URL, which lodge serves from `web/index.html` |

`LODGE` is lodge's DNS name, `lodge.<tailnet>.ts.net`.

- `url` is required. The page offers to save it only when it parses as an
  `http` or `https` URL.
- `title` is optional, and fills the title field.
- The page lists the folders of `ListFolders` (recursive, from the root), and
  the person picks 1. The folder of the last save is chosen first; the browser
  keeps it in local storage under the key `marks:lastFolder`.
- **Save** calls `CreateBookmark` with the URL, the title, the notes and the
  tags of the form, and the chosen folder. On success the page names the
  folder, links it in the list view, and offers to close the window.
  `already_exists` shows the API's message, which names the existing
  bookmark's id, and saves nothing.
- The page sends no request to another origin, and it reads nothing from the
  page that opened it but the 2 query parameters.

### The bookmarklet

```
javascript:void(window.open('https://LODGE/marks/save?url='+encodeURIComponent(location.href)+'&title='+encodeURIComponent(document.title),'_blank','noopener'))
```

It opens the canonical URL in a new window. The window loads lodge's shell,
whose top-level navigation lodge admits from any site, and the shell loads the
save page in its frame. The web UI also offers the bookmarklet with lodge's own
origin in place of `LODGE`, as a link to drag to the browser's bookmarks.

## Rules

1. Every file of the package is a directory or a regular file under `/usr`,
   owned by root, and not group- or world-writable. The control archive holds no
   maintainer script.
2. `tools.json` holds exactly the agent operations of `manifest.json`.
3. The app socket admits only lodge's uid, and trusts `Lodge-Principal` and
   `Lodge-Grant` only on `/connect/` with `Lodge-Caller: principal`.
4. Without `-lodge`, the daemon opens no app socket and no broker connection.
5. The daemon serves both sockets while the broker is down, and the outbox keeps
   every event until the broker acknowledges it.
6. The bookmarklet opens the canonical URL `/marks/save`, never `/app/marks/save`:
   lodge refuses a cross-site request to an app route, and admits a top-level
   navigation to a shell page from any site.

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| `packaging/lodge/app.json`, `packaging/systemd/`, `packaging/sysusers.d/`, `packaging/nfpm.yaml` | [The package](#the-package), [`app.json`](#appjson), [The units](#the-units) |
| `internal/catalog` | [The tool catalog](#the-tool-catalog) |
| `internal/cmd/stage` and `internal/cmd/debcheck` | [The package](#the-package), rule 1 |
| `internal/rpc/lodge.go`, `internal/rpc/activation.go` | [`frostmarksd -lodge`](#frostmarksd--lodge), [The app socket](#the-app-socket) |
| `.github/workflows/release.yml` | [The package](#the-package) |
| `web/` (the web UI, its save page and its bookmarklet link) | [The save page](#the-save-page), [The bookmarklet](#the-bookmarklet) |

## References

- Rationale: [ADR-0007](../adr/0007-run-on-lodge-from-a-hand-installed-deb.md), [ADR-0008](../adr/0008-build-the-web-ui-with-react-and-lodge-ui.md), [ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md)
- Context: [design/overview.md](../design/overview.md)
- Related: [api.md](api.md), [cli.md](cli.md)
- Built in: [plan 0001, phase 2](../plans/0001-roadmap.md#phase-2--the-lodge-app-and-the-pilot)
- lodge: [app contract](https://github.com/frostyard/lodge/blob/main/docs/specs/app-contract.md), [lodge-http.md](https://github.com/frostyard/lodge/blob/main/docs/specs/lodge-http.md)
