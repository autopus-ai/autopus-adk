package cli_test

// SPEC-PANERM-001 T3 (S13): `auto doctor` reports exactly what `auto update`
// deletes. Before the update doctor.legacy_orchestra_config and
// doctor.stale_completion_hooks warn with the remedy run "auto update"; after
// it both pass and no warning names a retired hook or key. Red at B, where
// neither check existed; runs since T14 landed both.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	panermLegacyCheck = "doctor.legacy_orchestra_config"
	panermStaleCheck  = "doctor.stale_completion_hooks"
	panermRemedy      = `run "auto update"`
)

type panermDoctorCheck struct {
	ID     string            `json:"id"`
	Status string            `json:"status"`
	Detail string            `json:"detail"`
	Fields map[string]string `json:"fields"`
}

func runPanermDoctor(t *testing.T, root string, jsonMode bool) (string, []panermDoctorCheck) {
	t.Helper()
	cmd := newTestRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	args := []string{"doctor", "--dir", root}
	if jsonMode {
		args = append(args, "--json")
	}
	cmd.SetArgs(args)
	_ = cmd.Execute() // doctor exits non-zero while any check warns
	if !jsonMode {
		return out.String(), nil
	}
	var envelope struct {
		Checks []panermDoctorCheck `json:"checks"`
	}
	require.NoError(t, json.Unmarshal(out.Bytes(), &envelope), out.String())
	return out.String(), envelope.Checks
}

func findPanermCheck(checks []panermDoctorCheck, id string) *panermDoctorCheck {
	for i := range checks {
		if checks[i].ID == id {
			return &checks[i]
		}
	}
	return nil
}

// panermCheckText joins a check's detail and fields so the oracle does not
// depend on where T14 puts the list or the remedy.
func panermCheckText(check *panermDoctorCheck) string {
	data, _ := json.Marshal(check)
	return string(data)
}

func scriptNames(paths []string) []string {
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		names = append(names, filepath.Base(path))
	}
	sort.Strings(names)
	return names
}

func TestPanermS13_DoctorReportsWhatUpdateDeletes(t *testing.T) {
	for _, ws := range staleHookRetractionWorkspaces {
		t.Run(ws.name, func(t *testing.T) {
			useStaleHookEnv(t, ws.opencode)
			root := copyStaleHookWorkspace(t, ws.name)
			configPath := filepath.Join(root, "autopus.yaml")
			if ws.name == "W-claude" {
				require.NoError(t, os.WriteFile(configPath, readLegacyPaneConfig(t, "c2.yaml"), 0o644))
			}
			configData, err := os.ReadFile(configPath)
			require.NoError(t, err)
			wantKeys := groupKPaths(t, configData)

			_, before := runPanermDoctor(t, root, true)
			legacy := findPanermCheck(before, panermLegacyCheck)
			require.NotNil(t, legacy, "doctor --json must report %s", panermLegacyCheck)
			if len(wantKeys) > 0 {
				assert.Equal(t, "warn", legacy.Status)
				assert.Equal(t, "legacy orchestra keys: "+strings.Join(wantKeys, ", "), legacy.Detail)
				assert.Contains(t, panermCheckText(legacy), `run \"auto update\"`)
			}
			stale := findPanermCheck(before, panermStaleCheck)
			require.NotNil(t, stale, "doctor --json must report %s", panermStaleCheck)
			assert.Equal(t, "warn", stale.Status)
			assert.Equal(t, scriptNames(ws.deleted), uniqueSorted(staleHookScriptRefs([]byte(stale.Detail+panermCheckText(stale)))),
				"the scripts doctor reports must equal the S11 deletion set")
			text, _ := runPanermDoctor(t, root, false)
			if len(wantKeys) > 0 {
				assert.Contains(t, text, "legacy orchestra keys: "+strings.Join(wantKeys, ", "))
			}
			assert.Contains(t, text, panermRemedy)

			out, err := runStaleHookUpdate(t, root)
			require.NoError(t, err, out)

			_, after := runPanermDoctor(t, root, true)
			for _, id := range []string{panermLegacyCheck, panermStaleCheck} {
				check := findPanermCheck(after, id)
				require.NotNil(t, check, id)
				assert.Equal(t, "pass", check.Status, "%s after auto update", id)
			}
			for _, check := range after {
				if check.Status != "warn" && check.Status != "fail" {
					continue
				}
				for _, token := range []string{"hook-claude-stop.sh", "hook-codex-stop.sh", "hook-gemini-afteragent.sh",
					"AUTOPUS_SESSION_ID", "monitor_pattern_timeout_ms"} {
					assert.NotContains(t, panermCheckText(&check), token, "%s after auto update", check.ID)
				}
			}
			textAfter, _ := runPanermDoctor(t, root, false)
			assert.NotContains(t, textAfter, "legacy orchestra keys:")
			assert.NotContains(t, textAfter, "stale completion hooks:")
		})
	}
}

func uniqueSorted(items []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			out = append(out, item)
		}
	}
	sort.Strings(out)
	return out
}

// W-mix: claude-code is configured and opencode is not, and opencode.json loads
// the orphan .ts. Only the opencode update edits opencode.json, and every
// update keeps a script a settings file still names, so after any number of
// updates doctor still lists the .ts. The remedy says what clears it instead
// of run "auto update", and doing that clears it.
func TestPanermDoctor_ScriptThatAnUnconfiguredOpenCodeLoadsGetsTheRemedyThatClearsIt(t *testing.T) {
	useStaleHookEnv(t, "")
	root := copyStaleHookWorkspace(t, "W-mix")
	out, err := runStaleHookUpdate(t, root)
	require.NoError(t, err, out)
	const wantRemedy = "opencode is not configured, so auto update never edits opencode.json and keeps the scripts " +
		"it loads (.claude/hooks/autopus/hook-opencode-complete.ts): remove those plugin entries from opencode.json " +
		`by hand or run "auto platform add opencode", then run "auto update"`

	_, checks := runPanermDoctor(t, root, true)

	stale := findPanermCheck(checks, panermStaleCheck)
	require.NotNil(t, stale)
	assert.Equal(t, "warn", stale.Status)
	assert.Equal(t, "stale completion hooks: .claude/hooks/autopus/hook-opencode-complete.ts", stale.Detail)
	assert.Equal(t, wantRemedy, stale.Fields["remedy"])
	text, _ := runPanermDoctor(t, root, false)
	assert.Contains(t, text, "remedy: "+wantRemedy)
	assert.NotContains(t, doctorResultBanner(text), "auto update", "the summary must not send the user to update either")

	require.NoError(t, os.WriteFile(filepath.Join(root, "opencode.json"), []byte(`{"plugin": []}`+"\n"), 0o644))
	out, err = runStaleHookUpdate(t, root)
	require.NoError(t, err, out)
	_, after := runPanermDoctor(t, root, true)
	assert.Equal(t, "pass", findPanermCheck(after, panermStaleCheck).Status, "removing the plugin entry clears it")
}

// doctorResultBanner returns the closing result box of a text doctor run as
// one line: the box wraps its sentence across bordered lines.
func doctorResultBanner(text string) string {
	start := strings.LastIndex(text, "\u256d")
	if start < 0 {
		return ""
	}
	box := strings.NewReplacer("\u2502", " ", "\u256d", " ", "\u256e", " ", "\u2570", " ", "\u256f", " ", "\u2500", " ").
		Replace(text[start:])
	return strings.Join(strings.Fields(box), " ")
}
