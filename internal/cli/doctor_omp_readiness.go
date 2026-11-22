package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/insajin/autopus-adk/internal/cli/tui"
	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/adapter/omp"
)

// @AX:NOTE [AUTO]: OMP 18 RPC state and command frames exceed 64 KiB; 256 KiB bounds the provider-free transcript.
const (
	ompDoctorProbeTimeout = 3 * time.Second
	ompDoctorTotalTimeout = 20 * time.Second
	ompDoctorMaxOutput    = 256 * 1024
)

var (
	ompDoctorSafeToken = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._/-]{0,127}$`)
	ompDoctorVersion   = regexp.MustCompile(`^omp/[0-9]+\.[0-9]+\.[0-9]+$`)
	ompDoctorAbsPath   = regexp.MustCompile(`(^|[[:space:]=])/(?:[^[:space:],\]]+/)*[^[:space:],\]:]+`)
)

func projectOMPDoctorChecks(
	findings []adapter.ValidationError,
	report omp.OMPReadinessReport,
) []jsonCheck {
	checks := make([]jsonCheck, 0, len(findings)+len(report.Capabilities)+len(report.SelectorResolutions)+1)
	for _, finding := range findings {
		detail := ompDoctorValidationDetail(finding)
		severity, status := ompDoctorLevel(finding.Level)
		identity := severity + "\x00" + ompDoctorFindingFile(finding.File) + "\x00" + detail
		checks = append(checks, jsonCheck{
			ID:       "doctor.platform.omp.validation." + ompDoctorDigest(identity),
			Severity: severity,
			Status:   status,
			Detail:   detail,
		})
	}

	for _, capability := range report.Capabilities {
		severity, status := "info", "pass"
		if !capability.Supported {
			severity, status = "error", "fail"
		}
		detail := fmt.Sprintf("capability=%s supported=%t reason=%s",
			ompDoctorToken(capability.ID), capability.Supported, ompDoctorReason(capability.Reason))
		if capability.ID == "identity.version" && ompDoctorVersion.MatchString(report.Version) {
			detail += " version=" + report.Version
		}
		checks = append(checks, jsonCheck{
			ID:       "doctor.platform.omp.capability." + capability.ID,
			Severity: severity,
			Status:   status,
			Detail:   detail,
		})
	}

	catalogReady := report.CatalogReason == "catalog_ready"
	catalogSeverity, catalogStatus := "info", "pass"
	if !catalogReady && len(report.SelectorResolutions) == 0 {
		catalogStatus = "skip"
	} else if !catalogReady {
		catalogSeverity, catalogStatus = "error", "fail"
	}
	checks = append(checks, jsonCheck{
		ID:       "doctor.platform.omp.catalog",
		Severity: catalogSeverity,
		Status:   catalogStatus,
		Detail:   "catalog reason=" + ompDoctorReason(report.CatalogReason),
	})

	for _, resolution := range report.SelectorResolutions {
		severity, status := "info", "pass"
		if resolution.Reason == "credential_unavailable" {
			severity, status = "warning", "warn"
		} else if resolution.Status != "resolved" {
			severity, status = "error", "fail"
		}
		digest := ompDoctorDigest(resolution.Selector)
		checks = append(checks, jsonCheck{
			ID:       "doctor.platform.omp.selector." + digest,
			Severity: severity,
			Status:   status,
			Detail: fmt.Sprintf("selector=%s status=%s reason=%s",
				digest, ompDoctorToken(resolution.Status), ompDoctorReason(resolution.Reason)),
		})
	}

	sort.SliceStable(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })
	return checks
}

func renderOMPDoctorChecksText(out io.Writer, checks []jsonCheck) {
	for _, check := range checks {
		detail := check.ID + ": " + check.Detail
		switch check.Status {
		case "fail":
			tui.FAIL(out, detail)
		case "warn":
			tui.SKIP(out, detail)
		default:
			tui.OK(out, detail)
		}
	}
}

func renderOMPDoctorReadinessText(
	ctx context.Context,
	out io.Writer,
	root string,
	findings []adapter.ValidationError,
) (healthy, failed bool) {
	checks := probeAndProjectOMPDoctorChecks(ctx, root, findings)
	renderOMPDoctorChecksText(out, checks)
	return ompDoctorChecksHealthy(checks), ompDoctorChecksFailed(checks)
}

func (r *doctorJSONReport) collectOMPReadinessChecks(
	ctx context.Context,
	dir string,
	findings []adapter.ValidationError,
	payload doctorPlatformPayload,
) {
	checks := probeAndProjectOMPDoctorChecks(ctx, dir, findings)
	payload.Valid = ompDoctorChecksHealthy(checks)
	for _, finding := range findings {
		severity, _ := ompDoctorLevel(finding.Level)
		payload.Messages = append(payload.Messages, doctorMessagePayload{
			Level: severity, Message: ompDoctorValidationDetail(finding),
		})
	}
	r.checks = append(r.checks, checks...)
	if !payload.Valid {
		r.status = jsonStatusWarn
	}
	r.data.Platforms = append(r.data.Platforms, payload)
}

func probeAndProjectOMPDoctorChecks(
	ctx context.Context,
	root string,
	findings []adapter.ValidationError,
) []jsonCheck {
	if ctx == nil {
		ctx = context.Background()
	}
	probeCtx, cancel := context.WithTimeout(ctx, ompDoctorTotalTimeout)
	defer cancel()
	collection := collectOMPDoctorSelectors(root)
	report := omp.ProbeOMPReadiness(probeCtx, omp.OMPReadinessOptions{
		Root: root, Timeout: ompDoctorProbeTimeout, MaxOutput: ompDoctorMaxOutput,
		Selectors: collection.selectors,
	})
	allFindings := append(append([]adapter.ValidationError(nil), findings...), collection.findings...)
	checks := projectOMPDoctorChecks(allFindings, report)
	checks = appendOMPModelRoutingDoctorChecks(checks, probeOMPModelRoutingDoctor(probeCtx, root))
	return appendOMPContextDoctorChecks(checks, probeOMPContextDoctor(probeCtx, root))
}

func ompDoctorChecksHealthy(checks []jsonCheck) bool {
	for _, check := range checks {
		if check.Status == "fail" || check.Status == "warn" {
			return false
		}
	}
	return true
}

// ompDoctorChecksFailed reports whether any check failed outright, as
// opposed to a degraded check that only warns.
func ompDoctorChecksFailed(checks []jsonCheck) bool {
	for _, check := range checks {
		if check.Status == "fail" {
			return true
		}
	}
	return false
}

func ompDoctorLevel(level string) (string, string) {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "error":
		return "error", "fail"
	case "warn", "warning":
		return "warning", "warn"
	default:
		return "info", "pass"
	}
}

func ompDoctorValidationDetail(finding adapter.ValidationError) string {
	detail := strings.TrimSpace(finding.Message)
	file := strings.TrimSpace(finding.File)
	for _, prefix := range []string{file + ":", file} {
		if file != "" && strings.HasPrefix(detail, prefix) {
			detail = strings.TrimSpace(strings.TrimPrefix(detail, prefix))
		}
	}
	detail = ompDoctorAbsPath.ReplaceAllString(detail, "$1<path>")
	if detail == "" {
		return "validation failed"
	}
	return detail
}

func ompDoctorFindingFile(path string) string {
	path = filepath.ToSlash(filepath.Clean(strings.TrimSpace(path)))
	if path == "." || path == "" {
		return "unknown"
	}
	if filepath.IsAbs(path) {
		return filepath.Base(path)
	}
	return path
}

// @AX:NOTE [AUTO] @AX:SPEC: SPEC-OMP-002: unknown reasons are deliberately collapsed to "redacted" so
// provider output, filesystem paths, and unstable subprocess errors cannot become doctor receipt fields.
func ompDoctorReason(value string) string {
	value = strings.TrimSpace(value)
	allowed := map[string]bool{
		"observed": true, "version_verified": true, "flag_present": true,
		"output_valid": true, "event_observed": true, "available": true,
		"event_observed_partial_timeout":      true,
		"event_observed_partial_exit_nonzero": true,
		"flag_missing":                        true, "output_invalid": true, "event_missing": true,
		"timeout": true, "exit_nonzero": true, "output_oversized": true,
		"catalog_ready": true, "catalog_empty": true, "catalog_invalid": true,
		"catalog_timeout": true, "catalog_oversized": true, "catalog_exit_nonzero": true,
		"credential_unavailable": true, "selector_malformed": true, "selector_unresolved": true,
		"identity_unverified": true, "behavioral_probe_required": true,
		"fresh": true, "version_stale": true, "catalog_stale": true,
		"projection_mismatch": true, "receipt_missing": true, "receipt_invalid": true,
		"catalog_metadata_insufficient": true, "role_degraded": true, "role_blocked": true,
		"selected": true, "explicit_degraded": true, "no_compatible_candidate": true,
		"same_family_only": true, "family_unknown": true,
		"not_applicable": true,
	}
	if allowed[value] {
		return value
	}
	return "redacted"
}

func ompDoctorToken(value string) string {
	value = strings.TrimSpace(value)
	if ompDoctorSafeToken.MatchString(value) {
		return value
	}
	return "redacted"
}

func ompDoctorDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}
