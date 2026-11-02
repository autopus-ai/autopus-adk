package templates_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/spec"
)

// SPEC-REVIEWPOLICY-001 S6 (REQ-RP-POL-09): every surface that tells an agent
// how to write, review, or plan a lean SPEC states the lean floor and the lean
// allowed omissions exactly as the review policy in pkg/spec lists them, so no
// surface can ask for a section the policy lets a lean SPEC omit. The two
// lines are rendered from the policy, not copied, so a policy change fails
// this test until every surface follows it.
func TestLeanPolicySurfaceParity(t *testing.T) {
	t.Parallel()

	lean, ok := spec.ReviewPolicyFor(spec.ReviewProfileLean)
	require.True(t, ok)
	floor := "Lean floor: " + renderPolicySections(lean, lean.RequiredSections)
	omissions := "Lean allowed omissions: " + renderPolicySections(lean, lean.OmittedSections)

	root := templateRoot()
	paths := []string{
		filepath.Join(root, "..", "content", "agents", "spec-writer.md"),
		filepath.Join(root, "..", "content", "skills", "spec-review.md"),
		filepath.Join(root, "..", "content", "rules", "spec-quality.md"),
		filepath.Join(root, "codex", "agents", "spec-writer.toml.tmpl"),
		filepath.Join(root, "gemini", "agents", "spec-writer.md.tmpl"),
		filepath.Join(root, "codex", "skills", "spec-review.md.tmpl"),
		filepath.Join(root, "gemini", "skills", "spec-review", "SKILL.md.tmpl"),
		filepath.Join(root, "claude", "commands", "auto-workflows.md.tmpl"),
		filepath.Join(root, "codex", "skills", "auto-plan.md.tmpl"),
		filepath.Join(root, "codex", "prompts", "auto-plan.md.tmpl"),
		filepath.Join(root, "gemini", "skills", "auto-plan", "SKILL.md.tmpl"),
	}
	for _, path := range paths {
		path := path
		t.Run(filepath.Base(filepath.Dir(path))+"/"+filepath.Base(path), func(t *testing.T) {
			t.Parallel()
			body, err := os.ReadFile(path)
			require.NoError(t, err)
			text := string(body)
			assert.Contains(t, text, floor, path)
			assert.Contains(t, text, omissions, path)
			floorLine := lineContaining(text, floor)
			for _, omitted := range lean.OmittedHeadings() {
				assert.NotContains(t, floorLine, "`"+omitted+"`", "%s lean floor asks for omitted %s", path, omitted)
			}
		})
	}
}

// The judge template carries the canonical verdict rule lines verbatim; the
// renderer lives in pkg/orchestra, so this parity check is what keeps them in
// step with the reviewer prompt and the runtime.
func TestSpecReviewJudgeTemplateCarriesCanonicalVerdictRules(t *testing.T) {
	t.Parallel()

	body, err := os.ReadFile(filepath.Join(templateRoot(), "shared", "spec-review-judge.md.tmpl"))
	require.NoError(t, err)
	for _, line := range spec.ReviewVerdictRuleLines() {
		assert.Contains(t, string(body), line+"\n")
	}
	assert.NotContains(t, string(body), "Otherwise, any accepted critical or major finding requires")
	// REQ-RP-POL-04: the judge learns the only way to clear a reviewer's
	// REJECT vote or checklist FAIL.
	assert.Contains(t, string(body), "`scope_ref` set to `verdict` for the REJECT vote or to the checklist id")
}

func renderPolicySections(policy spec.ReviewPolicy, sections map[string][]string) string {
	var parts []string
	for _, document := range policy.Documents {
		headings := sections[document]
		if len(headings) == 0 {
			continue
		}
		quoted := make([]string, len(headings))
		for i, heading := range headings {
			quoted[i] = "`" + heading + "`"
		}
		parts = append(parts, document+" "+strings.Join(quoted, ", "))
	}
	return strings.Join(parts, "; ") + "."
}

func lineContaining(text, needle string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, needle) {
			return line
		}
	}
	return ""
}
