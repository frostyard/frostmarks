# Import your bookmarks

`marksctl import` reads the bookmark file that a browser exports, and it adds
each bookmark to frostmarks, in the same folders. It reads the export of
Firefox, Chrome, Edge, Safari and Pinboard: they all write the same HTML form,
the Netscape bookmark file.

## Export the file

| From | Where |
| --- | --- |
| Firefox | Bookmarks > Manage bookmarks > Import and Backup > Export Bookmarks to HTML |
| Chrome | Bookmarks and lists > Bookmark manager > the menu > Export bookmarks |
| Edge | Favorites > the menu > Export favorites |
| Safari | File > Export > Bookmarks |
| Pinboard | Settings > Backup > the HTML export |

Each one writes 1 `.html` file, such as `bookmarks.html`.

## Look before you import

Start the daemon, then ask for a dry run:

```sh
marksctl import bookmarks.html -folder /imported -dry-run
```

A dry run reads the file and the folder, prints what the import would do, and
writes nothing. Each line names 1 item:

```text
folder	/imported/Bookmarks bar
add	/imported/Bookmarks bar	https://example.com/	Example
skip	/imported/Bookmarks bar	https://example.com/	Example again
refuse	/imported	javascript:alert(1)	the URL "javascript:alert(1)" must start with http:// or https://
dry run: folders made: 1, bookmarks added: 1, skipped as duplicates: 1, refused: 1
```

## Import

```sh
marksctl import bookmarks.html -folder /imported
```

The import makes the folder `/imported` when it is absent, then the file's
folders below it, then adds each bookmark with its title, its tags and its
description as notes. Without `-folder`, the file's folders go to the root
folder `/`.

Run it again at any time: a bookmark whose address is already in its folder is
skipped, so a second import of the same file adds nothing.

| Option | Meaning |
| --- | --- |
| `-folder` | the folder to import into; the root folder `/` when absent |
| `-dry-run` | print the plan, and write nothing |
| `-json` | print the result as 1 JSON object, for a script |

## What the import keeps

- **Folders.** Each folder of the file becomes a folder, with the same name. A
  `/` in a name becomes `-`, because `/` separates the folders of a path.
- **Titles, tags and notes.** The tags come from the file's `TAGS` (Firefox and
  Pinboard), lower-cased, with a `-` for each space. The description of a
  Pinboard or Firefox bookmark becomes its notes.
- **The date is not kept.** frostmarks records when it stored each bookmark, so
  an imported bookmark carries the date of the import.

## What the import refuses

frostmarks stores only `http` and `https` addresses. The import refuses a
bookmarklet (`javascript:`), a `data:` or `file:` address, and Firefox's own
`place:` entries, such as "Most Visited", and it names each one with its
reason. The rest of the file still comes in.

The file can be at most 32 MiB. A browser's export with thousands of bookmarks
and their icons stays well below that.

## Exit status

| Code | Meaning |
| --- | --- |
| 0 | the import ran; the counts say what it refused |
| 1 | the file cannot be read, is too large or is no bookmark export, or the daemon failed |
| 2 | the command line is wrong: no file, or an unknown flag |

When the daemon fails part of the way, what the import wrote stays: run the
same import again, and it adds only the rest.

## Next

See [Use marksctl](marksctl.md) to find and change what you imported, and
[Commands](../reference/commands.md) for every option.
