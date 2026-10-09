package cli

// SPEC-PANERM-001 fix round: provider keys and settings event names come from
// user files, so every line that prints them to a terminal escapes control
// and format runes instead of letting them move the cursor or retitle the
// window. Printable text, including non-ASCII, is unchanged.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

const escProvider = "\x1b]0;PWNED\a\x1b[2K"

func TestTerminalSafe_EscapesOnlyUnprintableRunes(t *testing.T) {
	t.Parallel()
	for input, want := range map[string]string{
		"orchestra.providers.claude.pane_args": "orchestra.providers.claude.pane_args",
		"orchestra.providers.클로드.pane_args":    "orchestra.providers.클로드.pane_args",
		escProvider:           `\x1b]0;PWNED\a\x1b[2K`,
		"a\u202eb\u0085c\x7f": "a\\u202eb\\u0085c\\x7f",
		"bad\xffbyte\n":       `bad\xffbyte\n`,
	} {
		assert.Equal(t, want, terminalSafe(input), "%q", input)
		assert.Equal(t, want, terminalSafe(terminalSafe(input)), "idempotent for %q", input)
	}
}

func TestConfigNotice_EscapesControlRunesInPaths(t *testing.T) {
	t.Parallel()
	notice := newConfigNotice(alwaysTerminal)
	cmd, _, stderr := noticeProbeCommand(t, false)
	notice.bind(cmd)

	notice.report([]string{"orchestra.providers." + escProvider + ".pane_args"})

	assert.NotContains(t, stderr.String(), "\x1b")
	assert.Contains(t, stderr.String(), `orchestra.providers.\x1b]0;PWNED\a\x1b[2K.pane_args; the orchestra pane`)
}

func TestUpdateMigrationsAndDoctor_EscapeControlRunesFromUserFiles(t *testing.T) {
	isolateDoctorEnv(t)
	yamlKey := `"\e]0;PWNED\a\e[2K"`
	dir := legacyPaneDir(t, []byte("orchestra:\n  providers:\n    "+yamlKey+":\n      pane_args: []\n"))
	writeDoctorFixture(t, dir, map[string]string{
		".claude/settings.json": `{"hooks":{"Stop\u001b[2K":[{"hooks":[{"command":".claude/hooks/autopus/hook-claude-stop.sh"}]}]}}`,
	})

	var text bytes.Buffer
	checkRetiredOrchestraText(&text, dir, doctorConfigFor("claude-code"))
	assert.NotContains(t, text.String(), "\x1b")
	assert.Contains(t, text.String(), `orchestra.providers.\x1b]0;PWNED\a\x1b[2K.pane_args`)
	assert.Contains(t, text.String(), `.claude/settings.json Stop\x1b[2K .claude/hooks/autopus/hook-claude-stop.sh`)

	var out bytes.Buffer
	require.NoError(t, persistUpdateConfigMigrations(&out, dir, &config.HarnessConfig{}, false))
	assert.NotContains(t, out.String(), "\x1b")
	assert.True(t, strings.Contains(out.String(), `orchestra.providers.\x1b]0;PWNED\a\x1b[2K.pane_args`), out.String())
}

// doctor --json carries the same user-derived members as the text report, and
// encoding/json escapes C0 controls but leaves a bidi override, DEL, or C1
// control in the bytes, so the JSON check escapes them the way the text does.
func TestRetiredOrchestraChecks_JSONEscapesUnprintableRunesFromUserFiles(t *testing.T) {
	isolateDoctorEnv(t)
	root := filepath.Join(t.TempDir(), "proj\u202ex")
	require.NoError(t, os.Mkdir(root, 0o755))
	writeDoctorFixture(t, root, map[string]string{
		"autopus.yaml":          "orchestra:\n  providers:\n    \"a\\u202Eb\\x7F\":\n      pane_args: []\n",
		".claude/settings.json": `{"hooks":{"Stop\u202e":[{"hooks":[{"command":".claude/hooks/autopus/hook-claude-stop.sh"}]}]}}`,
	})
	require.NoError(t, os.Mkdir(filepath.Join(root, "opencode.json"), 0o755), "a config update cannot read")

	report := doctorJSONReport{status: jsonStatusOK}
	report.collectRetiredOrchestraChecks(root, doctorConfigFor("claude-code", "opencode"))

	byID := map[string]jsonCheck{}
	for _, check := range report.checks {
		byID[check.ID] = check
	}
	assert.Equal(t, `legacy orchestra keys: orchestra.providers.a\u202eb\x7f.pane_args`,
		byID[legacyOrchestraConfigCheckID].Detail)
	stale := byID[staleCompletionHooksCheckID]
	assert.Equal(t, `stale completion hooks: .claude/settings.json Stop\u202e `+claudeStopScript, stale.Detail)
	assert.Contains(t, stale.Fields["opencode_config_error"], `proj\u202ex`)
	encoded, err := json.Marshal(report.checks)
	require.NoError(t, err)
	assert.NotContains(t, string(encoded), "\u202e")
	assert.NotContains(t, string(encoded), "\x7f")
}

func TestPruneRetiredConfig_RewriteErrorEscapesTheUserPath(t *testing.T) {
	t.Parallel()
	// The flow entry cannot be cut by lines, and the reserved block's empty
	// explicit key does not survive yaml.v3's re-encode, so the rewrite fails
	// and names the retired path it could not remove.
	data := anchorTestBase + "future_extension: {? : v}\norchestra:\n  providers:\n    \"a\\u202Eb\": {pane_args: []}\n"

	_, _, err := pruneRetiredConfig([]byte(data))

	require.ErrorContains(t, err, `remove retired orchestra keys orchestra.providers.a\u202eb.pane_args`)
	assert.NotContains(t, err.Error(), "\u202e")
}
