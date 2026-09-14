package omp

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
)

type ompNativeLiveReceipt struct {
	Version              string   `json:"version"`
	ChildIDs             []string `json:"child_ids"`
	HubOps               []string `json:"hub_ops"`
	LoopbackRequests     int      `json:"loopback_requests"`
	ExternalRequests     int      `json:"external_network_requests"`
	AuthHeaders          int      `json:"auth_headers"`
	CredentialLeaks      int      `json:"credential_leaks"`
	ResidualArtifacts    int      `json:"residual_artifacts"`
	ChildOverridesAbsent bool     `json:"child_overrides_absent"`
	Status               string   `json:"status"`
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
	scratch := generateOMPOnly(t)
	baseline := snapshotOMPNativeTree(t, scratch)

	provider := newOMPNativeProvider(t)
	profile := t.TempDir()
	writeOMPLiveModelConfig(t, profile, provider.server.URL)
	overlay := writeOMPNativeLiveOverlay(t, profile)
	version := probeOMPLiveVersion(t, executable, profile, overlay)
	require.Equal(t, "omp/17.2.7", version, "native task/hub schema is pinned to the supported OMP release")

	sandboxCtx, cancelSandbox := context.WithTimeout(context.Background(), 5*time.Second)
	sandboxEvidence, sandboxErr := probeOMPRPCNetworkSandbox(sandboxCtx, provider.server.URL)
	cancelSandbox()
	require.NoError(t, sandboxErr, "OS-level loopback-only sandbox is mandatory")
	require.Equal(t, ompRPCNetworkSandboxEvidence{
		AllowedEndpointConnections: 1, DeniedOtherLoopback: 1, DeniedNonLoopback: 2,
	}, sandboxEvidence)

	prompt := ompNativeSmokeToken + ": issue exactly one task batch with the two named read-only children, " +
		"then observe them only through hub list, jobs, one wait per child, and final list before finishing."
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	trace, err := runActualOMPNativeRPC(
		ctx, executable, scratch, profile, overlay, prompt, provider.server.URL, provider,
	)
	providerReceipt := provider.receipt()
	require.NoError(t, err, "native RPC failed: provider=%+v", providerReceipt)
	require.Empty(t, providerReceipt.Failure)
	require.Equal(t, 10, providerReceipt.Requests)
	require.Equal(t, 8, providerReceipt.ParentStages)
	require.Equal(t, map[string]int{ompNativeAlphaID: 1, ompNativeBetaID: 1}, providerReceipt.ChildRequests)
	require.Zero(t, providerReceipt.AuthHeaders)
	require.Zero(t, providerReceipt.CredentialLeaks)
	require.Zero(t, providerReceipt.UnexpectedEndpoints)
	require.Equal(t, []string{
		"task", "hub:list", "hub:jobs", "hub:wait", "hub:jobs", "hub:wait", "hub:list",
	}, trace.toolSequence)
	require.Equal(t, []string{"list", "jobs", "wait", "jobs", "wait", "list"}, trace.hubOps)
	require.Equal(t, 7, trace.pairedCalls)

	after := snapshotOMPNativeTree(t, scratch)
	require.Equal(t, baseline, after, "native children must leave the generated project byte-identical")
	residual := findOMPNativeResidualArtifacts(t, scratch, profile)
	require.Empty(t, residual, "task/session/worktree artifacts must not survive process exit")

	receipt := ompNativeLiveReceipt{
		Version: version, ChildIDs: []string{ompNativeAlphaID, ompNativeBetaID}, HubOps: trace.hubOps,
		LoopbackRequests: providerReceipt.Requests, ExternalRequests: 0,
		AuthHeaders: providerReceipt.AuthHeaders, CredentialLeaks: providerReceipt.CredentialLeaks,
		ResidualArtifacts: len(residual), ChildOverridesAbsent: true, Status: "completed",
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
  maxRuntimeMs: 20000
  agentIdleTtlMs: 60000
  isolation:
    mode: none
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
