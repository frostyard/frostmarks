# 0006 — Store bookmarks in SQLite with full-text search

- **Status:** Accepted
- **Date:** 2026-11-16

## Context

A bookmark manager reads a whole folder, searches its own text, and changes 1 row
at a time. The app is 1 daemon of 1 local user, on 1 machine, with no server to
run and no database to administer. It must build with `CGO_ENABLED=0`, so that the
binary is 1 static file and the `.deb` needs no C library.

The app also publishes 1 event per change, and hearth's outbox holds each event
in the transaction of its change: a change must never be published without its
event, and an event must never survive a change that rolled back.

## Decision

The app stores its data in 1 SQLite database, `frostmarks.db` in its state
directory, through `modernc.org/sqlite`: pure Go, no CGO, with SQLite's FTS5
compiled in.

The database holds 1 connection (`SetMaxOpenConns(1)`), so the app's transactions
and the relay's deletes wait for each other instead of failing with a lock, and
WAL is on, so a reader never blocks the relay. Foreign keys are on, so a deleted
bookmark takes its tags with it. The schema version is `PRAGMA user_version`, and
each migration runs in 1 transaction: an open database at the current version is
a no-op, and a database of a later version is refused.

The tables are `folders(path)`, `bookmarks(id, url, title, notes, folder,
create_time, update_time)`, with `UNIQUE(folder, url)`, `bookmark_tags` and the
FTS5 table `bookmarks_fts(id UNINDEXED, title, url, notes, tags)`. The times are
Unix nanoseconds in UTC.

The rules that the storage backs:

- **A URL is http or https, with a lower-case scheme and host, and no
  fragment.** A saved `javascript:` URL is a script in a later web UI, and 1 click
  on a saved bookmark would run it in the app's own page. `ftp:` and `mailto:`
  have no saved page to open.
- **The same URL in the same folder is refused**, with the id of the bookmark
  that holds it. `UNIQUE(folder, url)` backs the rule, so no race can break it.
  The same URL in another folder is a different bookmark.
- **A folder is a path**, canonical in Unicode NFC, with no empty, `.` or `..`
  segment and no trailing `/`. The root `/` always exists and is not stored. A
  folder's parent must exist, so the tree has no orphan, and a rename rewrites the
  path and every path below it in 1 transaction.
- **A folder is deleted only when it is empty**: a folder that holds a bookmark or
  a folder is `failed_precondition`. A delete that swept its contents away would
  be 1 irreversible call that no user asked for.
- **A tag is trimmed, lower-cased, sorted and without a repeat**, 1 to 64
  characters, with no white space, comma or control character. A tag is then 1
  token, 1 word of the search, and 1 item of a list.
- **The search is FTS5 over the title, the URL, the notes and the tags**, in the
  transaction of every change, so a search never sees a stale row. Each word of
  the query is quoted as 1 FTS5 string, and the words are joined by spaces, so
  every word must match.

Each change writes its event with `outbox.Tx(tx)` in the same transaction, and
the relay publishes the outbox's rows in order and deletes 1 only after the
transport acknowledged it. 1 process runs the relay.

Folders publish no events in version 1. A consumer of the app's events follows
bookmarks: a bookmark's created, updated, deleted, moved_out and moved_in events
carry its folder, and a folder's rename publishes moved_out and moved_in for each
bookmark below it. A folder event would tell a consumer nothing that the
bookmarks do not already tell it.

The effects of the 2 deletes differ. `DeleteBookmark` is `EFFECT_OUTWARD`: it
cannot be undone, so the MCP tool carries the destructive hint and an agent sees
it before it calls. `DeleteFolder` is `EFFECT_WRITE`: an empty folder is made
again with `CreateFolder`, so the call is reversible.

## Consequences

- 1 process writes the database. A second process must call the API
  ([ADR-0004](0004-serve-the-api-over-connect-on-a-private-socket.md)).
- A migration ships with the binary and runs on the next start, so a downgrade is
  refused rather than guessed.
- The FTS table duplicates the text of each bookmark. A change writes the row
  again, which costs 1 delete and 1 insert per change.
- The search ranks by FTS5's `rank` and returns at most 200 bookmarks. A caller
  that needs more narrows the query.
- A folder path is the bookmark's folder, so a rename rewrites the bookmarks of
  its subtree. That is 1 transaction and it changes no `update_time`.
- A tag is lower-case, so `/work` cannot hold the tags `Go` and `go` at once.
- SQLite has no concurrent writers, so a long transaction holds the relay. The
  app's transactions are 1 to a few rows.

## Alternatives considered

- **A Postgres or a MySQL server:** rejected because a bookmark manager of 1 local
  user would need a server, credentials and a backup of its own.
- **`mattn/go-sqlite3`:** rejected because it needs CGO, and the binary would
  need a C library on every host.
- **A file per bookmark on the file system:** rejected because the folder tree,
  the unique URL, the search and the events would each be hand-written, and no
  transaction would cover 2 of them.
- **A folder table with ids and parent pointers:** rejected because a path is the
  selector of the permissions, the topic of the events and the CLI's argument, and
  the path is then 1 column and no join.
- **Full-text search in Go over every row:** rejected because FTS5 is in the
  database, in the transaction, and it ranks without loading every bookmark.
- **1 event per folder change:** rejected because no consumer needs it in version
  1, and a bookmark's events already carry its folder.

## References

- Shapes: [design/overview.md](../design/overview.md), [specs/api.md](../specs/api.md)
- Serving: [ADR-0004](0004-serve-the-api-over-connect-on-a-private-socket.md)
- Builds on: [ADR-0001](0001-record-architecture-decisions.md)
- hearth's events: [hearth docs/specs/events.md](https://github.com/frostyard/hearth/blob/main/docs/specs/events.md)
