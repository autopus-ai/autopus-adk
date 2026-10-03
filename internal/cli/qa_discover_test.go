package cli

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	qadiscover "github.com/insajin/autopus-adk/pkg/qa/discover"
	qascenario "github.com/insajin/autopus-adk/pkg/qa/scenario"
)

// qaDiscoverFake records each crawler invocation and prints a two-page crawl
// of the origin it was given.
func qaDiscoverFake(calls *[][]string) qadiscover.Runner {
	return func(_ context.Context, _ string, stdout io.Writer, _ string, args ...string) error {
		*calls = append(*calls, args)
		_, err := io.WriteString(stdout, `{"schema_version":"qamesh.discover.v1","origin":"`+args[1]+`","pages":[`+
			`{"path":"/","title":"Shop","headings":["Welcome"],"landmarks":["main"]},`+
			`{"path":"/cart","title":"Cart","headings":[],"landmarks":[]}]}`)
		return err
	}
}

// AC-QALOOP-017 through the CLI: --origin is explicit, and the output says
// the result is a regression baseline, not a correctness proof.
func TestQADiscoverCmd_ExplicitOriginWritesBaselineAndSaysItIsNoProof(t *testing.T) {
	project := scenarioProject(t)
	var calls [][]string

	_, text, err := qaRecordExecute(t, newQADiscoverCmdWith(qaDiscoverFake(&calls)),
		"--origin", "http://localhost:3000", "--max-pages", "5", "--project-dir", project)

	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"http://localhost:3000", "5"}, calls[0][1:])
	assert.Contains(t, text, "regression baseline, not a correctness proof")
	loaded, err := qascenario.LoadFile(filepath.Join(qascenario.CandidatesDir(project), "discovered-baseline.yaml"))
	require.NoError(t, err)
	assert.Equal(t, qascenario.IntentBaseline, loaded.IntentSource)
	assert.Equal(t, "http://localhost:3000", loaded.Origin)
	assert.Len(t, loaded.Screens, 2)
}

func TestQADiscoverCmd_WithoutOriginCrawlsThePackOrigin(t *testing.T) {
	project := scenarioProject(t)
	var calls [][]string

	payload, _, err := qaRecordExecute(t, newQADiscoverCmdWith(qaDiscoverFake(&calls)),
		"--project-dir", project, "--format", "json")

	require.NoError(t, err)
	require.Len(t, calls, 1)
	assert.Equal(t, []string{"http://127.0.0.1:4173", "20"}, calls[0][1:])
	data := payload["data"].(map[string]any)
	assert.Contains(t, data["notice"], "not a correctness proof")
	assert.Equal(t, "browser-gui-explore", data["journey"])
	assert.EqualValues(t, 2, data["screens"])
}

func TestQADiscoverCmd_ExposesDiscoveryFlags(t *testing.T) {
	cmd := newQADiscoverCmd()
	assert.Equal(t, "discover", cmd.Name())
	for _, flag := range []string{"origin", "journey", "id", "max-pages", "project-dir", "format"} {
		assert.NotNil(t, cmd.Flags().Lookup(flag), flag)
	}
}

func TestQADiscoverCmd_NoOriginAnywhereFailsWithCode(t *testing.T) {
	var calls [][]string

	payload, _, err := qaRecordExecute(t, newQADiscoverCmdWith(qaDiscoverFake(&calls)),
		"--project-dir", t.TempDir(), "--format", "json")

	require.Error(t, err)
	assert.Equal(t, "qa_discover_origin_missing", payload["error"].(map[string]any)["code"])
	assert.Empty(t, calls)
}
