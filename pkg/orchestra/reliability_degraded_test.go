package orchestra

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// S7: when receipt persistence fails, recordPrompt still returns "" and exactly one
// "reliability" warning is emitted per store, regardless of how many records fail.
func TestReliabilityStore_PersistenceFailure_WarnsOncePerStore(t *testing.T) {
	// Make the store directory un-creatable: its parent is a regular file.
	parent := filepath.Join(t.TempDir(), "not-a-dir")
	if err := os.WriteFile(parent, []byte("x"), 0o600); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	store := &reliabilityStore{runID: "run-degraded", dir: filepath.Join(parent, "sub")}

	var first, second string
	logged := captureLog(t, func() {
		first = store.recordPrompt(promptReceipt("run-degraded", "claude", "pipe", "prompt one", 1, "pass", ""))
		second = store.recordPrompt(promptReceipt("run-degraded", "gemini", "pipe", "prompt two", 1, "pass", ""))
	})

	if first != "" || second != "" {
		t.Fatalf("expected empty receipt paths on failure, got %q and %q", first, second)
	}
	if n := strings.Count(logged, "reliability"); n != 1 {
		t.Fatalf("expected exactly one 'reliability' warning, got %d: %q", n, logged)
	}
}

// S7: a writable store returns a non-empty receipt path and emits no warning.
func TestReliabilityStore_WritableStore_NoWarning(t *testing.T) {
	store := &reliabilityStore{runID: "run-ok", dir: filepath.Join(t.TempDir(), "artifacts")}

	var path string
	logged := captureLog(t, func() {
		path = store.recordPrompt(promptReceipt("run-ok", "claude", "pipe", "hello", 1, "pass", ""))
	})

	if path == "" {
		t.Fatalf("expected non-empty receipt path on a writable store")
	}
	if strings.Contains(logged, "reliability") {
		t.Fatalf("writable store must not emit a reliability warning, got: %q", logged)
	}
	if store.degraded {
		t.Fatalf("writable store must not be marked degraded")
	}
}

// captureLog redirects the standard logger to a buffer for the duration of fn.
func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prevOut := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(prevOut)
		log.SetFlags(prevFlags)
	}()
	fn()
	return buf.String()
}
