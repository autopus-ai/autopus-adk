package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Selection sources that make a provider explicit. A policy violation of an
// explicit provider fails the review; a provider that enters only through
// discovery (--multi, orchestra.commands.review.providers, orchestra.providers
// keys, or built-in defaults) is excluded instead (REQ-06).
const (
	specReviewSourceProvidersFlag = "--providers"
	specReviewSourceGateProviders = "spec.review_gate.providers"
	specReviewSourceGateJudge     = "spec.review_gate.judge"
)

// specReviewRemedyValueRunes bounds a rejected value quoted in a remedy.
const specReviewRemedyValueRunes = 64

// specReviewProviderRequest carries the inputs of the read-only reviewer assembly.
type specReviewProviderRequest struct {
	Config *config.HarnessConfig
	// FlagProviders is --providers; when set it replaces the configured names.
	FlagProviders []string
	Multi         bool
	// RequestedTimeout is --timeout in seconds; zero keeps provider defaults.
	RequestedTimeout int
	// Warnings receives one line per excluded discovered provider; nil discards.
	Warnings io.Writer
}

// specReviewExclusion is a discovered provider the read-only gate removed.
type specReviewExclusion struct {
	Provider string
	Reason   string
}

// specReviewProviderSet is the reviewer set ready for execution.
type specReviewProviderSet struct {
	// Names is the quorum denominator: the selected names minus exclusions.
	Names     []string
	Providers []orchestra.ProviderConfig
	Excluded  []specReviewExclusion
}

// assembleSpecReviewProviders builds the reviewers in an order that keeps any
// configured binary from running before the read-only gate accepts it:
// resolve, gate, installed filter, codex capabilities, configure, timeout,
// and the read-only projection as the final argv mutation.
func assembleSpecReviewProviders(ctx context.Context, req specReviewProviderRequest) (specReviewProviderSet, error) {
	names, explicit := selectSpecReviewProviderNames(req.Config, req.FlagProviders, req.Multi)
	var accepted []orchestra.ProviderConfig
	var excluded []specReviewExclusion
	for _, provider := range resolveSpecReviewProviderConfigs(req.Config, names) {
		violation := checkReadOnlyProvider(provider)
		if violation == nil {
			accepted = append(accepted, provider)
			continue
		}
		if source, ok := explicit[provider.Name]; ok {
			return specReviewProviderSet{}, specReviewPolicyError(violation, source)
		}
		excluded = append(excluded, specReviewExclusion{Provider: provider.Name, Reason: violation.Reason()})
	}
	warnings := req.Warnings
	if warnings == nil {
		warnings = io.Discard
	}
	for _, exclusion := range excluded {
		fmt.Fprintf(warnings, "spec review: excluding discovered provider %q: %s\n", exclusion.Provider, exclusion.Reason)
	}

	providers := filterInstalledProviders(accepted)
	providers = configureSpecReviewProviders(resolveCodexProviderCapabilities(ctx, providers))
	providers = applySpecReviewExecutionTimeout(providers, req.RequestedTimeout)
	// The gate already removed unsupported providers, so a projection
	// violation always names a provider field and needs no selection source.
	projected, err := applyReadOnlyProviderPolicy(providers, readOnlyPolicyOptions{})
	if err != nil {
		return specReviewProviderSet{}, specReviewPolicyError(err, "")
	}
	return specReviewProviderSet{Names: withoutExcludedNames(names, excluded), Providers: projected, Excluded: excluded}, nil
}

// assembleSpecReviewJudge returns the judge through the same gate and
// projection as the reviewers (REQ-02). A judge that names an assembled
// reviewer reuses it; otherwise it is resolved separately as an explicit
// spec.review_gate.judge selection and checked before any of its binaries run.
func assembleSpecReviewJudge(
	ctx context.Context,
	cfg *config.HarnessConfig,
	reviewers []orchestra.ProviderConfig,
	judge string,
	requestedTimeout int,
) (*orchestra.ProviderConfig, error) {
	if judge == "" {
		return nil, nil
	}
	var candidates []orchestra.ProviderConfig
	for _, reviewer := range reviewers {
		if reviewer.Name == judge {
			candidates = []orchestra.ProviderConfig{reviewer}
			break
		}
	}
	if candidates == nil {
		if cfg == nil {
			return nil, nil
		}
		candidates = resolveProviders(&cfg.Orchestra, "review", []string{judge})
		if violation := checkReadOnlyProvider(candidates[0]); violation != nil {
			return nil, specReviewPolicyError(violation, specReviewSourceGateJudge)
		}
		candidates = configureSpecReviewProviders(resolveCodexProviderCapabilities(ctx, candidates))
		candidates = applySpecReviewExecutionTimeout(candidates, requestedTimeout)
	}
	projected, err := applyReadOnlyProviderPolicy(candidates, readOnlyPolicyOptions{})
	if err != nil {
		return nil, specReviewPolicyError(err, specReviewSourceGateJudge)
	}
	return &projected[0], nil
}

// selectSpecReviewProviderNames returns the reviewer names and the explicit
// selection source of each explicitly named provider.
func selectSpecReviewProviderNames(cfg *config.HarnessConfig, flagProviders []string, multi bool) ([]string, map[string]string) {
	explicit := make(map[string]string)
	if len(flagProviders) > 0 {
		for _, name := range flagProviders {
			explicit[name] = specReviewSourceProvidersFlag
		}
		return append([]string(nil), flagProviders...), explicit
	}
	if cfg != nil {
		for _, name := range cfg.Spec.ReviewGate.Providers {
			explicit[name] = specReviewSourceGateProviders
		}
	}
	return resolveSpecReviewProviderNames(cfg, multi), explicit
}

// resolveSpecReviewProviderConfigs turns names into provider configs without
// executing anything; installation is checked only after the gate.
func resolveSpecReviewProviderConfigs(cfg *config.HarnessConfig, names []string) []orchestra.ProviderConfig {
	if cfg == nil {
		return buildProviderConfigs(names)
	}
	return resolveProviders(&cfg.Orchestra, "review", names)
}

func withoutExcludedNames(names []string, excluded []specReviewExclusion) []string {
	dropped := make(map[string]struct{}, len(excluded))
	for _, exclusion := range excluded {
		dropped[exclusion.Provider] = struct{}{}
	}
	kept := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := dropped[name]; !ok {
			kept = append(kept, name)
		}
	}
	return kept
}

// specReviewPolicyError renders the spec review Error Contract from a typed
// policy violation; source names the selection of an unsupported provider.
func specReviewPolicyError(err error, source string) error {
	var violation *readOnlyPolicyViolation
	if !errors.As(err, &violation) {
		return err
	}
	key := source
	if violation.Kind != readOnlyUnsupportedProvider {
		key = "orchestra.providers." + violation.Provider + "." + violation.Field
	}
	return fmt.Errorf("spec review: provider %q rejected by the read-only policy: %s (config key: %s; remedy: %s)",
		violation.Provider, violation.Reason(), key, specReviewPolicyRemedy(violation, key))
}

func specReviewPolicyRemedy(violation *readOnlyPolicyViolation, key string) string {
	switch violation.Kind {
	case readOnlyNativeBinary:
		return fmt.Sprintf("set %s to %q", key, violation.Item)
	case readOnlyUnsupportedSchemaFlag:
		return "remove " + key
	case readOnlyUnsafeValue:
		value := []rune(violation.Value)
		if len(value) > specReviewRemedyValueRunes {
			value = value[:specReviewRemedyValueRunes]
		}
		separator := " "
		if violation.Inline {
			separator = "="
		}
		return fmt.Sprintf("remove %q from %s", violation.Item+separator+string(value), key)
	default:
		return fmt.Sprintf("remove %q from %s", violation.Item, key)
	}
}
