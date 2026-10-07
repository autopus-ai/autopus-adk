package editguard

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// Fixture T of acceptance.md and the content S6 rewrites it to.
const (
	tRel        = "internal/foo/foo_repro_test.go"
	tContent    = "package foo\n"
	tHash       = "1b63a92736f126a00f521c0ef804e67d0cf949b5ff790d6d4c3a4b7681da8d21"
	weakContent = "package foo // weakened\n"
	weakHash    = "e4303e1b170ed51f64829282b577efdb87a5334d86f2b07f9185084fea2f6b96"
	uRel        = "internal/foo/u_test.go"
	xRel        = "internal/foo/x_test.go"
)

var (
	t0          = time.Date(2026, 10, 7, 9, 0, 0, 0, time.UTC)
	errInjected = errors.New("injected fault")
)

// testClock is a settable clock that goroutines may read while a test sets it.
type testClock struct{ unixNano atomic.Int64 }

func newClock(at time.Time) *testClock {
	c := &testClock{}
	c.Set(at)
	return c
}

func (c *testClock) Now() time.Time   { return time.Unix(0, c.unixNano.Load()).UTC() }
func (c *testClock) Set(at time.Time) { c.unixNano.Store(at.UnixNano()) }

// lockProject returns fixture R's root with T, u_test.go, and x_test.go.
func lockProject(t *testing.T) string {
	t.Helper()
	root := newProject(t)
	writeFile(t, root, tRel, tContent)
	writeFile(t, root, uRel, "package foo\n\n// u\n")
	writeFile(t, root, xRel, "package foo\n\n// x\n")
	return root
}

func openTestStore(t *testing.T, root string, clock *testClock) *Store {
	t.Helper()
	store, err := OpenStore(root)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	store.Now = clock.Now
	return store
}

func mustLock(t *testing.T, store *Store, paths ...string) {
	t.Helper()
	if err := store.Lock(paths, 0); err != nil {
		t.Fatalf("Lock(%q): %v", paths, err)
	}
}

func listPaths(t *testing.T, store *Store) []string {
	t.Helper()
	entries, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.Path)
	}
	return out
}

// storeFiles returns the record files of root's store, skipping the store lock
// and temp files the way readers do.
func storeFiles(t *testing.T, root string) map[string]string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(FixLocksDir))
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}
	}
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[entry.Name()] = string(data)
	}
	return out
}

// recordHashOf returns the sha256 the store's record for rel holds.
func recordHashOf(t *testing.T, root, rel string) string {
	t.Helper()
	for _, data := range storeFiles(t, root) {
		var rec LockRecord
		if json.Unmarshal([]byte(data), &rec) == nil && rec.Path == rel {
			return rec.SHA256
		}
	}
	t.Fatalf("no record for %s", rel)
	return ""
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
