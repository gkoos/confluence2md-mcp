package main

import (
	"errors"
	"strings"
	"testing"
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
