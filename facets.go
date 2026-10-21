package main

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gkoos/confluence2md-indexer/pkg/indexerapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// defaultFacetDimensions is what confluence.facets counts when the caller does not pick.
var defaultFacetDimensions = []string{"space", "author"}

// facetsTool registers confluence.facets: the same request surface as a search, but only
// the shape of the matching set. A client asks "where does this query live" without
// pulling chunk text into the conversation.
func facetsTool(s *server.MCPServer, defaultDBPath string) {
	tool := mcp.NewTool("confluence.facets",
		append([]mcp.ToolOption{
			mcp.WithDescription("Count the documents a search matches per space and author, without returning them"),
		}, searchArgumentOptions()...)...)

	s.AddTool(tool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		queryText, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		effectiveDBPath := getString(args, "dbPath", defaultDBPath)
		req, err := queryRequestFromArgs(queryText, args, time.Now().UTC())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if len(req.Facets) == 0 {
			req.Facets = append([]string(nil), defaultFacetDimensions...)
		}

		resp, err := indexerapi.Query(ctx, effectiveDBPath, req)
		if err != nil {
			return mcp.NewToolResultError(queryError(effectiveDBPath, err)), nil
		}

		// Counts only: the rows exist to be counted, not returned.
		payload := map[string]any{
			"schemaVersion": schemaVersion,
			"tool":          "confluence.facets",
			"dbPath":        effectiveDBPath,
			"request":       req,
			"total":         resp.Total,
		}
		if len(resp.Facets) > 0 {
			payload["facets"] = resp.Facets
		} else {
			payload["facets"] = map[string]map[string]int{}
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	})
}
