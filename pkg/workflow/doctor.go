package workflow

import (
	"encoding/json"
	"fmt"
)

const (
	// RouteA is the model-agnostic deterministic workflow route.
	RouteA = "route_a"
	// RouteTeam is the deterministic team route whose planning phase pins Opus 5.5.
	RouteTeam = "route_team"

	// RouteAMinVersion preserves the original Dynamic Workflows compatibility
	// floor for the model-agnostic Route A.
	RouteAMinVersion = "2.1.154"
	// RouteTeamMinVersion is the first Claude Code release that recognizes the
	// fixed claude-opus-5-5 model used by Route Team.
	RouteTeamMinVersion = "2.1.280"

	// MinVersion is kept as the Route A compatibility floor for callers that use
	// the original route-agnostic EvaluateCapabilities API.
	MinVersion = RouteAMinVersion
)

const (
	// StatusAvailable / StatusUnavailable are the per-primitive probe states.
	StatusAvailable   = "available"
	StatusUnavailable = "unavailable"
	// OverallPass / OverallFail are the capability gate verdicts.
	OverallPass = "pass"
	OverallFail = "fail"
)

// RequiredPrimitives are hard-gated: any unavailable one fails the gate.
//
// parallel and isolation are required (not advisory) because the generated
// route_team workflow JS hard-depends on them: the implementation phase
// dispatches its executor fan-out through parallel(...) with each executor
// carrying isolation: 'worktree' (SPEC-HARNESS-WORKFLOW-FIDELITY-001 REQ-004).
// A runtime missing either primitive would pass an advisory-only gate and then
// crash mid-launch at the parallel(...) call — strictly worse than failing the
// gate up front and falling back to the safe Route A path. Gating them makes
// that failure fail-fast at the doctor boundary.
var RequiredPrimitives = []string{"claude", "agent", "schema", "phase", "parallel", "isolation"}

// AdvisoryPrimitives are probed and reported but never affect the verdict.
var AdvisoryPrimitives = []string{"budget", "agent-model-override"}

// Prober is the injectable capability-probe seam. The production implementation
// inspects the claude-code runtime; tests inject a fake.
type Prober interface {
	// Probe reports whether a named workflow primitive is available.
	Probe(primitive string) bool
	// Version returns the probed claude-code version string (dotted ints).
	Version() string
}

// PrimitiveStatus is a single probed primitive in the capability report.
// Gating is true for required primitives, false for advisory ones.
type PrimitiveStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Gating bool   `json:"gating"`
}

// CapabilityReport is the structured `auto workflow doctor` output.
type CapabilityReport struct {
	Route          string            `json:"route"`
	MinimumVersion string            `json:"minimum_version"`
	Primitives     []PrimitiveStatus `json:"primitives"`
	Version        string            `json:"version"`
	VersionOK      bool              `json:"version_ok"`
	Overall        string            `json:"overall"`
}

// EvaluateCapabilities probes required and advisory primitives plus the version
// pin for the original Route A contract. Call EvaluateCapabilitiesForRoute when
// the selected route is known.
//
// Named EvaluateCapabilities (not Evaluate) because the gate evaluator
// EvaluateGate shares this package; Go forbids two same-named funcs.
func EvaluateCapabilities(p Prober) CapabilityReport {
	report, _ := EvaluateCapabilitiesForRoute(p, RouteA)
	return report
}

// EvaluateCapabilitiesForRoute probes required and advisory primitives plus the
// selected route's version pin. Overall is "fail" iff any required primitive is
// unavailable OR the version is below that route's minimum. Advisory primitives
// are reported with Gating=false and never change Overall.
func EvaluateCapabilitiesForRoute(p Prober, route string) (CapabilityReport, error) {
	minimumVersion, err := MinimumVersionForRoute(route)
	if err != nil {
		return CapabilityReport{}, err
	}

	report := CapabilityReport{
		Route:          route,
		MinimumVersion: minimumVersion,
		Version:        p.Version(),
	}
	report.VersionOK = versionAtLeast(report.Version, minimumVersion)

	failed := !report.VersionOK

	for _, name := range RequiredPrimitives {
		status := StatusAvailable
		if !p.Probe(name) {
			status = StatusUnavailable
			failed = true
		}
		report.Primitives = append(report.Primitives, PrimitiveStatus{
			Name:   name,
			Status: status,
			Gating: true,
		})
	}

	for _, name := range AdvisoryPrimitives {
		status := StatusAvailable
		if !p.Probe(name) {
			status = StatusUnavailable
		}
		report.Primitives = append(report.Primitives, PrimitiveStatus{
			Name:   name,
			Status: status,
			Gating: false,
		})
	}

	report.Overall = OverallPass
	if failed {
		report.Overall = OverallFail
	}
	return report, nil
}

// MinimumVersionForRoute returns the Claude Code compatibility floor for a
// canonical workflow route and fails closed for unknown routes.
func MinimumVersionForRoute(route string) (string, error) {
	switch route {
	case RouteA:
		return RouteAMinVersion, nil
	case RouteTeam:
		return RouteTeamMinVersion, nil
	default:
		return "", fmt.Errorf("unknown workflow route %q", route)
	}
}

// EncodeJSON serializes the capability report for CLI stdout consumption.
func (r CapabilityReport) EncodeJSON() ([]byte, error) {
	return json.Marshal(r)
}
