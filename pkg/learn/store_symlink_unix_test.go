//go:build unix

package learn

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStore_SymlinkedStore_IsRefusedNotWrittenThrough pins review S7: a
// committed pipeline.jsonl that is a symlink (here to ../../.env) is refused
// by every writer, and the file it points at keeps its bytes.
func TestStore_SymlinkedStore_IsRefusedNotWrittenThrough(t *testing.T) {
	t.Parallel()
	writers := map[string]func(*Store) error{
		"append":      func(s *Store) error { return s.AppendAtomic(EntryTypeGateFail, RecordOpts{Pattern: "p"}) },
		"prune":       func(s *Store) error { _, err := Prune(s, 30); return err },
		"reuse count": func(s *Store) error { return s.UpdateReuseCount("L-001") },
	}
	for name, write := range writers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			target := filepath.Join(dir, ".env")
			env := `{"id":"L-001","timestamp":"2020-01-01T00:00:00Z","type":"gate_fail","pattern":"old"}` + "\nAPI_TOKEN=keep-me\n"
			require.NoError(t, os.WriteFile(target, []byte(env), 0o600))
			store, err := NewStore(dir)
			require.NoError(t, err)
			require.NoError(t, os.Symlink(filepath.Join("..", "..", ".env"), store.path))

			err = write(store)

			require.ErrorIs(t, err, errStoreNotRegular)
			data, readErr := os.ReadFile(target)
			require.NoError(t, readErr)
			assert.Equal(t, env, string(data), "the symlink target keeps its bytes")
			info, lstatErr := os.Lstat(store.path)
			require.NoError(t, lstatErr)
			assert.Equal(t, os.ModeSymlink, info.Mode().Type(), "the symlink itself is left in place")
		})
	}
}

// storeOps is every store operation that opens the store file.
var storeOps = map[string]func(*Store) error{
	"read tolerant": func(s *Store) error { _, _, err := s.ReadTolerant(); return err },
	"next id":       func(s *Store) error { _, err := s.NextID(); return err },
	"append atomic": func(s *Store) error { return s.AppendAtomic(EntryTypeGateFail, RecordOpts{Pattern: "p"}) },
	"append": func(s *Store) error {
		return s.Append(LearningEntry{ID: "L-009", Type: EntryTypeGateFail, Pattern: "p"})
	},
	"prune":       func(s *Store) error { _, err := Prune(s, 30); return err },
	"reuse count": func(s *Store) error { return s.UpdateReuseCount("L-001") },
}

// returnsWithin runs op and fails the test when it has not returned after
// five seconds: a read that follows a FIFO or a device never ends.
func returnsWithin(t *testing.T, op func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- op() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("the store operation did not return: it opened a non-regular store")
		return nil
	}
}

// TestStore_NonRegularStore_IsRefusedBeforeReading pins review round 2
// finding 5: every operation, readers included, refuses a store that is a
// FIFO or a symlink before it reads a byte. A FIFO blocked the read for good,
// a link to /dev/urandom read without end, and a link to ../../.env returned
// that file's lines as skipped entries.
func TestStore_NonRegularStore_IsRefusedBeforeReading(t *testing.T) {
	t.Parallel()
	stores := map[string]func(t *testing.T, path string){
		"fifo": func(t *testing.T, path string) { require.NoError(t, syscall.Mkfifo(path, 0o644)) },
		"device link": func(t *testing.T, path string) {
			if _, err := os.Stat("/dev/urandom"); err != nil {
				t.Skip("no /dev/urandom")
			}
			require.NoError(t, os.Symlink("/dev/urandom", path))
		},
		"decoy link": func(t *testing.T, path string) {
			decoy := filepath.Join(filepath.Dir(path), "..", "..", ".env")
			require.NoError(t, os.WriteFile(decoy, []byte("API_TOKEN=keep-me\n"), 0o600))
			require.NoError(t, os.Symlink(filepath.Join("..", "..", ".env"), path))
		},
	}
	for kind, plant := range stores {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			for name, op := range storeOps {
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					store, err := NewStore(t.TempDir())
					require.NoError(t, err)
					plant(t, store.path)

					err = returnsWithin(t, func() error { return op(store) })

					require.ErrorIs(t, err, errStoreNotRegular)
				})
			}
		})
	}
}

// TestStore_SymlinkedStoreDirectory_IsRefused pins review round 2 finding 5:
// a committed .autopus or .autopus/learnings that is a symlink would carry
// every read and write to the directory it names. NewStore refuses it without
// creating anything there, and a store whose directory became a link after
// NewStore refuses every operation.
func TestStore_SymlinkedStoreDirectory_IsRefused(t *testing.T) {
	t.Parallel()
	const entry = `{"id":"L-001","timestamp":"2020-01-01T00:00:00Z","type":"gate_fail","pattern":"elsewhere"}` + "\n"
	links := map[string]string{"autopus dir": ".autopus", "learnings dir": filepath.Join(".autopus", "learnings")}
	for label, linked := range links {
		t.Run("new store with "+label, func(t *testing.T) {
			t.Parallel()
			dir, elsewhere := t.TempDir(), t.TempDir()
			require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, linked)), 0o755))
			require.NoError(t, os.Symlink(elsewhere, filepath.Join(dir, linked)))

			_, err := NewStore(dir)

			require.ErrorIs(t, err, errStoreDirUnsafe)
			names, readErr := os.ReadDir(elsewhere)
			require.NoError(t, readErr)
			assert.Empty(t, names, "nothing is created through the link")
		})
		for name, op := range storeOps {
			t.Run(name+" after "+label+" became a link", func(t *testing.T) {
				t.Parallel()
				dir, elsewhere := t.TempDir(), t.TempDir()
				store, err := NewStore(dir)
				require.NoError(t, err)
				require.NoError(t, os.MkdirAll(filepath.Join(elsewhere, "learnings"), 0o755))
				planted := filepath.Join(elsewhere, "learnings", "pipeline.jsonl")
				if label == "learnings dir" {
					planted = filepath.Join(elsewhere, "pipeline.jsonl")
				}
				require.NoError(t, os.WriteFile(planted, []byte(entry), 0o644))
				require.NoError(t, os.RemoveAll(filepath.Join(dir, linked)))
				require.NoError(t, os.Symlink(elsewhere, filepath.Join(dir, linked)))

				err = returnsWithin(t, func() error { return op(store) })

				require.ErrorIs(t, err, errStoreDirUnsafe)
				data, readErr := os.ReadFile(planted)
				require.NoError(t, readErr)
				assert.Equal(t, entry, string(data), "the store the link names keeps its bytes")
			})
		}
	}
}
