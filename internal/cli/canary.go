package cli

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type canaryOptions struct {
	projectDir   string
	url          string
	frontendURL  string
	apiURL       string
	browserPaths []string
	watch        string
	compare      string
	dryRun       bool
	jsonOut      bool
	format       string
}

// No default endpoint or browser URL exists on purpose. A canary URL is
// deployment state, not repository state: hardcoding the Autopus staging hosts
// pointed every third-party project's release gate at Autopus infrastructure,
// and `auto qa init` wires `auto canary` into the canary-explicit lane for
// every project. With no URL supplied both checks report SKIPPED with the flag
// to set.
type canaryTargets struct {
	FrontendURL  string
	APIURL       string
	BrowserPaths []string
}

type canaryResult struct {
	Timestamp string               `json:"timestamp"`
	Project   string               `json:"project"`
	Mode      string               `json:"mode"`
	Verdict   string               `json:"verdict"`
	Build     string               `json:"build"`
	E2E       string               `json:"e2e"`
	Doctor    string               `json:"doctor"`
	Endpoint  string               `json:"endpoint"`
	Browser   string               `json:"browser"`
	Targets   []canaryTargetResult `json:"targets,omitempty"`
	Skipped   []canarySkippedCheck `json:"skipped,omitempty"`
	Flags     map[string]string    `json:"flags,omitempty"`
	Summary   map[string]string    `json:"summary"`
}

type canaryTargetResult struct {
	ID      string `json:"id"`
	Command string `json:"command,omitempty"`
	Status  string `json:"status"`
	Detail  string `json:"detail,omitempty"`
}

type canarySkippedCheck struct {
	Area   string `json:"area"`
	Reason string `json:"reason"`
}

func newCanaryCmd() *cobra.Command {
	opts := canaryOptions{}
	cmd := &cobra.Command{
		Use:   "canary",
		Short: "Run post-deploy health checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCanaryCmd(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.projectDir, "project-dir", ".", "Project root directory")
	cmd.Flags().StringVar(&opts.url, "url", "", "Deployment URL used for both endpoint and browser checks")
	cmd.Flags().StringVar(&opts.frontendURL, "frontend-url", "", "Frontend URL for browser checks; empty skips the check")
	cmd.Flags().StringVar(&opts.apiURL, "api-url", "", "API URL for endpoint checks; empty skips the check")
	cmd.Flags().StringArrayVar(&opts.browserPaths, "browser-path", nil, "Path to probe during the browser check; repeatable, defaults to /")
	cmd.Flags().StringVar(&opts.watch, "watch", "", "Repeat interval such as 5m (reserved)")
	cmd.Flags().StringVar(&opts.compare, "compare", "", "Commit SHA to compare against (reserved)")
	cmd.Flags().BoolVar(&opts.dryRun, "dry-run", false, "Plan canary checks without executing project commands")
	addJSONFlags(cmd, &opts.jsonOut, &opts.format)
	return cmd
}

func runCanaryCmd(cmd *cobra.Command, opts canaryOptions) error {
	jsonMode, err := resolveJSONMode(opts.jsonOut, opts.format)
	if err != nil {
		return err
	}
	result, err := runCanary(cmd.Context(), opts)
	if err != nil && result.Verdict != "FAIL" {
		result.Verdict = "FAIL"
		result.Summary = canarySummary(result)
	}
	recordCanaryHistory(cmd, opts, result)
	if jsonMode {
		status := jsonStatusOK
		if result.Verdict == "WARN" {
			status = jsonStatusWarn
		}
		if result.Verdict == "FAIL" {
			return writeJSONResultAndExit(cmd, jsonStatusError, errOrDefault(err, "canary failed"), "canary_failed", result, nil, canaryChecks(result))
		}
		return writeJSONResult(cmd, status, result, nil, canaryChecks(result))
	}
	printCanaryText(cmd, result)
	if result.Verdict == "FAIL" {
		return errOrDefault(err, "canary failed")
	}
	return nil
}

func runCanary(ctx context.Context, opts canaryOptions) (canaryResult, error) {
	projectDir, err := filepath.Abs(defaultString(opts.projectDir, "."))
	if err != nil {
		return canaryResult{}, err
	}
	targets := resolveCanaryTargets(opts)
	result := canaryResult{
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Project:   filepath.Base(projectDir),
		Mode:      "staging-canary",
		Verdict:   "PASS",
		Build:     "SKIPPED",
		E2E:       "SKIPPED",
		Doctor:    "SKIPPED",
		Endpoint:  "SKIPPED",
		Browser:   "SKIPPED",
		Flags: map[string]string{
			"url":          opts.url,
			"frontend_url": targets.FrontendURL,
			"api_url":      targets.APIURL,
			"watch":        opts.watch,
			"compare":      opts.compare,
		},
	}
	buildTargets, buildSkips := canaryBuildTargets(projectDir)
	result.Skipped = append(result.Skipped, buildSkips...)
	if opts.dryRun {
		result.Mode = "dry-run"
		result.Build = "SKIPPED"
		result.E2E = "SKIPPED"
		result.Doctor = "SKIPPED"
		// The planned build targets are the whole point of a dry run: they show
		// what detection resolved without paying for a build.
		for _, target := range buildTargets {
			result.Targets = append(result.Targets, canaryTargetResult{ID: target.ID, Command: target.Command, Status: "SKIPPED", Detail: "dry-run"})
		}
		result.Skipped = append(result.Skipped,
			canarySkippedCheck{"build", "dry-run"},
			canarySkippedCheck{"e2e", "dry-run"},
			canarySkippedCheck{"doctor", "dry-run"},
			canarySkippedCheck{"endpoint", "dry-run"},
			canarySkippedCheck{"browser", "dry-run"},
		)
		result.Summary = canarySummary(result)
		return result, writeCanaryLatest(projectDir, result)
	}

	if buildErr := runCanaryBuildPhase(ctx, projectDir, buildTargets, &result); buildErr != nil {
		result.Summary = canarySummary(result)
		_ = writeCanaryLatest(projectDir, result)
		return result, buildErr
	}
	if harnessErr := runCanaryHarnessPhase(ctx, projectDir, &result); harnessErr != nil {
		result.Summary = canarySummary(result)
		_ = writeCanaryLatest(projectDir, result)
		return result, harnessErr
	}

	if targets.APIURL == "" {
		result.Skipped = append(result.Skipped, canarySkippedCheck{"endpoint", "no API URL supplied (pass --api-url or --url)"})
	} else {
		result.Endpoint = runCanaryEndpointChecks(ctx, targets.APIURL, &result)
	}
	result.Browser = runCanaryRemoteBrowser(ctx, projectDir, targets, &result)
	if result.Endpoint == "FAIL" || result.Browser == "FAIL" {
		result.Verdict = "FAIL"
	}
	result.Summary = canarySummary(result)
	return result, writeCanaryLatest(projectDir, result)
}

// resolveCanaryTargets never substitutes a default host. An unset URL means the
// caller has not told canary where the deployment is, and probing a guessed
// host would report someone else's uptime as this project's verdict.
func resolveCanaryTargets(opts canaryOptions) canaryTargets {
	frontendURL := strings.TrimSpace(opts.frontendURL)
	apiURL := strings.TrimSpace(opts.apiURL)
	if sharedURL := strings.TrimSpace(opts.url); sharedURL != "" {
		frontendURL = sharedURL
		apiURL = sharedURL
	}
	paths := make([]string, 0, len(opts.browserPaths))
	for _, path := range opts.browserPaths {
		if trimmed := strings.TrimSpace(path); trimmed != "" {
			paths = append(paths, "/"+strings.TrimLeft(trimmed, "/"))
		}
	}
	if len(paths) == 0 {
		paths = []string{"/"}
	}
	return canaryTargets{
		FrontendURL:  strings.TrimRight(frontendURL, "/"),
		APIURL:       strings.TrimRight(apiURL, "/"),
		BrowserPaths: paths,
	}
}
