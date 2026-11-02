//go:build darwin || linux

package cli

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// SPEC-DISK-002 review r2: SIGTERM sent to the wrapper reaches the command,
// whose own exit status then becomes the lease's.
func TestDiskLease_ForwardsSIGTERM(t *testing.T) {
	dir := leaseProject(t, "")
	ready := filepath.Join(dir, "ready")
	go func() {
		for i := 0; i < 500; i++ {
			if _, err := os.Stat(ready); err == nil {
				_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}()
	out := runDiskCLI(t, "disk", "lease", "--dir", dir, "--need", "1", "--", "sh", "-c",
		`trap 'exit 9' TERM; touch "$0"; while :; do sleep 0.05; done`, ready)
	assert.Equal(t, 9, out.code, out.stderr)
}
