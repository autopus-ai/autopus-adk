// Package triage assigns one deterministic class to each failed QA journey so
// the autonomous loop knows what it may fix and what it must leave alone.
//
// The class decides the repair boundary, not the repair itself: a product
// defect may only be fixed in product code, a drifted locator only in the
// scenario's action steps. Classification therefore reads evidence the
// harness produced (status, setup gaps, runner output, the compiler's step
// map) and never asks an agent.
package triage

import (
	"fmt"

	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// Class is the triage verdict for one failed journey.
type Class string

const (
	// ClassEnvironment means the run could not exercise the product: a setup
	// gap, a refused connection, a missing browser, a missing env variable.
	ClassEnvironment Class = "environment"
	// ClassFlaky means the journey passed on an immediate re-run.
	ClassFlaky Class = "flaky"
	// ClassTestDrift means the failing line is an action step: the test could
	// not reach an element, so its locator may be stale.
	ClassTestDrift Class = "test_drift"
	// ClassTestDefect means the test itself is broken (syntax, type, module,
	// strict-mode ambiguity).
	ClassTestDefect Class = "test_defect"
	// ClassProductDefect means an expectation the intent states did not hold.
	ClassProductDefect Class = "product_defect"
	// ClassUnknown means no rule matched; the loop stops rather than guess.
	ClassUnknown Class = "unknown"
)

// Input is everything Classify reads for one journey.
type Input struct {
	JourneyID string
	Adapter   string
	// Status is the adapter result status: failed, blocked, or skipped.
	Status string
	// SetupGapCode is non-empty when the adapter reported a setup gap.
	SetupGapCode string
	// FailureText is the bounded tail of the runner output.
	FailureText string
	// ProjectDir resolves spec paths cited in FailureText to step maps.
	ProjectDir string
	// RerunPassed is nil when no re-run happened, otherwise its outcome.
	RerunPassed *bool
}

// Verdict is the class plus the evidence that produced it.
type Verdict struct {
	JourneyID string `json:"journey_id"`
	Class     Class  `json:"class"`
	// Signal names the matched rule and the evidence fragment, for example
	// `stepmap:action e2e/autopus-generated/login.spec.ts:29`.
	Signal string `json:"signal"`
	// SpecPath and Line locate the failing generated-spec line when known.
	SpecPath string `json:"spec_path,omitempty"`
	Line     int    `json:"line,omitempty"`
	// Step is the step-map entry for Line when the spec was compiled by the
	// harness.
	Step *scenario.StepRef `json:"step,omitempty"`
	// ScenarioID is the scenario the failing spec was compiled from.
	ScenarioID string `json:"scenario_id,omitempty"`
}

// Fingerprint identifies a failure for no-progress detection: the same
// journey failing the same way at the same step of the same spec after a fix
// means the fix did nothing. The step index is part of it, so a fix that
// moves the failure to another step on the same screen counts as progress;
// the line is not, because a recompile can shift it without any progress.
func (v Verdict) Fingerprint() string {
	step := ""
	if v.Step != nil {
		step = fmt.Sprintf("%s#%d#%s", v.Step.Screen, v.Step.Index, v.Step.Kind)
	}
	return v.JourneyID + "|" + string(v.Class) + "|" + v.SpecPath + "|" + step
}
