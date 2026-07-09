# `confluence2md-mcp` - MCP Server for `confluence2md` Indexes

MCP server that exposes [confluence2md-indexer](https://github.com/gkoos/confluence2md-indexer) search to any MCP-compatible AI client. Runs as a local stdio server, queries a SQLite index built from [confluence2md](https://github.com/gkoos/confluence2md) exports, and returns ranked results with score metadata.

## Part of the `confluence2md` Platform

`confluence2md-mcp` is the third step in a three-tool local Confluence knowledge pipeline. It wraps a SQLite index built by [`confluence2md-indexer`](https://github.com/gkoos/confluence2md-indexer) (which indexes output from [`confluence2md`](https://github.com/gkoos/confluence2md)) and serves it to AI clients via MCP. See [docs/platform.md](docs/platform.md) for the full architecture.

## Requirements

- A SQLite index built by [confluence2md-indexer](https://github.com/gkoos/confluence2md-indexer)
- Source content must use [`confluence2md`](https://github.com/gkoos/confluence2md) metadata format — other formats are not supported

## Installation

Download the binary for your platform from [Releases](../../releases) and place it somewhere on your `PATH`.

### VS Code

Create or edit `.vscode/mcp.json` in your workspace:

```json
{
  "servers": {
    "confluence2md": {
      "type": "stdio",
      "command": "confluence2md-mcp",
      "args": [],
      "env": {
        "CONFLUENCE_INDEX_DB": "/path/to/confluence2md-index.db"
      }
    }
  }
}
```

> `MCP: Add Server` in the Command Palette also works.

### Claude Code

```bash
claude mcp add confluence2md \
  confluence2md-mcp \
  -e CONFLUENCE_INDEX_DB=/path/to/confluence2md-index.db
```

**WSL note:** Use the Linux binary, not the Windows `.exe` — the `.exe` does not inherit WSL environment variables. The DB path must be a native Linux path (e.g. `/home/user/confluence2md-index.db`), not `/mnt/c/`, to avoid SQLite locking issues on NTFS mounts.

### Codex CLI

Add to `~/.codex/config.json`:

```json
{
  "mcpServers": {
    "confluence2md": {
      "command": "confluence2md-mcp",
      "args": [],
      "env": {
        "CONFLUENCE_INDEX_DB": "/path/to/confluence2md-index.db"
      }
    }
  }
}
```

## Tool

### `confluence.search`

Search indexed Confluence content from a local SQLite DB.

| Argument | Required | Description |
|---|---|---|
| `query` | ✓ | Search query text |
| `dbPath` | | Override DB path (defaults to `CONFLUENCE_INDEX_DB`) |
| `mode` | | `hybrid` (default) \| `lexical` \| `vector` |
| `fusion` | | `weighted` (default) \| `rrf` |
| `alpha` | | Weighted fusion alpha `[0..1]`, default `0.70` |
| `rrfK` | | RRF k constant, default `60` |
| `topK` | | Candidates to rank, default `10` |
| `limit` | | Max results to return |
| `offset` | | Result offset |
| `candidateK` | | Candidates per retrieval channel, default `50` |
| `expand` | | Context expansion chunk count |
| `spaceKey` | | Filter by space key |
| `pageId` | | Filter by page ID |
| `fromDate` | | Lower bound `YYYY-MM-DD` |
| `toDate` | | Upper bound `YYYY-MM-DD` |

Response includes `schemaVersion`, `count`, `total`, and a `results` array with score breakdown per chunk.

## Development

### Build

```bash
# Linux / macOS / WSL
go build -o bin/confluence2md-mcp .

# Windows
go build -o bin/confluence2md-mcp.exe .

# Cross-compile Linux binary from Windows
GOOS=linux GOARCH=amd64 go build -o bin/confluence2md-mcp-linux-amd64 .
```

> If module downloads fail with `403`, set `GOPROXY=direct`.

### Test

```bash
go test ./... -run TestMCPStdioSmoke -v
```

## Troubleshooting

- **No results:** verify `CONFLUENCE_INDEX_DB` points to a built index containing the `chunks_fts` and `embeddings` tables.
- **WSL + Windows binary:** use the Linux binary with a native Linux DB path — see the WSL note above.
- **Tools not appearing in chat:** restart your MCP client after registration.


