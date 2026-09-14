package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/adapter"
)

func writeCodexDoc(t *testing.T, root, relative, body string) string {
	t.Helper()
	path := filepath.Join(root, relative)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
	return path
}

func codexDiagnosticFor(errs []adapter.ValidationError, file string) (adapter.ValidationError, bool) {
	for _, item := range errs {
		if filepath.ToSlash(item.File) == file {
			return item, true
		}
	}
	return adapter.ValidationError{}, false
}

// Guards against a doctor that stays silent when an agent document lost the
// V2 collaboration contract: the diagnostic must name the offending file.
func TestValidateCodexAgentDocuments_ReportsMissingContractPerFile(t *testing.T) {
	root := t.TempDir()
	complete := "## Codex Multi-Agent V2 Contract\nshared cwd, disjoint write ownership\n" +
		"spawn_agent send_message followup_task wait_agent interrupt_agent list_agents\n"
	writeCodexDoc(t, root, ".codex/agents/executor.toml", complete)
	writeCodexDoc(t, root, ".codex/agents/partial.toml",
		"## Codex Multi-Agent V2 Contract\nshared cwd, disjoint write ownership\nspawn_agent\n")
	writeCodexDoc(t, root, ".codex/agents/ignored.md", "not toml")
	require.NoError(t, os.MkdirAll(filepath.Join(root, ".codex", "agents", "nested.toml"), 0o755))

	var errs []adapter.ValidationError
	validateCodexAgentDocuments(root, &errs)

	require.Len(t, errs, 1, "only the incomplete TOML document is a finding")
	assert.Equal(t, ".codex/agents/partial.toml", filepath.ToSlash(errs[0].File))
	assert.Equal(t, "error", errs[0].Level)
	assert.Contains(t, errs[0].Message, "V2 collaboration")
}

// Guards against reporting findings for an absent agents directory, which
// would make the doctor fail on projects that never installed the surface.
func TestValidateCodexAgentDocuments_StaysSilentWithoutDirectory(t *testing.T) {
	var errs []adapter.ValidationError
	validateCodexAgentDocuments(t.TempDir(), &errs)
	assert.Empty(t, errs)
}

// Guards against accepting a hooks document that parses but carries no Autopus
// handler, and against conflating that with malformed JSON.
func TestValidateCodexHooksDocument_SeparatesMalformedFromUnmanaged(t *testing.T) {
	t.Run("missing file is not a finding", func(t *testing.T) {
		var errs []adapter.ValidationError
		validateCodexHooksDocument(t.TempDir(), &errs)
		assert.Empty(t, errs)
	})

	t.Run("malformed json", func(t *testing.T) {
		root := t.TempDir()
		writeCodexDoc(t, root, ".codex/hooks.json", "{not json")
		var errs []adapter.ValidationError
		validateCodexHooksDocument(root, &errs)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Message, "malformed")
	})

	t.Run("foreign handlers only", func(t *testing.T) {
		root := t.TempDir()
		writeCodexDoc(t, root, ".codex/hooks.json",
			`{"hooks":{"PreToolUse":[{"hooks":[{"command":"make lint"}]}]}}`)
		var errs []adapter.ValidationError
		validateCodexHooksDocument(root, &errs)
		require.Len(t, errs, 1)
		assert.Contains(t, errs[0].Message, "hook entry")
		assert.Equal(t, "error", errs[0].Level)
	})

	t.Run("managed handler present", func(t *testing.T) {
		root := t.TempDir()
		writeCodexDoc(t, root, ".codex/hooks.json",
			`{"hooks":{"PreToolUse":[{"hooks":[{"command":"make lint"},`+
				`{"command":"auto check --hygiene --arch --quiet --staged --warn-only"}]}]}}`)
		var errs []adapter.ValidationError
		validateCodexHooksDocument(root, &errs)
		assert.Empty(t, errs, "an Autopus handler beside a user handler is healthy")
	})
}

// Guards against a plugin surface that silently drifts: a manifest with the
// wrong identity and a marketplace without the Autopus entry must both be
// reported against their own paths.
func TestValidateCodexPluginDocuments_ReportsDriftedIdentityAndMarketplace(t *testing.T) {
	manifestPath := ".autopus/plugins/auto/.codex-plugin/plugin.json"
	marketPath := ".agents/plugins/marketplace.json"

	t.Run("no documents at all", func(t *testing.T) {
		var errs []adapter.ValidationError
		validateCodexPluginDocuments(t.TempDir(), &errs)
		assert.Empty(t, errs)
	})

	t.Run("wrong manifest identity", func(t *testing.T) {
		root := t.TempDir()
		writeCodexDoc(t, root, manifestPath, `{"name":"other","skills":"./skills"}`)
		var errs []adapter.ValidationError
		validateCodexPluginDocuments(root, &errs)
		require.Len(t, errs, 1)
		assert.Equal(t, manifestPath, filepath.ToSlash(errs[0].File))
	})

	t.Run("wrong skills root", func(t *testing.T) {
		root := t.TempDir()
		writeCodexDoc(t, root, manifestPath, `{"name":"auto","skills":"skills"}`)
		var errs []adapter.ValidationError
		validateCodexPluginDocuments(root, &errs)
		require.Len(t, errs, 1)
		assert.Equal(t, manifestPath, filepath.ToSlash(errs[0].File))
	})

	t.Run("malformed manifest", func(t *testing.T) {
		root := t.TempDir()
		writeCodexDoc(t, root, manifestPath, `{`)
		var errs []adapter.ValidationError
		validateCodexPluginDocuments(root, &errs)
		require.Len(t, errs, 1)
		assert.Equal(t, manifestPath, filepath.ToSlash(errs[0].File))
	})

	t.Run("malformed marketplace", func(t *testing.T) {
		root := t.TempDir()
		writeCodexDoc(t, root, manifestPath, `{"name":"auto","skills":"./skills"}`)
		writeCodexDoc(t, root, marketPath, `{"plugins":"not-a-list"}`)
		var errs []adapter.ValidationError
		validateCodexPluginDocuments(root, &errs)
		require.Len(t, errs, 1)
		finding, ok := codexDiagnosticFor(errs, marketPath)
		require.True(t, ok)
		assert.Contains(t, finding.Message, "malformed")
	})

	t.Run("marketplace without the autopus entry", func(t *testing.T) {
		root := t.TempDir()
		writeCodexDoc(t, root, marketPath, `{"plugins":[{"name":"other"}]}`)
		var errs []adapter.ValidationError
		validateCodexPluginDocuments(root, &errs)
		require.Len(t, errs, 1)
		finding, ok := codexDiagnosticFor(errs, marketPath)
		require.True(t, ok)
		assert.Contains(t, finding.Message, "marketplace entry")
	})
}

// Guards against the empty-directory pruner deleting a directory that still
// holds user data, and against it leaving empty nested managed directories.
func TestRemoveEmptyCodexDir_PrunesOnlyFullyEmptyTrees(t *testing.T) {
	root := t.TempDir()
	emptyTree := filepath.Join(root, "empty", "deep", "deeper")
	require.NoError(t, os.MkdirAll(emptyTree, 0o755))
	require.NoError(t, removeEmptyCodexDir(filepath.Join(root, "empty")))
	assert.NoFileExists(t, filepath.Join(root, "empty"))

	populated := filepath.Join(root, "kept", "empty-child")
	require.NoError(t, os.MkdirAll(populated, 0o755))
	writeCodexDoc(t, root, "kept/user.md", "user data")
	require.NoError(t, removeEmptyCodexDir(filepath.Join(root, "kept")))
	assert.DirExists(t, filepath.Join(root, "kept"), "a directory with user data survives")
	assert.NoFileExists(t, populated, "its empty child is still pruned")

	assert.NoError(t, removeEmptyCodexDir(filepath.Join(root, "absent")),
		"a missing directory is not an error")
}

// Guards against the pruner following a symlink and deleting outside the
// managed tree.
func TestRemoveEmptyCodexDir_RefusesSymlinksAndFiles(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "outside")
	require.NoError(t, os.MkdirAll(target, 0o755))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(target, link))

	require.NoError(t, removeEmptyCodexDir(link))
	assert.DirExists(t, target, "an empty symlink target must not be removed")
	_, err := os.Lstat(link)
	assert.NoError(t, err, "the symlink itself must survive")

	file := writeCodexDoc(t, root, "plain.txt", "x")
	require.NoError(t, removeEmptyCodexDir(file))
	assert.FileExists(t, file)
}
