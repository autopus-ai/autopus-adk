package taskroute

import (
	"path"
	"slices"
	"strings"

	"github.com/insajin/autopus-adk/pkg/spec/gates"
)

// Decide recommends work structure only; it grants no gate waiver or spawn.
func Decide(f Facts) (Decision, error) {
	d := Decision{Version: 1, Route: "guided", Reasons: []string{}, RequiredSteps: []string{"scoped_verification", "honest_report", "preserve_existing_gates"}, SuggestedSkills: []string{}, Execution: "serial", ModelPolicy: "inherit", Advisory: true}
	paths, err := validate(f)
	if err != nil {
		return d, err
	}
	classification := gates.Classify(classificationPaths(paths), nil)
	sensitive := false
	for _, path := range paths {
		if sensitivePath(path) {
			sensitive = true
		}
	}
	positive := func(value *bool) bool { return value != nil && *value }
	inline := f.Risk == "low" && positive(f.ScopeComplete) && positive(f.RequirementsClear) && positive(f.AcceptanceKnown) && len(paths) > 0 && len(paths) <= 2 && f.EstimatedChangedLines != nil && *f.EstimatedChangedLines <= 80 && (f.Kind == "docs" || f.Kind == "bugfix") && f.FailedAttempts == 0
	if inline {
		d.Route = "inline"
		d.Reasons = append(d.Reasons, "bounded_low_risk_scope")
	}
	plan := func(reason string) { d.Route = "planned"; d.Reasons = append(d.Reasons, reason) }
	if len(classification.Roots) >= 2 {
		plan("multiple_domains")
	}
	if len(paths) >= 6 {
		plan("large_file_scope")
	}
	if f.EstimatedChangedLines != nil && *f.EstimatedChangedLines > 300 {
		plan("large_change_estimate")
	}
	if f.RequirementsClear != nil && !*f.RequirementsClear {
		plan("requirements_unclear")
	}
	if f.Risk == "high" || f.Risk == "critical" {
		plan("declared_high_risk")
	}
	if sensitive {
		plan("sensitive_path")
	}
	if f.FailedAttempts >= 2 {
		plan("repeated_failed_attempts")
	} else if f.FailedAttempts == 1 {
		d.Reasons = append(d.Reasons, "prior_failed_attempt")
	}
	if !positive(f.ScopeComplete) || len(paths) == 0 {
		d.Reasons = append(d.Reasons, "scope_incomplete")
		d.RequiredSteps = append(d.RequiredSteps, "inspect_scope")
	}
	if f.RequirementsClear == nil || !*f.RequirementsClear {
		d.RequiredSteps = append(d.RequiredSteps, "clarify_requirements")
	}
	if !positive(f.AcceptanceKnown) {
		d.RequiredSteps = append(d.RequiredSteps, "define_acceptance")
	}
	if f.EstimatedChangedLines == nil {
		d.Reasons = append(d.Reasons, "change_estimate_unknown")
	}
	if f.Risk == "" || f.Risk == "unknown" {
		d.Reasons = append(d.Reasons, "risk_unknown")
		d.RequiredSteps = append(d.RequiredSteps, "assess_risk")
	}
	rank := map[string]int{"inline": 0, "guided": 1, "planned": 2}
	if f.Requested != "" && f.Requested != "auto" {
		if rank[f.Requested] > rank[d.Route] {
			d.Route = f.Requested
			d.Reasons = append(d.Reasons, "explicit_higher_route")
		} else if rank[f.Requested] < rank[d.Route] {
			d.Reasons = append(d.Reasons, "requested_route_below_required")
		}
	}
	if sensitive || f.Risk == "high" || f.Risk == "critical" {
		d.RequiredSteps = append(d.RequiredSteps, "security_review", "integration_acceptance")
	}
	switch d.Route {
	case "inline":
		d.SuggestedSkills = []string{"verification"}
	case "guided":
		d.RequiredSteps = append(d.RequiredSteps, "focused_implementation")
		d.SuggestedSkills = []string{"debugging", "verification"}
	case "planned":
		d.RequiredSteps = append(d.RequiredSteps, "plan_scope_and_interfaces", "integration_acceptance", "review_changes")
		d.SuggestedSkills = []string{"planning", "verification"}
	}
	independent := true
	for _, worker := range f.Workers {
		if !worker.Independent {
			independent = false
		}
	}
	if len(f.Workers) >= 2 && !independent {
		d.Reasons = append(d.Reasons, "independence_unverified")
	}
	verifiedScope := positive(f.ScopeComplete) && positive(f.RequirementsClear) && positive(f.AcceptanceKnown) && workerScopeContained(f.Workers, paths)
	if len(f.Workers) >= 2 && !verifiedScope {
		d.Reasons = append(d.Reasons, "worker_scope_unverified")
	}
	overlap := ownershipOverlap(f.Workers)
	if overlap {
		d.Reasons = append(d.Reasons, "ownership_overlap")
	}
	if f.Solo {
		d.Reasons = append(d.Reasons, "explicit_solo")
	}
	if len(f.Workers) >= 2 && !f.NativeParallelAvailable {
		d.Reasons = append(d.Reasons, "native_parallel_unavailable")
	}
	if d.Route == "planned" && len(f.Workers) >= 2 && verifiedScope && independent && !overlap && f.NativeParallelAvailable && !f.Solo {
		d.Execution = "parallel"
		d.Reasons = append(d.Reasons, "independent_owned_work")
	}
	if len(d.Reasons) == 0 {
		d.Reasons = append(d.Reasons, "guided_scope")
	}
	d.RequiredSteps = dedup(d.RequiredSteps)
	return d, nil
}
func dedup(values []string) []string {
	out := []string{}
	for _, v := range values {
		if !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
func sensitivePath(path string) bool {
	if gates.IsSecurityPath(path) {
		return true
	}
	for _, part := range strings.FieldsFunc(strings.ToLower(path), func(r rune) bool { return r == '/' || r == '_' || r == '-' || r == '.' }) {
		if slices.Contains([]string{"finance", "financial", "payment", "payments", "billing", "refund", "refunds", "deploy", "deployment", "credentials", "credential", "routing", "authorization", "authentication", "secrets"}, part) {
			return true
		}
	}
	return false
}
func ownershipOverlap(workers []Worker) bool {
	for i, a := range workers {
		for _, left := range a.OwnedPaths {
			for _, b := range workers[i+1:] {
				for _, right := range b.OwnedPaths {
					l, _ := cleanPath(left)
					r, _ := cleanPath(right)
					if l == r || strings.HasPrefix(l, r+"/") || strings.HasPrefix(r, l+"/") {
						return true
					}
				}
			}
		}
	}
	return false
}

// Containment checks declarations only, never filesystem or symlink ownership.
func workerScopeContained(workers []Worker, parents []string) bool {
	for _, worker := range workers {
		for _, owned := range worker.OwnedPaths {
			normalized, _ := cleanPath(owned)
			contained := false
			for _, parent := range parents {
				if normalized == parent || strings.HasPrefix(normalized, parent+"/") {
					contained = true
					break
				}
			}
			if !contained {
				return false
			}
		}
	}
	return true
}

// Extensionless two-segment grants under generic containers denote module
// directories for classification only; original paths still govern scope/counts.
func classificationPaths(paths []string) []string {
	result := append([]string(nil), paths...)
	for i, value := range result {
		parts := strings.Split(value, "/")
		if len(parts) == 2 && path.Ext(parts[1]) == "" && slices.Contains([]string{"pkg", "internal", "src", "cmd", "app"}, parts[0]) {
			result[i] = value + "/__taskroute_scope__"
		}
	}
	return result
}
