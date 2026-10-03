// Package discover crawls an app read-only and turns what it saw into a v2
// baseline candidate scenario (SPEC-QALOOP-001 REQ-17). A baseline records
// what the pages show today: it is a regression baseline, not a correctness
// proof, and every artifact this package writes says so.
package discover

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/record"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// SchemaVersion is the crawler output dialect Decode reads.
const SchemaVersion = "qamesh.discover.v1"

// DefaultID names the baseline candidate when no id is given.
const DefaultID = "discovered-baseline"

// Notice is the label every discovery artifact and command output carries.
const Notice = "regression baseline, not a correctness proof: it records what the app shows today, not what it should show"

// BaselineTitle is the scenario title of every discovered baseline.
const BaselineTitle = "Discovered baseline (regression baseline, not a correctness proof)"

// maxHeadingSteps caps heading assertions per screen; the first headings
// identify a page, the rest mostly track content churn.
const maxHeadingSteps = 3

//go:embed crawler.cjs
var crawlerScript string

// CrawlerScript returns the self-contained CommonJS crawler `auto qa discover`
// runs with node. It resolves Playwright from the project, visits same-origin
// links with page.goto only, and prints one qamesh.discover.v1 document.
func CrawlerScript() string { return crawlerScript }

// Page is what the crawler saw on one page.
type Page struct {
	Path      string   `json:"path"`
	Title     string   `json:"title"`
	Headings  []string `json:"headings"`
	Landmarks []string `json:"landmarks"`
}

// Crawl is the crawler's whole output.
type Crawl struct {
	SchemaVersion string `json:"schema_version"`
	Origin        string `json:"origin"`
	Pages         []Page `json:"pages"`
}

// BaselineOptions names the baseline scenario. An empty Origin falls back to
// the crawl's origin, and an empty ID to DefaultID.
type BaselineOptions struct {
	ID      string
	Journey string
	Origin  string
}

// SkippedPage is a crawled page that could not become a screen.
type SkippedPage struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

// Decode reads the crawler's stdout. The document is normally the whole
// output; when a library printed a warning first, the final line is used.
func Decode(stdout []byte) (Crawl, error) {
	body := bytes.TrimSpace(stdout)
	crawl, err := decodeCrawl(body)
	if err != nil {
		if cut := bytes.LastIndexByte(body, '\n'); cut >= 0 {
			crawl, err = decodeCrawl(body[cut+1:])
		}
	}
	switch {
	case err != nil:
		return Crawl{}, &record.Error{Code: CodeDecodeInvalid,
			Message: "crawler output is not a " + SchemaVersion + " document: " + err.Error()}
	case crawl.SchemaVersion != SchemaVersion:
		return Crawl{}, &record.Error{Code: CodeDecodeInvalid,
			Message: fmt.Sprintf("crawler output declares schema_version %q, want %s", crawl.SchemaVersion, SchemaVersion)}
	case len(crawl.Pages) == 0:
		return Crawl{}, &record.Error{Code: CodeNoPages, Message: "the crawler reached no page on " + crawl.Origin}
	}
	return crawl, nil
}

func decodeCrawl(body []byte) (Crawl, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var crawl Crawl
	err := decoder.Decode(&crawl)
	return crawl, err
}

// ToBaseline turns a crawl into a validated v2 baseline scenario with one
// screen per page. Each screen asserts the page title and up to three h1/h2
// headings and never acts, which keeps the baseline on the read-only lane.
func ToBaseline(crawl Crawl, opts BaselineOptions) (scenario.Scenario, error) {
	s, _, err := baseline(crawl, opts)
	return s, err
}

func baseline(crawl Crawl, opts BaselineOptions) (scenario.Scenario, []SkippedPage, error) {
	origin := strings.TrimSpace(opts.Origin)
	if origin == "" {
		origin = crawl.Origin
	}
	canonical, err := record.CanonicalOrigin(origin)
	if err != nil {
		return scenario.Scenario{}, nil, err
	}
	s := scenario.Scenario{SchemaVersion: scenario.SchemaVersionV2, ID: strings.TrimSpace(opts.ID), Title: BaselineTitle,
		Journey: strings.TrimSpace(opts.Journey), Origin: canonical, IntentSource: scenario.IntentBaseline}
	if s.ID == "" {
		s.ID = DefaultID
	}
	var skipped []SkippedPage
	used, seen := map[string]bool{}, map[string]bool{}
	for _, page := range crawl.Pages {
		steps, reason := pageSteps(page)
		if reason == "" && seen[page.Path] {
			reason = "duplicate path"
		}
		if reason != "" {
			skipped = append(skipped, SkippedPage{Path: page.Path, Reason: reason})
			continue
		}
		seen[page.Path] = true
		s.Screens = append(s.Screens, scenario.Screen{
			ID: record.ScreenID(page.Path, used, len(s.Screens)+1), Path: page.Path, Steps: steps})
	}
	if len(s.Screens) == 0 {
		return scenario.Scenario{}, skipped, &record.Error{Code: CodeNoPages,
			Message: "no crawled page had a usable path and a title or heading to assert"}
	}
	if err := scenario.Validate(s); err != nil {
		return scenario.Scenario{}, skipped, err
	}
	return s, skipped, nil
}

// pageSteps returns a page's assertions, or the reason it cannot be a screen.
func pageSteps(page Page) ([]scenario.Step, string) {
	if !validPath(page.Path) {
		return nil, "path cannot be a screen path"
	}
	var steps []scenario.Step
	if title := strings.TrimSpace(page.Title); title != "" {
		steps = append(steps, scenario.Step{ExpectTitle: title})
	}
	named := map[string]bool{}
	for _, heading := range page.Headings {
		name := strings.Join(strings.Fields(heading), " ")
		if name == "" || named[name] {
			continue
		}
		named[name] = true
		steps = append(steps, scenario.Step{ExpectRole: &scenario.RoleTarget{Role: "heading", Name: name}})
		if len(named) == maxHeadingSteps {
			break
		}
	}
	if len(steps) == 0 {
		return nil, "no title or heading to assert"
	}
	return steps, ""
}

// validPath mirrors the scenario screen path rule, so one odd URL skips its
// page instead of failing the whole baseline.
func validPath(path string) bool {
	return strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") &&
		!strings.ContainsAny(path, " \t\"'`\\") && !strings.Contains(path, "..")
}
