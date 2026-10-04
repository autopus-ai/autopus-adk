package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/autopilot"
	"github.com/insajin/autopus-adk/pkg/qa/generate"
	qaloop "github.com/insajin/autopus-adk/pkg/qa/loop"
	"github.com/insajin/autopus-adk/pkg/qa/promote"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

func fakeQAGoDeps(order *[]string) autopilot.Deps {
	return autopilot.Deps{
		Generate: func(_ context.Context, o generate.Options) (generate.Report, error) {
			*order = append(*order, "generate")
			return generate.Report{Spec: o.SpecID, Criteria: 1, Written: []string{"a.yaml"},
				Coverage: []generate.CriterionCoverage{{ID: "AC-1", Status: "covered_by_user_scenario", Refs: []string{"scenario:a"}}}}, nil
		},
		Promote: func(string, promote.Options) (promote.Report, error) {
			*order = append(*order, "promote")
			return promote.Report{}, nil
		},
		Compile: func(string, bool) (scenario.Result, error) {
			*order = append(*order, "compile")
			return scenario.Result{}, nil
		},
		Loop: func(context.Context, qaloop.Options) (qaloop.Report, error) {
			*order = append(*order, "loop")
			return qaloop.Report{RunID: "qaloop-1", StopReason: qaloop.StopPassed, Branch: "autopus/qa-loop/qaloop-1"}, nil
		},
	}
}

func execQAGo(t *testing.T, stdin string, args ...string) (string, []string, error) {
	t.Helper()
	var order []string
	cmd := newQAGoCmdWith(fakeQAGoDeps(&order))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetIn(strings.NewReader(stdin))
	cmd.SetArgs(append(args, "--agent", "claude", "--lane", "fast", "--project-dir", t.TempDir()))
	err := cmd.Execute()
	return out.String(), order, err
}

func TestQAGoCmd_AsksBeforePromotingAndRunsOnYes(t *testing.T) {
	t.Parallel()
	out, order, err := execQAGo(t, "y\n", "SPEC-X")
	require.NoError(t, err)
	assert.Equal(t, []string{"generate", "promote", "compile", "loop"}, order)
	assert.Contains(t, out, "AC-1")
	assert.Contains(t, out, "promote these candidates and run the loop?")
	assert.Contains(t, out, "next: review and merge autopus/qa-loop/qaloop-1")
}

func TestQAGoCmd_NoAnswerStopsBeforePromotion(t *testing.T) {
	t.Parallel()
	_, order, err := execQAGo(t, "", "SPEC-X")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "--auto")
	assert.Equal(t, []string{"generate"}, order)
}

func TestQAGoCmd_JSONRunsUnattended(t *testing.T) {
	t.Parallel()
	_, order, err := execQAGo(t, "", "SPEC-X", "--json")
	require.NoError(t, err)
	assert.Equal(t, []string{"generate", "promote", "compile", "loop"}, order)
}

func TestQAGoCmd_WithoutSpecSkipsGeneration(t *testing.T) {
	t.Parallel()
	_, order, err := execQAGo(t, "")
	require.NoError(t, err)
	assert.Equal(t, []string{"compile", "loop"}, order)
}
