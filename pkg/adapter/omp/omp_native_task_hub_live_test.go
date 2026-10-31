package omp

// TestOMPNativeTaskHubLiveSmoke drives the pinned omp through its native
// child lifecycle on a project generated with the native agent source. omp
// 18.3 removed the `hub` tool, so the lifecycle the parent proves is the
// one 18.12.3 offers: one `task` batch spawning the bundled scout and
// reviewer, then one `wait` per child, each returning the child's final
// message inside a `<task-result>`. Children end their turn on the receipt
// text (there is no `yield`), the fake provider holds every child until the
// parent waits for it, and the loopback-only sandbox keeps every request on
// the fake provider. The name keeps the hub era's spelling because CI pins
// it by name.

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

type ompNativeLiveReceipt struct {
	Version              string            `json:"version"`
	ChildIDs             []string          `json:"child_ids"`
	ToolSequence         []string          `json:"tool_sequence"`
	ChildSources         map[string]string `json:"child_sources"`
	LoopbackRequests     int               `json:"loopback_requests"`
	ExternalRequests     int               `json:"external_network_requests"`
	AuthHeaders          int               `json:"auth_headers"`
	CredentialLeaks      int               `json:"credential_leaks"`
	ResidualArtifacts    int               `json:"residual_artifacts"`
	ChildTranscripts     int               `json:"child_transcripts"`
	ChildOverridesAbsent bool              `json:"child_overrides_absent"`
	Status               string            `json:"status"`
}

func TestOMPNativeTaskHubLiveSmoke(t *testing.T) {
	if os.Getenv("AUTOPUS_OMP_NATIVE_LIVE") != "1" {
		t.Skip("set AUTOPUS_OMP_NATIVE_LIVE=1 to run the isolated native task/hub smoke")
	}
	for _, key := range []string{
		"ANTHROPIC_API_KEY", "AWS_SECRET_ACCESS_KEY", "GITHUB_TOKEN", "GOOGLE_APPLICATION_CREDENTIALS",
		"OPENAI_API_KEY", "OMP_ACCESS_TOKEN", "PI_PROVIDER_TOKEN", "SSH_AUTH_SOCK",
	} {
		t.Setenv(key, ompNativeCredential)
	}
	executable := resolveOMPNativeLiveExecutable(t)
	// The batch names the bundled scout and reviewer, so the project carries
	// no agent surface: autopus mode replaces reviewer and disables scout
	// (TestOMPAutopusAgentSurfaceLiveSmoke covers that surface).
	scratch := generateOMPOnlyWithoutAgentSurface(t)
	baseline := snapshotOMPNativeTree(t, scratch)

	provider := newOMPNativeProvider(t)
	profile := t.TempDir()
	writeOMPLiveModelConfig(t, profile, provider.server.URL)
	overlay := writeOMPNativeLiveOverlay(t, profile)
	version := probeOMPLiveVersion(t, executable, profile, overlay)
	require.Equal(t, "omp/18.12.3", version, "native task/wait schema is pinned to the supported OMP release")

	sandboxCtx, cancelSandbox := context.WithTimeout(context.Background(), 5*time.Second)
	sandboxEvidence, sandboxErr := probeOMPRPCNetworkSandbox(sandboxCtx, provider.server.URL)
	cancelSandbox()
	require.NoError(t, sandboxErr, "OS-level loopback-only sandbox is mandatory")
	require.Equal(t, ompRPCNetworkSandboxEvidence{
		AllowedEndpointConnections: 1, DeniedOtherLoopback: 1, DeniedNonLoopback: 2,
	}, sandboxEvidence)

	prompt := ompNativeSmokeToken + ": issue exactly one task batch with the two named read-only children, " +
		"then wait once per child before finishing."
	// Each wait pays the sandbox's 10s daemon broker timeout before it
	// snapshots the settled child, so the budget is two of those plus slack.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	trace, err := runActualOMPNativeRPC(
		ctx, executable, scratch, profile, overlay, prompt, provider.server.URL, provider,
	)
	providerReceipt := provider.receipt()
	require.NoError(t, err, "native RPC failed: provider=%+v", providerReceipt)
	require.Empty(t, providerReceipt.Failure)
	require.Equal(t, 6, providerReceipt.Requests)
	require.Equal(t, 4, providerReceipt.ParentStages)
	require.Equal(t, map[string]int{ompNativeAlphaID: 1, ompNativeBetaID: 1}, providerReceipt.ChildRequests)
	require.Zero(t, providerReceipt.AuthHeaders)
	require.Zero(t, providerReceipt.CredentialLeaks)
	require.Zero(t, providerReceipt.UnexpectedEndpoints)
	require.Equal(t, ompNativeToolSequence, trace.toolSequence)
	require.Equal(t, len(ompNativeToolSequence), trace.pairedCalls)
	require.Equal(t, map[string]string{ompNativeAlphaID: "bundled", ompNativeBetaID: "bundled"}, trace.childSources,
		"both children must run as omp's bundled agents, not under a parent override")

	after := snapshotOMPNativeTree(t, scratch)
	require.Equal(t, baseline, after, "native children must leave the generated project byte-identical")
	// omp 18.12.3 keeps each child's transcript in TMPDIR after the parent
	// exits, even for a --no-session parent; TMPDIR is the isolated profile's,
	// so the gate pins exactly those files and forbids anything else: nothing
	// in the project, no sessions or worktrees, and no credential on disk.
	transcripts, residual := splitOMPNativeChildTranscripts(profile, findOMPNativeResidualArtifacts(t, scratch, profile))
	require.Empty(t, residual, "task/session/worktree artifacts must not survive process exit")
	require.Len(t, transcripts, 6, "one transcript, lock, and output file per child")
	for _, path := range transcripts {
		data, readErr := os.ReadFile(path)
		require.NoError(t, readErr)
		assert.NotContains(t, string(data), ompNativeCredential, path)
		assert.NotContains(t, string(data), provider.server.URL, path)
	}

	receipt := ompNativeLiveReceipt{
		Version: version, ChildIDs: []string{ompNativeAlphaID, ompNativeBetaID}, ToolSequence: trace.toolSequence,
		ChildSources: trace.childSources, LoopbackRequests: providerReceipt.Requests, ExternalRequests: 0,
		AuthHeaders: providerReceipt.AuthHeaders, CredentialLeaks: providerReceipt.CredentialLeaks,
		ResidualArtifacts: len(residual), ChildTranscripts: len(transcripts), ChildOverridesAbsent: true,
		Status: "completed",
	}
	encoded, marshalErr := json.Marshal(receipt)
	require.NoError(t, marshalErr)
	for _, forbidden := range []string{ompNativeCredential, executable, os.Getenv("HOME"), provider.server.URL} {
		if forbidden != "" {
			assert.NotContains(t, string(encoded), forbidden)
		}
	}
	t.Logf("sanitized_native_task_hub_receipt=%s", encoded)
}

// generateOMPOnlyWithoutAgentSurface is generateOMPOnly with the agent
// surface stripped, the layout agent_source: native left before v0.50.131:
// no project agent definition and no task.disabledAgents, so OMP serves its
// bundled registry and the smoke's children are bundled agents.
func generateOMPOnlyWithoutAgentSurface(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cfg := configForOMP()
	require.NoError(t, config.Save(dir, cfg))
	_, err := NewWithRoot(dir).Generate(context.Background(), cfg)
	require.NoError(t, err)
	stripOMPAgentSurface(t, dir)
	return dir
}

func resolveOMPNativeLiveExecutable(t *testing.T) string {
	t.Helper()
	if explicit := os.Getenv("AUTOPUS_OMP_LIVE_EXECUTABLE"); explicit != "" {
		require.True(t, filepath.IsAbs(explicit), "explicit OMP live executable must be absolute")
		info, err := os.Lstat(explicit)
		require.NoError(t, err)
		require.True(t, info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0,
			"explicit OMP live executable must be a regular executable")
		return explicit
	}
	executable, err := exec.LookPath(cliBinary)
	require.NoError(t, err, "actual omp binary is required for the native live gate")
	return executable
}

// writeOMPNativeLiveOverlay is the profile overlay of both native-binary
// smokes. task.maxRuntimeMs is a safety cap, not a budget: a child lives
// from its spawn until the parent's wait releases it, and the second child
// sits behind the first wait's 10s broker timeout before that happens.
func writeOMPNativeLiveOverlay(t *testing.T, profile string) string {
	t.Helper()
	path := filepath.Join(profile, "native-live-config.yml")
	content := `skills:
  enableCodexUser: false
  enableClaudeUser: false
  enableClaudeProject: false
  enablePiUser: false
  enablePiProject: false
  enableAgentsUser: false
  enableAgentsProject: false
  enableSkillCommands: true
async:
  enabled: true
  maxJobs: 2
  pollWaitDuration: 5s
task:
  batch: true
  enableEffort: false
  maxConcurrency: 2
  maxRuntimeMs: 40000
  agentIdleTtlMs: 60000
  isolation:
    enabled: false
tools:
  approvalMode: yolo
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func snapshotOMPNativeTree(t *testing.T, root string) map[string]string {
	t.Helper()
	result := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		key := filepath.ToSlash(relative)
		switch {
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[key] = fmt.Sprintf("file:%o:%x", info.Mode().Perm(), sha256.Sum256(data))
		case info.Mode()&os.ModeSymlink != 0:
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			result[key] = "symlink:" + target
		case info.IsDir():
			result[key] = fmt.Sprintf("dir:%o", info.Mode().Perm())
		default:
			result[key] = info.Mode().String()
		}
		return nil
	})
	require.NoError(t, err)
	return result
}

func findOMPNativeResidualArtifacts(t *testing.T, roots ...string) []string {
	t.Helper()
	var residual []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if path == root {
				return nil
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			normalized := strings.ToLower(filepath.ToSlash(relative))
			base := strings.ToLower(entry.Name())
			if strings.Contains(base, strings.ToLower(ompNativeAlphaID)) ||
				strings.Contains(base, strings.ToLower(ompNativeBetaID)) ||
				strings.HasSuffix(base, ".jsonl") || strings.HasSuffix(base, ".patch") ||
				strings.Contains("/"+normalized+"/", "/sessions/") ||
				strings.Contains("/"+normalized+"/", "/worktrees/") ||
				strings.Contains("/"+normalized+"/", "/.omp/wt/") {
				residual = append(residual, filepath.Join(root, relative))
			}
			return nil
		})
		require.NoError(t, err)
	}
	sort.Strings(residual)
	return residual
}

// splitOMPNativeChildTranscripts separates the child transcript files omp
// 18.12.3 writes under TMPDIR/omp-task-<id>/ (<Child>.jsonl, its lock, and
// <Child>.md holding the final message) from every other residual path.
func splitOMPNativeChildTranscripts(profile string, residual []string) (transcripts, other []string) {
	allowed := make(map[string]bool, 6)
	for _, child := range []string{ompNativeAlphaID, ompNativeBetaID} {
		allowed[child+".jsonl"] = true
		allowed["."+child+".jsonl.lock.os"] = true
		allowed[child+".md"] = true
	}
	tmp := filepath.Join(profile, "tmp")
	for _, path := range residual {
		parent := filepath.Dir(path)
		if allowed[filepath.Base(path)] && filepath.Dir(parent) == tmp &&
			strings.HasPrefix(filepath.Base(parent), "omp-task-") {
			transcripts = append(transcripts, path)
			continue
		}
		other = append(other, path)
	}
	return transcripts, other
}
