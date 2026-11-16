# Commands

frostmarks ships 2 commands: `frostmarksd`, the daemon, and `marksctl`, the CLI
with its `mcp` command. An option comes before the arguments, and `-h` prints
the usage.

## `frostmarksd`

Run it with no arguments to serve the default state directory and socket.

| Option | Meaning |
| --- | --- |
| `-state-dir` | the state directory; default `$XDG_STATE_HOME/frostmarks`, else `~/.local/state/frostmarks` |
| `-socket` | the socket path; default `$XDG_RUNTIME_DIR/frostmarks.sock` |
| `-debug` | print each call and its result |
| `-version` | print the version and exit |

The daemon creates the state directory and the SQLite store when they are
absent, binds the socket with your user's own permission bits, and removes the
socket when it stops. It serves the app's API over Connect, and no port is open.

## `marksctl`

```
marksctl [-socket PATH] [-json] COMMAND [ARGS]
```

`-socket` names the daemon's socket, and it defaults to
`$XDG_RUNTIME_DIR/frostmarks.sock`. `-json` prints the response in the proto3 JSON form
instead of 1 line for each bookmark or folder. Run `marksctl` with no command for the list of commands.

### Bookmarks

| Command | Arguments | What it does |
| --- | --- | --- |
| `create-bookmark` | `-url URL [-title] [-notes] [-tag]... [-folder]` | store 1 bookmark in an existing folder, and print it |
| `get-bookmark` | `ID` | read 1 bookmark |
| `update-bookmark` | `ID [-url] [-title] [-notes] [-add-tag]... [-remove-tag]...` | change what you name |
| `move-bookmark` | `ID -folder F` | move 1 bookmark to a folder |
| `delete-bookmark` | `ID` | delete 1 bookmark |
| `list-bookmarks` | `[-folder] [-recursive] [-tag]` | list 1 folder, or it and every folder below it |
| `search-bookmarks` | `[-folder] WORDS...` | find the bookmarks whose title, URL, notes or tags hold every word |

### Folders

| Command | Arguments | What it does |
| --- | --- | --- |
| `create-folder` | `PATH` | create 1 folder, whose parent exists |
| `rename-folder` | `PATH NEW_PATH` | rename 1 folder, with every folder and bookmark below it |
| `delete-folder` | `PATH` | delete 1 empty folder |
| `list-folders` | `[-folder] [-recursive]` | list the folders |

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
- [Connect an MCP client](../guides/mcp-client.md) for the `mcp` command.
- [Folders and permissions](../guides/folders-and-permissions.md) for what a
  folder decides.
