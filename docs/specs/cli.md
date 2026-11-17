# Spec: the command line of frostmarks

This contract governs the app's 2 commands: the daemon `frostmarksd`, and the CLI
`marksctl` with 1 command for each RPC, the `import` command and its stdio MCP
server. Its consumers are the local user, the packaging of a host that runs the
daemon as a service, and an MCP client that starts `marksctl mcp`.

## Interface

### `frostmarksd`

```
frostmarksd [-state-dir DIR] [-socket PATH] [-lodge] [-debug] [-version]
```

| Flag | Default | Meaning |
| --- | --- | --- |
| `-state-dir` | `$XDG_STATE_HOME/frostmarks`, else `$HOME/.local/state/frostmarks` | the directory of `frostmarks.db` |
| `-socket` | `$XDG_RUNTIME_DIR/frostmarks.sock` | the private Unix socket |
| `-lodge` | off | also serve lodge's app socket, and publish the events to lodge's broker ([lodge-app.md](lodge-app.md#frostmarksd--lodge)) |
| `-debug` | off | log each published event at debug level (without `-lodge`) |
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

With `-lodge`, it also serves lodge's app socket, which systemd passes (or,
under `LODGE_ROOT`, which it makes), and it publishes each event to lodge's
broker socket instead of its own bus; [lodge-app.md](lodge-app.md) is that
contract. `LODGE_ROOT` and `LODGE_UID` are read only with `-lodge`. A start
without a lodge socket, or without user `lodge` outside `LODGE_ROOT`, exits 1.

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
| `import` | `FILE`, `-folder F`, `-dry-run`, `-json` ([the import](#the-import)) |
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
| `import` | [the output of the import](#the-output-of-the-import) |
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

### The import

```
marksctl [-socket PATH] [-json] import FILE [-folder F] [-dry-run] [-json]
```

`import` reads 1 Netscape bookmark file: the HTML export of Firefox, Chrome,
Edge, Safari and Pinboard. It makes the file's folder tree below the target
folder, and it adds each bookmark of the file with its title, its notes and its
tags. It calls only the RPCs of [api.md](api.md): `ListFolders`,
`ListBookmarks`, `CreateFolder` and `CreateBookmark`. A second import of the
same file adds nothing.

| Flag | Default | Meaning |
| --- | --- | --- |
| `-folder` | `/` | the target folder; the import makes it and its missing parents |
| `-dry-run` | off | read the file and the target, print the plan, and call no RPC that writes |
| `-json` | off | the same as the global `-json` |

#### The file

1. The file is at most 33554432 bytes (32 MiB). A larger file stops the import
   before a call, with `the file <path> is larger than 33554432 bytes`.
2. After an optional UTF-8 byte order mark, white space and comments, the file
   starts with the declaration `<!DOCTYPE NETSCAPE-Bookmark-file-1>`, in any
   letter case. Any other file stops the import with `the file <path> is no
   Netscape bookmark file`.
3. The file opens at most 1024 `<DL>` levels at 1 time. A deeper file stops the
   import with `the file <path> nests more than 1024 levels`: a folder path
   holds at most 512 folders, and the limit keeps the parser's memory linear in
   the size of the file.
4. Each byte sequence that is not valid UTF-8 becomes U+FFFD.
5. The parser reads tag names and attribute names in any letter case, and an
   attribute value in double quotes, in single quotes or without quotes. It
   decodes the HTML character references of each text and each value.

#### The tree

1. `<DL>` opens 1 level of the list, and `</DL>` closes it. An `<H3>` names the
   folder of the next `<DL>`, which opens that folder below the current 1. A
   `<DL>` with no `<H3>` before it opens no folder: that is the outer list of
   every export. The end of the file closes each open level; a `</DL>` with no
   open level is ignored.
2. An `<A>` is 1 bookmark in the current folder, and a `<DT>` outside every
   `<DL>` (Safari) is in the target folder. `<P>`, `<HR>`, `<META>`, `<TITLE>`,
   `<H1>` and every other tag change nothing.
3. Folders with the same name below the same parent are 1 folder. A folder with
   no bookmark is made too.
4. A folder's name is the text of its `<H3>`: each run of white space and
   control characters becomes 1 space, the name is trimmed, and each `/`
   becomes `-`. A name that is then empty, `.` or `..` is `Untitled`.

#### A bookmark

| Field | From |
| --- | --- |
| `url` | the `HREF` attribute, trimmed, in the form of rule 2 of [api.md](api.md#rules) |
| `title` | the text of the `<A>`: each run of white space and control characters becomes 1 space, trimmed, and cut at a character boundary to at most 2048 bytes |
| `notes` | the text of a `<DD>` after the `<A>`, up to the next tag, trimmed, and cut at a character boundary to at most 65536 bytes; empty without a `<DD>` |
| `tags` | the `TAGS` attribute, split at each comma: each tag trimmed, each run of white space in it `-`, each control character removed, lower-cased and cut to 64 characters; empty tags and repeats dropped, and at most the first 64 kept |
| `folder` | the target folder, then the names of the `<H3>` folders that hold the `<A>` |

`ADD_DATE` is read and not stored: the API has no field for it, and
`create_time` is the time of the import. `ICON`, `LAST_MODIFIED`, `PRIVATE`,
`TOREAD` and every other attribute are ignored.

#### The refusals, the duplicates and the writes

1. The parser accepts only an `http` or `https` URL with a host. It refuses an
   `<A>` with no `HREF`, a URL that does not parse, a URL of another scheme
   (`javascript:`, `data:`, `file:`, Firefox's `place:`), a URL with no host and
   a URL longer than 8192 bytes, with the text of the matching error of
   [api.md](api.md#the-errors) as the reason.
2. The import refuses a folder path that breaks a folder rule of
   [api.md](api.md#a-folder), with the rule's text as the reason, before it
   makes a folder. It refuses each bookmark in or below a refused folder, with
   the reason `the folder <path> is refused`.
3. The import reads the target's folders with `ListFolders` (`recursive`) and its
   bookmarks with `ListBookmarks` (`recursive`). A bookmark whose URL, in its
   normalized form, is already in its folder, or comes earlier in the file for
   the same folder, is skipped as a duplicate.
4. Without `-dry-run`, the import makes each missing folder with `CreateFolder`,
   each parent before its children, then adds each bookmark with
   `CreateBookmark`, in the order of the file. A `CreateFolder` that answers
   `already_exists` makes no folder; a `CreateBookmark` that answers
   `already_exists` is a duplicate, and 1 that answers `invalid_argument` is a
   refusal with the API's text as the reason.
5. Any other error of a call stops the import: it exits 1 with
   `marksctl: <error>` and prints nothing on the standard output. What it wrote
   before the error stays, and the next import of the file skips it.
6. With `-dry-run`, the import calls only `ListFolders` and `ListBookmarks`. Its
   plan names what the import would write; the daemon can still refuse a call
   at the import itself.

#### The output of the import

Without `-json`, the import prints 1 line for each item, then 1 line of counts.
The folders come first, parents before children, then the added bookmarks,
then the duplicates, each in the order of the file, then the refusals: the
refused folders, the parser's refusals, then the bookmarks that the folder
rules or the daemon refuse. The JSON lists keep the same order.

| Line | Meaning |
| --- | --- |
| `folder<TAB><path>` | a folder made |
| `add<TAB><folder><TAB><url><TAB><title>` | a bookmark added |
| `skip<TAB><folder><TAB><url><TAB><title>` | a duplicate |
| `refuse<TAB><folder><TAB><url><TAB><reason>` | a refusal: a refused folder has an empty URL |
| `folders made: <n>, bookmarks added: <n>, skipped as duplicates: <n>, refused: <n>` | the counts |

With `-dry-run`, the lines are the same, and the counts line starts with
`dry run: `. A control character of a value is printed as 1 space, as for the
other commands.

With `-json`, the import prints 1 JSON object, on several lines. Each list is
`[]` when it is empty, and `id` is only in an added bookmark of an import
without `-dry-run`:

```json
{
  "dryRun": false,
  "folder": "/imported",
  "foldersMade": 1,
  "bookmarksAdded": 1,
  "skippedDuplicates": 1,
  "refused": 1,
  "folders": ["/imported/Bookmarks bar"],
  "added": [{"id": "019a8b6e-6a00-7000-8000-000000000001", "folder": "/imported/Bookmarks bar", "url": "https://example.com/", "title": "Example"}],
  "skipped": [{"folder": "/imported/Bookmarks bar", "url": "https://example.com/", "title": "Example again"}],
  "refusals": [{"folder": "/imported", "url": "javascript:alert(1)", "title": "A script", "reason": "the URL \"javascript:alert(1)\" must start with http:// or https://"}]
}
```

The import exits 0 when it ran, with refusals or without; 1 for a file that it
cannot read, a file that rules 1 to 3 of [the file](#the-file) refuse, an
invalid `-folder`, or a call that stops it; and 2 for a usage error: no `FILE`,
an extra argument or a bad flag.

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
   `import` reads its file, and it writes only through the RPCs.
6. `-json` prints the proto3 JSON form of the response, with the lower camel case
   of the field names.

## References

- Rationale: [ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md), [ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md), [ADR-0007](../adr/0007-run-on-lodge-from-a-hand-installed-deb.md)
- Context: [design/overview.md](../design/overview.md)
- The API that the commands call: [specs/api.md](api.md)
- The lodge mode: [specs/lodge-app.md](lodge-app.md)
- Built in: [plan 0001, phase 1](../plans/0001-roadmap.md), and `-lodge` and `import` in [phase 2](../plans/0001-roadmap.md#phase-2--the-lodge-app-and-the-pilot)
