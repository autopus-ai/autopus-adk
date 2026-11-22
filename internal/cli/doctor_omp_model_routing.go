package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/insajin/autopus-adk/pkg/adapter/omp"
)

func appendOMPModelRoutingDoctorChecks(
	checks []jsonCheck,
	report omp.OMPModelDoctorReport,
) []jsonCheck {
	if !report.Enabled {
		return checks
	}
	checks = append(checks, projectOMPModelRoutingDoctorChecks(report)...)
	sort.SliceStable(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })
	return checks
}

func projectOMPModelRoutingDoctorChecks(report omp.OMPModelDoctorReport) []jsonCheck {
	if !report.Enabled {
		return nil
	}
	severity, status := ompModelDoctorCheckLevel(report.Status)
	checks := []jsonCheck{{
		ID:       "doctor.platform.omp.model-routing.receipt",
		Severity: severity,
		Status:   status,
		Detail: fmt.Sprintf("routing status=%s reason=%s profile=%s receipt=%s",
			ompDoctorToken(report.Status), ompDoctorReason(report.Reason),
			ompModelDoctorSafeToken(report.Profile), ompModelDoctorSafeToken(report.ReceiptStatus)),
	}}
	for _, role := range report.Roles {
		roleSeverity, roleStatus := ompModelDoctorCheckLevel(role.Status)
		identity := ompModelDoctorSafeToken(role.Agent) + "\x00" + ompModelDoctorSafeToken(role.Role)
		checks = append(checks, jsonCheck{
			ID:       "doctor.platform.omp.model-routing.role." + ompDoctorDigest(identity),
			Severity: roleSeverity,
			Status:   roleStatus,
			Detail: fmt.Sprintf(
				"agent=%s role=%s capability=%s status=%s reason=%s family_diversity=%s family_reason=%s evidence=availability quorum=false independent_provider=false",
				ompModelDoctorSafeToken(role.Agent), ompModelDoctorSafeToken(role.Role),
				ompModelDoctorSafeToken(role.Capability),
				ompDoctorToken(role.Status), ompDoctorReason(role.Reason),
				ompModelDoctorSafeToken(role.FamilyDiversity),
				ompDoctorReason(role.FamilyReason),
			),
		})
	}
	sort.SliceStable(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })
	return checks
}

func ompModelDoctorSafeToken(value string) string {
	value = ompDoctorToken(value)
	lower := strings.ToLower(value)
	for _, marker := range []string{"secret", "token", "password", "bearer", "api_key", "api-key", "provider"} {
		if strings.Contains(lower, marker) {
			return "redacted"
		}
	}
	return value
}

func ompModelDoctorCheckLevel(value string) (string, string) {
	switch value {
	case "supported":
		return "info", "pass"
	case "degraded":
		return "warning", "warn"
	case omp.OMPReaSessionModelStatus:
		// SPEC-REA-001 S11: running on the session model is the intended
		// fallback for a REA role, not a blocked route.
		return "info", "pass"
	default:
		return "error", "fail"
	}
}
