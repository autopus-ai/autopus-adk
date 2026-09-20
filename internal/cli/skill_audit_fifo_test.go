//go:build unix

package cli

import (
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestSkillAuditRejectsFIFOConfig(t *testing.T) {
	root := t.TempDir()
	if err := syscall.Mkfifo(filepath.Join(root, "autopus.yaml"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := buildSkillAudit(root, "codex"); err == nil || !strings.Contains(err.Error(), "regular file at most") {
		t.Fatalf("FIFO config: %v", err)
	}
}
