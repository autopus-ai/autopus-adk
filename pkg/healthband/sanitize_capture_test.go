package healthband_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/healthband"
)

// writeChunks feeds data to w in fixed chunks, as a subprocess pipe would.
func writeChunks(t *testing.T, w io.Writer, data string, chunk int) {
	t.Helper()
	for len(data) > 0 {
		n := min(chunk, len(data))
		written, err := w.Write([]byte(data[:n]))
		require.NoError(t, err)
		require.Equal(t, n, written)
		data = data[n:]
	}
}

// S11: a 6 MiB log keeps its last 4 MiB at capture and its last 8 KiB after
// redaction, so a failing line in the final 1 KiB survives with size_cap.
func TestTailBuffer_SixMiBLogKeepsTheFailingTail(t *testing.T) {
	t.Parallel()
	filler := strings.Repeat(strings.Repeat("x", 99)+"\n", 6<<20/100)
	raw := filler + "step 42 failed: exit 1\n" + strings.Repeat("y", 499) + "\n"
	capture := healthband.NewTailBuffer(healthband.CILogCaptureBytes)

	writeChunks(t, capture, raw, 32<<10)
	captured, dropped := capture.Captured()
	evidence := healthband.SanitizeCILog(captured, dropped, s11ProjectDir)

	assert.True(t, dropped)
	assert.LessOrEqual(t, len(captured), healthband.CILogCaptureBytes)
	assert.True(t, strings.HasPrefix(captured, strings.Repeat("x", 99)+"\n"), "capture starts at a line boundary")
	assert.Contains(t, evidence.Text, "step 42 failed: exit 1")
	assert.LessOrEqual(t, len(evidence.Text), healthband.CILogExcerptBytes)
	assert.Equal(t, []string{"size_cap"}, evidence.Reasons)
}

// Capture alignment: the tail window starts after the first newline unless
// the byte before it already ended a line; a short stream drops nothing.
func TestTailBuffer_AlignsForwardToALineBoundary(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, data, want string
		dropped          bool
	}{
		{name: "fits", data: "ab\ncd", want: "ab\ncd"},
		{name: "partial first line is dropped", data: "0123\nab\ncd\n", want: "cd\n", dropped: true},
		{name: "window already at a line start", data: "0123\nab\ncd", want: "ab\ncd", dropped: true},
		{name: "no newline keeps a rune-aligned tail", data: "aaaaé" + strings.Repeat("b", 4), want: strings.Repeat("b", 4), dropped: true},
		{name: "long stream is trimmed in memory", data: strings.Repeat("x", 50) + "\nab\ncd", want: "ab\ncd", dropped: true},
	} {
		capture := healthband.NewTailBuffer(5)
		writeChunks(t, capture, tc.data, 2)
		got, dropped := capture.Captured()
		assert.Equal(t, tc.want, got, tc.name)
		assert.Equal(t, tc.dropped, dropped, tc.name)
	}
}

// Provider stdout keeps its first 1 MiB at capture, cut back to a line
// boundary so a token split at the limit cannot leak a prefix.
func TestHeadBuffer_KeepsTheFirstBytesAlignedBack(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, data, want string
		dropped          bool
	}{
		{name: "fits", data: "ab\ncd", want: "ab\ncd"},
		{name: "partial last line is dropped", data: "ab\ncdefgh", want: "ab", dropped: true},
		{name: "limit right before a newline", data: "ab\ncd\nef", want: "ab\ncd", dropped: true},
		{name: "no newline keeps a rune-aligned head", data: "abcdé" + "fgh", want: "abcd", dropped: true},
	} {
		capture := healthband.NewHeadBuffer(5)
		writeChunks(t, capture, tc.data, 3)
		got, dropped := capture.Captured()
		assert.Equal(t, tc.want, got, tc.name)
		assert.Equal(t, tc.dropped, dropped, tc.name)
	}
}

// Untrusted Input Contract item 5: provider output keeps its first 32 KiB
// after redaction, aligned back to a line boundary, and adds size_cap; a
// capture that already dropped bytes adds size_cap even without a cut.
func TestSanitizeProviderOutput_KeepsTheHeadAndReportsSizeCap(t *testing.T) {
	t.Parallel()
	line := strings.Repeat("d", 99) + "\n"
	raw := "## Diagnosis\n" + strings.Repeat(line, 400)

	evidence := healthband.SanitizeProviderOutput(raw, false, s11ProjectDir)
	small := healthband.SanitizeProviderOutput("## Diagnosis\nroot cause: flaky test", true, s11ProjectDir)

	assert.True(t, strings.HasPrefix(evidence.Text, "## Diagnosis\n"))
	assert.LessOrEqual(t, len(evidence.Text), healthband.ProviderExcerptBytes)
	assert.True(t, strings.HasSuffix(evidence.Text, strings.Repeat("d", 99)), "ends on a whole line")
	assert.Equal(t, []string{"size_cap"}, evidence.Reasons)
	assert.Equal(t, "## Diagnosis\nroot cause: flaky test", small.Text)
	assert.Equal(t, []string{"size_cap"}, small.Reasons)
}

// Local paths: the project dir becomes <project> before home directories
// become ~, on every platform spelling.
func TestSanitizeText_RedactsLocalPaths(t *testing.T) {
	t.Parallel()
	raw := "cd /Users/alice/work/repo/pkg; cat /home/bob/.netrc; type C:\\Users\\carol\\x.txt; ls /Users/alice/work/repository"

	evidence := healthband.Sanitize(raw, healthband.SanitizeOptions{ProjectDir: s11ProjectDir, Cut: healthband.KeepHead, Limit: 1 << 10})

	assert.Equal(t, "cd <project>/pkg; cat ~/.netrc; type ~\\x.txt; ls <project>sitory", evidence.Text)
	assert.Empty(t, evidence.Reasons)
}

// A project reached through a symlink is redacted in its resolved spelling
// too, and a root project dir never rewrites every separator.
func TestSanitizeText_RedactsResolvedProjectDirAndIgnoresRoot(t *testing.T) {
	t.Parallel()
	target, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	opts := healthband.SanitizeOptions{ProjectDir: link, Cut: healthband.KeepTail, Limit: 1 << 10}

	evidence := healthband.Sanitize("open "+filepath.Join(target, "go.mod")+" and "+filepath.Join(link, "x"), opts)
	root := healthband.Sanitize("cat /etc/hosts", healthband.SanitizeOptions{ProjectDir: "/", Limit: 1 << 10})

	sep := string(filepath.Separator)
	assert.Equal(t, "open <project>"+sep+"go.mod and <project>"+sep+"x", evidence.Text)
	assert.Equal(t, "cat /etc/hosts", root.Text)
}
