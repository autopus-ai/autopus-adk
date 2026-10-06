package harneval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSurface writes owned files per platform plus unowned files under one root.
func fakeSurface(t *testing.T, owned map[string]map[string]string, unowned map[string]string) *Surface {
	t.Helper()
	root := t.TempDir()
	write := func(rel, body string) {
		full := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}
	surface := &Surface{Root: root, Owned: map[string]map[string]bool{}}
	for platform, files := range owned {
		surface.Owned[platform] = map[string]bool{}
		for rel, body := range files {
			write(rel, body)
			surface.Owned[platform][rel] = true
		}
	}
	for rel, body := range unowned {
		write(rel, body)
	}
	return surface
}

const routerBody = "# Router\n| plan | .claude/skills/auto-plan/SKILL.md |\n| go | .claude/skills/missing/SKILL.md |\n"

const settingsBody = `{"hooks":{"PreToolUse":[{"matcher":"Bash","hooks":[{"command":"auto check --arch"}]}]},"n":null}`

func standardFakeSurface(t *testing.T) *Surface {
	return fakeSurface(t, map[string]map[string]string{
		"claude-code": {
			".claude/settings.json":             settingsBody,
			".claude/skills/auto/SKILL.md":      routerBody,
			".claude/skills/auto-plan/SKILL.md": "plan detail",
			".claude/broken.json":               "{not json",
			".claude/doc.md":                    "intro\n## Plan\nstep one  \n```sh\n# not a heading\n```\n### Sub\nkept\n## Next\nother\n",
		},
		"codex": {
			"AGENTS.md":   "codex root doc",
			".codex/a.md": "## Plan\nstep one\n```sh\n# not a heading\n```\n### Sub\nkept\n# Top\n",
			".codex/b.md": "## Plan\nstep two\n## Next\n",
		},
	}, map[string]string{".claude/skills/missing/SKILL.md": "on disk, not owned by claude"})
}

func strPtr(value string) *string { return &value }

func TestEvaluateAssertion_EachKind_PassAndFailDetail(t *testing.T) {
	t.Parallel()
	surface := standardFakeSurface(t)
	parity := func(heading string, files ...FileRef) Assertion {
		return Assertion{Kind: AssertSectionParity, Heading: heading, Files: files}
	}
	cases := []struct {
		name      string
		assertion Assertion
		detail    string
	}{
		{"exists", Assertion{Kind: AssertFileExists, Platform: "codex", Path: "AGENTS.md"}, ""},
		{"exists other platform", Assertion{Kind: AssertFileExists, Platform: "omp", Path: "AGENTS.md"}, "not_generated"},
		{"exists unowned on disk", Assertion{Kind: AssertFileExists, Platform: "claude-code", Path: ".claude/skills/missing/SKILL.md"}, "not_generated"},
		{"absent", Assertion{Kind: AssertFileAbsent, Platform: "codex", Path: ".claude/settings.json"}, ""},
		{"absent but generated", Assertion{Kind: AssertFileAbsent, Platform: "codex", Path: "AGENTS.md"}, "present"},
		{"contains", Assertion{Kind: AssertContains, Platform: "codex", Path: "AGENTS.md", Needle: "root doc"}, ""},
		{"contains is case sensitive", Assertion{Kind: AssertContains, Platform: "codex", Path: "AGENTS.md", Needle: "Root doc"}, "needle_absent"},
		{"contains missing file", Assertion{Kind: AssertContains, Platform: "omp", Path: "AGENTS.md", Needle: "x"}, "not_generated"},
		{"not contains", Assertion{Kind: AssertNotContains, Platform: "codex", Path: "AGENTS.md", Needle: "claude"}, ""},
		{"not contains hit", Assertion{Kind: AssertNotContains, Platform: "codex", Path: "AGENTS.md", Needle: "codex"}, "needle_present"},
		{"not contains needs the file", Assertion{Kind: AssertNotContains, Platform: "omp", Path: "AGENTS.md", Needle: "x"}, "not_generated"},
		{"json present", Assertion{Kind: AssertJSONPathPresent, Platform: "claude-code", Path: ".claude/settings.json", JSONPath: "hooks.PreToolUse[0].matcher"}, ""},
		{"json present wildcard value", Assertion{Kind: AssertJSONPathPresent, Platform: "claude-code", Path: ".claude/settings.json", JSONPath: "hooks.PreToolUse[*].hooks[*].command", ValueContains: strPtr("--arch")}, ""},
		{"json present value miss", Assertion{Kind: AssertJSONPathPresent, Platform: "claude-code", Path: ".claude/settings.json", JSONPath: "hooks.PreToolUse[*].hooks[*].command", ValueContains: strPtr("--lore")}, "json_path_absent"},
		{"json present null counts", Assertion{Kind: AssertJSONPathPresent, Platform: "claude-code", Path: ".claude/settings.json", JSONPath: "n"}, ""},
		{"json present out of range", Assertion{Kind: AssertJSONPathPresent, Platform: "claude-code", Path: ".claude/settings.json", JSONPath: "hooks.PreToolUse[1]"}, "json_path_absent"},
		{"json absent", Assertion{Kind: AssertJSONPathAbsent, Platform: "claude-code", Path: ".claude/settings.json", JSONPath: "hooks.PostToolUse"}, ""},
		{"json absent hit", Assertion{Kind: AssertJSONPathAbsent, Platform: "claude-code", Path: ".claude/settings.json", JSONPath: "hooks.PreToolUse"}, "json_path_present"},
		{"json absent value", Assertion{Kind: AssertJSONPathAbsent, Platform: "claude-code", Path: ".claude/settings.json", JSONPath: "hooks.PreToolUse[*].matcher", ValueContains: strPtr("Edit")}, ""},
		{"json invalid", Assertion{Kind: AssertJSONPathAbsent, Platform: "claude-code", Path: ".claude/broken.json", JSONPath: "a"}, "json_invalid"},
		{"route", Assertion{Kind: AssertRouteDetail, Platform: "claude-code", Path: ".claude/skills/auto/SKILL.md", Route: "plan", Detail: ".claude/skills/auto-plan/SKILL.md"}, ""},
		{"route line missing", Assertion{Kind: AssertRouteDetail, Platform: "claude-code", Path: ".claude/skills/auto/SKILL.md", Route: "fix", Detail: ".claude/skills/auto-plan/SKILL.md"}, "route_absent"},
		{"route detail not on the platform", Assertion{Kind: AssertRouteDetail, Platform: "claude-code", Path: ".claude/skills/auto/SKILL.md", Route: "go", Detail: ".claude/skills/missing/SKILL.md"}, "detail_not_generated"},
		{"parity", parity("## Plan", FileRef{"claude-code", ".claude/doc.md"}, FileRef{"codex", ".codex/a.md"}), ""},
		{"parity differs", parity("## Plan", FileRef{"codex", ".codex/a.md"}, FileRef{"codex", ".codex/b.md"}), "section_differs"},
		{"parity heading missing", parity("## Gone", FileRef{"codex", ".codex/a.md"}, FileRef{"codex", ".codex/b.md"}), "heading_missing"},
		{"parity file missing", parity("## Plan", FileRef{"codex", ".codex/a.md"}, FileRef{"omp", ".codex/b.md"}), "not_generated"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.detail, evaluateAssertion(tc.assertion, surface), tc.name)
	}
}

func TestEvaluateAssertion_DeletedOwnedFile_IsMissing(t *testing.T) {
	t.Parallel()
	surface := standardFakeSurface(t)
	require.NoError(t, os.Remove(filepath.Join(surface.Root, "AGENTS.md")))

	assert.Equal(t, "missing", evaluateAssertion(Assertion{Kind: AssertFileExists, Platform: "codex", Path: "AGENTS.md"}, surface))
	assert.Equal(t, "", evaluateAssertion(Assertion{Kind: AssertFileAbsent, Platform: "codex", Path: "AGENTS.md"}, surface))
}

func TestSectionBody_StopsAtSameOrHigherHeadingOutsideFences(t *testing.T) {
	t.Parallel()
	body, ok := sectionBody([]byte("## Plan  \r\na \r\n```\n## fenced\n```\n### Sub\nb\n## Next\nc\n"), "## Plan")
	require.True(t, ok)
	assert.Equal(t, "a\n```\n## fenced\n```\n### Sub\nb", body)

	_, ok = sectionBody([]byte("```\n## Plan\n```\n"), "## Plan")
	assert.False(t, ok, "a heading inside a fence is not a heading")

	body, ok = sectionBody([]byte("## Plan\nlast line"), "## Plan")
	require.True(t, ok)
	assert.Equal(t, "last line", body)
}

func TestEvaluateTask_EveryVariantAndAssertionMustPass(t *testing.T) {
	t.Parallel()
	on := standardFakeSurface(t)
	off := fakeSurface(t, map[string]map[string]string{"codex": {"AGENTS.md": "codex root doc"}}, nil)
	generation := &Generation{Surfaces: map[string]*Surface{"": on, archOff: off}}
	task := Task{ID: "GT-FIX-A", Kind: KindSurface, Assertions: []Assertion{
		{Kind: AssertFileExists, Platform: "codex", Path: "AGENTS.md"},
		{Kind: AssertFileExists, Platform: "claude-code", Path: ".claude/settings.json"},
	}}

	assert.True(t, EvaluateTask(task, generation).Passed, "no variants: default surface only")

	task.Variants = []Variant{
		{Name: "default", Overrides: map[string]bool{}},
		{Name: "arch-off", Overrides: map[string]bool{OverridePreCommitArch: false}},
	}
	outcome := EvaluateTask(task, generation)
	assert.False(t, outcome.Passed)
	assert.Equal(t, []AssertionFailure{{Variant: "arch-off", Index: 1, Detail: "not_generated"}}, outcome.Failures)

	task.Variants = []Variant{{Name: "arch-on", Overrides: map[string]bool{OverridePreCommitArch: true}}}
	outcome = EvaluateTask(task, generation)
	assert.Equal(t, []AssertionFailure{{Variant: "arch-on", Index: -1, Detail: "variant_not_generated"}}, outcome.Failures)
}

func TestTaskPlatformsAndMulti_DeriveFromAssertions(t *testing.T) {
	t.Parallel()
	single := Task{Assertions: []Assertion{{Kind: AssertFileExists, Platform: "codex", Path: "AGENTS.md"}}}
	twoPaths := Task{Assertions: []Assertion{
		{Kind: AssertRouteDetail, Platform: "claude-code", Path: "r.md", Route: "plan", Detail: "d.md"},
	}}
	twoPlatforms := Task{Assertions: []Assertion{
		{Kind: AssertSectionParity, Heading: "## A", Files: []FileRef{{"omp", "x.md"}, {"codex", "x.md"}}},
	}}

	assert.Equal(t, []string{"codex"}, TaskPlatforms(single))
	assert.False(t, IsMulti(single))
	assert.Equal(t, []string{"claude-code"}, TaskPlatforms(twoPaths))
	assert.True(t, IsMulti(twoPaths), "route_detail reads the router and the detail file")
	assert.Equal(t, []string{"codex", "omp"}, TaskPlatforms(twoPlatforms))
	assert.True(t, IsMulti(twoPlatforms))
}
