package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/orchestra"
)

const (
	defaultOMPReviewTimeout = 30 * time.Minute

	ompReviewHardeningOverlayYAML = `lsp:
  enabled: false
mcp:
  enableProjectConfig: false
web_search:
  enabled: false
secrets:
  enabled: true
memory:
  backend: off
tools:
  approvalMode: yolo
`
)

type pipelineOMPProcessOptions struct {
	ExtraArgs []string
}

func startPipelineOMPProcess(ctx context.Context, config pipelineOMPBackendConfig) (*pipelineOMPProcess, error) {
	return startPipelineOMPProcessWithOptions(ctx, config, pipelineOMPProcessOptions{})
}

func prepareOMPReviewProcessConfig(projectDir string, maxTime time.Duration, unsetEnv []string) (pipelineOMPBackendConfig, string, error) {
	if strings.TrimSpace(projectDir) == "" {
		projectDir = "."
	}
	absoluteProjectDir, err := filepath.Abs(projectDir)
	if err != nil {
		return pipelineOMPBackendConfig{}, "", errors.New("OMP review project directory is invalid")
	}
	projectInfo, err := os.Lstat(absoluteProjectDir)
	if err != nil || !projectInfo.IsDir() || projectInfo.Mode()&os.ModeSymlink != 0 {
		return pipelineOMPBackendConfig{}, "", errors.New("OMP review project directory is unsafe")
	}

	executable, err := exec.LookPath("omp")
	if err != nil {
		return pipelineOMPBackendConfig{}, "", fmt.Errorf("OMP review executable is unavailable: %w", err)
	}
	canonicalExecutable, executableID, err := canonicalPipelineOMPExecutable(executable)
	if err != nil {
		return pipelineOMPBackendConfig{}, "", err
	}
	environment, err := ompReviewEnvironment(unsetEnv)
	if err != nil {
		return pipelineOMPBackendConfig{}, "", err
	}

	runtimeBase, err := os.MkdirTemp(os.TempDir(), "autopus-omp-review-")
	if err != nil {
		return pipelineOMPBackendConfig{}, "", fmt.Errorf("create OMP review runtime base: %w", err)
	}
	cleanup := func(err error) (pipelineOMPBackendConfig, string, error) {
		_ = os.RemoveAll(runtimeBase)
		return pipelineOMPBackendConfig{}, "", err
	}
	if err := os.Chmod(runtimeBase, 0o700); err != nil {
		return cleanup(fmt.Errorf("secure OMP review runtime base: %w", err))
	}
	runtimeInfo, err := os.Lstat(runtimeBase)
	if err != nil || !runtimeInfo.IsDir() || runtimeInfo.Mode()&os.ModeSymlink != 0 || runtimeInfo.Mode().Perm()&0o077 != 0 {
		return cleanup(errors.New("OMP review runtime base is unsafe"))
	}

	return pipelineOMPBackendConfig{
		Executable: canonicalExecutable, ProjectDir: filepath.Clean(absoluteProjectDir),
		RuntimeBase: runtimeBase, Environment: environment,
		canonicalEnv: pipelineOMPCanonicalEnvironment(environment),
		MaxTime:      maxTime, executableID: executableID,
	}, runtimeBase, nil
}

// ompReviewEnvironment is the normalized OMP review environment without the
// inherited variables unsetEnv names (ProviderConfig.UnsetEnv).
func ompReviewEnvironment(unsetEnv []string) ([]string, error) {
	return normalizePipelineOMPEnvironment(orchestra.EnvironWithout(os.Environ(), unsetEnv))
}

func writeOMPReviewHardeningOverlay(runtimeBase string) (string, error) {
	absoluteRuntimeBase, err := filepath.Abs(runtimeBase)
	if err != nil {
		return "", fmt.Errorf("resolve OMP review runtime base: %w", err)
	}
	overlayPath := filepath.Join(absoluteRuntimeBase, "review-hardening.yml")
	cleanup := func(err error) (string, error) {
		_ = os.Remove(overlayPath)
		return "", err
	}
	if err := os.WriteFile(overlayPath, []byte(ompReviewHardeningOverlayYAML), 0o600); err != nil {
		return cleanup(fmt.Errorf("write OMP review hardening overlay: %w", err))
	}
	if err := os.Chmod(overlayPath, 0o600); err != nil {
		return cleanup(fmt.Errorf("secure OMP review hardening overlay: %w", err))
	}
	info, err := os.Lstat(overlayPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm() != 0o600 {
		return cleanup(errors.New("OMP review hardening overlay is unsafe"))
	}
	return overlayPath, nil
}

func ompReviewTimeout(requested time.Duration) time.Duration {
	if requested > 0 {
		return requested
	}
	return defaultOMPReviewTimeout
}

func ompReviewTimeoutSeconds(timeout time.Duration) int {
	seconds := int(timeout / time.Second)
	if timeout%time.Second != 0 {
		seconds++
	}
	if seconds < 1 {
		return 1
	}
	return seconds
}
