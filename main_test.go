package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

// The indexer messages below are quoted verbatim from confluence2md-indexer
// v0.5.0, so a change in wording surfaces here rather than in a tool result.
func TestQueryErrorAddsActionableGuidance(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		contains []string
	}{
		{
			name: "embedding mismatch",
			err: errors.New(
				`query: embedding mismatch: index vectors are "bow-local:fnv1a@256" but the configured provider is ` +
					`"bow-local:fnv1a@512"; re-index with --rebuild, select the stored provider, or query with --mode lexical`,
			),
			contains: []string{"embedding mismatch", "CONFLUENCE2MD_EMBEDDING_*", "mode=lexical"},
		},
		{
			name: "index without vectors",
			err: errors.New(
				`query: the index holds no embeddings to search with "bow-local:fnv1a@256"; ` +
					`re-index with --embedding bow-local, or query with --mode lexical`,
			),
			contains: []string{"holds no embeddings", "Rebuild it with an embedding provider"},
		},
		{
			name:     "file without an index",
			err:      errors.New(`this file holds no index yet; build one with: confluence2md-indexer index <folder>`),
			contains: []string{"confluence2md-indexer index <folder>"},
		},
		{
			name: "embeddings disabled",
			err: errors.New(
				`query mode "hybrid" requires embeddings, but they are disabled ` +
					`(drop --skip-embeddings or select a provider with --embedding)`,
			),
			contains: []string{"CONFLUENCE2MD_EMBEDDING_SKIP", "mode=lexical"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := queryError("/tmp/index.db", tc.err)

			if !strings.Contains(got, "/tmp/index.db") {
				t.Fatalf("query error does not name the database: %q", got)
			}
			if !strings.Contains(got, tc.err.Error()) {
				t.Fatalf("query error does not keep the indexer message: %q", got)
			}
			for _, want := range tc.contains {
				if !strings.Contains(got, want) {
					t.Fatalf("query error %q does not contain %q", got, want)
				}
			}
		})
	}
}

func TestQueryErrorPassesUnknownFailuresThrough(t *testing.T) {
	got := queryError("/tmp/index.db", errors.New("some unexpected failure"))
	want := "query failed for /tmp/index.db: some unexpected failure"

	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSchemaVersionMatchesIndexerContract(t *testing.T) {
	if schemaVersion == "" {
		t.Fatal("schemaVersion must mirror the indexer output contract")
	}
}

func TestQueryRequestFromArgsDefaults(t *testing.T) {
	req, err := queryRequestFromArgs("page", map[string]any{}, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Mode != "hybrid" || req.Fusion != "weighted" || req.Alpha != 0.70 {
		t.Fatalf("unexpected retrieval defaults: mode=%q fusion=%q alpha=%v", req.Mode, req.Fusion, req.Alpha)
	}
	if req.RRFK != 60 || req.TopK != 10 || req.CandidateK != 50 || req.Offset != 0 || req.Limit != 0 || req.Expand != 0 {
		t.Fatalf("unexpected numeric defaults: %+v", req)
	}
	if req.Filters.DepthMin != nil || req.Filters.DepthMax != nil {
		t.Fatal("depth bounds must stay unset when the caller does not supply them")
	}
	if req.Filters.SeedOnly || req.Filters.HasAttachments || req.Filters.UpdatedSince != "" {
		t.Fatalf("unexpected filter defaults: %+v", req.Filters)
	}
	if req.Embedding.Provider != "" || req.Embedding.Model != "" || req.Embedding.Dimension != 0 || req.Embedding.Skip {
		t.Fatalf("embedding overrides must stay empty so the environment applies: %+v", req.Embedding)
	}
}

func TestQueryRequestFromArgsMapsMetadataFilters(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	args := map[string]any{
		"spaceKey":       "ENG",
		"spaces":         []any{"ENG", "OPS"},
		"pageId":         "1001",
		"fromDate":       "2026-01-01",
		"toDate":         "2026-02-01",
		"host":           "wiki.example.com",
		"author":         "Ada Lovelace",
		"createdBy":      "Ada Lovelace",
		"modifiedBy":     "Grace Hopper",
		"depthMin":       float64(0),
		"depthMax":       float64(3),
		"seedOnly":       true,
		"hasAttachments": true,
		"updatedSince":   "30d",
	}

	req, err := queryRequestFromArgs("page", args, now)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Filters.SpaceKey != "ENG" || req.Filters.PageID != "1001" || req.Filters.Host != "wiki.example.com" {
		t.Fatalf("unexpected identity filters: %+v", req.Filters)
	}
	if len(req.Filters.Spaces) != 2 || req.Filters.Spaces[0] != "ENG" || req.Filters.Spaces[1] != "OPS" {
		t.Fatalf("unexpected spaces: %v", req.Filters.Spaces)
	}
	if req.Filters.Author != "Ada Lovelace" || req.Filters.CreatedBy != "Ada Lovelace" || req.Filters.ModifiedBy != "Grace Hopper" {
		t.Fatalf("unexpected author filters: %+v", req.Filters)
	}
	if req.Filters.DepthMin == nil || *req.Filters.DepthMin != 0 {
		t.Fatal("depthMin 0 is a seed page and must be preserved")
	}
	if req.Filters.DepthMax == nil || *req.Filters.DepthMax != 3 {
		t.Fatal("depthMax 3 must be preserved")
	}
	if !req.Filters.SeedOnly || !req.Filters.HasAttachments {
		t.Fatalf("unexpected boolean filters: %+v", req.Filters)
	}
	if want := "2026-08-21T12:00:00Z"; req.Filters.UpdatedSince != want {
		t.Fatalf("updatedSince = %q, want %q", req.Filters.UpdatedSince, want)
	}
}

func TestQueryRequestFromArgsAcceptsCommaSeparatedSpaces(t *testing.T) {
	req, err := queryRequestFromArgs("page", map[string]any{"spaces": "ENG, OPS ,"}, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Filters.Spaces) != 2 || req.Filters.Spaces[0] != "ENG" || req.Filters.Spaces[1] != "OPS" {
		t.Fatalf("unexpected spaces: %v", req.Filters.Spaces)
	}
}

func TestQueryRequestFromArgsMapsEmbeddingOverrides(t *testing.T) {
	args := map[string]any{
		"embeddingProvider":       "openai",
		"embeddingModel":          "text-embedding-3-small",
		"embeddingDim":            float64(1024),
		"embeddingBaseURL":        "https://api.example.com/v1",
		"embeddingApiKeyEnv":      "MY_KEY",
		"embeddingAuthHeader":     "X-Api-Key",
		"embeddingAuthScheme":     "Bearer",
		"embeddingSkip":           false,
		"embeddingDocumentPrefix": "passage: ",
		"embeddingQueryPrefix":    "query: ",
	}

	req, err := queryRequestFromArgs("page", args, time.Now().UTC())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Embedding.Provider != "openai" || req.Embedding.Model != "text-embedding-3-small" || req.Embedding.Dimension != 1024 {
		t.Fatalf("unexpected provider override: %+v", req.Embedding)
	}
	if req.Embedding.BaseURL != "https://api.example.com/v1" || req.Embedding.APIKeyEnv != "MY_KEY" {
		t.Fatalf("unexpected endpoint override: %+v", req.Embedding)
	}
	if req.Embedding.AuthHeader != "X-Api-Key" || req.Embedding.AuthScheme != "Bearer" {
		t.Fatalf("unexpected auth override: %+v", req.Embedding)
	}
	if req.Embedding.DocPrefix != "passage: " || req.Embedding.QueryPrefix != "query: " {
		t.Fatalf("prefixes must keep their exact whitespace: %+v", req.Embedding)
	}
}

func TestQueryRequestFromArgsRejectsInvalidArguments(t *testing.T) {
	tests := []struct {
		name     string
		args     map[string]any
		contains string
	}{
		{name: "negative depthMin", args: map[string]any{"depthMin": float64(-1)}, contains: ">= 0"},
		{name: "depth range inverted", args: map[string]any{"depthMin": float64(3), "depthMax": float64(1)}, contains: "depthMin"},
		{name: "negative dimension", args: map[string]any{"embeddingDim": float64(-1)}, contains: "embeddingDim"},
		{name: "unparsable age", args: map[string]any{"updatedSince": "yesterday"}, contains: "30d"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := queryRequestFromArgs("page", tc.args, time.Now().UTC()); err == nil {
				t.Fatal("expected an error")
			} else if !strings.Contains(err.Error(), tc.contains) {
				t.Fatalf("error %q does not mention %q", err, tc.contains)
			}
		})
	}
}

func TestUpdatedSinceBound(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		value string
		want  string
	}{
		{value: "", want: ""},
		{value: "30d", want: "2026-08-21T12:00:00Z"},
		{value: "2w", want: "2026-09-06T12:00:00Z"},
		{value: "12h", want: "2026-09-20T00:00:00Z"},
		{value: "0d", want: ""},
		{value: "2026-01-01", want: "2026-01-01T00:00:00Z"},
		{value: "2026-01-01T05:30:00Z", want: "2026-01-01T05:30:00Z"},
	}

	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			got, err := updatedSinceBound(tc.value, now)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}
