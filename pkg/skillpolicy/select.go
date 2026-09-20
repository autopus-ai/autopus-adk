package skillpolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// Select uses exact declarations and file existence; it performs no inference.
func Select(policy Policy, task Task, directory string) (Result, error) {
	if err := validatePolicy(policy); err != nil {
		return Result{}, err
	}
	if err := validateTask(task); err != nil {
		return Result{}, err
	}
	directory = filepath.Clean(directory)
	info, err := os.Lstat(directory)
	if err != nil {
		return Result{}, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return Result{}, fmt.Errorf("project root must be a directory, not a symlink")
	}
	root, err := os.OpenRoot(directory)
	if err != nil {
		return Result{}, err
	}
	defer root.Close()
	result := Result{SchemaVersion: "skill_selection.v1", Selected: []string{}, Decisions: []Decision{}}
	candidates := append([]Candidate(nil), policy.Candidates...)
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].ID < candidates[j].ID })
	for _, candidate := range candidates {
		decision, err := evaluate(root, candidate, task)
		if err != nil {
			return Result{}, err
		}
		result.Decisions = append(result.Decisions, decision)
		if decision.Status == "selected" {
			result.Selected = append(result.Selected, candidate.ID)
		}
	}
	return result, nil
}

func evaluate(root *os.Root, candidate Candidate, task Task) (Decision, error) {
	decision := Decision{ID: candidate.ID, Status: "selected", Reasons: []string{}, Evidence: []Evidence{{Source: "declared", Key: "task.class", Value: task.Class}}}
	exclude := func(reason string) { decision.Status = "excluded"; decision.Reasons = append(decision.Reasons, reason) }
	if slices.Contains(candidate.ExcludedTaskClasses, task.Class) {
		exclude("task_class_excluded")
		return decision, nil
	}
	if !slices.Contains(candidate.AllowedTaskClasses, task.Class) {
		exclude("task_class_not_allowed")
		return decision, nil
	}
	for _, marker := range candidate.RequiredFiles {
		state, err := fileMarker(root, marker)
		if err != nil {
			return Decision{}, err
		}
		decision.Evidence = append(decision.Evidence, Evidence{Source: "local_file", Key: marker, Value: state})
		if state != "present" {
			exclude("required_file_" + state)
		}
	}
	capabilities := make([]string, 0, len(candidate.SupportedVersions))
	for capability := range candidate.SupportedVersions {
		capabilities = append(capabilities, capability)
	}
	sort.Strings(capabilities)
	for _, capability := range capabilities {
		version, present := task.DeclaredVersions[capability]
		if !present {
			if decision.Status != "excluded" {
				decision.Status = "unknown"
			}
			decision.Reasons = append(decision.Reasons, "version_missing:"+capability)
			decision.Evidence = append(decision.Evidence, Evidence{Source: "declared", Key: "version:" + capability, Value: "unknown"})
			continue
		}
		decision.Evidence = append(decision.Evidence, Evidence{Source: "declared", Key: "version:" + capability, Value: version})
		if !slices.Contains(candidate.SupportedVersions[capability], version) {
			exclude("version_incompatible:" + capability)
		}
	}
	if decision.Status == "selected" {
		decision.Reasons = append(decision.Reasons, "explicit_constraints_satisfied")
	}
	return decision, nil
}

func fileMarker(root *os.Root, path string) (string, error) {
	parts := strings.Split(path, "/")
	for index := range parts {
		info, err := root.Lstat(strings.Join(parts[:index+1], "/"))
		if os.IsNotExist(err) {
			return "missing", nil
		}
		if err != nil {
			return "", fmt.Errorf("inspect project marker: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "symlink", nil
		}
		if index < len(parts)-1 && !info.IsDir() {
			return "not_directory", nil
		}
		if index == len(parts)-1 && !info.Mode().IsRegular() {
			return "not_regular", nil
		}
	}
	return "present", nil
}
