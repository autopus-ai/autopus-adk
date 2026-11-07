package cli

// SPEC-REA-001 S7 (1), (2) over an autopus.yaml written by hand: the update's
// REA decision adds or edits only the features.rea.enabled lines. The
// v0.50.126 RC re-encoded the whole file there (smoke 2026-10-17): key order
// changed and empty provider values appeared.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// reaHandWritten rewrites dir's autopus.yaml, which yaml.v3 wrote, in a
// layout yaml.v3 never writes and returns the text: a comment line, an inline
// comment, two-space indentation, and, as in the RC's own autopus.yaml, no
// features block. Every other value loads the same.
func reaHandWritten(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, "autopus.yaml")
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	var kept strings.Builder
	inFeatures := false
	for _, line := range strings.SplitAfter(string(data), "\n") {
		if !strings.HasPrefix(line, " ") {
			inFeatures = line == "features:\n"
		}
		if inFeatures {
			continue
		}
		body := strings.TrimLeft(line, " ")
		kept.WriteString(strings.Repeat(" ", (len(line)-len(body))/2) + body)
	}
	text := "# autopus.yaml, edited by hand\n" + strings.Replace(kept.String(), "mode: full\n", "mode: full  # keep\n", 1)
	want, err := config.ParseConfig(data)
	require.NoError(t, err)
	got, err := config.ParseConfig([]byte(text))
	require.NoError(t, err)
	want.Features = config.FeaturesConf{}
	require.Equal(t, want, got, "the hand layout loads the same values")
	require.NoError(t, os.WriteFile(path, []byte(text), 0o644))
	return text
}

// An interactive undecided update: n appends exactly the three key lines,
// Enter leaves the file byte-identical because the managed entries carry the
// decision, and Enter under a failing Node gate appends enabled: true.
func TestUpdateRea_AnswerChangesOnlyTheKeyLines(t *testing.T) {
	recorder := reaCLIEnv(t, "v24.11.0\n")
	old := reaOLD(t, reaAllTargets)
	text := reaHandWritten(t, old)
	stubDriverProbes(t, true, "")

	no := cloneComposeProject(t, old)
	out, err := reaUpdate(t, strings.NewReader("n\n\n"), no)
	require.NoError(t, err, out)
	assert.Equal(t, [3]int{1, 0, 1}, reaCounts(out), out)
	assert.Equal(t, text+"features:\n  rea:\n    enabled: false\n", reaYAML(t, no))

	yes := cloneComposeProject(t, old)
	out, err = reaUpdate(t, strings.NewReader("y\n\n"), yes)
	require.NoError(t, err, out)
	assertReaManagedOnce(t, yes)
	assert.Equal(t, text, reaYAML(t, yes))

	recorder.stdout = "v20.11.0\n"
	gate := cloneComposeProject(t, old)
	out, err = reaUpdate(t, strings.NewReader("y\n\n"), gate)
	require.NoError(t, err, out)
	assertReaAbsent(t, gate)
	assert.Equal(t, text+"features:\n  rea:\n    enabled: true\n", reaYAML(t, gate))
}

// --rea over a stored false replaces only the value token; the rest of the
// line, its comment included, and every other byte stay.
func TestUpdateRea_FlagReplacesTheStoredValueInPlace(t *testing.T) {
	reaCLIEnv(t, "v24.11.0\n")
	stubDriverProbes(t, false, "")
	dir := reaOLD(t, reaAllTargets)
	text := reaHandWritten(t, dir) + "features:\n  rea:\n    enabled: false  # chosen in review\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(text), 0o644))

	out, err := reaUpdate(t, strings.NewReader(""), dir, "--rea")

	require.NoError(t, err, out)
	assertReaManagedOnce(t, dir)
	assert.Equal(t, strings.Replace(text, "enabled: false  # chosen", "enabled: true  # chosen", 1), reaYAML(t, dir))
}

// A file the splice cannot edit safely fails the update with the refusal and
// keeps every byte; the off decision still reached the target files, and no
// old-binary line is printed for a key that was not written.
func TestUpdateRea_RefusedKeyFailsTheRunAndKeepsTheFile(t *testing.T) {
	reaCLIEnv(t, "v24.11.0\n")
	stubDriverProbes(t, false, "")
	dir := reaQ(t)
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	flow := strings.Replace(string(data), "features:\n    cc21:\n        enabled: false\n        task_created_mode: warn\n",
		"features: {cc21: {enabled: false, task_created_mode: warn}}\n", 1)
	require.NotEqual(t, string(data), flow)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(flow), 0o644))

	out, err := reaUpdate(t, strings.NewReader(""), dir, "--yes", "--no-rea")

	require.Error(t, err, out)
	assert.Contains(t, err.Error(), "rea: features.rea 저장 실패: refuse to edit features.rea.enabled in autopus.yaml")
	assert.Contains(t, err.Error(), "set features.rea.enabled: false by hand")
	assert.Equal(t, flow, reaYAML(t, dir))
	assertReaAbsent(t, dir)
	assert.Zero(t, strings.Count(out, reaTextKo.OldBinary), out)
}
