package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/config"
)

func TestDoctorWarningAdvice_NamesSections(t *testing.T) {
	got := doctorWarningAdvice([]string{"omp", "Hygiene"})
	assert.Contains(t, got, "nothing failed")
	assert.Contains(t, got, "omp, Hygiene")
	assert.NotContains(t, got, "auto update",
		"a warning-only report must not send the reader to the installer")

	assert.Contains(t, doctorWarningAdvice(nil), "[WARN] lines above")
}

func TestOMPDoctorChecksFailed_DistinguishesWarnFromFail(t *testing.T) {
	degraded := []jsonCheck{{ID: "a", Status: "pass"}, {ID: "b", Status: "warn"}}
	assert.False(t, ompDoctorChecksHealthy(degraded))
	assert.False(t, ompDoctorChecksFailed(degraded),
		"degraded model routing warns; it is not a managed-surface failure")

	broken := append(degraded, jsonCheck{ID: "c", Status: "fail"})
	assert.True(t, ompDoctorChecksFailed(broken))
}

// TestRunDoctorText_FailBoxRequiresAnErrorLine pins the invariant the result
// box used to break: it said FAIL over a page with no ERROR line, so nobody
// could tell what failed. Whatever this fixture produces, a FAIL box must sit
// under at least one ERROR line and a WARN box under none.
func TestRunDoctorText_FailBoxRequiresAnErrorLine(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, config.Save(dir, config.DefaultFullConfig("verdict-proj")))

	var buf bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&buf)
	require.NoError(t, runDoctorText(cmd, doctorOptions{dir: dir}))
	out := buf.String()

	hasError := strings.Contains(out, "[ERROR]")
	switch {
	case strings.Contains(out, "FAIL"):
		assert.True(t, hasError, "a FAIL verdict must name what failed:\n%s", out)
	case strings.Contains(out, "Warnings found"):
		assert.False(t, hasError, "an ERROR line must not be summarised as a warning:\n%s", out)
	default:
		require.Contains(t, out, "All checks passed", "the result box prints one of three verdicts")
		assert.False(t, hasError)
	}
}
