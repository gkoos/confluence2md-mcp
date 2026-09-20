package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// indexerModuleVersion pins the indexer release that builds the fixture index. It
// matches the version this module requires, so the test follows the same paths a
// release build takes.
const indexerModuleVersion = "v0.5.0"

var (
	installIndexerOnce sync.Once
	indexerBinaryPath  string
	installIndexerErr  error
)

// indexerBinary installs the indexer CLI once per test run. The CLI builds the
// index in its own module context, and the fixture uses the default offline
// provider, so no API key or network service is involved.
func indexerBinary(t *testing.T) string {
	t.Helper()

	installIndexerOnce.Do(func() {
		binDir, err := os.MkdirTemp("", "confluence2md-indexer-bin")
		if err != nil {
			installIndexerErr = err
			return
		}

		exeName := "confluence2md-indexer"
		if runtime.GOOS == "windows" {
			exeName += ".exe"
		}

		cmd := exec.Command("go", "install",
			"github.com/gkoos/confluence2md-indexer/cmd/confluence2md-indexer@"+indexerModuleVersion)
		cmd.Env = append(os.Environ(), "GOBIN="+binDir)
		if out, err := cmd.CombinedOutput(); err != nil {
			installIndexerErr = fmt.Errorf("go install indexer: %w\n%s", err, out)
			return
		}

		indexerBinaryPath = filepath.Join(binDir, exeName)
	})

	if installIndexerErr != nil {
		t.Skipf("cannot install the indexer CLI that builds the fixture index: %v", installIndexerErr)
	}

	return indexerBinaryPath
}

// writeCorpus writes the smallest realistic crawler output: one metadata.json plus
// one markdown file per page. The two pages live in different spaces so a space
// list has to collapse duplicates, and the ops page mentions the same word as the
// engineering page so a space filter has something to exclude.
func writeCorpus(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	pages := map[string]map[string]string{
		"2001": {
			"local_path":       "apple.md",
			"title":            "Apple Runbook",
			"space_key":        "ENG",
			"last_modified_at": "2026-02-10T10:00:00Z",
			"source_url":       "https://example.test/pages/2001",
		},
		"2002": {
			"local_path":       "oncall.md",
			"title":            "On-call Handbook",
			"space_key":        "OPS",
			"last_modified_at": "2025-01-05T08:00:00Z",
			"source_url":       "https://example.test/pages/2002",
		},
	}

	metadata, err := json.MarshalIndent(map[string]any{"pages": pages}, "", "  ")
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "metadata.json"), metadata, 0o644); err != nil {
		t.Fatalf("write metadata.json: %v", err)
	}

	files := map[string]string{
		"apple.md":  "# Apple Runbook\n\nThe apple rotation happens on Mondays.\n",
		"oncall.md": "# On-call Handbook\n\nEscalate the apple incident to the ops channel.\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	return dir
}

// buildIndex runs the installed indexer against the corpus. bow-local is the
// default provider and needs no credentials, so the fixture works offline.
func buildIndex(t *testing.T, corpus string) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "index.db")
	cmd := exec.Command(indexerBinary(t), "index", corpus, "--db", dbPath, "--rebuild")
	cmd.Env = append(os.Environ(), "CONFLUENCE2MD_EMBEDDING_PROVIDER=bow-local")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fixture index: %v\n%s", err, out)
	}

	return dbPath
}

// mcpSession is one running server process driven over the stdio protocol.
type mcpSession struct {
	t      *testing.T
	stdin  io.Writer
	reader *bufio.Reader
}

// startSession launches the server against a fixed index and completes the MCP
// handshake, so every test call starts from an initialised session.
func startSession(t *testing.T, dbPath string, extraEnv ...string) *mcpSession {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, buildServerBinary(t))
	cmd.Env = append(append(os.Environ(), "CONFLUENCE_INDEX_DB="+dbPath), extraEnv...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_, _ = io.ReadAll(stderr)
	})

	session := &mcpSession{t: t, stdin: stdin, reader: bufio.NewReader(stdout)}

	sendMCP(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "offline-test", "version": "1.0.0"},
		},
	})
	assertResponseID(t, readMCP(t, session.reader, 15*time.Second), 1)

	sendMCP(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	})

	return session
}

// call performs one tools/call and returns the raw response.
func (s *mcpSession) call(id int, tool string, args map[string]any) []byte {
	s.t.Helper()

	sendMCP(s.t, s.stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "tools/call",
		"params":  map[string]any{"name": tool, "arguments": args},
	})

	return readMCP(s.t, s.reader, 30*time.Second)
}

// toolResult returns the text of the first content block plus whether the server
// reported a tool error.
func toolResult(t *testing.T, msg []byte) (string, bool) {
	t.Helper()

	var response struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
	}
	if err := json.Unmarshal(msg, &response); err != nil {
		t.Fatalf("invalid tool response: %v\n%s", err, msg)
	}
	if len(response.Result.Content) == 0 {
		t.Fatalf("tool response carries no content: %s", msg)
	}

	return response.Result.Content[0].Text, response.Result.IsError
}

// toolPayload decodes the JSON payload of a successful tool result.
func toolPayload(t *testing.T, msg []byte) map[string]any {
	t.Helper()

	text, isError := toolResult(t, msg)
	if isError {
		t.Fatalf("expected a successful tool call, got error: %s", text)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("tool result is not JSON: %v\n%s", err, text)
	}

	return payload
}

// resultTitles collects the titles of a search payload, in rank order. An empty
// result set is treated as an empty list.
func resultTitles(t *testing.T, payload map[string]any) []string {
	t.Helper()

	raw, present := payload["results"]
	if !present {
		t.Fatalf("payload carries no results field: %v", payload)
	}
	if raw == nil {
		return nil
	}

	results, ok := raw.([]any)
	if !ok {
		t.Fatalf("results must be an array: %v", raw)
	}

	var titles []string
	for _, raw := range results {
		result, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		titles = append(titles, fmt.Sprint(result["title"]))
	}

	return titles
}

// TestOfflineIndexServesSearchAndSpaces covers an index built by the pinned
// indexer release: the tool list, the space list and a search narrowed by space.
func TestOfflineIndexServesSearchAndSpaces(t *testing.T) {
	dbPath := buildIndex(t, writeCorpus(t))
	session := startSession(t, dbPath)

	sendMCP(t, session.stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})
	toolsMsg := readMCP(t, session.reader, 15*time.Second)
	assertResponseID(t, toolsMsg, 2)
	assertHasTool(t, toolsMsg, "confluence.search")
	assertHasTool(t, toolsMsg, "confluence.list_spaces")

	spacesPayload := toolPayload(t, session.call(3, "confluence.list_spaces", map[string]any{}))
	if got := spacesPayload["schemaVersion"]; got != schemaVersion {
		t.Fatalf("schemaVersion = %v, want %q", got, schemaVersion)
	}
	if got := spacesPayload["count"]; got != float64(2) {
		t.Fatalf("count = %v, want 2", got)
	}
	spaces, ok := spacesPayload["spaces"].([]any)
	if !ok {
		t.Fatalf("spaces must be an array: %v", spacesPayload["spaces"])
	}
	if len(spaces) != 2 || spaces[0] != "ENG" || spaces[1] != "OPS" {
		t.Fatalf("spaces = %v, want [ENG OPS]", spaces)
	}

	scoped := toolPayload(t, session.call(4, "confluence.search", map[string]any{
		"query": "apple", "mode": "hybrid", "spaces": []string{"ENG"}, "topK": 5,
	}))
	if titles := resultTitles(t, scoped); len(titles) != 1 || titles[0] != "Apple Runbook" {
		t.Fatalf("ENG search returned %v, want [Apple Runbook]", titles)
	}

	// The same query in the other space returns that space's page: the ops page
	// mentions the word too, so the filter (not the text) decides the result.
	scopedOps := toolPayload(t, session.call(5, "confluence.search", map[string]any{
		"query": "apple", "mode": "hybrid", "spaces": []string{"OPS"},
	}))
	if titles := resultTitles(t, scopedOps); len(titles) != 1 || titles[0] != "On-call Handbook" {
		t.Fatalf("OPS search returned %v, want [On-call Handbook]", titles)
	}

	unknown := toolPayload(t, session.call(6, "confluence.search", map[string]any{
		"query": "apple", "mode": "hybrid", "spaces": []string{"NONE"},
	}))
	if unknown["results"] == nil {
		t.Fatal("an empty result set must be an empty array, not null")
	}
	if titles := resultTitles(t, unknown); len(titles) != 0 {
		t.Fatalf("unknown space returned %v, want no results", titles)
	}
}

// TestOfflineIndexSurfacesProviderMismatch covers what a user hits when the server
// resolves a different embedding configuration than the index was built with, and
// that lexical mode still works in that process.
func TestOfflineIndexSurfacesProviderMismatch(t *testing.T) {
	dbPath := buildIndex(t, writeCorpus(t))
	session := startSession(t, dbPath, "CONFLUENCE2MD_EMBEDDING_DIM=512")

	text, isError := toolResult(t, session.call(2, "confluence.search", map[string]any{
		"query": "apple", "mode": "hybrid",
	}))
	if !isError {
		t.Fatalf("expected a tool error for a mismatched provider, got: %s", text)
	}
	for _, want := range []string{"embedding mismatch", "bow-local", "mode=lexical", "CONFLUENCE2MD_EMBEDDING_*"} {
		if !strings.Contains(text, want) {
			t.Fatalf("mismatch error %q does not mention %q", text, want)
		}
	}

	lexical, lexicalError := toolResult(t, session.call(3, "confluence.search", map[string]any{
		"query": "apple", "mode": "lexical",
	}))
	if lexicalError {
		t.Fatalf("lexical search must not need matching embeddings: %s", lexical)
	}
}
