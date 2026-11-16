# Spec: the API of frostmarks

This contract governs the app's 1 Protobuf service,
`frostyard.frostmarks.v1.Marks`, in
[`proto/frostyard/frostmarks/v1/marks.proto`](../../proto/frostyard/frostmarks/v1/marks.proto).
Its consumers are the Connect server on the app's private socket, the client of
the CLI, the MCP tools of an agent, and, in phase 2, the web UI and lodge. The
generated Go code in `gen/go/` is derived from it.

## Interface

### The service

| Property | Value |
| --- | --- |
| Service | `frostyard.frostmarks.v1.Marks`, 11 unary RPCs |
| Capabilities | `marks:read`, `marks:write` |
| Selector | `folder`: the path of the folder that scopes a capability |
| Caller | the local user on the private socket, a lodge principal on the lodge socket |
| Transport | Connect over HTTP/1.1 on a Unix socket, the `simple` code generation ([ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md)) |

Every RPC carries hearth's options: a permission, an effect, its exposures
(`EXPOSE_UI`, `EXPOSE_AGENT`) and an agent description. Each is in the proto, and
`gen/manifest.json` publishes them.

### A bookmark

| Field | Type | Constraints |
| --- | --- | --- |
| `id` | string | a UUIDv7 that the app makes |
| `url` | string | http or https, lower-case scheme and host, no fragment, at most 8192 bytes |
| `title` | string | valid UTF-8, at most 2048 bytes; it can be empty |
| `notes` | string | valid UTF-8, at most 65536 bytes; they can be empty |
| `tags` | repeated string | lower-case, sorted, without a repeat, at most 64 of them |
| `folder` | string | 1 canonical folder path |
| `create_time` | Timestamp | when the app made the bookmark, in UTC |
| `update_time` | Timestamp | when the app last changed it, in UTC |

```json
{
  "id": "019a8b6e-6a00-7000-8000-000000000001",
  "url": "https://example.com/article",
  "title": "An article",
  "notes": "read later",
  "tags": ["go", "reading"],
  "folder": "/work",
  "createTime": "2026-11-16T09:00:00Z",
  "updateTime": "2026-11-16T09:00:00Z"
}
```

### A folder

A folder is 1 path: `/` is the root, and every other folder is `/` followed by
`/`-separated segments, in Unicode NFC, with no empty, `.` or `..` segment and no
trailing `/`, at most 1024 bytes. The root always exists and is never stored. A
folder's parent must exist.

| Field | Type | Constraints |
| --- | --- | --- |
| `path` | string | 1 canonical folder path |

### The RPCs

The permission column names the capability and the folder that the check reads.
`stored` is the folder of the bookmark that the request's `id` names, read inside
the call's transaction.

| RPC | Request | Permission | Effect | Event |
| --- | --- | --- | --- | --- |
| `CreateBookmark` | `url`, `title`, `notes`, `tags`, `folder` | `marks:write` on the request's `folder` | `EFFECT_WRITE` | created |
| `GetBookmark` | `id` | `marks:read` on `stored` | `EFFECT_READ` | none |
| `UpdateBookmark` | `id`, optional `url`, `title`, `notes`, `add_tags`, `remove_tags` | `marks:write` on `stored` | `EFFECT_WRITE` | updated |
| `MoveBookmark` | `id`, `folder` | `marks:write` on `stored` and on the request's `folder` | `EFFECT_WRITE` | moved_out, moved_in |
| `DeleteBookmark` | `id` | `marks:write` on `stored` | `EFFECT_OUTWARD` | deleted |
| `ListBookmarks` | `folder`, `recursive`, `tag` | `marks:read` on the request's `folder` | `EFFECT_READ` | none |
| `SearchBookmarks` | `query`, `folder` | `marks:read` on the request's `folder` | `EFFECT_READ` | none |
| `CreateFolder` | `path` | `marks:write` on the request's `path` | `EFFECT_WRITE` | none |
| `RenameFolder` | `path`, `new_path` | `marks:write` on the request's `path` and `new_path` | `EFFECT_WRITE` | moved_out, moved_in per bookmark |
| `DeleteFolder` | `path` | `marks:write` on the request's `path` | `EFFECT_WRITE` | none |
| `ListFolders` | `folder`, `recursive` | `marks:read` on the request's `folder` | `EFFECT_READ` | none |

An absent `folder` is the root `/`.

### The errors

| Code | Text | When |
| --- | --- | --- |
| `permission_denied` | `permission denied: needs <cap> on folder=<path>` | the caller's grant does not cover the folder |
| `permission_denied` | `permission denied: no caller` | the context carries no caller |
| `invalid_argument` | `invalid folder path "<path>": <reason>` | the path is no folder path |
| `invalid_argument` | `the folder is <n> bytes, and the limit is 1024` | the path is too long |
| `invalid_argument` | `the URL "<url>" must start with http:// or https://` | the scheme is neither http nor https |
| `invalid_argument` | `the URL "<url>" has no host` | the URL has no host |
| `invalid_argument` | `the URL is <n> bytes, and the limit is 8192` | the URL is too long |
| `invalid_argument` | `the title is <n> bytes, and the limit is 2048` | the title is too long |
| `invalid_argument` | `the notes is <n> bytes, and the limit is 65536` | the notes are too long |
| `invalid_argument` | `the <title\|notes> is not valid UTF-8` | the text is not UTF-8 |
| `invalid_argument` | `invalid tag "<tag>": <reason>` | the tag is empty, too long, or holds white space, a comma or a control character |
| `invalid_argument` | `the tag "<tag>" is in add_tags and remove_tags` | the same tag is added and removed |
| `invalid_argument` | `the bookmark would hold <n> tags, and the limit is 64` | the tags exceed the limit |
| `invalid_argument` | `the query holds no word` | the query has no word |
| `invalid_argument` | `the root folder / always exists` | `CreateFolder` names the root |
| `invalid_argument` | `the root folder / is not renamed` | `RenameFolder` names the root as `path` |
| `invalid_argument` | `no folder becomes the root folder /` | `RenameFolder` names the root as `new_path` |
| `invalid_argument` | `the new path <path> is the folder's own path` | `RenameFolder` names the same path twice |
| `invalid_argument` | `the new path <path> is below the folder <path>` | `RenameFolder` moves a folder into itself |
| `invalid_argument` | `the root folder / is not deleted` | `DeleteFolder` names the root |
| `not_found` | `no folder <path>` | the folder is not there |
| `not_found` | `no bookmark <id>` | the bookmark is not there |
| `already_exists` | `the folder <path> exists` | `CreateFolder` or `RenameFolder` finds the folder |
| `already_exists` | `the URL <url> is already bookmarked in <folder> as <id>` | the same URL is in the same folder |
| `failed_precondition` | `the folder <path> is not empty` | `DeleteFolder` names a folder that holds a bookmark or a folder |
| `internal` | the error's text | the app failed |

hearth's runtime returns the 3 permission errors and the path error, and the
interceptor of `internal/rpc` maps them to those codes
([ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md)).

### The events

An event is 1 CloudEvents 1.0 envelope in its structured JSON form, with no data.
Its `source` is `/marks`, its `author` is the caller's principal (`local` on the
private socket), its `subject` is the bookmark's id, and its `selector` is the
bookmark's folder.

| Type | Topic | When |
| --- | --- | --- |
| `org.frostyard.marks.bookmark.created` | `marks.bookmark.created.folder` | 1 bookmark is created |
| `org.frostyard.marks.bookmark.updated` | `marks.bookmark.updated.folder` | 1 bookmark's URL, title, notes or tags change |
| `org.frostyard.marks.bookmark.deleted` | `marks.bookmark.deleted.folder` | 1 bookmark is deleted |
| `org.frostyard.marks.bookmark.moved_out` | `marks.bookmark.moved_out.folder` | 1 bookmark leaves a folder |
| `org.frostyard.marks.bookmark.moved_in` | `marks.bookmark.moved_in.folder` | 1 bookmark arrives in a folder |

A topic adds 1 segment for each segment of the folder: `/work` gives
`marks.bookmark.created.folder.work`, and the root adds none. Each byte outside
`[A-Za-z0-9-]` of a segment is percent-encoded with upper-case hex. Every event
needs `marks:read` on its selector
([ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md)).

## Rules

1. A bookmark's `folder` is 1 canonical path, and the folder exists when the call
   writes the bookmark. An absent `folder` is the root, which always exists.
2. A URL is trimmed, parsed, and kept with a lower-case scheme and host, its path
   and query as they were given, and no fragment. Its length is at most 8192
   bytes after that normalization.
3. A tag is trimmed and lower-cased with `strings.ToLower`. A bookmark keeps its
   tags sorted, without a repeat, and at most 64 of them.
4. The same URL in the same folder is `already_exists`, in a create, an update of
   the URL and a move into the folder. The same URL in another folder passes.
5. The same tag in `add_tags` and in `remove_tags` is `invalid_argument`.
6. `UpdateBookmark` sets the fields that the request carries and keeps the others.
   A call that changes nothing writes no event and keeps `update_time`.
7. `MoveBookmark` to the bookmark's own folder changes nothing and writes no
   event. Otherwise it sets `update_time` and writes moved_out of the old folder,
   then moved_in of the new 1.
8. `RenameFolder` rewrites the path of the folder, of every folder below it and of
   every bookmark in them, in 1 transaction, and it writes moved_out and moved_in
   for each moved bookmark in id order. A bookmark's `update_time` does not change.
9. `DeleteFolder` deletes only a folder that holds no bookmark and no folder.
10. `ListBookmarks` lists the bookmarks of the folder, and with `recursive` of
    every folder below it, ordered by folder, `create_time` and id, optionally
    only those with 1 tag.
11. `SearchBookmarks` matches every word of the query against the title, the URL,
    the notes and the tags, in the folder and every folder below it, ordered by
    FTS5's `rank`, at most 200 of them.
12. A list or a search keeps only the rows whose folder the caller's grant covers
    with `marks:read`.
13. Each change and its event are in 1 transaction. A refused or failed call
    writes neither the change nor the event.
14. A missing bookmark is `not_found`, in the hook of an RPC on an id and in the
    RPC itself.

## Derived artifacts

| Artifact | Derivation |
| --- | --- |
| `gen/go/frostyard/frostmarks/v1/marks.pb.go` | the messages, `protoc-gen-go` |
| `gen/go/frostyard/frostmarks/v1/marksv1connect/` | the Connect server and client, `protoc-gen-connect-go` with `simple` |
| `gen/go/frostyard/frostmarks/v1/marks_hearth_authz.go` | the permission checks and the 4 hooks, `protoc-gen-hearth-authz` |
| `gen/go/frostyard/frostmarks/v1/marks_hearth_events.go` | the event specs and publishers, `protoc-gen-hearth-events` |
| `gen/go/frostyard/frostmarks/v1/marks_hearth_mcp.go` | the 11 MCP tools, `protoc-gen-hearth-mcp` |
| `gen/manifest.json` | the app's manifest, `protoc-gen-hearth-manifest` |

## References

- Rationale: [ADR-0004](../adr/0004-serve-the-api-over-connect-on-a-private-socket.md), [ADR-0006](../adr/0006-store-bookmarks-in-sqlite-with-full-text-search.md)
- Context: [design/overview.md](../design/overview.md)
- The CLI that calls it: [specs/cli.md](cli.md)
- Built in: [plan 0001, phase 1](../plans/0001-roadmap.md)
