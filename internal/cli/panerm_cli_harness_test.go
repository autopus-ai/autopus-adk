package cli_test

// SPEC-PANERM-001 T6: a scratch workspace for the built auto binary with
// recording fake providers (argv per call) and recording fake terminal
// multiplexers on PATH, plus a tree snapshot for no-side-effect oracles.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/spec"
)

// panermArgvRecorder prefixes s16FakeProvider: every call writes its own
// record file (providers run concurrently) holding the binary name and each
// argument, joined by US (\037) so multi-line prompts stay one record.
const panermArgvRecorder = `#!/bin/sh
record=$(mktemp "$PANERM_ARGV_DIR/call.XXXXXX")
{ printf '%s' "$(basename "$0")"; for arg in "$@"; do printf '\037%s' "$arg"; done; } > "$record"
`

// panermTerminalRecorder stands in for tmux and cmux: it records the call and
// fails, so any terminal use is visible and inert.
const panermTerminalRecorder = `#!/bin/sh
printf '%s %s\n' "$(basename "$0")" "$*" >> "$PANERM_TERM_LOG"
exit 1
`

type panermCLIWorkspace struct {
	root, home, tmp, bin string
	argvDir, termLog     string
	extraEnv             []string
}

// newPanermCLIWorkspace pins every input a run depends on: fake providers and
// multiplexers first on PATH, a scratch HOME, TMPDIR, and CODEX_HOME, no
// invoking agent runtime, and autopus.yaml (B's defaults when config is nil)
// next to a.go.
func newPanermCLIWorkspace(t *testing.T, cfg []byte) panermCLIWorkspace {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("fake providers require a POSIX shell")
	}
	ws := panermCLIWorkspace{root: t.TempDir(), home: t.TempDir(), tmp: t.TempDir(), bin: t.TempDir()}
	ws.argvDir = filepath.Join(ws.home, "argv")
	ws.termLog = filepath.Join(ws.home, "terminal.log")
	require.NoError(t, os.Mkdir(ws.argvDir, 0o755))
	provider := panermArgvRecorder + strings.TrimPrefix(s16FakeProvider, "#!/bin/sh\n")
	for _, name := range []string{"claude", "codex", "agy"} {
		require.NoError(t, os.WriteFile(filepath.Join(ws.bin, name), []byte(provider), 0o755))
	}
	for _, name := range []string{"tmux", "cmux"} {
		require.NoError(t, os.WriteFile(filepath.Join(ws.bin, name), []byte(panermTerminalRecorder), 0o755))
	}
	if cfg == nil {
		require.NoError(t, config.Save(ws.root, config.DefaultFullConfig("panerm-t6")))
	} else {
		require.NoError(t, os.WriteFile(filepath.Join(ws.root, "autopus.yaml"), cfg, 0o644))
	}
	require.NoError(t, os.WriteFile(filepath.Join(ws.root, "a.go"), []byte("package a\n\nfunc A() int { return 1 }\n"), 0o644))
	return ws
}

// scaffoldPanermSpec adds the SPEC that the S8 spec review pairs review.
func scaffoldPanermSpec(t *testing.T, ws panermCLIWorkspace) {
	t.Helper()
	require.NoError(t, spec.Scaffold(ws.root, "PANERMS8-001", "S8 pair"))
}

func (ws panermCLIWorkspace) env() []string {
	return append([]string{
		"HOME=" + ws.home, "TMPDIR=" + ws.tmp, "PATH=" + ws.bin + ":/usr/bin:/bin",
		"CODEX_HOME=" + filepath.Join(ws.home, ".codex"), "AUTOPUS_PLATFORM=panerm-golden",
		"PANERM_ARGV_DIR=" + ws.argvDir, "PANERM_TERM_LOG=" + ws.termLog,
	}, ws.extraEnv...)
}

// run executes the binary in the workspace with stdout and stderr on pipes.
func (ws panermCLIWorkspace) run(t *testing.T, bin string, args ...string) panermRun {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = ws.root
	cmd.Env = ws.env()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	exit := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		exit = exitErr.ExitCode()
	} else {
		require.NoError(t, err)
	}
	return panermRun{stdout: stdout.String(), stderr: stderr.String(), exit: exit}
}

func (ws panermCLIWorkspace) roots() map[string]string {
	return map[string]string{"<root>": ws.root, "<home>": ws.home, "<tmpdir>": ws.tmp}
}

var (
	// panermDuration matches rendered durations such as 1.234s, 12ms, or 1m2.5s.
	panermDuration = regexp.MustCompile(`\b[0-9]+(\.[0-9]+)?(ns|µs|ms|s|m|h)([0-9]+(\.[0-9]+)?(ms|s))*\b`)
	// panermStamp matches the time-and-random suffix of saved result names.
	panermStamp = regexp.MustCompile(`\b[0-9]{8}-[0-9]{6}-[0-9]+\b`)
	// panermFence matches the per-run random fence of debate prompts.
	panermFence = regexp.MustCompile(`AUTOPUS_PART_[0-9a-f]+`)
)

// normalize applies the S16 normalization plus rendered durations, saved
// result stamps, and debate fences, and sorts each block of provider progress
// lines, whose order follows process scheduling.
func (ws panermCLIWorkspace) normalize(t *testing.T, text string) string {
	t.Helper()
	text = s16Normalize(t, []byte(text), ws.roots())
	text = panermDuration.ReplaceAllString(text, "<duration>")
	text = panermStamp.ReplaceAllString(text, "<stamp>")
	text = panermFence.ReplaceAllString(text, "AUTOPUS_PART_<nonce>")
	lines := strings.Split(text, "\n")
	for start := 0; start < len(lines); start++ {
		end := start
		for end < len(lines) && isPanermProgressLine(lines[end]) {
			end++
		}
		sort.Strings(lines[start:end])
		start = end
	}
	return strings.Join(lines, "\n")
}

func isPanermProgressLine(line string) bool {
	return strings.HasPrefix(line, "[⏳] ") || strings.HasPrefix(line, "[✓] ") || strings.HasPrefix(line, "[✗] ")
}

// calls returns the normalized provider calls in sorted order: providers run
// concurrently, so only the multiset of calls is deterministic.
func (ws panermCLIWorkspace) calls(t *testing.T) []string {
	t.Helper()
	records, err := filepath.Glob(filepath.Join(ws.argvDir, "call.*"))
	require.NoError(t, err)
	calls := make([]string, 0, len(records))
	for _, record := range records {
		data, err := os.ReadFile(record)
		require.NoError(t, err)
		calls = append(calls, ws.normalize(t, strings.ReplaceAll(string(data), "\x1f", " | ")))
	}
	sort.Strings(calls)
	return calls
}

// snapshotTree lists every file under dir as "path mode sha256", sorted.
func snapshotTree(t *testing.T, dir string) []string {
	t.Helper()
	var entries []string
	require.NoError(t, filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		digest := ""
		if entry.Type().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			sum := sha256.Sum256(data)
			digest = hex.EncodeToString(sum[:])
		}
		entries = append(entries, rel+" "+info.Mode().String()+" "+digest)
		return nil
	}))
	sort.Strings(entries)
	return entries
}
