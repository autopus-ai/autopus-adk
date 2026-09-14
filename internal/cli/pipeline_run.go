package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/learn"
	"github.com/insajin/autopus-adk/pkg/orcarun"
	"github.com/insajin/autopus-adk/pkg/pipeline"
	"github.com/insajin/autopus-adk/pkg/spec/gates"
)

// pipelineRunConfig holds parsed flag values for the pipeline run command.
type pipelineRunConfig struct {
	Platform               string
	Strategy               string
	ExecutionOwner         string
	executionOwnerExplicit bool
	Continue               bool
	DryRun                 bool
}

// newPipelineRunCmd creates the `auto pipeline run <spec-id>` subcommand.
func newPipelineRunCmd() *cobra.Command {
	cfg := &pipelineRunConfig{}
	return newPipelineRunCmdWithConfig(cfg)
}

// newPipelineRunCmdWithConfig creates the pipeline run command bound to the
// given config pointer, allowing tests to inspect parsed flag values.
func newPipelineRunCmdWithConfig(cfg *pipelineRunConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:          "run <spec-id>",
		Short:        "Execute a full pipeline for a SPEC",
		SilenceUsage: true,
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return fmt.Errorf("spec-id argument is required: auto pipeline run <spec-id>")
			}
			if err := pipeline.ValidateSpecID(args[0]); err != nil {
				return err
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			specID := args[0]
			return runPipeline(cmd, specID, cfg)
		},
	}

	cmd.Flags().StringVar(&cfg.Platform, "platform", "", "AI platform to use (claude, codex, gemini, omp). Auto-detected when omitted.")
	cmd.Flags().Var(
		newExecutionOwnerValue(&cfg.ExecutionOwner, &cfg.executionOwnerExplicit),
		"execution-owner",
		"OMP DAG owner: exact value omp or orca. Defaults to omp.",
	)
	// @AX:NOTE [AUTO]: magic constant — default strategy "sequential" encodes execution policy; change with care
	cmd.Flags().Var(newStrategyValue("sequential", &cfg.Strategy), "strategy", "Execution strategy: sequential.")
	cmd.Flags().BoolVar(&cfg.Continue, "continue", false, "Resume from the last saved checkpoint.")
	cmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "Build prompts without invoking the backend.")

	return cmd
}

// @AX:ANCHOR [AUTO]: CLI integration boundary — wires cobra command args into pipeline engine (fan-in: CLI + tests)
// @AX:REASON [AUTO]: Resolves SPEC identity, executable backend, and canonical receipt storage before dispatch.
// @AX:WARN [AUTO]: Pipeline launch has more than eight validation, backend-selection, persistence, and review branches.
// @AX:REASON [AUTO]: Every preflight failure must persist a blocked receipt before any phase authority is dispatched.
// runPipeline executes the pipeline for the given SPEC ID.
func runPipeline(cmd *cobra.Command, specID string, cfg *pipelineRunConfig) error {
	if err := pipeline.ValidateSpecID(specID); err != nil {
		return err
	}
	ownerDecision, err := resolvePipelineExecutionOwner(cfg)
	if err != nil {
		return err
	}
	// A run holds resources that outlive this process: with owner orca the
	// phase worker is a supervised agent terminal, and nothing reaps it if the
	// CLI is killed outright. Turning the interrupt into a cancelled context is
	// what lets the backend settle its dispatches on Ctrl-C, which is the
	// cancellation branch of SPEC-EXECPLANE-002 REQ-105.
	ctx, stopSignals := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()
	gitHash, _ := getCurrentGitHash()
	requestedStrategy := pipeline.Strategy(cfg.Strategy)
	resolvedSpec, err := resolvePipelineSpec(specID)
	if err != nil {
		return pipelineBlockedError(specID, "", gitHash, requestedStrategy, err)
	}
	if err := pipeline.ValidateStrategy(requestedStrategy); err != nil {
		return pipelineBlockedError(specID, resolvedSpec.SnapshotHash, gitHash, requestedStrategy, err)
	}
	platform := resolvePlatform(cfg.Platform)
	flags := globalFlagsFromContext(cmd.Context())
	projectDir, err := os.Getwd()
	if err != nil {
		return pipelineBlockedError(specID, resolvedSpec.SnapshotHash, gitHash, requestedStrategy,
			fmt.Errorf("pipeline: resolve project directory: %w", err))
	}
	projectDir = filepath.Clean(projectDir)
	route, routeReason := resolvePipelineRoute(projectDir, resolvedSpec.Dir, specID)
	fmt.Fprintf(cmd.ErrOrStderr(), "Pipeline route: %s — %s\n", route, routeReason)
	if platform == "omp" {
		// REQ-106/INV-105: the read-only tier integrity gate completes here,
		// before any checkpoint, worktree, Run, worker, or provider session
		// exists, so no worker can start under an unverified tier contract.
		integrity := pipelineTierIntegritySkipped()
		if ownerDecision.Owner == pipelineExecutionOwnerOrca {
			integrity = runPipelineTierIntegrityGate(ctx, projectDir, specID)
		}
		if _, _, receiptErr := persistPipelineExecutionOwnerReceipt(
			specID, ownerDecision, integrity.Status,
		); receiptErr != nil {
			return receiptErr
		}
		if ownerDecision.Owner == pipelineExecutionOwnerOrca {
			// The verdict used to travel inside the handoff payload this path no
			// longer emits. An unverified contract degrades the run instead of
			// stopping it (SPEC-EXECPLANE-001 REQ-009), so the reason has to
			// reach the operator on the run's own diagnostic stream.
			reportPipelineTierIntegrity(cmd.ErrOrStderr(), integrity)
		}
	}

	cp, err := LoadCheckpointIfContinue(specID, cfg.Continue)
	if err != nil {
		return err
	}

	var backend pipeline.PhaseBackend
	if !cfg.DryRun {
		switch {
		case platform == "omp" && ownerDecision.Owner == pipelineExecutionOwnerOrca:
			// REQ-101: the orca path differs from the omp path in exactly one
			// place — the injected backend. Ordering, gates, retry accounting,
			// and checkpointing stay with the engine below.
			backend, err = newPipelineOrcaBackendForRun(projectDir, specID, resolvedSpec, gitHash)
			err = explainPipelineOrcaUnavailable(err)
		case platform == "omp":
			backend, err = newPipelineOMPBackendForRun(projectDir, specID, resolvedSpec, gitHash)
		default:
			backend, err = newPipelineProviderBackend(platform)
		}
		if err != nil {
			return pipelineBlockedError(specID, resolvedSpec.SnapshotHash, gitHash, requestedStrategy, err)
		}
	}

	// Initialize learn store if learnings directory exists.
	var learnStore *learn.Store
	learningsDir := filepath.Join(".autopus", "learnings")
	if _, statErr := os.Stat(learningsDir); statErr == nil {
		learnStore, _ = learn.NewStore(".")
	}

	engineCfg := pipeline.EngineConfig{
		ProjectDir:    projectDir,
		SpecID:        specID,
		SpecDir:       resolvedSpec.Dir,
		Platform:      platform,
		Strategy:      requestedStrategy,
		Backend:       backend,
		Route:         route,
		Checkpoint:    cp,
		DryRun:        cfg.DryRun,
		SnapshotHash:  resolvedSpec.SnapshotHash,
		GitCommitHash: gitHash,
		RunConfig: pipeline.RunConfig{
			SpecID:            specID,
			CheckpointDir:     pipelineStateDir,
			LearnStore:        learnStore,
			CoverageThreshold: pipelineCoverageThreshold(projectDir),
		},
	}

	engine := pipeline.NewSubprocessEngine(engineCfg)
	result, err := engine.Run(ctx)
	if err != nil {
		return fmt.Errorf("pipeline run failed: %w", err)
	}

	if flags.MultiMode && !cfg.DryRun {
		fmt.Fprintf(cmd.ErrOrStderr(), "Running multi-provider review for %s\n", specID)
		if err := runSpecReview(ctx, specID, "", 0); err != nil {
			return fmt.Errorf("pipeline multi review failed: %w", err)
		}
	}
	if cfg.DryRun {
		fmt.Fprintf(cmd.OutOrStdout(), "Pipeline dry run: %d phases planned; no backend execution\n", len(result.PhaseResults))
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Pipeline complete: %d phases executed\n", len(result.PhaseResults))
	}
	return nil
}

// resolvePipelineRoute asks the gate package whether the compact route is
// authorized for this SPEC, feeding it the same project UI globs and the same
// working-tree change set `auto spec gates` reads, so both commands answer
// from identical evidence. Anything that cannot be established — unreadable
// config, an undeterminable change set, an absent, ambiguous, or contradicted
// contract — keeps every canonical phase.
func resolvePipelineRoute(projectDir, specDir, specID string) (pipeline.PhaseRoute, string) {
	// LoadPreview, not Load: reading a route decision must not rewrite the
	// project's own configuration file as a side effect.
	cfg, err := config.LoadPreview(projectDir)
	if err != nil {
		return pipeline.RouteFull, fmt.Sprintf("project config unreadable: %v", err)
	}
	changed, err := resolveGatesChangeSet(projectDir, "", "")
	if err != nil {
		return pipeline.RouteFull, fmt.Sprintf("actual change set undeterminable: %v", err)
	}
	decision := gates.AuthorizeCompactRoute(specDir, specID, cfg.Design.UIFileGlobs, changed)
	if !decision.Authorized {
		return pipeline.RouteFull, decision.Reason
	}
	return pipeline.RouteCompact, decision.Reason +
		"; plan and test_scaffold are not dispatched"
}

// explainPipelineOrcaUnavailable names the remedy for a missing orca CLI. The
// operator selected supervised execution by name, so falling back to the native
// OMP DAG would run the workload under an execution owner they declined; the run
// stops instead and says what would let it proceed.
func explainPipelineOrcaUnavailable(err error) error {
	if !errors.Is(err, orcarun.ErrOrcaUnavailable) {
		return err
	}
	return fmt.Errorf(
		"%w: --execution-owner orca runs every phase in a supervised orca worker; "+
			"install the orca CLI on PATH or rerun with --execution-owner omp", err)
}

func newPipelineOMPBackendForRun(
	projectDir string,
	specID string,
	resolvedSpec resolvedPipelineSpec,
	gitHash string,
) (*pipelineOMPBackend, error) {
	executable, err := exec.LookPath("omp")
	if err != nil {
		return nil, fmt.Errorf("pipeline: platform %q executable %q is unavailable: %w", "omp", "omp", err)
	}
	executable, executableID, err := canonicalPipelineOMPExecutable(executable)
	if err != nil {
		return nil, err
	}
	environment, err := normalizePipelineOMPEnvironment(os.Environ())
	if err != nil {
		return nil, err
	}
	phaseModels, err := loadPipelineOMPPhaseModelsWithAuthority(projectDir, executable, executableID, environment)
	if err != nil {
		return nil, fmt.Errorf("pipeline: load OMP model routes: %w", err)
	}
	return newPipelineOMPBackend(pipelineOMPBackendConfig{
		Executable: executable, ProjectDir: projectDir, SpecID: specID, SpecDir: resolvedSpec.Dir,
		SnapshotHash: resolvedSpec.SnapshotHash, GitCommitHash: gitHash,
		// @AX:NOTE [AUTO] @AX:SPEC: SPEC-OMP-004: each canonical OMP phase is bounded to 30 minutes.
		Environment: environment, PhaseModels: phaseModels, MaxTime: 30 * time.Minute, executableID: executableID,
		ManagedActive: newPipelineOMPManagedActiveCoordinator(),
	})
}
