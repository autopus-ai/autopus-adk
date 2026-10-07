package cli

// SPEC-PANERM-001 fix round: provider keys and settings event names come from
// user files, so every line that prints them to a terminal escapes control
// and format runes instead of letting them move the cursor or retitle the
// window. Printable text, including non-ASCII, is unchanged.

import (
	"bytes"
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
