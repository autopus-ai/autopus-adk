package pipeline

import (
	"fmt"
	"sort"
	"strings"
)

func parallelPhaseGraph(phases []Phase) (map[PhaseID]int, error) {
	indices := make(map[PhaseID]int, len(phases))
	for i, phase := range phases {
		if strings.TrimSpace(string(phase.ID)) == "" {
			return nil, fmt.Errorf("parallel phase ID is empty")
		}
		if _, exists := indices[phase.ID]; exists {
			return nil, fmt.Errorf("duplicate parallel phase %s", phase.ID)
		}
		indices[phase.ID] = i
	}
	for _, phase := range phases {
		seen := map[PhaseID]bool{}
		for _, dep := range phase.DependsOn {
			if _, exists := indices[dep]; !exists {
				return nil, fmt.Errorf("phase %s: missing dependency %s", phase.ID, dep)
			}
			if dep == phase.ID || seen[dep] {
				return nil, fmt.Errorf("phase %s: self or duplicate dependency %s", phase.ID, dep)
			}
			seen[dep] = true
		}
	}
	states := make([]int, len(phases))
	var visit func(int) error
	visit = func(i int) error {
		if states[i] == 1 {
			return fmt.Errorf("parallel dependency cycle at %s", phases[i].ID)
		}
		if states[i] == 2 {
			return nil
		}
		states[i] = 1
		for _, dep := range phases[i].DependsOn {
			if err := visit(indices[dep]); err != nil {
				return err
			}
		}
		states[i] = 2
		return nil
	}
	for i := range phases {
		if err := visit(i); err != nil {
			return nil, err
		}
	}
	return indices, nil
}

func parallelDependencyPrompt(phase Phase, indices map[PhaseID]int, results []PhaseResult) string {
	dependencies := append([]PhaseID(nil), phase.DependsOn...)
	sort.Slice(dependencies, func(i, j int) bool { return taskIDLess(string(dependencies[i]), string(dependencies[j])) })
	var previous strings.Builder
	for _, dep := range dependencies {
		if previous.Len() > 0 {
			previous.WriteString("\n\n")
		}
		fmt.Fprintf(&previous, "Phase: %s\n%s", dep, results[indices[dep]].Output)
	}
	return buildRunnerPrompt(phase.ID, previous.String())
}
