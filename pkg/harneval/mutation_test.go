package harneval

import (
	"context"
	"encoding/json"
	"path/filepath"
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// seededMutation is one row of the committed mutation table (REQ-HE-12, CD-5):
// a fixed edit of the generated surface, applied after generation and before
// any assertion, and the exact task ids it must turn from pass to fail.
type seededMutation struct {
	ID        string
	Defect    string
	Regressed []string
	Apply     func(*surfaceEditor)
}

// triageInheritBullet is the Task Triage bullet M3 drops on Antigravity.
const triageInheritBullet = "- Inherit the requested model and reasoning settings. " +
	"Do not lower the model or effort to make a route appear cheaper.\n"

// seededMutations is the committed mutation table. M1-M5 are the REQ-HE-12
// classes. M6-M14, with M2-M4, are the source spot-checks recorded in
// evals/harness/README.md, each written as the surface edit its source change
// makes; every one regresses the task the spot-check regressed. A row whose
// target is gone or whose regressed set moves fails until it is reviewed.
var seededMutations = []seededMutation{
	{ID: "M1", Defect: "the managed PreToolUse rule-dispatch hook is removed from .claude/settings.json",
		Regressed: []string{"GT-HOOK-ARCH-GATE-OPT-OUT", "GT-HOOK-CLAUDE-RULE-DISPATCH"},
		Apply: func(e *surfaceEditor) {
			e.editJSON("claude-code", ".claude/settings.json", dropHookEntries("PreToolUse", "auto rules fire --event PreToolUse"))
		}},
	{ID: "M2", Defect: "the Claude router maps plan to auto-go (auto-router.md.tmpl)",
		Regressed: []string{"GT-ROUTE-CLAUDE-DETAILS"},
		Apply: func(e *surfaceEditor) {
			e.replace("claude-code", ".claude/skills/auto/SKILL.md",
				"| `plan` | `.claude/skills/auto-plan/SKILL.md` |", "| `plan` | `.claude/skills/auto-go/SKILL.md` |")
		}},
	{ID: "M3", Defect: "one Task Triage bullet is dropped on Antigravity (gemini auto-router.md.tmpl)",
		Regressed: []string{"GT-PROMPT-TRIAGE-PARITY"},
		Apply: func(e *surfaceEditor) {
			e.replace("antigravity-cli", ".agents/plugins/autopus/skills/auto/SKILL.md", triageInheritBullet, "")
			e.replace("antigravity-cli", ".gemini/skills/auto/SKILL.md", triageInheritBullet, "")
		}},
	{ID: "M4", Defect: "hooks.pre_commit_arch=false is ignored: the variant gets the flag-on surface (pkg/content/hooks.go)",
		Regressed: []string{"GT-HOOK-ARCH-GATE-OPT-OUT"},
		Apply:     func(e *surfaceEditor) { e.swapVariant(OverridePreCommitArch+"=false", "") }},
	{ID: "M5", Defect: "the tdd skill is no longer exposed on OMP",
		Regressed: []string{"GT-SKILL-CORE-CATALOG"},
		Apply:     func(e *surfaceEditor) { e.remove("omp", ".omp/skills/tdd/SKILL.md") }},
	{ID: "M6", Defect: "the reviewer gains Write, Edit (content/agents/reviewer.md)",
		Regressed: []string{"GT-AGENT-READONLY-REVIEW"},
		Apply: func(e *surfaceEditor) {
			e.replace("claude-code", ".claude/agents/autopus/reviewer.md",
				"tools: Read, Grep, Glob, Bash\n", "tools: Read, Write, Edit, Grep, Glob, Bash\n")
		}},
	{ID: "M7", Defect: "harness-workflow loses its claude-only gating (content/skills/harness-workflow.md)",
		Regressed: []string{"GT-SKILL-CLAUDE-NATIVE-ORCHESTRATION"},
		Apply: func(e *surfaceEditor) {
			e.leak("claude-code", ".claude/skills/harness-workflow/SKILL.md", "omp", ".omp/skills/harness-workflow/SKILL.md")
		}},
	{ID: "M8", Defect: "the OpenCode after-hook runs for every tool (opencode_plugin.go)",
		Regressed: []string{"GT-HOOK-OPENCODE-PLUGIN"},
		Apply: func(e *surfaceEditor) {
			e.replace("opencode", ".opencode/plugins/autopus-hooks.js",
				"\"tool.execute.after\": async (input) => {\n      if (input.tool !== \"bash\") return\n",
				"\"tool.execute.after\": async (input) => {\n")
		}},
	{ID: "M9", Defect: "the Codex marketplace points at ./.autopus/plugins/autopus (codex_plugin_manifest.go)",
		Regressed: []string{"GT-SKILL-CODEX-PLUGIN-ENTRY"},
		Apply: func(e *surfaceEditor) {
			e.replace("codex", ".agents/plugins/marketplace.json",
				`"path": "./.autopus/plugins/auto"`, `"path": "./.autopus/plugins/autopus"`)
		}},
	{ID: "M10", Defect: "the Claude managed-block markers are renamed (claude.go)",
		Regressed: []string{"GT-HYGIENE-MANAGED-BLOCKS"},
		Apply: func(e *surfaceEditor) {
			e.replace("claude-code", "CLAUDE.md", "<!-- AUTOPUS:BEGIN -->", "<!-- AUTOPUS-ADK:BEGIN -->")
			e.replace("claude-code", "CLAUDE.md", "<!-- AUTOPUS:END -->", "<!-- AUTOPUS-ADK:END -->")
		}},
	{ID: "M11", Defect: "OMP /auto-plan loads the router instead of its detail (omp_commands.go)",
		Regressed: []string{"GT-ROUTE-OMP-EXACT-MAP"},
		Apply: func(e *surfaceEditor) {
			e.replace("omp", ".omp/commands/auto-plan.md", "Load exact detail skill `auto-plan`;", "Load exact detail skill `auto`;")
		}},
	{ID: "M12", Defect: "lore-commit loses its hook condition (content/rules/lore-commit.md)",
		Regressed: []string{"GT-HOOK-CLAUDE-RULE-DISPATCH"},
		Apply: func(e *surfaceEditor) {
			e.replace("claude-code", ".claude/hooks/autopus/conditional-rules.json",
				"\"conditions\": [\n        "+`"\\bgit\\s+commit\\b"`+"\n      ]", `"conditions": []`)
		}},
	{ID: "M13", Defect: "the Claude hook directory is made absolute (hooks_completion.go)",
		Regressed: []string{"GT-HOOK-SESSION-LIFECYCLE"},
		Apply: func(e *surfaceEditor) {
			e.replace("claude-code", ".claude/settings.json",
				`\"${CLAUDE_PROJECT_DIR:-.}\"/.claude/hooks/autopus/hook-claude-sessionstart.sh`,
				`/.claude/hooks/autopus/hook-claude-sessionstart.sh`)
		}},
	{ID: "M14", Defect: "legacy /auto:plan loads auto-go (gemini commands/auto/plan.toml.tmpl)",
		Regressed: []string{"GT-ROUTE-ANTIGRAVITY-COMMANDS"},
		Apply: func(e *surfaceEditor) {
			e.replace("antigravity-cli", ".gemini/commands/auto/plan.toml",
				".gemini/skills/autopus/auto-plan/SKILL.md", ".gemini/skills/autopus/auto-go/SKILL.md")
		}},
}

// TestSeededMutations_S13_EachRegressesExactlyItsTabledTasks is the S13
// self-test on the committed golden set and baseline. Inside the production
// mutation seam it evaluates the unmutated surface, then applies each row,
// evaluates and compares exactly as Run does after the seam, and undoes the
// edit. Every row must fail with reason regression and exactly its tabled
// tasks; Run then finishes on the restored surface and passes. One
// generation serves every row, so the test costs one pinned generation. The
// template check is skipped: it reads content/, not the surface under test.
func TestSeededMutations_S13_EachRegressesExactlyItsTabledTasks(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	set, err := LoadSet(root)
	require.NoError(t, err)
	baseline, err := LoadBaseline(root)
	require.NoError(t, err)
	evaluator := RunOptions{}.withDefaults().Evaluate
	surfaceResult := func(generation *Generation) *Result {
		return compare(set, baseline, evaluate(set, generation, evaluator))
	}
	applied := 0
	selfTest := func(generation *Generation) error {
		clean := surfaceResult(generation)
		require.Equal(t, StatusPass, clean.Status, "the unmutated surface must pass first: %v", clean.Transitions)
		before := fingerprint(t, generation)
		for _, mutation := range seededMutations {
			editor := &surfaceEditor{generation: generation}
			t.Run(mutation.ID, func(t *testing.T) {
				require.NotEmpty(t, mutation.Regressed, "a row must regress at least one task")
				editor.t = t
				mutation.Apply(editor)

				result := surfaceResult(generation)

				assert.Equal(t, []string{ReasonRegression}, result.FailureReasons, mutation.Defect)
				assert.Equal(t, regressions(mutation.Regressed), result.Transitions, mutation.Defect)
			})
			require.NoError(t, editor.restore(), "undo %s", mutation.ID)
			applied++
		}
		require.Equal(t, before, fingerprint(t, generation), "every mutation must leave the clean surface behind")
		return nil
	}

	result, err := Run(context.Background(), root, RunOptions{StaleCheck: noStaleTemplates, Mutate: selfTest})

	require.NoError(t, err)
	assert.Equal(t, len(seededMutations), applied)
	assert.Equal(t, StatusPass, result.Status, "%v %v", result.FailureReasons, result.Transitions)
	assert.Empty(t, result.Transitions)
	assert.Equal(t, result.Totals.DeclaredSurface, result.Totals.PassedSurface)
}

// TestSeededMutations_TableCoversEveryREQHE12Class pins the table shape the
// SPEC requires: at least five rows, unique ids, and only active surface
// tasks of the committed set as regression targets.
func TestSeededMutations_TableCoversEveryREQHE12Class(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	set, err := LoadSet(root)
	require.NoError(t, err)
	active := map[string]bool{}
	for _, task := range set.Tasks {
		active[task.ID] = task.Kind == KindSurface && task.Status.State == StateActive
	}
	require.GreaterOrEqual(t, len(seededMutations), 5)
	ids := map[string]bool{}
	for _, mutation := range seededMutations {
		assert.False(t, ids[mutation.ID], "duplicate mutation id %s", mutation.ID)
		ids[mutation.ID] = true
		for _, id := range mutation.Regressed {
			assert.True(t, active[id], "%s names %s, which is not an active surface task", mutation.ID, id)
		}
	}
	for _, class := range []string{"M1", "M2", "M3", "M4", "M5"} {
		assert.True(t, ids[class], "REQ-HE-12 class %s is missing", class)
	}
}

// regressions is the transition list of exactly the given tasks regressing,
// in the task id order a result uses.
func regressions(ids []string) []Transition {
	list := make([]Transition, 0, len(ids))
	for _, id := range slices.Sorted(slices.Values(ids)) {
		list = append(list, Transition{TaskID: id, Kind: TransitionRegression})
	}
	return list
}

// fingerprint records, per variant, the surface it resolves to, the digest of
// its files, and the paths each platform owns.
func fingerprint(t *testing.T, generation *Generation) map[string]string {
	t.Helper()
	prints := make(map[string]string, len(generation.Surfaces))
	for key, surface := range generation.Surfaces {
		digest, err := SurfaceDigest(surface.Root)
		require.NoError(t, err)
		owned, err := json.Marshal(surface.Owned)
		require.NoError(t, err)
		prints[key] = surface.Root + "\n" + digest + "\n" + sha256Hex(owned)
	}
	return prints
}

// dropHookEntries removes every hooks.<event> entry that runs command.
func dropHookEntries(event, command string) func(map[string]any) bool {
	return func(doc map[string]any) bool {
		hooks, _ := doc["hooks"].(map[string]any)
		entries, _ := hooks[event].([]any)
		kept := make([]any, 0, len(entries))
		for _, entry := range entries {
			if !runsCommand(entry, command) {
				kept = append(kept, entry)
			}
		}
		if len(kept) == len(entries) {
			return false
		}
		hooks[event] = kept
		return true
	}
}

func runsCommand(entry any, command string) bool {
	group, _ := entry.(map[string]any)
	handlers, _ := group["hooks"].([]any)
	for _, handler := range handlers {
		if fields, _ := handler.(map[string]any); fields["command"] == command {
			return true
		}
	}
	return false
}
