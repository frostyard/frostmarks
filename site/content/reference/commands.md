# Commands

frostmarks ships 2 commands: `frostmarksd`, the daemon, and `marksctl`, the CLI
with its `mcp` command. Both read their options with the standard library
`flag` package: an option comes before the arguments, and `-h` prints the usage.

## `frostmarksd`

Run it with no arguments to serve the default state directory and socket.

| Option | Meaning |
| --- | --- |
| `-state-dir` | the state directory; default `$XDG_STATE_HOME/frostmarks`, else `~/.local/state/frostmarks` |
| `-socket` | the socket path; default `$XDG_RUNTIME_DIR/frostmarks.sock`, and required when that variable is unset |
| `-lodge` | also serve lodge's app socket, and publish each event to lodge's broker ([Run on lodge](../guides/lodge.md)) |
| `-debug` | log each event at debug level (without `-lodge`) |
| `-version` | print the version and exit |

The daemon creates the state directory and the SQLite store when they are
absent, binds the socket with mode `0600` for your user, and removes the socket
when it stops. When it is ready, it prints `frostmarksd: serving on <path>` to
its standard error. It serves the app's API over Connect, and no port is open.

With `-lodge`, it also serves the socket that systemd's `frostmarks.socket`
passes, `/run/lodge/apps/marks.sock`, to lodge alone, and it publishes each
event to lodge's broker, `/run/lodge/nats.sock`. `LODGE_ROOT` moves both lodge
paths under a development root, where the daemon makes the app socket itself,
and `LODGE_UID` names lodge's user id there. A start without the app socket, or
without user `lodge` outside `LODGE_ROOT`, exits 1.

| Code | Meaning |
| --- | --- |
| 0 | a clean stop on SIGINT or SIGTERM |
| 1 | an error: the store, the socket, or the server |
| 2 | a wrong usage: an unknown flag, or an argument |

## `marksctl`

```
marksctl [-socket PATH] [-json] COMMAND [ARGS]
```

`-socket` names the daemon's socket, and it defaults to
`$XDG_RUNTIME_DIR/frostmarks.sock`. `-json` prints the response as proto3 JSON,
indented, instead of a table. Run `marksctl help` for the list of commands.

### Bookmarks

| Command | Arguments | What it does |
| --- | --- | --- |
| `create-bookmark` | `-url URL [-title] [-notes] [-tag]... [-folder]` | store 1 bookmark, and print its id |
| `get-bookmark` | `ID` | read 1 bookmark |
| `update-bookmark` | `ID [-url] [-title] [-notes] [-add-tag]... [-remove-tag]...` | change what you name |
| `move-bookmark` | `ID -folder F` | move 1 bookmark to a folder, which must exist |
| `delete-bookmark` | `ID` | delete 1 bookmark |
| `list-bookmarks` | `[-folder] [-recursive] [-tag]` | list a folder, which must exist, or the whole store |
| `search-bookmarks` | `[-folder] WORDS...` | search 1 folder and every folder below it |

### Folders

| Command | Arguments | What it does |
| --- | --- | --- |
| `create-folder` | `PATH` | create 1 folder, whose parent exists |
| `rename-folder` | `PATH NEW_PATH` | rename 1 folder; the new parent exists, and the new path does not |
| `delete-folder` | `PATH` | delete 1 empty folder |
| `list-folders` | `[-folder] [-recursive]` | list the folders of a folder, which must exist |

A folder path starts with `/`, such as `/reading/2026`, and `/` is the root
folder. `-folder` defaults to `/`.

### Import

| Command | Arguments | What it does |
| --- | --- | --- |
| `import` | `FILE [-folder] [-dry-run] [-json]` | add the bookmarks and folders of a browser's HTML export; a bookmark already in its folder is skipped |

`-dry-run` prints the plan and writes nothing. The file is at most 32 MiB, and
only `http` and `https` addresses come in. See
[Import your bookmarks](../guides/import.md).

### MCP

| Command | What it does |
| --- | --- |
| `mcp` | serve the bookmark tools over stdio, as the local user |

## Exit status

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | an error: the daemon refused the call, or it could not be reached |
| 2 | a wrong usage: an unknown flag, a missing argument, or an unknown command |

## See also

- [Use marksctl](../guides/marksctl.md) for a task with each command.
- [Import your bookmarks](../guides/import.md) for the `import` command.
- [Connect an MCP client](../guides/mcp-client.md) for the `mcp` command.
- [Folders and permissions](../guides/folders-and-permissions.md) for what a
  folder decides.
