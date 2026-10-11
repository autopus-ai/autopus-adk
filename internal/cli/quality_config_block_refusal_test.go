package cli

// SPEC-MODELUX-001 REQ-MUX-08, acceptance S13: the composition step's writes
// refuse a quality block that carries an environment value or a YAML
// reference and keep the file byte-identical, while the one-line subcommands
// keep their behavior (REQ-MUX-16).

import (
	"crypto/rand"
	"crypto/sha256"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// randomAnchor returns a fresh anchor name, so the refusal does not hinge on
// the name F-001 happened to use.
func randomAnchor(t *testing.T) string {
	t.Helper()
	return "r" + strings.ToLower(rand.Text())
}

func blockRefusalFixtures(t *testing.T) map[string]struct {
	content string
	setEnv  bool
} {
	anchor := randomAnchor(t)
	base := "project_name: demo\nmode: full\nplatforms: [claude-code]\n"
	presetsTail := "    ultra:\n      description: u\n"
	return map[string]struct {
		content string
		setEnv  bool
	}{
		"F1 with the variable set":   {content: blockLeakAlias, setEnv: true},
		"F1 with the variable unset": {content: blockLeakAlias},
		"F2 with the variable set":   {content: blockLeakDirect, setEnv: true},
		"F3 anchor and alias inside": {content: blockInnerAlias},
		"random anchor on an outside scalar": {setEnv: true, content: base +
			"future_extension:\n  note: &" + anchor + " '${PRESET_NOTE}'\n" +
			"quality:\n  default: balanced\n  presets:\n    balanced:\n      description: *" + anchor + "\n" + presetsTail},
		"random anchor on an outside mapping": {setEnv: true, content: base +
			"future_extension:\n  tiers: &" + anchor + "\n    planner: opus\n" +
			"quality:\n  default: balanced\n  presets:\n    balanced:\n      agents: *" + anchor + "\n" + presetsTail},
		"merge key from a random anchor": {setEnv: true, content: base +
			"future_extension:\n  base: &" + anchor + "\n    description: '${PRESET_NOTE}'\n" +
			"quality:\n  default: balanced\n  presets:\n    balanced:\n      <<: *" + anchor + "\n" + presetsTail},
		"merge key with an inline mapping": {content: base +
			"quality:\n  default: balanced\n  presets:\n    balanced:\n      <<: {description: b}\n" + presetsTail},
	}
}

func setPresetNote(t *testing.T, set bool) {
	t.Helper()
	t.Setenv("PRESET_NOTE", presetNoteSecret)
	if !set {
		require.NoError(t, os.Unsetenv("PRESET_NOTE"))
	}
}

func TestSaveQualityBlock_RefusesEnvValuesAndReferencesAndKeepsTheFile(t *testing.T) {
	for name, fixture := range blockRefusalFixtures(t) {
		t.Run(name, func(t *testing.T) {
			setPresetNote(t, fixture.setEnv)
			dir := writeBlockFixture(t, fixture.content)
			want := sha256.Sum256([]byte(fixture.content))
			cfg, err := config.LoadPreview(dir)
			require.NoError(t, err)
			loadedQuality := cfg.Quality

			err = saveQualityBlock(dir, cfg, compositionCustomEdit)

			require.Error(t, err)
			assert.Equal(t, qualityWriteRefusalText, err.Error())
			written := readBlockFixture(t, dir)
			assert.Equal(t, want, sha256.Sum256([]byte(written)), "a refused write leaves the file byte-identical")
			assert.NotContains(t, written, presetNoteSecret)
			assert.Equal(t, loadedQuality, cfg.Quality, "a refused write leaves the caller's config alone")
		})
	}
}

// The composition step's line write runs the same precheck (REQ-MUX-08), so a
// composition refuses a reference file whichever writer its plan takes.
func TestPersistComposeBinding_LineWrite_RefusesEnvValuesAndReferencesInQuality(t *testing.T) {
	for name, fixture := range blockRefusalFixtures(t) {
		t.Run(name, func(t *testing.T) {
			setPresetNote(t, fixture.setEnv)
			dir := writeBlockFixture(t, fixture.content)
			want := sha256.Sum256([]byte(fixture.content))
			cfg, err := config.LoadPreview(dir)
			require.NoError(t, err)

			err = persistComposeBinding(dir, cfg, composeBindingPlan{Writer: composeWriterLine, Default: "ultra"})

			require.ErrorIs(t, err, errQualityWriteRefused)
			assert.Equal(t, qualityWriteRefusalText, err.Error())
			written := readBlockFixture(t, dir)
			assert.Equal(t, want, sha256.Sum256([]byte(written)), "the line write leaves the file byte-identical")
			assert.NotContains(t, written, presetNoteSecret)
			assert.Equal(t, "balanced", cfg.Quality.Default, "a refused write leaves the caller's config alone")
		})
	}
}

// REQ-MUX-16 keeps the subcommands that take arguments as they were: their
// one-line writer replaces or inserts a validated preset name or policy, which
// cannot carry an expanded value, and verifyQualityLineEdit already refuses an
// edit that would rebind an alias. So they still edit a file whose quality
// block holds a YAML reference, changing that one line alone.
func TestQualityLineCommands_EditAReferenceFileAsBefore(t *testing.T) {
	base := "project_name: demo\nmode: full\nplatforms: [claude-code]\n"
	fixtures := map[string]string{
		"anchor and alias inside quality": blockInnerAlias,
		"agents alias inside quality": base + "quality:\n  default: balanced\n  presets:\n" +
			"    balanced:\n      agents: &base\n        planner: opus\n" +
			"    ultra:\n      description: u\n    fast:\n      agents: *base\n",
		"merge key with an inline mapping": base + "quality:\n  default: balanced\n  presets:\n" +
			"    balanced:\n      <<: {description: b}\n    ultra:\n      description: u\n",
		"F1 with the variable unset": blockLeakAlias,
	}
	commands := map[string]struct {
		args []string
		line string
	}{
		"auto quality ultra": {args: []string{"ultra"}, line: "  default: ultra\n"},
		"auto quality supervisor inherit": {
			args: []string{"supervisor", "inherit"},
			line: "  default: balanced\n  supervisor_model_policy: inherit\n",
		},
	}
	for name, content := range fixtures {
		for command, tc := range commands {
			t.Run(name+"/"+command, func(t *testing.T) {
				setPresetNote(t, false)
				dir := writeBlockFixture(t, content)

				require.NoError(t, runQualityCommand(t, filepath.Join(dir, "autopus.yaml"), tc.args...))

				assert.Equal(t, strings.Replace(content, "  default: balanced\n", tc.line, 1), readBlockFixture(t, dir))
			})
		}
	}
}

func TestQualityWriters_KeepAnUnsetPlaceholderLiterally(t *testing.T) {
	setPresetNote(t, false)

	t.Run("block writer", func(t *testing.T) {
		dir := writeBlockFixture(t, blockLeakDirect)
		cfg, err := config.LoadPreview(dir)
		require.NoError(t, err)

		require.NoError(t, saveQualityBlock(dir, cfg, compositionCustomEdit))

		written := readBlockFixture(t, dir)
		assert.Equal(t, "${PRESET_NOTE}", rawQualityOf(t, written).Presets["fast"].Description)
		assert.Equal(t, rawQualityOf(t, blockLeakDirect).Presets["balanced"], rawQualityOf(t, written).Presets["balanced"])
		assert.Equal(t, "custom", rawQualityOf(t, written).Default)
	})

	t.Run("line writer", func(t *testing.T) {
		dir := writeBlockFixture(t, blockLeakDirect)

		require.NoError(t, runQualityCommand(t, filepath.Join(dir, "autopus.yaml"), "ultra"))

		written := readBlockFixture(t, dir)
		assert.Equal(t, strings.Replace(blockLeakDirect, "  default: balanced\n", "  default: ultra\n", 1), written,
			"the line writer changes the default line alone")
	})
}

func TestSaveQualityBlock_RefusesAnInvalidResultAndKeepsTheFile(t *testing.T) {
	dir := writeBlockFixture(t, blockColumnPrefix+blockColumnQuality+blockColumnSuffix)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)

	err = saveQualityBlock(dir, cfg, func(quality *config.QualityConf) error {
		quality.Default = "missing"
		return nil
	})

	require.ErrorContains(t, err, `quality.default "missing" is not defined`)
	assert.Equal(t, blockColumnPrefix+blockColumnQuality+blockColumnSuffix, readBlockFixture(t, dir))
	assert.Equal(t, "balanced", cfg.Quality.Default)
}

func TestSaveQualityBlock_RefusesAFileTheLoaderRejects(t *testing.T) {
	original := "project_name: demo\nmode: full\nplatforms: [claude-code]\nquality:\n  default: missing\n"
	dir := writeBlockFixture(t, original)

	err := saveQualityBlock(dir, nil, compositionCustomEdit)

	require.ErrorContains(t, err, "load config")
	assert.Equal(t, original, readBlockFixture(t, dir))
}

func TestSaveQualityBlock_WithoutAConfigFileAnEditErrorCreatesNothing(t *testing.T) {
	dir := t.TempDir()

	err := saveQualityBlock(dir, nil, func(*config.QualityConf) error { return os.ErrInvalid })

	require.ErrorIs(t, err, os.ErrInvalid)
	_, statErr := os.Stat(filepath.Join(dir, "autopus.yaml"))
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
