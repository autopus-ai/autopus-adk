// Package scenario compiles project-authored user scenarios into runner specs.
//
// The harness never invents assertions. A Scenario is the project's own
// declaration of what a user sees; this package only translates it into the
// runner dialect so the declaration becomes executable evidence.
package scenario

import "strings"

// SchemaVersion is the read-only v1 schema. Unknown keys and unknown versions
// are rejected rather than ignored: a typo that silently dropped a screen would
// turn a scenario into a passing no-op.
const SchemaVersion = "qamesh.scenario.v1"

// SchemaVersionV2 adds action steps and intent provenance (SPEC-QALOOP-001).
// v1 stays a separate read-only dialect: a v1 file naming a v2 field is
// rejected rather than reinterpreted, so no existing scenario can start
// mutating a page because the schema grew.
const SchemaVersionV2 = "qamesh.scenario.v2"

// ScreenAnnotation is the Playwright annotation type the generated spec pushes
// so the capture producer can label each step with the screen it covers. The
// gui.screen_matrix oracle keys on that label.
const ScreenAnnotation = "autopus-screen"

// ExploreTag marks generated specs as read-only exploration. The gui-explore
// Journey Pack selects specs with `--grep @explore`, so the tag is what makes a
// compiled scenario reachable from the lane.
const ExploreTag = "@explore"

// JourneyTag marks a spec that drives the page. A spec with any action step
// carries it instead of ExploreTag, so the read-only lane never selects it.
const JourneyTag = "@journey"

// BaselineTag labels a spec whose expectations were recorded from the running
// app: a regression baseline, not a correctness proof.
const BaselineTag = "@baseline"

// Intent sources name where a v2 scenario's expected values came from. Intent
// owns the oracle, so every v2 scenario has to say which intent it encodes.
const (
	IntentAcceptance = "acceptance"
	IntentRecording  = "recording"
	IntentBaseline   = "baseline"
)

// ConfirmRequired marks an agent-authored assertion no person has confirmed.
// Promotion refuses the scenario until the step gains an ac or is accepted.
const ConfirmRequired = "required"

// Step provenance for recorded steps.
const (
	ByHuman = "human"
	ByAgent = "agent"
)

// Scenario is one user-facing journey through a GUI surface.
type Scenario struct {
	SchemaVersion string `yaml:"schema_version"`
	ID            string `yaml:"id"`
	Title         string `yaml:"title"`
	Journey       string `yaml:"journey"`
	Origin        string `yaml:"origin,omitempty"`
	// IntentSource, Spec, and RecordingRef exist only in v2; Validate rejects
	// them on a v1 scenario.
	IntentSource  string   `yaml:"intent_source,omitempty"`
	Spec          string   `yaml:"spec,omitempty"`
	RecordingRef  string   `yaml:"recording_ref,omitempty"`
	AcceptanceRef []string `yaml:"acceptance_refs,omitempty"`
	Screens       []Screen `yaml:"screens"`
	// Path is the file the scenario was loaded from. Set by the loader so
	// generated specs and error messages can cite their source.
	Path string `yaml:"-"`
}

// Screen is one addressable view plus the steps that run on it. Each screen
// compiles to exactly one runner test, which is what lets the screen matrix
// oracle count coverage per screen.
type Screen struct {
	ID    string `yaml:"id"`
	Path  string `yaml:"path"`
	Steps []Step `yaml:"steps"`
}

// Step is one assertion or, in v2, one action. Exactly one expect_* or action
// field may be set; ac, by, and confirm annotate the step and are v2-only.
//
// v1 has no navigation-mutating or input action: a v1 step naming click, fill,
// or press is rejected, so a compiled v1 scenario still cannot trip the
// gui-explore mutation guard. A v2 action compiles under @journey instead,
// which the gui-explore lane never selects.
type Step struct {
	ExpectTitle string       `yaml:"expect_title,omitempty"`
	ExpectURL   string       `yaml:"expect_url,omitempty"`
	ExpectText  string       `yaml:"expect_text,omitempty"`
	ExpectRole  *RoleTarget  `yaml:"expect_role,omitempty"`
	ExpectCount *CountTarget `yaml:"expect_count,omitempty"`

	Click   *Target       `yaml:"click,omitempty"`
	Fill    *FillAction   `yaml:"fill,omitempty"`
	Press   *PressAction  `yaml:"press,omitempty"`
	Check   *Target       `yaml:"check,omitempty"`
	Select  *SelectAction `yaml:"select,omitempty"`
	WaitURL string        `yaml:"wait_url,omitempty"`

	// Ac is the acceptance id the step proves. By records who authored a
	// recorded step, and Confirm marks an assertion awaiting a person.
	Ac      string `yaml:"ac,omitempty"`
	By      string `yaml:"by,omitempty"`
	Confirm string `yaml:"confirm,omitempty"`
}

// Action names the action a step performs by its YAML key, or "" for an
// assertion. wait_url counts as an action: the spec groups it with the steps
// that move the page, and a baseline may not contain it.
func (s Step) Action() string {
	switch {
	case s.Click != nil:
		return "click"
	case s.Fill != nil:
		return "fill"
	case s.Press != nil:
		return "press"
	case s.Check != nil:
		return "check"
	case s.Select != nil:
		return "select"
	case strings.TrimSpace(s.WaitURL) != "":
		return "wait_url"
	}
	return ""
}

// Kind classifies the step the way the step map and triage do.
func (s Step) Kind() string {
	if s.Action() != "" {
		return StepKindAction
	}
	return StepKindExpect
}

// IsV2 reports whether the scenario declares the v2 schema.
func (s Scenario) IsV2() bool {
	return strings.TrimSpace(s.SchemaVersion) == SchemaVersionV2
}

// HasActions reports whether any step drives the page, which is what decides
// between the @journey and @explore lanes.
func (s Scenario) HasActions() bool {
	for _, screen := range s.Screens {
		for _, step := range screen.Steps {
			if step.Action() != "" {
				return true
			}
		}
	}
	return false
}

// RoleTarget addresses an element through the accessibility tree. Only
// role-based addressing exists here: the generated pack declares
// `selector_strategy: role-first`, and a CSS escape hatch would let compiled
// specs contradict the strategy the pack advertises.
type RoleTarget struct {
	Role  string `yaml:"role"`
	Name  string `yaml:"name,omitempty"`
	Exact bool   `yaml:"exact,omitempty"`
}

// CountTarget asserts how many elements of a role exist, which is how a
// scenario states "the catalog lists three products" without naming each one.
type CountTarget struct {
	Role  string `yaml:"role"`
	Name  string `yaml:"name,omitempty"`
	Count int    `yaml:"count"`
}

// Target addresses the element a v2 action acts on. Exactly one locator kind
// is set: role (optionally narrowed by name), label, placeholder, text, or
// test_id; there is still no CSS escape hatch. Exact tightens name, label,
// placeholder, and text to a full match, which is what Playwright codegen
// emits for ambiguous text. Test ids already match exactly, and a nameless
// role has nothing to match, so Exact has no effect on either.
type Target struct {
	Role        string `yaml:"role,omitempty"`
	Name        string `yaml:"name,omitempty"`
	Exact       bool   `yaml:"exact,omitempty"`
	Label       string `yaml:"label,omitempty"`
	Placeholder string `yaml:"placeholder,omitempty"`
	Text        string `yaml:"text,omitempty"`
	TestID      string `yaml:"test_id,omitempty"`
}

// FillAction types into its target. Value and ValueEnv are mutually exclusive.
// ValueEnv names an environment variable read when the spec runs, so a
// credential never lives in a committed scenario or generated spec.
type FillAction struct {
	Target   `yaml:",inline"`
	Value    string `yaml:"value,omitempty"`
	ValueEnv string `yaml:"value_env,omitempty"`
}

// PressAction presses a key. With no locator it goes to the page keyboard,
// which is what a recorded "press Enter" after a fill means.
type PressAction struct {
	Target `yaml:",inline"`
	Key    string `yaml:"key"`
}

// SelectAction picks an option, by value or label, in its target.
type SelectAction struct {
	Target `yaml:",inline"`
	Option string `yaml:"option"`
}

// Compiled is one emitted spec file.
type Compiled struct {
	ScenarioID  string `json:"scenario_id"`
	SourcePath  string `json:"source_path"`
	SpecPath    string `json:"spec_path"`
	StepMapPath string `json:"step_map_path"`
	Screens     int    `json:"screens"`
	Steps       int    `json:"steps"`
	Bytes       int    `json:"bytes"`
}
