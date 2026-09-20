package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/insajin/autopus-adk/pkg/agentprobe"
	"github.com/insajin/autopus-adk/pkg/telemetry"
	"github.com/spf13/cobra"
)

type doctorAgentObservation struct {
	Platform         string                       `json:"platform"`
	Installed        *bool                        `json:"installed"`
	RuntimeVersion   string                       `json:"runtime_version"`
	Status           string                       `json:"status"`
	RuntimeVerified  bool                         `json:"lifecycle_verified"`
	Lifecycle        *agentprobe.Report           `json:"lifecycle,omitempty"`
	Evidence         *agentprobe.Evidence         `json:"evidence,omitempty"`
	Transport        any                          `json:"transport,omitempty"`
	ExecutionSurface string                       `json:"execution_surface,omitempty"`
	TeamEvidence     *telemetry.TeamUsageEvidence `json:"team_usage_evidence,omitempty"`
	TeamUsage        *telemetry.TeamUsageReport   `json:"team_usage,omitempty"`
	UsageReason      string                       `json:"usage_reason,omitempty"`
	FailureReason    string                       `json:"failure_reason,omitempty"`
}
type doctorAgentsReport struct {
	Version      int                      `json:"version"`
	Mode         string                   `json:"mode"`
	Observations []doctorAgentObservation `json:"observations"`
}
type doctorAgentsOptions struct {
	platform, format, trace, endpoint, username, passwordEnv, runtimeVersion string
	provider, model, directory                                               string
	live                                                                     bool
	timeout                                                                  time.Duration
}

func newDoctorAgentsCmd() *cobra.Command {
	var opts doctorAgentsOptions
	cmd := &cobra.Command{Use: "agents", Short: "Inspect native agent lifecycle evidence (live probes are opt-in)", Args: cobra.NoArgs, SilenceUsage: true, SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := opts.validate(); err != nil {
				return err
			}
			report := doctorAgentsReport{Version: 1, Mode: "inventory", Observations: []doctorAgentObservation{}}
			var probeErr error
			switch {
			case opts.trace != "":
				report.Mode = "supplied_trace"
				evidence, err := readAgentLifecycleTrace(opts.trace)
				if err != nil {
					return err
				}
				decision, err := agentprobe.Evaluate(evidence)
				if err != nil {
					return err
				}
				report.Observations = append(report.Observations, doctorAgentObservation{Platform: evidence.Platform, RuntimeVersion: evidence.RuntimeVersion, Status: decision.Overall, Lifecycle: &decision, Evidence: &evidence})
			case opts.live:
				report.Mode = "live_requested"
				if opts.platform == "opencode" {
					report.Mode = "live_endpoint_capture"
				}
				if opts.platform == "codex" {
					report.Mode = "live_local_process_capture"
				}
				observation, err := runDoctorAgentLive(cmd.Context(), opts)
				probeErr = err
				report.Observations = append(report.Observations, observation)
			default:
				for _, platform := range agentProbePlatforms {
					if opts.platform == "all" || opts.platform == platform.name {
						report.Observations = append(report.Observations, inspectAgentRuntime(cmd.Context(), platform))
					}
				}
			}
			if opts.format == "json" {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(report); err != nil {
					return err
				}
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "Agent lifecycle: %s (version/help is not lifecycle proof)\n", report.Mode)
				for _, observation := range report.Observations {
					installed := "unknown"
					if observation.Installed != nil {
						installed = fmt.Sprint(*observation.Installed)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "%s version=%s installed=%s status=%s lifecycle_verified=%t\n", observation.Platform, observation.RuntimeVersion, installed, observation.Status, observation.RuntimeVerified)
					if observation.FailureReason != "" {
						fmt.Fprintf(cmd.OutOrStdout(), "  failure: %s\n", observation.FailureReason)
					}
					if observation.TeamUsage != nil {
						fmt.Fprintf(cmd.OutOrStdout(), "  usage: known_tokens=%d total_tokens=%s known_estimated_usd=%g actual_usd=%s\n", observation.TeamUsage.KnownActualTokens, harnessNumber(observation.TeamUsage.ActualTokens), observation.TeamUsage.KnownEstimatedCostUSD, teamCost(observation.TeamUsage.ActualCostUSD))
					}
					if observation.Lifecycle != nil {
						for _, gate := range observation.Lifecycle.Gates {
							fmt.Fprintf(cmd.OutOrStdout(), "  %s: %s (%s)\n", gate.ID, gate.Status, gate.Reason)
						}
					}
				}
			}
			return probeErr
		}}
	cmd.Flags().StringVar(&opts.platform, "platform", "all", "Platform: all, codex, claude-code, opencode, omp, antigravity-cli, gemini-cli")
	cmd.Flags().StringVar(&opts.format, "format", "human", "Output format: human or json")
	cmd.Flags().StringVar(&opts.trace, "trace-json", "", "Evaluate a supplied normalized trace; not independent runtime proof")
	cmd.Flags().BoolVar(&opts.live, "live", false, "Run the selected native lifecycle probe; may consume provider quota")
	cmd.Flags().StringVar(&opts.endpoint, "endpoint", "", "Task-owned authenticated OpenCode loopback server URL")
	cmd.Flags().StringVar(&opts.username, "username", "opencode", "OpenCode server username")
	cmd.Flags().StringVar(&opts.passwordEnv, "password-env", "", "Environment variable containing the server password (not printed)")
	cmd.Flags().StringVar(&opts.runtimeVersion, "runtime-version", "", "Expected endpoint runtime version")
	cmd.Flags().StringVar(&opts.provider, "provider", "", "Explicit OpenCode model provider ID")
	cmd.Flags().StringVar(&opts.model, "model", "", "Explicit probe model ID")
	cmd.Flags().StringVar(&opts.directory, "dir", "", "Task-owned probe directory (default: isolated collector/server directory)")
	cmd.Flags().DurationVar(&opts.timeout, "timeout", 90*time.Second, "Live probe timeout (positive, at most 3m; cleanup has separate bound)")
	return cmd
}

func (o doctorAgentsOptions) validate() error {
	if o.format != "human" && o.format != "json" {
		return fmt.Errorf("unsupported output format")
	}
	if o.timeout <= 0 || o.timeout > 3*time.Minute {
		return fmt.Errorf("timeout must be positive and at most 3m")
	}
	known := o.platform == "all"
	for _, p := range agentProbePlatforms {
		known = known || p.name == o.platform
	}
	if !known {
		return fmt.Errorf("unsupported platform")
	}
	if o.trace != "" && o.live {
		return fmt.Errorf("trace input and live execution are mutually exclusive")
	}
	if o.live && o.platform == "all" {
		return fmt.Errorf("live probes require one explicit platform")
	}
	if o.live && o.platform == "opencode" && o.timeout > 2*time.Minute {
		return fmt.Errorf("OpenCode probe timeout must be at most 2m")
	}
	if o.live && o.platform == "opencode" && (o.provider == "" || o.model == "") {
		return fmt.Errorf("OpenCode live probe requires explicit --provider and --model")
	}
	if o.live && o.platform == "opencode" && (o.endpoint == "" || o.passwordEnv == "" || os.Getenv(o.passwordEnv) == "") {
		return fmt.Errorf("OpenCode live probe requires an authenticated task-owned endpoint and password-env")
	}
	return nil
}
