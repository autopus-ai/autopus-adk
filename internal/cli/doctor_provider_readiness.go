package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/orchestra"
)

// Doctor provider readiness (SPEC-REVIEWRO-001 REQ-14): every distinct
// review-gate provider and the judge get the spec review status probes by
// default. The probes make no model call, so --provider-smoke stays the only
// doctor path that does.

// doctorReadinessCheck is one provider's readiness in doctor terms; every
// field is rendered from status tokens and is already redacted.
type doctorReadinessCheck struct {
	provider string
	// status is pass (ready), fail (not_ready), or skip (unknown, skipped).
	status   string
	token    string
	remedy   string
	warnings []string
}

// doctorReadinessProviders resolves each distinct review-gate provider and the
// judge without executing anything; probes need only names, backends, models.
func doctorReadinessProviders(cfg *config.HarnessConfig) []orchestra.ProviderConfig {
	if cfg == nil {
		return nil
	}
	names := mergeProviderNames(resolveSpecReviewProviderNames(cfg, false), []string{cfg.Spec.ReviewGate.Judge})
	if len(names) == 0 {
		return nil
	}
	return resolveProviders(&cfg.Orchestra, "review", names)
}

func collectDoctorReadiness(ctx context.Context, cfg *config.HarnessConfig) []doctorReadinessCheck {
	results := reviewReadinessProbe(ctx, doctorReadinessProviders(cfg), providerReadinessOptions{})
	checks := make([]doctorReadinessCheck, 0, len(results))
	for _, result := range results {
		check := doctorReadinessCheck{provider: redactReadinessText(result.Provider), status: "skip", token: result.Token()}
		switch result.Status {
		case providerReadinessReady:
			check.status = "pass"
		case providerReadinessNotReady:
			check.status = "fail"
			check.remedy = redactReadinessText(result.RunRemedy())
		}
		for _, warning := range result.Warnings {
			check.warnings = append(check.warnings, redactReadinessText(warning))
		}
		checks = append(checks, check)
	}
	return checks
}

// checkProviderReadinessText renders the Provider Readiness section and
// reports false when a review provider is not ready.
func checkProviderReadinessText(ctx context.Context, w io.Writer, cfg *config.HarnessConfig) bool {
	tui.SectionHeader(w, "Provider Readiness")
	allReady := true
	for _, check := range collectDoctorReadiness(ctx, cfg) {
		line := check.provider + " readiness: " + check.token
		switch check.status {
		case "pass":
			tui.OK(w, line)
		case "fail":
			tui.FAIL(w, line+" - "+check.remedy)
			allReady = false
		default:
			tui.SKIP(w, line)
		}
		for _, warning := range check.warnings {
			tui.Info(w, warning)
		}
	}
	if !allReady {
		tui.Bullet(w, providerReadinessAdvice)
	}
	return allReady
}

// collectProviderReadinessChecks maps readiness onto doctor.provider_readiness.*
// with the severity and report-status mapping of doctor.provider_transport.*.
func (r *doctorJSONReport) collectProviderReadinessChecks(ctx context.Context, cfg *config.HarnessConfig) {
	for _, check := range collectDoctorReadiness(ctx, cfg) {
		entry := jsonCheck{
			ID: "doctor.provider_readiness." + check.provider, Severity: "info", Status: check.status, Detail: check.token,
		}
		if len(check.warnings) > 0 {
			entry.Fields = map[string]string{"warning": strings.Join(check.warnings, "\n")}
		}
		if check.status == "fail" {
			entry.Severity = "error"
			entry.Detail = check.token + ": " + check.remedy
			r.status = jsonStatusWarn
			r.warnings = append(r.warnings, jsonMessage{
				Code:    "provider_unready",
				Message: fmt.Sprintf("%s provider is not ready: %s; %s", check.provider, check.token, check.remedy),
			})
		}
		r.checks = append(r.checks, entry)
	}
}
