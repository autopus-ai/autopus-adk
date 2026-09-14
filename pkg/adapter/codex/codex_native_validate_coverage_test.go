package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// Guards against the routing rule that separates a skill entrypoint from a
// reference body: a reference must never be judged by the entrypoint naming
// contract, and an orphaned reference must be reported.
func TestValidateNativeCodexSkillPath_RoutesReferencesAwayFromEntrypointRule(t *testing.T) {
	root := t.TempDir()
	reference := ".codex/skills/codex-auto/references/detail.md"
	writeCodexDoc(t, root, reference, "detail")

	var orphaned []adapter.ValidationError
	validateNativeCodexSkillPath(root, filepath.FromSlash(reference), &orphaned)
	require.Len(t, orphaned, 1)
	assert.Contains(t, orphaned[0].Message, "entrypoint")

	writeCodexDoc(t, root, ".codex/skills/codex-auto/SKILL.md", "---\nname: codex-auto\n---\n")
	var healthy []adapter.ValidationError
	validateNativeCodexSkillPath(root, filepath.FromSlash(reference), &healthy)
	assert.Empty(t, healthy, "a reference beside an installed entrypoint is legitimate")
}

// Guards against accepting a reference path that is recorded in the manifest
// but is not a regular file on disk.
func TestValidateNativeCodexSkillResource_RejectsNonRegularReference(t *testing.T) {
	root := t.TempDir()
	writeCodexDoc(t, root, ".codex/skills/codex-auto/SKILL.md", "name: codex-auto")
	reference := filepath.Join(".codex", "skills", "codex-auto", "references", "dir.md")
	require.NoError(t, os.MkdirAll(filepath.Join(root, reference), 0o755))

	var errs []adapter.ValidationError
	validateNativeCodexSkillResource(root, "codex-auto", reference, &errs)

	require.Len(t, errs, 1)
	assert.Contains(t, errs[0].Message, "읽을 수 없음")
	assert.Equal(t, "error", errs[0].Level)
}

// Guards against a skill surface drifting from the native layout contract:
// wrong depth, wrong prefix, wrong filename, missing body, and a front-matter
// name that disagrees with its directory must each be reported distinctly.
func TestValidateNativeCodexSkill_EnforcesLayoutAndNameAgreement(t *testing.T) {
	root := t.TempDir()
	writeCodexDoc(t, root, ".codex/skills/codex-auto/SKILL.md", "---\nname: codex-auto\n---\nbody")
	writeCodexDoc(t, root, ".codex/skills/codex-drift/SKILL.md", "---\nname: codex-other\n---\nbody")
	writeCodexDoc(t, root, ".codex/skills/plain/SKILL.md", "---\nname: plain\n---\n")

	for _, test := range []struct {
		name, path, want string
	}{
		{name: "valid", path: ".codex/skills/codex-auto/SKILL.md"},
		{name: "name disagrees with directory", path: ".codex/skills/codex-drift/SKILL.md", want: "일치하지 않음"},
		{name: "missing codex prefix", path: ".codex/skills/plain/SKILL.md", want: "layout"},
		{name: "wrong depth", path: ".codex/skills/codex-auto/nested/SKILL.md", want: "layout"},
		{name: "wrong filename", path: ".codex/skills/codex-auto/README.md", want: "layout"},
		{name: "outside the skills root", path: ".claude/skills/codex-auto/SKILL.md", want: "layout"},
		{name: "declared but absent", path: ".codex/skills/codex-gone/SKILL.md", want: "읽을 수 없음"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var errs []adapter.ValidationError
			validateNativeCodexSkill(root, filepath.FromSlash(test.path), &errs)
			if test.want == "" {
				assert.Empty(t, errs)
				return
			}
			require.Len(t, errs, 1)
			assert.Contains(t, errs[0].Message, test.want)
			assert.Equal(t, filepath.FromSlash(test.path), errs[0].File)
		})
	}
}

// Guards the ownership boundary of user model settings: an unmanaged file is
// preserved wholesale and marked, a marker narrows preservation to the named
// keys, and a managed generated file only keeps values that differ from the
// managed supervisor tuple.
func TestCodexModelSettingsToPreserve_ResolvesOwnershipPerSource(t *testing.T) {
	managedManifest := &adapter.Manifest{Files: map[string]adapter.ManifestFile{
		codexConfigRelPath: {Checksum: "x"},
	}}

	t.Run("no user settings", func(t *testing.T) {
		got := codexModelSettingsToPreserve([]byte("approval_policy = \"never\"\n"), nil, false)
		assert.Empty(t, got.overrides)
		assert.False(t, got.mark)
	})

	t.Run("unmanaged file is preserved and marked", func(t *testing.T) {
		got := codexModelSettingsToPreserve(
			[]byte("model = \"user-model\"\nmodel_verbosity = \"high\"\n"), nil, false)
		assert.Equal(t, map[string]string{
			".model": `"user-model"`, ".model_verbosity": `"high"`,
		}, got.overrides)
		assert.True(t, got.mark)
	})

	t.Run("marker narrows preservation", func(t *testing.T) {
		content := codexUserModelMarker + ": model\nmodel = \"user-model\"\nmodel_verbosity = \"high\"\n"
		got := codexModelSettingsToPreserve([]byte(content), managedManifest, false)
		assert.Equal(t, map[string]string{".model": `"user-model"`}, got.overrides,
			"a marker naming only model must not preserve verbosity")
		assert.True(t, got.mark)
	})

	t.Run("marker naming nothing preserved drops the mark", func(t *testing.T) {
		content := codexUserModelMarker + ": sandbox_mode\nmodel = \"user-model\"\n"
		got := codexModelSettingsToPreserve([]byte(content), managedManifest, false)
		assert.Empty(t, got.overrides)
		assert.False(t, got.mark)
	})

	t.Run("managed generated file reclaims the managed tuple", func(t *testing.T) {
		content := codexGeneratedConfigHeader + "\nmodel = \"" + config.CodexSolModel +
			"\"\nmodel_reasoning_effort = \"xhigh\"\nmodel_reasoning_summary = \"auto\"\n" +
			"model_verbosity = \"medium\"\n"
		got := codexModelSettingsToPreserve([]byte(content), managedManifest, false)
		assert.Empty(t, got.overrides, "values equal to the managed defaults are not user settings")
		assert.False(t, got.mark)
	})

	t.Run("managed generated file keeps a custom tuple", func(t *testing.T) {
		content := codexGeneratedConfigHeader +
			"\nmodel = \"user-model\"\nmodel_reasoning_effort = \"low\"\nmodel_verbosity = \"high\"\n"
		got := codexModelSettingsToPreserve([]byte(content), managedManifest, false)
		assert.Equal(t, map[string]string{
			".model": `"user-model"`, ".model_reasoning_effort": `"low"`, ".model_verbosity": `"high"`,
		}, got.overrides)
		assert.True(t, got.mark)
	})

	t.Run("explicit supervisor policy reclaims without marking", func(t *testing.T) {
		content := codexGeneratedConfigHeader + "\nmodel = \"user-model\"\n"
		got := codexModelSettingsToPreserve([]byte(content), managedManifest, true)
		assert.Equal(t, map[string]string{".model": `"user-model"`}, got.overrides)
		assert.False(t, got.mark, "an explicit policy is the reclaim boundary, so no marker is written")
	})
}

// Guards against the merge writing more than one ownership marker, and against
// placing it anywhere other than immediately before the first user-owned key.
func TestPreserveUserCodexModelSettings_WritesExactlyOneMarker(t *testing.T) {
	rendered := codexGeneratedConfigHeader + "\napproval_policy = \"never\"\n" +
		codexUserModelMarker + ": model\nmodel = \"managed\"\n[profiles.alpha]\nmodel = \"scoped\"\n"

	merged := preserveUserCodexModelSettings(rendered, codexModelPreservation{
		overrides: map[string]string{".model": `"user-model"`, ".model_verbosity": `"high"`},
		mark:      true,
	})

	lines := strings.Split(merged, "\n")
	markers, modelIndex, markerIndex := 0, -1, -1
	for index, line := range lines {
		switch {
		case line == codexUserModelMarker+": model, model_verbosity":
			markers++
			markerIndex = index
		case line == `model = "user-model"` && modelIndex < 0:
			modelIndex = index
		}
	}
	assert.Equal(t, 1, markers, "the stale marker must be replaced, not duplicated")
	require.Positive(t, modelIndex)
	assert.Equal(t, modelIndex-1, markerIndex, "the marker sits directly above the first user key")
	assert.Contains(t, merged, `model_verbosity = "high"`,
		"a preserved key absent from the rendered output must be reinserted")
	assert.Contains(t, merged, "[profiles.alpha]\nmodel = \"scoped\"",
		"scoped sections keep their generated values")
}
