//go:build unix

package agentprobe

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCodexRPCSubprocessFixture(t *testing.T) {
	if os.Getenv("AUTOPUS_CODEX_RPC_FIXTURE") != "1" {
		return
	}
	scanner := bufio.NewScanner(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var frame struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		if json.Unmarshal(scanner.Bytes(), &frame) != nil {
			os.Exit(2)
		}
		if frame.Method == "initialize" {
			_ = encoder.Encode(map[string]any{"method": "thread/started", "params": map[string]string{"threadId": "fixture"}})
			_ = encoder.Encode(map[string]any{"id": frame.ID, "result": map[string]string{"userAgent": "fixture"}})
		}
	}
	os.Exit(0)
}

func TestCodexRPCBoundedSubprocessRoundTrip(t *testing.T) {
	t.Setenv("AUTOPUS_CODEX_RPC_FIXTURE", "1")
	root := t.TempDir()
	exe := filepath.Join(root, "codex-fixture")
	binary := strings.ReplaceAll(os.Args[0], "'", "'\\''")
	script := "#!/bin/sh\nexec '" + binary + "' -test.run='^TestCodexRPCSubprocessFixture$'\n"
	if err := os.WriteFile(exe, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	rpc, err := startCodexRPC(ctx, exe, root)
	if err != nil {
		t.Fatal(err)
	}
	defer rpc.close()
	raw, err := rpc.call(ctx, "initialize", map[string]any{})
	if err != nil || !strings.Contains(string(raw), "fixture") {
		t.Fatalf("%s %v", raw, err)
	}
	event, err := rpc.next(ctx)
	if err != nil || event.Method != "thread/started" {
		t.Fatalf("%+v %v", event, err)
	}
	if err := rpc.notify("initialized", map[string]any{}); err != nil {
		t.Fatal(err)
	}
}
