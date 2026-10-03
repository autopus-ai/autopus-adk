package record

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/journey"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// maxSlug keeps a slug short enough that a "-NN" suffix still fits the
// 64-character scenario id limit.
const maxSlug = 56

var slugRun = regexp.MustCompile(`[^a-z0-9]+`)

// Options names the scenario a recording becomes.
type Options struct {
	ID           string
	Title        string
	Journey      string
	Origin       string
	RecordingRef string
}

// ToScenario converts a recording into a validated v2 recording scenario.
// Each goto opens a screen and the events after it become that screen's steps
// in recording order. A goto followed directly by another goto holds no step,
// so it adds no screen: the harness never invents an assertion to fill one.
func ToScenario(rec Recording, opts Options) (scenario.Scenario, error) {
	origin, err := parseOrigin(opts.Origin)
	if err != nil {
		return scenario.Scenario{}, err
	}
	if len(rec.Events) == 0 {
		return scenario.Scenario{}, failf(CodeEmpty, "the recording holds no supported step")
	}
	if first := rec.Events[0]; first.Action != ActionGoto {
		return scenario.Scenario{}, failf(CodeGotoMissing,
			"line %d: a recording must start with goto so its first screen has a path", first.Line)
	}
	var screens []scenario.Screen
	for _, event := range rec.Events {
		if event.Action == ActionGoto {
			path, err := originPath(event.URL, event.Line, origin)
			if err != nil {
				return scenario.Scenario{}, err
			}
			screens = append(screens, scenario.Screen{Path: path})
			continue
		}
		step, err := toStep(event, origin)
		if err != nil {
			return scenario.Scenario{}, err
		}
		last := &screens[len(screens)-1]
		last.Steps = append(last.Steps, step)
	}
	s := scenario.Scenario{
		SchemaVersion: scenario.SchemaVersionV2,
		ID:            strings.TrimSpace(opts.ID),
		Title:         strings.TrimSpace(opts.Title),
		Journey:       strings.TrimSpace(opts.Journey),
		Origin:        origin.String(),
		IntentSource:  scenario.IntentRecording,
		RecordingRef:  strings.TrimSpace(opts.RecordingRef),
	}
	if s.Title == "" {
		s.Title = "Recorded journey " + s.ID
	}
	used := map[string]bool{}
	for _, screen := range screens {
		if len(screen.Steps) > 0 {
			screen.ID = ScreenID(screen.Path, used, len(s.Screens)+1)
			s.Screens = append(s.Screens, screen)
		}
	}
	if len(s.Screens) == 0 {
		return scenario.Scenario{}, failf(CodeEmpty, "the recording navigates but holds no action or assertion")
	}
	if err := scenario.Validate(s); err != nil {
		return scenario.Scenario{}, err
	}
	return s, nil
}

func toStep(event Event, origin *url.URL) (scenario.Step, error) {
	step := scenario.Step{By: event.By, Ac: event.Ac}
	target := event.Target
	switch event.Action {
	case ActionClick:
		step.Click = &target
	case ActionCheck:
		step.Check = &target
	case ActionFill:
		step.Fill = &scenario.FillAction{Target: target, Value: event.Value, ValueEnv: event.ValueEnv}
	case ActionPress:
		step.Press = &scenario.PressAction{Target: target, Key: event.Key}
	case ActionSelect:
		step.Select = &scenario.SelectAction{Target: target, Option: event.Value}
	case "":
		switch event.Expect {
		case ExpectText:
			step.ExpectText = event.Value
		case ExpectTitle:
			step.ExpectTitle = event.Value
		case ExpectURL:
			path, err := originPath(event.URL, event.Line, origin)
			if err != nil {
				return scenario.Step{}, err
			}
			step.ExpectURL = path
		case ExpectRole:
			step.ExpectRole = &scenario.RoleTarget{Role: target.Role, Name: target.Name, Exact: target.Exact}
		default:
			return scenario.Step{}, failf(CodeUnsupportedLines, "line %d: unknown expectation %q", event.Line, event.Expect)
		}
		// REQ-16: an assertion an agent chose and tied to no criterion waits
		// for a person before it may guard anything.
		if event.By == scenario.ByAgent && step.Ac == "" {
			step.Confirm = scenario.ConfirmRequired
		}
	default:
		return scenario.Step{}, failf(CodeUnsupportedLines, "line %d: unknown action %q", event.Line, event.Action)
	}
	return step, nil
}

// originPath turns a goto or expected URL into the origin-relative path a
// screen or expect_url needs. An absolute URL on another origin is refused:
// the scenario would silently test a different site.
func originPath(raw string, line int, origin *url.URL) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "/") && !strings.HasPrefix(trimmed, "//") {
		return trimmed, nil
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", failf(CodeURLInvalid, "line %d: %q is neither an absolute http(s) URL nor an origin-relative path", line, raw)
	}
	if u.Scheme != origin.Scheme || !strings.EqualFold(u.Host, origin.Host) {
		return "", failf(CodeOriginMismatch, "line %d: %s is not on origin %s", line, trimmed, origin.String())
	}
	path := u.EscapedPath()
	if path == "" {
		path = "/"
	}
	if u.RawQuery != "" {
		path += "?" + u.RawQuery
	}
	if u.Fragment != "" {
		path += "#" + u.EscapedFragment()
	}
	return path, nil
}

func parseOrigin(raw string) (*url.URL, error) {
	trimmed := strings.TrimRight(strings.TrimSpace(raw), "/")
	if trimmed == "" {
		return nil, failf(CodeOriginMissing, "an origin is required: pass --origin or declare gui.allowed_origins in a Journey Pack")
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, failf(CodeOriginInvalid, "origin must be an absolute http(s) origin with no path, got %q", raw)
	}
	return &url.URL{Scheme: u.Scheme, Host: strings.ToLower(u.Host)}, nil
}

// CanonicalOrigin returns raw as scheme://host, lowercased and without a
// trailing slash, or an error when it is not a bare http(s) origin.
func CanonicalOrigin(raw string) (string, error) {
	u, err := parseOrigin(raw)
	if err != nil {
		return "", err
	}
	return u.String(), nil
}

// ResolveJourney picks the Journey Pack and origin a new candidate runs
// under. An explicit journey wins; otherwise the first pack whose
// gui.allowed_origins lists origin, otherwise the first pack with any GUI
// origin. An empty origin inherits the chosen pack's first allowed origin.
// allowed reports whether any pack lists the resolved origin.
func ResolveJourney(packs []journey.Pack, journeyID, origin string) (string, string, bool) {
	journeyID, origin = strings.TrimSpace(journeyID), strings.TrimSpace(origin)
	if chosen := choosePack(packs, journeyID, canonicalOrNone(origin)); chosen != nil {
		if journeyID == "" {
			journeyID = chosen.ID
		}
		if origin == "" && len(chosen.GUI.AllowedOrigins) > 0 {
			origin = chosen.GUI.AllowedOrigins[0]
		}
	}
	want := canonicalOrNone(origin)
	for _, pack := range packs {
		if want != "" && listsOrigin(pack, want) {
			return journeyID, origin, true
		}
	}
	return journeyID, origin, false
}

func choosePack(packs []journey.Pack, journeyID, origin string) *journey.Pack {
	var fallback *journey.Pack
	for i := range packs {
		pack := &packs[i]
		switch {
		case journeyID != "":
			if pack.ID == journeyID {
				return pack
			}
		case len(pack.GUI.AllowedOrigins) == 0:
		case origin == "" || listsOrigin(*pack, origin):
			return pack
		case fallback == nil:
			fallback = pack
		}
	}
	return fallback
}

func listsOrigin(pack journey.Pack, origin string) bool {
	for _, candidate := range pack.GUI.AllowedOrigins {
		if canonicalOrNone(candidate) == origin {
			return true
		}
	}
	return false
}

func canonicalOrNone(raw string) string {
	canonical, err := CanonicalOrigin(raw)
	if err != nil {
		return ""
	}
	return canonical
}

// Slug lowercases s into kebab-case of at most 56 characters.
func Slug(s string) string {
	slug := strings.Trim(slugRun.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(slug) > maxSlug {
		slug = strings.Trim(slug[:maxSlug], "-")
	}
	return slug
}

// ScreenID derives a unique screen id from a path, ignoring its query and
// fragment: "/" is home and "/a/b" is a-b. A path that leaves no slug falls
// back to s<n>. used collects the ids handed out so far.
func ScreenID(path string, used map[string]bool, n int) string {
	if cut := strings.IndexAny(path, "?#"); cut >= 0 {
		path = path[:cut]
	}
	base := Slug(path)
	switch {
	case strings.Trim(path, "/") == "":
		base = "home"
	case base == "":
		base = fmt.Sprintf("s%d", n)
	}
	id := base
	for suffix := 2; used[id]; suffix++ {
		id = fmt.Sprintf("%s-%d", base, suffix)
	}
	used[id] = true
	return id
}
