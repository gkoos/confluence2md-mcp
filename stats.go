package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gkoos/confluence2md-indexer/pkg/indexerapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// statsTool registers confluence.stats, which reports what an index holds, the crawl it
// was built from and the embedding identity behind its vectors. It is what turns "the
// query failed" into "the index is empty, older than the crawl, or built with another
// provider".
func statsTool(s *server.MCPServer, defaultDBPath string) {
	tool := mcp.NewTool("confluence.stats",
		mcp.WithDescription("Report an index's size, crawl freshness, metadata coverage and stored embedding identity"),
		mcp.WithString("dbPath",
			mcp.Description("Optional path to SQLite DB. Defaults to CONFLUENCE_INDEX_DB or confluence2md-index.db"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		effectiveDBPath := getString(request.GetArguments(), "dbPath", defaultDBPath)

		response, err := indexerapi.Stats(ctx, effectiveDBPath)
		if err != nil {
			return mcp.NewToolResultError(statsError(effectiveDBPath, err)), nil
		}

		payload := map[string]any{
			"schemaVersion": schemaVersion,
			"tool":          "confluence.stats",
			"dbPath":        effectiveDBPath,
			"stats":         response.Stats,
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	})
}

// statsError names the database and keeps the indexer's message. That message already
// explains a schema mismatch or a file without an index, so no extra guidance is added.
func statsError(dbPath string, err error) string {
	return fmt.Sprintf("stats failed for %s: %v", dbPath, err)
}
