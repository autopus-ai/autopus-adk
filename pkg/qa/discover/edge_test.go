package discover_test

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/qa/discover"
	"github.com/insajin/autopus-adk/pkg/qa/record"
)

// printCrawl stands in for the crawler by printing body verbatim.
func printCrawl(body string) discover.Runner {
	return func(_ context.Context, _ string, stdout io.Writer, _ string, _ ...string) error {
		_, err := io.WriteString(stdout, body)
		return err
	}
}

func TestQADiscoverRun_ReportsPagesThatCannotBeScreens(t *testing.T) {
	t.Parallel()
	body := `{"schema_version":"qamesh.discover.v1","origin":"http://127.0.0.1:4173","pages":[` +
		`{"path":"/","title":"Home","headings":[],"landmarks":[]},` +
		`{"path":"/a b","title":"Spaced","headings":[],"landmarks":[]},` +
		`{"path":"/","title":"Home again","headings":[],"landmarks":[]},` +
		`{"path":"/blank","title":"","headings":[],"landmarks":["main"]}]}`

	result, err := discover.Run(context.Background(), projectWithPack(t), discover.Options{Exec: printCrawl(body)})

	require.NoError(t, err)
	assert.Equal(t, 4, result.Pages)
	assert.Equal(t, 1, result.Screens)
	assert.Equal(t, []discover.SkippedPage{
		{Path: "/a b", Reason: "path cannot be a screen path"},
		{Path: "/", Reason: "duplicate path"},
		{Path: "/blank", Reason: "no title or heading to assert"},
	}, result.Skipped)

	_, err = discover.ToBaseline(discover.Crawl{Origin: "http://a.test", Pages: []discover.Page{{Path: "/blank"}}},
		discover.BaselineOptions{Journey: "j"})
	code, _ := record.CodeOf(err)
	assert.Equal(t, discover.CodeNoPages, code, "a crawl with nothing to assert writes no baseline")
}

func TestQADiscoverRun_SurfacesCrawlerAndPolicyFailures(t *testing.T) {
	t.Parallel()
	failing := func(context.Context, string, io.Writer, string, ...string) error { return errors.New("exit status 1") }
	_, err := discover.Run(context.Background(), projectWithPack(t), discover.Options{Exec: failing})
	code, setupGap := record.CodeOf(err)
	assert.Equal(t, discover.CodeCrawlFailed, code)
	assert.False(t, setupGap)

	_, err = discover.Run(context.Background(), t.TempDir(),
		discover.Options{Origin: "http://a.test", Explicit: true, Exec: printCrawl(threePages)})
	code, _ = record.CodeOf(err)
	assert.Equal(t, discover.CodeJourneyMissing, code, "an explicit origin still needs a pack to run under")

	_, err = discover.Run(context.Background(), projectWithPack(t), discover.Options{Exec: printCrawl("not json")})
	code, _ = record.CodeOf(err)
	assert.Equal(t, discover.CodeDecodeInvalid, code)

	_, err = discover.Run(context.Background(), t.TempDir(), discover.Options{Exec: printCrawl(threePages)})
	code, _ = record.CodeOf(err)
	assert.Equal(t, discover.CodeOriginMissing, code)
}

// The embedded crawler really runs under node and, in a project without
// Playwright, exits with the setup-gap code before any browser starts.
func TestQADiscoverRun_RealNodeWithoutPlaywrightIsASetupGap(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not installed")
	}
	// require.resolve also searches global module folders under HOME and
	// NODE_PATH, so both point at nothing to keep the answer deterministic.
	t.Setenv("HOME", t.TempDir())
	t.Setenv("NODE_PATH", "")

	_, err := discover.Run(context.Background(), projectWithPack(t), discover.Options{})

	code, setupGap := record.CodeOf(err)
	assert.Equal(t, discover.CodePlaywrightMissing, code, "%v", err)
	assert.True(t, setupGap)
}
