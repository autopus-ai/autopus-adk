package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// SPEC-SIGMABAND-002 REQ-13 / S11: the auto react band help text and
// docs/health-band.md state the local patch boundary in the same sentences,
// and CHANGELOG.md names the keys, the location, and the reviewer warning.

// bandLocalPatchDocRequired are the S11 statements, spelled as
// docs/health-band.md spells them, so the help cannot drift from the docs.
var bandLocalPatchDocRequired = []string{
	"health_band.allow_local_patch",
	"health_band.local_patch_provider",
	"With the default configuration the command never changes a git ref, a worktree, or GitHub state",
	"the artifacts live in autopus/local-patches under the user cache directory",
	"band never pushes, fetches, or opens a pull request: a human reviews the patch file and pushes the branch by hand.",
	"Reviewer warning: this local branch holds a patch that an AI model derived from untrusted CI logs and that " +
		"nothing has run. Read the whole patch file before you open the worktree in an IDE, run any command or agent " +
		"in it, or push the branch, because repository hooks and tool configuration files run on checkout, commit, " +
		"and build, and the edit guard does not cover that worktree.",
	"This patch was derived by an AI model from untrusted CI logs; read the whole patch file before running anything.",
	"Only a subprocess claude provider can run a confined diagnosis.",
	"band runs the claude named by health_band.local_patch_provider as a CLI subprocess whatever its orchestra " +
		"backend, while orchestra keeps that backend",
	"The expected deployment is the claude CLI signed in with a Claude subscription (claude auth login); it needs no API key.",
	"An API-key-only deployment exports ANTHROPIC_API_KEY for the band run instead.",
	"Upgrade every auto binary that reads this autopus.yaml before you set health_band.allow_local_patch or " +
		"health_band.local_patch_provider: a binary without SPEC-SIGMABAND-002 rejects a file that sets either key.",
}

func TestReactBandHelp_LocalPatchStatementsMatchTheDocs(t *testing.T) {
	t.Parallel()
	help, doc := flowText(reactBandLong), flowText(readRepoFile(t, "docs", "health-band.md"))
	for _, want := range bandLocalPatchDocRequired {
		assert.Contains(t, help, want, "auto react band --help")
		assert.Contains(t, doc, want, "docs/health-band.md")
	}
}

// The diagnosis-only statements of SPEC-SIGMABAND-001 hold only while the
// flag is off, so the help qualifies each one with it.
func TestReactBandHelp_DiagnosisOnlyStatementsNameTheFlag(t *testing.T) {
	t.Parallel()
	help := flowText(reactBandLong)
	assert.Contains(t, help, "Tier 3 is diagnosis-only while health_band.allow_local_patch is false, the default:")
	assert.NotContains(t, help, "Tier 3 is diagnosis-only:", "an unqualified diagnosis-only sentence")
	assert.NotContains(t, help, "It never changes a git ref", "an unqualified no-change sentence")
	assert.Contains(t, help, "an episode that opens at tier 3 can also get one local patch outside the repository")
	assert.Contains(t, help, "unavailable(provider_unconfined)")
	assert.Contains(t, help, "local_patches[]")
	assert.Contains(t, help, "git worktree remove --force --force <path>")
}

func TestReactBandHelp_ChangelogNamesTheLocalPatchBoundary(t *testing.T) {
	t.Parallel()
	changelog := flowText(readRepoFile(t, "CHANGELOG.md"))
	for _, want := range []string{
		"health_band.allow_local_patch", "health_band.local_patch_provider", "autopus/local-patches",
		bandLocalPatchDocRequired[5], "claude auth login", "local_patches[]",
	} {
		assert.Contains(t, changelog, want, "CHANGELOG.md")
	}
	assert.False(t, strings.Contains(changelog, "allow_draft_pr: true"), "no draft PR switch is documented")
}
