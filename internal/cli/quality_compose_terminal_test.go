package cli

// SPEC-MODELUX-001 REQ-MUX-05, security review F3: text read from
// autopus.yaml reaches the terminal only through terminalSafe, which spells
// out control characters (ESC, BEL) and format characters (bidi overrides,
// zero-width spaces) alike. A preset description could otherwise set the
// clipboard (OSC 52), retitle the terminal, or reverse the line it prints on.

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// terminalHostile holds an OSC 52 clipboard write, an OSC 0 title, a
// right-to-left override, and a zero-width space.
const terminalHostile = "\x1b]52;c;ZWNobyBwd25lZA==\a\x1b]0;owned\a\u202ereversed\u200b"

// terminalHostileYAML spells terminalHostile with YAML double-quoted escapes.
const terminalHostileYAML = `\e]52;c;ZWNobyBwd25lZA==\a\e]0;owned\a\u202ereversed\u200b`

// assertNoTerminalControls checks that out carries none of the raw
// characters terminalHostile holds.
func assertNoTerminalControls(t *testing.T, out string) {
	t.Helper()
	for _, raw := range []string{"\x1b", "\a", "\u202e", "\u200b"} {
		assert.NotContains(t, out, raw)
	}
}

// The global quality menu prints each preset's description; it escapes one
// written with YAML escapes (F3, moved code).
func TestQualityGlobalMenu_EscapesTerminalControlsInADescription(t *testing.T) {
	stubDriverProbes(t, true, "")
	setComposeLanguage(t, "en")
	dir := writeBlockFixture(t, "project_name: demo\nmode: full\nplatforms: [claude-code]\nquality:\n"+
		"  default: balanced\n  presets:\n    balanced:\n      description: \""+terminalHostileYAML+"\"\n"+
		"    ultra:\n      description: u\n")
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	require.Equal(t, terminalHostile, cfg.Quality.Presets["balanced"].Description)

	out, err := runQualityCompose(t, dir, strings.NewReader("global\n1\n"))

	require.NoError(t, err, out)
	assert.Contains(t, out, "balanced - "+terminalSafe(terminalHostile)+"\n")
	assertNoTerminalControls(t, out)
}

// The composition status line, the preview change lines, and the update
// warning escape format characters as well as control characters.
func TestComposeTerminalText_EscapesFormatCharacters(t *testing.T) {
	t.Parallel()
	name := "fast\u202e\u200b"

	quality := driverConfig().Quality
	quality.Default = name
	status := composeDriverStatus(quality, composeTextFor("en"))
	assert.True(t, strings.HasPrefix(status, "quality.default = "+terminalSafe(name)+"   "), status)
	assertNoTerminalControls(t, status)

	cfg := config.DefaultFullConfig("p")
	cfg.Quality.Presets[name] = config.QualityPreset{Agents: map[string]string{"planner": "haiku"}}
	cfg.Quality.Default = name
	start := newQualityComposition(cfg.Quality, composeGroupsSources(t))
	plan := planComposeBinding(cfg, start, composeBindEdited(t, start, map[string]string{"planner": "opus"}))
	lines := describeComposeBinding(cfg, plan, driftGenerationSnapshot{}, nil, "en")
	assert.Contains(t, lines, "quality.default "+terminalSafe(name)+" → custom")
	assertNoTerminalControls(t, strings.Join(lines, "\n"))

	var out bytes.Buffer
	warnUpdateCompose(&out, "en", errors.New("refused "+terminalHostile))
	assert.Equal(t, "  warning: refused "+terminalSafe(terminalHostile)+"\n", out.String())
	assertNoTerminalControls(t, out.String())
}
