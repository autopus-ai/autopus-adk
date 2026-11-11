package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/diskspace"
)

func gatePayload(t *testing.T, dir, tool, command string, extra map[string]any) string {
	t.Helper()
	payload := map[string]any{"hook_event_name": "PreToolUse", "tool_name": tool, "cwd": dir,
		"tool_input": map[string]any{"command": command}}
	for k, v := range extra {
		payload[k] = v
	}
	data, err := json.Marshal(payload)
	require.NoError(t, err)
	return string(data)
}

// runDiskGate runs `auto disk gate` with stdin and returns its stdout; it
// must always exit 0, because a hook that fails would block the call.
func runDiskGate(t *testing.T, stdin string) string {
	t.Helper()
	root := NewRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs([]string{"disk", "gate"})
	code, _ := runRootCommand(t.Context(), root, &stderr)
	require.Equal(t, 0, code, stderr.String())
	return stdout.String()
}

func gateReason(t *testing.T, stdout string) string {
	t.Helper()
	if stdout == "" {
		return ""
	}
	var out struct {
		HookSpecificOutput diskGateDecision `json:"hookSpecificOutput"`
	}
	require.NoError(t, json.Unmarshal([]byte(stdout), &out), stdout)
	assert.Equal(t, "PreToolUse", out.HookSpecificOutput.Event)
	assert.Equal(t, "deny", out.HookSpecificOutput.Decision)
	return out.HookSpecificOutput.Reason
}

func TestDiskGate_DeniesHeavyCommandsBelowTheFloor(t *testing.T) {
	dir := writeDiskProject(t, "")
	withDiskFree(t, gib(4), "/fixture")

	reason := gateReason(t, runDiskGate(t, gatePayload(t, dir, "Bash", "cd sub && go test ./...", nil)))
	assert.Contains(t, reason, "4.0 GiB free on /fixture, below the floor of 10.0 GiB")
	assert.Contains(t, reason, "`go test`")
	assert.Contains(t, reason, "auto disk status")
	assert.Contains(t, reason, "auto disk lease --need <GiB> -- <command>")

	subagent := gatePayload(t, dir, "Bash", "npm ci", map[string]any{"agent_id": "a1", "agent_type": "Explore"})
	assert.Contains(t, gateReason(t, runDiskGate(t, subagent)), "`npm ci`", "a subagent's tool call is gated the same way")

	for _, allowed := range []string{
		gatePayload(t, dir, "Bash", "git status && go vet ./...", nil),
		gatePayload(t, dir, "Bash", "auto disk lease --need 8 -- go test ./...", nil),
		gatePayload(t, dir, "Bash", "make test", nil),
		gatePayload(t, dir, "Read", "go test ./...", nil),
		gatePayload(t, dir, "Bash", "", nil),
		"not json",
	} {
		assert.Empty(t, runDiskGate(t, allowed), allowed)
	}
}

func TestDiskGate_AllowsAboveTheFloorAndWhenUnmeasured(t *testing.T) {
	dir := writeDiskProject(t, "disk:\n  min_free_gb: 20\n")
	withDiskFree(t, gib(25), "/fixture")
	assert.Empty(t, runDiskGate(t, gatePayload(t, dir, "Bash", "go build ./...", nil)))

	withDiskFree(t, gib(15), "/fixture")
	assert.NotEmpty(t, runDiskGate(t, gatePayload(t, dir, "Bash", "go build ./...", nil)), "the floor comes from disk.min_free_gb")

	withDiskFreeFunc(t, func(string) (diskspace.Space, error) { return diskspace.Space{}, diskspace.ErrUnsupported })
	assert.Empty(t, runDiskGate(t, gatePayload(t, dir, "Bash", "go build ./...", nil)), "a system without statfs is not gated")
	withDiskFreeFunc(t, func(string) (diskspace.Space, error) { return diskspace.Space{}, errors.New("statfs failed") })
	assert.Empty(t, runDiskGate(t, gatePayload(t, dir, "Bash", "go build ./...", nil)))
}

func TestDiskGate_OptOutsAndExtraCommands(t *testing.T) {
	withDiskFree(t, gib(1), "/fixture")
	plain := writeDiskProject(t, "")
	t.Setenv("AUTOPUS_DISK_GATE", "off")
	assert.Empty(t, runDiskGate(t, gatePayload(t, plain, "Bash", "go test ./...", nil)))
	t.Setenv("AUTOPUS_DISK_GATE", "")

	off := writeDiskProject(t, "hooks:\n  disk_gate: false\n")
	assert.Empty(t, runDiskGate(t, gatePayload(t, off, "Bash", "go test ./...", nil)))

	extended := writeDiskProject(t, "hooks:\n  disk_gate:\n    extra_commands: [make release]\n")
	assert.Contains(t, gateReason(t, runDiskGate(t, gatePayload(t, extended, "Bash", "make release -j4", nil))), "`make release`")
	assert.Empty(t, runDiskGate(t, gatePayload(t, extended, "Bash", "make lint", nil)))

	broken := writeDiskProject(t, "hooks:\n  disk_gate: [\n")
	assert.NotEmpty(t, runDiskGate(t, gatePayload(t, broken, "Bash", "go test ./...", nil)),
		"a config that does not load keeps the default floor and the gate on")
}
