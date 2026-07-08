package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/gkoos/confluence2md-indexer/pkg/indexerapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const schemaVersion = "1"

func main() {
	dbPath := os.Getenv("CONFLUENCE_INDEX_DB")
	if dbPath == "" {
		dbPath = "confluence2md-index.db"
	}

	s := server.NewMCPServer(
		"Confluence MCP",
		"0.1.0",
		server.WithToolCapabilities(false),
		server.WithRecovery(),
	)

	searchTool := mcp.NewTool("confluence.search",
		mcp.WithDescription("Search indexed Confluence content from a local SQLite DB"),
		mcp.WithString("query",
			mcp.Required(),
			mcp.Description("Search query text"),
		),
		mcp.WithString("dbPath",
			mcp.Description("Optional path to SQLite DB. Defaults to CONFLUENCE_INDEX_DB or confluence2md-index.db"),
		),
		mcp.WithString("mode",
			mcp.Description("Retrieval mode: hybrid, lexical, or vector"),
			mcp.Enum("hybrid", "lexical", "vector"),
		),
		mcp.WithString("fusion",
			mcp.Description("Fusion mode: weighted or rrf"),
			mcp.Enum("weighted", "rrf"),
		),
		mcp.WithNumber("alpha", mcp.Description("Weighted fusion alpha in [0..1]")),
		mcp.WithNumber("rrfK", mcp.Description("RRF k constant (>0)")),
		mcp.WithNumber("topK", mcp.Description("Top ranked results to consider")),
		mcp.WithNumber("offset", mcp.Description("Result offset")),
		mcp.WithNumber("limit", mcp.Description("Result limit")),
		mcp.WithNumber("candidateK", mcp.Description("Candidates per retrieval channel")),
		mcp.WithNumber("expand", mcp.Description("Context expansion chunk count")),
		mcp.WithString("spaceKey", mcp.Description("Optional filter by space key")),
		mcp.WithString("pageId", mcp.Description("Optional filter by page ID")),
		mcp.WithString("fromDate", mcp.Description("Optional lower bound YYYY-MM-DD")),
		mcp.WithString("toDate", mcp.Description("Optional upper bound YYYY-MM-DD")),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		queryText, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		effectiveDBPath := getString(args, "dbPath", dbPath)
		req := indexerapi.QueryRequest{
			Text:       queryText,
			Mode:       getString(args, "mode", "hybrid"),
			Fusion:     getString(args, "fusion", "weighted"),
			Alpha:      getFloat(args, "alpha", 0.70),
			RRFK:       getInt(args, "rrfK", 60),
			TopK:       getInt(args, "topK", 10),
			Offset:     getInt(args, "offset", 0),
			Limit:      getInt(args, "limit", 0),
			CandidateK: getInt(args, "candidateK", 50),
			Expand:     getInt(args, "expand", 0),
			Filters:    indexerapi.QueryRequest{}.Filters,
		}
		req.Filters.SpaceKey = getString(args, "spaceKey", "")
		req.Filters.PageID = getString(args, "pageId", "")
		req.Filters.FromDate = getString(args, "fromDate", "")
		req.Filters.ToDate = getString(args, "toDate", "")

		resp, err := indexerapi.Query(ctx, effectiveDBPath, req)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("query failed: %v", err)), nil
		}

		payload := map[string]any{
			"schemaVersion": schemaVersion,
			"tool":          "confluence.search",
			"dbPath":        effectiveDBPath,
			"request":       req,
			"count":         len(resp.Results),
			"total":         resp.Total,
			"results":       resp.Results,
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	})

	log.Printf("starting MCP server: dbPath=%s", dbPath)
	if err := server.ServeStdio(s); err != nil {
		log.Printf("server error: %v", err)
		os.Exit(1)
	}
}

func getString(args map[string]any, key, fallback string) string {
	v, ok := args[key]
	if !ok || v == nil {
		return fallback
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return fallback
	}
	return s
}

func getFloat(args map[string]any, key string, fallback float64) float64 {
	v, ok := args[key]
	if !ok || v == nil {
		return fallback
	}
	n, ok := v.(float64)
	if ok {
		return n
	}
	if s, ok := v.(string); ok {
		if parsed, err := strconv.ParseFloat(s, 64); err == nil {
			return parsed
		}
	}
	return fallback
}

func getInt(args map[string]any, key string, fallback int) int {
	v, ok := args[key]
	if !ok || v == nil {
		return fallback
	}
	n, ok := v.(float64)
	if ok {
		return int(n)
	}
	if i, ok := v.(int); ok {
		return i
	}
	if s, ok := v.(string); ok {
		if parsed, err := strconv.Atoi(s); err == nil {
			return parsed
		}
	}
	return fallback
}
