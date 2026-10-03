package scenario

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// StepMapSchemaVersion versions the sidecar written next to each compiled spec.
const StepMapSchemaVersion = "qamesh.stepmap.v1"

// Step kinds recorded in a step map.
const (
	StepKindGoto   = "goto"
	StepKindAction = "action"
	StepKindExpect = "expect"
)

// StepMap ties each emitted step line of a compiled spec back to the scenario
// step it came from. Triage depends on it to tell drift from a product bug: a
// failing action line means the page changed shape, while a failing expect
// line means the product broke the oracle.
type StepMap struct {
	SchemaVersion string `json:"schema_version"`
	ScenarioID    string `json:"scenario_id"`
	Spec          string `json:"spec,omitempty"`
	IntentSource  string `json:"intent_source,omitempty"`
	// SpecPath is the project-relative, slash-separated spec the map describes.
	SpecPath string `json:"spec_path,omitempty"`
	// Lines is keyed by the 1-based spec line number in decimal, because JSON
	// object keys are strings. Header and scaffolding lines are absent.
	Lines map[string]StepRef `json:"lines"`
}

// StepRef locates one emitted step. Index is the 1-based position in the
// screen's steps, matching validation messages; the screen's implicit goto
// has Index 0.
type StepRef struct {
	Screen string `json:"screen"`
	Index  int    `json:"index"`
	Kind   string `json:"kind"`
	Ac     string `json:"ac,omitempty"`
}

func newStepMap(s Scenario) StepMap {
	return StepMap{
		SchemaVersion: StepMapSchemaVersion,
		ScenarioID:    strings.TrimSpace(s.ID),
		Spec:          strings.TrimSpace(s.Spec),
		IntentSource:  strings.TrimSpace(s.IntentSource),
		Lines:         map[string]StepRef{},
	}
}

func (m *StepMap) add(line int, ref StepRef) {
	m.Lines[strconv.Itoa(line)] = ref
}

// Lookup returns the step emitted on a 1-based spec line.
func (m StepMap) Lookup(line int) (StepRef, bool) {
	ref, ok := m.Lines[strconv.Itoa(line)]
	return ref, ok
}

// StepMapPath is the sidecar path for a compiled spec path. The name keeps
// Playwright's default testMatch from ever collecting it as a test file.
func StepMapPath(specPath string) string {
	return strings.TrimSuffix(specPath, ".spec.ts") + ".spec.map.json"
}

func marshalStepMap(m StepMap) ([]byte, error) {
	body, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(body, '\n'), nil
}

// LoadStepMap reads a sidecar strictly. A map that decodes loosely could send
// triage to the wrong step, so unknown fields, a foreign schema, non-numeric
// line keys, and unknown kinds are all refused.
func LoadStepMap(path string) (StepMap, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return StepMap{}, err
	}
	name := filepath.Base(path)
	var loaded StepMap
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&loaded); err != nil {
		return StepMap{}, invalid(name, "qa_scenario_stepmap_invalid", "%s", err.Error())
	}
	if loaded.SchemaVersion != StepMapSchemaVersion {
		return StepMap{}, invalid(name, "qa_scenario_stepmap_invalid", "schema_version must be %q", StepMapSchemaVersion)
	}
	if loaded.Lines == nil {
		loaded.Lines = map[string]StepRef{}
	}
	for key, ref := range loaded.Lines {
		if line, err := strconv.Atoi(key); err != nil || line < 1 || strconv.Itoa(line) != key {
			return StepMap{}, invalid(name, "qa_scenario_stepmap_invalid", "line key %q is not a positive line number", key)
		}
		switch ref.Kind {
		case StepKindGoto, StepKindAction, StepKindExpect:
		default:
			return StepMap{}, invalid(name, "qa_scenario_stepmap_invalid", "line %s has unknown kind %q", key, ref.Kind)
		}
	}
	return loaded, nil
}
