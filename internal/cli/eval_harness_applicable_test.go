package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// injectedClosure is the S6 dependency closure of pkg/harneval.
func injectedClosure(context.Context, string) ([]string, error) {
	return []string{"pkg/adapter/codex", "pkg/content", "pkg/harneval"}, nil
}

// writeChangedFiles writes a `git diff --name-only --no-renames` listing.
func writeChangedFiles(t *testing.T, listing string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "changed.txt")
	require.NoError(t, os.WriteFile(path, []byte(listing), 0o644))
	return path
}

// TestEvalHarnessApplicable_S6_PullRequestMatchesTheDerivedInputSet is S6: a
// pull request is applicable exactly when a changed path falls under a fixed
// glob or a closure package directory at a path-segment boundary, and the
// decision always exits 0.
func TestEvalHarnessApplicable_S6_PullRequestMatchesTheDerivedInputSet(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		listing string
		status  string
		reason  string
		matched []any
	}{
		{"readme only", "README.md\n", "not_applicable", "no_harness_input_changed", []any{}},
		{"closure package", "README.md\npkg/adapter/codex/codex.go\n", "applicable", "harness_input_changed",
			[]any{"pkg/adapter/codex/codex.go"}},
		{"segment boundary", "pkg/adapter/codexfoo/x.go\n", "not_applicable", "no_harness_input_changed", []any{}},
		{"benchmark corpus", "scripts/benchmarks/harness/corpus_a.json\n", "applicable", "harness_input_changed",
			[]any{"scripts/benchmarks/harness/corpus_a.json"}},
		{"no-renames pair", "content/a.md\ndocs/a.md\n", "applicable", "harness_input_changed", []any{"content/a.md"}},
		{"fixed inputs", "go.sum\r\ntemplates/x.tmpl\nevals/harness/manifest.json\n.github/workflows/ci.yaml\n" +
			"internal/cli/eval_harness_run.go\ninternal/cli/root.go\ngo.mod\ngo.mod\n\n", "applicable", "harness_input_changed",
			[]any{".github/workflows/ci.yaml", "evals/harness/manifest.json", "go.mod", "go.sum",
				"internal/cli/eval_harness_run.go", "templates/x.tmpl"}},
		{"git-quoted path", `"content/\355\225\234.md"` + "\n", "applicable", "harness_input_changed", []any{"content/한.md"}},
		{"look-alike prefixes", "contents/a.md\ngo.mod.bak\nevals/harnessx/a.json\n", "not_applicable", "no_harness_input_changed", []any{}},
		{"empty diff", "", "not_applicable", "no_harness_input_changed", []any{}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			changed := writeChangedFiles(t, tc.listing)

			got := runHarness(t, evalHarnessDeps{closure: injectedClosure},
				"applicable", "--event", "pull_request", "--changed-files", changed, "--format", "json")

			require.Equal(t, 0, got.code, got.stderr)
			doc := harnessDoc(t, got.stdout)
			assert.Equal(t, map[string]any{"status": tc.status, "reason": tc.reason, "matched": tc.matched}, doc)
		})
	}
}

// TestEvalHarnessApplicable_S6_FailClosedReasons: a closure that cannot be
// computed is evaluated with reason closure_unavailable, and every event
// other than pull_request is evaluated without reading a diff.
func TestEvalHarnessApplicable_S6_FailClosedReasons(t *testing.T) {
	t.Parallel()
	failing := evalHarnessDeps{closure: func(context.Context, string) ([]string, error) {
		return nil, errors.New("go list: exit status 1")
	}}
	readme := writeChangedFiles(t, "README.md\ncontent/a.md\n")

	unavailable := runHarness(t, failing, "applicable", "--event", "pull_request", "--changed-files", readme)

	require.Equal(t, 0, unavailable.code, unavailable.stderr)
	assert.Equal(t, map[string]any{"status": "applicable", "reason": "closure_unavailable", "matched": []any{"content/a.md"}},
		harnessDoc(t, unavailable.stdout))
	assert.Contains(t, unavailable.stderr, "harness-eval: closure unavailable: go list: exit status 1")
	for _, event := range []string{"push", "workflow_call"} {
		got := runHarness(t, failing, "applicable", "--event", event)
		require.Equal(t, 0, got.code, got.stderr)
		assert.Equal(t, map[string]any{"status": "applicable", "reason": "non_pull_request_event", "matched": []any{}},
			harnessDoc(t, got.stdout), event)
	}
}

// TestEvalHarnessApplicable_UnusableInvocation_ExitsOne: no event, a pull
// request without a diff listing, an unreadable listing, or a non-json format
// is an error rather than a skipped check.
func TestEvalHarnessApplicable_UnusableInvocation_ExitsOne(t *testing.T) {
	t.Parallel()
	deps := evalHarnessDeps{closure: injectedClosure}
	for name, args := range map[string][]string{
		"no event":           {"applicable"},
		"no listing":         {"applicable", "--event", "pull_request"},
		"unreadable listing": {"applicable", "--event", "pull_request", "--changed-files", filepath.Join(t.TempDir(), "absent.txt")},
		"text format":        {"applicable", "--event", "push", "--format", "text"},
	} {
		got := runHarness(t, deps, args...)
		assert.Equal(t, 1, got.code, name)
		assert.Empty(t, got.stdout, name)
		assert.Contains(t, got.stderr, "Error: ", name)
	}
}

// TestEvalHarnessApplicable_GoListClosure_IsTheInModuleDependencySet runs the
// production closure: `go list -deps ./pkg/harneval` in this module yields
// repository-relative package directories, and fails outside a module.
func TestEvalHarnessApplicable_GoListClosure_IsTheInModuleDependencySet(t *testing.T) {
	t.Parallel()
	root := moduleRootForTest(t)

	dirs, err := goListClosure(context.Background(), root)

	require.NoError(t, err)
	for _, want := range []string{"pkg/harneval", "pkg/content", "pkg/adapter/codex", "pkg/config"} {
		assert.Contains(t, dirs, want)
	}
	assert.NotContains(t, dirs, "internal/cli", "the CLI is a fixed glob, not part of the closure")
	for _, dir := range dirs {
		assert.False(t, filepath.IsAbs(dir), dir)
		assert.NotContains(t, dir, "github.com/", dir)
	}
	_, err = goListClosure(context.Background(), t.TempDir())
	assert.Error(t, err, "outside a module the closure is unavailable")

	changed := writeChangedFiles(t, "pkg/content/generate.go\n")
	got := runHarness(t, evalHarnessDeps{}, "applicable", "--event", "pull_request", "--changed-files", changed, "--dir", root)
	require.Equal(t, 0, got.code, got.stderr)
	assert.Equal(t, "harness_input_changed", harnessDoc(t, got.stdout)["reason"], "the default deps use go list")
}
