package loop

import (
	"encoding/json"
	"fmt"
	"path"
	"reflect"
	"strings"
)

// runnerFiles configure how a project's tests run. A product or test fix
// that edits one could switch an oracle off instead of satisfying it.
var runnerFiles = map[string]bool{
	"pytest.ini": true, "setup.cfg": true, "tox.ini": true, "conftest.py": true,
	"Makefile": true, "makefile": true, "GNUmakefile": true, "cypress.json": true,
}

// runnerFilePrefixes are base-name prefixes of runner configs in any
// extension (.js, .ts, .mjs, .json, .yml, ...).
var runnerFilePrefixes = []string{"jest.config.", "vitest.config.", "vitest.workspace.", "karma.conf.", "cypress.config.", ".mocharc", "mocha."}

// packageRunnerKeys are the package.json entries that decide what `npm test`
// runs and which tests the runner sees.
var packageRunnerKeys = []string{"scripts", "jest", "mocha", "ava"}

// runnerViolation rejects a product or test fix that rewires the test runner.
// package.json and pyproject.toml carry product settings too, so only their
// runner entries are compared; every other runner config is off limits.
func runnerViolation(in GuardInput, rel string) string {
	base := path.Base(rel)
	if runnerFiles[base] || hasAnyPrefix(base, runnerFilePrefixes) {
		return "a repair may not edit test runner configuration (" + base + ")"
	}
	switch base {
	case "package.json":
		return packageViolation(in, rel)
	case "pyproject.toml":
		before, _ := read(in.Before, rel)
		after, _ := read(in.After, rel)
		if pytestSettings(before) != pytestSettings(after) {
			return "a repair may not change the pytest settings in pyproject.toml"
		}
	}
	return ""
}

func packageViolation(in GuardInput, rel string) string {
	old, err := runnerEntries(read(in.Before, rel))
	if err != nil {
		return "package.json at HEAD does not parse, so its test scripts cannot be compared: " + err.Error()
	}
	next, err := runnerEntries(read(in.After, rel))
	if err != nil {
		return "package.json does not parse after the fix: " + err.Error()
	}
	for _, key := range packageRunnerKeys {
		if !reflect.DeepEqual(old[key], next[key]) {
			return fmt.Sprintf("a repair may not change package.json %q, the test runner entry point", key)
		}
	}
	return ""
}

// runnerEntries decodes the runner keys of a package.json; an absent file
// has none.
func runnerEntries(body []byte, ok bool) (map[string]any, error) {
	entries := map[string]any{}
	if !ok {
		return entries, nil
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, err
	}
	for _, key := range packageRunnerKeys {
		entries[key] = doc[key]
	}
	return entries, nil
}

// pytestSettings returns the pyproject.toml lines that configure pytest:
// every [tool.pytest*] table, plus any other line naming pytest (a dotted key
// under [tool], a plugin pin).
func pytestSettings(body []byte) string {
	var b strings.Builder
	inTable := false
	for _, line := range strings.Split(string(body), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "[") {
			inTable = strings.HasPrefix(strings.TrimSpace(strings.Trim(trimmed, "[]")), "tool.pytest")
		}
		if inTable || strings.Contains(strings.ToLower(trimmed), "pytest") {
			b.WriteString(trimmed + "\n")
		}
	}
	return b.String()
}

func hasAnyPrefix(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
