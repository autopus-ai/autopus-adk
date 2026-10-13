package cli

// SPEC-MODELUX-003 T12 (REQ-MUX3-09, REQ-MUX3-14, REQ-MUX3-15; acceptance
// S8, S12, S13): auto update shows the Migration Contract lines before the
// OMP write, the OMP adapter's transaction does the migration and its way
// back, and autopus.yaml is never written for it.

import (
	"crypto/sha256"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

// ompMigrationFileSums is every OMP file of dir by SHA-256: the .omp tree
// and the OMP manifest and ledger under .autopus. Backups and transaction
// journals are the history of earlier runs, not files a run generates, and
// the manifest's generated_at stamps every write, so it is left out.
func ompMigrationFileSums(t *testing.T, dir string) map[string][32]byte {
	t.Helper()
	sums := map[string][32]byte{}
	for _, root := range []string{".omp", ".autopus"} {
		err := filepath.WalkDir(filepath.Join(dir, root), func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(dir, path)
			rel = filepath.ToSlash(rel)
			if entry.IsDir() {
				if rel == ".autopus/backup" || rel == ".autopus/txns" {
					return filepath.SkipDir
				}
				return nil
			}
			if root == ".autopus" && !strings.Contains(rel, "omp") {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if rel == ".autopus/omp-manifest.json" {
				body = ompMigrationUnstamped(t, body)
			}
			sums[rel] = sha256.Sum256(body)
			return nil
		})
		if !os.IsNotExist(err) {
			require.NoError(t, err)
		}
	}
	return sums
}

func ompMigrationUnstamped(t *testing.T, manifest []byte) []byte {
	t.Helper()
	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(manifest, &fields))
	delete(fields, "generated_at")
	body, err := json.Marshal(fields)
	require.NoError(t, err)
	return body
}

func ompMigrationUpdate(t *testing.T, dir string) string {
	t.Helper()
	out, err := runComposeUpdate(t, strings.NewReader(""), "--dir", dir, "--yes")
	require.NoError(t, err, out)
	return out
}

func ompMigrationAgentFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(dir, ".omp", "agents"))
	if os.IsNotExist(err) {
		return nil
	}
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

// S12 and S13: NM migrates on the first update and stays on the second; a
// hand-written agent_source: native returns to NM's OMP files, and dropping
// it again returns to the migrated files. autopus.yaml is never written.
func TestOMPAgentMigration_S12_S13_UpdateMigratesAndGoesBack(t *testing.T) {
	newComposeUpdateRun(t, false, "")
	usePlatformOMPModelRunner(t, &ompCLIFakeRunner{fail: true})
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("nm")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(dir, nativeOMPConfig(cfg)))
	ompMigrationUpdate(t, dir)
	settled, err := config.LoadPreview(dir)
	require.NoError(t, err)
	settled.RoleModelPolicy.AgentSource = ""
	require.NoError(t, config.Save(dir, settled))
	yamlPath := filepath.Join(dir, "autopus.yaml")
	nmYAML, err := os.ReadFile(yamlPath)
	require.NoError(t, err)
	require.NotContains(t, string(nmYAML), "role_model_policy")
	nm := ompMigrationFileSums(t, dir)
	require.Empty(t, ompMigrationAgentFiles(t, dir), "NM is the v0.50.125 OMP layout")

	first := ompMigrationUpdate(t, dir)

	for _, agent := range config.CanonicalAgentNames() {
		assert.Contains(t, first, "    + .omp/agents/"+agent+".md\n")
	}
	assert.Contains(t, first, "    task.disabledAgents: scout, security-reviewer, sonic\n")
	assert.Contains(t, first, "\n    OMP는 이제 Autopus 에이전트 16개를 씁니다.")
	assert.Less(t, strings.Index(first, "+ .omp/agents/planner.md"), strings.Index(first, "✓ omp updated"),
		"the lines preview the write")
	assert.Len(t, ompMigrationAgentFiles(t, dir), 16)
	ompConfig, err := os.ReadFile(filepath.Join(dir, ".omp", "config.yml"))
	require.NoError(t, err)
	assert.Contains(t, string(ompConfig), "disabledAgents")
	assert.FileExists(t, filepath.Join(dir, filepath.FromSlash(omp.OMPModelProjectOwnershipRelativePath)))
	afterYAML, err := os.ReadFile(yamlPath)
	require.NoError(t, err)
	assert.Equal(t, sha256.Sum256(nmYAML), sha256.Sum256(afterYAML), "the migration does not write autopus.yaml")
	migrated := ompMigrationFileSums(t, dir)

	second := ompMigrationUpdate(t, dir)

	assert.NotContains(t, second, "+ .omp/agents/")
	assert.NotContains(t, second, "OMP는 이제")
	assert.Equal(t, migrated, ompMigrationFileSums(t, dir), "a second update changes no OMP file")

	require.NoError(t, os.WriteFile(yamlPath, append(append([]byte(nil), nmYAML...),
		"role_model_policy:\n  agent_source: native\n"...), 0o644))
	native := ompMigrationUpdate(t, dir)

	assert.Contains(t, native, "    - .omp/agents/planner.md\n")
	assert.Equal(t, 16, strings.Count(native, "    - .omp/agents/"))
	assert.Contains(t, native, "    task.disabledAgents 복원\n")
	assert.Empty(t, ompMigrationAgentFiles(t, dir))
	assert.NoFileExists(t, filepath.Join(dir, ".omp", "config.yml"))
	assert.NoFileExists(t, filepath.Join(dir, filepath.FromSlash(omp.OMPModelProjectOwnershipRelativePath)))
	assert.Equal(t, nm, ompMigrationFileSums(t, dir), "native returns to the v0.50.125 OMP files")

	require.NoError(t, os.WriteFile(yamlPath, nmYAML, 0o644))
	ompMigrationUpdate(t, dir)

	assert.Equal(t, migrated, ompMigrationFileSums(t, dir), "dropping the line returns to the migrated files")
}

// S8: init (Generate) and update both name a user file and leave it alone.
func TestOMPAgentMigration_S8_UserFileLineInInitAndUpdate(t *testing.T) {
	newComposeUpdateRun(t, false, "")
	usePlatformOMPModelRunner(t, &ompCLIFakeRunner{fail: true})
	userFile := []byte("---\nname: reviewer\ndescription: mine\n---\nmine\n")
	line := "    .omp/agents/reviewer.md는 사용자 파일이라 그대로 둡니다\n"
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".omp", "agents"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".omp", "agents", "reviewer.md"), userFile, 0o644))

	root := NewRootCmd()
	var out strings.Builder
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	root.SetArgs([]string{"init", "--dir", dir, "--project", "uf", "--platforms", "omp", "--yes"})
	require.NoError(t, root.Execute(), out.String())
	update := ompMigrationUpdate(t, dir)

	assert.Contains(t, out.String(), line, "Generate names the user file")
	assert.NotContains(t, out.String(), "OMP는 이제", "init is no migration")
	assert.Contains(t, update, line)
	assert.NotContains(t, update, "+ .omp/agents/reviewer.md")
	body, err := os.ReadFile(filepath.Join(dir, ".omp", "agents", "reviewer.md"))
	require.NoError(t, err)
	assert.Equal(t, userFile, body)
	assert.Len(t, ompMigrationAgentFiles(t, dir), 16, "15 generated files and the user's")
}

// The lines for every report shape, in both languages.
func TestOMPAgentSurfaceLines(t *testing.T) {
	cfg := config.DefaultFullConfig("lines")
	sonic := config.DefaultFullConfig("sonic")
	sonic.RoleModelPolicy.Agents = map[string]config.RoleAgentOverrideConf{"sonic": {Candidates: []config.RoleModelCandidateConf{
		{Selector: "openai-codex/gpt-6-luna", Thinking: "high", Family: "openai"},
	}}}
	recorded := config.CanonicalAgentNames()
	ko, en := ompAgentSurfaceTextKO, ompAgentSurfaceTextEN
	autopus := omp.OMPAgentSurfaceReport{Source: config.OMPAgentSourceAutopus}

	owned := autopus
	owned.UserAgentFiles, owned.DisabledAgentsUserOwned = []string{"reviewer"}, true
	lines := ompAgentSurfaceLines(owned, cfg, true, ko)
	assert.Len(t, lines, 18)
	assert.Contains(t, lines, ".omp/agents/reviewer.md는 사용자 파일이라 그대로 둡니다")
	assert.NotContains(t, lines, "+ .omp/agents/reviewer.md")
	assert.Contains(t, lines, ko.userOwned)
	assert.NotContains(t, lines, "task.disabledAgents: scout, security-reviewer, sonic")

	migrated := autopus
	migrated.RecordedAgents, migrated.DisabledAgentsClaimed = recorded, true
	assert.Empty(t, ompAgentSurfaceLines(migrated, cfg, true, ko))
	assert.Equal(t, []string{en.sonic}, ompAgentSurfaceLines(migrated, sonic, true, en))
	assert.Equal(t, "warning: role_model_policy.agents.sonic pins a model, but autopus mode disables sonic, so no agent receives it", en.sonic)

	native := omp.OMPAgentSurfaceReport{Source: config.OMPAgentSourceNative, RecordedAgents: []string{"planner"}, DisabledAgentsClaimed: true}
	assert.Equal(t, []string{"- .omp/agents/planner.md", "task.disabledAgents restored"}, ompAgentSurfaceLines(native, cfg, true, en))
	assert.Empty(t, ompAgentSurfaceLines(native, cfg, false, en))
	assert.Equal(t, en, ompAgentSurfaceTextFor("en"))
	assert.Equal(t, ko, ompAgentSurfaceTextFor("ko"))
}

// S8, UC: a task.disabledAgents the user set is named on every update and
// left byte-identical.
func TestOMPAgentMigration_S8_UserOwnedDisabledAgentsLine(t *testing.T) {
	newComposeUpdateRun(t, false, "")
	usePlatformOMPModelRunner(t, &ompCLIFakeRunner{fail: true})
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("uc")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(dir, cfg))
	userConfig := []byte("task:\n  disabledAgents:\n    - browser\n")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, ".omp"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".omp", "config.yml"), userConfig, 0o600))
	line := "    " + ompAgentSurfaceTextKO.userOwned + "\n"

	first := ompMigrationUpdate(t, dir)
	second := ompMigrationUpdate(t, dir)

	assert.Contains(t, first, line)
	assert.NotContains(t, first, "task.disabledAgents: scout, security-reviewer, sonic")
	assert.Contains(t, second, line)
	body, err := os.ReadFile(filepath.Join(dir, ".omp", "config.yml"))
	require.NoError(t, err)
	assert.Equal(t, userConfig, body)
}

// REQ-MUX3-14: auto update --plan shows the migration lines and writes no
// OMP file.
func TestOMPAgentMigration_PlanPreviewShowsTheMigration(t *testing.T) {
	newComposeUpdateRun(t, false, "")
	dir := t.TempDir()
	cfg := config.DefaultFullConfig("plan")
	cfg.Platforms = []string{"omp"}
	require.NoError(t, config.Save(dir, cfg))

	out, err := runComposeUpdate(t, strings.NewReader(""), "--dir", dir, "--plan")

	require.NoError(t, err, out)
	assert.Contains(t, out, "    + .omp/agents/planner.md\n")
	assert.Contains(t, out, "    OMP는 이제 Autopus 에이전트 16개를 씁니다.")
	assert.Empty(t, ompMigrationFileSums(t, dir))
}
