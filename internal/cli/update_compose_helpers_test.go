package cli

import (
	"bytes"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeUpdateNotice is the one non-interactive line naming fixture R's
// historical presets (REQ-MUX-14).
const composeUpdateNotice = "참고: ultra, balanced 프리셋이 이전 버전 기본값입니다. " +
	"대화형 auto update 또는 auto quality에서 갱신할 수 있습니다."

// composeUpdateRun is the isolated environment of one real `auto update`
// run: the TTY and CI seams, Korean text, the run's catalog probe answering
// CAT-A, and a PATH holding only a codex stand-in that logs every call and
// answers `debug models` with CAT-B. An adapter that probed for itself would
// therefore generate from another catalog than the table showed.
type composeUpdateRun struct {
	probes   *[]composeProbeCall
	codexLog string
}

func newComposeUpdateRun(t *testing.T, tty bool, ci string) composeUpdateRun {
	t.Helper()
	stubDriverProbes(t, tty, ci)
	originalLanguage := composeLanguage
	t.Cleanup(func() { composeLanguage = originalLanguage })
	composeLanguage = func() string { return "ko" }
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())
	t.Setenv("PATH", t.TempDir())
	codexLog := installComposeFakeCodex(t, mustReadComposeCatalog(t, "CAT-B"))
	return composeUpdateRun{probes: composeSessionCatalog(t, "CAT-A"), codexLog: codexLog}
}

// rerun prepares one more run in r's environment for a subtest: its own TTY
// and CI seams, a fresh probe counter, and no codex calls remembered.
func (r composeUpdateRun) rerun(t *testing.T, tty bool, ci string) composeUpdateRun {
	t.Helper()
	stubDriverProbes(t, tty, ci)
	require.NoError(t, os.RemoveAll(r.codexLog))
	return composeUpdateRun{probes: composeSessionCatalog(t, "CAT-A"), codexLog: r.codexLog}
}

// cloneComposeProject copies a settled project into a fresh temp dir, so the
// subtests of one test share a single settle run. Permission bits and
// symlinks are kept: generated owner-only files must stay owner-only.
func cloneComposeProject(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, path)
		if err != nil || rel == "." {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case info.IsDir():
			if err := os.Mkdir(target, 0o700); err != nil {
				return err
			}
		default:
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if err := os.WriteFile(target, data, 0o600); err != nil {
				return err
			}
		}
		return os.Chmod(target, info.Mode().Perm())
	})
	require.NoError(t, err)
	return dst
}

// adapterProbed reports whether a platform adapter asked the codex CLI for
// its own catalog during the run.
func (r composeUpdateRun) adapterProbed(t *testing.T) bool {
	t.Helper()
	return strings.Contains(composeCodexCalls(t, r.codexLog), "debug models")
}

// runComposeUpdate executes the real root command with in as its input.
func runComposeUpdate(t *testing.T, in io.Reader, args ...string) (string, error) {
	t.Helper()
	root := NewRootCmd()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(in)
	root.SetArgs(append([]string{"update"}, args...))
	err := root.Execute()
	return out.String(), err
}

// settled writes fixture D or R and runs one non-interactive update over it
// in the run's environment, so the update's own migrations and language
// prompts have already run: a later run that changes autopus.yaml changes it
// because of the composition step. The codex calls of that run are
// forgotten.
func (r composeUpdateRun) settled(t *testing.T, fixture string) string {
	t.Helper()
	dir, _ := composeSessionProject(t, fixture)
	interactive := composeTTYProbe
	composeTTYProbe = func() bool { return false }
	out, err := runComposeUpdate(t, strings.NewReader(""), "--dir", dir)
	composeTTYProbe = interactive
	require.NoError(t, err, out)
	require.NoError(t, os.RemoveAll(r.codexLog))
	return dir
}

// countingComposeStep wraps the update's composition step and counts how
// often a run enters it.
func countingComposeStep(t *testing.T) *int {
	t.Helper()
	original := updateComposeStep
	t.Cleanup(func() { updateComposeStep = original })
	entries := new(int)
	updateComposeStep = func(cmd *cobra.Command, dir string, cfg *config.HarnessConfig, gate composeGate) driftGenerationSnapshot {
		*entries++
		return original(cmd, dir, cfg, gate)
	}
	return entries
}

// composeTableColumns splits one rendered table line into its cells: columns
// are padded apart by two or more spaces and no cell holds two in a row.
var composeTableColumns = regexp.MustCompile(` {2,}`)

// composeRenderedTable reads the 16-row table that follows the first line of
// out equal to marker, keyed by agent and then by column header. It reads the
// screen the operator saw, so a test can hold generated files against it.
func composeRenderedTable(t *testing.T, out, marker string) map[string]map[string]string {
	t.Helper()
	lines := strings.Split(out, "\n")
	start := -1
	for index, line := range lines {
		if line == marker {
			start = index + 1
			break
		}
	}
	require.GreaterOrEqual(t, start, 0, "no line %q in:\n%s", marker, out)
	require.Greater(t, len(lines), start+len(config.CanonicalAgentNames()))
	header := composeTableColumns.Split(lines[start], -1)
	rows := map[string]map[string]string{}
	for _, line := range lines[start+1 : start+1+len(config.CanonicalAgentNames())] {
		cells := composeTableColumns.Split(line, -1)
		require.Len(t, cells, len(header), "row %q", line)
		row := map[string]string{}
		for index, cell := range cells {
			row[header[index]] = cell
		}
		rows[cells[1]] = row
	}
	require.Len(t, rows, len(config.CanonicalAgentNames()))
	return rows
}

// assertGeneratedEqualsTable holds every agent's generated Claude Code
// frontmatter and Codex TOML against the cells the operator saw: the cell
// text, without its change mark, is the generated model without its provider
// prefix and the effort, `-` when the line is absent (REQ-MUX-19). It
// returns how many Claude Code and Codex cells carried the change mark.
func assertGeneratedEqualsTable(t *testing.T, dir string, table map[string]map[string]string) (int, int) {
	t.Helper()
	marked := func(cell string) (string, int) {
		if trimmed, ok := strings.CutSuffix(cell, "*"); ok {
			return trimmed, 1
		}
		return cell, 0
	}
	effortText := func(effort string) string {
		if effort == "" {
			return "-"
		}
		return effort
	}
	claudeMarks, codexMarks := 0, 0
	for _, agent := range config.CanonicalAgentNames() {
		row := table[agent]
		claude := readComposeTableClaudeProfile(t, dir, agent)
		cell, mark := marked(row["Claude Code"])
		claudeMarks += mark
		assert.Equal(t, strings.TrimPrefix(claude.model, "claude-")+" · "+effortText(claude.effort), cell, "Claude Code %s", agent)
		codexProfile := readComposeCodexProfile(t, dir, agent)
		cell, mark = marked(row["Codex"])
		codexMarks += mark
		assert.Equal(t, strings.TrimPrefix(codexProfile.Model, "gpt-")+" · "+effortText(codexProfile.Effort), cell, "Codex %s", agent)
	}
	return claudeMarks, codexMarks
}

func rewriteComposeFile(t *testing.T, path, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), old, path)
	require.NoError(t, os.WriteFile(path, []byte(strings.Replace(string(data), old, replacement, 1)), 0o644))
}

// rewriteComposeLine replaces the first line of path that starts with prefix.
func rewriteComposeLine(t *testing.T, path, prefix, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := strings.Split(string(data), "\n")
	for index, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[index] = replacement
			require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644))
			return
		}
	}
	t.Fatalf("no line starting with %q in %s", prefix, path)
}

func composeGeneratedPath(dir string, parts ...string) string {
	return filepath.Join(append([]string{dir}, parts...)...)
}
