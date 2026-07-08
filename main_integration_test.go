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
	"sync"
	"testing"
	"time"
)

var (
	buildBinaryOnce sync.Once
	builtBinaryPath string
	builtBinaryErr  error
)

func TestMCPStdioSmoke(t *testing.T) {
	exePath := buildServerBinary(t)
	missingDBPath := filepath.Join(t.TempDir(), "missing.db")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, exePath)
	cmd.Env = append(os.Environ(), "CONFLUENCE_INDEX_DB="+missingDBPath)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("failed to get stdin pipe: %v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("failed to get stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("failed to get stderr pipe: %v", err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start MCP server: %v", err)
	}

	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		_, _ = io.ReadAll(stderr)
	}()

	reader := bufio.NewReader(stdout)

	sendMCP(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2025-03-26",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "go-test-smoke",
				"version": "1.0.0",
			},
		},
	})

	initMsg := readMCP(t, reader, 8*time.Second)
	assertResponseID(t, initMsg, 1)

	sendMCP(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	})

	sendMCP(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
		"params":  map[string]any{},
	})

	listMsg := readMCP(t, reader, 8*time.Second)
	assertResponseID(t, listMsg, 2)
	assertHasTool(t, listMsg, "confluence.search")

	sendMCP(t, stdin, map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name": "confluence.search",
			"arguments": map[string]any{
				"query": "smoke test query",
				"mode":  "hybrid",
				"topK":  3,
			},
		},
	})

	callMsg := readMCP(t, reader, 8*time.Second)
	assertResponseID(t, callMsg, 3)
}

func buildServerBinary(t *testing.T) string {
	t.Helper()

	buildBinaryOnce.Do(func() {
		exeName := "confluence2md-mcp-test"
		if runtime.GOOS == "windows" {
			exeName += ".exe"
		}

		cacheRoot, err := os.UserCacheDir()
		if err != nil {
			cacheRoot = os.TempDir()
		}
		cacheDir := filepath.Join(cacheRoot, "confluence2md-mcp")
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			builtBinaryErr = fmt.Errorf("create cache dir: %w", err)
			return
		}

		exePath := filepath.Join(cacheDir, exeName)
		cmd := exec.Command("go", "build", "-o", exePath, ".")
		cmd.Dir = "."
		out, err := cmd.CombinedOutput()
		if err != nil {
			builtBinaryErr = fmt.Errorf("build test binary: %w\n%s", err, string(out))
			return
		}

		builtBinaryPath = exePath
	})

	if builtBinaryErr != nil {
		t.Fatalf("failed to build test binary: %v", builtBinaryErr)
	}

	return builtBinaryPath
}

func sendMCP(t *testing.T, w io.Writer, payload map[string]any) {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}
	if _, err := w.Write(append(body, '\n')); err != nil {
		t.Fatalf("failed to write payload: %v", err)
	}
}

func readMCP(t *testing.T, r *bufio.Reader, timeout time.Duration) []byte {
	t.Helper()

	type result struct {
		msg []byte
		err error
	}
	ch := make(chan result, 1)

	go func() {
		msg, err := readMCPBlocking(r)
		ch <- result{msg: msg, err: err}
	}()

	select {
	case res := <-ch:
		if res.err != nil {
			t.Fatalf("failed to read MCP message: %v", res.err)
		}
		return res.msg
	case <-time.After(timeout):
		t.Fatalf("timed out after %s waiting for MCP response", timeout)
		return nil
	}
}

func readMCPBlocking(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	for len(line) > 0 && (line[len(line)-1] == '\n' || line[len(line)-1] == '\r') {
		line = line[:len(line)-1]
	}
	return line, nil
}

func assertResponseID(t *testing.T, msg []byte, expected int) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(msg, &payload); err != nil {
		t.Fatalf("invalid JSON response: %v\n%s", err, string(msg))
	}

	id, ok := payload["id"].(float64)
	if !ok {
		t.Fatalf("response missing numeric id: %s", string(msg))
	}
	if int(id) != expected {
		t.Fatalf("unexpected id, got %d want %d: %s", int(id), expected, string(msg))
	}
}

func assertHasTool(t *testing.T, msg []byte, toolName string) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(msg, &payload); err != nil {
		t.Fatalf("invalid JSON response: %v\n%s", err, string(msg))
	}

	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("tools/list response missing result: %s", string(msg))
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("tools/list response missing tools array: %s", string(msg))
	}

	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := tool["name"].(string); ok && name == toolName {
			return
		}
	}

	t.Fatalf("tool %q not found in tools/list response: %s", toolName, string(msg))
}
