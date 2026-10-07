package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type doctorJSONReport struct {
	status   jsonEnvelopeStatus
	data     doctorJSONData
	warnings []jsonMessage
	checks   []jsonCheck
}

type doctorJSONData struct {
	OverallOK     bool                          `json:"overall_ok"`
	Config        *doctorConfigPayload          `json:"config,omitempty"`
	Platforms     []doctorPlatformPayload       `json:"platforms,omitempty"`
	Dependencies  []doctorDependencyPayload     `json:"dependencies,omitempty"`
	Runtime       []doctorRuntimeProcessPayload `json:"runtime_processes,omitempty"`
	RuleConflicts []doctorRuleConflictPayload   `json:"rule_conflicts,omitempty"`
	InstalledCLIs []doctorCLIPayload            `json:"installed_clis,omitempty"`
	Hygiene       *statusHygienePayload         `json:"hygiene,omitempty"`
}

type doctorConfigPayload struct {
	Loaded       bool     `json:"loaded"`
	Mode         string   `json:"mode,omitempty"`
	Platforms    []string `json:"platforms,omitempty"`
	IsolateRules bool     `json:"isolate_rules,omitempty"`
}

type doctorPlatformPayload struct {
	Name     string                 `json:"name"`
	Valid    bool                   `json:"valid"`
	Messages []doctorMessagePayload `json:"messages,omitempty"`
}

type doctorDependencyPayload struct {
	Name       string `json:"name"`
	Binary     string `json:"binary"`
	Installed  bool   `json:"installed"`
	Required   bool   `json:"required"`
	InstallCmd string `json:"install_cmd,omitempty"`
}

type doctorRuntimeProcessPayload struct {
	PID        int    `json:"pid"`
	PPID       int    `json:"ppid,omitempty"`
	Executable string `json:"executable"`
	Command    string `json:"command,omitempty"`
	Reason     string `json:"reason,omitempty"`
}

type doctorRuleConflictPayload struct {
	ParentDir string `json:"parent_dir"`
	Namespace string `json:"namespace"`
	Ignored   bool   `json:"ignored"`
}

type doctorCLIPayload struct {
	Name    string `json:"name"`
	Binary  string `json:"binary"`
	Version string `json:"version"`
}

type doctorMessagePayload struct {
	Level   string `json:"level"`
	Message string `json:"message"`
	// File is the repo-relative path the finding refers to. Findings without a
	// path (e.g. unknown platform) omit the key entirely.
	File string `json:"file,omitempty"`
}

func runDoctorJSON(cmd *cobra.Command, opts doctorOptions) error {
	report := collectDoctorJSONReport(cmd, opts)
	report.data.OverallOK = report.status == jsonStatusOK
	return writeJSONResult(cmd, report.status, report.data, report.warnings, report.checks)
}

func collectDoctorJSONReport(cmd *cobra.Command, opts doctorOptions) doctorJSONReport {
	report := doctorJSONReport{status: jsonStatusOK}
	ctx := doctorCommandContext(cmd)

	cfg, err := loadHarnessConfigForDir(opts.dir, globalFlags{})
	if err != nil {
		report.status = jsonStatusWarn
		report.data.Config = &doctorConfigPayload{Loaded: false}
		report.warnings = append(report.warnings, jsonMessage{
			Code:    "config_load_failed",
			Message: fmt.Sprintf("autopus.yaml load failed: %v", err),
		})
		report.checks = append(report.checks, jsonCheck{
			ID:       "doctor.config.autopus_yaml",
			Severity: "error",
			Status:   "fail",
			Detail:   fmt.Sprintf("autopus.yaml load failed: %v", err),
		})
		report.collectHygieneChecks(opts.dir)
		return report
	}

	report.data.Config = &doctorConfigPayload{
		Loaded:       true,
		Mode:         string(cfg.Mode),
		Platforms:    append([]string{}, cfg.Platforms...),
		IsolateRules: cfg.IsolateRules,
	}
	report.checks = append(report.checks, jsonCheck{
		ID:       "doctor.config.autopus_yaml",
		Severity: "info",
		Status:   "pass",
		Detail:   fmt.Sprintf("autopus.yaml loaded (mode: %s)", cfg.Mode),
	})

	report.collectPlatformChecks(ctx, opts.dir, cfg)
	report.collectDependencyChecks(cmd, opts)
	report.collectRuntimeProcessChecks(opts)
	// Both of these read `.claude/**`; see configuresClaudeCode for why an
	// unconfigured claude-code makes them unresolvable noise rather than signal.
	if configuresClaudeCode(cfg) {
		report.collectRuleConflictChecks(opts.dir, cfg)
	}
	report.collectCLIChecks()
	report.collectDesktopShimCheck(diagnoseDesktopShim())
	report.collectHomebrewTrustCheck(diagnoseHomebrewTrust(ctx))
	report.collectQualityGateChecks(cfg)
	report.collectCodexModelOwnershipCheck(opts.dir, cfg)
	report.collectCodexAgentConcurrencyCheck(opts.dir, cfg)
	report.collectProviderTransportSmokeChecks(cfg, opts)
	report.collectProviderReadinessChecks(ctx, cfg)
	if configuresClaudeCode(cfg) {
		report.collectHookChecks(opts.dir)
	}
	report.collectRetiredOrchestraChecks(opts.dir, cfg)
	report.collectContextWeightChecks(opts.dir)
	report.collectHygieneChecks(opts.dir)
	report.collectDriftGateChecksContext(ctx, opts.dir, cfg)
	report.collectEvidenceFreshnessChecks(opts.dir, cfg)

	return report
}
