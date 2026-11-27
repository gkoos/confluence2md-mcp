package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func clearConfigEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{envIndexDB, envIndexConfig, envIndexerConfig} {
		t.Setenv(name, "")
	}
	t.Chdir(t.TempDir())
}

func TestNoConfigFileKeepsBuiltInBehaviour(t *testing.T) {
	clearConfigEnv(t)
	// A config.yaml in the working directory must not be picked up on its own.
	if err := os.WriteFile("config.yaml", []byte("query:\n  mode: lexical\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	req, dbPath, err := buildQueryRequest("page", map[string]any{}, time.Now().UTC())
	if err != nil {
		t.Fatalf("buildQueryRequest: %v", err)
	}
	if req.Mode != "hybrid" || dbPath != defaultDBFile {
		t.Fatalf("mode=%q db=%q, want the built-in hybrid and %s", req.Mode, dbPath, defaultDBFile)
	}
}

func TestConfigFileSuppliesDefaultsAndArgumentsOverrideThem(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv(envIndexConfig, writeConfig(t, `
db:
  path: from-config.db
query:
  mode: lexical
  top_k: 25
  priors: [recency]
  source_weights:
    - id: docs
      weight: 1.5
`))

	req, dbPath, err := buildQueryRequest("page", map[string]any{}, time.Now().UTC())
	if err != nil {
		t.Fatalf("buildQueryRequest: %v", err)
	}
	if dbPath != "from-config.db" || req.Mode != "lexical" || req.TopK != 25 {
		t.Fatalf("config values lost: db=%q mode=%q topK=%d", dbPath, req.Mode, req.TopK)
	}
	if len(req.Priors.Priors) != 1 || req.Priors.Now.IsZero() {
		t.Fatalf("priors = %+v, want recency anchored to now", req.Priors)
	}
	if req.SourceWeights["docs"] != 1.5 {
		t.Fatalf("source weights = %v", req.SourceWeights)
	}

	req, dbPath, err = buildQueryRequest("page", map[string]any{"mode": "hybrid", "topK": float64(3), "dbPath": "arg.db"}, time.Now().UTC())
	if err != nil {
		t.Fatalf("buildQueryRequest: %v", err)
	}
	if dbPath != "arg.db" || req.Mode != "hybrid" || req.TopK != 3 {
		t.Fatalf("arguments must beat the file: db=%q mode=%q topK=%d", dbPath, req.Mode, req.TopK)
	}
}

func TestDBPathPrecedence(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv(envIndexConfig, writeConfig(t, "db:\n  path: from-config.db\n"))

	if got, _ := dbPathFor(nil); got != "from-config.db" {
		t.Fatalf("file db = %q, want from-config.db", got)
	}

	t.Setenv(envIndexDB, "from-env.db")
	if got, _ := dbPathFor(nil); got != "from-env.db" {
		t.Fatalf("env must beat the file: %q", got)
	}
	if got, _ := dbPathFor(map[string]any{"dbPath": "arg.db"}); got != "arg.db" {
		t.Fatalf("argument must beat the env: %q", got)
	}
}

func TestConfigFileIsReadOnEveryCall(t *testing.T) {
	clearConfigEnv(t)
	path := writeConfig(t, "query:\n  top_k: 5\n")
	t.Setenv(envIndexConfig, path)

	first, _, err := buildQueryRequest("page", map[string]any{}, time.Now().UTC())
	if err != nil || first.TopK != 5 {
		t.Fatalf("first call: topK=%d err=%v", first.TopK, err)
	}

	if err := os.WriteFile(path, []byte("query:\n  top_k: 7\n"), 0o644); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	second, _, err := buildQueryRequest("page", map[string]any{}, time.Now().UTC())
	if err != nil || second.TopK != 7 {
		t.Fatalf("an edit must apply without a restart: topK=%d err=%v", second.TopK, err)
	}
}

func TestConfigEmbeddingLayersBelowEnvironmentAndArguments(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv(envIndexConfig, writeConfig(t, "embedding:\n  provider: bow-local\n  dimension: 128\n  context: title\n"))

	req, _, err := buildQueryRequest("page", map[string]any{}, time.Now().UTC())
	if err != nil {
		t.Fatalf("buildQueryRequest: %v", err)
	}
	if req.Embedding.Dimension != 128 || len(req.Embedding.Context) != 1 {
		t.Fatalf("file embedding lost: %+v", req.Embedding)
	}

	t.Setenv("CONFLUENCE2MD_EMBEDDING_DIM", "256")
	req, _, err = buildQueryRequest("page", map[string]any{}, time.Now().UTC())
	if err != nil || req.Embedding.Dimension != 256 {
		t.Fatalf("environment must beat the file: %+v err=%v", req.Embedding, err)
	}

	req, _, err = buildQueryRequest("page", map[string]any{"embeddingDim": float64(512)}, time.Now().UTC())
	if err != nil || req.Embedding.Dimension != 512 {
		t.Fatalf("argument must beat the environment: %+v err=%v", req.Embedding, err)
	}
}

func TestLexicalQueryIgnoresBrokenEmbeddingEnvironment(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv("CONFLUENCE2MD_EMBEDDING_DIM", "not-a-number")

	if _, _, err := buildQueryRequest("page", map[string]any{"mode": "lexical"}, time.Now().UTC()); err != nil {
		t.Fatalf("lexical must not need embedding settings: %v", err)
	}
	if _, _, err := buildQueryRequest("page", map[string]any{"mode": "hybrid"}, time.Now().UTC()); err == nil {
		t.Fatal("hybrid must report the broken embedding setting")
	}
}

func TestBrokenConfigFileIsAToolError(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv(envIndexConfig, filepath.Join(t.TempDir(), "absent.yaml"))

	_, _, err := buildQueryRequest("page", map[string]any{}, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "configuration file") {
		t.Fatalf("a missing named file must be reported, got %v", err)
	}
	if _, err := dbPathFor(nil); err == nil {
		t.Fatal("dbPathFor must report an unreadable file")
	}
}

func TestSuppliedEmbeddingArgumentsCanClearConfiguredValues(t *testing.T) {
	clearConfigEnv(t)
	t.Setenv(envIndexConfig, writeConfig(t, `
embedding:
  provider: bow-local
  dimension: 128
  query_prefix: "q: "
  skip: true
`))

	req, _, err := buildQueryRequest("page", map[string]any{}, time.Now().UTC())
	if err != nil {
		t.Fatalf("buildQueryRequest: %v", err)
	}
	if !req.Embedding.Skip || req.Embedding.QueryPrefix != "q: " || req.Embedding.Dimension != 128 {
		t.Fatalf("file values lost: %+v", req.Embedding)
	}

	req, _, err = buildQueryRequest("page", map[string]any{
		"embeddingSkip":        false,
		"embeddingQueryPrefix": "",
		"embeddingDim":         float64(0),
	}, time.Now().UTC())
	if err != nil {
		t.Fatalf("buildQueryRequest: %v", err)
	}
	if req.Embedding.Skip || req.Embedding.QueryPrefix != "" || req.Embedding.Dimension != 0 {
		t.Fatalf("supplied empty values must override the file: %+v", req.Embedding)
	}
}
