package skillpolicy

import "slices"

// Check replays caller-authored expectations without invoking a model or tool.
func Check(policy Policy, cases Cases, directory string) (CheckResult, error) {
	if err := validateCases(cases); err != nil {
		return CheckResult{}, err
	}
	if err := validatePolicy(policy); err != nil {
		return CheckResult{}, err
	}
	result := CheckResult{SchemaVersion: "skill_policy_check.v1", Passed: true, Cases: []CaseResult{}}
	for _, replay := range cases.Cases {
		selection, err := Select(policy, replay.Task, directory)
		if err != nil {
			return CheckResult{}, err
		}
		expected := append([]string{}, replay.ExpectedSelected...)
		slices.Sort(expected)
		passed := slices.Equal(expected, selection.Selected)
		result.Passed = result.Passed && passed
		result.Cases = append(result.Cases, CaseResult{Name: replay.Name, Passed: passed, ExpectedSelected: expected, ActualSelected: selection.Selected})
	}
	return result, nil
}
