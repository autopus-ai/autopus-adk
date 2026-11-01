package diskspace

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
)

// S2: macOS counts Bavail in f_bsize blocks, and the mount is f_mntonname.
func TestStatfsSpace_DarwinCountsBsizeBlocks(t *testing.T) {
	t.Parallel()
	st := syscall.Statfs_t{Bavail: 37844, Bsize: 4096, Iosize: 1 << 20}
	for i, c := range "/System/Volumes/Data" {
		st.Mntonname[i] = int8(c)
	}

	assert.Equal(t, Space{AvailBytes: 155009024, BlockUnit: 4096, Mount: "/System/Volumes/Data"}, statfsSpace(&st))
}
