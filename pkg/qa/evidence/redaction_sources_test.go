package evidence

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSecretDetectorSources_ListsRedactTextRegexesInOrder pins the accessor to
// the regexes RedactText applies, in application order, so pkg/secretscan's
// drift test compares its table against what RedactText really runs.
func TestSecretDetectorSources_ListsRedactTextRegexesInOrder(t *testing.T) {
	t.Parallel()
	applied := append([]*regexp.Regexp{}, secretPatterns...)
	applied = append(applied,
		sensitiveAssignmentRe, sensitiveFlagValueRe, jsonSensitiveRe,
		credentialURLRe, secretQueryRe, privateNoteRe, jsonPrivateNoteRe,
		userPathRe, windowsUserPathRe,
	)
	want := make([]string, 0, len(applied))
	for _, re := range applied {
		want = append(want, re.String())
	}

	got := SecretDetectorSources()
	require.Len(t, got, 14, "5 secret patterns plus 9 value detectors")
	assert.Equal(t, want, got)

	got[0] = "mutated"
	assert.Equal(t, want, SecretDetectorSources(), "each call returns a fresh slice")
}
