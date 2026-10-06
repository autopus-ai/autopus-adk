package cli_test

// SPEC-PANERM-001 T2 (S16): output and receipt parity goldens. At B the six
// orchestra invocations below print --format json stdout and `auto spec
// review` writes review-receipt.json and the review.md Provider Health rows,
// all against fake providers. testdata/panerm_s16 holds those outputs after
// normalizing only the volatile fields named by s16VolatileFields. The same
// invocations must reproduce them byte for byte after the pane backend is
// deleted. AUTOPUS_PANERM_S16_CAPTURE=1 rewrites the goldens; run it only at B.

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// s16GoldenDir is absolute because every S16 run changes into its workspace.
var s16GoldenDir, _ = filepath.Abs(filepath.Join("testdata", "panerm_s16"))

// s16FakeProvider answers every provider role with one JSON body, also into
// the codex --output-last-message file, and serves `codex debug models`.
const s16FakeProvider = `#!/bin/sh
name=$(basename "$0")
if [ "$name" = codex ] && [ "$1" = debug ] && [ "$2" = models ]; then
  printf '%s\n' '{"models":[{"slug":"gpt-6-astra","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]},{"slug":"gpt-6.1-sol","supported_reasoning_levels":[{"effort":"xhigh"},{"effort":"max"},{"effort":"ultra"}]}]}'
  exit 0
fi
cat >/dev/null
body='{"verdict":"PASS","summary":"fake '"$name"' answer","findings":[],"recommendation":"fake '"$name"' answer"}'
while [ $# -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then printf '%s' "$body" > "$2"; fi
  shift
done
printf '%s\n' "$body"
`

// s16VolatileFields are the only fields S16 normalizes: timestamps,
// durations, run ids, process ids, and temp paths (s16Normalize).
var s16VolatileFields = []string{
	"run_id", "started_at", "ended_at", "finished_at", "generated_at", "created_at", "timestamp", "pid", "duration_ms",
}

// s16Invocations are the orchestra goldens: name and auto argv.
var s16Invocations = []struct {
	name string
	args []string
}{
	{"brainstorm", []string{"orchestra", "brainstorm", "panerm topic", "--strategy", "debate", "--rounds", "1",
		"--providers", "codex,gemini", "--judge", "claude", "--format", "json"}},
	{"plan", []string{"orchestra", "plan", "panerm plan", "--strategy", "consensus", "--providers", "claude,codex",
		"--no-persist", "--format", "json"}},
	{"review", []string{"orchestra", "review", "a.go", "--risk-tier", "high", "--strategy", "debate",
		"--providers", "claude,codex", "--format", "json"}},
	{"secure", []string{"orchestra", "secure", "a.go", "--strategy", "consensus", "--providers", "claude,codex",
		"--format", "json"}},
	{"run", []string{"orchestra", "run", "panerm run", "--strategy", "debate", "--rounds", "fast",
		"--providers", "claude,codex", "--judge", "claude", "--format", "json"}},
	{"recheck", []string{"orchestra", "review", "a.go", "--strategy", "recheck", "--providers", "claude",
		"--format", "json"}},
}

type s16Workspace struct{ root, home string }

// roots maps each scratch root to its placeholder; the workspace and HOME
// live under the temp dir, so they are replaced first.
func (ws s16Workspace) roots() map[string]string {
	return map[string]string{"<root>": ws.root, "<home>": ws.home, "<tmpdir>": os.TempDir()}
}

// newS16Workspace pins every input S16 depends on: fake providers first on a
// PATH without tmux or cmux, a scratch HOME and CODEX_HOME, no invoking agent
// runtime, and a fresh workspace holding B's default config and a.go.
func newS16Workspace(t *testing.T) s16Workspace {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake providers require a POSIX shell")
	}
	ws := s16Workspace{root: t.TempDir(), home: t.TempDir()}
	binDir := t.TempDir()
	for _, name := range []string{"claude", "codex", "agy"} {
		require.NoError(t, os.WriteFile(filepath.Join(binDir, name), []byte(s16FakeProvider), 0o755))
	}
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+"/usr/bin:/bin")
	t.Setenv("HOME", ws.home)
	t.Setenv("CODEX_HOME", filepath.Join(ws.home, ".codex"))
	// An unknown platform token wins over the process-ancestry scan, so the
	// host agent runtime never becomes the invoking provider.
	t.Setenv("AUTOPUS_PLATFORM", "panerm-golden")
	for _, key := range []string{"CLAUDECODE", "CODEX", "CODEX_CI", "CODEX_THREAD_ID", "CODEX_MANAGED_BY_NPM", "TMUX",
		"CMUX_SOCKET_PATH", "CMUX_WORKSPACE_ID", "CMUX_SURFACE_ID", "CMUX_PANE_ID", "CI"} {
		t.Setenv(key, "")
	}
	require.NoError(t, config.Save(ws.root, config.DefaultFullConfig("panerm-s16")))
	require.NoError(t, os.WriteFile(filepath.Join(ws.root, "a.go"), []byte("package a\n\nfunc A() int { return 1 }\n"), 0o644))
	t.Chdir(ws.root)
	return ws
}

// runS16 executes one auto argv in-process and returns what it wrote to
// stdout; orchestra commands write to os.Stdout directly, so it is swapped.
func runS16(t *testing.T, args []string) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	original := os.Stdout
	os.Stdout = writer
	done := make(chan []byte)
	go func() {
		data, _ := io.ReadAll(reader)
		done <- data
	}()
	cmd := newTestRootCmd()
	cmd.SetOut(writer)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	runErr := cmd.Execute()
	os.Stdout = original
	require.NoError(t, writer.Close())
	return string(<-done), runErr
}

var (
	s16FieldPattern = regexp.MustCompile(`"(` + strings.Join(s16VolatileFields, "|") + `)":(\s*)("[^"]*"|-?[0-9.]+)`)
	s16IDPattern    = regexp.MustCompile(`\b(run-[0-9]{10,}-[0-9a-f]+|orch-[0-9a-f]{32})\b`)
	s16TempName     = regexp.MustCompile(`\b(codex-last-message|schema-[A-Za-z0-9_]+|autopus-brainstorm)-[0-9]+`)
)

// s16Normalize replaces the volatile values: the named fields, run ids, and
// temp paths (the scratch roots and the random suffixes of temp file names).
// It then indents JSON without reordering keys.
func s16Normalize(t *testing.T, data []byte, roots map[string]string) string {
	t.Helper()
	text := string(data)
	for _, placeholder := range []string{"<root>", "<home>", "<tmpdir>"} {
		for _, form := range s16PathForms(roots[placeholder]) {
			text = strings.ReplaceAll(text, form, placeholder)
		}
	}
	text = s16FieldPattern.ReplaceAllString(text, `"$1":$2"<volatile>"`)
	text = s16IDPattern.ReplaceAllString(text, "<run-id>")
	text = s16TempName.ReplaceAllString(text, "$1-<n>")
	var out bytes.Buffer
	if json.Indent(&out, []byte(text), "", "  ") != nil {
		return text
	}
	return out.String() + "\n"
}

func s16PathForms(path string) []string {
	path = strings.TrimRight(path, string(filepath.Separator))
	if path == "" {
		return nil
	}
	forms := []string{path}
	if resolved, err := filepath.EvalSymlinks(path); err == nil && resolved != path {
		forms = append([]string{resolved}, forms...)
	}
	return forms
}

// assertS16Golden compares got with the named golden, or rewrites it in
// capture mode.
func assertS16Golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join(s16GoldenDir, name)
	if os.Getenv("AUTOPUS_PANERM_S16_CAPTURE") == "1" {
		require.NoError(t, os.MkdirAll(s16GoldenDir, 0o755))
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden %s is captured at B", name)
	assert.Equal(t, string(want), got, "S16 %s differs from the B golden after normalization", name)
}

func TestPanermS16_OrchestraJSONStdoutMatchesBGolden(t *testing.T) {
	for _, inv := range s16Invocations {
		t.Run(inv.name, func(t *testing.T) {
			ws := newS16Workspace(t)
			stdout, err := runS16(t, inv.args)
			require.NoError(t, err, stdout)
			got := s16Normalize(t, []byte(stdout), ws.roots())
			assertS16Golden(t, inv.name+".json", got)
			assertS16Backends(t, got)
		})
	}
}

// assertS16Backends pins the S16 backend values independently of the golden:
// every dispatched CLI provider ran on the subprocess backend.
func assertS16Backends(t *testing.T, normalized string) {
	t.Helper()
	var payload struct {
		Receipt struct {
			ProviderReceipts []struct {
				Provider string `json:"provider"`
				Backend  string `json:"backend"`
			} `json:"provider_receipts"`
		} `json:"receipt"`
	}
	if json.Unmarshal([]byte(normalized), &payload) != nil {
		return
	}
	for _, receipt := range payload.Receipt.ProviderReceipts {
		assert.Equal(t, "subprocess", receipt.Backend, receipt.Provider)
	}
}

func TestPanermS16_SpecReviewReceiptMatchesBGolden(t *testing.T) {
	ws := newS16Workspace(t)
	const specID = "SPEC-PANERMS16-001"
	require.NoError(t, spec.Scaffold(ws.root, strings.TrimPrefix(specID, "SPEC-"), "S16 golden"))
	stdout, err := runS16(t, []string{"spec", "review", specID, "--skip-provider-readiness", "--providers", "claude,codex"})
	require.NoError(t, err, stdout)

	specDir := filepath.Join(ws.root, ".autopus", "specs", specID)
	receipt, err := os.ReadFile(filepath.Join(specDir, "review-receipt.json"))
	require.NoError(t, err)
	assertS16Golden(t, "spec-review-receipt.json", s16Normalize(t, receipt, ws.roots()))

	var parsed struct {
		Providers []struct {
			Provider string `json:"provider"`
			Backend  string `json:"executed_backend"`
		} `json:"providers"`
	}
	require.NoError(t, json.Unmarshal(receipt, &parsed))
	require.NotEmpty(t, parsed.Providers)
	for _, row := range parsed.Providers {
		assert.Equal(t, "subprocess", row.Backend, row.Provider)
	}

	review, err := os.ReadFile(filepath.Join(specDir, "review.md"))
	require.NoError(t, err)
	assertS16Golden(t, "spec-review-provider-health.md", s16ProviderHealthRows(string(review)))
}

// s16ProviderHealthRows returns the "## Provider Health" section of review.md.
func s16ProviderHealthRows(review string) string {
	_, section, found := strings.Cut(review, "## Provider Health\n")
	if !found {
		return ""
	}
	var rows []string
	for _, line := range strings.Split(section, "\n") {
		if strings.HasPrefix(line, "## ") {
			break
		}
		if strings.HasPrefix(line, "|") {
			rows = append(rows, line)
		}
	}
	return strings.Join(rows, "\n") + "\n"
}
