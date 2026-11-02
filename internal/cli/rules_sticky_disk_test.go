package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

const stickyAlertAt5GiB = "Disk alert: 5.0 GiB free on /fixture, below the floor of 10.0 GiB (disk.min_free_gb). " +
	"Do not start builds, worktrees, or parallel agents until space recovers; run `auto disk status`, " +
	"then `auto disk reclaim --apply` and `auto disk trim --apply`. " +
	"If you lead other agents, tell each of them to pause heavy work."

func stickyContext(t *testing.T, stdout string) string {
	t.Helper()
	var payload struct {
		HookSpecificOutput struct {
			AdditionalContext string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &payload), stdout)
	return payload.HookSpecificOutput.AdditionalContext
}

// SPEC-DISK-002 S8: below the floor the alert follows the rule bodies when
// they are due, stands alone when they are not, and is logged once per 30
// minutes.
func TestRulesSticky_DiskAlertBelowFloor(t *testing.T) {
	dir := stickyProjectRoot(t, "ALERT-BODY")

	due, stderr := runRulesStickyWithFree(t, dir, cliStickyPayload("S-alert", "one"), gib(5))
	assert.Empty(t, stderr)
	context := stickyContext(t, due)
	assert.Contains(t, context, "ALERT-BODY")
	assert.True(t, len(context) > len(stickyAlertAt5GiB) && context[len(context)-len(stickyAlertAt5GiB):] == stickyAlertAt5GiB, context)

	offCadence, _ := runRulesStickyWithFree(t, dir, cliStickyPayload("S-alert", "two"), gib(5))
	assert.Equal(t, stickyAlertAt5GiB, stickyContext(t, offCadence), "the alert does not wait for the cadence")

	state, err := openDiskState()
	require.NoError(t, err)
	events, err := diskspace.TailEvents(state, 0)
	require.NoError(t, err)
	alerts := 0
	for _, e := range events {
		if e.Kind == diskspace.EventLowSpaceAlert {
			alerts++
		}
	}
	assert.Equal(t, 1, alerts, "two alerts within 30 minutes log one event")
}

// SPEC-DISK-002 S8: at or above the floor, and when free space cannot be
// measured, the output is byte-identical to the rules alone.
func TestRulesSticky_NoAlertLeavesOutputUnchanged(t *testing.T) {
	dir := stickyProjectRoot(t, "QUIET-BODY")
	baseline, _ := runRulesSticky(t, dir, cliStickyPayload("S-base", "one"))
	require.NotEmpty(t, baseline)

	above, _ := runRulesStickyWithFree(t, dir, cliStickyPayload("S-above", "one"), gib(20))
	assert.Equal(t, baseline, above)

	withDiskFreeFunc(t, func(string) (diskspace.Space, error) { return diskspace.Space{}, errors.New("statfs failed") })
	t.Chdir(dir)
	failing, _ := runRulesStickyNoPin(t, cliStickyPayload("S-fail", "one"))
	assert.Equal(t, baseline, failing)
}

// SPEC-DISK-002 review F-15: a config that does not load drops only the
// alert; the hook still exits zero with the rule output.
func TestRulesSticky_BrokenConfigDropsOnlyTheAlert(t *testing.T) {
	dir := stickyProjectRoot(t, "BROKEN-CFG-BODY")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte("mode: [unclosed\n"), 0o644))
	low, _ := runRulesStickyWithFree(t, dir, cliStickyPayload("S-broken", "one"), gib(1))
	assert.Contains(t, stickyContext(t, low), "BROKEN-CFG-BODY")
	assert.NotContains(t, low, "Disk alert")
}

// SPEC-DISK-002 review r2: an unusable state dir drops only the alert's
// sample and event; the alert itself is still shown.
func TestRulesSticky_UnusableStateStillAlerts(t *testing.T) {
	dir := stickyProjectRoot(t, "STATE-BODY")
	link := filepath.Join(t.TempDir(), "link")
	require.NoError(t, os.Symlink(t.TempDir(), link))
	prev := diskStatePath
	diskStatePath = func() string { return filepath.Join(link, "disk") }
	t.Cleanup(func() { diskStatePath = prev })

	low, _ := runRulesStickyWithFree(t, dir, cliStickyPayload("S-state", "one"), gib(5))
	assert.Contains(t, stickyContext(t, low), stickyAlertAt5GiB)
}
