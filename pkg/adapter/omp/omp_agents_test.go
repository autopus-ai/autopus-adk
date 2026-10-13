package omp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

func readOMPAgentFrontmatter(t *testing.T, root, agent string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".omp", "agents", agent+".md"))
	require.NoError(t, err)
	parts := strings.SplitN(string(data), "---\n", 3)
	require.Len(t, parts, 3)
	fields := map[string]any{}
	require.NoError(t, yaml.Unmarshal([]byte(parts[1]), &fields))
	return fields
}

func ompAgentDirNames(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, ".omp", "agents"))
	require.NoError(t, err)
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, strings.TrimSuffix(entry.Name(), ".md"))
	}
	sort.Strings(names)
	return names
}

func ompTreeDigests(t *testing.T, root, dir string) map[string]string {
	t.Helper()
	digests := map[string]string{}
	require.NoError(t, filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(data)
		rel, _ := filepath.Rel(root, path)
		digests[filepath.ToSlash(rel)] = hex.EncodeToString(sum[:])
		return nil
	}))
	return digests
}

func ompReviewerGolden(t *testing.T) any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "omp-18.6.1-reviewer-output.yaml"))
	require.NoError(t, err)
	var golden any
	require.NoError(t, yaml.Unmarshal(data, &golden))
	return golden
}

// S7 (O3): the default source writes the 16 canonical agents, records each in
// the manifest, and regenerates them byte for byte.
func TestOMPGenerate_WritesTheSixteenAutopusAgents(t *testing.T) {
	root := t.TempDir()
	cfg := configForOMP()
	require.NoError(t, config.Save(root, cfg))
	_, err := NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)

	assert.Equal(t, config.CanonicalAgentNames(), ompAgentDirNames(t, root))
	assert.Equal(t, 16, countPathsWithPrefix(manifestPaths(t, root), ".omp/agents/"))

	planner := readOMPAgentFrontmatter(t, root, "planner")
	assert.Equal(t, "planner", planner["name"])
	assert.Equal(t, "기능 기획 및 요구사항 분석 전문 에이전트. 사용자 요청을 명확한 요구사항과 구현 계획으로 변환한다.",
		planner["description"])
	assert.Equal(t, "max", planner["thinkingLevel"])
	assert.Equal(t, []any{
		"bash", "glob", "grep", "mcp__sequential-thinking__sequentialthinking", "read", "web_search",
	}, planner["tools"])
	assert.NotContains(t, planner, "model")

	executor := readOMPAgentFrontmatter(t, root, "executor")
	assert.Equal(t, "medium", executor["thinkingLevel"])
	assert.Contains(t, executor["tools"], "lsp")

	reviewer := readOMPAgentFrontmatter(t, root, "reviewer")
	assert.Equal(t, "high", reviewer["thinkingLevel"])
	assert.Equal(t, ompReviewerGolden(t), reviewer["output"], "reviewer must keep the bundled verdict schema")

	before := ompTreeDigests(t, root, ".omp/agents")
	_, err = NewWithRoot(root).Update(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, before, ompTreeDigests(t, root, ".omp/agents"))
	_, err = NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)
	assert.Equal(t, before, ompTreeDigests(t, root, ".omp/agents"))
}

// A routed agent takes its thinking from task.agentModelOverrides, so only
// the agents the generation does not route declare one.
func TestPrepareOMPAgentMappings_DeclareThinkingOnlyForUnroutedAgents(t *testing.T) {
	t.Parallel()
	mappings, err := prepareOMPAgentMappings(map[string]bool{"planner": true, "reviewer": true})
	require.NoError(t, err)
	require.Len(t, mappings, 16)
	byAgent := map[string]string{}
	for _, mapping := range mappings {
		byAgent[strings.TrimSuffix(filepath.Base(mapping.TargetPath), ".md")] = string(mapping.Content)
	}
	assert.NotContains(t, byAgent["planner"], "thinkingLevel")
	assert.NotContains(t, byAgent["reviewer"], "thinkingLevel")
	assert.Contains(t, byAgent["architect"], "\nthinkingLevel: max\n")
	assert.Contains(t, byAgent["executor"], "\nthinkingLevel: medium\n")
}

// S8 (UF): a same-name definition the manifest does not record belongs to the
// user. Generate and Update both leave it byte-identical and skip only it.
func TestOMPGenerateAndUpdate_LeaveAnUnrecordedAgentFileUntouched(t *testing.T) {
	root := t.TempDir()
	cfg := configForOMP()
	require.NoError(t, config.Save(root, cfg))
	userPath := filepath.Join(root, ".omp", "agents", "reviewer.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(userPath), 0o755))
	user := []byte("---\nname: reviewer\ndescription: mine\n---\nmine\n")
	require.NoError(t, os.WriteFile(userPath, user, 0o644))

	for _, operation := range []string{"generate", "update"} {
		var err error
		if operation == "generate" {
			_, err = NewWithRoot(root).Generate(context.Background(), cfg)
		} else {
			_, err = NewWithRoot(root).Update(context.Background(), cfg)
		}
		require.NoError(t, err, operation)
		data, readErr := os.ReadFile(userPath)
		require.NoError(t, readErr)
		assert.Equal(t, user, data, operation)
		paths := manifestPaths(t, root)
		assert.Equal(t, 15, countPathsWithPrefix(paths, ".omp/agents/"), operation)
		assert.NotContains(t, paths, ".omp/agents/reviewer.md", operation)
		assert.Len(t, ompAgentDirNames(t, root), 16, operation)
	}
}

// S8: a recorded definition the user edited is regenerated only through the
// OMP transaction, whose backup keeps the edit.
func TestOMPUpdate_BacksUpAnEditedGeneratedAgent(t *testing.T) {
	root := t.TempDir()
	cfg := configForOMP()
	require.NoError(t, config.Save(root, cfg))
	_, err := NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)
	path := filepath.Join(root, ".omp", "agents", "executor.md")
	generated, err := os.ReadFile(path)
	require.NoError(t, err)
	edited := append(append([]byte(nil), generated...), []byte("USER LINE\n")...)
	require.NoError(t, os.WriteFile(path, edited, 0o644))

	_, err = NewWithRoot(root).Update(context.Background(), cfg)
	require.NoError(t, err)

	current, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, generated, current)
	backups, err := filepath.Glob(filepath.Join(root, ".autopus", "backup", "*", "transaction", "omp", ".omp", "agents", "executor.md"))
	require.NoError(t, err)
	require.Len(t, backups, 1)
	backup, err := os.ReadFile(backups[0])
	require.NoError(t, err)
	assert.Equal(t, edited, backup)
}

// agent_source: native keeps the v0.50.125 surface, which has no agent files.
func TestOMPGenerate_NativeSourceWritesNoAgentFiles(t *testing.T) {
	root := t.TempDir()
	cfg := configForOMP()
	cfg.RoleModelPolicy.AgentSource = config.OMPAgentSourceNative
	require.NoError(t, config.Save(root, cfg))
	_, err := NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)
	assert.NoDirExists(t, filepath.Join(root, ".omp", "agents"))
	manifest, err := adapter.LoadManifest(root, adapterName)
	require.NoError(t, err)
	for path := range manifest.Files {
		assert.False(t, strings.HasPrefix(filepath.ToSlash(path), ".omp/agents/"), path)
	}
}
