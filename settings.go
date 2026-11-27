package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/gkoos/confluence2md-indexer/pkg/indexerapi"
)

const (
	envIndexDB     = "CONFLUENCE_INDEX_DB"
	envIndexConfig = "CONFLUENCE_INDEX_CONFIG"
	// envIndexerConfig is the variable the indexer CLI reads; honouring it lets one
	// exported path serve both tools.
	envIndexerConfig = "CONFLUENCE2MD_CONFIG"
	defaultDBFile    = "confluence2md-index.db"
)

// loadSettings reads the configuration file on every call, so an edit applies to the
// next tool call without restarting the server. No file is configured by default: unlike
// the indexer CLI, the server never picks up a config.yaml from its working directory.
func loadSettings() (*indexerapi.Settings, error) {
	path := strings.TrimSpace(os.Getenv(envIndexConfig))
	if path == "" {
		path = strings.TrimSpace(os.Getenv(envIndexerConfig))
	}
	if path == "" {
		return &indexerapi.Settings{}, nil
	}

	settings, err := indexerapi.LoadSettings(path)
	if err != nil {
		return nil, fmt.Errorf("configuration file: %w", err)
	}
	return settings, nil
}

// resolveDBPath picks the index: the dbPath argument, then CONFLUENCE_INDEX_DB, then
// db.path from the configuration file, then the default file name.
func resolveDBPath(args map[string]any, settings *indexerapi.Settings) string {
	if path := getString(args, "dbPath", ""); path != "" {
		return path
	}
	if path := os.Getenv(envIndexDB); path != "" {
		return path
	}
	if path := settings.DBPath(); path != "" {
		return path
	}
	return defaultDBFile
}

// dbPathFor resolves the index for tools that take no retrieval settings.
func dbPathFor(args map[string]any) (string, error) {
	settings, err := loadSettings()
	if err != nil {
		return "", err
	}
	return resolveDBPath(args, settings), nil
}

// buildQueryRequest maps tool arguments onto an indexer request. Settings layer as the
// indexer CLI does: an argument beats the environment, which beats the configuration
// file, which beats the built-in default.
func buildQueryRequest(text string, args map[string]any, now time.Time) (indexerapi.QueryRequest, string, error) {
	settings, err := loadSettings()
	if err != nil {
		return indexerapi.QueryRequest{}, "", err
	}

	defaults, err := settings.QueryDefaults()
	if err != nil {
		return indexerapi.QueryRequest{}, "", fmt.Errorf("configuration file: %w", err)
	}

	req, err := queryRequestFromArgs(text, args, now, defaults)
	if err != nil {
		return indexerapi.QueryRequest{}, "", err
	}

	// A lexical query never resolves a provider, so a broken embedding setting must not
	// stop it.
	if req.Mode != "lexical" {
		req.Embedding, err = settings.ResolveEmbedding(req.Embedding, embeddingPresence(args))
		if err != nil {
			return indexerapi.QueryRequest{}, "", err
		}
	}

	return req, resolveDBPath(args, settings), nil
}

// embeddingPresence marks the embedding arguments the caller supplied, whatever their
// value, following the indexer's presence contract: an empty prefix, a zero dimension or
// a false skip is a decision that overrides the environment and the file, while an
// omitted argument leaves them in force.
func embeddingPresence(args map[string]any) indexerapi.EmbeddingPresent {
	supplied := func(key string) bool {
		value, ok := args[key]
		return ok && value != nil
	}

	return indexerapi.EmbeddingPresent{
		Provider:    supplied("embeddingProvider"),
		Model:       supplied("embeddingModel"),
		BaseURL:     supplied("embeddingBaseURL"),
		APIKeyEnv:   supplied("embeddingApiKeyEnv"),
		AuthHeader:  supplied("embeddingAuthHeader"),
		AuthScheme:  supplied("embeddingAuthScheme"),
		DocPrefix:   supplied("embeddingDocumentPrefix"),
		QueryPrefix: supplied("embeddingQueryPrefix"),
		Dimension:   supplied("embeddingDim"),
		Context:     supplied("embeddingContext"),
		Skip:        supplied("embeddingSkip"),
	}
}
