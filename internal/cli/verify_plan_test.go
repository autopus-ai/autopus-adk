package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/verifygate"
)

// auto verify plan lists exactly the steps the Stop hook runs, with the
// narrow ones the agent should run itself and the whole ones it should not.
func TestBuildVerifyPlan(t *testing.T) {
	changedTest := "package stopgate\n\nimport \"testing\"\n\nfunc TestAdd(t *testing.T) {}\n\nfunc TestSub(t *testing.T) {}\n"
	cases := []struct {
		name      string
		files     map[string]string
		wantScope []string
		wantRun   []string
		wantNote  string
	}{
		{name: "nothing changed", wantNote: "runs nothing"},
		{name: "source only", files: map[string]string{"add.go": "package stopgate\n\nfunc Add(a, b int) int { return b + a }\n"},
			wantScope: []string{"build", "whole"}, wantRun: []string{"go build ./...", "go test ."},
			wantNote: "auto verify stop --last"},
		{name: "a changed test", files: map[string]string{"add_test.go": changedTest},
			wantScope: []string{"build", "changed-tests", "whole"},
			wantRun:   []string{"go build ./...", "go test -run '^(TestAdd|TestSub)$' .", "go test ."},
			wantNote:  "do not start a whole package"},
		{name: "docs only", files: map[string]string{"README.md": "# x\n"}, wantNote: "runs nothing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := newStopRepo(t, "")
			for name, body := range tc.files {
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
			}
			root, changed, ok := verifygate.ChangedFiles(context.Background(), dir)
			require.True(t, ok)
			out := buildVerifyPlan(root, changed, 8*time.Minute)
			var scopes, runs []string
			for _, s := range out.Steps {
				scopes, runs = append(scopes, s.Scope), append(runs, s.Run)
				assert.Equal(t, root, s.Dir)
			}
			assert.Equal(t, tc.wantScope, scopes)
			assert.Equal(t, tc.wantRun, runs)
			assert.Contains(t, out.Note, tc.wantNote)
		})
	}
}

// The JSON form keeps the hook's exact argv, -json included.
func TestVerifyPlanCmd_JSONKeepsTheHookArgv(t *testing.T) {
	dir := newStopRepo(t, "package stopgate\n\nfunc Add(a, b int) int { return b + a }\n")
	t.Chdir(dir)
	cmd := newVerifyPlanCmd()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--json"})
	require.NoError(t, cmd.Execute())
	assert.Contains(t, buf.String(), `"-json"`)
	assert.Contains(t, buf.String(), `"scope": "whole"`)
	assert.Contains(t, buf.String(), `"budget": "8m0s"`)
}
