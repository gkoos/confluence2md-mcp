package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gkoos/confluence2md-indexer/pkg/indexerapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// listSpacesTool registers confluence.list_spaces, which lets a model discover the
// spaces an index contains instead of guessing space keys for filters.
func listSpacesTool(s *server.MCPServer, name string) {
	tool := mcp.NewTool(name,
		mcp.WithDescription("List the Confluence space keys present in a local SQLite index"),
		mcp.WithString("dbPath",
			mcp.Description("Optional path to SQLite DB. Defaults to CONFLUENCE_INDEX_DB, then the configuration file, then confluence2md-index.db"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		effectiveDBPath, err := dbPathFor(request.GetArguments())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		spaces, err := listSpaces(ctx, effectiveDBPath)
		if err != nil {
			return mcp.NewToolResultError(withBusyGuidance(err.Error(), err)), nil
		}

		payload := map[string]any{
			"schemaVersion": schemaVersion,
			"tool":          name,
			"dbPath":        effectiveDBPath,
			"count":         len(spaces),
			"spaces":        spaces,
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	})
}

// listSpaces returns every distinct space key in the index, sorted. Sources without
// spaces, such as a folder of plain files, contribute none.
func listSpaces(ctx context.Context, dbPath string) ([]string, error) {
	trimmed := strings.TrimSpace(dbPath)
	if trimmed == "" {
		return nil, fmt.Errorf("list_spaces requires a non-empty --db path")
	}

	absPath, err := filepath.Abs(trimmed)
	if err != nil {
		return nil, fmt.Errorf("resolve db path %q: %w", trimmed, err)
	}

	// Refuse a missing file before opening it: sqlite would create an empty database,
	// which hides a mistyped path behind an empty space list.
	if _, err := os.Stat(absPath); err != nil {
		return nil, fmt.Errorf("open index %s: %v; build one with: confluence2md-indexer index <folder>", absPath, err)
	}

	spaces, err := indexerapi.Spaces(ctx, absPath)
	if err != nil {
		return nil, fmt.Errorf("read spaces from %s: %w", absPath, err)
	}

	return spaces, nil
}
