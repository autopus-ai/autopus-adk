package cli

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"time"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/processprobe"
	"github.com/insajin/autopus-adk/pkg/workflow"
)

const workflowVersionProbeTimeout = 2 * time.Second

// NewWorkflowCmd builds the `auto workflow` command tree (doctor/render/gate).
// A nil prober or runner selects the production default; tests inject fakes via
// this exported constructor to keep the seams hermetic.
func NewWorkflowCmd(prober workflow.Prober, runner workflow.CommandRunner) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "workflow",
		Short:         "Inspect and gate the opt-in deterministic workflow route",
		Long:          "Commands for the claude-scoped `/auto go --workflow` Route A: doctor capability gate, dry-run render, and the deterministic gate JS->Go bridge.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newWorkflowDoctorCmd(prober))
	cmd.AddCommand(newWorkflowGateCmd(runner))
	cmd.AddCommand(newWorkflowRenderCmd())
	cmd.AddCommand(newWorkflowMergeCmd())
	cmd.AddCommand(newWorkflowBindingCmd(nil))
	cmd.AddCommand(newWorkflowContextCmd())
	cmd.AddCommand(newWorkflowContextPlanCmd())
	cmd.AddCommand(newWorkflowContextRuntimeCmd())
	cmd.AddCommand(newWorkflowTriageCmd())
	return cmd
}

// newWorkflowCmd is the production registration entry point used by root.go.
func newWorkflowCmd() *cobra.Command {
	return NewWorkflowCmd(nil, nil)
}

// newWorkflowDoctorCmd runs the capability gate and exits non-zero when the
// overall verdict is fail (S4/S12), zero when it passes (S14).
func newWorkflowDoctorCmd(prober workflow.Prober) *cobra.Command {
	var route string
	cmd := &cobra.Command{
		Use:           "doctor",
		Short:         "Probe workflow capabilities and the selected route's version pin",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, routeKey, err := selectRouteEmbed(route)
			if err != nil {
				return err
			}
			p := prober
			if p == nil {
				p = newLiveProber()
			}
			report, err := workflow.EvaluateCapabilitiesForRoute(p, routeKey)
			if err != nil {
				return fmt.Errorf("evaluate workflow capabilities: %w", err)
			}
			data, err := report.EncodeJSON()
			if err != nil {
				return fmt.Errorf("encode capability report: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			if report.Overall == workflow.OverallFail {
				return fmt.Errorf("workflow doctor: capability gate failed (route=%s, overall=fail)", routeKey)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&route, "route", workflow.RouteA, "Workflow route to probe (route_a|route_team; shorthands: a|team)")
	return cmd
}

// liveProber is the production capability prober. Without access to the closed
// claude-code Workflow primitive registry, it infers the availability of the
// claude-scoped primitives from the claude binary presence and reads the
// version from `claude --version`. This is a defensible heuristic for the
// hard-gate and is documented as such.
type liveProber struct {
	version string
	present bool
}

func newLiveProber() liveProber {
	return newLiveProberWithin(workflowVersionProbeTimeout)
}

// newLiveProberWithin is newLiveProber with an explicit ceiling. The ceiling is
// the last-resort bound: processprobe.Output stops draining an inherited pipe
// shortly after the probed process exits, so a healthy probe returns long
// before it. Tests widen the ceiling to tell those two bounds apart without
// timing the machine.
func newLiveProberWithin(timeout time.Duration) liveProber {
	path, err := exec.LookPath("claude")
	if err != nil {
		return liveProber{}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, "--version") //nolint:gosec // fixed binary, no user input
	out, err := processprobe.Output(cmd)
	if err != nil {
		return liveProber{present: true}
	}
	return liveProber{version: parseClaudeVersion(string(out)), present: true}
}

func (p liveProber) Version() string { return p.version }

func (p liveProber) Probe(string) bool { return p.present }

var versionTokenRe = regexp.MustCompile(`(\d+\.\d+\.\d+)`)

func parseClaudeVersion(out string) string {
	if m := versionTokenRe.FindString(out); m != "" {
		return m
	}
	return ""
}

// execCommandRunner is the production CommandRunner: it runs the command and
// returns the process exit code.
type execCommandRunner struct{}

func (execCommandRunner) Run(ctx context.Context, name string, args ...string) (int, error) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // commands come from operator flags
	err := cmd.Run()
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if ok := asExitError(err, &exitErr); ok {
		return exitErr.ExitCode(), err
	}
	return 1, err
}

func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}
