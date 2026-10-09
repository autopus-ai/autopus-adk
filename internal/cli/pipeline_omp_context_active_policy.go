package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/insajin/autopus-adk/pkg/promptlayer"
	"github.com/insajin/autopus-adk/pkg/version"
)

const pipelineOMPActiveStaticPolicyMaxBytes = 16 * 1024

// pipelineOMPActiveStaticPolicyB64 is populated only by release ldflags. It is
// never read from an environment variable or a workspace file.
var pipelineOMPActiveStaticPolicyB64 string

func compiledPipelineOMPActiveStaticPolicy(
	_ pipelineOMPManagedActiveCandidate,
) (promptlayer.OMPContextPromotionStaticPolicyV3, error) {
	encoded := strings.TrimSpace(pipelineOMPActiveStaticPolicyB64)
	if encoded == "" || encoded != pipelineOMPActiveStaticPolicyB64 {
		return promptlayer.OMPContextPromotionStaticPolicyV3{}, errors.New("pipeline: managed active trust policy is not compiled")
	}
	body, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(body) == 0 || len(body) > pipelineOMPActiveStaticPolicyMaxBytes {
		return promptlayer.OMPContextPromotionStaticPolicyV3{}, errors.New("pipeline: compiled managed active trust policy is invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var policy promptlayer.OMPContextPromotionStaticPolicyV3
	if decoder.Decode(&policy) != nil {
		return policy, errors.New("pipeline: compiled managed active trust policy is invalid")
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		return policy, errors.New("pipeline: compiled managed active trust policy contains trailing JSON")
	}
	canonical, err := json.Marshal(policy)
	if err != nil || !bytes.Equal(canonical, body) {
		return policy, errors.New("pipeline: compiled managed active trust policy is non-canonical")
	}
	if err := promptlayer.ValidateOMPContextPromotionActiveStaticPolicyV3(policy); err != nil {
		return policy, errors.New("pipeline: compiled managed active trust policy is invalid")
	}
	return policy, nil
}

func newPipelineOMPActiveCurrentRuntimeProvider(
	config pipelineOMPBackendConfig,
) pipelineOMPActiveCurrentRuntimeProvider {
	return newPipelineOMPActiveCurrentRuntimeProviderWithHooks(config, pipelineOMPActiveCurrentRuntimeHooks{})
}

type pipelineOMPActiveCurrentRuntimeHooks struct {
	beforeVersionProbe     func()
	afterVerifiedVersionFD func()
}

func newPipelineOMPActiveCurrentRuntimeProviderWithHooks(
	config pipelineOMPBackendConfig,
	hooks pipelineOMPActiveCurrentRuntimeHooks,
) pipelineOMPActiveCurrentRuntimeProvider {
	return func(ctx context.Context, candidate pipelineOMPManagedActiveCandidate,
		expected promptlayer.OMPContextPromotionStaticPolicyV3,
	) (promptlayer.OMPContextPromotionCurrentRuntimeV3, error) {
		autoExecutableSHA256, err := pipelineOMPActiveCurrentAutoExecutableSHA256()
		if err != nil {
			return promptlayer.OMPContextPromotionCurrentRuntimeV3{}, err
		}
		endpoint, endpointFound := pipelineOMPEnvironmentValue(config.Environment, pipelineOMPActiveEndpointKey)
		credential, credentialFound := pipelineOMPEnvironmentValue(config.Environment, pipelineOMPActiveCredentialKey)
		if !endpointFound || !credentialFound {
			return promptlayer.OMPContextPromotionCurrentRuntimeV3{}, errors.New("pipeline: active OMP provider authority is unavailable")
		}
		providerAuthority, err := pipelineOMPActiveProviderAuthorityDigest(
			expected.PolicyDigest, expected.PipelineImplementationDigest,
			candidate.ModelScopeDigest, config.ModelContextWindow, endpoint, credential,
		)
		if err != nil {
			return promptlayer.OMPContextPromotionCurrentRuntimeV3{}, err
		}
		if err := verifyPipelineOMPExecutable(config.Executable, config.executableID); err != nil {
			return promptlayer.OMPContextPromotionCurrentRuntimeV3{}, err
		}
		if hooks.beforeVersionProbe != nil {
			hooks.beforeVersionProbe()
		}
		ompVersion, err := probePipelineOMPActiveVersion(ctx, config, hooks)
		identityErr := verifyPipelineOMPExecutable(config.Executable, config.executableID)
		if err != nil || identityErr != nil {
			return promptlayer.OMPContextPromotionCurrentRuntimeV3{}, errors.New("pipeline: active OMP identity probe failed")
		}
		return promptlayer.OMPContextPromotionCurrentRuntimeV3{
			ExecutableSHA256: autoExecutableSHA256,
			SourceCommit:     candidate.AutoSourceCommit, SourceTree: candidate.AutoSourceTree,
			Target: runtime.GOOS + "-" + runtime.GOARCH, AutoVersion: version.Version(),
			OMPVersion: ompVersion, OMPExecutableSHA256: fmt.Sprintf("sha256:%x", config.executableID.digest[:]),
			PipelineImplementationDigest: expected.PipelineImplementationDigest,
			ProviderAuthorityDigest:      providerAuthority,
		}, nil
	}
}

const pipelineOMPActiveVersionProbeMaxBytes = 4 << 10

type pipelineOMPActiveProbeOutput struct {
	mu       sync.Mutex
	data     []byte
	exceeded bool
	stop     context.CancelFunc
}

func (output *pipelineOMPActiveProbeOutput) Write(data []byte) (int, error) {
	written := len(data)
	output.mu.Lock()
	remaining := pipelineOMPActiveVersionProbeMaxBytes - len(output.data)
	if remaining >= len(data) {
		output.data = append(output.data, data...)
		output.mu.Unlock()
		return written, nil
	}
	if remaining > 0 {
		output.data = append(output.data, data[:remaining]...)
	}
	output.exceeded = true
	output.mu.Unlock()
	output.stop()
	return written, errors.New("managed active OMP version output exceeded limit")
}

func (output *pipelineOMPActiveProbeOutput) result() ([]byte, bool) {
	output.mu.Lock()
	defer output.mu.Unlock()
	return bytes.Clone(output.data), output.exceeded
}

// pipelineOMPActiveVersionProbeTimeout bounds `omp --version` under the probe
// sandbox. A healthy probe answers in well under a second; the ceiling only
// matters for a hang. Five seconds was too tight for the first sandboxed launch
// of a freshly installed binary on a loaded hosted runner (v0.50.124 release).
const pipelineOMPActiveVersionProbeTimeout = 15 * time.Second

func probePipelineOMPActiveVersion(
	ctx context.Context,
	config pipelineOMPBackendConfig,
	hooks pipelineOMPActiveCurrentRuntimeHooks,
) (version string, resultErr error) {
	probeCtx, cancel := context.WithTimeout(ctx, pipelineOMPActiveVersionProbeTimeout)
	defer cancel()
	verifiedCommand, err := newPipelineOMPVerifiedExecCommandContext(
		probeCtx, config.Executable, config.executableID, "--version",
	)
	if err != nil {
		return "", err
	}
	defer func() { resultErr = errors.Join(resultErr, verifiedCommand.Close()) }()
	if hooks.afterVerifiedVersionFD != nil {
		hooks.afterVerifiedVersionFD()
	}
	cmd := verifiedCommand.cmd
	cmd.Dir = filepath.Dir(config.Executable)
	cmd.Env = []string{"PATH=/usr/bin:/bin", "LANG=C", "LC_ALL=C"}
	if err := configurePipelineOMPActiveVersionProbeSandbox(cmd, config.Executable); err != nil {
		return "", err
	}
	if err := configureWorkflowContextManagedRPCProcessGroup(cmd); err != nil {
		return "", err
	}
	stdout := &pipelineOMPActiveProbeOutput{stop: cancel}
	stderr := &pipelineOMPActiveProbeOutput{stop: cancel}
	cmd.Stdout, cmd.Stderr, cmd.WaitDelay = stdout, stderr, 500*time.Millisecond
	if err := verifiedCommand.Start(); err != nil {
		return "", err
	}
	waitErr := cmd.Wait()
	body, stdoutExceeded := stdout.result()
	_, stderrExceeded := stderr.result()
	if waitErr != nil || stdoutExceeded || stderrExceeded {
		killErr := terminateWorkflowContextManagedRPCProcessGroup(cmd)
		return "", errors.Join(errors.New("managed active OMP version probe failed"), waitErr, killErr)
	}
	ompVersion := strings.TrimSpace(string(body))
	if !installedOMPVersionPattern.MatchString(ompVersion) {
		return "", errors.New("managed active OMP version probe returned invalid identity")
	}
	return ompVersion, nil
}

func configurePipelineOMPActiveVersionProbeSandbox(cmd *exec.Cmd, expectedPath string) error {
	if runtime.GOOS == "linux" {
		if cmd == nil || cmd.Path != "/proc/self/fd/3" || len(cmd.Args) != 2 || cmd.Args[1] != "--version" {
			return errors.New("managed active OMP inode-bound version command is unsafe")
		}
		return nil
	}
	if runtime.GOOS != "darwin" {
		return errors.New("managed active OMP authority-free version sandbox is unavailable")
	}
	const executable = "/usr/bin/sandbox-exec"
	info, err := os.Lstat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 ||
		cmd == nil || cmd.Path != expectedPath || len(cmd.Args) != 2 || cmd.Args[1] != "--version" {
		return errors.New("managed active OMP authority-free version sandbox is unsafe")
	}
	profile := "(version 1)\n(allow default)\n(deny network*)\n(deny file-write*)\n"
	cmd.Path = executable
	cmd.Args = []string{executable, "-p", profile, expectedPath, "--version"}
	return nil
}

func materializePipelineOMPActiveRuntimeConfig(
	config pipelineOMPBackendConfig,
) (pipelineOMPBackendConfig, string, error) {
	runtimeRoot, err := os.MkdirTemp(config.RuntimeBase, "pipeline-active-executable-")
	if err != nil {
		return config, "", fmt.Errorf("create managed active executable runtime: %w", err)
	}
	failed := true
	defer func() {
		if failed {
			_ = os.RemoveAll(runtimeRoot)
		}
	}()
	if err := os.Chmod(runtimeRoot, 0o700); err != nil {
		return config, "", fmt.Errorf("secure managed active executable runtime: %w", err)
	}
	rootInfo, err := os.Lstat(runtimeRoot)
	if err != nil || !rootInfo.IsDir() || rootInfo.Mode()&os.ModeSymlink != 0 || rootInfo.Mode().Perm() != 0o700 {
		return config, "", errors.New("pipeline: managed active executable runtime is unsafe")
	}
	privateExecutable, err := materializePipelineOMPExecutable(
		config.Executable, config.executableID, runtimeRoot,
	)
	if err != nil {
		return config, "", err
	}
	privateExecutable, privateIdentity, err := canonicalPipelineOMPExecutable(privateExecutable)
	resolvedRoot, rootErr := filepath.EvalSymlinks(runtimeRoot)
	if err != nil || privateIdentity.size != config.executableID.size ||
		privateIdentity.digest != config.executableID.digest || privateIdentity.mode.Perm() != 0o700 ||
		rootErr != nil || filepath.Dir(privateExecutable) != filepath.Clean(resolvedRoot) {
		return config, "", errors.New("pipeline: managed active private executable identity is invalid")
	}
	config.Executable, config.executableID = privateExecutable, privateIdentity
	failed = false
	return config, runtimeRoot, nil
}

func pipelineOMPActiveCurrentAutoExecutableSHA256() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", errors.New("pipeline: active auto executable is unavailable")
	}
	_, identity, err := canonicalPipelineOMPExecutable(executable)
	if err != nil {
		return "", errors.New("pipeline: active auto executable identity is unavailable")
	}
	return fmt.Sprintf("sha256:%x", identity.digest[:]), nil
}
