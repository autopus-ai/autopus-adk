//go:build unix

package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// RR-7 through the production adapter: the Patch Policy reads the base
// manifests with git cat-file --batch through the hardened runner and its
// allowlist, so a patch to the sources of a proc-macro crate at the base
// ends failed:path_denied, while a patch elsewhere in that base is done.
func TestLocalPatchPatch_ProcMacroCrateAtTheBase(t *testing.T) {
	withMacros := func(f *lpFixture) {
		f.write("macros/Cargo.toml", "[package]\nname = \"macros\"\nversion = \"0.1.0\"\n\n[lib]\nproc-macro = true\n")
		f.write("macros/src/lib.rs", "pub fn m() {}\n")
		f.git(f.repo, "add", "-A")
		f.git(f.repo, "-c", "core.hooksPath=/dev/null", "commit", "-q", "-m", "macros")
		f.git(f.repo, "push", "-q", "origin", "main")
		f.base = strings.TrimSpace(f.git(f.repo, "rev-parse", "HEAD"))
	}
	macroDiff := "diff --git a/macros/src/lib.rs b/macros/src/lib.rs\n--- a/macros/src/lib.rs\n+++ b/macros/src/lib.rs\n" +
		"@@ -1 +1,2 @@\n pub fn m() {}\n+pub fn n() {}\n"
	for name, tc := range map[string]struct{ diff, want string }{
		"proc-macro source": {macroDiff, "failed:" + healthband.PatchCodePathDenied},
		"ordinary source":   {lpFooDiff, healthband.ClaimDone},
	} {
		t.Run(name, func(t *testing.T) {
			w := newLPPatchWorld(t, lpS13Harness(), withMacros)
			w.fake.setStream(t, lpStream(lpInit55, lpAssistant55, lpResult(lpReplyWith(tc.diff))))
			assert.Equal(t, tc.want, w.run(t).Status)
		})
	}
}
