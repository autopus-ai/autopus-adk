//go:build unix

package harneval

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadSignerInput_FIFOResult_IsRefusedWithoutBlocking: a FIFO in place of
// an oracle result is no regular file; opening it would block forever.
func TestLoadSignerInput_FIFOResult_IsRefusedWithoutBlocking(t *testing.T) {
	t.Parallel()
	dir := writeLiveResult(t)
	require.NoError(t, syscall.Mkfifo(filepath.Join(dir, OracleResultsDir, "zz-fifo.json"), 0o644))
	done := make(chan error, 1)

	go func() {
		_, err := LoadSignerInput(dir)
		done <- err
	}()

	select {
	case err := <-done:
		requireInvalid(t, err, DetailReadFailed)
		assert.Contains(t, err.Error(), "oracle-results/zz-fifo.json is not a regular file")
	case <-time.After(5 * time.Second):
		t.Fatal("LoadSignerInput blocked on a FIFO")
	}
}

// TestLoadSignerInput_UnreadableEntry_IsReadFailed: a document or the oracle
// results directory that cannot be opened is refused, never read as absent.
func TestLoadSignerInput_UnreadableEntry_IsReadFailed(t *testing.T) {
	t.Parallel()
	if os.Geteuid() == 0 {
		t.Skip("permission bits do not bind root")
	}
	for _, rel := range []string{ProtocolFile, OracleResultsDir} {
		dir := writeLiveResult(t)
		path := filepath.Join(dir, rel)
		require.NoError(t, os.Chmod(path, 0))
		t.Cleanup(func() { _ = os.Chmod(path, 0o755) })

		_, err := LoadSignerInput(dir)

		requireInvalid(t, err, DetailReadFailed)
		assert.Contains(t, err.Error(), "("+rel+")")
	}
	file := filepath.Join(t.TempDir(), "result.zip")
	require.NoError(t, os.WriteFile(file, []byte("PK"), 0o644))
	_, err := LoadSignerInput(file)
	requireInvalid(t, err, DetailReadFailed)
	assert.Contains(t, err.Error(), "("+ProtocolFile+")")
}
