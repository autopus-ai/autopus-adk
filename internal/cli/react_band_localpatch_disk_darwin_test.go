package cli

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// S3 block unit on macOS: Bavail counts f_bsize blocks.
func TestLPStatfsUnit_Darwin_Bsize(t *testing.T) {
	t.Parallel()
	assert.Equal(t, uint64(4096), lpStatfsUnit(&syscall.Statfs_t{Bsize: 4096, Iosize: 1 << 20}))
}
