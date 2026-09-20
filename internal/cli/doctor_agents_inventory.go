package cli

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"time"

	"github.com/insajin/autopus-adk/pkg/agentprobe"
	"github.com/insajin/autopus-adk/pkg/processprobe"
)

type agentProbePlatform struct{ name, executable string }

var agentProbePlatforms = []agentProbePlatform{{"codex", "codex"}, {"claude-code", "claude"}, {"opencode", "opencode"}, {"omp", "omp"}, {"antigravity-cli", "agy"}, {"gemini-cli", "gemini"}}
var agentProbeVersion = regexp.MustCompile(`\bv?([0-9]+\.[0-9]+\.[0-9]+(?:[-+][a-zA-Z0-9.-]+)?)\b`)

func inspectAgentRuntime(ctx context.Context, platform agentProbePlatform) doctorAgentObservation {
	installed := false
	observation := doctorAgentObservation{Platform: platform.name, Installed: &installed, RuntimeVersion: "unknown", Status: "not_installed"}
	binary, err := exec.LookPath(platform.executable)
	if err != nil {
		return observation
	}
	installed = true
	observation.Status = "lifecycle_not_run"
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	output, err := processprobe.OutputLimited(exec.CommandContext(ctx, binary, "--version"), 8192)
	if err != nil {
		observation.Status = "version_probe_unavailable"
		return observation
	}
	if version := parseAgentRuntimeVersion(string(output)); version != "" {
		observation.RuntimeVersion = version
	}
	return observation
}

func readAgentLifecycleTrace(path string) (agentprobe.Evidence, error) {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return agentprobe.Evidence{}, fmt.Errorf("trace must be a regular file")
	}
	file, err := os.Open(path)
	if err != nil {
		return agentprobe.Evidence{}, fmt.Errorf("trace unavailable")
	}
	defer file.Close()
	opened, err := file.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return agentprobe.Evidence{}, fmt.Errorf("trace changed while opening")
	}
	return agentprobe.Decode(file)
}

func parseAgentRuntimeVersion(output string) string {
	match := agentProbeVersion.FindStringSubmatch(output)
	if len(match) < 2 {
		return ""
	}
	return match[1]
}
