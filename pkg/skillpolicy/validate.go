package skillpolicy

import (
	"fmt"
	"io/fs"
	"strings"
	"unicode"
)

func validName(value string) bool {
	if value == "" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func validList(values []string, required bool) bool {
	if len(values) > 256 || (required && len(values) == 0) {
		return false
	}
	seen := map[string]bool{}
	for _, value := range values {
		if !validName(value) || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func validMarker(path string) bool {
	return len(path) <= 512 && fs.ValidPath(path) && path != "." && !strings.ContainsAny(path, "\\:\x00")
}

func validatePolicy(policy Policy) error {
	if policy.SchemaVersion != PolicySchema || len(policy.Candidates) == 0 || len(policy.Candidates) > 256 {
		return fmt.Errorf("invalid policy schema or candidate count")
	}
	seen := map[string]bool{}
	for _, candidate := range policy.Candidates {
		if !validName(candidate.ID) || seen[candidate.ID] {
			return fmt.Errorf("invalid or duplicate candidate ID")
		}
		seen[candidate.ID] = true
		if !validList(candidate.AllowedTaskClasses, true) || !validList(candidate.ExcludedTaskClasses, false) {
			return fmt.Errorf("invalid task class policy for %q", candidate.ID)
		}
		if len(candidate.RequiredFiles) > 256 || len(candidate.SupportedVersions) > 128 {
			return fmt.Errorf("candidate constraints exceed limit")
		}
		for _, marker := range candidate.RequiredFiles {
			if !validMarker(marker) {
				return fmt.Errorf("invalid project-relative file marker")
			}
		}
		for capability, versions := range candidate.SupportedVersions {
			if !validName(capability) || !validList(versions, true) {
				return fmt.Errorf("invalid exact version policy")
			}
		}
	}
	return nil
}

func validateTask(task Task) error {
	if task.SchemaVersion != TaskSchema || !validName(task.Class) || len(task.DeclaredVersions) > 128 {
		return fmt.Errorf("invalid task schema or class")
	}
	for capability, version := range task.DeclaredVersions {
		if !validName(capability) || !validName(version) {
			return fmt.Errorf("invalid declared version")
		}
	}
	return nil
}

func validateCases(cases Cases) error {
	if cases.SchemaVersion != CasesSchema || len(cases.Cases) == 0 || len(cases.Cases) > 256 {
		return fmt.Errorf("invalid replay schema or case count")
	}
	seen := map[string]bool{}
	for _, replay := range cases.Cases {
		if !validName(replay.Name) || seen[replay.Name] || replay.ExpectedSelected == nil || !validList(replay.ExpectedSelected, false) {
			return fmt.Errorf("invalid replay name or expected_selected")
		}
		seen[replay.Name] = true
		if err := validateTask(replay.Task); err != nil {
			return err
		}
	}
	return nil
}
