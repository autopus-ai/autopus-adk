//go:build unix

package learn

import (
	"os"
	"path/filepath"
	"testing"

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
