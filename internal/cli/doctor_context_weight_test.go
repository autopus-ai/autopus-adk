package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

// loadSetRelPaths mirrors the seven session-load documents independently of the
// production ContextLoadSet table so the fixtures cross-check the implementation
// paths against the SPEC-defined locations (acceptance S1/S6).
var loadSetRelPaths = map[string]string{
	"product.md":      ".autopus/project/product.md",
	"ARCHITECTURE.md": "ARCHITECTURE.md",
	"scenarios.md":    ".autopus/project/scenarios.md",
	"workspace.md":    ".autopus/project/workspace.md",
	"tech.md":         ".autopus/project/tech.md",
	"structure.md":    ".autopus/project/structure.md",
	"canary.md":       ".autopus/project/canary.md",
}

// writeSizedFile writes a file of exactly size bytes at relPath under dir.
func writeSizedFile(t *testing.T, dir, relPath string, size int) {
	t.Helper()
	full := filepath.Join(dir, filepath.FromSlash(relPath))
	require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
	require.NoError(t, os.WriteFile(full, bytes.Repeat([]byte("x"), size), 0o644))
}

// seedLoadSet creates a temp dir and writes each named load-set document at its
// SPEC path with the requested byte size.
func seedLoadSet(t *testing.T, sizes map[string]int) string {
	t.Helper()
	dir := t.TempDir()
	for name, size := range sizes {
		relPath, ok := loadSetRelPaths[name]
		require.Truef(t, ok, "unknown load-set doc %q", name)
		writeSizedFile(t, dir, relPath, size)
	}
	return dir
}

// fixtureA: total 130000B, every doc <= 20000B — only the combined-total branch fires.
var fixtureASizes = map[string]int{
	"product.md": 19000, "ARCHITECTURE.md": 19000, "scenarios.md": 19000,
	"workspace.md": 19000, "tech.md": 19000, "structure.md": 19000, "canary.md": 16000,
}

// fixtureB: total 90000B, every doc <= 20000B — neither threshold fires.
var fixtureBSizes = map[string]int{
	"product.md": 13000, "ARCHITECTURE.md": 13000, "scenarios.md": 13000,
	"workspace.md": 13000, "tech.md": 13000, "structure.md": 13000, "canary.md": 12000,
}

// fixtureC: total 95000B (< combined cap) but product.md is 22000B — only the
// single-document branch fires (acceptance S6 third fixture).
var fixtureCSizes = map[string]int{
	"product.md": 22000, "ARCHITECTURE.md": 12000, "scenarios.md": 12000,
	"workspace.md": 12000, "tech.md": 12000, "structure.md": 12000, "canary.md": 13000,
}

func TestContextLoadSet_CapsSumToRotationTarget(t *testing.T) {
	require.Len(t, ContextLoadSet, 7)
	total := 0
	for _, doc := range ContextLoadSet {
		total += doc.Cap
	}
	// Per-document compression caps (REQ-CLD-005) sum to the 100000B target.
	assert.Equal(t, 100000, total)
}

func TestMeasureContextWeight_TotalOverThreshold(t *testing.T) {
	rep := measureContextWeight(seedLoadSet(t, fixtureASizes))

	assert.Equal(t, 130000, rep.TotalBytes)
	assert.Equal(t, 7, rep.PresentCount)
	assert.True(t, rep.OverTotal, "130000B exceeds the 120000B combined soft cap")
	assert.True(t, rep.warned())
	for _, d := range rep.Docs {
		assert.Falsef(t, d.OverCap, "%s (%dB) must be within the per-doc soft cap", d.Name, d.Bytes)
	}
}

func TestMeasureContextWeight_AllUnderCaps(t *testing.T) {
	rep := measureContextWeight(seedLoadSet(t, fixtureBSizes))

	assert.Equal(t, 90000, rep.TotalBytes)
	assert.Equal(t, 7, rep.PresentCount)
	assert.False(t, rep.OverTotal)
	assert.False(t, rep.warned(), "neither threshold is exceeded")
}

func TestMeasureContextWeight_SingleDocOverCap(t *testing.T) {
	rep := measureContextWeight(seedLoadSet(t, fixtureCSizes))

	assert.Equal(t, 95000, rep.TotalBytes)
	assert.False(t, rep.OverTotal, "95000B stays under the 120000B combined soft cap")
	assert.True(t, rep.warned(), "the single-document branch fires")

	over := map[string]bool{}
	for _, d := range rep.Docs {
		if d.OverCap {
			over[d.Name] = true
		}
	}
	assert.Equal(t, map[string]bool{"product.md": true}, over)
}

func TestCheckContextWeight_TotalOver_WarnsWithMeasuredBytes(t *testing.T) {
	var buf bytes.Buffer
	warned := checkContextWeight(&buf, seedLoadSet(t, fixtureASizes))
	out := buf.String()

	assert.True(t, warned)
	assert.Contains(t, out, "Context Weight")
	assert.Contains(t, out, "exceeds")
	assert.Contains(t, out, "130000", "the WARN line reports the measured combined bytes")
}

func TestCheckContextWeight_UnderCaps_NoWarn(t *testing.T) {
	var buf bytes.Buffer
	warned := checkContextWeight(&buf, seedLoadSet(t, fixtureBSizes))
	out := buf.String()

	assert.False(t, warned)
	assert.NotContains(t, out, "exceeds", "no WARN line is emitted below both thresholds")
	assert.Contains(t, out, "90000", "the OK line still reports the measured combined bytes")
}

func TestCheckContextWeight_SingleDocOver_WarnsPerDoc(t *testing.T) {
	var buf bytes.Buffer
	warned := checkContextWeight(&buf, seedLoadSet(t, fixtureCSizes))
	out := buf.String()

	assert.True(t, warned)
	assert.Contains(t, out, "context doc product.md", "the single-document branch names the offender")
	assert.Contains(t, out, "22000")
	assert.Contains(t, out, "exceeds")
}

func TestCheckContextWeight_NoDocsPresent_SilentSkip(t *testing.T) {
	var buf bytes.Buffer
	warned := checkContextWeight(&buf, t.TempDir())

	assert.False(t, warned)
	assert.Empty(t, buf.String(), "the guard is silent in repos without meta-workspace docs")
}

// TestRunDoctorText_ContextWeight_NonBlocking proves acceptance S6's "does not
// flip allOK": running the full text doctor with an over-weight load set surfaces
// the WARN yet leaves the overall verdict identical to the under-weight run. The
// differential isolates the guard's marginal effect on allOK to zero.
func TestRunDoctorText_ContextWeight_NonBlocking(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, config.DefaultFullConfig("test-proj")))

	runDoctor := func() string {
		var buf bytes.Buffer
		cmd := &cobra.Command{}
		cmd.SetOut(&buf)
		require.NoError(t, runDoctorText(cmd, doctorOptions{dir: dir}))
		return buf.String()
	}
	verdict := func(out string) string {
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "All checks passed") || strings.Contains(line, "Issues found") ||
				strings.Contains(line, "Warnings found") {
				return strings.TrimSpace(line)
			}
		}
		return ""
	}

	for name, size := range fixtureBSizes {
		writeSizedFile(t, dir, loadSetRelPaths[name], size)
	}
	underOut := runDoctor()

	for name, size := range fixtureASizes {
		writeSizedFile(t, dir, loadSetRelPaths[name], size)
	}
	overOut := runDoctor()

	assert.NotContains(t, underOut, "exceeds", "under-weight run emits no context-weight WARN")
	assert.Contains(t, overOut, "Context Weight")
	assert.Contains(t, overOut, "exceeds", "over-weight run surfaces the context-weight WARN")
	assert.Equal(t, verdict(underOut), verdict(overOut), "the advisory guard must not change the doctor verdict")
	require.NotEmpty(t, verdict(overOut), "the doctor result box prints a verdict line")
}

func TestCollectContextWeightChecks_TotalOver_NonBlocking(t *testing.T) {
	report := doctorJSONReport{status: jsonStatusOK}
	report.collectContextWeightChecks(seedLoadSet(t, fixtureASizes))

	require.Len(t, report.checks, 1)
	assert.Equal(t, "doctor.context_weight.total", report.checks[0].ID)
	assert.Equal(t, "warn", report.checks[0].Status)
	assert.Contains(t, report.checks[0].Detail, "130000")
	// Advisory: the warn check must not flip the envelope status.
	assert.Equal(t, jsonStatusOK, report.status)
}

func TestCollectContextWeightChecks_SingleDocOver_NonBlocking(t *testing.T) {
	report := doctorJSONReport{status: jsonStatusOK}
	report.collectContextWeightChecks(seedLoadSet(t, fixtureCSizes))

	require.Len(t, report.checks, 2)
	assert.Equal(t, "doctor.context_weight.total", report.checks[0].ID)
	assert.Equal(t, "pass", report.checks[0].Status)
	assert.Equal(t, "doctor.context_weight.doc.product.md", report.checks[1].ID)
	assert.Equal(t, "warn", report.checks[1].Status)
	assert.Contains(t, report.checks[1].Detail, "22000")
	assert.Equal(t, jsonStatusOK, report.status)
}

func TestCollectContextWeightChecks_UnderCaps_PassOnly(t *testing.T) {
	report := doctorJSONReport{status: jsonStatusOK}
	report.collectContextWeightChecks(seedLoadSet(t, fixtureBSizes))

	require.Len(t, report.checks, 1)
	assert.Equal(t, "pass", report.checks[0].Status)
	assert.Equal(t, jsonStatusOK, report.status)
}

func TestCollectContextWeightChecks_NoDocs_SilentSkip(t *testing.T) {
	report := doctorJSONReport{status: jsonStatusOK}
	report.collectContextWeightChecks(t.TempDir())

	assert.Empty(t, report.checks)
	assert.Equal(t, jsonStatusOK, report.status)
}
