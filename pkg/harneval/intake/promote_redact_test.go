package intake

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPromote_UnredactedCandidateText_IsRefusedBeforePublishing pins review
// S6: a person edits the candidate by hand, so promote applies the redactor
// again to the text it publishes (the evidence the link record copies, and the
// draft task's intent, outcome, and status reason) and refuses a candidate
// whose text redaction would still change, writing nothing.
func TestPromote_UnredactedCandidateText_IsRefusedBeforePublishing(t *testing.T) {
	t.Parallel()
	secret := "sk-" + strings.Repeat("Zq9", 8)
	edits := map[string]func(*Candidate){
		"expected":      func(c *Candidate) { c.Expected = "y " + secret },
		"actual":        func(c *Candidate) { c.Actual = secret + " seen" },
		"repro":         func(c *Candidate) { c.Repro = "auto init --token " + secret },
		"intent":        func(c *Candidate) { c.Task.Intent = "hook leaks " + secret },
		"outcome":       func(c *Candidate) { c.Task.Outcome = secret },
		"status reason": func(c *Candidate) { c.Task.Status.Reason = secret },
	}
	for name, edit := range edits {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := newCompletedProject(t)
			editCandidate(t, root, edit)
			before := treeDigest(t, root)

			_, err := runPromote(root, func(r *PromoteRequest) { r.Redactor = maskingRedactor(secret) })

			requirePromoteRefusal(t, err, ReasonCandidateInvalid, DetailUnredactedText)
			assert.Equal(t, before, treeDigest(t, root))
		})
	}
}

func TestPromote_RedactedCandidateText_IsPublishedAsIs(t *testing.T) {
	t.Parallel()
	secret := "sk-" + strings.Repeat("Zq9", 8)
	root := newCompletedProject(t)
	editCandidate(t, root, func(c *Candidate) { c.Actual = "[REDACTED_SECRET] seen" })

	_, err := runPromote(root, func(r *PromoteRequest) { r.Redactor = maskingRedactor(secret) })

	require.NoError(t, err)
	assert.Contains(t, readFile(t, root, promoteLinkAt), `"actual": "[REDACTED_SECRET] seen"`)
}

func TestPromote_WithoutRedactor_IsAProgrammingError(t *testing.T) {
	t.Parallel()
	root := newCompletedProject(t)
	before := treeDigest(t, root)

	_, err := Promote(context.Background(), PromoteRequest{Root: root, CandidateID: promoteCandidateID})

	require.Error(t, err)
	var refusal *RunError
	assert.False(t, errors.As(err, &refusal), "a missing redactor is not a refusal reason")
	assert.Equal(t, before, treeDigest(t, root))
}
