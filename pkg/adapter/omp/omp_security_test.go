package omp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

// TestOMPClean_PreservesUserConfigKeys is the H-1 regression. Generate records
// the checksum of the MERGED .omp/config.yml, so an untouched user file matches
// that checksum and the plain manifest path would delete it with no backup,
// destroying keys omp never owned. REQ-007 and REQ-017 require the managed
// marker section to be stripped and the remainder rewritten instead.
func TestOMPClean_PreservesUserConfigKeys(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, configFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgPath), 0o755))
	original := []byte("disabledProviders:\n  - anthropic\nmodel: my-local-model\n")
	require.NoError(t, os.WriteFile(cfgPath, original, 0o644))

	harness := config.DefaultFullConfig("omp-sec")
	harness.Platforms = []string{"omp"}
	harness.RoleModelPolicy.AgentSource = config.OMPAgentSourceNative
	require.NoError(t, config.Save(dir, harness))
	_, err := NewWithRoot(dir).Generate(context.Background(), harness)
	require.NoError(t, err)

	generated, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.Equal(t, original, generated, "plain OMP generation must not own user config")
	require.NotContains(t, string(generated), markerBeginYml)

	// `auto platform remove omp` drops omp before Clean runs.
	remaining := config.DefaultFullConfig("omp-sec")
	remaining.Platforms = []string{"claude-code"}
	require.NoError(t, config.Save(dir, remaining))
	require.NoError(t, NewWithRoot(dir).Clean(context.Background()))

	after, err := os.ReadFile(cfgPath)
	require.NoError(t, err, ".omp/config.yml must survive Clean because it holds user keys")

	var parsed struct {
		DisabledProviders []string `yaml:"disabledProviders"`
		Model             string   `yaml:"model"`
	}
	require.NoError(t, yaml.Unmarshal(after, &parsed), "the rewritten file must still parse")
	assert.Equal(t, []string{"anthropic"}, parsed.DisabledProviders)
	assert.Equal(t, "my-local-model", parsed.Model)
	assert.NotContains(t, string(after), markerBeginYml, "the managed section is stripped")
	assert.NotContains(t, string(after), markerEndYml)
	assert.NotContains(t, string(after), "customDirectories", "managed content is gone")
}

// TestOMPClean_FailsClosedWithoutHarnessConfig verifies that OMP never claims
// the lower-priority shared Agent Skills surface, even without harness config.
func TestOMPClean_FailsClosedWithoutHarnessConfig(t *testing.T) {
	t.Parallel()

	dir := generateOMPOnly(t)
	skillPath := filepath.Join(dir, ".agents", "skills", "auto", "SKILL.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(skillPath), 0o755))
	require.NoError(t, os.WriteFile(skillPath, []byte("opencode owned skill\n"), 0o644))
	require.NoError(t, os.Remove(filepath.Join(dir, "autopus.yaml")))

	require.NoError(t, NewWithRoot(dir).Clean(context.Background()))

	after, err := os.ReadFile(skillPath)
	require.NoError(t, err, "a surface omp cannot prove it owns must survive")
	assert.Equal(t, "opencode owned skill\n", string(after))
	assert.NoDirExists(t, filepath.Join(dir, ".autopus", "backup"),
		"a skipped surface must not be backed up either")

	// Surfaces omp owns unconditionally are still cleaned.
	assert.NoFileExists(t, filepath.Join(dir, ompRuleDir, ompRuleFilePrefix+"branding.md"))
	assert.NoFileExists(t, filepath.Join(dir, ".omp", "agents", "executor.md"))
}

// TestOMPDetect_HonorsContext is the M-3 regression. Detect used to ignore its
// context entirely and exec with no timeout, so a binary named omp that never
// answers --version froze `auto doctor`. An already-cancelled context must abort
// the probe instead of blocking.
func TestOMPDetect_HonorsContext(t *testing.T) {
	t.Parallel()

	if _, err := exec.LookPath(cliBinary); err != nil {
		t.Skip("no omp binary on PATH to probe")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	ok, err := NewWithRoot(t.TempDir()).Detect(ctx)
	require.NoError(t, err, "a cancelled probe reports absence, not an error")
	assert.False(t, ok, "a probe that cannot run must not claim the platform is present")
}

// TestOMPDetect_NilContextIsSafe pins the defensive branch: callers in this
// repo always pass a context, but a nil one must not panic.
func TestOMPDetect_NilContextIsSafe(t *testing.T) {
	t.Parallel()

	//nolint:staticcheck // deliberately passing a nil context to pin the guard
	_, err := NewWithRoot(t.TempDir()).Detect(nil)
	require.NoError(t, err)
}

// TestOMPGenerate_RefusesUnbalancedMarkerConfig is the F-2 regression. The
// managed-section regex is non-greedy and the rewrite branch only checked that
// both markers appear somewhere. A user file carrying an orphaned BEGIN gained a
// second BEGIN on the first Generate, and the next Generate then matched from
// the first BEGIN to the first END and silently ate every user key between them.
func TestOMPGenerate_RefusesUnbalancedMarkerConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, configFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgPath), 0o755))
	original := markerBeginYml + "\ndisabledProviders:\n  - anthropic\nmodel: keep-me\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(original), 0o644))

	harness := config.DefaultFullConfig("omp-sec")
	harness.Platforms = []string{"omp"}
	require.NoError(t, config.Save(dir, harness))

	adapterUnderTest := NewWithRoot(dir)
	_, firstErr := adapterUnderTest.Generate(context.Background(), harness)
	_, secondErr := adapterUnderTest.Generate(context.Background(), harness)

	after, readErr := os.ReadFile(cfgPath)
	require.NoError(t, readErr)
	if firstErr == nil && secondErr == nil {
		assert.Contains(t, string(after), "disabledProviders:",
			"user keys must never be lost silently")
		assert.Contains(t, string(after), "model: keep-me")
		return
	}
	require.Error(t, firstErr, "an unbalanced managed section must fail closed, not mangle the file")
	assert.Contains(t, string(after), "model: keep-me", "the file is left as the user wrote it")
}

// TestOMPGenerate_EscapesAdversarialSkillName is the F-3 regression: emitted
// scalars in the skill and command surfaces went through raw %s while
// ompYAMLScalar existed precisely to close that class.
func TestOMPGenerate_EscapesAdversarialSkillName(t *testing.T) {
	t.Parallel()

	hostile := "evil\ncompatibility: claude"
	rendered := buildMarkdown(ompSkillFrontmatter(hostile, "desc: with colon"), "Body.")

	fmText, _, found := strings.Cut(strings.TrimPrefix(rendered, "---\n"), "\n---\n")
	require.True(t, found)

	var parsed map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(fmText), &parsed),
		"an adversarial name must still produce parseable YAML")
	assert.Equal(t, hostile, parsed["name"], "the name round-trips as one scalar")
	assert.Equal(t, "omp", parsed["compatibility"],
		"an injected compatibility line must not override the emitted one")
}

// TestOMPGenerate_RefusesMarkerInsideBlockScalar is the O-4 regression. The
// line-anchored marker regex is YAML-structure-blind: a marker line inside a
// literal block scalar is ordinary text to YAML but a section opener to the
// regex, so the rewrite swallowed the user's value.
func TestOMPGenerate_RefusesMarkerInsideBlockScalar(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	cfgPath := filepath.Join(dir, configFile)
	require.NoError(t, os.MkdirAll(filepath.Dir(cfgPath), 0o755))
	original := "notes: |\n  " + markerBeginYml + "\n  keep this note\n  " + markerEndYml +
		"\nmodel: keep-me\n"
	require.NoError(t, os.WriteFile(cfgPath, []byte(original), 0o644))

	harness := config.DefaultFullConfig("omp-o4")
	harness.Platforms = []string{"omp"}
	require.NoError(t, config.Save(dir, harness))

	_, genErr := NewWithRoot(dir).Generate(context.Background(), harness)

	after, readErr := os.ReadFile(cfgPath)
	require.NoError(t, readErr)
	if genErr == nil {
		assert.Contains(t, string(after), "keep this note",
			"a marker inside a scalar value must never be treated as a section")
		assert.Contains(t, string(after), "model: keep-me")
		return
	}
	assert.Equal(t, original, string(after),
		"failing closed must leave the file exactly as the user wrote it")
}

// TestOMPUpdate_PreservesConfigFileMode is the O-5 regression. The update
// transaction wrote every managed file at 0644, so a config the user had
// tightened to 0600 was silently widened — and .omp/config.yml is exactly where
// provider credentials live.
func TestOMPUpdate_PreservesConfigFileMode(t *testing.T) {
	t.Parallel()

	dir := generateOMPOnly(t)
	cfgPath := filepath.Join(dir, configFile)
	require.NoError(t, os.WriteFile(cfgPath, []byte("provider: user-owned\n"), 0o600))

	harness := config.DefaultFullConfig("omp-o5")
	harness.Platforms = []string{"omp"}
	_, err := NewWithRoot(dir).Update(context.Background(), harness)
	require.NoError(t, err)

	info, statErr := os.Stat(cfgPath)
	require.NoError(t, statErr)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"update must not widen a config the user tightened")
}
