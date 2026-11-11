package cireport

import (
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lintFixture(t *testing.T, public *bool) []string {
	t.Helper()
	files, err := LoadWorkflows(filepath.Join("testdata", "workflows"))
	require.NoError(t, err)
	require.Len(t, files, 4)
	findings, problems := LintWorkflows(files, public)
	assert.Empty(t, problems)
	var got []string
	for _, f := range findings {
		got = append(got, f.Severity+" "+f.Rule+" "+f.File+" "+f.Job)
	}
	sort.Strings(got)
	return got
}

func TestLint_FixtureWorkflows(t *testing.T) {
	t.Parallel()
	public := true
	assert.Equal(t, []string{
		"high concurrency-cancels-other-events release.yml ",
		"high prt-checkout-head prt.yml label",
		"high self-hosted-fork-pr pr-bad.yml build",
		"medium missing-timeout pr-bad.yml build",
		"medium pr-no-concurrency pr-bad.yml ",
		"medium shared-release-label release.yml publish",
	}, lintFixture(t, &public))
}

// A private repo, or one whose visibility is unknown, has no fork exposure
// finding; everything else stays.
func TestLint_PrivateRepoSkipsForkRule(t *testing.T) {
	t.Parallel()
	private := false
	for _, vis := range []*bool{&private, nil} {
		for _, line := range lintFixture(t, vis) {
			assert.NotContains(t, line, "self-hosted-fork-pr")
		}
	}
}

func TestLint_ReportsUnparsableFile(t *testing.T) {
	t.Parallel()
	findings, problems := LintWorkflows(map[string][]byte{"bad.yml": []byte("on: [push\njobs: {")}, nil)
	assert.Empty(t, findings)
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "bad.yml")
}

func TestLint_TriggerForms(t *testing.T) {
	t.Parallel()
	for body, want := range map[string][]string{
		"on: push\njobs: {}\n":                                {"push"},
		"on: [push, pull_request]\njobs: {}\n":                {"pull_request", "push"},
		"on:\n  release:\n    types: [published]\njobs: {}\n": {"release"},
		"true:\n  workflow_dispatch: {}\njobs: {}\n":          {"workflow_dispatch"},
	} {
		wf, err := parseWorkflow([]byte(body))
		require.NoError(t, err, body)
		assert.Equal(t, want, wf.triggers(), body)
	}
}
