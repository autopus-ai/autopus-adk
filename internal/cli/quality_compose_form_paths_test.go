package cli

// SPEC-MODELUX-002 REQ-MUX2-10, REQ-MUX2-14: Ctrl+C in any form of the
// flow ends the step with nothing further written, and the OMP tab's back,
// capability filter, single-level, and unpin paths keep the OMP editor's
// rules.

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every form position of the flow: the nth form returns huh.ErrUserAborted
// after the answers that reach it.
func TestComposeForms_InterruptAtEveryFormEndsTheStep(t *testing.T) {
	cases := []struct {
		name    string
		fixture func(*testing.T) string
		lines   []string
		nth     int
	}{
		{"method", composeD, []string{"2\n", "1\n"}, 3},
		{"role group", composeD, []string{"2\n", "1\n", "\n"}, 4},
		{"agent list", composeD, []string{"2\n", "2\n", "2\n"}, 4},
		{"agent value", composeD, []string{"2\n", "2\n", "2\n", "6\n"}, 5},
		{"default preset", composeD, []string{"3\n"}, 2},
		{"override question", writeComposeProjectP, composeFormS16, 8},
		{"OMP profile", composeO3, []string{"2\n", "4\n"}, 3},
		{"OMP family", composeO3, []string{"2\n", "4\n", "2\n"}, 4},
		{"OMP agent list", composeO3, []string{"2\n", "4\n", "4\n", "1\n"}, 5},
		{"OMP model", composeO3, []string{"2\n", "4\n", "4\n", "1\n", "5\n"}, 6},
		{"OMP thinking", composeO3, []string{"2\n", "4\n", "4\n", "1\n", "5\n", "3\n"}, 7},
		{"OMP confirmation", composeO3, composeFormS8, 9},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			composeSessionCatalog(t, "CAT-A")
			dir := tc.fixture(t)
			before := composeSessionHash(t, dir)
			deps, activations := composeOMPDeps(ompCLIBalancedCatalogJSON())
			abortComposeFormAt(t, tc.nth)
			cmd, out := newComposeTestCommand()
			cmd.SetIn(formInput(tc.lines...))

			_, err := newComposeFormSession(t, dir).runForms(cmd, composeFormOptions{Presets: true, OMP: &composeOMPTab{deps: deps}})

			assert.True(t, errors.Is(err, errComposeInterrupted), "%v", err)
			assert.Equal(t, 1, strings.Count(out.String(), composeFormTextKO.Interrupted+"\n"))
			assert.Equal(t, before, composeSessionHash(t, dir))
			assert.Zero(t, activations.calls)
		})
	}
}

// After a pin is applied, the tab opens again on the synced policy, and
// back returns to the menu with nothing more written or activated.
func TestComposeOMPTab_BackAfterAnApplyWritesNothingMore(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := composeO3(t)
	deps, activations := composeOMPDeps(ompCLIBalancedCatalogJSON())

	out, err := runComposeOMPForms(t, dir, deps, composeFormLines(composeFormS8, "2\n", "2\n", "4\n", "1\n", "\n")...)

	require.NoError(t, err, out)
	assert.Equal(t, 2, strings.Count(out, "OMP 구성 \n1. 돌아가기\n"))
	assert.Equal(t, 1, activations.calls)
}

// The model select lists only the installed models that declare the
// agent's capability; a model with one thinking level is pinned at it
// without a question, and the profile default unpins the agent again.
func TestComposeOMPTab_SingleLevelPinAndUnpin(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := composeO3(t)
	before := composeSessionHash(t, dir)
	astra := `"id":"gpt-6-astra","family":"openai","capabilities":["deep_reasoning","coding_tool_use","independent_dissent"],"thinking":["high","max"]`
	catalog := string(ompCLIBalancedCatalogJSON())
	require.Contains(t, catalog, astra)
	deps, activations := composeOMPDeps([]byte(strings.Replace(catalog, astra, strings.Replace(astra, `["high","max"]`, `["max"]`, 1), 1)))

	out, err := runComposeOMPForms(t, dir, deps,
		"2\n", "4\n", "4\n", "1\n", "5\n", "3\n", "5\n", "1\n", "\n", "\n")

	require.NoError(t, err, out)
	assert.Contains(t, out, "task (deep_reasoning) \n1. 프로필 기본값\n2. anthropic/claude-fable-5-1\n3. openai-codex/gpt-6-astra\nEnter")
	assert.Contains(t, out, "5. task — openai-codex/gpt-6-astra:max\n")
	assert.NotContains(t, out, "task thinking")
	assert.Contains(t, out, "No agent was pinned; nothing changed.\n")
	assert.Zero(t, activations.calls)
	assert.Equal(t, before, composeSessionHash(t, dir))
}

func composeD(t *testing.T) string {
	t.Helper()
	return writeComposeFixture(t, "claude-code", "codex")
}

// composeNativeOMPCatalog is the catalog shape an installed omp prints:
// exact selectors, display names, and thinking levels, but no family or
// capability metadata.
const composeNativeOMPCatalog = `{"models":[
	{"provider":"anthropic","id":"claude-fable-5-1","selector":"anthropic/claude-fable-5-1","name":"Claude Fable 5.1","contextWindow":200000,"reasoning":true,"thinking":["high","max"],"input":["text"]},
	{"provider":"openai-codex","id":"gpt-6-astra","selector":"openai-codex/gpt-6-astra","name":"GPT-6 Astra","contextWindow":272000,"reasoning":true,"thinking":["max"],"input":["text"]},
	{"provider":"openai-codex","id":"gpt-6-luna","selector":"openai-codex/gpt-6-luna","name":"GPT-6 Luna","contextWindow":272000,"reasoning":true,"thinking":["high","max"],"input":["text"]}
]}`

// Over the catalog an installed omp prints, the model select lists every
// installed model, and the pin is attested against the built-in
// declarations, planned, and applied through the OMP writer.
func TestComposeOMPTab_NativeCatalogListsEveryModelAndApplies(t *testing.T) {
	composeSessionCatalog(t, "CAT-A")
	dir := composeO3(t)
	deps, activations := composeOMPDeps([]byte(composeNativeOMPCatalog))

	out, err := runComposeOMPForms(t, dir, deps, "2\n", "4\n", "4\n", "1\n", "5\n", "3\n", "\n", "2\n", "\n")

	require.NoError(t, err, out)
	assert.Contains(t, out, "The installed catalog reports no family or capability metadata")
	assert.Contains(t, out, "4. openai-codex/gpt-6-luna\n", "no capability filter without metadata")
	assert.Contains(t, out, "OMP models applied. Start a new OMP session to use them.\n")
	assert.Equal(t, 1, activations.calls)
	policy := rawRoleModelPolicyOf(t, readBlockFixture(t, dir))
	require.Len(t, policy.Agents["task"].Candidates, 1)
	candidate := policy.Agents["task"].Candidates[0]
	assert.Equal(t, []string{"openai-codex/gpt-6-astra", "max"}, []string{candidate.Selector, candidate.Thinking})
}
