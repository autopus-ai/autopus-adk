package templates_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/spec"
)

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
