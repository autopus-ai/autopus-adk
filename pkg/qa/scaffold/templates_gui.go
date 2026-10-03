package scaffold

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// defaultBrowserGUIOrigin is the Vite dev-server origin, used when the project's
// Playwright config states no usable baseURL. It is a guess, which is why the
// example pack keeps its review-before-executing header.
const defaultBrowserGUIOrigin = "http://127.0.0.1:5173"

const maxPlaywrightConfigBytes = 64 * 1024

// playwrightBaseURLRe is deliberately a bounded regex over a bounded read rather
// than a config parser: resolving a project's real baseURL would mean executing
// its TypeScript, and a wrong guess must degrade to the default rather than to an
// origin the pack would then fail validation on.
var playwrightBaseURLRe = regexp.MustCompile(`baseURL\s*:\s*['"]([^'"\n\r]{1,200})['"]`)

var playwrightConfigNames = []string{
	"playwright.config.ts",
	"playwright.config.js",
	"playwright.config.mjs",
	"playwright.config.cjs",
}

// guiExplorePack parameterizes the one gui-explore template the desktop and
// browser packs share. A single template keeps the two from drifting: capture
// policy, network policy, retention, and the safety-oracle artifact are the same
// contract on both surfaces, and only the target and owned paths differ.
type guiExplorePack struct {
	ID         string
	Title      string
	Surface    string
	Origin     string
	OwnedPaths []string
	Argv       []string
}

// browserGUIExplorePackExample renders the gui-explore pack a browser project
// copies into .autopus/qa/journeys/browser-gui-explore.yaml. The capture README
// carries it instead of auto qa init emitting it as a starter: init cannot know
// which specs are read-only, and a pack aimed at the whole suite is blocked on the
// first click. Some pack is still required for the capture producer assets to do
// anything, because gui.capture is only valid on the gui-explore adapter and that
// adapter is the one installing the origin and forbidden-action guard.
func browserGUIExplorePackExample(signals projectSignals) string {
	origin := signals.BaseOrigin
	if origin == "" {
		origin = defaultBrowserGUIOrigin
	}
	return renderGUIExplorePack(guiExplorePack{
		ID:         BrowserGUIJourneyID,
		Title:      "Browser GUI exploration",
		Surface:    "frontend",
		Origin:     origin,
		OwnedPaths: []string{"src/**", "e2e/**"},
		Argv:       exploreGrepArgv(signals.PackageManager),
	})
}

// desktopGUIExplorePackExample is the same pack for a desktop shell, whose webview
// is served by the framework dev server rather than by the project's Playwright
// baseURL, so its origin is fixed rather than detected.
func desktopGUIExplorePackExample(signals projectSignals) string {
	return renderGUIExplorePack(guiExplorePack{
		ID:         DesktopGUIJourneyID,
		Title:      "Desktop GUI exploration",
		Surface:    "desktop",
		Origin:     "http://127.0.0.1:1420",
		OwnedPaths: []string{"src/**", "src-tauri/**"},
		Argv:       exploreGrepArgv(signals.PackageManager),
	})
}

// exploreTag marks the read-only subset this pack is allowed to run.
const exploreTag = "@explore"

// exploreGrepArgv builds the runner argv that selects only the tagged subset.
//
// The `--` separator is npm-only, and the asymmetry is load-bearing. npm consumes
// the separator and forwards `--grep @explore` to Playwright; without it npm eats
// `--grep` as an unknown CLI config and Playwright never sees the filter. `pnpm
// exec` and Yarn 2+ do the opposite - they pass the literal `--` through, and
// Playwright then reads `--grep` and `@explore` as positional file filters and runs
// the whole suite, which is the mutation-blocked outcome this pack exists to avoid.
func exploreGrepArgv(packageManager string) []string {
	command := nodeCommand(packageManager)
	args := []string{"test"}
	if command == "npm" {
		args = append(args, "--")
	}
	return jsRunnerArgv(command, "playwright", append(args, "--grep", exploreTag)...)
}

func renderGUIExplorePack(pack guiExplorePack) string {
	return fmt.Sprintf(`# Example gui-explore Journey Pack, carried by .autopus/qa/capture/README.md.
# Copy it into .autopus/qa/journeys/ and review before executing.
# ADK is a harness; this project-local Journey Pack owns product-specific GUI policy.
# QAMESH owns evidence/release policy. Playwright is only the runner adapter for this GUI journey.
# Review command, allowed_origins, forbidden_actions, env, oracle, and artifact policy before --run.
# The command targets the read-only @explore-tagged subset because mutation is a
# forbidden action: a spec that clicks, fills, or submits must NOT carry that tag,
# or the GUI runtime blocks it and the journey is reported blocked.
# This pack declares no artifacts. Policy enforcement is witnessed by the harness-owned
# guard receipt, which the producer cannot write, and per-step console, network,
# screenshot, and trace evidence comes from gui.capture.
# Raw screenshots, traces, and videos stay local-only under .autopus/qa/runs/** and are
# never published; only the sanitized capture_index projection becomes evidence.
id: %[1]s
title: %[2]s
surface: %[3]s
lanes: [gui-explore]
adapter:
  id: gui-explore
command:
  argv: [%[4]s]
  cwd: .
  timeout: 120s
checks:
  - id: %[1]s
    type: gui_exploration
gui:
  allowed_origins:
    - %[5]s
  # Only "mutation" and the exact Playwright method names (click, dblclick, tap,
  # fill, press, check, uncheck, selectOption, setInputFiles, dragTo) are enforced
  # at runtime. The guard sees method names, never business intent, so a label like
  # "payment" would read as a guarantee nothing provides; labels outside this set
  # are reported as unenforceable_forbidden_actions in the check evidence.
  forbidden_actions:
    - mutation
  selector_strategy: role-first
  network_policy:
    # summary-only observes without stopping anything. local-only aborts every
    # request whose origin is outside allowed_origins. blocked does that and also
    # aborts xhr/fetch to an allowed origin, so the UI renders from static assets
    # with no data traffic. Aborted requests are reported as network_stopped in
    # the gui-policy-runtime check, and do not fail the journey.
    mode: local-only
  artifact_retention:
    publish_raw: false
  capture:
    # mode always demands every declared stream on every step, which makes a green
    # exploration run expensive and turns an empty selection into a hard contract
    # violation. on-failure keeps a passing run cheap and still guarantees the
    # filmstrip exactly when something broke.
    mode: on-failure
    streams: [screenshot, console, network, trace]
    screenshot: per-step
    console_severity: warning
    retain_local: true
    # optional, not required: a project whose subset produces no replay reference
    # must not fail the capture contract for it.
    replay_script: optional
source_refs:
  source_spec: %[6]s
  acceptance_refs:
    - AC-QAMESH3-004
    - AC-QAMESH3-006
  owned_paths:
%[7]s
  do_not_modify_paths:
    - .codex/**
    - .opencode/**
    - .autopus/plugins/**
`, pack.ID, pack.Title, pack.Surface, renderInlineStrings(pack.Argv), pack.Origin,
		sourceSpecForLanes([]string{"gui-explore"}), renderBlockList("    ", pack.OwnedPaths))
}

func renderBlockList(indent string, values []string) string {
	lines := make([]string, 0, len(values))
	for _, value := range values {
		lines = append(lines, indent+"- "+value)
	}
	return strings.Join(lines, "\n")
}

// DetectPlaywrightOrigin is detectBaseOrigin for callers outside init, such as
// scenario generation offering a Playwright pack's origin to the agent.
func DetectPlaywrightOrigin(projectDir string) string {
	return detectBaseOrigin(projectDir)
}

// detectBaseOrigin returns the origin of a Playwright baseURL when the config
// states one as a plain string literal. Anything else - a template expression, a
// relative URL, an origin carrying a path, query, fragment, or credentials -
// returns "" so the caller falls back to the default: journey validation rejects
// a non-origin allowed_origins entry, and a starter that fails validation is
// worse than a reviewable guess.
func detectBaseOrigin(projectDir string) string {
	for _, name := range playwrightConfigNames {
		body, err := readBounded(filepath.Join(projectDir, name), maxPlaywrightConfigBytes)
		if err != nil {
			continue
		}
		match := playwrightBaseURLRe.FindSubmatch(body)
		if match == nil {
			continue
		}
		if origin := normalizeHTTPOrigin(string(match[1])); origin != "" {
			return origin
		}
	}
	return ""
}

func readBounded(path string, limit int64) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(io.LimitReader(file, limit))
}

func normalizeHTTPOrigin(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	// A bare trailing slash is the one form worth normalizing; a real path,
	// query, or fragment means the config states a page, not an origin.
	if parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Path != "" && parsed.Path != "/") {
		return ""
	}
	return parsed.Scheme + "://" + parsed.Host
}
