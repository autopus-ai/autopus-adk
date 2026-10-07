//go:build !windows

package editguard

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// H2: a FIFO named like a manifest is skipped without being opened, so it
// can neither block the guard nor drop the manifest stage.
func TestManifestStage_FIFONamedLikeAManifest_IsSkippedWithoutBlocking(t *testing.T) {
	t.Parallel()
	root := fixtureR(t)
	if err := syscall.Mkfifo(filepath.Join(root, ".autopus", "zzz-manifest.json"), 0o644); err != nil {
		t.Skipf("mkfifo unavailable: %v", err)
	}
	done := make(chan manifestStage, 1)
	go func() { done <- loadManifestStage(root, false) }()
	select {
	case stage := <-done:
		if _, ok := stage.generated(skillRel); stage.fault != "" || !ok {
			t.Fatalf("stage = fault %q, skill protected %v; want the FIFO skipped", stage.fault, ok)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("reading the manifests blocked on the FIFO")
	}
}
