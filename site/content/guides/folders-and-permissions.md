# Folders and permissions

A bookmark belongs to 1 folder, and the folder is what decides who may read or
change it. This page explains the folder path, and what a grant covers.

## The folder path

A folder path starts with `/`, and each segment after it is 1 folder:

| Path | Meaning |
| --- | --- |
| `/` | the root folder, which always exists |
| `/reading` | a folder `reading` at the root |
| `/reading/2026` | a folder `2026` inside `/reading` |

A path has no trailing slash, no empty segment and no `.` or `..` segment.
frostmarks compares paths in their Unicode NFC form. A folder is created only
when its parent exists, and a folder that holds bookmarks or other folders is
not deleted until it is empty.

## What the store holds

Each bookmark holds a URL, a title, the notes, a list of tags and its folder
path. A search reads the title, the URL, the notes and the tags, in 1 folder
and every folder below it. A list call reads 1 folder, with or without the
folders below it.

## What a grant covers

A caller reaches the store with a grant. A grant is a list of items, and each
item is a capability on a folder path:

| Part | Example | Meaning |
| --- | --- | --- |
| capability | `marks:read`, `marks:write` | what the caller may do |
| folder | `/reading` | the folder and every folder below it |

An item on `/reading` covers `/reading` and `/reading/2026`, and never
`/readings`. An item on `/` covers every folder.

The local user's grant covers every folder with every capability, so
`marksctl` and `marksctl mcp` reach the whole store.

A call on 1 stored bookmark (`get`, `update`, `move`, `delete`) is checked
against the bookmark's **stored** folder, read inside the call's transaction:
no request field decides the check. A move needs `marks:write` on the folder
the bookmark leaves and on the folder it moves to, and a folder rename on the
old path and on the new path. A list or a search names a folder that the grant
covers (an absent folder is the root `/`), and it also filters each row by the
grant.

## What the daemon answers

A call outside the grant fails with `permission_denied`, and the message names
the capability and the folder that the grant does not cover, for example
`permission denied: needs marks:write on folder=/home`. Nothing is written.

## Where a grant comes from

- The private socket: the local user, who holds every capability. Only your
  user reaches it.
- lodge, in a later phase: lodge authenticates the caller and passes the
  caller's grant on the app's lodge socket, where the app trusts it only after
  the peer check. The app imports no lodge code.

## Next

Continue with [Commands](../reference/commands.md) to look up every option.
