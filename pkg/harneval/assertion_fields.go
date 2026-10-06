package harneval

import (
	"slices"
	"sort"
)

// assertionReads returns the (platform, path) surfaces one assertion reads.
func assertionReads(a Assertion) []FileRef {
	switch a.Kind {
	case AssertSectionParity:
		return append([]FileRef(nil), a.Files...)
	case AssertRouteDetail:
		return []FileRef{{Platform: a.Platform, Path: a.Path}, {Platform: a.Platform, Path: a.Detail}}
	default:
		return []FileRef{{Platform: a.Platform, Path: a.Path}}
	}
}

func uniqueSorted(task Task, pick func(FileRef) string) []string {
	seen := map[string]bool{}
	var values []string
	for _, assertion := range task.Assertions {
		for _, read := range assertionReads(assertion) {
			if value := pick(read); !seen[value] {
				seen[value] = true
				values = append(values, value)
			}
		}
	}
	sort.Strings(values)
	return values
}

// TaskPlatforms returns the sorted unique platforms a task's assertions read.
func TaskPlatforms(task Task) []string {
	return uniqueSorted(task, func(read FileRef) string { return read.Platform })
}

// TaskPaths returns the sorted unique paths a task's assertions read.
func TaskPaths(task Task) []string {
	return uniqueSorted(task, func(read FileRef) string { return read.Path })
}

// IsMulti reports whether a task asserts two or more platforms or two or more
// paths, the breadth REQ-HE-13 counts toward its 60 percent floor.
func IsMulti(task Task) bool {
	return len(TaskPlatforms(task)) >= 2 || len(TaskPaths(task)) >= 2
}

// assertionFields lists the fields each assertion kind uses. Every listed
// field except value_contains is required, and an unlisted field must be empty.
var assertionFields = map[string][]string{
	AssertFileExists:      {"platform", "path"},
	AssertFileAbsent:      {"platform", "path"},
	AssertContains:        {"platform", "path", "needle"},
	AssertNotContains:     {"platform", "path", "needle"},
	AssertJSONPathPresent: {"platform", "path", "json_path", "value_contains"},
	AssertJSONPathAbsent:  {"platform", "path", "json_path", "value_contains"},
	AssertRouteDetail:     {"platform", "path", "route", "detail"},
	AssertSectionParity:   {"files", "heading"},
}

const optionalAssertionField = "value_contains"

func setAssertionFields(a Assertion) []string {
	var set []string
	add := func(name string, present bool) {
		if present {
			set = append(set, name)
		}
	}
	add("platform", a.Platform != "")
	add("path", a.Path != "")
	add("needle", a.Needle != "")
	add("json_path", a.JSONPath != "")
	add("value_contains", a.ValueContains != nil)
	add("route", a.Route != "")
	add("detail", a.Detail != "")
	add("files", a.Files != nil)
	add("heading", a.Heading != "")
	return set
}

// validateAssertion enforces the closed kind set and each kind's field shape.
func validateAssertion(a Assertion) error {
	allowed, known := assertionFields[a.Kind]
	if !known {
		return invalidf(DetailUnknownAssertionKind, "unknown assertion kind %q", a.Kind)
	}
	set := setAssertionFields(a)
	for _, name := range set {
		if !slices.Contains(allowed, name) {
			return invalidf(DetailAssertionFieldInvalid, "%s does not use %s", a.Kind, name)
		}
	}
	for _, name := range allowed {
		if name != optionalAssertionField && !slices.Contains(set, name) {
			return invalidf(DetailAssertionFieldInvalid, "%s requires %s", a.Kind, name)
		}
	}
	return validateAssertionValues(a)
}

func validateAssertionValues(a Assertion) error {
	if a.Kind == AssertSectionParity {
		return validateSectionParity(a)
	}
	if !isPlatform(a.Platform) {
		return invalidf(DetailAssertionFieldInvalid, "unknown platform %q", a.Platform)
	}
	if !isCleanRelPath(a.Path) {
		return invalidf(DetailAssertionFieldInvalid, "path %q is not a clean relative path", a.Path)
	}
	switch a.Kind {
	case AssertJSONPathPresent, AssertJSONPathAbsent:
		if _, err := parseJSONPath(a.JSONPath); err != nil {
			return &InvalidError{Detail: DetailAssertionFieldInvalid, Err: err}
		}
		if a.ValueContains != nil && *a.ValueContains == "" {
			return invalidf(DetailAssertionFieldInvalid, "value_contains must not be empty")
		}
	case AssertRouteDetail:
		if blank(a.Route) || !isCleanRelPath(a.Detail) {
			return invalidf(DetailAssertionFieldInvalid, "route_detail needs a route and a clean detail path")
		}
	}
	return nil
}

func validateSectionParity(a Assertion) error {
	if len(a.Files) < 2 {
		return invalidf(DetailAssertionFieldInvalid, "section_parity needs at least two files")
	}
	seen := make(map[FileRef]bool, len(a.Files))
	for _, file := range a.Files {
		if !isPlatform(file.Platform) || !isCleanRelPath(file.Path) || seen[file] {
			return invalidf(DetailAssertionFieldInvalid, "section_parity file %v is unknown, unclean, or repeated", file)
		}
		seen[file] = true
	}
	if !headingPattern.MatchString(a.Heading) {
		return invalidf(DetailAssertionFieldInvalid, "heading %q is not a Markdown ATX heading", a.Heading)
	}
	return nil
}
