package cli_test

// SPEC-EDITGUARD-001 S14 and REQ-EG-24: guard process wall time stays at or
// below 150 ms at p95 over 200 warm invocations each of S1 row 1 and the
// .claude/settings.json payload. A wall-clock budget on a loaded scheduler
// measures the load as much as the guard (Makefile PROCESS_HEAVY_TESTS), so
// the benchmark runs only on request: AUTOPUS_EDITGUARD_LATENCY=1 times a
// five-platform `auto init` project, and AUTOPUS_EDITGUARD_LATENCY_ROOT=<dir>
// times an existing project instead, such as the autopus-adk repository with
// its five manifests (the guard only reads it).

import (
	"bytes"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	latencyEnv     = "AUTOPUS_EDITGUARD_LATENCY"
	latencyRootEnv = "AUTOPUS_EDITGUARD_LATENCY_ROOT"
	latencyRuns    = 200
	latencyWarmup  = 20
	latencyBudget  = 150 * time.Millisecond
	hookDenyPrefix = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny",`
)

func TestEditGuardBinary_LatencyBudget(t *testing.T) {
	if os.Getenv(latencyEnv) != "1" {
		t.Skipf("set %s=1 to time the guard binary (S14)", latencyEnv)
	}
	bin := buildGuardBinary(t)
	root := os.Getenv(latencyRootEnv)
	if root == "" {
		root = corpusProject(t)
	}
	manifests, err := filepath.Glob(filepath.Join(root, ".autopus", "*-manifest.json"))
	require.NoError(t, err)
	t.Logf("root %s: %d manifests", root, len(manifests))

	cases := []struct {
		label, stdin string
		deny, budget bool
	}{
		{"S1 row 1 " + binSkill, binEdit(root, "Edit", binSkill), true, true},
		{".claude/settings.json", binEdit(root, "Edit", ".claude/settings.json"), false, true},
		// Reference only: an empty payload is decided before any file is read,
		// so it shows the process start-up floor of the same binary.
		{"start-up floor (empty stdin)", "", false, false},
	}
	for _, c := range cases {
		got := guardProc(t, bin, root, "claude-code", c.stdin)
		if c.deny {
			require.True(t, strings.HasPrefix(got.stdout, hookDenyPrefix), "%s: %q", c.label, got.stdout)
		} else {
			require.Empty(t, got.stdout, c.label)
		}
		for range latencyWarmup {
			timeGuard(t, bin, root, c.stdin)
		}
		samples := make([]time.Duration, latencyRuns)
		for i := range samples {
			samples[i] = timeGuard(t, bin, root, c.stdin)
		}
		slices.Sort(samples)
		p50, p95 := percentile(samples, 0.50), percentile(samples, 0.95)
		t.Logf("%s: p50 %v, p95 %v, min %v, max %v over %d warm runs", c.label,
			p50.Round(10*time.Microsecond), p95.Round(10*time.Microsecond),
			samples[0].Round(10*time.Microsecond), samples[len(samples)-1].Round(10*time.Microsecond), latencyRuns)
		if c.budget {
			assert.LessOrEqual(t, p95, latencyBudget, "%s p95 over the REQ-EG-24 budget", c.label)
		}
	}
}

// timeGuard is one guard process's wall time as its parent sees it, from the
// spawn to the reaped exit with stdout captured the way a host captures it.
func timeGuard(t *testing.T, bin, dir, stdin string) time.Duration {
	t.Helper()
	cmd := exec.Command(bin, "guard", "edit", "--platform", "claude-code")
	cmd.Dir = dir
	cmd.Env = []string{"PATH=/usr/bin:/bin", "HOME=" + dir}
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	start := time.Now()
	err := cmd.Run()
	elapsed := time.Since(start)
	require.NoError(t, err, stderr.String())
	return elapsed
}

// percentile is the nearest-rank percentile of sorted samples.
func percentile(sorted []time.Duration, p float64) time.Duration {
	rank := int(math.Ceil(p * float64(len(sorted))))
	return sorted[max(rank, 1)-1]
}
