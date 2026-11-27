package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gkoos/confluence2md-indexer/pkg/indexerapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// listSourcesTool registers confluence.list_sources, which names the sources an index
// holds with their document and chunk counts, so a model can pick ids for the sources
// filter instead of guessing them.
func listSourcesTool(s *server.MCPServer, name string) {
	tool := mcp.NewTool(name,
		mcp.WithDescription("List the sources in a local SQLite index with their type and document and chunk counts"),
		mcp.WithString("dbPath",
			mcp.Description("Optional path to SQLite DB. Defaults to CONFLUENCE_INDEX_DB, then the configuration file, then confluence2md-index.db"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		effectiveDBPath, err := dbPathFor(request.GetArguments())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		response, err := indexerapi.Stats(ctx, effectiveDBPath)
		if err != nil {
			message := fmt.Sprintf("list_sources failed for %s: %v", effectiveDBPath, err)
			return mcp.NewToolResultError(withBusyGuidance(message, err)), nil
		}

		// The indexer reports an empty list, never null, for an index without documents.
		sources := response.Stats.Sources

		payload := map[string]any{
			"schemaVersion": schemaVersion,
			"tool":          name,
			"dbPath":        effectiveDBPath,
			"count":         len(sources),
			"sources":       sources,
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	})
}
