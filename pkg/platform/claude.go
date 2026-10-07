// Package platform provides detection of the current CLI runtime and CC21 feature availability.
//
// The primary entry points are DetectRuntime and DetectFeatures.
// Non-Claude-Code runtimes always return all-false Features (R8 no-op guarantee).
package platform

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/processprobe"
)

// Runtime identifies the CLI host executing the harness.
type Runtime string

const (
	// RuntimeClaudeCode is the Anthropic Claude Code CLI.
	RuntimeClaudeCode Runtime = "claude-code"
	// RuntimeCodex is the OpenAI Codex CLI.
	RuntimeCodex Runtime = "codex"
	// RuntimeAntigravityCLI is the Google Antigravity CLI.
	RuntimeAntigravityCLI Runtime = "antigravity-cli"
	// RuntimeGeminiCLI is kept as a legacy alias for older integrations.
	RuntimeGeminiCLI Runtime = RuntimeAntigravityCLI
	// RuntimeOpenCode is the OpenCode CLI.
	RuntimeOpenCode Runtime = "opencode"
	// RuntimeUnknown means no supported runtime could be detected.
	RuntimeUnknown Runtime = "unknown"
)

// MinClaudeVersion is the lowest Claude Code version that supports CC21 primitives.
const MinClaudeVersion = "2.1.0"

// Features represents which CC21 primitives are available in the current runtime.
// All fields are false when the runtime is not Claude Code (R8).
type Features struct {
	// Effort indicates the `effort` frontmatter field is honoured by the runtime.
	Effort bool
	// InitialPrompt indicates the main-session initialPrompt field is supported.
	InitialPrompt bool
	// Monitor indicates the Monitor tool event-stream is available.
	Monitor bool
	// TaskCreated indicates the TaskCreated hook is supported.
	TaskCreated bool
	// ClaudeVersion is the raw version string returned by `claude --version`, e.g. "2.1.113".
	ClaudeVersion string
	// HasOpus47 is a best-effort probe: true when CLAUDE_MODEL env contains "opus-4-7".
	HasOpus47 bool
}

// execCommandFunc is the function used to create exec.Cmd instances.
// Overriding this variable in tests allows callers to inject a fake `claude` binary.
var execCommandFunc = func(ctx context.Context, name string, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, name, args...)
}

// claudeVersionFn resolves the Claude Code version string. In-package tests can
// swap it to avoid the fork/exec overhead of `claude --version`, which becomes a
// flake source under CPU-contended test runs (e.g. `go test -race ./...`).
var claudeVersionFn = claudeVersion

// validRuntimes maps the allowed AUTOPUS_PLATFORM values to Runtime constants.
var validRuntimes = map[string]Runtime{
	"claude-code":     RuntimeClaudeCode,
	"codex":           RuntimeCodex,
	"antigravity-cli": RuntimeAntigravityCLI,
	"gemini-cli":      RuntimeAntigravityCLI,
	"opencode":        RuntimeOpenCode,
}

// DetectRuntime returns the detected CLI runtime.
//
// Priority order:
//  1. AUTOPUS_PLATFORM env var (only recognised values; unknown values → RuntimeUnknown).
//  2. Heuristic: if `claude` binary is reachable via PATH → RuntimeClaudeCode.
//  3. Fallback: RuntimeUnknown.
func DetectRuntime() Runtime {
	if env := os.Getenv("AUTOPUS_PLATFORM"); env != "" {
		if r, ok := validRuntimes[env]; ok {
			return r
		}
		return RuntimeUnknown
	}

	if _, err := exec.LookPath("claude"); err == nil {
		return RuntimeClaudeCode
	}

	return RuntimeUnknown
}

// DetectFeatures returns feature availability for the current runtime.
// When the runtime is not RuntimeClaudeCode, all fields are false (R8 no-op).
func DetectFeatures() Features {
	if DetectRuntime() != RuntimeClaudeCode {
		return Features{}
	}

	ver, err := claudeVersionFn()
	if err != nil || !VersionSupportsCC21(ver) {
		return Features{ClaudeVersion: ver}
	}

	cc21 := loadCC21Features()
	if !cc21.Enabled {
		return Features{ClaudeVersion: ver, HasOpus47: probeOpus47()}
	}

	return Features{
		Effort:        cc21.EffortEnabled,
		InitialPrompt: cc21.InitialPromptEnabled,
		Monitor:       cc21.MonitorEnabled,
		TaskCreated:   cc21.TaskCreatedEnabled,
		ClaudeVersion: ver,
		HasOpus47:     probeOpus47(),
	}
}

func loadCC21Features() config.CC21FeaturesConf {
	wd, err := os.Getwd()
	if err != nil {
		return config.CC21FeaturesConf{}
	}
	cfgDir, ok := findNearestConfigDir(wd)
	if !ok {
		return config.CC21FeaturesConf{}
	}
	cfg, err := config.Load(cfgDir)
	if err != nil {
		return config.CC21FeaturesConf{}
	}
	return cfg.Features.CC21
}

func findNearestConfigDir(start string) (string, bool) {
	dir := filepath.Clean(start)
	for {
		cfgPath := filepath.Join(dir, "autopus.yaml")
		if _, err := os.Stat(cfgPath); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

// VersionSupportsCC21 returns true when the given semver-ish version string is >= 2.1.0.
// Malformed input returns false (conservative).
func VersionSupportsCC21(version string) bool {
	// Strip optional trailing text like " (Claude Code)" before parsing.
	version = strings.TrimSpace(version)
	if idx := strings.IndexByte(version, ' '); idx != -1 {
		version = version[:idx]
	}

	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return false
	}

	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return false
	}
	minor, err := strconv.Atoi(parts[1])
	if err != nil {
		return false
	}

	if major > 2 {
		return true
	}
	if major == 2 && minor >= 1 {
		return true
	}
	return false
}

// claudeVersion runs `claude --version` with a 2-second timeout and returns the
// first token of the output (the bare version number, e.g. "2.1.113").
func claudeVersion() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	out, err := processprobe.Output(execCommandFunc(ctx, "claude", "--version"))
	if err != nil {
		return "", err
	}

	raw := strings.TrimSpace(string(out))
	// The output may be "2.1.113 (Claude Code)" — extract the first token.
	if idx := strings.IndexByte(raw, ' '); idx != -1 {
		return raw[:idx], nil
	}
	return raw, nil
}

// probeOpus47 returns true when the CLAUDE_MODEL environment variable indicates
// Opus 4.7 availability.  This is a best-effort heuristic; callers may override
// the returned Features.HasOpus47 field when they have more reliable information.
func probeOpus47() bool {
	model := strings.ToLower(os.Getenv("CLAUDE_MODEL"))
	return strings.Contains(model, "opus-4-7")
}
