package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	// The indexer writes the index through this driver. Listing space keys is the
	// only read of the file this server performs on its own.
	_ "github.com/glebarez/go-sqlite"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// distinctSpacesQuery reads the space keys an index holds. It deliberately
// duplicates one read of the indexer schema: pkg/indexerapi exposes queries, and a
// space list is not part of a query result. Move it behind the indexer API once
// that package publishes the space keys, which the tracking issue for the
// agent-facing result surface covers.
const distinctSpacesQuery = `SELECT DISTINCT space_key FROM documents WHERE space_key <> '' ORDER BY space_key`

// listSpacesTool registers confluence.list_spaces, which lets a model discover the
// spaces an index contains instead of guessing space keys for filters.
func listSpacesTool(s *server.MCPServer, defaultDBPath string) {
	tool := mcp.NewTool("confluence.list_spaces",
		mcp.WithDescription("List the Confluence space keys present in a local SQLite index"),
		mcp.WithString("dbPath",
			mcp.Description("Optional path to SQLite DB. Defaults to CONFLUENCE_INDEX_DB or confluence2md-index.db"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		effectiveDBPath := getString(request.GetArguments(), "dbPath", defaultDBPath)

		spaces, err := listSpaces(ctx, effectiveDBPath)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		payload := map[string]any{
			"schemaVersion": schemaVersion,
			"tool":          "confluence.list_spaces",
			"dbPath":        effectiveDBPath,
			"count":         len(spaces),
			"spaces":        spaces,
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	})
}

// listSpaces returns every distinct space key in the index, sorted, and reports
// what is wrong when the file is missing or holds no index.
func listSpaces(ctx context.Context, dbPath string) ([]string, error) {
	trimmed := strings.TrimSpace(dbPath)
	if trimmed == "" {
		return nil, fmt.Errorf("list_spaces requires a non-empty --db path")
	}

	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		return nil, fmt.Errorf("resolve db path %q: %w", trimmed, err)
	}

	// Refuse a missing file before opening it: sqlite would create an empty
	// database, which hides a mistyped path behind an empty space list.
	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("open index %s: %v; build one with: confluence2md-indexer index <folder>", absPath, err)
	}

	database, err := sql.Open("sqlite", filepath.ToSlash(absPath))
	if err != nil {
		return nil, fmt.Errorf("open index %s: %w", absPath, err)
	}
	defer func() { _ = database.Close() }()

	rows, err := database.QueryContext(ctx, distinctSpacesQuery)
	if err != nil {
		return nil, fmt.Errorf("read spaces from %s: %v; build an index with: confluence2md-indexer index <folder>", absPath, err)
	}
	defer func() { _ = rows.Close() }()

	spaces := []string{}
	for rows.Next() {
		var space string
		if err := rows.Scan(&space); err != nil {
			return nil, fmt.Errorf("read spaces from %s: %w", absPath, err)
		}
		spaces = append(spaces, space)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read spaces from %s: %w", absPath, err)
	}

	return spaces, nil
}
