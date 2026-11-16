---
title: frostmarks
description: "frostyard's bookmark manager: a daemon, a CLI and a stdio MCP server for bookmarks in folders, with full-text search."
hide:
  - navigation
  - toc
---

<div class="fy-hero" markdown>
<div class="fy-hero__text" markdown>
<p class="fy-eyebrow">frostmarks</p>

# frostmarks

<p class="fy-hero__lede">frostmarks keeps your bookmarks in folders on your own machine, with full-text search. 1 daemon, 1 CLI and 1 stdio MCP server serve the same 11 calls, so a person and an agent see the same bookmarks.</p>

[Install frostmarks →](getting-started/install.md){ .md-button .md-button--primary }
[Use marksctl](guides/marksctl.md){ .md-button }

</div>
<img class="fy-hero__icon" src="assets/logo.svg" alt="">
</div>

[Install](getting-started/install.md) covers the Go toolchain and the 2
commands. [Use marksctl](guides/marksctl.md) stores the first bookmark.

## What frostmarks gives you

<div class="grid cards" markdown>

-   **Bookmarks in folders**

    ---

    Each bookmark has a URL, a title, notes, tags and 1 folder. A folder path
    starts with `/`, such as `/reading/2026`, and a search reads the title,
    the URL, the notes and the tags.

-   **A daemon on a private socket**

    ---

    `frostmarksd` keeps the SQLite store and serves the app's API over Connect
    on a Unix socket that only your user reaches. No port is open, and there is
    no password to lose.

-   **A CLI that scripts**

    ---

    `marksctl` sends each call over the socket. It prints a table, or 1 JSON
    object with `-json`, so a shell script reads it without a parser.

-   **MCP tools for agents**

    ---

    `marksctl mcp` serves the same calls as MCP tools over stdio, as the local
    user and with no token. An agent reads, searches and files bookmarks with
    the calls that the CLI uses.

-   **Folders decide access**

    ---

    A grant names the capabilities and the folders it covers, and each stored
    bookmark is checked against the folder that it holds, inside the
    transaction that reads it.

-   **No lodge needed**

    ---

    The daemon, the CLI and the MCP server work on their own. A lodge app, with
    a web UI, a save page for a browser bookmarklet and agent grants, comes in
    a later phase.

</div>

## Start here

<div class="grid cards" markdown>

- <span class="fy-index">01</span> **[Install](getting-started/install.md)**

    Install `frostmarksd` and `marksctl`, and keep them on `PATH`.

- <span class="fy-index">02</span> **[Use marksctl](guides/marksctl.md)**

    Store a bookmark, search it, and move a folder.

- <span class="fy-index">03</span> **[Connect an MCP client](guides/mcp-client.md)**

    Give an agent the bookmark tools, with 1 entry in its configuration.

- <span class="fy-index">04</span> **[Folders and permissions](guides/folders-and-permissions.md)**

    Read a folder path, and see what a grant covers.

- <span class="fy-index">05</span> **[Commands](reference/commands.md)**

    Look up every option of the 2 commands.

</div>
