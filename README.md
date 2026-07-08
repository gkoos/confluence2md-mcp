# `confluence2md-mcp` - MCP Server for `confluence2md` Indexes

[![CI](https://github.com/gkoos/confluence2md-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/gkoos/confluence2md-mcp/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/gkoos/confluence2md-mcp)](https://github.com/gkoos/confluence2md-mcp/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://github.com/gkoos/confluence2md-mcp/blob/main/LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/gkoos/confluence2md-mcp)](https://github.com/gkoos/confluence2md-mcp/blob/main/go.mod)

MCP server that exposes [confluence2md-indexer](https://github.com/gkoos/confluence2md-indexer) search to any MCP-compatible AI client. Runs as a local stdio server, queries a SQLite index built from [confluence2md](https://github.com/gkoos/confluence2md) exports, and returns ranked results with score metadata.

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


