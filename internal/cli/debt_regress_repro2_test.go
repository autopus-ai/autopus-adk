package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The build step waits while a subagent may still be editing the tree; the
// debt gate runs before that check and judges the half-finished tree.
func TestDebtGateRegression_DebtGateIgnoresBackgroundWriters(t *testing.T) {
	dir := debtStopRepo(t)
	rvGitC(t, dir, "commit", "-qm", "baseline")
	session := "rv-bg"
	t.Cleanup(func() { resetStopState(session); _ = os.Remove(stopStateFile(session, "debt")) })
	// A subagent mid-way through moving add.go: the copy exists, the original is not yet edited.
	body := "package stopgate\n\n// TODO handle overflow\nfunc Add(a, b int) int { return a + b }\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "add.go"), []byte(body), 0o644))
	isolateStop(t)
	cmd := newVerifyStopCmd()
	in := stopHookInput{SessionID: session, Cwd: dir}
	in.BackgroundTasks = append(in.BackgroundTasks, struct {
		Type string `json:"type"`
	}{Type: "subagent"})
	payload, _ := json.Marshal(in)
	cmd.SetIn(bytes.NewReader(payload))
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"--budget", (3 * time.Minute).String()})
	require.NoError(t, cmd.Execute())
	var got stopHookOutput
	require.NoError(t, json.Unmarshal(out.Bytes(), &got), out.String())
	t.Logf("with a subagent still running: decision=%q system=%q", got.Decision, got.SystemMessage)
	if got.Decision == "block" {
		t.Errorf("the debt gate blocks while a background subagent may still be editing the tree")
	}
}
