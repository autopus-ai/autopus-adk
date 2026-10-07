package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// auto react band (SPEC-SIGMABAND-001 REQ-14, REQ-07 wiring). One run is the
// network step without the store lock, phase A under it (Lock, OpenWAL,
// merge, ReadSeries, Plan, Commit, Unlock), then phases B and C one claim at
// a time. --dry-run only loads the log and plans: no write, no lock, no
// provider. Tier 2 and tier 3 both diagnose; nothing changes git or GitHub.

type reactBandOptions struct {
	projectDir string
	noFetch    bool
	noAgent    bool
	dryRun     bool
	series     []string
	limit      int
	jsonOut    bool
	format     string
}

// reactBandDeps are the seams of a band run: the git and gh runner, the
// clock of phases A and C, the store lock wait, and a hook that adjusts the
// diagnoser before phase B (nil keeps it).
type reactBandDeps struct {
	runner   bandRunner
	clock    func() time.Time
	lockWait time.Duration
	prepare  func(*bandDiagnoser)
}

func newReactBandCmd() *cobra.Command {
	return newReactBandCmdWith(reactBandDeps{runner: execBandRunner{}, clock: time.Now, lockWait: healthband.StoreLockWait})
}

func newReactBandCmdWith(deps reactBandDeps) *cobra.Command {
	opts := reactBandOptions{}
	cmd := &cobra.Command{
		Use:   "band",
		Short: reactBandShort,
		Long:  reactBandLong,
		Args:  cobra.NoArgs,
		// A store failure is not a usage error, and usage text after the
		// JSON envelope would break it.
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReactBand(cmd, opts, deps)
		},
	}
	flags := cmd.Flags()
	flags.StringVar(&opts.projectDir, "project-dir", ".", "Project root directory")
	flags.BoolVar(&opts.noFetch, "no-fetch", false, "Skip the gh fetch and evaluate the stored series")
	flags.BoolVar(&opts.noAgent, "no-agent", false, "Run no provider; BS files carry evidence only")
	flags.BoolVar(&opts.dryRun, "dry-run", false, "Report planned actions without writing, locking, or calling a provider")
	flags.StringArrayVar(&opts.series, "series", nil, "Evaluate only this series ID, such as ci.failure_rate:CI (repeatable)")
	flags.IntVar(&opts.limit, "limit", bandDefaultLimit, "CI runs to fetch, 1 to 1000")
	addJSONFlags(cmd, &opts.jsonOut, &opts.format)
	return cmd
}

// runReactBand rejects an invalid invocation or config before any git, gh,
// or store access (the only expected non-zero exits, REQ-14), runs band, and
// writes the report. A store I/O failure is returned after the report.
func runReactBand(cmd *cobra.Command, opts reactBandOptions, deps reactBandDeps) error {
	jsonMode, err := resolveJSONMode(opts.jsonOut, opts.format)
	if err != nil {
		return err
	}
	if err := validateBandLimit(opts.limit); err != nil {
		return err
	}
	projectDir, err := bandProjectDir(opts.projectDir)
	if err != nil {
		return err
	}
	// LoadPreview decodes strictly and never rewrites autopus.yaml.
	harness, err := config.LoadPreview(projectDir)
	if err != nil {
		return err
	}
	run := bandRun{opts: opts, deps: deps, projectDir: projectDir, harness: harness, client: newBandGHClient(deps.runner), stderr: cmd.ErrOrStderr()}
	report, runErr := run.execute(cmd.Context())
	if jsonMode {
		if runErr != nil {
			return writeJSONResultAndExit(cmd, jsonStatusError, runErr, "band_failed", report, nil, report.checks())
		}
		return writeJSONResult(cmd, report.status(), report, nil, report.checks())
	}
	printBandText(cmd.OutOrStdout(), report)
	return runErr
}

// bandProjectDir resolves --project-dir, which must name an existing
// directory: the store lock would otherwise create the path.
func bandProjectDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	if info, err := os.Stat(abs); err != nil || !info.IsDir() {
		return "", fmt.Errorf("--project-dir %s is not a directory", dir)
	}
	return abs, nil
}

// bandRun is one band invocation.
type bandRun struct {
	opts       reactBandOptions
	deps       reactBandDeps
	projectDir string
	harness    *config.HarnessConfig
	client     bandGHClient
	stderr     io.Writer // warnings that are no part of the report
}

// bandPlanned is what phase A (or the --dry-run plan) decided.
type bandPlanned struct {
	plan        healthband.Plan
	state       healthband.Checkpoint // after the plan's events in phase A
	counts      healthband.ReadCounts
	fresh       int // CI observations appended (planned under --dry-run)
	skipped     int
	interrupted int
}

func (r bandRun) execute(ctx context.Context) (bandReport, error) {
	report := bandReport{DryRun: r.opts.dryRun, Constants: healthband.DefaultConstants(), Series: []bandSeriesResult{}}
	var fetch bandCIFetch
	if !r.opts.noFetch {
		// Network first, outside the lock (Durability item 1); an unavailable
		// source is a reason code, never an error.
		var err error
		if fetch, err = r.client.fetchCI(ctx, r.projectDir, r.opts.limit); err != nil {
			return report, err
		}
	}
	report.setCI(!r.opts.noFetch, fetch)
	store := healthband.NewStore(r.projectDir)
	if r.opts.dryRun {
		planned, err := r.planDry(store, fetch)
		if err == nil {
			report.addPlan(planned, fetch)
			report.addClaims(planned.plan.Claims, nil, nil)
		}
		return report, err
	}
	locked, err := store.Lock(ctx, r.deps.lockWait)
	if errors.Is(err, healthband.ErrStoreLocked) {
		report.Reasons = append(report.Reasons, healthband.ReasonStoreLocked)
		return report, nil
	}
	if err != nil {
		return report, err
	}
	planned, err := r.phaseA(locked, fetch)
	if err = errors.Join(err, locked.Unlock()); err != nil {
		return report, err
	}
	report.addPlan(planned, fetch)
	outcomes, recorded, err := r.phaseB(ctx, store, planned.plan.Claims, fetch)
	report.addClaims(planned.plan.Claims, outcomes, recorded)
	return report, err
}

// phaseA runs under the store lock: replay and settle the log, merge the
// fetched runs, plan every pending position, and commit the plan.
func (r bandRun) phaseA(locked *healthband.Locked, fetch bandCIFetch) (bandPlanned, error) {
	wal, err := locked.OpenWAL(r.deps.clock())
	if err != nil {
		return bandPlanned{}, err
	}
	fresh, err := mergeBandCI(locked, fetch)
	if err != nil {
		return bandPlanned{}, err
	}
	series, counts, err := locked.Store().ReadSeries()
	if err != nil {
		return bandPlanned{}, err
	}
	plan, err := wal.Plan(series, healthband.PlanOptions{Owner: healthband.NewOwner(), Only: r.opts.series, Fresh: fresh})
	if err == nil {
		err = wal.Commit(plan)
	}
	return bandPlanned{
		plan: plan, state: wal.Checkpoint(), counts: counts, fresh: len(fresh),
		skipped: wal.Skipped, interrupted: len(wal.Interrupted),
	}, err
}

// planDry is phase A without the lock and without a write: the log loads
// read-only and the fetched runs merge in memory by the same rule.
func (r bandRun) planDry(store *healthband.Store, fetch bandCIFetch) (bandPlanned, error) {
	wal, err := store.LoadWAL(r.deps.clock())
	if err != nil {
		return bandPlanned{}, err
	}
	series, counts, err := store.ReadSeries()
	if err != nil {
		return bandPlanned{}, err
	}
	var stored []healthband.Observation
	for _, observations := range series {
		stored = append(stored, observations...)
	}
	var fresh []healthband.Observation
	if fetch.Reason == "" {
		fresh = healthband.NewerAttempts(stored, fetch.Observations)
		series = healthband.OrderedSeries(append(stored, fresh...))
	}
	plan, err := wal.Plan(series, healthband.PlanOptions{Owner: healthband.NewOwner(), Only: r.opts.series, Fresh: fresh})
	return bandPlanned{
		plan: plan, state: wal.Checkpoint(), counts: counts, fresh: len(fresh),
		skipped: wal.Skipped, interrupted: len(wal.Interrupted),
	}, err
}

// phaseB executes this run's claims one at a time without the lock, each
// recorded (phase C) before the next starts, and returns their outcomes.
// Failed-step logs are fetched only when this run resolved the repository
// and listed its runs, so --no-fetch and a skipped source call no gh.
func (r bandRun) phaseB(ctx context.Context, store *healthband.Store, claims []healthband.DueClaim, fetch bandCIFetch) (map[string]healthband.ClaimOutcome, []healthband.Recorded, error) {
	evidence := bandRunEvidence{client: r.client, projectDir: r.projectDir}
	if fetch.Reason == "" {
		evidence.target = fetch.Target
	}
	diagnoser := newBandDiagnoser(r.projectDir, r.harness, r.opts.noAgent, evidence)
	diagnoser.now, diagnoser.warn = r.deps.clock, r.stderr
	if r.deps.prepare != nil {
		r.deps.prepare(diagnoser)
	}
	outcomes := make(map[string]healthband.ClaimOutcome, len(claims))
	runClaim := func(ctx context.Context, claim healthband.DueClaim) healthband.ClaimOutcome {
		outcome := diagnoser.Run(ctx, claim)
		outcomes[claim.ID] = outcome
		return outcome
	}
	recorded, err := store.ExecuteClaims(ctx, claims, runClaim, healthband.ExecuteOptions{Clock: r.deps.clock})
	return outcomes, recorded, err
}
