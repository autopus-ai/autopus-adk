package cli

// SPEC-MODELUX-001 REQ-MUX-08 and SPEC-PANERM-001 REQ-10, security re-check
// of F1: a retired orchestra entry that does not own whole lines makes
// pruneRetiredConfig re-encode the whole document, and yaml.Marshal spells a
// value written "\x24{NAME}" as ${NAME}, which config.Load then expands. The
// prune refuses such a rewrite, so no writer that prunes first (the block
// writer, the composition line write, `auto quality <preset>`, `auto quality
// provider`, and the update cleanup) writes it, and the file stays as it is.

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// retiredPruneEscapedFixture keeps an escaped placeholder in project_name and
// in a quality description, and a retired entry that leaves its mapping empty.
const retiredPruneEscapedFixture = `project_name: "\x24{PRESET_NOTE}"
mode: full
platforms: [claude-code]
orchestra:
  subprocess:
    enabled: true
quality:
  default: balanced
  presets:
    balanced:
      description: "\x24{PRESET_NOTE}"
    ultra:
      description: u
`

func TestPruneRetiredConfig_RefusesAReencodeThatActivatesAPlaceholder(t *testing.T) {
	t.Parallel()
	for name, input := range map[string]string{
		"mapping left empty": retiredPruneEscapedFixture,
		"flow mapping": "project_name: demo\norchestra:\n  providers:\n    claude: {binary: claude, pane_args: [x]}\n" +
			"note: \"\\x24{PRESET_NOTE}\"\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, _, err := pruneRetiredConfig([]byte(input))

			require.ErrorIs(t, err, errRetiredPruneActivatesPlaceholder)
		})
	}
}

func TestRetiredPrune_EveryWriterRefusesAnActivatingReencodeAndKeepsTheFile(t *testing.T) {
	writers := map[string]func(t *testing.T, dir string) error{
		"block writer": func(t *testing.T, dir string) error {
			cfg, err := config.LoadPreview(dir)
			require.NoError(t, err)
			return saveQualityBlock(dir, cfg, compositionCustomEdit)
		},
		"composition line write": func(t *testing.T, dir string) error {
			cfg, err := config.LoadPreview(dir)
			require.NoError(t, err)
			return persistComposeBinding(dir, cfg, composeBindingPlan{Writer: composeWriterLine, Default: "ultra"})
		},
		"auto quality set ultra": func(t *testing.T, dir string) error {
			return runQualityCommand(t, filepath.Join(dir, "autopus.yaml"), "set", "ultra")
		},
		"auto quality provider claude ultra": func(t *testing.T, dir string) error {
			return runQualityCommand(t, filepath.Join(dir, "autopus.yaml"), "provider", "claude", "ultra")
		},
		"update cleanup": func(t *testing.T, dir string) error {
			return persistUpdateConfigMigrations(&bytes.Buffer{}, dir, &config.HarnessConfig{}, false)
		},
	}
	for name, write := range writers {
		for _, set := range []bool{true, false} {
			t.Run(name+map[bool]string{true: "/variable set", false: "/variable unset"}[set], func(t *testing.T) {
				setPresetNote(t, set)
				dir := writeBlockFixture(t, retiredPruneEscapedFixture)

				err := write(t, dir)

				require.ErrorIs(t, err, errRetiredPruneActivatesPlaceholder)
				assert.Equal(t, retiredPruneEscapedFixture, readBlockFixture(t, dir))
			})
		}
	}
}
