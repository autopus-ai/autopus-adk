package cli

// SPEC-MODELUX-001 REQ-MUX-08/09, acceptance S13: the quality writers build
// every written value from the raw quality node, refuse a block that carries
// an environment value or a YAML reference, and splice only the top-level
// quality block into the original bytes.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// qualityWriteRefusalText is the refusal REQ-MUX-08 prescribes word for word.
const qualityWriteRefusalText = "quality 블록에 환경 변수 값이나 YAML 참조(alias·merge)가 섞여 있어 자동으로 쓰지 않습니다. autopus.yaml을 직접 고치세요."

// presetNoteSecret stands in for an operator's environment value; it must
// never reach autopus.yaml.
const presetNoteSecret = "leak-value-123"

func writeBlockFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "autopus.yaml"), []byte(content), 0o644))
	return dir
}

func readBlockFixture(t *testing.T, dir string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "autopus.yaml"))
	require.NoError(t, err)
	return string(data)
}

// rawQualityOf decodes the quality block of data the way an older binary or
// a reviewer reads the file: no environment expansion.
func rawQualityOf(t *testing.T, data string) config.QualityConf {
	t.Helper()
	var doc struct {
		Quality config.QualityConf `yaml:"quality"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(data), &doc))
	return doc.Quality
}

// s6CustomAgents is the composition acceptance S6 confirms.
func s6CustomAgents() map[string]string {
	agents := map[string]string{"explorer": "haiku", "annotator": "haiku"}
	for _, name := range []string{"architect", "planner", "reviewer", "security-auditor", "spec-writer"} {
		agents[name] = "fable"
	}
	for _, name := range []string{
		"debugger", "deep-worker", "devops", "executor", "frontend-specialist",
		"perf-engineer", "tester", "ux-validator", "validator",
	} {
		agents[name] = "sonnet"
	}
	return agents
}

// compositionCustomEdit is the S6 write: presets.custom and quality.default.
func compositionCustomEdit(quality *config.QualityConf) error {
	if quality.Presets == nil {
		quality.Presets = map[string]config.QualityPreset{}
	}
	quality.Presets["custom"] = config.QualityPreset{Description: "역할 그룹 구성", Agents: s6CustomAgents()}
	quality.Default = "custom"
	return nil
}

// legacyQualitySchema is the quality schema before a0355acf added
// quality.providers. A binary of that era decodes the block strictly.
type legacyQualitySchema struct {
	Default               string `yaml:"default"`
	SupervisorModelPolicy string `yaml:"supervisor_model_policy"`
	Presets               map[string]struct {
		Description string            `yaml:"description"`
		Agents      map[string]string `yaml:"agents"`
	} `yaml:"presets"`
}

func crlf(s string) string { return strings.ReplaceAll(s, "\n", "\r\n") }

func TestSaveQualityBlock_SplicesOnlyTheQualityBlockAndKeepsEveryOuterByte(t *testing.T) {
	tests := []struct {
		name, prefix, quality, suffix string
	}{
		{name: "repo autopus.yaml copy", prefix: blockRepoPrefix, quality: blockRepoQuality, suffix: blockRepoSuffix},
		{name: "hand-written two-space file", prefix: blockHandPrefix, quality: blockHandQuality, suffix: blockHandSuffix},
		{
			name:   "hand-written two-space file with CRLF",
			prefix: crlf(blockHandPrefix), quality: crlf(blockHandQuality), suffix: crlf(blockHandSuffix),
		},
		{name: "quality last with mixed indentation", prefix: blockMixedPrefix, quality: blockMixedQuality},
		{
			name:   "column-one comment between quality children",
			prefix: blockColumnPrefix, quality: blockColumnQuality, suffix: blockColumnSuffix,
		},
		{name: "no quality block", prefix: blockAbsentPrefix},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.prefix + tt.quality + tt.suffix
			dir := writeBlockFixture(t, original)
			cfg, err := config.LoadPreview(dir)
			require.NoError(t, err)

			require.NoError(t, saveQualityBlock(dir, cfg, compositionCustomEdit))

			written := readBlockFixture(t, dir)
			require.True(t, strings.HasPrefix(written, tt.prefix), "bytes before the block changed:\n%s", written)
			require.True(t, strings.HasSuffix(written, tt.suffix), "bytes after the block changed:\n%s", written)
			block := written[len(tt.prefix) : len(written)-len(tt.suffix)]
			assert.True(t, strings.HasPrefix(block, "quality:"), "the new block replaces the old one in place:\n%s", block)
			assert.Equal(t, 1, strings.Count(block, "presets:"), "the block holds one presets key:\n%s", block)
			newline := "\n"
			if strings.Contains(original, "\r\n") {
				newline = "\r\n"
				assert.Equal(t, strings.Count(block, "\n"), strings.Count(block, "\r\n"), "a CRLF file gets a CRLF block")
			}
			assert.True(t, strings.HasSuffix(block, newline) && !strings.HasSuffix(block, newline+newline),
				"the block ends on its last child; the blank and comment lines after it are the original's:\n%q", block)

			loaded, err := config.Load(dir)
			require.NoError(t, err)
			assert.Equal(t, "custom", loaded.Quality.Default)
			assert.Equal(t, s6CustomAgents(), loaded.Quality.Presets["custom"].Agents)
			assert.Equal(t, "역할 그룹 구성", loaded.Quality.Presets["custom"].Description)
			assert.Equal(t, loaded.Quality, cfg.Quality, "the caller's config follows the written quality")

			before, after := rawQualityOf(t, original), rawQualityOf(t, written)
			for name, preset := range before.Presets {
				assert.Equal(t, preset, after.Presets[name], "untouched preset %s keeps its raw value", name)
			}
			assert.Nil(t, after.Providers, "no quality.providers key is added")
			assert.Equal(t, before.SupervisorModelPolicy, after.SupervisorModelPolicy)

			var legacy struct {
				Quality legacyQualitySchema `yaml:"quality"`
			}
			decoder := yaml.NewDecoder(strings.NewReader(block))
			decoder.KnownFields(true)
			require.NoError(t, decoder.Decode(&legacy), "an older binary must still decode the block")
			assert.Len(t, legacy.Quality.Presets["custom"].Agents, len(config.CanonicalAgentNames()))
		})
	}
}

func TestSaveQualityBlock_FailedRenameKeepsTheOriginalAndNoTempFile(t *testing.T) {
	original := blockHandPrefix + blockHandQuality + blockHandSuffix
	dir := writeBlockFixture(t, original)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)
	previous := renameQualityConfig
	renameQualityConfig = func(string, string) error { return errors.New("rename seam failed") }
	t.Cleanup(func() { renameQualityConfig = previous })

	err = saveQualityBlock(dir, cfg, compositionCustomEdit)

	require.ErrorContains(t, err, "rename seam failed")
	assert.Equal(t, original, readBlockFixture(t, dir))
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "only autopus.yaml remains")
	assert.Equal(t, "balanced", cfg.Quality.Default, "a failed write leaves the caller's config alone")
}

func TestSaveQualityBlock_AnEditThatChangesNothingWritesNothing(t *testing.T) {
	original := blockHandPrefix + blockHandQuality + blockHandSuffix
	dir := writeBlockFixture(t, original)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)

	require.NoError(t, saveQualityBlock(dir, cfg, func(quality *config.QualityConf) error {
		quality.Default = "balanced"
		return nil
	}))

	assert.Equal(t, original, readBlockFixture(t, dir), "the block keeps its comment and quoting")
}

func TestSaveQualityBlock_AnEditErrorWritesNothing(t *testing.T) {
	original := blockHandPrefix + blockHandQuality + blockHandSuffix
	dir := writeBlockFixture(t, original)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)

	err = saveQualityBlock(dir, cfg, func(*config.QualityConf) error { return errors.New("edit failed") })

	require.ErrorContains(t, err, "edit failed")
	assert.Equal(t, original, readBlockFixture(t, dir))
}

func TestSaveQualityBlock_RefusesAnEditThatBreaksAnAliasOutsideQuality(t *testing.T) {
	// The block is re-encoded, so an anchor inside it disappears; an alias
	// elsewhere in the file would dangle or rebind.
	original := "project_name: demo\nmode: full\nplatforms: [claude-code]\n" +
		"quality:\n  default: &d balanced\n  presets:\n    balanced:\n      description: b\n" +
		"future_extension:\n  copy: *d\n"
	dir := writeBlockFixture(t, original)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)

	require.Error(t, saveQualityBlock(dir, cfg, compositionCustomEdit))

	assert.Equal(t, original, readBlockFixture(t, dir))
}

func TestSaveQualityBlock_RefusesAFlowRootMapping(t *testing.T) {
	original := "{project_name: demo, mode: full, platforms: [claude-code], quality: {default: balanced, " +
		"presets: {balanced: {description: b}}}}\n"
	dir := writeBlockFixture(t, original)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)

	require.Error(t, saveQualityBlock(dir, cfg, compositionCustomEdit))

	assert.Equal(t, original, readBlockFixture(t, dir))
}

func TestSaveQualityBlock_WithoutAConfigFileWritesTheDefaultsWithTheEdit(t *testing.T) {
	dir := t.TempDir()
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)

	require.NoError(t, saveQualityBlock(dir, cfg, compositionCustomEdit))

	loaded, err := config.Load(dir)
	require.NoError(t, err)
	assert.Equal(t, "custom", loaded.Quality.Default)
	assert.Equal(t, s6CustomAgents(), loaded.Quality.Presets["custom"].Agents)
	assert.Equal(t, config.DefaultFullConfig(filepath.Base(dir)).Quality.Presets["balanced"],
		loaded.Quality.Presets["balanced"], "the shipped presets ride along")
}

func TestSaveQualityBlock_ResultLoadsLikeTheOriginalOutsideQuality(t *testing.T) {
	// The W fixture's codex.binary alias must still resolve through the
	// decoy anchor after the block write.
	original := blockHandPrefix + blockHandQuality + blockHandSuffix
	dir := writeBlockFixture(t, original)
	cfg, err := config.LoadPreview(dir)
	require.NoError(t, err)

	require.NoError(t, saveQualityBlock(dir, cfg, compositionCustomEdit))

	loaded, err := config.LoadPreview(dir)
	require.NoError(t, err)
	assert.Equal(t, "codex", loaded.Orchestra.Providers["codex"].Binary)
	assert.True(t, bytes.Contains([]byte(readBlockFixture(t, dir)), []byte("  decoy: &bin codex\n")))
}
