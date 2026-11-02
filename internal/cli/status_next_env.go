package cli

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
)

// statusProbe is what status reads from the machine. Status only looks
// things up; it never runs a CLI or helper it finds (REQ-SEV-SCOPE-01).
type statusProbe struct {
	lookPath      func(string) (string, error)
	desktopHelper func() (string, error)
	homeDir       func() (string, error)
}

func defaultStatusProbe() statusProbe {
	return statusProbe{
		lookPath:      exec.LookPath,
		desktopHelper: func() (string, error) { return resolveRuntimeHelper() },
		homeDir:       os.UserHomeDir,
	}
}

// statusAICLIs are the AI CLIs status reports, in the order it lists them.
var statusAICLIs = []string{"claude", "codex", "agy", "opencode", "omp"}

// Desktop states. Desktop is optional: the ADK works standalone.
const (
	desktopConnected    = "connected"
	desktopInstalled    = "installed"
	desktopNotInstalled = "not_installed"
)

// statusEnvironment is the install state next commands are chosen against
// (REQ-SEV-NEXT-02, REQ-SEV-NEXT-03).
type statusEnvironment struct {
	Dir                 string                 `json:"dir"`
	InstalledCLIs       []string               `json:"installed_clis"`
	ConfigFound         bool                   `json:"config_found"`
	ConfigError         string                 `json:"config_error,omitempty"`
	ConfiguredPlatforms []string               `json:"configured_platforms"`
	ReviewGate          bool                   `json:"review_gate"`
	ReviewProviders     []string               `json:"review_providers"`
	PipelinePlatform    statusPipelinePlatform `json:"pipeline_platform"`
	Desktop             statusDesktop          `json:"desktop"`
}

// statusPipelinePlatform is the platform pipeline commands below use. It is
// the platform `auto pipeline run` selects without --platform when one of
// claude, codex, agy is installed; omp runs only when named; with neither,
// no pipeline command is offered.
type statusPipelinePlatform struct {
	Platform       string `json:"platform"`
	Reason         string `json:"reason"`
	AutoSelectable bool   `json:"auto_selectable"`
	Configured     bool   `json:"configured"`
	Note           string `json:"note,omitempty"`
}

type statusDesktop struct {
	State           string `json:"state"`
	HelperInstalled bool   `json:"helper_installed"`
	WorkerConfig    bool   `json:"worker_config"`
	Note            string `json:"note"`
}

// pipelinePlatformConfigNames maps pipeline platforms to the autopus.yaml
// platform that installs the same CLI.
var pipelinePlatformConfigNames = map[string]string{
	"claude": "claude-code", "codex": "codex", "gemini": "antigravity-cli", "omp": "omp",
}

const (
	pipelinePlatformReasonOMPOnly = "claude, codex, and agy are not installed; omp is, and runs only with --platform omp"
	pipelinePlatformReasonNone    = "no pipeline platform is installed (claude, codex, agy, or omp)"
)

func collectStatusEnvironment(dir string, probe statusProbe) statusEnvironment {
	env := statusEnvironment{
		Dir: statusAbsDir(dir), InstalledCLIs: []string{}, ConfiguredPlatforms: []string{}, ReviewProviders: []string{},
	}
	for _, name := range statusAICLIs {
		if _, err := probe.lookPath(name); err == nil {
			env.InstalledCLIs = append(env.InstalledCLIs, name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "autopus.yaml")); err == nil {
		env.ConfigFound = true
		// LoadPreview: reading the platforms must not rewrite autopus.yaml.
		if cfg, err := config.LoadPreview(dir); err == nil {
			env.ConfiguredPlatforms = append(env.ConfiguredPlatforms, cfg.Platforms...)
			env.ReviewGate = cfg.Spec.ReviewGate.Enabled
			env.ReviewProviders = append(env.ReviewProviders, cfg.Spec.ReviewGate.Providers...)
		} else {
			env.ConfigError = statusSingleLine(err.Error())
		}
	}
	env.PipelinePlatform = selectStatusPipelinePlatform(env, probe)
	env.Desktop = collectDesktopState(probe)
	return env
}

func selectStatusPipelinePlatform(env statusEnvironment, probe statusProbe) statusPipelinePlatform {
	platform, reason := resolvePlatformWith("", probe.lookPath)
	selected := statusPipelinePlatform{Platform: platform, Reason: reason, AutoSelectable: true}
	if reason == pipelinePlatformReasonFallback {
		selected = statusPipelinePlatform{Reason: pipelinePlatformReasonNone}
		if _, err := probe.lookPath("omp"); err == nil {
			selected = statusPipelinePlatform{Platform: "omp", Reason: pipelinePlatformReasonOMPOnly}
		}
	}
	if selected.Platform == "" || !env.ConfigFound || env.ConfigError != "" {
		return selected
	}
	if containsPlatform(env.ConfiguredPlatforms, pipelinePlatformConfigNames[selected.Platform]) {
		selected.Configured = true
		return selected
	}
	for _, candidate := range pipelinePlatformOrder {
		if !containsPlatform(env.ConfiguredPlatforms, pipelinePlatformConfigNames[candidate.provider]) {
			continue
		}
		if _, err := probe.lookPath(candidate.binary); err == nil {
			selected.Note = fmt.Sprintf("%s is not a configured platform here; pass --platform %s to use %s",
				selected.Platform, candidate.provider, pipelinePlatformLabel(candidate.provider))
			return selected
		}
	}
	selected.Note = fmt.Sprintf("%s is not a configured platform here, and no configured platform's CLI is installed",
		selected.Platform)
	return selected
}

func collectDesktopState(probe statusProbe) statusDesktop {
	desktop := statusDesktop{}
	if _, err := probe.desktopHelper(); err == nil {
		desktop.HelperInstalled = true
	}
	if home, err := probe.homeDir(); err == nil {
		if _, err := os.Stat(filepath.Join(home, ".config", "autopus", "worker.yaml")); err == nil {
			desktop.WorkerConfig = true
		}
	}
	switch {
	case desktop.WorkerConfig:
		desktop.State, desktop.Note = desktopConnected, "worker config found"
	case desktop.HelperInstalled:
		desktop.State, desktop.Note = desktopInstalled, "optional; the ADK works standalone"
	default:
		desktop.State, desktop.Note = desktopNotInstalled, "optional; the ADK works standalone"
	}
	return desktop
}

func renderStatusEnvironment(out io.Writer, env statusEnvironment) {
	tui.SectionHeader(out, "Environment")
	_, _ = fmt.Fprintf(out, "  Directory: %s\n", statusSingleLine(env.Dir))
	_, _ = fmt.Fprintf(out, "  AI CLIs on PATH: %s\n", statusListOrNone(env.InstalledCLIs))
	configured := statusListOrNone(env.ConfiguredPlatforms)
	switch {
	case !env.ConfigFound:
		configured = "none (no autopus.yaml)"
	case env.ConfigError != "":
		configured = "unknown (" + env.ConfigError + ")"
	}
	_, _ = fmt.Fprintf(out, "  Configured platforms: %s\n", configured)
	platform := env.PipelinePlatform
	name := "none"
	if platform.Platform != "" {
		name = pipelinePlatformLabel(platform.Platform)
	}
	_, _ = fmt.Fprintf(out, "  auto pipeline run platform: %s — %s\n", name, platform.Reason)
	if platform.Note != "" {
		_, _ = fmt.Fprintf(out, "    note: %s\n", platform.Note)
	}
	_, _ = fmt.Fprintf(out, "  Desktop: %s (%s)\n", desktopStateText(env.Desktop.State), env.Desktop.Note)
}

func desktopStateText(state string) string {
	switch state {
	case desktopInstalled:
		return "installed, not connected"
	case desktopNotInstalled:
		return "not installed"
	default:
		return state
	}
}

func statusListOrNone(values []string) string {
	if len(values) == 0 {
		return "none"
	}
	return strings.Join(values, ", ")
}
