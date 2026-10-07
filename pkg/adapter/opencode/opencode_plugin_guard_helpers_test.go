package opencode

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/content"
	"github.com/insajin/autopus-adk/pkg/editguard"
)

// guardHelperEnv turns this test binary into the real `auto guard edit` when a
// generated plugin spawns it through the `auto` symlink a test puts on PATH.
const guardHelperEnv = "AUTOPUS_OPENCODE_GUARD_HELPER"

func TestMain(m *testing.M) {
	if os.Getenv(guardHelperEnv) == "1" {
		os.Exit(runGuardHelper(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// runGuardHelper runs the decision engine and OpenCode dialect behind the
// argv the plugin spawns, which is what `auto guard edit --platform opencode`
// runs after its environment switch.
func runGuardHelper(args []string) int {
	if !slices.Equal(args, []string{"guard", "edit", "--platform", editguard.PlatformOpenCode}) {
		fmt.Fprintf(os.Stderr, "unexpected guard argv %q\n", args)
		return 1
	}
	dialect, _ := editguard.DialectFor(editguard.PlatformOpenCode)
	return editguard.Run(os.Stdin, os.Stdout, os.Stderr, dialect, editguard.Options{})
}

const (
	guardSkill = ".claude/skills/auto-fix/SKILL.md"
	// guardGSCon is the GS-CON reason of the guard for guardSkill in a
	// consumer project (spec.md Decision Output Contract).
	guardGSCon = "autopus edit-guard [generated_surface]: .claude/skills/auto-fix/SKILL.md is generated " +
		"(manifest .autopus/claude-code-manifest.json, policy always). " +
		"Change autopus.yaml or the upstream Autopus source, then run: auto update"
)

// guardAuto says which `auto` the plugin finds on PATH.
type guardAuto int

const (
	autoStub   guardAuto = iota // the recording stub
	autoReal                    // this test binary as the real guard
	autoAbsent                  // no auto at all
)

type pluginCall struct {
	Event map[string]any `json:"event"`
	Mode  *string        `json:"mode,omitempty"`
}

type pluginResult struct {
	Outcome string `json:"outcome"`
	Message string `json:"message"`
	MS      int    `json:"ms"`
}

// guardHarness runs one generated plugin under node against a consumer project
// whose manifest lists guardSkill as always.
type guardHarness struct {
	t                     *testing.T
	node, root, stubState string
	toolDirs              []string
}

func newGuardHarness(t *testing.T) *guardHarness {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the generated plugin fixture needs POSIX sh and process groups; skipped is not runtime verification")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("the generated plugin fixture needs Node; skipped is not runtime verification")
	}
	h := &guardHarness{t: t, node: node, stubState: t.TempDir()}
	for _, tool := range []string{"sh", "cat", "sleep"} {
		path, err := exec.LookPath(tool)
		if err != nil {
			t.Skipf("the generated plugin fixture needs POSIX command %s", tool)
		}
		if dir := filepath.Dir(path); !slices.Contains(h.toolDirs, dir) {
			h.toolDirs = append(h.toolDirs, dir)
		}
	}
	for _, dir := range h.toolDirs {
		if _, err := os.Stat(filepath.Join(dir, "auto")); err == nil {
			t.Skipf("%s holds an auto binary, so no PATH can be built without one", dir)
		}
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	h.root = root
	for rel, body := range map[string]string{
		"autopus.yaml":                       "project:\n  name: opencode-guard\n",
		".autopus/claude-code-manifest.json": `{"files":{"` + guardSkill + `":{"checksum":"x","policy":"always"}}}`,
		guardSkill:                           "original line\n",
		"pkg/a.go":                           "package pkg\n",
		"pkg/movable.go":                     "package pkg // movable\n",
	} {
		full := filepath.Join(root, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(body), 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(h.stubState, "mode"), nil, 0o600))
	return h
}

// renderGuardPlugin renders the version's plugin with two shell-tool hooks
// that log to hook.log and the guard registration pkg/content generates for
// OpenCode, its timeout replaced when timeout is positive.
func renderGuardPlugin(t *testing.T, version string, timeout int) string {
	t.Helper()
	generated, _, err := content.GenerateProjectHookConfigs(config.DefaultFullConfig("guard"), adapterName, true)
	require.NoError(t, err)
	var guards []adapter.HookConfig
	for _, hook := range generated {
		if strings.Contains(hook.Command, "auto guard edit") {
			guards = append(guards, hook)
		}
	}
	require.Len(t, guards, 1, "pkg/content registers one OpenCode guard")
	if timeout > 0 {
		guards[0].Timeout = timeout
	}
	hooks := []adapter.HookConfig{
		{Event: "PreToolUse", Matcher: "Bash", Command: "printf before >> hook.log", Timeout: 5},
		guards[0],
		{Event: "PostToolUse", Matcher: "Bash", Command: "printf after >> hook.log", Timeout: 5},
	}
	render := renderHookPluginV2
	if version == "v1" {
		render = renderHookPlugin
	}
	body, err := render(hooks)
	require.NoError(t, err)
	return body
}

// a2Calls replays the execute.before events OpenCode 2.0.10 sent in probe A2.
// V1 receives them with its own spellings, which no V1 host has confirmed
// (CD-1): tool bash for shell and argument key filePath for path.
func (h *guardHarness) a2Calls(version string) []pluginCall {
	h.t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "opencode-2.0.10-execute-before.jsonl"))
	require.NoError(h.t, err)
	var calls []pluginCall
	for _, line := range strings.Split(strings.TrimSpace(h.withRoot(string(raw))), "\n") {
		var fixture struct {
			Event map[string]any `json:"event"`
		}
		require.NoError(h.t, json.Unmarshal([]byte(line), &fixture))
		if version == "v1" {
			fixture.Event = v1Spelling(fixture.Event)
		}
		calls = append(calls, pluginCall{Event: fixture.Event})
	}
	return calls
}

func v1Spelling(event map[string]any) map[string]any {
	if event["tool"] == "shell" {
		event["tool"] = "bash"
	}
	if input, ok := event["input"].(map[string]any); ok {
		if path, ok := input["path"]; ok {
			delete(input, "path")
			input["filePath"] = path
		}
	}
	return event
}

// withRoot replaces the probe root placeholder with the JSON-escaped root.
func (h *guardHarness) withRoot(text string) string {
	quoted, err := json.Marshal(h.root)
	require.NoError(h.t, err)
	return strings.ReplaceAll(text, "<R>", string(quoted[1:len(quoted)-1]))
}

// run replays calls through plugin under node with the given `auto` on PATH.
func (h *guardHarness) run(plugin, version string, auto guardAuto, calls []pluginCall) []pluginResult {
	h.t.Helper()
	work := h.t.TempDir()
	pluginPath := filepath.Join(work, "plugin.mjs")
	require.NoError(h.t, os.WriteFile(pluginPath, []byte(plugin), 0o600))
	callsPath := filepath.Join(work, "calls.json")
	encoded, err := json.Marshal(calls)
	require.NoError(h.t, err)
	require.NoError(h.t, os.WriteFile(callsPath, encoded, 0o600))
	driver, err := filepath.Abs(filepath.Join("testdata", "plugin_guard_driver.mjs"))
	require.NoError(h.t, err)

	bin := h.t.TempDir()
	switch auto {
	case autoStub:
		writeOpenCodeGuardStub(h.t, bin)
	case autoReal:
		self, err := os.Executable()
		require.NoError(h.t, err)
		require.NoError(h.t, os.Symlink(self, filepath.Join(bin, "auto")))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, h.node, driver, pluginPath, version, h.root, callsPath)
	cmd.Env = []string{
		"PATH=" + strings.Join(append([]string{bin}, h.toolDirs...), string(os.PathListSeparator)),
		"HOME=" + h.t.TempDir(), "GUARD_STUB=" + h.stubState, "GUARD_STUB_MODE=" + filepath.Join(h.stubState, "mode"),
		guardHelperEnv + "=1",
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	require.NoError(h.t, err, stderr.String())
	var results []pluginResult
	require.NoError(h.t, json.Unmarshal(out, &results), string(out))
	require.Len(h.t, results, len(calls))
	return results
}

// writeOpenCodeGuardStub installs an `auto` that records its stdin and argv in
// $GUARD_STUB and answers as the mode file says: deny0 prints a deny and exits
// 0, deny2 prints it and exits 2, sleep outlives the hook timeout, text and
// noreason print a non-decision, and anything else prints nothing.
func writeOpenCodeGuardStub(t *testing.T, bin string) {
	t.Helper()
	script := `#!/bin/sh
cat >> "$GUARD_STUB/stdin.log"
printf '\n' >> "$GUARD_STUB/stdin.log"
printf '%s\n' "$*" >> "$GUARD_STUB/args.log"
mode=$(cat "$GUARD_STUB_MODE")
case "$mode" in
  deny0) printf '%s' '{"decision":"deny","reason":"R1"}' ;;
  deny2) printf '%s' '{"decision":"deny","reason":"R1"}'; exit 2 ;;
  sleep) sleep 5 ;;
  text) printf 'deny R1' ;;
  noreason) printf '%s' '{"decision":"deny"}' ;;
esac
exit 0
`
	require.NoError(t, os.WriteFile(filepath.Join(bin, "auto"), []byte(script), 0o755))
}

// stubLog returns the lines the recording stub wrote to name.
func (h *guardHarness) stubLog(name string) []string {
	h.t.Helper()
	raw, err := os.ReadFile(filepath.Join(h.stubState, name))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(h.t, err)
	return strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
}

func modeOf(mode string) *string { return &mode }
