package intake

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/harneval"
)

func TestPromote_CurrentOutcome_EvaluatesThePromotedTaskAlone(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, outcome, reason string
		run                   harneval.RunOptions
	}{
		{"assertion holds", OutcomePass, "", promoteRun(".codex/hooks.json")},
		{"assertion fails", OutcomeFail, "", promoteRun(".codex/config.toml")},
		{"templates stale", OutcomeNotEvaluated, harneval.ReasonTemplatesStale, func() harneval.RunOptions {
			run := promoteRun(".codex/hooks.json")
			run.StaleCheck = func(string) ([]string, error) { return []string{"templates/codex/x.tmpl"}, nil }
			return run
		}()},
		{"adapter error", OutcomeNotEvaluated, harneval.ReasonGenerationFailed, func() harneval.RunOptions {
			run := promoteRun()
			run.Adapters = func(root string, _ harneval.Pins, _ []byte) []adapter.PlatformAdapter {
				return []adapter.PlatformAdapter{promoteSurface{name: "codex", root: root, err: errors.New("broken template")}}
			}
			return run
		}()},
		{"surface unreadable after generation", OutcomeNotEvaluated, harneval.ReasonGenerationFailed, func() harneval.RunOptions {
			run := promoteRun(".codex/hooks.json")
			run.Mutate = func(*harneval.Generation) error { return errors.New("surface unreadable") }
			return run
		}()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Given a completed draft and run seams for this surface.
			root := newCompletedProject(t)
			var evaluated []string
			run := tc.run
			run.Evaluate = func(task harneval.Task, g *harneval.Generation) (harneval.TaskOutcome, bool) {
				evaluated = append(evaluated, task.ID)
				return harneval.EvaluateTask(task, g), true
			}

			// When the candidate is promoted.
			result, err := runPromote(root, func(r *PromoteRequest) { r.Run = run })

			// Then the outcome is that of the promoted task alone, and the
			// promotion holds even when the task was not evaluated.
			require.NoError(t, err)
			assert.Equal(t, PromoteResultPromoted, result.Result)
			assert.Equal(t, tc.outcome, result.CurrentOutcome)
			assert.Equal(t, tc.reason, result.NotEvaluatedReason)
			if tc.outcome == OutcomeNotEvaluated {
				assert.Empty(t, evaluated)
			} else {
				assert.Equal(t, []string{promoteTaskID}, evaluated)
			}
			assert.Equal(t, promotedTask, readFile(t, root, promoteTaskAt))
			assert.NoFileExists(t, root+"/"+promoteCandidateAt)
		})
	}
}

func TestPromote_CurrentOutcome_ComparesWithNoBaseline(t *testing.T) {
	t.Parallel()
	// Given a committed baseline the loader rejects.
	root := newCompletedProject(t)
	writeFile(t, root, harneval.BaselinePath, "{")

	// When the candidate is promoted.
	result, err := runPromote(root, nil)

	// Then the task is still evaluated: one outcome compares with nothing,
	// and the baseline stays as it was.
	require.NoError(t, err)
	assert.Equal(t, OutcomePass, result.CurrentOutcome)
	assert.Equal(t, "{", readFile(t, root, harneval.BaselinePath))
}

func TestPromote_S6RealCodexSurface_CurrentOutcomePass(t *testing.T) {
	t.Parallel()
	// Given the production adapters with the template check skipped, so the
	// assertion reads the real generated codex surface.
	root := newCompletedProject(t)
	run := harneval.RunOptions{StaleCheck: func(string) ([]string, error) { return nil, nil }}

	// When the candidate is promoted.
	result, err := runPromote(root, func(r *PromoteRequest) { r.Run = run })

	// Then .codex/hooks.json of the default full config passes.
	require.NoError(t, err)
	assert.Equal(t, OutcomePass, result.CurrentOutcome, "reason %q", result.NotEvaluatedReason)
}
