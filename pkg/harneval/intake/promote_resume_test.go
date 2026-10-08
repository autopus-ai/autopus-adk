package intake

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// crashAt returns an interrupt seam that stops the promotion after step.
func crashAt(step int) func(int) error {
	return func(current int) error {
		if current == step {
			return errPromoteCrash
		}
		return nil
	}
}

// interruptedPromotion leaves root as a promotion stopped after step leaves it.
func interruptedPromotion(step int) func(t *testing.T, root string) {
	return func(t *testing.T, root string) {
		t.Helper()
		_, err := runPromote(root, func(r *PromoteRequest) { r.interrupt = crashAt(step) })
		require.ErrorIs(t, err, errPromoteCrash)
	}
}

func TestPromote_S6RerunFromEachInterruption_ConvergesToTheOneShotFiles(t *testing.T) {
	t.Parallel()
	oneShot := newCompletedProject(t)
	_, err := runPromote(oneShot, nil)
	require.NoError(t, err)
	want := treeDigest(t, oneShot)
	cases := []struct {
		name   string
		result string
		leave  func(t *testing.T, root string)
	}{
		// A crash inside step 10, after the temp file was written and before
		// Root.Link, leaves the temp alone; the state is staged directly
		// because the crash point lies inside the publication helper.
		{"temp written in step 10", PromoteResultPromoted, func(t *testing.T, root string) {
			writeFile(t, root, SurfaceTaskDir+"/."+promoteTaskID+".json.tmp-0123456789abcdef", `{"schema_version": "harn`)
		}},
		{"link made in step 10, temp not yet removed", PromoteResultAlreadyActive, func(t *testing.T, root string) {
			writeFile(t, root, promoteTaskAt, promotedTask)
			writeFile(t, root, SurfaceTaskDir+"/."+promoteTaskID+".json.tmp-0123456789abcdef", promotedTask)
		}},
		{"between steps 10 and 11", PromoteResultAlreadyActive, interruptedPromotion(stepTaskPublished)},
		{"between steps 11 and 12", PromoteResultAlreadyActive, interruptedPromotion(stepPostChecked)},
		{"temp written in step 12", PromoteResultAlreadyActive, func(t *testing.T, root string) {
			interruptedPromotion(stepPostChecked)(t, root)
			writeFile(t, root, PromotedDir+"/."+promoteTaskID+".json.tmp-0123456789abcdef", "{")
		}},
		{"between steps 12 and 13", PromoteResultAlreadyActive, interruptedPromotion(stepLinkPublished)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given a promotion interrupted at this point.
			root := newCompletedProject(t)
			tc.leave(t, root)

			// When promote runs again.
			result, err := runPromote(root, nil)

			// Then it finishes with the one-shot files and no temp file.
			require.NoError(t, err)
			assert.Equal(t, tc.result, result.Result)
			assert.Equal(t, OutcomePass, result.CurrentOutcome)
			assert.Equal(t, want, treeDigest(t, root))
			assert.Empty(t, tempNames(t, root))
		})
	}
}

func TestPromote_RerunAfterSuccess_IsCandidateMissingAndChangesNothing(t *testing.T) {
	t.Parallel()
	root := newCompletedProject(t)
	_, err := runPromote(root, nil)
	require.NoError(t, err)
	before := treeDigest(t, root)

	_, err = runPromote(root, nil)

	requirePromoteRefusal(t, err, ReasonCandidateMissing, "")
	assert.Equal(t, before, treeDigest(t, root))
}
