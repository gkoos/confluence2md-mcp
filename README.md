# `confluence2md-mcp` - MCP Server for `confluence2md` Indexes

[![CI](https://github.com/gkoos/confluence2md-mcp/actions/workflows/ci.yml/badge.svg)](https://github.com/gkoos/confluence2md-mcp/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/gkoos/confluence2md-mcp)](https://github.com/gkoos/confluence2md-mcp/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://github.com/gkoos/confluence2md-mcp/blob/main/LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/gkoos/confluence2md-mcp)](https://github.com/gkoos/confluence2md-mcp/blob/main/go.mod)

MCP server that exposes [confluence2md-indexer](https://github.com/gkoos/confluence2md-indexer) search to any MCP-compatible AI client. Runs as a local stdio server, queries a SQLite index built from [confluence2md](https://github.com/gkoos/confluence2md) exports, and returns ranked results with score metadata.

## Part of the `confluence2md` Platform

`confluence2md-mcp` is the third step in a three-tool local Confluence knowledge pipeline. It wraps a SQLite index built by [`confluence2md-indexer`](https://github.com/gkoos/confluence2md-indexer) (which indexes output from [`confluence2md`](https://github.com/gkoos/confluence2md)) and serves it to AI clients via MCP. See [docs/platform.md](docs/platform.md) for the full architecture.

## Requirements

- A SQLite index at schema version 1, written by [confluence2md-indexer](https://github.com/gkoos/confluence2md-indexer) **v0.5.0 or newer**. This server links v0.9.0; an index from an older release is refused with a message naming the rebuild command.
- Source content must use [`confluence2md`](https://github.com/gkoos/confluence2md) metadata format — other formats are not supported

## Environment Variables

| Variable | Required | Description |
|---|---|---|
| `CONFLUENCE_INDEX_DB` | recommended | Path to the SQLite DB file. Falls back to `confluence2md-index.db` in the current working directory if unset. |
| `CONFLUENCE2MD_EMBEDDING_PROVIDER` | optional | `bow-local` (default: local, offline, no API key), `openai`, or `openai-compatible`. |
| `CONFLUENCE2MD_EMBEDDING_MODEL` | optional | Model id, for example `text-embedding-3-small`. |
| `CONFLUENCE2MD_EMBEDDING_DIM` | optional | Vector dimension such as `1024`. Part of the embedding identity. |
| `CONFLUENCE2MD_EMBEDDING_BASE_URL` | optional | Endpoint for an `openai-compatible` provider. |
| `CONFLUENCE2MD_EMBEDDING_API_KEY_ENV` | optional | Name of the variable that holds the API key (preferred over a literal key). |
| `CONFLUENCE2MD_EMBEDDING_API_KEY` | optional | Literal API key. |
| `CONFLUENCE2MD_EMBEDDING_SKIP` | optional | `true` disables the vector channel and leaves lexical search. |
| `CONFLUENCE2MD_EMBEDDING_CONTEXT` | optional | Document context header the index was built with, for example `title,space`; it must match the index or the query fails with an identity mismatch. |

The remaining `CONFLUENCE2MD_EMBEDDING_*` variables of the indexer are honoured too — `AUTH_HEADER`, `AUTH_SCHEME`, `HEADERS`, `QUERY_PARAMS`, `DOCUMENT_PREFIX`, `QUERY_PREFIX`, `BATCH_SIZE`, `TIMEOUT`, `MAX_RETRIES` — see the indexer's `docs/embedding-providers.md`.

> **The index and the query must agree on the embedding configuration.** An index records the identity of the vectors it holds (`provider:variant@dimension`, for example `bow-local:fnv1a@256`), while a hybrid or vector query resolves its own identity from these variables. When the two differ, the query fails with `embedding mismatch: ...` instead of returning weak results; the tool error names both identities and how to fix it. Use `mode: "lexical"` to search without embeddings at all.

> This server reads environment variables only. The indexer CLI additionally accepts a `config.yaml`; if you indexed through a configuration file, export the equivalent `CONFLUENCE2MD_EMBEDDING_*` variables here so that both sides resolve the same provider.

## Upgrading an Index

This server links `confluence2md-indexer` v0.9.0, and every indexer release since v0.5.0 writes the same schema version 1 — the one that introduced the document metadata columns. An index built by any of those releases keeps working without a rebuild. A database written by an older indexer is refused with a message naming the rebuild command:

```bash
confluence2md-indexer index ./output --rebuild
```

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

## Tools

### `confluence.search`

Search indexed Confluence content from a local SQLite DB.

| Argument | Required | Description |
|---|---|---|
| `query` | ✓ | Search query text |
| `dbPath` | | Override DB path. Falls back to `CONFLUENCE_INDEX_DB` env var, then to `confluence2md-index.db` in the current working directory. |
| `mode` | | `hybrid` (default) \| `lexical` \| `vector` |
| `fusion` | | `weighted` (default) \| `rrf` |
| `alpha` | | Weighted fusion alpha `[0..1]`, default `0.70` |
| `rrfK` | | RRF k constant, default `60` |
| `topK` | | How many ranked results to consider; also the default for `limit`. Default `10` |
| `limit` | | Max results to return. Defaults to `topK` (`10`); `0` inherits `topK` — it does not mean unlimited (that is `maxPerPage`) |
| `offset` | | Result offset |
| `candidateK` | | Candidates per retrieval channel, default `50` |
| `expand` | | Context expansion chunk count |
| `group` | | `document` returns one result per page (page score, `chunkCount`); default chunk-level |
| `maxPerPage` | | Cap chunks per document in the results; `0` (default) means unlimited |
| `spaceKey` | | Filter by space key |
| `pageId` | | Filter by page ID |
| `fromDate` | | Lower bound `YYYY-MM-DD` |
| `toDate` | | Upper bound `YYYY-MM-DD` |
| `spaces` | | Filter by any of several space keys; wins over `spaceKey` |
| `host` | | Filter by crawled site host |
| `author` | | Creator or last modifier name, case-insensitive |
| `createdBy` | | Creator name only |
| `modifiedBy` | | Last modifier name only |
| `depthMin` / `depthMax` | | Crawl depth bounds; `depthMin: 1` excludes seed pages |
| `seedOnly` | | Only the pages the crawl started from |
| `hasAttachments` | | Only pages that carry at least one attachment |
| `updatedSince` | | Modified within an age (`30d`, `2w`, `12h`) or after an absolute date (`2026-01-01`) |
| `facets` | | Count documents per bucket over the whole matching set, before `topK`: `space`, `author` |
| `embeddingProvider` | | Override the provider for this call: `bow-local` \| `openai` \| `openai-compatible` |
| `embeddingModel` | | Override the embedding model |
| `embeddingDim` | | Override the embedding dimension |
| `embeddingBaseURL` | | Override the embedding endpoint |
| `embeddingApiKeyEnv` | | Name of the environment variable that holds the API key |
| `embeddingAuthHeader` / `embeddingAuthScheme` | | Override the authentication header and scheme |
| `embeddingSkip` | | Disable the vector channel for this call |
| `embeddingDocumentPrefix` / `embeddingQueryPrefix` | | Text prefixes for asymmetric models |
| `embeddingContext` | | Document context header the index was built with: `title`, `space`, `section`. An index built with one only matches when the query names the same selection |

Metadata filters apply to lexical, vector and hybrid retrieval alike. Arguments win over environment variables: the indexer fills only the embedding fields that the arguments leave unset, so `embeddingModel` overrides `CONFLUENCE2MD_EMBEDDING_MODEL` for that call. A literal API key is deliberately **not** an argument — it would travel through the client conversation and the server log — so set `CONFLUENCE2MD_EMBEDDING_API_KEY` or name another variable with `embeddingApiKeyEnv`.

Response fields:

| Field | Description |
|---|---|
| `schemaVersion` | Schema version string for contract stability |
| `tool` | Always `"confluence.search"` |
| `dbPath` | Resolved DB path used for the query |
| `request` | Echoed request parameters |
| `count` | Number of results returned in this response |
| `total` | Results available after ranking — `topK` caps it — and before `offset`/`limit`; in `group: "document"` mode it counts documents. It is not the number of matching documents when `topK` truncates: read `facets` for that |
| `results` | Array of result objects with chunk text and score breakdown |
| `facets` | Present only when `facets` was requested: `{ "space": {...}, "author": {...} }` with document counts |

Result objects carry `canonicalUrl` when the crawler recorded one, and `chunkCount` in `group: "document"` mode.

Failures are returned as tool errors that keep the indexer's message and add the fix when the cause is actionable: an embedding mismatch names both identities, a missing or vector-less index names the rebuild command, and a disabled vector channel points at `CONFLUENCE2MD_EMBEDDING_SKIP`. `mode: "lexical"` works without any embedding configuration.

### `confluence.list_spaces`

List the Confluence space keys the index contains, so a client can scope a search without knowing the keys in advance.

| Argument | Required | Description |
|---|---|---|
| `dbPath` | | Override DB path. Falls back to `CONFLUENCE_INDEX_DB`, then to `confluence2md-index.db` in the current working directory. |

Response fields: `schemaVersion`, `tool`, `dbPath`, `count`, and `spaces` — a sorted array of space key strings, empty when the index holds no spaces.

### `confluence.stats`

Report what an index holds, the crawl it was built from and the embedding identity behind its vectors. This is the tool that turns a failed or empty search into an answer: the index is empty, it is older than the crawl, or it was built with another provider.

| Argument | Required | Description |
|---|---|---|
| `dbPath` | | Override DB path. Falls back to `CONFLUENCE_INDEX_DB`, then to `confluence2md-index.db` in the current working directory. |

Response fields: `schemaVersion`, `tool`, `dbPath`, and `stats`. The `stats` object is the indexer's `stats --json` object unchanged — only the envelope is this server's (`tool` here, `command` there) — and it carries:

- `stats.documents`, `stats.chunks`, `stats.embeddings`, `stats.runs`
- `stats.vectorReady`, `stats.vectorName` (the stored embedding identity), `stats.vectorCapability`
- `stats.embeddingModels` — one entry per identity present: `name`, `dimension`, `chunks`
- `stats.metadata` — coverage: `documents`, `withAuthors`, `withLinks`, `withAttachments`, `withComments`, `seeds`, `nested`, `hosts`
- `stats.corpus` — the crawl the index was built from: `runId`, `crawlStartedAt`, `crawlCompletedAt`, `crawlSucceededAt`, `crawlMode`, `seedCount`, `pageCount`, `indexedAt`. A `crawlCompletedAt` later than `indexedAt` means the indexing run read a moving corpus; the indexer's `--fail-if-stale` is the CLI equivalent.

`stats.corpus` is omitted when the index holds no crawl snapshot, which is what a content folder indexed without a `metadata.json` produces; `stats.metadata` is always present. `crawlStartedAt`, `crawlSucceededAt`, `crawlMode` and `seedCount` stay empty when the corpus only carried a completion timestamp.

### `confluence.facets`

Count the documents a query matches per space and author, without returning them. It takes the same arguments as `confluence.search` and defaults `facets` to `["space", "author"]`.

Response fields: `schemaVersion`, `tool`, `dbPath`, `request`, `total` (matching documents), and `facets`; there are no result rows. The tool groups by page internally, so `total` counts documents even when a page has several matching chunks, and counts describe the whole matching set before `topK`.

## Query semantics

The tool schemas cover the argument names and types; this section covers what the arguments do once they reach the indexer.

### Defaults and how they interact

The server sends the ranking arguments explicitly rather than leaving them empty, so a client sees the same numbers the CLI's defaults produce: `mode` `hybrid`, `fusion` `weighted`, `alpha` `0.70`, `rrfK` `60`, `topK` `10`, `candidateK` `50`.

Three of them bound each other:

- `candidateK` is how many candidates each retrieval channel fetches before fusion.
- `topK` truncates the fused ranking, so it caps everything downstream. Asking for more than `topK` rows cannot return more rows, however large `limit` is.
- `limit` pages through that truncated set, starting at `offset`. `limit: 0` inherits `topK`; it does not mean unlimited.

`total` therefore reports the size of the truncated set, not the number of matching documents. A query with two matching pages and `topK: 1` answers `total: 1` while the facets count both.

### What is rejected, and what is silently defaulted

Returned as a tool error: `group` set to anything but `document`; a negative `maxPerPage`, `depthMin`, `depthMax` or `embeddingDim`; `depthMin` greater than `depthMax`; an `updatedSince` that is neither an age (`30d`, `2w`, `12h`) nor a date; an unknown facet dimension (`unknown facet "x" (available: space, author)`).

Not rejected: a value of the wrong JSON type falls back to the default. `limit: "ten"` behaves like `limit: 0` (ten results), `alpha: "high"` like `0.70`, and `seedOnly: "yes"` like `false`. Send numbers as numbers and booleans as booleans; the server cannot tell a typo from an intentional default.

A query that is empty or only whitespace returns an empty result set, not an error; a missing `query` (or a non-string one) is rejected. Whitespace is trimmed before matching.

### `expand`

`expand: N` widens each result to a window of up to `N` chunks on either side of the match. `chunkText` becomes that window, `baseChunkText` keeps the matched chunk, and `contextStartIndex`, `contextEndIndex` and `contextChunkCount` describe the window (a start index of `0` is omitted, like every other zero-valued optional field).

### `group: "document"`

One result per page, carrying the page's best-scoring chunk: `fusedScore` is that chunk's score, `chunkIndex` is its position, and `chunkCount` is how many of the page's chunks matched. `total` counts documents. `maxPerPage` has no effect here — there is already one row per page.

### Facet counting

Counts are document counts over the full matching set, taken before `topK`, so they describe the corpus rather than the page of results. Rules worth knowing:

- A page counts once per dimension however many of its chunks matched.
- `author` counts a page for its creator *and* for its last modifier, so the buckets can sum to more than `total`. That matches the `author` filter, which also matches either name.
- A page whose metadata has no value for the dimension adds no bucket, so buckets can sum to less than `total`.
- Only the dimensions asked for are counted; the `confluence.facets` tool defaults to `space` and `author`.

## Response reference

Every tool returns one JSON document as its text content. The envelope below is this server's; `results` and `facets` are the indexer's payloads, unchanged.

### Result objects

Each `results[]` entry is the indexer's `Result` object: see [Output Reference → *JSON Output (`query --json`)*](https://github.com/gkoos/confluence2md-indexer/blob/main/docs/output-reference.md#json-output-query---json) for the field list and how the scores are computed. Which fields appear:

| Field | When |
|---|---|
| `rank`, `chunkId`, `documentId`, `pageId`, `title`, `localPath`, `spaceKey`, `sourceUrl`, `chunkText`, `chunkIndex`, `lexicalScore`, `vectorScore`, `fusedScore`, `fusion` | always |
| `canonicalUrl` | when the crawler recorded one; it is the link to open, never reconstructed from `sourceUrl` |
| `baseChunkText`, `contextStartIndex`, `contextEndIndex`, `contextChunkCount` | when `expand` widened the result |
| `chunkCount` | in `group: "document"` mode |
| `metadataBoost`, `metadataFactors` | reserved for ranking priors, which this server does not expose yet |

An empty result set is `[]`, never `null`, so a client can iterate it without a nil check. The `facets` block appears in a search payload only when the call asked for facets.

### The `request` echo

`request` shows what the server sent to the indexer, not the arguments as you typed them:

- Keys are the indexer's Go field names — `Text`, `Mode`, `RRFK`, `TopK`, `Limit`, `Filters.SpaceKey` — not the MCP argument names.
- Embedding overrides are never echoed; the indexer tags them out of its request contract. Priors are excluded the same way, so they will not appear once they are exposed.
- `Filters` appears in full with every filter key, empty when unused, and includes `Candidate`, the internal candidate limit.
- Values are as sent: `Limit` reads `0` even though the effective page size is `topK`.
- `Group`, `MaxPerPage` and `Facets` are omitted when unset, so a default call's echo has no trace of them.

### Sample payloads

A chunk-level search with default arguments against a three-page fixture index:

```json
{
  "schemaVersion": "1",
  "tool": "confluence.search",
  "dbPath": "/path/to/confluence2md-index.db",
  "count": 2,
  "total": 2,
  "request": {
    "Text": "apple",
    "Mode": "hybrid",
    "Fusion": "weighted",
    "Alpha": 0.7,
    "RRFK": 60,
    "TopK": 10,
    "Offset": 0,
    "Limit": 0,
    "CandidateK": 50,
    "Expand": 0,
    "Filters": {
      "SpaceKey": "",
      "PageID": "",
      "FromDate": "",
      "ToDate": "",
      "Candidate": 0
    }
  },
  "results": [
    {
      "rank": 1,
      "chunkId": "2001:000000",
      "documentId": "2001",
      "pageId": "2001",
      "title": "Apple Runbook",
      "localPath": "apple.md",
      "spaceKey": "ENG",
      "sourceUrl": "https://example.test/pages/2001",
      "canonicalUrl": "https://example.test/pages/2001-canonical",
      "chunkText": "# Apple Runbook\n\nThe apple rotation happens on Mondays.",
      "chunkIndex": 0,
      "lexicalScore": 1,
      "vectorScore": 1,
      "fusedScore": 1,
      "fusion": "weighted"
    },
    {
      "rank": 2,
      "chunkId": "2002:000000",
      "documentId": "2002",
      "pageId": "2002",
      "title": "On-call Handbook",
      "localPath": "oncall.md",
      "spaceKey": "OPS",
      "sourceUrl": "https://example.test/pages/2002",
      "chunkText": "# On-call Handbook\n\nEscalate the apple incident to the ops channel.",
      "chunkIndex": 0,
      "lexicalScore": 0,
      "vectorScore": 0,
      "fusedScore": 0,
      "fusion": "weighted"
    }
  ]
}
```

The same index with `group: "document"` and `query: "deployment"`: two chunks of one page matched, so the page appears once and `total` counts the page.

```json
{
  "count": 1,
  "total": 1,
  "results": [
    {
      "rank": 1,
      "chunkId": "2003:000000",
      "documentId": "2003",
      "pageId": "2003",
      "title": "Deployment Runbook",
      "localPath": "deploy.md",
      "spaceKey": "ENG",
      "sourceUrl": "https://example.test/pages/2003",
      "chunkText": "# Deployment\n\nIntro text.",
      "chunkIndex": 0,
      "lexicalScore": 1,
      "vectorScore": 1,
      "fusedScore": 1,
      "fusion": "weighted",
      "chunkCount": 2
    }
  ]
}
```

(Abridged: `schemaVersion`, `tool`, `dbPath` and `request` are as in the sample above, with `"Group": "document"` added to the echo.)

With `expand: 2`, the matched chunk arrives inside its window — `chunkText` is the window, `baseChunkText` is what actually matched:

```json
{
  "title": "Deployment Runbook",
  "chunkText": "# Deployment\n\nIntro text.\n\n## Rollback\n\nSteps to undo the change.",
  "baseChunkText": "## Rollback\n\nSteps to undo the change.",
  "chunkIndex": 1,
  "contextEndIndex": 1,
  "contextChunkCount": 2
}
```

`confluence.facets` for the same query, defaulting to both dimensions. `total` counts documents and each bucket is a document count:

```json
{
  "schemaVersion": "1",
  "tool": "confluence.facets",
  "dbPath": "/path/to/confluence2md-index.db",
  "total": 2,
  "facets": {
    "space": { "ENG": 1, "OPS": 1 },
    "author": { "Ada Lovelace": 2, "Grace Hopper": 1 }
  },
  "request": {
    "Text": "apple",
    "Mode": "hybrid",
    "Group": "document",
    "Facets": ["space", "author"]
  }
}
```

(Abridged: `request` shows only the fields that matter for this call — the rest match the first sample.)

`confluence.stats` on the same index. The `stats` object is the indexer's, complete:

```json
{
  "schemaVersion": "1",
  "tool": "confluence.stats",
  "dbPath": "/path/to/confluence2md-index.db",
  "stats": {
    "runs": 1,
    "documents": 3,
    "chunks": 4,
    "embeddings": 4,
    "vectorReady": true,
    "vectorName": "bow-local:fnv1a@256",
    "vectorCapability": "lexical",
    "embeddingModels": [
      { "name": "bow-local:fnv1a@256", "dimension": 256, "chunks": 4 }
    ],
    "metadata": {
      "documents": 3,
      "withAuthors": 3,
      "withLinks": 0,
      "withAttachments": 0,
      "withComments": 0,
      "seeds": 0,
      "nested": 0,
      "hosts": 0
    },
    "corpus": {
      "runId": "21566c45e992fe22",
      "crawlStartedAt": "",
      "crawlCompletedAt": "2026-02-01T00:00:00Z",
      "crawlSucceededAt": "",
      "crawlMode": "",
      "seedCount": 0,
      "pageCount": 3,
      "indexedAt": "2026-10-21T21:20:43.8201126Z"
    }
  }
}
```

`confluence.list_spaces`:

```json
{
  "schemaVersion": "1",
  "tool": "confluence.list_spaces",
  "dbPath": "/path/to/confluence2md-index.db",
  "count": 2,
  "spaces": ["ENG", "OPS"]
}
```

## Known limitations

- **Chunk-level text.** A result carries a chunk, or a window of chunks with `expand`; no tool returns a whole page. `group: "document"` plus `expand` is the closest equivalent.
- **Attachments and comments are metadata only.** The index stores their counts, so a search can filter on them but not read them.
- **No ranking priors yet.** Recency, authority, seed, depth and richness weighting exist in the indexer but are not exposed as arguments; see gkoos/confluence2md-indexer#59.
- **No index build or refresh.** The server only reads. Building is `confluence2md-indexer index <folder>`, and `stats.corpus.crawlCompletedAt` is how you notice the index fell behind the crawl.
- **stdio only.** There is no HTTP or SSE transport.
- **`confluence.list_spaces` is the one direct read.** It queries `documents.space_key` itself because `pkg/indexerapi` publishes no space list, and it returns keys only — no titles, counts or hierarchy.
- **The caller owns the trust boundary.** `dbPath` opens any SQLite file the process can read, and `embeddingBaseURL` with `embeddingApiKeyEnv` can send a named environment variable's value to any host. Where the indexed content is untrusted, treat those two as privileged.

## Development

### Build

```bash
# Linux / macOS / WSL
go build -o bin/confluence2md-mcp .

# Windows
go build -o bin/confluence2md-mcp.exe .

# Cross-compile a Linux binary (PowerShell)
$env:GOOS = "linux"; $env:GOARCH = "amd64"
go build -o bin/confluence2md-mcp-linux-amd64 .

# The same in bash, WSL or macOS
GOOS=linux GOARCH=amd64 go build -o bin/confluence2md-mcp-linux-amd64 .
```

> If module downloads fail with `403`, set `GOPROXY=direct`.

### Test

```bash
go test ./... -v
```

The suite builds the server and drives it over stdio. Four offline end-to-end tests install the pinned indexer (`go install ...@v0.9.0`), build an index from a temporary corpus with the default `bow-local` provider, and check:

- the tool list, `confluence.list_spaces`, and a search narrowed by space;
- result shaping: `group: "document"` with `chunkCount`, `maxPerPage`, and `canonicalUrl`;
- facet counts attached to a search payload;
- `confluence.stats`: size, stored embedding identity, metadata coverage and the crawl timestamp; then `confluence.facets`, including the document total for a page with several matching chunks.

One of the four covers the actionable failure — an index built with a different embedding identity — and a separate smoke test starts the server with no index at all to check the tool list and the missing-database error. Nothing needs an API key or a running service, and the offline tests skip only when the indexer CLI cannot be installed.

### Version

Release builds stamp the binary through `ldflags`; the version is reported in the MCP `initialize` response and written to the startup log. An unstamped `go build` reports `dev`.

## Troubleshooting

- **No results:** verify `CONFLUENCE_INDEX_DB` points to a built index containing the `chunks_fts` and `embeddings` tables.
- **No results although `stats` counts documents:** a filter is excluding everything — `spaceKey` is an exact match (read the keys with `confluence.list_spaces`), `updatedSince` is relative to now, and `depthMin: 1` drops seed pages. Also compare `stats.corpus.crawlCompletedAt` with `stats.corpus.indexedAt`: a later timestamp means the indexing run read a moving corpus, so the page may have moved.
- **`facets` missing from a search payload:** only `confluence.facets` always returns the block. A search omits it unless the call passes `facets`.
- **`total` smaller than the facet buckets:** `topK` truncated the ranked set. Raise `topK` for more rows, or read the buckets for the match counts.
- **A filter or limit had no effect:** an argument sent with the wrong JSON type falls back to its default — see [Query semantics](#what-is-rejected-and-what-is-silently-defaulted).
- **WSL + Windows binary:** use the Linux binary with a native Linux DB path — see the WSL note above.
- **Tools not appearing in chat:** restart your MCP client after registration.


