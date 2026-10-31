package diskspace

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

const mountsTable = `sysfs /sys sysfs rw,nosuid 0 0
/dev/root / ext4 rw,relatime 0 0
tmpfs /tmp tmpfs rw 0 0
/dev/sdb1 /mnt/data ext4 rw 0 0
/dev/sdb2 /mnt/data/cache ext4 rw 0 0
/dev/sdc1 /mnt/with\040space ext4 rw 0 0
/dev/sdd1 /mnt/database ext4 rw 0 0
`

// REQ-DISK-FREE-01 on linux: the mount is the longest mount point that holds
// the path, compared by directory, never by string prefix alone.
func TestLongestMount_PicksTheDeepestHoldingMount(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]string{
		"/home/dev/project":         "/",
		"/tmp/claude-1000/x":        "/tmp",
		"/tmp":                      "/tmp",
		"/mnt/data/repo":            "/mnt/data",
		"/mnt/data/cache/go-build":  "/mnt/data/cache",
		"/mnt/database/x":           "/mnt/database",
		"/mnt/datab":                "/",
		"/mnt/with space/worktrees": "/mnt/with space",
	} {
		assert.Equal(t, want, longestMount([]byte(mountsTable), path), path)
	}
	assert.Equal(t, "/", longestMount(nil, "/anything"), "an unreadable table keeps the root")
}

func TestUnescapeMount_DecodesKernelOctalEscapes(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "/a b\tc\nd\\e", unescapeMount(`/a\040b\011c\012d\134e`))
	assert.Equal(t, `/plain`, unescapeMount(`/plain`))
	assert.Equal(t, `/trailing\04`, unescapeMount(`/trailing\04`), "a short escape stays literal")
	assert.Equal(t, `/bad\9xy`, unescapeMount(`/bad\9xy`), "a non-octal escape stays literal")
}
