package report

import (
	"html/template"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A write into a path whose parent is a regular file must fail loudly instead
// of silently reporting success with no report on disk.
func TestWriteFileFailsWhenParentIsNotADirectory(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocked")
	require.NoError(t, os.WriteFile(blocker, []byte("not a dir"), 0o644))

	err := WriteFile(Report{SchemaVersion: SchemaVersion}, filepath.Join(blocker, "report.html"))
	require.Error(t, err)
}

// A destination that is an existing directory cannot be replaced by the
// rendered file; the temp file must not survive the failed rename.
func TestWriteFileFailsOnDirectoryDestinationAndCleansUp(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dest := filepath.Join(dir, "report.html")
	require.NoError(t, os.Mkdir(dest, 0o755))

	err := WriteFile(Report{SchemaVersion: SchemaVersion}, dest)
	require.Error(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	for _, entry := range entries {
		assert.False(t, strings.HasPrefix(entry.Name(), ".qa-report-"),
			"a failed write must not leave a temp render behind: %s", entry.Name())
	}
}

// Without a run index there is no evidence directory to sit beside, so the
// report must land on the documented default name rather than an empty path.
func TestDefaultOutputPathWithoutRunIndex(t *testing.T) {
	t.Parallel()
	assert.Equal(t, DefaultReportFile, DefaultOutputPath(""))
}

// Timeline widths are emitted straight into a CSS percentage, so a value out of
// range must be clamped instead of producing a bar wider than its track.
func TestFormatPercentClampsToTrack(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "0.000", formatPercent(-12.5))
	assert.Equal(t, "100.000", formatPercent(1400))
	assert.Equal(t, "12.500", formatPercent(12.5))
}

// A digest short enough to fit must stay intact; truncating it would make two
// distinct captures look identical in the filmstrip.
func TestShortDigestKeepsShortValuesIntact(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "sha256:abc", shortDigest("sha256:abc"))
	assert.Equal(t, "", shortDigest(""))

	long := "sha256:" + strings.Repeat("a", 40)
	got := shortDigest(long)
	assert.Equal(t, "sha256:"+strings.Repeat("a", 12)+"…", got)
}

// Only the base64 image URL the capture projection produces may be admitted
// into a src attribute; anything else must collapse rather than be trusted.
func TestImageSourceAdmitsOnlyBase64ImageURLs(t *testing.T) {
	t.Parallel()
	ok := "data:image/png;base64,iVBORw0KGgo="
	assert.Equal(t, template.URL(ok), imageSource(ok))

	for _, value := range []string{
		"",
		"https://evil.test/x.png",
		"data:text/html;base64,PHNjcmlwdD4=",
		"data:image/png,notbase64",
		`data:image/png;base64,AAA" onerror=alert(1)`,
		"data:image/png;base64,AA\nAA",
	} {
		assert.Empty(t, string(imageSource(value)), value)
	}
}
