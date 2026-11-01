package diskspace

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// S2: Linux counts Bavail in f_frsize blocks, f_bsize when f_frsize is 0.
func TestStatfsSpace_LinuxCountsFrsizeBlocks(t *testing.T) {
	t.Parallel()
	assert.Equal(t, Space{AvailBytes: 38752256, BlockUnit: 1024, Mount: "/"},
		statfsSpace(&syscall.Statfs_t{Bavail: 37844, Frsize: 1024, Bsize: 4096}, "/"))
	assert.Equal(t, Space{AvailBytes: 155009024, BlockUnit: 4096, Mount: "/data"},
		statfsSpace(&syscall.Statfs_t{Bavail: 37844, Frsize: 0, Bsize: 4096}, "/data"))
}
