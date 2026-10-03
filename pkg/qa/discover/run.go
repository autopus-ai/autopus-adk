package discover

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/insajin/autopus-adk/pkg/qa/journey"
	"github.com/insajin/autopus-adk/pkg/qa/record"
)

// Stable error codes. Node or Playwright missing is a setup gap.
const (
	CodeOriginNotAllowed  = "qa_discover_origin_not_allowed"
	CodeOriginMissing     = "qa_discover_origin_missing"
	CodeJourneyMissing    = "qa_discover_journey_missing"
	CodeNodeMissing       = "qa_discover_node_missing"
	CodePlaywrightMissing = "qa_discover_playwright_missing"
	CodeCrawlFailed       = "qa_discover_crawl_failed"
	CodeDecodeInvalid     = "qa_discover_decode_invalid"
	CodeNoPages           = "qa_discover_no_pages"
)

// DefaultMaxPages is the crawl budget when none is given.
const DefaultMaxPages = 20

// maxPagesLimit bounds a crawl so a typo cannot walk a whole site.
const maxPagesLimit = 200

// playwrightMissingExit is the exit code the crawler uses when it cannot
// resolve Playwright from the project.
const playwrightMissingExit = 3

// Runner runs name with args in dir, writing the child's stdout to stdout,
// and waits for it to exit. Run uses node by default; tests inject a fake.
type Runner func(ctx context.Context, dir string, stdout io.Writer, name string, args ...string) error

// Options configures one discovery run.
type Options struct {
	Origin string
	// Explicit marks Origin as named by the person (the --origin flag), which
	// is what lets it reach an origin no Journey Pack allows.
	Explicit bool
	MaxPages int
	Journey  string
	ID       string
	Exec     Runner
}

// Result describes the baseline candidate a run wrote.
type Result struct {
	Path    string        `json:"path"`
	Created bool          `json:"created"`
	ID      string        `json:"id"`
	Journey string        `json:"journey"`
	Origin  string        `json:"origin"`
	Pages   int           `json:"pages"`
	Screens int           `json:"screens"`
	Steps   int           `json:"steps"`
	Skipped []SkippedPage `json:"skipped,omitempty"`
	Notice  string        `json:"notice"`
}

// Run crawls the origin read-only with the embedded crawler and writes one
// baseline candidate (REQ-17). The origin must be allowed by a Journey Pack
// unless Explicit is set.
func Run(ctx context.Context, projectDir string, opts Options) (Result, error) {
	journeyID, origin, err := target(projectDir, opts)
	if err != nil {
		return Result{}, err
	}
	stdout, err := crawl(ctx, projectDir, origin, clampPages(opts.MaxPages), opts.Exec)
	if err != nil {
		return Result{}, err
	}
	decoded, err := Decode(stdout)
	if err != nil {
		return Result{}, err
	}
	s, skipped, err := baseline(decoded, BaselineOptions{ID: opts.ID, Journey: journeyID, Origin: origin})
	if err != nil {
		return Result{Skipped: skipped, Notice: Notice}, err
	}
	path, created, err := record.WriteCandidate(projectDir, s, []string{
		"Discovered " + Notice + ".",
		fmt.Sprintf("Crawled read-only from %s (page.goto on same-origin links only), %d page(s).", s.Origin, len(decoded.Pages)),
		"Review every expectation before promoting: a page that is wrong today is recorded as expected.",
	})
	if err != nil {
		return Result{}, err
	}
	result := Result{Path: path, Created: created, ID: s.ID, Journey: s.Journey, Origin: s.Origin,
		Pages: len(decoded.Pages), Screens: len(s.Screens), Skipped: skipped, Notice: Notice}
	for _, screen := range s.Screens {
		result.Steps += len(screen.Steps)
	}
	return result, nil
}

// target resolves the journey and origin and enforces the origin policy: a
// crawl reaches only an origin some Journey Pack allows, unless the person
// named it explicitly.
func target(projectDir string, opts Options) (string, string, error) {
	explicit := opts.Explicit && strings.TrimSpace(opts.Origin) != ""
	packs, err := journey.LoadDir(projectDir)
	if err != nil && !explicit {
		return "", "", fmt.Errorf("load Journey Packs: %w", err)
	}
	journeyID, origin, allowed := record.ResolveJourney(packs, opts.Journey, opts.Origin)
	if origin == "" {
		return "", "", &record.Error{Code: CodeOriginMissing,
			Message: "no origin to crawl: pass --origin or declare gui.allowed_origins in a Journey Pack"}
	}
	canonical, err := record.CanonicalOrigin(origin)
	if err != nil {
		return "", "", err
	}
	if !allowed && !explicit {
		return "", "", &record.Error{Code: CodeOriginNotAllowed, Message: fmt.Sprintf(
			"origin %s is not listed in any Journey Pack's gui.allowed_origins; pass it with --origin to crawl it explicitly", canonical)}
	}
	if journeyID == "" {
		return "", "", &record.Error{Code: CodeJourneyMissing,
			Message: "no Journey Pack declares gui.allowed_origins; pass --journey to name the pack that runs the baseline"}
	}
	return journeyID, canonical, nil
}

func crawl(ctx context.Context, projectDir, origin string, maxPages int, run Runner) ([]byte, error) {
	if run == nil {
		run = runNode
	}
	dir, err := os.MkdirTemp("", "autopus-discover-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	script := filepath.Join(dir, "autopus-discover.cjs")
	if err := os.WriteFile(script, []byte(CrawlerScript()), 0o600); err != nil {
		return nil, err
	}
	var stdout bytes.Buffer
	err = run(ctx, projectDir, &stdout, "node", script, origin, strconv.Itoa(maxPages))
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return stdout.Bytes(), nil
	case errors.Is(err, exec.ErrNotFound):
		return nil, &record.Error{Code: CodeNodeMissing, SetupGap: true,
			Message: "node was not found on PATH: install Node.js to run discovery"}
	case errors.As(err, &exitErr) && exitErr.ExitCode() == playwrightMissingExit:
		return nil, &record.Error{Code: CodePlaywrightMissing, SetupGap: true,
			Message: "Playwright is not installed in the project: run npm i -D @playwright/test && npx playwright install chromium"}
	}
	return nil, &record.Error{Code: CodeCrawlFailed, Message: fmt.Sprintf("discovery crawler failed: %v", err)}
}

func runNode(ctx context.Context, dir string, stdout io.Writer, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = stdout, os.Stderr
	return cmd.Run()
}

func clampPages(n int) int {
	switch {
	case n <= 0:
		return DefaultMaxPages
	case n > maxPagesLimit:
		return maxPagesLimit
	}
	return n
}
