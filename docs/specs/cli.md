# Spec: the command line of frostmarks

This contract governs the app's 2 commands: the daemon `frostmarksd`, and the CLI
`marksctl` with 1 command for each RPC and its stdio MCP server. Its consumers are
the local user, the packaging of a host that runs the daemon as a service, and an
MCP client that starts `marksctl mcp`.

## Interface

### `frostmarksd`

```
frostmarksd [-state-dir DIR] [-socket PATH] [-debug] [-version]
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-state-dir` | `$XDG_STATE_HOME/frostmarks`, else `$HOME/.local/state/frostmarks` | the directory of `frostmarks.db` |
| `-socket` | `$XDG_RUNTIME_DIR/frostmarks.sock` | the private Unix socket |
| `-debug` | off | log each published event at debug level |
| `-version` | | print the version to the standard error and exit |

The daemon makes the state directory (0700) and opens
`<state-dir>/frostmarks.db`, runs the outbox relay, and listens on the socket
(0600, in a directory of 0700), which serves only the daemon's own user: another
user's connection is closed before the server reads a byte of it. It writes
exactly `frostmarksd: serving on <path>` and a newline to the standard error once
the socket accepts. On SIGINT or
SIGTERM it stops the server, then the relay, then closes the database, removes
the socket, and exits 0.

A host that runs the daemon as a service passes both paths, for example
`-state-dir /var/lib/frostmarks -socket /run/frostmarks/private.sock`.

### `marksctl`

```
marksctl [-socket PATH] [-json] COMMAND [ARGS]
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-socket` | `$XDG_RUNTIME_DIR/frostmarks.sock` | the private socket of `frostmarksd` |
| `-json` | off | print the response as proto3 JSON, on several lines |

| Command | Arguments and flags |
| --- | --- |
| `create-bookmark` | `-url URL` (required), `-title T`, `-notes N`, `-tag T` (repeatable), `-folder F` |
| `get-bookmark` | `ID` |
| `update-bookmark` | `ID`, `-url U`, `-title T`, `-notes N`, `-add-tag T`, `-remove-tag T` (both repeatable) |
| `move-bookmark` | `ID`, `-folder F` (required) |
| `delete-bookmark` | `ID` |
| `list-bookmarks` | `-folder F`, `-recursive`, `-tag T` |
| `search-bookmarks` | `-folder F`, `QUERY...` |
| `create-folder` | `PATH` |
| `rename-folder` | `PATH NEW_PATH` |
| `delete-folder` | `PATH` |
| `list-folders` | `-folder F`, `-recursive` |
| `mcp` | no argument: serve the MCP tools on the standard input and output |
| `help` | print the usage |

A flag comes before or after the command's arguments. `update-bookmark` changes
the fields whose flags it is given, and it keeps the others. `search-bookmarks`
joins its words with 1 space, so `marksctl search-bookmarks sqlite fts` sends the
query `sqlite fts`.

### The output

| Input | Standard output |
| --- | --- |
| `-json` | the response message as proto3 JSON, on several lines, and a newline |
| a bookmark command, without `-json` | 1 line: `<id><TAB><folder><TAB><url><TAB><title>` |
| a folder command, without `-json` | 1 line for each folder: `<path>` |
| a delete, without `-json` | nothing |
| `help` | the usage on the standard error |

A control character of the title, of the URL or of the folder is printed as 1
space, so that each bookmark is 1 line.

### The exit codes

| Code | Meaning |
| --- | --- |
| 0 | the command passed |
| 1 | the call failed: `marksctl: <error>` on the standard error, with the code of the API's error |
| 2 | a usage error: no command, an unknown command, a missing or an extra argument, or a bad flag. The usage goes to the standard error. |

A usage error prints 1 line that names the problem, and then the usage, on the
standard error. The 4 lines are `marksctl: unknown command "nonsense"`,
`marksctl get-bookmark: want 1 argument(s), and it has 0` (or `want at least 1
argument` for a query), the flag package's own `flag provided but not defined:
-bogus`, and `marksctl create-bookmark: -url is required`.

A call's failure is no usage error: it exits 1 with `marksctl: <error>`. So does
a host with no `$XDG_RUNTIME_DIR` and no `-socket`, with `marksctl: no socket
path: set XDG_RUNTIME_DIR, or pass -socket`.

### The MCP client

`marksctl mcp` serves the app's 11 MCP tools over the standard input and output,
as the local user, and every tool calls `frostmarksd` over the private socket. An
MCP client starts it with:

```json
{"mcpServers": {"frostmarks": {"command": "marksctl", "args": ["mcp"]}}}
```

The server's name is `frostmarks`, and its version is the CLI's build version. The
tools are `create_bookmark`, `get_bookmark`, `update_bookmark`, `move_bookmark`,
`delete_bookmark`, `list_bookmarks`, `search_bookmarks`, `create_folder`,
`rename_folder`, `delete_folder` and `list_folders`. A tool's arguments are 1 JSON
object in the proto3 JSON form of the RPC's request, and its result is the
response in the same form. A refused call is a tool result with the error flag
set, whose text is the API's error: `already_exists: the URL <url> is already
bookmarked in <folder> as <id>`.

## Rules

1. `frostmarksd` exits 0 for a stop on SIGINT or SIGTERM, 1 for an error, and 2
   for a usage error: an unknown flag or an argument.
2. `frostmarksd` refuses a socket path that holds a live socket of another daemon
   with `another frostmarksd serves <path>`, and it replaces a socket whose dial
   the kernel refused with `ECONNREFUSED`. Any other dial error stops it and
   leaves the path as it is: a timeout or a permission error says nothing about
   what holds the socket.
3. `marksctl` exits 0 for a call that passed, 1 for a call that failed, and 2 for
   a usage error.
4. `marksctl` prints 1 error line on the standard error for a failed call, and it
   prints nothing on the standard output then.
5. Every command calls the daemon over the socket: no command opens the database.
6. `-json` prints the proto3 JSON form of the response, with the lower camel case
   of the field names.

## References

- Rationale: [ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md), [ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md)
- Context: [design/overview.md](../design/overview.md)
- The API that the commands call: [specs/api.md](api.md)
- Built in: [plan 0001, phase 1](../plans/0001-roadmap.md)
