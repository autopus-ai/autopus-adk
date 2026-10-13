package omp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/insajin/autopus-adk/pkg/config"
)

const claimSurfaceOnly = "task:\n  disabledAgents:\n    - scout\n    - security-reviewer\n    - sonic\n"

type claimProject struct {
	t      *testing.T
	root   string
	runner *modelIntegrationFakeRunner
}

func newClaimProject(t *testing.T) *claimProject {
	t.Helper()
	project := &claimProject{t: t, root: t.TempDir(), runner: newModelIntegrationRunner()}
	project.write(".keep", "")
	return project
}

func (p *claimProject) adapter() *Adapter {
	return NewWithRoot(p.root).WithModelIntegrationRunner(p.runner)
}

func (p *claimProject) run(operation string, cfg *config.HarnessConfig) error {
	p.t.Helper()
	require.NoError(p.t, config.Save(p.root, cfg))
	var err error
	if operation == "generate" {
		_, err = p.adapter().Generate(context.Background(), cfg)
	} else {
		_, err = p.adapter().Update(context.Background(), cfg)
	}
	return err
}

func (p *claimProject) mustRun(operation string, cfg *config.HarnessConfig) {
	p.t.Helper()
	require.NoError(p.t, p.run(operation, cfg), operation)
}

func (p *claimProject) write(relative, content string) {
	p.t.Helper()
	path := filepath.Join(p.root, filepath.FromSlash(relative))
	require.NoError(p.t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(p.t, os.WriteFile(path, []byte(content), 0o600))
}

func (p *claimProject) read(relative string) (string, bool) {
	p.t.Helper()
	data, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(relative)))
	if errors.Is(err, os.ErrNotExist) {
		return "", false
	}
	require.NoError(p.t, err)
	return string(data), true
}

func (p *claimProject) ledger() (ompModelProjectOwnership, bool) {
	p.t.Helper()
	data, ok := p.read(OMPModelProjectOwnershipRelativePath)
	if !ok {
		return ompModelProjectOwnership{}, false
	}
	var ownership ompModelProjectOwnership
	require.NoError(p.t, json.Unmarshal([]byte(data), &ownership))
	return ownership, true
}

func (p *claimProject) preimage() ([]byte, bool) {
	p.t.Helper()
	workspace, err := openOMPRootedWorkspace(p.root)
	require.NoError(p.t, err)
	defer func() { require.NoError(p.t, workspace.Close()) }()
	state, err := evaluateOMPClaimSetAt(workspace, true)
	require.NoError(p.t, err)
	return state.preimage, state.preimageMissing
}

func claimOwners(ownership ompModelProjectOwnership) map[string]string {
	owners := map[string]string{}
	for _, key := range ownership.ManagedKeys {
		owners[key.Path] = key.Owner
	}
	return owners
}

func claimDisabledAgents(t *testing.T, data string) []any {
	t.Helper()
	var document map[string]any
	require.NoError(t, yaml.Unmarshal([]byte(data), &document))
	task, _ := document["task"].(map[string]any)
	value, _ := task["disabledAgents"].([]any)
	return value
}

func nativeClaimConfig(cfg *config.HarnessConfig) *config.HarnessConfig {
	cfg.RoleModelPolicy.AgentSource = config.OMPAgentSourceNative
	return cfg
}

// S7: with no project config, autopus mode creates one that only disables the
// three replaced bundled agents, owner-only, recorded as an agent_surface claim.
func TestOMPClaimSet_CreatesTheDisabledAgentsClaimOnAMissingConfig(t *testing.T) {
	project := newClaimProject(t)
	project.mustRun("generate", configForOMP())

	data, ok := project.read(configFile)
	require.True(t, ok)
	assert.Equal(t, claimSurfaceOnly, data)
	info, err := os.Stat(filepath.Join(project.root, configFile))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	assert.Contains(t, manifestPaths(t, project.root), configFile)
	ownership, ok := project.ledger()
	require.True(t, ok)
	assert.True(t, ownership.OriginalMissing)
	assert.Equal(t, map[string]string{ompDisabledAgentsKey: ompAgentSurfaceOwner}, claimOwners(ownership))
	ledger, _ := project.read(OMPModelProjectOwnershipRelativePath)

	project.mustRun("update", configForOMP())
	again, _ := project.read(configFile)
	assert.Equal(t, data, again)
	ledgerAgain, _ := project.read(OMPModelProjectOwnershipRelativePath)
	assert.Equal(t, ledger, ledgerAgain, "an unchanged generation keeps the ledger bytes")
}

// S8 (UC): a value the user chose before ADK ever claimed the key is the
// user's from the start: the file keeps its bytes and only a mark remains.
func TestOMPClaimSet_LeavesAUserChosenValueAndMarksIt(t *testing.T) {
	project := newClaimProject(t)
	user := "task:\n  disabledAgents:\n    - browser\n"
	project.write(configFile, user)
	project.mustRun("generate", configForOMP())

	data, _ := project.read(configFile)
	assert.Equal(t, user, data)
	ownership, ok := project.ledger()
	require.True(t, ok)
	assert.True(t, ownership.markerOnly(), "%+v", ownership)
	assert.NotContains(t, manifestPaths(t, project.root), configFile)
	ledger, _ := project.read(OMPModelProjectOwnershipRelativePath)

	project.mustRun("update", configForOMP())
	project.mustRun("update", configForOMP())
	again, _ := project.read(configFile)
	assert.Equal(t, user, again)
	ledgerAgain, _ := project.read(OMPModelProjectOwnershipRelativePath)
	assert.Equal(t, ledger, ledgerAgain)
}

// S8 (UC2): the default value already in place is claimed as the preimage
// value, so native mode's release leaves it where the user put it.
func TestOMPClaimSet_ClaimsTheDefaultValueAndReleaseKeepsIt(t *testing.T) {
	project := newClaimProject(t)
	project.write(configFile, claimSurfaceOnly)
	project.mustRun("generate", configForOMP())

	data, _ := project.read(configFile)
	assert.Equal(t, claimSurfaceOnly, data)
	ownership, ok := project.ledger()
	require.True(t, ok)
	key, claimed := ownership.agentSurfaceKey()
	require.True(t, claimed)
	assert.Equal(t, ompDefaultDisabledAgentsFingerprint(), key.PriorFingerprint)

	project.mustRun("update", nativeClaimConfig(configForOMP()))
	data, ok = project.read(configFile)
	require.True(t, ok)
	assert.Equal(t, claimSurfaceOnly, data, "the preimage held the key, so it stays")
	_, ledgerLeft := project.ledger()
	assert.False(t, ledgerLeft)
}

// A project config the managed-claim parser refuses (unparseable, several
// documents, or a symlink) is left exactly as it is: the claim is skipped,
// nothing is read through the link, and no mark is written, so a later fixed
// file is decided like a first claim.
func TestOMPClaimSet_SkipsAConfigTheParserRefuses(t *testing.T) {
	for name, content := range map[string]string{
		"unparseable":    "model: keep-me\n---\n\tthis: is not [valid yaml\n",
		"multi-document": "model: keep-me\n---\nother: value\n",
	} {
		t.Run(name, func(t *testing.T) {
			project := newClaimProject(t)
			project.write(configFile, content)
			project.mustRun("generate", configForOMP())
			data, _ := project.read(configFile)
			assert.Equal(t, content, data)
			_, ledger := project.ledger()
			assert.False(t, ledger)
			assert.NotContains(t, manifestPaths(t, project.root), configFile)
		})
	}
	t.Run("symlink", func(t *testing.T) {
		project := newClaimProject(t)
		secret := filepath.Join(project.root, "outside-secret.yml")
		require.NoError(t, os.WriteFile(secret, []byte("apiKey: SUPER-SECRET\n"), 0o600))
		require.NoError(t, os.MkdirAll(filepath.Join(project.root, ".omp"), 0o755))
		if err := os.Symlink(secret, filepath.Join(project.root, configFile)); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		project.mustRun("generate", configForOMP())
		data, err := os.ReadFile(secret)
		require.NoError(t, err)
		assert.Equal(t, "apiKey: SUPER-SECRET\n", string(data))
		info, err := os.Lstat(filepath.Join(project.root, configFile))
		require.NoError(t, err)
		assert.NotZero(t, info.Mode()&os.ModeSymlink, "the link itself is left in place")
	})
}
