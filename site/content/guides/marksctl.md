# Use marksctl

`marksctl` is the CLI. It sends each call to `frostmarksd` over the app's Unix
socket, and it prints 1 line for each bookmark or folder, or the response as
JSON with `-json`.

Start the daemon first, in the background or under your service manager:

```sh
frostmarksd &
```

## Make a folder

A bookmark goes in a folder that exists. The root folder `/` always exists;
make the others with `create-folder`:

```sh
marksctl create-folder /reading
```

## Store a bookmark

```sh
marksctl create-bookmark -url https://example.com/ -title Example -tag Docs -folder /reading
```

The call prints the new bookmark as 1 line: its id, its folder, its URL and its
title, separated by tabs. Use the id to read the bookmark back:

```sh
marksctl get-bookmark ID
```

| Option | Meaning |
| --- | --- |
| `-url` | the `http` or `https` address to store; required |
| `-title` | the bookmark's title |
| `-notes` | the notes |
| `-tag` | 1 tag; repeat the option for each tag |
| `-folder` | the folder's path; the root folder `/` when absent |

frostmarks lower-cases the URL's scheme and host, and drops its fragment (the
part after `#`). The same URL in the same folder is refused with
`already_exists`, and the message names the bookmark that holds it. Tags are
lower-cased, 1 to 64 characters, with no space and no comma.

## Find a bookmark

List a folder, with every folder below it:

```sh
marksctl list-bookmarks -folder /reading -recursive
```

Search the title, the URL, the notes and the tags. A bookmark matches when it
holds every word:

```sh
marksctl search-bookmarks example docs
```

| Option | Meaning |
| --- | --- |
| `-folder` | read only this folder (for a search: and every folder below it) |
| `-recursive` | `list-bookmarks` only: include the folders below it |
| `-tag` | `list-bookmarks` only: keep only the bookmarks with this tag |

## Change a bookmark

```sh
marksctl update-bookmark ID -title "A better title" -add-tag later
marksctl create-folder /reading/2026
marksctl move-bookmark ID -folder /reading/2026
marksctl delete-bookmark ID
```

`update-bookmark` changes only what you name: `-url`, `-title`, `-notes`,
`-add-tag` and `-remove-tag`. `move-bookmark` takes the destination folder on
`-folder`, and the folder must exist. A deleted bookmark cannot be restored.

## Folders

```sh
marksctl create-folder /reading/2026
marksctl rename-folder /reading/2026 /reading/archive
marksctl list-folders -folder /reading -recursive
marksctl delete-folder /reading/archive
```

A folder path starts with `/`, and each segment after it is 1 folder, such as
`/reading/2026`. A folder is created only when its parent exists.
`rename-folder` moves the folder with every folder and bookmark below it.
`delete-folder` refuses a folder that holds a bookmark or another folder, and
it never deletes the root folder.

## Read JSON in a script

Pass `-json` before the command:

```sh
marksctl -json list-bookmarks -folder /reading
```

The output is the response in the proto3 JSON form, so `jq` reads it.

## Exit status

| Code | Meaning |
| --- | --- |
| 0 | the call succeeded |
| 1 | the daemon refused the call, or an error came back; the error is on standard error |
| 2 | the command line is wrong: an unknown command or flag, or a missing argument |

## Next

Continue with [Connect an MCP client](mcp-client.md), so that an agent reads
and files the same bookmarks.
