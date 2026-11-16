# Folders and permissions

A bookmark belongs to 1 folder, and the folder is what decides who may read or
change it. This page explains the folder path, and what a grant covers.

## The folder path

A folder path starts with `/` and is a slash-separated name:

| Path | Meaning |
| --- | --- |
| `/` | the root folder, which always exists |
| `/reading` | a folder at the root, named `reading` |
| `/reading/2026` | a folder `2026` inside `reading` |

A path has no trailing `/`, no empty segment, and no `.` or `..` segment, and it
is kept in Unicode NFC. A folder is created only when its parent exists, and a
folder that holds bookmarks or other folders is not deleted until it is empty.
The root folder is neither created nor deleted.

## What the store holds

Each bookmark holds a URL, a title, the notes, a list of tags and its folder
path. A search reads the title, the URL, the notes and the tags, in 1 folder and
every folder below it, and it finds the bookmarks that hold every word of the
query, best match first. A list call reads 1 folder, with or without the folders
below it.

## What a grant covers

A caller reaches the store with a grant. A grant is a list of items, and each
item is a capability on a path:

| Part | Example | Meaning |
| --- | --- | --- |
| capability | `marks:read`, `marks:write` | what the caller may do |
| path | `/reading/**` | which folders the capability covers |

The local user's grant covers every folder with every capability, so
`marksctl` and `marksctl mcp` reach the whole store.

A call that changes a bookmark is checked against the bookmark's **stored**
folder, read inside the transaction: a caller cannot move a bookmark past a
folder it may not write, and no request field decides the check. A list or a
search call also filters each row by the grant, so a folder outside the grant
contributes no row.

## What the daemon answers

A call outside the grant fails with `permission_denied`, and the message names
the capability and the selector that the grant does not cover. Nothing is
written.

## Where a grant comes from

- The private socket: the local user, who holds every capability. The daemon
  closes every connection whose peer is another user before it reads a byte,
  and the socket has mode `0600`.
- lodge, in a later phase: lodge authenticates the caller and passes the
  caller's grant on the app's lodge socket, where the app trusts it only after
  the peer check. The app's own code reads no header, and it imports no lodge
  code.

## Next

Continue with [Commands](../reference/commands.md) to look up every option.
