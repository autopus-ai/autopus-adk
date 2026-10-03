package scenario

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// ariaRoles is the closed set of roles a scenario may address. Playwright
// accepts any string and fails at runtime on an unknown role; validating here
// turns that into an authoring-time error with a citable file.
var ariaRoles = map[string]bool{
	"alert": true, "alertdialog": true, "application": true, "article": true,
	"banner": true, "blockquote": true, "button": true, "caption": true,
	"cell": true, "checkbox": true, "code": true, "columnheader": true,
	"combobox": true, "complementary": true, "contentinfo": true,
	"definition": true, "deletion": true, "dialog": true, "directory": true,
	"document": true, "emphasis": true, "feed": true, "figure": true,
	"form": true, "generic": true, "grid": true, "gridcell": true,
	"group": true, "heading": true, "img": true, "insertion": true,
	"link": true, "list": true, "listbox": true, "listitem": true,
	"log": true, "main": true, "marquee": true, "math": true, "menu": true,
	"menubar": true, "menuitem": true, "menuitemcheckbox": true,
	"menuitemradio": true, "meter": true, "navigation": true, "none": true,
	"note": true, "option": true, "paragraph": true, "presentation": true,
	"progressbar": true, "radio": true, "radiogroup": true, "region": true,
	"row": true, "rowgroup": true, "rowheader": true, "scrollbar": true,
	"search": true, "searchbox": true, "separator": true, "slider": true,
	"spinbutton": true, "status": true, "strong": true, "subscript": true,
	"superscript": true, "switch": true, "tab": true, "table": true,
	"tablist": true, "tabpanel": true, "term": true, "textbox": true,
	"time": true, "timer": true, "toolbar": true, "tooltip": true,
	"tree": true, "treegrid": true, "treeitem": true,
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// maxSteps bounds a single screen so one scenario cannot generate an unbounded
// spec. The limit is generous; hitting it means the scenario should be split.
const maxSteps = 60

// ValidationError names the offending file so the CLI can print an actionable
// message instead of a decode dump.
type ValidationError struct {
	Path    string
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Path == "" {
		return e.Message
	}
	return e.Path + ": " + e.Message
}

func invalid(path, code, format string, args ...any) error {
	return &ValidationError{Path: path, Code: code, Message: fmt.Sprintf(format, args...)}
}

// Validate rejects anything the compiler cannot turn into a deterministic spec.
func Validate(s Scenario) error {
	version := strings.TrimSpace(s.SchemaVersion)
	if version != SchemaVersion && version != SchemaVersionV2 {
		return invalid(s.Path, "qa_scenario_schema_version", "schema_version must be %q or %q", SchemaVersion, SchemaVersionV2)
	}
	v2 := version == SchemaVersionV2
	// A v2 key in a v1 file is reported before anything else: it is the one
	// mistake that would otherwise read as a decode or shape error.
	if !v2 {
		if err := rejectV2Fields(s); err != nil {
			return err
		}
	}
	if !idPattern.MatchString(strings.TrimSpace(s.ID)) {
		return invalid(s.Path, "qa_scenario_id_invalid", "id must be lowercase kebab-case, got %q", s.ID)
	}
	if strings.TrimSpace(s.Title) == "" {
		return invalid(s.Path, "qa_scenario_title_missing", "title is required")
	}
	if strings.TrimSpace(s.Journey) == "" {
		return invalid(s.Path, "qa_scenario_journey_missing", "journey is required: name the Journey Pack that runs this scenario")
	}
	if err := validateOrigin(s); err != nil {
		return err
	}
	if len(s.Screens) == 0 {
		return invalid(s.Path, "qa_scenario_screens_missing", "at least one screen is required")
	}
	seen := map[string]bool{}
	for _, screen := range s.Screens {
		if err := validateScreen(s.Path, screen, seen, v2); err != nil {
			return err
		}
	}
	if v2 {
		return validateIntent(s)
	}
	return nil
}

func validateOrigin(s Scenario) error {
	raw := strings.TrimSpace(s.Origin)
	if raw == "" {
		return nil
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return invalid(s.Path, "qa_scenario_origin_invalid", "origin must be an absolute http(s) origin, got %q", s.Origin)
	}
	// A credentialed or query-carrying origin would be baked verbatim into a
	// generated file that projects commit, so it is rejected by shape.
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || strings.Trim(parsed.Path, "/") != "" {
		return invalid(s.Path, "qa_scenario_origin_invalid", "origin must carry no path, query, fragment, or credentials")
	}
	return nil
}

func validateScreen(path string, screen Screen, seen map[string]bool, v2 bool) error {
	id := strings.TrimSpace(screen.ID)
	if !idPattern.MatchString(id) {
		return invalid(path, "qa_scenario_screen_id_invalid", "screen id must be lowercase kebab-case, got %q", screen.ID)
	}
	if seen[id] {
		return invalid(path, "qa_scenario_screen_duplicate", "duplicate screen id %q", id)
	}
	seen[id] = true
	if err := validateScreenPath(path, id, screen.Path); err != nil {
		return err
	}
	if len(screen.Steps) == 0 {
		return invalid(path, "qa_scenario_steps_missing", "screen %q needs at least one step", id)
	}
	if len(screen.Steps) > maxSteps {
		return invalid(path, "qa_scenario_steps_overflow", "screen %q declares %d steps, limit is %d", id, len(screen.Steps), maxSteps)
	}
	for index, step := range screen.Steps {
		if err := validateStep(path, id, index, step, v2); err != nil {
			return err
		}
	}
	return nil
}

func validateScreenPath(path, id, value string) error {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "//") {
		return invalid(path, "qa_scenario_screen_path_invalid", "screen %q path must be origin-relative and start with a single '/', got %q", id, value)
	}
	// The path is concatenated onto the origin in generated source, so anything
	// that could escape the origin or the string literal is refused up front.
	if strings.ContainsAny(trimmed, " \t\"'`\\") || strings.Contains(trimmed, "..") {
		return invalid(path, "qa_scenario_screen_path_invalid", "screen %q path contains an unsupported character", id)
	}
	return nil
}

func validateStep(path, screenID string, index int, step Step, v2 bool) error {
	set := 0
	for _, populated := range []bool{
		strings.TrimSpace(step.ExpectTitle) != "",
		strings.TrimSpace(step.ExpectURL) != "",
		strings.TrimSpace(step.ExpectText) != "",
		step.ExpectRole != nil,
		step.ExpectCount != nil,
		// Always zero on v1: rejectV2Fields refused every action key already.
		step.Click != nil, step.Fill != nil, step.Press != nil,
		step.Check != nil, step.Select != nil,
		strings.TrimSpace(step.WaitURL) != "",
	} {
		if populated {
			set++
		}
	}
	where := fmt.Sprintf("screen %q step %d", screenID, index+1)
	if set == 0 {
		if v2 {
			return invalid(path, "qa_scenario_step_empty", "%s declares no action or assertion", where)
		}
		return invalid(path, "qa_scenario_step_empty", "%s declares no assertion", where)
	}
	if set > 1 {
		if v2 {
			return invalid(path, "qa_scenario_step_ambiguous", "%s declares %d actions or assertions; use one step per action or assertion", where, set)
		}
		return invalid(path, "qa_scenario_step_ambiguous", "%s declares %d assertions; use one step per assertion", where, set)
	}
	if err := validateExpect(path, where, step); err != nil {
		return err
	}
	if !v2 {
		return nil
	}
	if err := validateRoleNames(path, where, step); err != nil {
		return err
	}
	if err := validateAction(path, where, step); err != nil {
		return err
	}
	return validateAnnotations(path, where, step)
}

func validateExpect(path, where string, step Step) error {
	if step.ExpectRole != nil {
		return validateRole(path, where, step.ExpectRole.Role)
	}
	if step.ExpectCount != nil {
		if step.ExpectCount.Count < 0 {
			return invalid(path, "qa_scenario_step_count_invalid", "%s count must not be negative", where)
		}
		return validateRole(path, where, step.ExpectCount.Role)
	}
	if url := strings.TrimSpace(step.ExpectURL); url != "" && !strings.HasPrefix(url, "/") {
		return invalid(path, "qa_scenario_step_url_invalid", "%s expect_url must be origin-relative, got %q", where, url)
	}
	return nil
}

func validateRole(path, where, role string) error {
	normalized := strings.ToLower(strings.TrimSpace(role))
	if normalized == "" {
		return invalid(path, "qa_scenario_step_role_missing", "%s requires a role", where)
	}
	if !ariaRoles[normalized] {
		return invalid(path, "qa_scenario_step_role_unknown", "%s role %q is not an ARIA role", where, role)
	}
	return nil
}
