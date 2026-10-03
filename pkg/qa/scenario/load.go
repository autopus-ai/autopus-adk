package scenario

import (
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// DirRel is the project-local directory scenarios are authored in.
var DirRel = filepath.Join(".autopus", "qa", "scenarios")

// CandidatesDirRel holds generated or recorded scenarios awaiting promotion.
// LoadDir does not recurse, so a candidate is never compiled or run until
// promotion moves it into DirRel.
var CandidatesDirRel = filepath.Join(DirRel, "candidates")

// Dir returns the scenario directory for a project.
func Dir(projectDir string) string {
	return filepath.Join(projectDir, DirRel)
}

// CandidatesDir returns the candidate directory for a project.
func CandidatesDir(projectDir string) string {
	return filepath.Join(projectDir, CandidatesDirRel)
}

// LoadDir reads every scenario in deterministic order. It fails closed: one
// invalid scenario stops the whole compile, because a partially compiled set
// would silently ship fewer assertions than the project declared.
func LoadDir(projectDir string) ([]Scenario, error) {
	return loadGlob(Dir(projectDir))
}

// LoadCandidates reads the candidate directory with LoadDir's strictness, so a
// candidate that would fail to compile after promotion fails here first. A
// missing directory is an empty set, not an error.
func LoadCandidates(projectDir string) ([]Scenario, error) {
	return loadGlob(CandidatesDir(projectDir))
}

func loadGlob(dir string) ([]Scenario, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	out := make([]Scenario, 0, len(paths))
	ids := map[string]string{}
	for _, path := range paths {
		loaded, err := LoadFile(path)
		if err != nil {
			return nil, err
		}
		if prior, clash := ids[loaded.ID]; clash {
			return nil, invalid(path, "qa_scenario_id_duplicate", "id %q already declared by %s", loaded.ID, prior)
		}
		ids[loaded.ID] = filepath.Base(path)
		out = append(out, loaded)
	}
	return out, nil
}

// LoadFile decodes one scenario with unknown keys rejected. A silently ignored
// key is how a misspelled assertion becomes an always-green test.
func LoadFile(path string) (Scenario, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, err
	}
	return ParseBytes(filepath.Base(path), body)
}

// ParseBytes decodes and validates a scenario held in memory, such as an
// agent's proposal, with exactly the strictness LoadFile applies on disk. Agent
// output is never trusted more than a file a person wrote. name becomes the
// scenario's Path and prefixes errors.
func ParseBytes(name string, body []byte) (Scenario, error) {
	var loaded Scenario
	decoder := yaml.NewDecoder(bytes.NewReader(body))
	decoder.KnownFields(true)
	if err := decoder.Decode(&loaded); err != nil {
		return Scenario{}, invalid(name, "qa_scenario_parse_invalid", "%s", err.Error())
	}
	loaded.Path = name
	loaded.ID = strings.TrimSpace(loaded.ID)
	loaded.Journey = strings.TrimSpace(loaded.Journey)
	loaded.Origin = strings.TrimRight(strings.TrimSpace(loaded.Origin), "/")
	loaded.IntentSource = strings.TrimSpace(loaded.IntentSource)
	loaded.Spec = strings.TrimSpace(loaded.Spec)
	loaded.RecordingRef = strings.TrimSpace(loaded.RecordingRef)
	if err := Validate(loaded); err != nil {
		return Scenario{}, err
	}
	return loaded, nil
}
