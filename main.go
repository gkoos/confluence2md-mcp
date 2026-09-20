package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gkoos/confluence2md-indexer/pkg/indexerapi"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// serverName is the display name an MCP client shows for this server.
const serverName = "Confluence MCP"

// version is the build identity reported to MCP clients and written to the
// startup log. Release builds stamp it through ldflags; an unstamped build
// reports "dev".
var version = "dev"

// schemaVersion mirrors the indexer's output contract, so a payload from this
// server and one from the indexer describe themselves identically.
const schemaVersion = indexerapi.OutputSchemaVersion

func main() {
	dbPath := os.Getenv("CONFLUENCE_INDEX_DB")
	if dbPath == "" {
		dbPath = "confluence2md-index.db"
	}

	s := server.NewMCPServer(
		serverName,
		version,
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
		mcp.WithArray("spaces", mcp.Description("Filter by any of these space keys; wins over spaceKey"), mcp.WithStringItems()),
		mcp.WithString("host", mcp.Description("Optional filter by crawled site host")),
		mcp.WithString("author", mcp.Description("Filter by creator or last modifier name, case-insensitive")),
		mcp.WithString("createdBy", mcp.Description("Filter by creator name only")),
		mcp.WithString("modifiedBy", mcp.Description("Filter by last modifier name only")),
		mcp.WithNumber("depthMin", mcp.Description("Minimum crawl depth; 1 excludes seed pages")),
		mcp.WithNumber("depthMax", mcp.Description("Maximum crawl depth")),
		mcp.WithBoolean("seedOnly", mcp.Description("Keep only the pages the crawl started from")),
		mcp.WithBoolean("hasAttachments", mcp.Description("Keep only pages that carry at least one attachment")),
		mcp.WithString("updatedSince", mcp.Description("Keep pages modified within an age such as 30d, 2w or 12h, or after an absolute date such as 2026-01-01")),
		mcp.WithString("embeddingProvider", mcp.Description("Override the embedding provider for this call: bow-local, openai or openai-compatible")),
		mcp.WithString("embeddingModel", mcp.Description("Override the embedding model for this call")),
		mcp.WithNumber("embeddingDim", mcp.Description("Override the embedding dimension for this call")),
		mcp.WithString("embeddingBaseURL", mcp.Description("Override the embedding endpoint for this call")),
		mcp.WithString("embeddingApiKeyEnv", mcp.Description("Name of the environment variable that holds the API key")),
		mcp.WithString("embeddingAuthHeader", mcp.Description("Override the authentication header name")),
		mcp.WithString("embeddingAuthScheme", mcp.Description("Override the authentication scheme prefix")),
		mcp.WithBoolean("embeddingSkip", mcp.Description("Disable the vector channel for this call")),
		mcp.WithString("embeddingDocumentPrefix", mcp.Description("Prefix added to document text before embedding")),
		mcp.WithString("embeddingQueryPrefix", mcp.Description("Prefix added to the query before embedding")),
	)

	s.AddTool(searchTool, func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		args := request.GetArguments()

		queryText, err := request.RequireString("query")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		effectiveDBPath := getString(args, "dbPath", dbPath)
		req, err := queryRequestFromArgs(queryText, args, time.Now().UTC())
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		resp, err := indexerapi.Query(ctx, effectiveDBPath, req)
		if err != nil {
			return mcp.NewToolResultError(queryError(effectiveDBPath, err)), nil
		}

		// An empty result set is an empty array rather than null, so a client can
		// iterate the field without a nil check.
		results := resp.Results
		if results == nil {
			results = []indexerapi.QueryResult{}
		}

		payload := map[string]any{
			"schemaVersion": schemaVersion,
			"tool":          "confluence.search",
			"dbPath":        effectiveDBPath,
			"request":       req,
			"count":         len(results),
			"total":         resp.Total,
			"results":       results,
		}
		b, _ := json.MarshalIndent(payload, "", "  ")
		return mcp.NewToolResultText(string(b)), nil
	})

	listSpacesTool(s, dbPath)

	log.Printf("starting %s %s: dbPath=%s", serverName, version, dbPath)
	if err := server.ServeStdio(s); err != nil {
		log.Printf("server error: %v", err)
		os.Exit(1)
	}
}

// queryError explains the failures a caller can act on. The indexer reports what
// is wrong; this adds what a client of this server can do about it.
func queryError(dbPath string, err error) string {
	message := fmt.Sprintf("query failed for %s: %v", dbPath, err)

	switch {
	case strings.Contains(message, "embedding mismatch:"):
		return message + "\n\nThis index was built with a different embedding configuration than the query " +
			"resolved. Set the CONFLUENCE2MD_EMBEDDING_* variables to match the provider used at index time, " +
			"or retry with mode=lexical, which needs no embeddings."
	case strings.Contains(message, "holds no embeddings"):
		return message + "\n\nThis index carries no vectors. Rebuild it with an embedding provider, or use mode=lexical."
	case strings.Contains(message, "holds no index yet"):
		return message + "\n\nBuild the index first: confluence2md-indexer index <folder>."
	case strings.Contains(message, "requires embeddings, but they are disabled"):
		return message + "\n\nEmbeddings are switched off for this server. Unset CONFLUENCE2MD_EMBEDDING_SKIP " +
			"(or select a provider) and retry, or use mode=lexical."
	}

	return message
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

// queryRequestFromArgs maps MCP arguments onto the indexer request. Now is a
// parameter so a relative updatedSince is reproducible in tests.
//
// Arguments win over the environment: the indexer fills only the embedding
// fields this function leaves empty, so an override here cannot be undone by a
// CONFLUENCE2MD_EMBEDDING_* variable.
func queryRequestFromArgs(text string, args map[string]any, now time.Time) (indexerapi.QueryRequest, error) {
	req := indexerapi.QueryRequest{
		Text:       text,
		Mode:       getString(args, "mode", "hybrid"),
		Fusion:     getString(args, "fusion", "weighted"),
		Alpha:      getFloat(args, "alpha", 0.70),
		RRFK:       getInt(args, "rrfK", 60),
		TopK:       getInt(args, "topK", 10),
		Offset:     getInt(args, "offset", 0),
		Limit:      getInt(args, "limit", 0),
		CandidateK: getInt(args, "candidateK", 50),
		Expand:     getInt(args, "expand", 0),
	}

	req.Filters.SpaceKey = getString(args, "spaceKey", "")
	req.Filters.Spaces = getStrings(args, "spaces")
	req.Filters.PageID = getString(args, "pageId", "")
	req.Filters.FromDate = getString(args, "fromDate", "")
	req.Filters.ToDate = getString(args, "toDate", "")
	req.Filters.Host = getString(args, "host", "")
	req.Filters.Author = getString(args, "author", "")
	req.Filters.CreatedBy = getString(args, "createdBy", "")
	req.Filters.ModifiedBy = getString(args, "modifiedBy", "")
	req.Filters.SeedOnly = getBool(args, "seedOnly", false)
	req.Filters.HasAttachments = getBool(args, "hasAttachments", false)

	depthMin, hasDepthMin := optionalInt(args, "depthMin")
	depthMax, hasDepthMax := optionalInt(args, "depthMax")
	if depthMin < 0 || depthMax < 0 {
		return indexerapi.QueryRequest{}, fmt.Errorf("depthMin and depthMax must be >= 0")
	}
	if hasDepthMin && hasDepthMax && depthMin > depthMax {
		return indexerapi.QueryRequest{}, fmt.Errorf("depthMin must not be greater than depthMax")
	}
	if hasDepthMin {
		req.Filters.DepthMin = &depthMin
	}
	if hasDepthMax {
		req.Filters.DepthMax = &depthMax
	}

	updatedSince, err := updatedSinceBound(getString(args, "updatedSince", ""), now)
	if err != nil {
		return indexerapi.QueryRequest{}, err
	}
	req.Filters.UpdatedSince = updatedSince

	// Embedding overrides. A literal API key is deliberately not an argument: it
	// would travel through the client conversation and the server log, so callers
	// name the variable that holds it instead.
	req.Embedding.Provider = getString(args, "embeddingProvider", "")
	req.Embedding.Model = getString(args, "embeddingModel", "")
	req.Embedding.BaseURL = getString(args, "embeddingBaseURL", "")
	req.Embedding.APIKeyEnv = getString(args, "embeddingApiKeyEnv", "")
	req.Embedding.AuthHeader = getString(args, "embeddingAuthHeader", "")
	req.Embedding.AuthScheme = getString(args, "embeddingAuthScheme", "")
	req.Embedding.DocPrefix = getString(args, "embeddingDocumentPrefix", "")
	req.Embedding.QueryPrefix = getString(args, "embeddingQueryPrefix", "")
	req.Embedding.Skip = getBool(args, "embeddingSkip", false)

	dimension, hasDimension := optionalInt(args, "embeddingDim")
	if hasDimension {
		if dimension < 0 {
			return indexerapi.QueryRequest{}, fmt.Errorf("embeddingDim must be >= 0")
		}
		req.Embedding.Dimension = dimension
	}

	return req, nil
}

// updatedSinceBound turns the caller's value into the absolute lower bound the
// index compares against, mirroring the indexer's --updated-since: a relative age
// such as 30d, 2w or 12h is subtracted from now, while an absolute timestamp is
// normalised to RFC3339.
func updatedSinceBound(value string, now time.Time) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}

	if age, ok := relativeAge(trimmed); ok {
		if age <= 0 {
			return "", nil
		}
		return now.Add(-age).Format(time.RFC3339), nil
	}

	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC().Format(time.RFC3339), nil
		}
	}

	return "", fmt.Errorf("updatedSince must be an age such as 30d, 2w or 12h, or a date such as 2026-01-01")
}

// relativeAge reports the duration of an age expression. Days and weeks are
// accepted next to Go durations, which is the syntax the indexer accepts.
func relativeAge(value string) (time.Duration, bool) {
	if amount, unit, ok := splitAmountUnit(value); ok {
		switch unit {
		case "d", "day", "days":
			return time.Duration(amount) * 24 * time.Hour, true
		case "w", "week", "weeks":
			return time.Duration(amount) * 7 * 24 * time.Hour, true
		}
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return 0, false
	}

	return duration, true
}

// splitAmountUnit splits a leading integer from a unit suffix ("30d" -> 30, "d").
func splitAmountUnit(value string) (int, string, bool) {
	digits := 0
	for digits < len(value) && value[digits] >= '0' && value[digits] <= '9' {
		digits++
	}
	if digits == 0 || digits == len(value) {
		return 0, "", false
	}

	amount, err := strconv.Atoi(value[:digits])
	if err != nil {
		return 0, "", false
	}

	return amount, strings.TrimSpace(strings.ToLower(value[digits:])), true
}

// getBool reads a boolean argument, accepting the strings a client may send.
func getBool(args map[string]any, key string, fallback bool) bool {
	v, ok := args[key]
	if !ok || v == nil {
		return fallback
	}
	if b, ok := v.(bool); ok {
		return b
	}
	if s, ok := v.(string); ok {
		if parsed, err := strconv.ParseBool(strings.TrimSpace(s)); err == nil {
			return parsed
		}
	}
	return fallback
}

// getStrings reads a list argument, accepting both a JSON array and the
// comma-separated form a client may send for convenience.
func getStrings(args map[string]any, key string) []string {
	v, ok := args[key]
	if !ok || v == nil {
		return nil
	}

	var values []string
	switch typed := v.(type) {
	case []string:
		values = typed
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok {
				values = append(values, s)
			}
		}
	case string:
		values = strings.Split(typed, ",")
	}

	var cleaned []string
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}

	return cleaned
}

// optionalInt reports an integer argument and whether the caller supplied it, so a
// meaningful zero (depth 0 is a seed page) is not mistaken for "unset".
func optionalInt(args map[string]any, key string) (int, bool) {
	if _, ok := args[key]; !ok {
		return 0, false
	}
	return getInt(args, key, 0), true
}
