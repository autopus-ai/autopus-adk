package codex

import (
	"strings"
	"testing"
)

// The codex worktree-isolation skill is a native body that generate-templates
// never produces, so the disk preflight of SPEC-DISK-001 REQ-DISK-GUIDE-01 is
// pinned here rather than by the template parity test.
func TestCodexWorktreeIsolationSkillBody_CarriesTheDiskPreflight(t *testing.T) {
	t.Parallel()

	body := codexWorktreeIsolationSkillBody()
	for _, token := range []string{"auto disk check --need", "exit 0", "exits 3"} {
		if !strings.Contains(body, token) {
			t.Errorf("codex worktree-isolation body lacks %q", token)
		}
	}
}
