package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
	"github.com/insajin/autopus-adk/pkg/config"
)

type ompProfileActivator func(context.Context, string, *config.HarnessConfig) error

type ompProfileApplyPayload struct {
	Platform           string                        `json:"platform"`
	Name               string                        `json:"name"`
	Status             string                        `json:"status"`
	Generated          bool                          `json:"generated"`
	Source             string                        `json:"source"`
	Family             string                        `json:"family"`
	AgentOverrides     []string                      `json:"agent_overrides"`
	ProfileDefinition  bool                          `json:"profile_definition"`
	ConfigPath         string                        `json:"config_path"`
	Activation         string                        `json:"activation"`
	CatalogVersion     string                        `json:"catalog_version"`
	CatalogFingerprint string                        `json:"catalog_fingerprint"`
	Capabilities       []ompProfileCapabilityPayload `json:"capabilities"`
}

func runOMPProfileApplyCommand(
	cmd *cobra.Command,
	root string,
	opts ompProfileApplyOptions,
	runner omp.OMPModelCatalogRunner,
	activate ompProfileActivator,
) error {
	if opts.plan {
		return runOMPProfilePlanPreview(cmd, root, opts, runner)
	}
	payload, err := applyOMPProfile(cmd.Context(), root, opts, runner, activate)
	if err != nil {
		if !opts.jsonMode {
			return err
		}
		return writeJSONResultAndExit(
			cmd, jsonStatusError, err, "omp_profile_apply_failed",
			map[string]any{"platform": "omp", "name": opts.name, "status": "blocked"},
			[]jsonMessage{{Code: "omp_profile_apply_failed", Message: err.Error()}}, nil,
		)
	}
	if opts.jsonMode {
		return writeJSONResult(cmd, jsonStatusOK, payload, nil, []jsonCheck{{
			ID: "omp.profile.activation", Severity: "info", Status: "pass", Detail: "omp_update",
		}})
	}
	renderOMPProfileApplyText(cmd, payload)
	return nil
}

func renderOMPProfileApplyText(cmd *cobra.Command, payload ompProfileApplyPayload) {
	out := cmd.OutOrStdout()
	_, _ = fmt.Fprintf(out, "OMP profile applied: %s\n", payload.Name)
	_, _ = fmt.Fprintf(out, "Source: %s\n", payload.Source)
	_, _ = fmt.Fprintf(out, "Family: %s\n", ompProfileDisplayValue(payload.Family, "default"))
	_, _ = fmt.Fprintf(out, "Profile definition: %t\n", payload.ProfileDefinition)
	_, _ = fmt.Fprintf(
		out, "Agent overrides: %s\n",
		ompProfileDisplayValue(strings.Join(payload.AgentOverrides, ", "), "none"),
	)
	_, _ = fmt.Fprintf(out, "Config: %s\nActivation: %s\n", payload.ConfigPath, payload.Activation)
	_, _ = fmt.Fprintf(out, "Catalog: %s %s\n", payload.CatalogVersion, payload.CatalogFingerprint)
}

// runOMPProfilePlanPreview renders the zero-write preview of the very proposal
// apply would persist. An unavailable route is displayed and then returned as a
// nonzero exit, never substituted with a lower tier.
func runOMPProfilePlanPreview(
	cmd *cobra.Command,
	root string,
	opts ompProfileApplyOptions,
	runner omp.OMPModelCatalogRunner,
) error {
	payload, err := planOMPProfile(cmd.Context(), root, opts, runner)
	if err != nil {
		return writeOMPProfilePlanFailure(cmd, opts.jsonMode, err)
	}
	blocked := ompProfileUnavailableError{blockers: payload.Blockers}
	if opts.jsonMode {
		if len(payload.Blockers) > 0 {
			return writeJSONResultAndExit(
				cmd, jsonStatusError, blocked, "omp_profile_unavailable", payload,
				[]jsonMessage{{Code: "omp_profile_unavailable", Message: blocked.Error()}},
				[]jsonCheck{{
					ID: "omp.profile.plan", Severity: "error", Status: "fail", Detail: "candidate_unavailable",
				}},
			)
		}
		return writeJSONResult(cmd, jsonStatusOK, payload, nil, []jsonCheck{{
			ID: "omp.profile.plan", Severity: "info", Status: "pass", Detail: "zero_writes",
		}})
	}
	renderOMPProfileApplyPreviewText(cmd, payload)
	if len(payload.Blockers) > 0 {
		return blocked
	}
	return nil
}

// planOMPProfile reads the project config through the same snapshot guard as
// apply and writes nothing: no autopus.yaml replace, no activation, no receipt.
func planOMPProfile(
	ctx context.Context,
	root string,
	opts ompProfileApplyOptions,
	runner omp.OMPModelCatalogRunner,
) (ompProfileApplyPreviewPayload, error) {
	snapshot, cfg, err := loadOMPProfileConfig(root)
	if err != nil {
		return ompProfileApplyPreviewPayload{}, err
	}
	defer snapshot.Close()
	return planOMPProfileForConfig(ctx, cfg, opts, runner)
}

// planOMPProfileForConfig plans opts against cfg, which it prepares in place
// exactly as apply would, and writes nothing. planOMPProfile hands it the
// config verified against the file snapshot; init hands it a detached copy
// of the config it has not saved yet (SPEC-MODELUX-002 REQ-MUX2-22). The
// catalog, attestation, capability, and blocker rules are the same either
// way.
func planOMPProfileForConfig(
	ctx context.Context,
	cfg *config.HarnessConfig,
	opts ompProfileApplyOptions,
	runner omp.OMPModelCatalogRunner,
) (ompProfileApplyPreviewPayload, error) {
	resolution, err := resolveOMPProfileProposal(ctx, cfg, opts, runner)
	if err != nil {
		return ompProfileApplyPreviewPayload{}, err
	}
	return newOMPProfileApplyPreviewPayload(resolution, opts), nil
}

// loadOMPProfileConfig opens the config snapshot, rejects placeholder-bearing
// role policies, and returns the loaded config verified against the snapshot.
// The caller owns closing the snapshot.
func loadOMPProfileConfig(root string) (*autopusConfigSnapshot, *config.HarnessConfig, error) {
	snapshot, err := openAutopusConfigSnapshot(root)
	if err != nil {
		return nil, nil, err
	}
	if err := validateOMPProfileSource(snapshot.data); err != nil {
		snapshot.Close()
		return nil, nil, err
	}
	cfg, loadErr := config.LoadPreview(snapshot.rootPath)
	if err := snapshot.Verify(); err != nil {
		snapshot.Close()
		return nil, nil, err
	}
	if loadErr != nil {
		snapshot.Close()
		return nil, nil, errors.New("autopus_config_invalid")
	}
	if !containsOMPString(cfg.Platforms, "omp") {
		snapshot.Close()
		return nil, nil, errors.New("omp_platform_not_configured")
	}
	return snapshot, cfg, nil
}

func applyOMPProfile(
	ctx context.Context,
	root string,
	opts ompProfileApplyOptions,
	runner omp.OMPModelCatalogRunner,
	activate ompProfileActivator,
) (ompProfileApplyPayload, error) {
	snapshot, cfg, err := loadOMPProfileConfig(root)
	if err != nil {
		return ompProfileApplyPayload{}, err
	}
	defer snapshot.Close()
	if activate == nil {
		return ompProfileApplyPayload{}, errors.New("omp_activation_path_unavailable")
	}
	resolution, err := resolveOMPProfileProposal(ctx, cfg, opts, runner)
	if err != nil {
		return ompProfileApplyPayload{}, err
	}
	if len(resolution.blockers) > 0 {
		return ompProfileApplyPayload{}, ompProfileUnavailableError{blockers: resolution.blockers}
	}
	original := snapshot.data
	encoded, err := marshalAutopusConfig(original, cfg)
	if err != nil {
		return ompProfileApplyPayload{}, err
	}
	if err := snapshot.Replace(encoded); err != nil {
		return ompProfileApplyPayload{}, fmt.Errorf("persist OMP profile: %w", err)
	}
	if err := activateAppliedOMPProfile(ctx, snapshot, cfg, activate, original); err != nil {
		return ompProfileApplyPayload{}, err
	}
	return newOMPProfileApplyPayload(resolution), nil
}

// activateAppliedOMPProfile activates the persisted selection and restores the
// original config byte-for-byte when activation fails.
func activateAppliedOMPProfile(
	ctx context.Context,
	snapshot *autopusConfigSnapshot,
	cfg *config.HarnessConfig,
	activate ompProfileActivator,
	original []byte,
) error {
	if err := activate(ctx, snapshot.rootPath, cfg); err != nil {
		if verifyErr := snapshot.Verify(); verifyErr != nil {
			return fmt.Errorf(
				"activate OMP profile: %w; config rollback blocked: autopus_config_changed", err,
			)
		}
		if rollbackErr := snapshot.Replace(original); rollbackErr != nil {
			return fmt.Errorf(
				"activate OMP profile: %w; config rollback failed: %v", err, rollbackErr,
			)
		}
		return fmt.Errorf("activate OMP profile: %w", err)
	}
	if err := snapshot.Verify(); err != nil {
		return errors.New("autopus_config_changed_during_activation")
	}
	return nil
}

func newOMPProfileApplyPayload(resolution ompProfileResolution) ompProfileApplyPayload {
	effective := resolution.effective
	return ompProfileApplyPayload{
		Platform: "omp", Name: effective.name, Status: "applied",
		Generated: effective.source == ompProfileSourceGenerated,
		Source:    effective.source, Family: effective.family,
		AgentOverrides:    sortedOMPProfileOverriddenAgents(effective.overrides),
		ProfileDefinition: effective.definition,
		ConfigPath:        autopusConfigName, Activation: "omp_update",
		CatalogVersion:     safeOMPOperatorVersion(resolution.probe.Version),
		CatalogFingerprint: resolution.probe.Catalog.Fingerprint,
		Capabilities:       profileCapabilityPayloads(effective.profile),
	}
}

func profileCapabilityPayloads(profile config.RoleModelProfileConf) []ompProfileCapabilityPayload {
	rows := make([]ompProfileCapabilityPayload, 0, len(config.OMPProviderNeutralCapabilities()))
	for _, capability := range config.OMPProviderNeutralCapabilities() {
		route := profile.Capabilities[capability]
		row := ompProfileCapabilityPayload{Capability: capability, Required: route.Required}
		for _, candidate := range route.Candidates {
			row.Candidates = append(row.Candidates, ompProfileCandidatePayload(candidate))
		}
		if row.Candidates == nil {
			row.Candidates = []ompProfileCandidatePayload{}
		}
		rows = append(rows, row)
	}
	return rows
}
