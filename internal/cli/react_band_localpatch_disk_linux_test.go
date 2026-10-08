package cli

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// S3 block unit on Linux: Bavail counts f_frsize blocks, or f_bsize blocks
// when f_frsize is 0.
func TestLPStatfsUnit_Linux_FrsizeThenBsize(t *testing.T) {
	t.Parallel()
	assert.Equal(t, uint64(4096), lpStatfsUnit(&syscall.Statfs_t{Frsize: 4096, Bsize: 1 << 20}))
	assert.Equal(t, uint64(4096), lpStatfsUnit(&syscall.Statfs_t{Bsize: 4096}))
}
