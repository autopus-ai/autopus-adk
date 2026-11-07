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

// S7 (O3): the default source writes the 4 canonical agents, records each in
// the manifest, and regenerates them byte for byte.
func TestOMPGenerate_WritesTheFourAutopusAgents(t *testing.T) {
	root := t.TempDir()
	cfg := configForOMP()
	require.NoError(t, config.Save(root, cfg))
	_, err := NewWithRoot(root).Generate(context.Background(), cfg)
	require.NoError(t, err)

	assert.Equal(t, config.CanonicalAgentNames(), ompAgentDirNames(t, root))
	assert.Equal(t, 4, countPathsWithPrefix(manifestPaths(t, root), ".omp/agents/"))

	explorer := readOMPAgentFrontmatter(t, root, "qa-explorer")
	assert.Equal(t, "qa-explorer", explorer["name"])
	assert.Equal(t, "만든 것을 실제로 실행·조작하며 버그를 찾는 탐색형 QA 에이전트. 재현 스크립트로 확인된 버그만 보고하고 제품 코드는 고치지 않는다.",
		explorer["description"])
	assert.Equal(t, "high", explorer["thinkingLevel"])
	assert.Equal(t, []any{
		"bash", "browser", "eval", "glob", "grep", "inspect_image", "read", "write",
	}, explorer["tools"])
	assert.NotContains(t, explorer, "model")

	frontend := readOMPAgentFrontmatter(t, root, "frontend-specialist")
	assert.Equal(t, "medium", frontend["thinkingLevel"])
	assert.Contains(t, frontend["tools"], "lsp")

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
	mappings, err := prepareOMPAgentMappings(map[string]bool{"qa-explorer": true, "reviewer": true})
	require.NoError(t, err)
	require.Len(t, mappings, 4)
	byAgent := map[string]string{}
	for _, mapping := range mappings {
		byAgent[strings.TrimSuffix(filepath.Base(mapping.TargetPath), ".md")] = string(mapping.Content)
	}
	assert.NotContains(t, byAgent["qa-explorer"], "thinkingLevel")
	assert.NotContains(t, byAgent["reviewer"], "thinkingLevel")
	assert.Contains(t, byAgent["frontend-specialist"], "\nthinkingLevel: medium\n")
	assert.Contains(t, byAgent["ux-validator"], "\nthinkingLevel: medium\n")
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
		assert.Equal(t, 3, countPathsWithPrefix(paths, ".omp/agents/"), operation)
		assert.NotContains(t, paths, ".omp/agents/reviewer.md", operation)
		assert.Len(t, ompAgentDirNames(t, root), 4, operation)
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
	path := filepath.Join(root, ".omp", "agents", "ux-validator.md")
	generated, err := os.ReadFile(path)
	require.NoError(t, err)
	edited := append(append([]byte(nil), generated...), []byte("USER LINE\n")...)
	require.NoError(t, os.WriteFile(path, edited, 0o644))

	_, err = NewWithRoot(root).Update(context.Background(), cfg)
	require.NoError(t, err)

	current, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, generated, current)
	backups, err := filepath.Glob(filepath.Join(root, ".autopus", "backup", "*", "transaction", "omp", ".omp", "agents", "ux-validator.md"))
	require.NoError(t, err)
	require.Len(t, backups, 1)
	backup, err := os.ReadFile(backups[0])
	require.NoError(t, err)
	assert.Equal(t, edited, backup)
}
