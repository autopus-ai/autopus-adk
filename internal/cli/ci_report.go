package cli

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/cireport"
)

// ciAPI is what the report needs from its API: reads and a request count.
type ciAPI interface {
	cireport.API
	Requests() int
}

// Seams of `auto ci report`: the clock, the API constructor, and the git
// lookups of the default repo and the workflow directory.
var (
	ciNow    = time.Now
	ciNewAPI = func(ctx context.Context, cacheDir string, budget int) (ciAPI, error) {
		gh, err := cireport.NewGH(cacheDir, budget)
		if err != nil {
			return nil, err
		}
		if err := gh.CheckAuth(ctx); err != nil {
			return nil, err
		}
		return gh, nil
	}
	ciGit = func(dir string, args ...string) (string, error) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
)

type ciReportOptions struct {
	repo, since, workflow, workflowsDir string
	limitRuns, maxRequests              int
	jsonOut                             bool
	format                              string
}

const ciReportLong = "Measures a GitHub Actions repository from the jobs API timestamps (filter=all, so reruns count) " +
	"through your own `gh` login, and lints the workflow files of the local checkout. It reports per-job median and " +
	"p90 run and queue time, end-to-end run time per event, busy minutes per runner, minutes lost to failed, " +
	"cancelled, and rerun attempts, push runs that re-test a tree a PR run already passed, and PR runs that kept " +
	"running after a newer push. Findings are ranked by severity, then by minutes per day.\n\n" +
	"It only reads. Requests are bounded by --max-requests, at most 4 run at a time, and responses are cached in a " +
	"temporary directory for the run. With more runs than --limit-runs, an evenly spaced sample is measured and " +
	"totals are scaled by the reported factor.\n\n" +
	"Exit status: 0 with a report, 1 a usage, gh, or API failure."

func newCICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "ci",
		Short:         "Measure GitHub Actions CI before optimising it",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newCIReportCmd())
	return cmd
}

func newCIReportCmd() *cobra.Command {
	var opts ciReportOptions
	cmd := &cobra.Command{
		Use:           "report [--repo owner/name] [--since 14d] [--limit-runs N] [--workflow name] [--json]",
		Short:         "Rank CI bottlenecks from jobs API timings and a workflow lint",
		Long:          ciReportLong,
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, _ []string) error { return runCIReport(cmd, opts) },
	}
	f := cmd.Flags()
	f.StringVar(&opts.repo, "repo", "", "owner/name (default: the origin remote of the current checkout)")
	f.StringVar(&opts.since, "since", "14d", "window start: Nh, Nd, Nw, or YYYY-MM-DD")
	f.IntVar(&opts.limitRuns, "limit-runs", 200, "measure at most this many runs, sampled evenly; 0 for all")
	f.StringVar(&opts.workflow, "workflow", "", "only runs of this workflow name or file (ci.yaml or ci)")
	f.IntVar(&opts.maxRequests, "max-requests", 1000, "API request budget for the run")
	f.StringVar(&opts.workflowsDir, "workflows-dir", "", "workflow files to lint (default: .github/workflows of the checkout)")
	addJSONFlags(cmd, &opts.jsonOut, &opts.format)
	return cmd
}

func runCIReport(cmd *cobra.Command, opts ciReportOptions) error {
	jsonMode, err := resolveJSONMode(opts.jsonOut, opts.format)
	if err != nil {
		return err
	}
	fail := func(code, line string) error { return diskFail(cmd, jsonMode, 1, code, "ci report: "+line, nil) }
	now := ciNow().UTC()
	since, err := cireport.ParseSince(opts.since, now)
	if err != nil {
		return fail("ci_report_usage", err.Error())
	}
	if opts.limitRuns < 0 || opts.maxRequests < 1 {
		return fail("ci_report_usage", "--limit-runs must be 0 or more and --max-requests at least 1")
	}
	cwd, _ := os.Getwd()
	top, gitErr := ciGit(cwd, "rev-parse", "--show-toplevel")
	if gitErr != nil {
		top = cwd
	}
	origin := ""
	if remote, err := ciGit(top, "remote", "get-url", "origin"); err == nil {
		origin, _ = cireport.ParseGitHubRemote(remote)
	}
	repo := opts.repo
	if repo == "" {
		if repo = origin; repo == "" {
			return fail("ci_report_usage", "no github.com origin remote here; pass --repo owner/name")
		}
	}
	if !cireport.ValidRepo(repo) {
		return fail("ci_report_usage", "--repo must be owner/name, got "+terminalSafe(repo))
	}
	cache, err := os.MkdirTemp("", "autopus-ci-report-*")
	if err != nil {
		return fail("ci_report_failed", err.Error())
	}
	defer func() { _ = os.RemoveAll(cache) }()
	api, err := ciNewAPI(cmd.Context(), cache, opts.maxRequests)
	if err != nil {
		return fail("ci_report_gh", strings.TrimPrefix(err.Error(), "ci report: "))
	}
	ds, err := cireport.Collect(cmd.Context(), api, cireport.CollectOptions{Repo: repo, Since: since, Until: now,
		LimitRuns: opts.limitRuns, Workflow: opts.workflow})
	if err != nil {
		return fail("ci_report_api", err.Error())
	}
	ds.Requests = api.Requests()
	report := cireport.Analyze(ds)
	report.WithLint(lintCheckout(&report, opts.workflowsDir, top, origin, repo, ds.Public))
	if jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, report, nil, nil)
	}
	writeCIReportText(cmd.OutOrStdout(), report)
	return nil
}

// lintCheckout lints the local workflow files, noting when they are missing
// or belong to another repository than the one measured.
func lintCheckout(r *cireport.Report, dir, top, origin, repo string, public *bool) []cireport.LintFinding {
	if dir == "" {
		dir = filepath.Join(top, ".github", "workflows")
	}
	files, err := cireport.LoadWorkflows(dir)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && len(files) == 0) {
		r.Notes = append(r.Notes, "no workflow files in "+dir+"; the workflow lint was skipped")
		return nil
	}
	if err != nil {
		r.Notes = append(r.Notes, "workflow lint skipped: "+err.Error())
		return nil
	}
	if origin != "" && !strings.EqualFold(origin, repo) {
		r.Notes = append(r.Notes, "the workflow lint read the local checkout of "+origin+", not "+repo)
	}
	if public == nil {
		r.Notes = append(r.Notes, "repository visibility unknown; the fork PR exposure rule was skipped")
	}
	findings, problems := cireport.LintWorkflows(files, public)
	for _, p := range problems {
		r.Notes = append(r.Notes, "workflow not parsed: "+p)
	}
	return findings
}
