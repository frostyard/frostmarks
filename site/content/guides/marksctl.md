# Use marksctl

`marksctl` is the CLI. It sends each call to `frostmarksd` over the app's Unix
socket, and it prints a table, or the response as proto3 JSON with `-json`.

Start the daemon first, in the background or under your service manager:

```sh
frostmarksd
```

## Store a bookmark

Make the folder, then store the bookmark in it:

```sh
marksctl create-folder /reading
marksctl create-bookmark -url https://example.com/ -title Example -folder /reading
```

The call prints 1 line for the new bookmark: its id, its folder, its URL and its
title. Use the id to read the bookmark back:

```sh
marksctl get-bookmark ID
```

The folder must exist, and the root folder `/` always does. The same URL in the
same folder is refused, with the id of the bookmark that holds it; the same URL
in another folder is allowed.

| Option | Meaning |
| --- | --- |
| `-url` | the address to store; required, and an `http` or `https` URL |
| `-title` | the bookmark's title |
| `-notes` | the notes |
| `-tag` | 1 tag; repeat the option for each tag. The store lower-cases it, and a tag holds no white space and no comma |
| `-folder` | the folder's path, which starts with `/`; the root folder `/` when absent |

## Find a bookmark

List a folder, with everything below it:

```sh
marksctl list-bookmarks -folder /reading -recursive
```

Search the title, the URL, the notes and the tags:

```sh
marksctl search-bookmarks -folder /reading notes
```

The search reads 1 folder and every folder below it, and it finds the bookmarks
that hold every word of the query, best match first.

| Option | Meaning |
| --- | --- |
| `-folder` | the folder to read, which must exist; the root folder `/` when absent |
| `-recursive` | `list-bookmarks` only: include every folder below it |
| `-tag` | `list-bookmarks` only: read only the bookmarks with this tag |

## Change a bookmark

```sh
marksctl update-bookmark ID -title "A better title" -add-tag later
marksctl move-bookmark ID -folder /reading/2026
marksctl delete-bookmark ID
```

`update-bookmark` changes only what you name: `-url`, `-title`, `-notes`,
`-add-tag` and `-remove-tag`. `move-bookmark` takes the destination folder on
`-folder`, which is required, and the folder must exist.

## Folders

```sh
marksctl create-folder /reading/2026
marksctl rename-folder /reading/2026 /reading/archive
marksctl delete-folder /reading/2026
marksctl list-folders -recursive
```

A folder path starts with `/`, such as `/reading/2026`: each segment is a
folder, and the parent must exist. `/` is the root folder, which always exists
and is neither created nor deleted. `delete-folder` refuses a folder that holds
bookmarks or other folders.

## Read JSON in a script

Pass `-json` before the command:

```sh
marksctl -json list-bookmarks -folder /reading
```

The output is 1 indented object of proto3 JSON, so `jq` reads it without a
table parser.

## Exit status

| Code | Meaning |
| --- | --- |
| 0 | the call succeeded |
| 1 | the daemon refused the call, or an error came back |
| 2 | the command line is wrong: an unknown flag, or a missing argument |

`marksctl help` prints the usage of every command.

## Next

Continue with [Connect an MCP client](mcp-client.md), so that an agent reads
and files the same bookmarks.
