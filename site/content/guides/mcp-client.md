# Connect an MCP client

`marksctl mcp` is a stdio MCP server: it serves the app's bookmark tools to an
MCP client on standard input and output. It needs no token and no port, and it
acts as the local user, who holds every capability.

The server reads the same socket as the CLI, so `frostmarksd` must be running.

## The client entry

Add 1 entry to the client's configuration:

```json
{"mcpServers": {"frostmarks": {"command": "marksctl", "args": ["mcp"]}}}
```

Any MCP client that reads this form takes the entry: the same 1 line gives an
agent the bookmark tools, and no per-agent setting decides what it reaches. An
agent's access is the grant of its caller.

Restart the client, and its tool list shows the bookmark tools:
`create_bookmark`, `get_bookmark`, `update_bookmark`, `move_bookmark`,
`delete_bookmark`, `list_bookmarks`, `search_bookmarks`, `create_folder`,
`rename_folder`, `delete_folder` and `list_folders`.

## Read 1 tool

Each tool's input schema is the proto3 JSON form of its request, and its hints
come from the RPC's effect: a read tool is marked read-only, a write tool is
marked as a change, and `delete_bookmark` is marked destructive. An agent that lists bookmarks calls `list_bookmarks` with a
folder path such as `/reading`, and the result holds only the bookmarks that
the caller's grant covers.

## Check the server by hand

The protocol is JSON on standard input, so you can ask the server for its tools
without a client:

```sh
printf '%s\n' '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"shell","version":"1"}}}' | marksctl mcp
```

It prints 1 JSON object with the server's name and its capabilities, and then it
waits for the next line.

## What the agent does not get

- No token, and no OAuth flow: the server runs on your machine, as your user.
- No access outside the socket: the daemon owns the store, and the tools call
  it.
- No event subscription: a client reads and changes bookmarks. An event reaches
  a subscriber through lodge, in a later phase.

## Next

Continue with [Folders and permissions](folders-and-permissions.md) to see what
a folder decides.
