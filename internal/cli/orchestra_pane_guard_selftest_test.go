package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Self-tests of the SPEC-PANERM-001 pane guard (S15, S18): each fixture holds
// members of every retired class next to retained look-alikes.

func TestPaneGuard_GoFixturesYieldOneFindingPerMember(t *testing.T) {
	// Given pkg/orchestra and internal/cli fixtures holding each Go class
	root := t.TempDir()
	writePaneFixture(t, root, "pkg/orchestra/fixture.go", "package orchestra\n\nfunc RunPaneOrchestra() {}\n\n"+
		"func args(p ProviderConfig) []string { return p.PaneArgs }\n")
	writePaneFixture(t, root, "pkg/orchestra/surface.go", "package orchestra\n\nimport \""+paneTerminalImport+"\"\n\n"+
		"func drive(t terminal.Terminal, job Job) {\n\tt.SendCommand()\n\tt.SendLongText()\n\tt.ReadScreen()\n\tt.Close()\n}\n")
	writePaneFixture(t, root, "pkg/orchestra/fixture_test.go", "package orchestra\n\nfunc RunPaneOrchestra() {}\n")
	writePaneFixture(t, root, "pkg/orchestra/testdata/pane.go", "package orchestra\n\nfunc RunPaneOrchestra() {}\n")
	writePaneFixture(t, root, "internal/cli/fixture.go", "package cli\n\nimport orch \""+paneOrchestraImport+"\"\n\n"+
		"var _ = orch.CleanupStaleJobs\nvar _ = orch.RunPaneOrchestraDetached\nvar Job = \"generic\"\n"+
		"const env = \"AUTOPUS_SESSION_ID\"\n")

	// When the Go scans run over the fixtures
	orchestraFindings := scanPaneGoFiles(t, root, "pkg/orchestra", paneScopeOrchestra)
	cliFindings := scanPaneGoFiles(t, root, "internal/cli", paneScopeCLI)

	// Then each member yields one finding; tests, testdata, and a generic
	// name outside the orchestra selector yield none
	assert.Equal(t, []string{
		`pkg/orchestra/fixture.go:3: group I identifier "RunPaneOrchestra"`,
		`pkg/orchestra/fixture.go:5: group I identifier "PaneArgs"`,
		`pkg/orchestra/surface.go:3: imports "github.com/insajin/autopus-adk/pkg/terminal"`,
		`pkg/orchestra/surface.go:5: group I identifier "Job"`,
		`pkg/orchestra/surface.go:6: pane surface call ".SendCommand("`,
		`pkg/orchestra/surface.go:7: pane surface call ".SendLongText("`,
		`pkg/orchestra/surface.go:8: pane surface call ".ReadScreen("`,
	}, orchestraFindings)
	assert.Equal(t, []string{
		`internal/cli/fixture.go:5: group I identifier "CleanupStaleJobs"`,
		`internal/cli/fixture.go:6: group I identifier "RunPaneOrchestraDetached"`,
		`internal/cli/fixture.go:8: group I identifier "AUTOPUS_SESSION_ID"`,
	}, cliFindings)
}

func TestPaneGuard_GroupPFixtureFailsPerFile(t *testing.T) {
	// Given group P files, a P-named test, and retained look-alikes
	root := t.TempDir()
	for _, name := range []string{"completion_poll_idle_test.go", "completion_poll_test.go", "job.go", "jobs_report.go",
		"pane_backend.go", "relay.go", "relay_execution_evidence.go", "session_store_test.go", "yield.go"} {
		writePaneFixture(t, root, "pkg/orchestra/"+name, "package orchestra\n")
	}

	// When the group P check runs, then only the group P names fail
	assert.Equal(t, []string{
		`pkg/orchestra/completion_poll_test.go: group P file "completion_poll.go"`,
		`pkg/orchestra/job.go: group P file "job.go"`,
		`pkg/orchestra/pane_backend.go: group P file "pane_*.go"`,
		`pkg/orchestra/session_store_test.go: group P file "session*.go"`,
	}, paneGroupFileFindings(t, root))
}

func TestPaneGuard_InstructionFixtureFailsOncePerLine(t *testing.T) {
	// Given the S18 template fixture, the other retired tokens, look-alikes,
	// an allowlisted line, and a file outside the instruction roots
	root := t.TempDir()
	writePaneFixture(t, root, "templates/codex/skills/fixture.md.tmpl", "auto orchestra brainstorm \"x\" --yield-rounds\n"+
		"then auto orchestra wait <job-id>\nkeep pane_args\npass --no-detach\nretry with --subprocess\n")
	writePaneFixture(t, root, "content/skills/rest.md", "--plain\nauto orchestra collect\nauto orchestra inject\n"+
		"auto orchestra cleanup\nauto orchestra status\nauto orchestra result\ninteractive_input\nworking_patterns\n"+
		"orchestra.subprocess.enabled\nfeatures.cc21.monitor_pattern_timeout_ms: 30000\n")
	writePaneFixture(t, root, "content/skills/allowed.md", "auto spec review --plain\n")
	writePaneFixture(t, root, "configs/autopus.yaml", "orchestra:\n  subprocess:\n    max_concurrent: 2\n"+
		"# auto orchestra results and auto orchestra review stay legal\n"+
		"args: [--subprocess-timeout, --plain-text, --no-detached, my_pane_args_v2]\n")
	writePaneFixture(t, root, "docs/retired.md", "--no-detach\n")
	allow := []paneGuardException{{path: "content/skills/allowed.md", token: "--plain", reason: "self-test row"}}

	// When the instruction guard runs over the fixture tree
	findings := retiredTokenFindings(t, root, allow)

	// Then every retired line fails once and nothing else does
	assert.Equal(t, []string{
		`content/skills/rest.md:1: retired token "--plain"`,
		`content/skills/rest.md:2: retired token "auto orchestra collect"`,
		`content/skills/rest.md:3: retired token "auto orchestra inject"`,
		`content/skills/rest.md:4: retired token "auto orchestra cleanup"`,
		`content/skills/rest.md:5: retired token "auto orchestra status"`,
		`content/skills/rest.md:6: retired token "auto orchestra result"`,
		`content/skills/rest.md:7: retired token "interactive_input"`,
		`content/skills/rest.md:8: retired token "working_patterns"`,
		`content/skills/rest.md:9: retired token "subprocess.enabled"`,
		`content/skills/rest.md:10: retired token "monitor_pattern_timeout_ms"`,
		`templates/codex/skills/fixture.md.tmpl:1: retired token "--yield-rounds"`,
		`templates/codex/skills/fixture.md.tmpl:2: retired token "auto orchestra wait"`,
		`templates/codex/skills/fixture.md.tmpl:3: retired token "pane_args"`,
		`templates/codex/skills/fixture.md.tmpl:4: retired token "--no-detach"`,
		`templates/codex/skills/fixture.md.tmpl:5: retired token "--subprocess"`,
	}, findings)
}

func writePaneFixture(t *testing.T, root, rel, body string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(body), 0o644))
}
