package discover_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/discover"
	"github.com/insajin/autopus-adk/pkg/qa/record"
	"github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// threePages is AC-QALOOP-017's crawl output for three pages.
const threePages = `{"schema_version":"qamesh.discover.v1","origin":"http://127.0.0.1:4173","pages":[
 {"path":"/","title":"Shop","headings":["Welcome","Featured","Deals","Extra"],"landmarks":["main","navigation"]},
 {"path":"/products?page=2","title":"Products","headings":["All products","All products"],"landmarks":["main"]},
 {"path":"/about","title":"","headings":["About  us"],"landmarks":[]}]}`

const guiPack = `id: browser-gui-explore
title: GUI exploration
surface: frontend
lanes: [gui-explore]
adapter:
  id: gui-explore
command:
  argv: ["npm", "exec", "playwright", "test"]
  cwd: .
  timeout: 120s
checks:
  - id: browser-gui-explore
    type: gui_exploration
    expected:
      exit_code: 0
gui:
  allowed_origins: ["http://127.0.0.1:4173"]
  forbidden_actions: [mutation]
  selector_strategy: role-first
  network_policy:
    mode: summary-only
source_refs:
  source_spec: SPEC-QAMESH-003
  acceptance_refs: [AC-1]
  owned_paths: ["tests/**"]
`

func projectWithPack(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	journeys := filepath.Join(dir, ".autopus", "qa", "journeys")
	require.NoError(t, os.MkdirAll(journeys, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(journeys, "browser-gui-explore.yaml"), []byte(guiPack), 0o644))
	return dir
}

type crawlCall struct {
	dir, name, script string
	args              []string
}

// fakeCrawler records each node invocation and prints threePages retargeted
// to the origin it was given.
func fakeCrawler(calls *[]crawlCall) discover.Runner {
	return func(_ context.Context, dir string, stdout io.Writer, name string, args ...string) error {
		script, err := os.ReadFile(args[0])
		if err != nil {
			return err
		}
		*calls = append(*calls, crawlCall{dir: dir, name: name, script: string(script), args: args})
		_, err = io.WriteString(stdout, strings.ReplaceAll(threePages, "http://127.0.0.1:4173", args[1]))
		return err
	}
}

func heading(name string) scenario.Step {
	return scenario.Step{ExpectRole: &scenario.RoleTarget{Role: "heading", Name: name}}
}

func TestQADiscoverToBaseline_ThreePagesBecomeThreeReadOnlyScreens(t *testing.T) {
	t.Parallel()
	crawl, err := discover.Decode([]byte(threePages))
	require.NoError(t, err)

	s, err := discover.ToBaseline(crawl, discover.BaselineOptions{Journey: "browser-gui-explore"})

	require.NoError(t, err)
	require.NoError(t, scenario.Validate(s))
	assert.Equal(t, "discovered-baseline", s.ID)
	assert.Equal(t, scenario.IntentBaseline, s.IntentSource)
	assert.Contains(t, s.Title, "regression baseline, not a correctness proof")
	assert.False(t, s.HasActions())
	assert.Equal(t, []scenario.Screen{
		{ID: "home", Path: "/", Steps: []scenario.Step{{ExpectTitle: "Shop"}, heading("Welcome"), heading("Featured"), heading("Deals")}},
		{ID: "products", Path: "/products?page=2", Steps: []scenario.Step{{ExpectTitle: "Products"}, heading("All products")}},
		{ID: "about", Path: "/about", Steps: []scenario.Step{heading("About us")}},
	}, s.Screens)
}

func TestQADiscoverDecode_RejectsOutputThatIsNotACrawl(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct{ stdout, code string }{
		"other schema": {`{"schema_version":"x","origin":"http://a.test","pages":[]}`, discover.CodeDecodeInvalid},
		"no pages":     {`{"schema_version":"qamesh.discover.v1","origin":"http://a.test","pages":[]}`, discover.CodeNoPages},
		"not json":     {"Error: browser closed", discover.CodeDecodeInvalid},
	} {
		_, err := discover.Decode([]byte(tc.stdout))
		code, _ := record.CodeOf(err)
		assert.Equal(t, tc.code, code, name)
	}
	crawl, err := discover.Decode([]byte("(node:1) Warning: something\n" +
		`{"schema_version":"qamesh.discover.v1","origin":"http://a.test","pages":[{"path":"/","title":"A","headings":[],"landmarks":[]}]}`))
	require.NoError(t, err)
	assert.Len(t, crawl.Pages, 1)
}

func TestQADiscoverCrawlerScript_OnlyNavigatesWithGoto(t *testing.T) {
	t.Parallel()
	script := discover.CrawlerScript()
	// The premise is load-bearing: the absence checks below would pass on an
	// empty or unrelated string, so first prove this is the crawler.
	for _, want := range []string{"page.goto(", "headless: true", "qamesh.discover.v1", "require.resolve(", "@playwright/test"} {
		assert.Contains(t, script, want)
	}
	for _, banned := range []string{".click(", ".fill(", ".press(", ".check(", ".selectOption(", ".type(",
		".dblclick(", ".tap(", ".uncheck(", ".setInputFiles(", ".dispatchEvent(", ".hover("} {
		assert.NotContains(t, script, banned)
	}
}

func TestQADiscoverCrawlerScript_IsValidJavaScript(t *testing.T) {
	t.Parallel()
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not installed; the read-only check above still runs")
	}
	path := filepath.Join(t.TempDir(), "crawler.cjs")
	require.NoError(t, os.WriteFile(path, []byte(discover.CrawlerScript()), 0o644))
	out, err := exec.Command(node, "--check", path).CombinedOutput()
	require.NoError(t, err, string(out))
}

func TestQADiscoverRun_RefusesAnOriginNoPackAllows(t *testing.T) {
	t.Parallel()
	var calls []crawlCall
	_, err := discover.Run(context.Background(), projectWithPack(t),
		discover.Options{Origin: "http://other.example", Exec: fakeCrawler(&calls)})

	code, _ := record.CodeOf(err)
	assert.Equal(t, discover.CodeOriginNotAllowed, code)
	assert.Empty(t, calls, "a refused origin is never crawled")
}

func TestQADiscoverRun_ExplicitOriginIsCrawledAndLabelledAsBaseline(t *testing.T) {
	t.Parallel()
	project := projectWithPack(t)
	var calls []crawlCall

	result, err := discover.Run(context.Background(), project,
		discover.Options{Origin: "http://other.example/", Explicit: true, Exec: fakeCrawler(&calls)})

	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, project, calls[0].dir)
	assert.Equal(t, "node", calls[0].name)
	assert.Equal(t, []string{"http://other.example", "20"}, calls[0].args[1:])
	assert.Equal(t, discover.CrawlerScript(), calls[0].script)
	assert.True(t, result.Created)
	assert.Equal(t, 3, result.Screens)
	assert.Equal(t, 7, result.Steps)
	assert.Equal(t, "browser-gui-explore", result.Journey)
	assert.Contains(t, result.Notice, "not a correctness proof")
	body, err := os.ReadFile(result.Path)
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(string(body), "# Discovered regression baseline, not a correctness proof"))
	loaded, err := scenario.LoadFile(result.Path)
	require.NoError(t, err)
	assert.Equal(t, "http://other.example", loaded.Origin)
	assert.Equal(t, scenario.IntentBaseline, loaded.IntentSource)
}

func TestQADiscoverRun_DefaultsToThePackOriginAndClampsPages(t *testing.T) {
	t.Parallel()
	var calls []crawlCall
	result, err := discover.Run(context.Background(), projectWithPack(t),
		discover.Options{MaxPages: 5000, ID: "shop-baseline", Exec: fakeCrawler(&calls)})

	require.NoError(t, err)
	assert.Equal(t, []string{"http://127.0.0.1:4173", "200"}, calls[0].args[1:])
	assert.Equal(t, "shop-baseline", result.ID)
	assert.Equal(t, "http://127.0.0.1:4173", result.Origin)
}

func TestQADiscoverRun_MissingNodeIsASetupGap(t *testing.T) {
	t.Parallel()
	missing := func(context.Context, string, io.Writer, string, ...string) error {
		return &exec.Error{Name: "node", Err: exec.ErrNotFound}
	}
	_, err := discover.Run(context.Background(), projectWithPack(t), discover.Options{Exec: missing})

	code, setupGap := record.CodeOf(err)
	assert.Equal(t, discover.CodeNodeMissing, code)
	assert.True(t, setupGap)
}
