package security

// SPEC-EDITGUARD-001 REQ-EG-19 and S11 for the legacy worker writer: the
// worker validate handler joins hooks.PreToolUse in Claude Code's handler
// object format next to whatever is already there, and cleanup takes back only
// that handler, so user handlers and other Autopus handlers sharing an entry
// or the event survive setup and cleanup.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Entries of the S11 settings.json as `auto update` leaves it, plus the mixed
// entry of the S11 seed in which a user handler shares an entry with the guard.
const (
	s11GuardHandler = `{"type":"command","command":"out=$(auto guard edit --platform claude-code) && ` +
		`[ -n \"$out\" ] && printf '%s\\n' \"$out\"; exit 0","timeout":5}`
	s11UserEntry       = `{"matcher":"Edit","hooks":[{"type":"command","command":"./my-hook.sh"}]}`
	s11MixedEntry      = `{"matcher":"Edit|Write|MultiEdit","hooks":[{"type":"command","command":"./mixed-hook.sh"},` + s11GuardHandler + `]}`
	s11ArchEntry       = `{"matcher":"Bash","hooks":[{"type":"command","command":"auto check --hygiene --arch --quiet --staged --warn-only","timeout":30}]}`
	s11GuardEntry      = `{"matcher":"Edit|Write|MultiEdit","hooks":[` + s11GuardHandler + `]}`
	s11DispatcherEntry = `{"matcher":"Bash","hooks":[{"type":"command","command":"auto rules fire --event PreToolUse","timeout":10}]}`
	s11PostToolUse     = `"PostToolUse":[{"matcher":"Bash","hooks":[{"type":"command","command":"auto react check --quiet","timeout":60}]}]`

	// workerHandler and workerEntry are what the writer adds for /tmp/policy.json.
	workerHandler = `{"type":"command","command":"auto worker validate --policy '/tmp/policy.json' --command \"$TOOL_INPUT\""}`
	workerEntry   = `{"matcher":"Bash|Write|Edit","hooks":[` + workerHandler + `]}`
	// staleWorkerHandler is a handler an earlier setup wrote for another policy.
	staleWorkerHandler = `{"type":"command","command":"auto worker validate --policy '/old/policy.json' --command \"$TOOL_INPUT\""}`
	// legacyWorkerEntry is the bare-string handler format older releases wrote.
	legacyWorkerEntry = `{"matcher":"Bash|Write|Edit","hooks":["auto worker validate --policy '/old/policy.json' --command \"$TOOL_INPUT\""]}`
)

var s11PreToolUse = []string{s11UserEntry, s11MixedEntry, s11ArchEntry, s11GuardEntry, s11DispatcherEntry}

// s11Settings is a settings.json with a key outside hooks, the given
// PreToolUse entries, and a second event.
func s11Settings(preToolUse ...string) string {
	return `{"permissions":{"allow":["Bash(auto *)"]},"hooks":{"PreToolUse":[` +
		strings.Join(preToolUse, ",") + `],` + s11PostToolUse + `}}`
}

func seedSettings(t *testing.T, body string) (dir, settingsPath string) {
	t.Helper()
	dir = t.TempDir()
	settingsPath = filepath.Join(dir, ".claude", "settings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(settingsPath), 0o755))
	require.NoError(t, os.WriteFile(settingsPath, []byte(body), 0o644))
	return dir, settingsPath
}

func readSettingsFile(t *testing.T, settingsPath string) string {
	t.Helper()
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	return string(data)
}

// workerCommands returns the command of every PreToolUse handler the worker
// owns, whatever its format.
func workerCommands(t *testing.T, body string) []string {
	t.Helper()
	var settings struct {
		Hooks map[string][]struct {
			Hooks []json.RawMessage `json:"hooks"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal([]byte(body), &settings))
	var commands []string
	for _, entry := range settings.Hooks["PreToolUse"] {
		for _, raw := range entry.Hooks {
			var handler struct {
				Command string `json:"command"`
			}
			if json.Unmarshal(raw, &handler) != nil {
				require.NoError(t, json.Unmarshal(raw, &handler.Command), "a handler is an object or a string")
			}
			if strings.HasPrefix(handler.Command, "auto worker validate ") {
				commands = append(commands, handler.Command)
			}
		}
	}
	return commands
}

// Setup appends one handler in its own entry and changes nothing else: the
// user entry, the mixed entry, the Autopus entries, the second event, and the
// key outside hooks keep their values and order.
func TestWriteHookConfig_AddsItsHandlerBesideEveryOtherHandler(t *testing.T) {
	t.Parallel()

	dir, settingsPath := seedSettings(t, s11Settings(s11PreToolUse...))
	require.NoError(t, WriteHookConfig(dir, "/tmp/policy.json"))

	assert.JSONEq(t, s11Settings(append(append([]string{}, s11PreToolUse...), workerEntry)...),
		readSettingsFile(t, settingsPath))
}

// A handler from an earlier setup, in the object or the legacy string format,
// is replaced rather than duplicated: a shared entry loses only that handler
// and keeps its matcher and user handler, an entry the replacement empties
// goes, and a second run writes the same bytes.
func TestWriteHookConfig_ReplacesOnlyItsOwnEarlierHandlers(t *testing.T) {
	t.Parallel()

	shared := `{"matcher":"Bash","hooks":[{"type":"command","command":"./audit.sh"},` + staleWorkerHandler + `]}`
	dir, settingsPath := seedSettings(t, s11Settings(s11UserEntry, shared, legacyWorkerEntry))
	require.NoError(t, WriteHookConfig(dir, "/tmp/policy.json"))
	first := readSettingsFile(t, settingsPath)

	assert.JSONEq(t, s11Settings(s11UserEntry,
		`{"matcher":"Bash","hooks":[{"type":"command","command":"./audit.sh"}]}`, workerEntry), first)
	assert.Equal(t, []string{"auto worker validate --policy '/tmp/policy.json' --command \"$TOOL_INPUT\""},
		workerCommands(t, first), "exactly one worker handler, for the new policy")

	require.NoError(t, WriteHookConfig(dir, "/tmp/policy.json"))
	assert.Equal(t, first, readSettingsFile(t, settingsPath), "a repeated setup must write identical bytes")
}

// Cleanup removes every worker handler and nothing else. A shared entry keeps
// its matcher and its other handlers; an entry left without a handler goes.
func TestRemoveHookConfig_RemovesOnlyItsHandlers(t *testing.T) {
	t.Parallel()

	shared := `{"matcher":"Bash","hooks":[{"type":"command","command":"./audit.sh"},` + workerHandler + `]}`
	dir, settingsPath := seedSettings(t, s11Settings(s11UserEntry, shared, s11GuardEntry, workerEntry, legacyWorkerEntry))
	require.NoError(t, RemoveHookConfig(dir))

	assert.JSONEq(t, s11Settings(s11UserEntry,
		`{"matcher":"Bash","hooks":[{"type":"command","command":"./audit.sh"}]}`, s11GuardEntry),
		readSettingsFile(t, settingsPath))
}

// The last worker handler takes its event and an emptied hooks object with it,
// while the keys outside hooks stay.
func TestRemoveHookConfig_DropsTheContainersItEmptied(t *testing.T) {
	t.Parallel()

	dir, settingsPath := seedSettings(t, `{"theme":"dark","hooks":{"PreToolUse":[`+workerEntry+`]}}`)
	require.NoError(t, RemoveHookConfig(dir))

	assert.JSONEq(t, `{"theme":"dark"}`, readSettingsFile(t, settingsPath))
}

// Without a worker handler there is nothing to clean up, so the file keeps
// its exact bytes, formatting and empty containers included.
func TestRemoveHookConfig_LeavesAFileWithoutItsHandlerByteForByte(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"empty event":     `{"theme":"dark","hooks":{"PreToolUse":[]}}`,
		"other handlers":  s11Settings(s11PreToolUse...),
		"hooks not a map": `{"hooks":["./my-hook.sh"]}`,
		"no hooks":        "{\n    \"theme\": \"dark\"\n}",
	} {
		dir, settingsPath := seedSettings(t, body)
		require.NoError(t, RemoveHookConfig(dir), name)
		assert.Equal(t, body, readSettingsFile(t, settingsPath), name)
	}
}

// A hooks value the writer cannot merge into is reported rather than replaced,
// and the file keeps its bytes.
func TestWriteHookConfig_RefusesHooksItCannotMergeInto(t *testing.T) {
	t.Parallel()

	for name, body := range map[string]string{
		"hooks not an object":      `{"hooks":["./my-hook.sh"]}`,
		"PreToolUse not an array":  `{"hooks":{"PreToolUse":{"matcher":"Edit"}}}`,
		"settings not an object":   `["./my-hook.sh"]`,
		"PreToolUse a bare string": `{"hooks":{"PreToolUse":"./my-hook.sh"}}`,
	} {
		dir, settingsPath := seedSettings(t, body)
		assert.Error(t, WriteHookConfig(dir, "/tmp/policy.json"), name)
		assert.Equal(t, body, readSettingsFile(t, settingsPath), name)
	}
}

// S11: SetupAutonomousMode and then CleanupAutonomousMode on the S11 file leave
// every other handler, and the whole file, identical as parsed JSON to the
// state before setup.
func TestAutonomousMode_SetupThenCleanupRestoresTheS11Settings(t *testing.T) {
	t.Parallel()

	before := s11Settings(s11PreToolUse...)
	dir, settingsPath := seedSettings(t, before)
	policy := filepath.Join(dir, "policy.json")
	require.NoError(t, os.WriteFile(policy, []byte(`{}`), 0o600))

	require.NoError(t, SetupAutonomousMode(dir, policy))
	setup := readSettingsFile(t, settingsPath)
	assert.Equal(t, []string{"auto worker validate --policy '" + policy + "' --command \"$TOOL_INPUT\""},
		workerCommands(t, setup), "setup adds exactly one worker handler")
	assert.Equal(t, 1, strings.Count(setup, "./mixed-hook.sh"))
	assert.Equal(t, 2, strings.Count(setup, "auto guard edit"), "both guard handlers stay")

	require.NoError(t, CleanupAutonomousMode(dir))
	assert.JSONEq(t, before, readSettingsFile(t, settingsPath))
}

// Values that are not an entry object with a handler array are not the
// writer's to change, so cleanup keeps them as they are.
func TestRemoveHookConfig_KeepsValuesThatAreNotEntries(t *testing.T) {
	t.Parallel()

	dir, settingsPath := seedSettings(t, s11Settings(`"./not-an-entry.sh"`, `{"matcher":"Edit"}`, workerEntry))
	require.NoError(t, RemoveHookConfig(dir))

	assert.JSONEq(t, s11Settings(`"./not-an-entry.sh"`, `{"matcher":"Edit"}`), readSettingsFile(t, settingsPath))
}

// A settings.json holding JSON null has no settings yet, so setup starts from
// an empty object.
func TestWriteHookConfig_TreatsNullSettingsAsEmpty(t *testing.T) {
	t.Parallel()

	dir, settingsPath := seedSettings(t, "null")
	require.NoError(t, WriteHookConfig(dir, "/tmp/policy.json"))

	assert.JSONEq(t, `{"hooks":{"PreToolUse":[`+workerEntry+`]}}`, readSettingsFile(t, settingsPath))
}

// A settings.json that cannot be read is an error for both directions.
func TestHookConfig_ReportsAnUnreadableSettingsFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".claude", "settings.json"), 0o755))

	assert.ErrorContains(t, WriteHookConfig(dir, "/tmp/policy.json"), "read settings.json")
	assert.ErrorContains(t, RemoveHookConfig(dir), "read settings.json")
}
