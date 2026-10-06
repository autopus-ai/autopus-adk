package harneval

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// AssertionFailure explains one failed (variant, assertion) evaluation. Index
// is the assertion position, or -1 when the variant surface was not generated.
type AssertionFailure struct {
	Variant string
	Index   int
	Detail  string
}

// TaskOutcome is the evaluation of one surface task.
type TaskOutcome struct {
	ID       string
	Passed   bool
	Failures []AssertionFailure
}

// EvaluateTask evaluates every assertion of the task on each of its variant
// surfaces, or on the default surface when it declares no variants. The task
// passes only when every (variant, assertion) evaluation passes.
func EvaluateTask(task Task, generation *Generation) TaskOutcome {
	variants := task.Variants
	if len(variants) == 0 {
		variants = []Variant{{}}
	}
	outcome := TaskOutcome{ID: task.ID}
	for _, variant := range variants {
		surface := generation.Surfaces[variantKey(variant.Overrides)]
		if surface == nil {
			outcome.Failures = append(outcome.Failures, AssertionFailure{Variant: variant.Name, Index: -1, Detail: "variant_not_generated"})
			continue
		}
		for index, assertion := range task.Assertions {
			if detail := evaluateAssertion(assertion, surface); detail != "" {
				outcome.Failures = append(outcome.Failures, AssertionFailure{Variant: variant.Name, Index: index, Detail: detail})
			}
		}
	}
	outcome.Passed = len(outcome.Failures) == 0
	return outcome
}

// evaluateAssertion returns "" when the assertion holds on the surface, else
// a short failure detail free of temp paths.
func evaluateAssertion(a Assertion, surface *Surface) string {
	if a.Kind == AssertSectionParity {
		return evaluateSectionParity(a, surface)
	}
	data, problem := surfaceFile(surface, a.Platform, a.Path)
	if a.Kind == AssertFileAbsent {
		if problem == "" {
			return "present"
		}
		return ""
	}
	if problem != "" {
		return problem
	}
	switch a.Kind {
	case AssertContains:
		return failUnless(bytes.Contains(data, []byte(a.Needle)), "needle_absent")
	case AssertNotContains:
		return failUnless(!bytes.Contains(data, []byte(a.Needle)), "needle_present")
	case AssertJSONPathPresent, AssertJSONPathAbsent:
		return evaluateJSONPath(a, data)
	case AssertRouteDetail:
		return evaluateRouteDetail(a, data, surface)
	}
	return ""
}

func failUnless(ok bool, detail string) string {
	if ok {
		return ""
	}
	return detail
}

// surfaceFile reads a file the platform's adapter generated. A path the
// adapter did not report is not on that platform's surface, even when another
// platform wrote it.
func surfaceFile(surface *Surface, platform, rel string) ([]byte, string) {
	if !surface.Owned[platform][rel] {
		return nil, "not_generated"
	}
	full := filepath.Join(surface.Root, filepath.FromSlash(rel))
	info, err := os.Lstat(full)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "missing"
	}
	if err != nil || !info.Mode().IsRegular() {
		return nil, "not_regular"
	}
	data, err := os.ReadFile(full)
	if err != nil {
		return nil, "unreadable"
	}
	return data, ""
}

func evaluateJSONPath(a Assertion, data []byte) string {
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return "json_invalid"
	}
	segments, err := parseJSONPath(a.JSONPath)
	if err != nil {
		return "json_path_invalid"
	}
	present := false
	for _, value := range resolveJSONPath(doc, segments) {
		if a.ValueContains == nil {
			present = true
			break
		}
		if text, ok := value.(string); ok && strings.Contains(text, *a.ValueContains) {
			present = true
			break
		}
	}
	if a.Kind == AssertJSONPathPresent {
		return failUnless(present, "json_path_absent")
	}
	return failUnless(!present, "json_path_present")
}

func evaluateRouteDetail(a Assertion, data []byte, surface *Surface) string {
	found := false
	for _, line := range splitLines(data) {
		if strings.Contains(line, a.Route) && strings.Contains(line, a.Detail) {
			found = true
			break
		}
	}
	if !found {
		return "route_absent"
	}
	if _, problem := surfaceFile(surface, a.Platform, a.Detail); problem != "" {
		return "detail_" + problem
	}
	return ""
}

func evaluateSectionParity(a Assertion, surface *Surface) string {
	var first string
	for index, file := range a.Files {
		data, problem := surfaceFile(surface, file.Platform, file.Path)
		if problem != "" {
			return problem
		}
		body, ok := sectionBody(data, a.Heading)
		if !ok {
			return "heading_missing"
		}
		if index == 0 {
			first = body
		} else if body != first {
			return "section_differs"
		}
	}
	return ""
}

// splitLines splits content into lines without a phantom final empty line.
func splitLines(data []byte) []string {
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

// sectionBody returns the lines after the first line equal to heading (both
// with trailing whitespace removed) up to the next heading of the same or a
// higher level. Lines inside fenced code blocks are never headings.
func sectionBody(data []byte, heading string) (string, bool) {
	level := headingLevel(heading)
	lines := splitLines(data)
	start := -1
	var fence markdownFence
	for index, raw := range lines {
		line := strings.TrimRight(raw, " \t\r")
		if !fence.step(line) && line == heading {
			start = index
			break
		}
	}
	if start < 0 {
		return "", false
	}
	var body []string
	fence = markdownFence{}
	for _, raw := range lines[start+1:] {
		line := strings.TrimRight(raw, " \t\r")
		if !fence.step(line) {
			if lineLevel := headingLevel(line); lineLevel > 0 && lineLevel <= level {
				break
			}
		}
		body = append(body, line)
	}
	return strings.Join(body, "\n"), true
}

// headingLevel returns the ATX heading level of a line, or 0 when the line is
// not a heading.
func headingLevel(line string) int {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	if level == 0 || level > 6 || (level < len(line) && line[level] != ' ') {
		return 0
	}
	return level
}

// markdownFence tracks fenced code blocks opened by ``` or ~~~.
type markdownFence struct{ marker string }

// step consumes one line and reports whether it is fenced content or a fence
// delimiter, which can never be a heading.
func (f *markdownFence) step(line string) bool {
	trimmed := strings.TrimLeft(line, " ")
	for _, marker := range []string{"```", "~~~"} {
		if strings.HasPrefix(trimmed, marker) {
			if f.marker == "" {
				f.marker = marker
				return true
			}
			if f.marker == marker {
				f.marker = ""
				return true
			}
		}
	}
	return f.marker != ""
}
