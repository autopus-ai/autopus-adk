package omp

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/detect"
)

const (
	defaultOMPModelProbeTimeout = 3 * time.Second
	defaultOMPModelProbeOutput  = 64 * 1024
)

var ompRoutingSettingAllowlist = map[string]struct{}{
	"cycleOrder":                           {},
	"disabledProviders":                    {},
	"enabledModels":                        {},
	"modelRoles":                           {},
	config.OMPNativeAgentModelOverridesKey: {},
	"retry.fallbackChains":                 {},
	"retry.modelFallback":                  {},
	"task.isolation.mode":                  {},
	"tools.approvalMode":                   {},
}

// OMPModelCatalogRunner constrains identity, setting, and model metadata discovery.
type OMPModelCatalogRunner interface {
	Run(ctx context.Context, executable string, args ...string) ([]byte, error)
}

type OMPModelCatalogProbeOptions struct {
	Executable string
	Runner     OMPModelCatalogRunner
	Timeout    time.Duration
	MaxOutput  int
	Settings   []string
}

type OMPModelMetadata struct {
	Provider         string   `json:"provider"`
	Model            string   `json:"model"`
	Family           string   `json:"family"`
	Capabilities     []string `json:"capabilities"`
	Thinking         []string `json:"thinking"`
	AuthEnabled      bool     `json:"auth_enabled"`
	Keyless          bool     `json:"keyless"`
	Disabled         bool     `json:"disabled"`
	OperatorAttested bool     `json:"operator_attested,omitempty"`
}

type OMPModelCatalog struct {
	Models      []OMPModelMetadata `json:"models"`
	Fingerprint string             `json:"fingerprint"`
}

type OMPModelSettingSupport struct {
	Key       string `json:"key"`
	Supported bool   `json:"supported"`
	Reason    string `json:"reason"`
}

type OMPModelCatalogProbeResult struct {
	Status       string                   `json:"status"`
	Reason       string                   `json:"reason"`
	Version      string                   `json:"version,omitempty"`
	CatalogTrust string                   `json:"catalog_trust"`
	Catalog      OMPModelCatalog          `json:"catalog"`
	Settings     []OMPModelSettingSupport `json:"settings,omitempty"`
}

// ProbeOMPModelCatalog observes only bounded, non-secret routing metadata.
// @AX:ANCHOR [AUTO] @AX:SPEC: SPEC-OMP-004: public boundary for installed OMP model and setting discovery.
// @AX:REASON [AUTO]: external CLI output is normalized here before routing, activation, or doctor consumers may trust it.
func ProbeOMPModelCatalog(ctx context.Context, opts OMPModelCatalogProbeOptions) OMPModelCatalogProbeResult {
	return probeOMPModelCatalog(ctx, opts, nil)
}

func probeOMPModelCatalog(
	ctx context.Context,
	opts OMPModelCatalogProbeOptions,
	profile *config.RoleModelProfileConf,
) OMPModelCatalogProbeResult {
	opts = normalizeOMPModelCatalogProbeOptions(opts)
	trust := config.RoleModelCatalogTrustStrict
	if profile != nil {
		trust = profile.EffectiveCatalogTrust()
	}
	result := OMPModelCatalogProbeResult{Status: "blocked", CatalogTrust: trust}
	if opts.Runner == nil {
		result.Reason = "runner_missing"
		return result
	}

	version, reason := runOMPModelCatalogProbe(ctx, opts, "--version")
	if reason != "" || !detect.OMPVersionMatchesIdentity(strings.TrimSpace(string(version))) {
		result.Reason = "identity_unverified"
		return result
	}
	result.Version = strings.TrimSpace(string(version))
	result.Settings = probeOMPModelSettings(ctx, opts)

	output, probeReason := runOMPModelCatalogProbe(ctx, opts, "models", "--json", "--no-extensions")
	if probeReason != "" {
		result.Reason = catalogReasonForProbeFailure(probeReason)
		return result
	}
	result.Catalog, result.Reason = NormalizeOMPModelCatalog(output, opts.MaxOutput)
	if result.Reason == "catalog_metadata_insufficient" &&
		profile != nil && trust == config.RoleModelCatalogTrustOperatorAttested {
		result.Catalog, result.Reason = normalizeOMPOperatorAttestedCatalog(output, opts.MaxOutput, *profile)
	}
	if result.Reason != "catalog_ready" {
		return result
	}
	result.Status = "ready"
	return result
}

func normalizeOMPModelCatalogProbeOptions(opts OMPModelCatalogProbeOptions) OMPModelCatalogProbeOptions {
	if opts.Executable == "" {
		opts.Executable = "omp"
	}
	if opts.Timeout <= 0 {
		opts.Timeout = defaultOMPModelProbeTimeout
	}
	if opts.MaxOutput <= 0 {
		opts.MaxOutput = defaultOMPModelProbeOutput
	}
	return opts
}

func runOMPModelCatalogProbe(parent context.Context, opts OMPModelCatalogProbeOptions, args ...string) ([]byte, string) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, opts.Timeout)
	defer cancel()
	output, err := opts.Runner.Run(ctx, opts.Executable, args...)
	if len(output) > opts.MaxOutput {
		return nil, "output_oversized"
	}
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return nil, "timeout"
		}
		return nil, "exit_nonzero"
	}
	return output, ""
}

func probeOMPModelSettings(ctx context.Context, opts OMPModelCatalogProbeOptions) []OMPModelSettingSupport {
	unique := make(map[string]struct{}, len(opts.Settings))
	for _, key := range opts.Settings {
		unique[key] = struct{}{}
	}
	keys := make([]string, 0, len(unique))
	for key := range unique {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	settings := make([]OMPModelSettingSupport, 0, len(keys))
	for _, key := range keys {
		entry := OMPModelSettingSupport{Key: key}
		if _, allowed := ompRoutingSettingAllowlist[key]; !allowed {
			entry.Reason = "setting_not_allowlisted"
			settings = append(settings, entry)
			continue
		}
		output, reason := runOMPModelCatalogProbe(ctx, opts, "config", "get", key, "--json")
		entry.Reason = reason
		if reason == "" && json.Valid(output) {
			entry.Supported, entry.Reason = true, "observed"
		} else if reason == "" {
			entry.Reason = "output_invalid"
		}
		settings = append(settings, entry)
	}
	return settings
}

func catalogReasonForProbeFailure(reason string) string {
	switch reason {
	case "timeout":
		return "catalog_timeout"
	case "output_oversized":
		return "catalog_oversized"
	default:
		return "catalog_invalid"
	}
}
