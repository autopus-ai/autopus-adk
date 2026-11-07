package cli

// SPEC-REA-001 T11 (acceptance S3): the REA decision inside the real
// `auto init`. Only the Node exec seam, the TTY seam, the locale seam, and
// the wizard result are stand-ins; the command, the save, and the platform
// adapters run for real.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/reamcp"
)

// reaAllTargets is the P3 platform list.
const reaAllTargets = "claude-code,codex,antigravity-cli"

// reaCLIEnv isolates REA command runs: a sandbox HOME, CODEX_HOME, and empty
// PATH, Korean text, and a Node seam answering stdout with exit 0.
func reaCLIEnv(t *testing.T, stdout string) *reaNodeExecRecorder {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	originalLanguage, originalExec := composeLanguage, reaNodeExec
	t.Cleanup(func() { composeLanguage, reaNodeExec = originalLanguage, originalExec })
	composeLanguage = func() string { return "ko" }
	recorder := &reaNodeExecRecorder{stdout: stdout}
	reaNodeExec = recorder.exec
	return recorder
}

// runReaCLI executes the real root command over input.
func runReaCLI(t *testing.T, input string, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(input))
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

// reaInit runs `auto init` for platforms in dir with extra arguments.
func reaInit(t *testing.T, dir, platforms string, args ...string) (string, error) {
	t.Helper()
	return runReaCLI(t, "", append([]string{"init", "--dir", dir, "--project", "p", "--platforms", platforms}, args...)...)
}

// stubReaInitWizard answers the wizard with answer for REA and its defaults
// elsewhere, and records every call's options.
func stubReaInitWizard(t *testing.T, answer bool) *[]tui.InitWizardOpts {
	t.Helper()
	stubDriverProbes(t, true, "")
	original := runInitWizard
	t.Cleanup(func() { runInitWizard = original })
	calls := &[]tui.InitWizardOpts{}
	runInitWizard = func(opts tui.InitWizardOpts) (*tui.InitWizardResult, error) {
		*calls = append(*calls, opts)
		return &tui.InitWizardResult{
			CommentsLang: "en", CommitsLang: "ko", AILang: "ko", Quality: opts.Quality,
			Methodology: "tdd", UsageProfile: "developer", Rea: &answer,
		}, nil
	}
	return calls
}

// assertReaManagedOnce checks that each target file holds exactly one rea
// entry and that it is the MCP Entry Shape Contract's managed shape.
func assertReaManagedOnce(t *testing.T, dir string) {
	t.Helper()
	for _, p := range reamcp.Platforms() {
		data := readReaFile(t, dir, p)
		class, err := reamcp.ClassifyFile(p, data)
		require.NoError(t, err, p)
		assert.Equal(t, reamcp.ClassManaged, class, p)
		if p == reamcp.Codex {
			assert.Equal(t, 1, strings.Count(string(data), "[mcp_servers.rea]"), p)
			continue
		}
		var doc struct {
			MCPServers map[string]any `json:"mcpServers"`
		}
		require.NoError(t, json.Unmarshal(data, &doc), p)
		assert.Equal(t, reamcp.ManagedJSONEntry(p), doc.MCPServers["rea"], p)
		assert.Equal(t, 1, strings.Count(string(data), `"rea": {`), p)
	}
}

// assertReaAbsent checks that no target file names rea-agents.
func assertReaAbsent(t *testing.T, dir string) {
	t.Helper()
	for _, p := range reamcp.Platforms() {
		assert.NotContains(t, string(readReaFile(t, dir, p)), "rea-agents", p)
	}
}

// reaKeyLine matches the features.rea block config.Save writes for value.
func reaKeyLine(value bool) *regexp.Regexp {
	return regexp.MustCompile(fmt.Sprintf(`(?m)^features:\n(?:\s+.*\n)*?\s+rea:\n\s+enabled: %t$`, value))
}

// reaYAML returns the project's autopus.yaml text.
func reaYAML(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	return string(data)
}

// S3 (1), (4), (5), (8), (9): non-interactive init leaves REA off unless --rea,
// --no-rea stores false once, both flags fail before any write, and a re-init
// carries the stored key; --rea over it corrects false to true.
func TestInitRea_NonInteractiveDecisions(t *testing.T) {
	reaCLIEnv(t, "v24.11.0\n")

	// REA is opt-in since v0.51.0: a plain --yes leaves it undecided and
	// registers nothing.
	undecided := t.TempDir()
	out, err := reaInit(t, undecided, reaAllTargets, "--yes")
	require.NoError(t, err, out)
	assertReaAbsent(t, undecided)
	assert.NotRegexp(t, `(?m)^\s+rea:`, reaYAML(t, undecided))
	assert.Zero(t, strings.Count(out, "REA 역공학 MCP를 등록했습니다"), out)
	assert.Zero(t, strings.Count(out, reaTextKo.OldBinary), out)

	on := t.TempDir()
	out, err = reaInit(t, on, reaAllTargets, "--yes", "--rea")
	require.NoError(t, err, out)
	assertReaManagedOnce(t, on)
	assert.NotRegexp(t, `(?m)^\s+rea:`, reaYAML(t, on))
	assert.Equal(t, 1, strings.Count(out, "REA 역공학 MCP를 등록했습니다: .mcp.json, .codex/config.toml, .agents/plugins/autopus/mcp_config.json (rea-agents@6.5.0)"), out)
	assert.Zero(t, strings.Count(out, reaTextKo.OldBinary))

	off := t.TempDir()
	out, err = reaInit(t, off, reaAllTargets, "--yes", "--no-rea")
	require.NoError(t, err, out)
	assertReaAbsent(t, off)
	assert.Regexp(t, reaKeyLine(false), reaYAML(t, off))
	assert.Equal(t, 1, strings.Count(out, reaTextKo.OldBinary), out)

	both := t.TempDir()
	out, err = reaInit(t, both, reaAllTargets, "--yes", "--rea", "--no-rea")
	require.Error(t, err)
	assert.Contains(t, err.Error()+out, "--rea와 --no-rea는 함께 쓸 수 없습니다")
	assert.NoFileExists(t, filepath.Join(both, "autopus.yaml"))
	assert.NoFileExists(t, filepath.Join(both, ".mcp.json"))

	out, err = reaInit(t, off, reaAllTargets, "--yes") // (8) E
	require.NoError(t, err, out)
	assertReaAbsent(t, off)
	assert.Regexp(t, reaKeyLine(false), reaYAML(t, off))
	assert.Zero(t, strings.Count(out, reaTextKo.OldBinary), out)

	out, err = reaInit(t, off, reaAllTargets, "--yes", "--rea") // (9) E2
	require.NoError(t, err, out)
	assertReaManagedOnce(t, off)
	assert.Regexp(t, reaKeyLine(true), reaYAML(t, off))
	assert.Zero(t, strings.Count(out, reaTextKo.OldBinary), out)
}

// S3 (2), (3): the interactive wizard asks while REA is undecided, and its
// answer decides; a flag or a stored decision presets the step away.
func TestInitRea_WizardAnswer(t *testing.T) {
	reaCLIEnv(t, "v24.11.0\n")

	yes := t.TempDir()
	calls := stubReaInitWizard(t, true)
	out, err := reaInit(t, yes, reaAllTargets, "--quality", "balanced")
	require.NoError(t, err, out)
	require.Len(t, *calls, 1)
	assert.Nil(t, (*calls)[0].Rea, "an undecided init asks")
	assertReaManagedOnce(t, yes)
	assert.NotRegexp(t, `(?m)^\s+rea:`, reaYAML(t, yes))
	assert.Equal(t, 1, strings.Count(out, "REA 역공학 MCP를 등록했습니다"), out)

	no := t.TempDir()
	calls = stubReaInitWizard(t, false)
	out, err = reaInit(t, no, reaAllTargets, "--quality", "balanced")
	require.NoError(t, err, out)
	assertReaAbsent(t, no)
	assert.Regexp(t, reaKeyLine(false), reaYAML(t, no))
	assert.Equal(t, 1, strings.Count(out, reaTextKo.OldBinary), out)

	out, err = reaInit(t, no, reaAllTargets, "--quality", "balanced")
	require.NoError(t, err, out)
	require.Len(t, *calls, 2)
	require.NotNil(t, (*calls)[1].Rea, "the stored decision presets the step")
	assert.False(t, *(*calls)[1].Rea)
}

// S3 (6), (7): a failing Node gate stores true with one Node line and one
// old-binary line; OMP and OpenCode alone get only the guidance line.
func TestInitRea_NodeGateAndGuidance(t *testing.T) {
	recorder := reaCLIEnv(t, "v20.11.0\n")

	n20 := t.TempDir()
	out, err := reaInit(t, n20, reaAllTargets, "--yes", "--rea")
	require.NoError(t, err, out)
	assertReaAbsent(t, n20)
	assert.Regexp(t, reaKeyLine(true), reaYAML(t, n20))
	assert.Equal(t, 1, strings.Count(out, "node v20.11.0은 ^22.19.0 || ^24.11.0 || >=26.0.0 범위가 아닙니다"), out)
	assert.Equal(t, 1, strings.Count(out, reaTextKo.OldBinary), out)
	assert.Len(t, recorder.commands, 1)

	recorder.commands = nil
	guided := t.TempDir()
	out, err = reaInit(t, guided, "opencode,omp", "--yes")
	require.NoError(t, err, out)
	assert.Equal(t, 1, strings.Count(out, reaTextKo.Guidance), out)
	assert.Zero(t, strings.Count(out, reaTextKo.Question))
	assert.Zero(t, strings.Count(out, reaTextKo.Hint))
	assert.NotRegexp(t, `(?m)^\s+rea:`, reaYAML(t, guided))
	assert.Empty(t, recorder.commands, "no REA target platform, no probe")
}
