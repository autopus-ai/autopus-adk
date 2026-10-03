package scenario

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/journey"
)

// Result is the outcome of compiling a project's scenario set.
type Result struct {
	ProjectDir string     `json:"project_dir"`
	TestDir    string     `json:"test_dir"`
	ConfigRef  string     `json:"playwright_config,omitempty"`
	SpecDir    string     `json:"spec_dir"`
	Compiled   []Compiled `json:"compiled"`
	// ScreenMatrix is the coverage projection a Journey Pack should declare to
	// enforce these scenarios. It is reported rather than written: the generated
	// pack carries explanatory comments that a YAML round-trip would destroy.
	ScreenMatrix map[string][]map[string]any `json:"screen_matrix"`
	DryRun       bool                        `json:"dry_run"`
}

// Compile loads, validates, and renders every scenario in a project. Origins
// unset on a scenario are inherited from the Journey Pack it names, so a
// compiled spec can only navigate where the guard already allows.
func CompileProject(projectDir string, dryRun bool) (Result, error) {
	scenarios, err := LoadDir(projectDir)
	if err != nil {
		return Result{}, err
	}
	if len(scenarios) == 0 {
		return Result{}, invalid("", "qa_scenario_none", "no scenarios found under %s", DirRel)
	}
	if !FixtureExists(projectDir) {
		return Result{}, invalid("", "qa_scenario_fixture_missing",
			"capture fixture %s is missing: run `auto qa init` first", filepath.ToSlash(FixtureRel))
	}
	origins, err := journeyOrigins(projectDir)
	if err != nil {
		return Result{}, err
	}
	testDir, configRef := TestDir(projectDir)
	result := Result{
		ProjectDir:   projectDir,
		TestDir:      filepath.ToSlash(testDir),
		ConfigRef:    configRef,
		SpecDir:      filepath.ToSlash(filepath.Join(testDir, GeneratedDirName)),
		DryRun:       dryRun,
		ScreenMatrix: map[string][]map[string]any{},
	}
	specDir := SpecDir(projectDir)
	if !dryRun {
		if err := os.MkdirAll(specDir, 0o755); err != nil {
			return Result{}, err
		}
	}
	for _, item := range scenarios {
		known, ok := origins[item.Journey]
		if !ok {
			return Result{}, invalid(item.Path, "qa_scenario_journey_unknown",
				"journey %q is not a Journey Pack in this project", item.Journey)
		}
		specPath := SpecPath(projectDir, item.ID)
		fixture, err := FixtureImport(projectDir, specPath)
		if err != nil {
			return Result{}, err
		}
		body, steps, err := CompileWithMap(item, Options{Origin: known, FixtureImport: fixture})
		if err != nil {
			return Result{}, err
		}
		specRel := filepath.ToSlash(filepath.Join(result.SpecDir, item.ID+".spec.ts"))
		steps.SpecPath = specRel
		stepMap, err := marshalStepMap(steps)
		if err != nil {
			return Result{}, err
		}
		if !dryRun {
			if err := os.WriteFile(specPath, body, 0o644); err != nil {
				return Result{}, err
			}
			// The sidecar is rewritten with its spec on every compile, so a map
			// can never describe an older revision of the file next to it.
			if err := os.WriteFile(StepMapPath(specPath), stepMap, 0o644); err != nil {
				return Result{}, err
			}
		}
		result.Compiled = append(result.Compiled, Compiled{
			ScenarioID:  item.ID,
			SourcePath:  filepath.ToSlash(filepath.Join(DirRel, item.Path)),
			SpecPath:    specRel,
			StepMapPath: StepMapPath(specRel),
			Screens:     len(item.Screens),
			Steps:       countSteps(item),
			Bytes:       len(body),
		})
	}
	for id := range origins {
		if rows := ScreenMatrix(scenarios, id); len(rows) > 0 {
			result.ScreenMatrix[id] = rows
		}
	}
	return result, nil
}

func countSteps(s Scenario) int {
	total := 0
	for _, screen := range s.Screens {
		total += len(screen.Steps)
	}
	return total
}

// journeyOrigins maps every Journey Pack ID to its first allowed origin. Packs
// with no GUI policy are still listed, with an empty origin, so naming a
// non-GUI journey fails on the missing origin instead of on an unknown id.
func journeyOrigins(projectDir string) (map[string]string, error) {
	packs, err := journey.LoadDir(projectDir)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, pack := range packs {
		origin := ""
		for _, candidate := range pack.GUI.AllowedOrigins {
			if trimmed := strings.TrimRight(strings.TrimSpace(candidate), "/"); trimmed != "" {
				origin = trimmed
				break
			}
		}
		out[pack.ID] = origin
	}
	return out, nil
}
