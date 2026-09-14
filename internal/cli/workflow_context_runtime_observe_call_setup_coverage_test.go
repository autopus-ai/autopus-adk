package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/promptlayer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Each row is a distinct rejection rule of the observe-call task endpoint:
// a widened rule would let a remote or credential-bearing URL reach the child.
func TestValidateWorkflowContextObserveEndpoint_RejectsEveryNonLoopbackForm(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{
		"",
		"https://127.0.0.1:43123",
		"http://user:pass@127.0.0.1:43123",
		"http://127.0.0.1:43123/v1",
		"http://127.0.0.1:43123?k=v",
		"http://127.0.0.1:43123#f",
		"http://localhost:43123",
		"http://10.0.0.1:43123",
		"http://127.0.0.1",
		"http://[::1]",
	} {
		got, err := validateWorkflowContextObserveEndpoint(raw)
		require.Error(t, err, "endpoint %q must be refused", raw)
		assert.Empty(t, got)
	}
}

func TestValidateWorkflowContextObserveEndpoint_NormalizesAcceptedLoopbackEndpoint(t *testing.T) {
	t.Parallel()
	got, err := validateWorkflowContextObserveEndpoint("  http://127.0.0.1:43123/  ")
	require.NoError(t, err)
	assert.Equal(t, "http://127.0.0.1:43123", got)

	got, err = validateWorkflowContextObserveEndpoint("http://[::1]:43123")
	require.NoError(t, err)
	assert.Equal(t, "http://[::1]:43123", got)
}

func TestCanonicalWorkflowContextObserveProject_AcceptsOnlyResolvedDirectories(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	target := filepath.Join(root, "real")
	require.NoError(t, os.Mkdir(target, 0o700))
	file := filepath.Join(root, "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))
	link := filepath.Join(root, "link")
	require.NoError(t, os.Symlink(target, link))

	// A symlinked project must collapse to its target, never stay a symlink.
	got, err := canonicalWorkflowContextObserveProject(link)
	require.NoError(t, err)
	resolved, err := filepath.EvalSymlinks(target)
	require.NoError(t, err)
	assert.Equal(t, resolved, got)

	_, err = canonicalWorkflowContextObserveProject(file)
	require.ErrorContains(t, err, "project directory is invalid")
	_, err = canonicalWorkflowContextObserveProject(filepath.Join(root, "missing"))
	require.ErrorContains(t, err, "project directory is invalid")
}

func TestCopyWorkflowContextObserveDocuments_RefusesNonRegularOrOversizedSources(t *testing.T) {
	t.Parallel()
	source := t.TempDir()
	target := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(source, "docs"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(source, "docs", "ok.md"), []byte("BODY"), 0o600))
	require.NoError(t, os.Symlink(filepath.Join(source, "docs", "ok.md"), filepath.Join(source, "linked.md")))
	require.NoError(t, os.WriteFile(filepath.Join(source, "huge.md"), make([]byte, (1<<20)+1), 0o600))

	require.NoError(t, copyWorkflowContextObserveDocuments(source, target,
		[]promptlayer.ContextDeliveryDocument{{SourceRef: "docs/ok.md"}}))
	copied := filepath.Join(target, "docs", "ok.md")
	body, err := os.ReadFile(copied)
	require.NoError(t, err)
	assert.Equal(t, "BODY", string(body))
	info, err := os.Lstat(copied)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	for _, ref := range []string{"linked.md", "huge.md", "absent.md", "docs"} {
		err := copyWorkflowContextObserveDocuments(source, target,
			[]promptlayer.ContextDeliveryDocument{{SourceRef: ref}})
		require.ErrorContains(t, err, "canonical source identity is invalid", "ref %q", ref)
	}
}

func TestWriteWorkflowContextObserveModels_EmitsLocatorReferenceNotCredentialValue(t *testing.T) {
	t.Parallel()
	runtimeRoot := t.TempDir()
	options := workflowContextObserveCallOptions{
		Provider: "openai", Model: "gpt-5.6-sol", CredentialLocator: "AUTOPUS_OBSERVE_TOKEN",
	}

	require.NoError(t, writeWorkflowContextObserveModels(runtimeRoot, options, "http://127.0.0.1:43123"))
	path := filepath.Join(runtimeRoot, "models.yml")
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	// The authority file must carry the locator name so no literal secret lands on disk.
	assert.Contains(t, string(body), "apiKey: AUTOPUS_OBSERVE_TOKEN")
	assert.Contains(t, string(body), "baseUrl: http://127.0.0.1:43123/v1")
	assert.Contains(t, string(body), "id: gpt-5.6-sol")
	info, err := os.Lstat(path)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	err = writeWorkflowContextObserveModels(filepath.Join(runtimeRoot, "missing"), options, "http://127.0.0.1:43123")
	require.ErrorContains(t, err, "write observe-call model authority")
}

// The observe-call harness must not inherit memory or non-probed capability:
// a loosened profile would make the canary observe a different runtime contract.
func TestWorkflowContextObserveHarnessConfig_PinsIsolatedProbeRequiredActiveProfile(t *testing.T) {
	t.Parallel()
	harness := workflowContextObserveHarnessConfig("SPEC-OMP-004")
	require.NotNil(t, harness)
	assert.Equal(t, []string{"omp"}, harness.Platforms)
	assert.Equal(t, "omp-observe-spec-omp-004", harness.ProjectName)
	assert.Equal(t, "active", harness.OMPContextPolicy.Profile)
	profile, ok := harness.OMPContextPolicy.Profiles["active"]
	require.True(t, ok)
	assert.Equal(t, config.OMPContextHistoryActive, profile.HistoryMode)
	assert.Equal(t, config.OMPContextMemoryOff, profile.MemoryMode)
	assert.Equal(t, config.OMPContextCapabilityProbeRequired, profile.CapabilityPolicy)
	assert.Equal(t, config.OMPContextRuntimeIsolatedTaskOwned, profile.RuntimeRootPolicy)
	assert.Equal(t, config.OMPContextMutationSessionOverlay, profile.MutationScope)
	assert.Equal(t, config.OMPContextFallbackCanonicalFull, profile.Fallback)
}

func TestWorkflowContextObservePathGone_DistinguishesRemovalFromDeniedParent(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	path := filepath.Join(root, "task-root")
	require.NoError(t, os.Mkdir(path, 0o700))
	assert.False(t, workflowContextObservePathGone(path))
	require.NoError(t, os.RemoveAll(path))
	assert.True(t, workflowContextObservePathGone(path))
	// A still-present path under a long name is not "gone" either.
	nested := filepath.Join(root, strings.Repeat("d", 32))
	require.NoError(t, os.Mkdir(nested, 0o700))
	assert.False(t, workflowContextObservePathGone(nested))
}
