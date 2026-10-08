//go:build unix

package cli

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/insajin/autopus-adk/pkg/brainstorm"
	"github.com/insajin/autopus-adk/pkg/config"
	"github.com/insajin/autopus-adk/pkg/healthband"
)

// Fixtures of the flag-on diagnose tests (SPEC-SIGMABAND-002 T8): plan task
// T7's executor fixture (a user checkout with a bare origin, <lp> under a
// temp user cache directory, T2's store as the ledger) behind a diagnoser
// that enableLocalPatch switched on, and a fake claude binary that answers
// each call with its own stream.

// dlpFakeClaudeScript records call n's argv, cwd, environment, and stdin
// under DLP_FAKE_LOG and prints stream.<n>, else stream.
const dlpFakeClaudeScript = `#!/bin/sh
n=$(( $(cat "$DLP_FAKE_LOG/count" 2>/dev/null || echo 0) + 1 ))
echo "$n" > "$DLP_FAKE_LOG/count"
printf '%s\n' "$@" > "$DLP_FAKE_LOG/argv.$n"
pwd -P > "$DLP_FAKE_LOG/cwd.$n"
env > "$DLP_FAKE_LOG/env.$n"
cat > "$DLP_FAKE_LOG/stdin.$n"
if [ -f "$DLP_FAKE_LOG/stream.$n" ]; then cat "$DLP_FAKE_LOG/stream.$n"; else cat "$DLP_FAKE_LOG/stream"; fi
`

// dlpFakeClaude is the fake claude binary on PATH and its record.
type dlpFakeClaude struct{ logs string }

func installDLPFakeClaude(t *testing.T) dlpFakeClaude {
	t.Helper()
	bin, logs := t.TempDir(), t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(bin, "claude"), []byte(dlpFakeClaudeScript), 0o755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DLP_FAKE_LOG", logs)
	return dlpFakeClaude{logs: logs}
}

// answer sets the stream of call n; n 0 sets the stream of every other call.
func (f dlpFakeClaude) answer(t *testing.T, n int, stream string) {
	t.Helper()
	name := "stream"
	if n > 0 {
		name += "." + strconv.Itoa(n)
	}
	require.NoError(t, os.WriteFile(filepath.Join(f.logs, name), []byte(stream), 0o600))
}

// calls is the number of provider calls.
func (f dlpFakeClaude) calls(t *testing.T) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.logs, "count"))
	if os.IsNotExist(err) {
		return 0
	}
	require.NoError(t, err)
	n, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	return n
}

func (f dlpFakeClaude) record(t *testing.T, kind string, n int) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.logs, kind+"."+strconv.Itoa(n)))
	require.NoError(t, err)
	return string(data)
}

func (f dlpFakeClaude) argv(t *testing.T, n int) []string {
	t.Helper()
	return strings.Split(strings.TrimSuffix(f.record(t, "argv", n), "\n"), "\n")
}

// dlpDiagnosisText is the S4 diagnosis reply.
const dlpDiagnosisText = "### Summary\nThe flaky step failed in pkg/foo/foo.go.\n"

// dlpClaim is a diagnose claim of ci.failure_rate:CI at sample key 1042
// that opens episode e1042 at tier, with a complete evaluation record.
func dlpClaim(id string, tier int, lease time.Time) healthband.DueClaim {
	n, x, mu, sd, sdEff, z := 20, 0.75, 0.0, 0.0, 0.25, 3.0
	constants := healthband.DefaultConstants()
	claim := healthband.Claim{ID: id, Kind: healthband.ClaimKindDiagnose, Owner: bandDiagnoseOwner, LeaseUntil: lease}
	event := healthband.Event{
		Schema: healthband.SchemaBandEvaluation, Seq: 42, Kind: healthband.EventKindEvaluation,
		Evaluation: healthband.Evaluation{
			Series: lpSeries, SampleKey: "1042", N: &n, X: &x, Mu: &mu, SD: &sd, SDEff: &sdEff, Z: &z, Tier: &tier, Constants: &constants,
		},
		Action: healthband.ActionDiagnose, EpisodeID: "e1042", Claims: []healthband.Claim{claim},
	}
	return healthband.DueClaim{Claim: claim, Series: lpSeries, SampleKey: "1042", EpisodeID: "e1042", Tier: tier, Event: event}
}

// dlpPatchClaim is the plan's claim record of the S4 local_patch claim.
func dlpPatchClaim() healthband.LocalPatchRecord {
	return healthband.LocalPatchRecord{
		Kind: healthband.LocalPatchKindClaim, ClaimID: lpPatchClaimID, DependsOn: lpDiagnoseClaimID,
		LeaseUntil: lpT0.Add(1800 * time.Second), Series: lpSeries, EpisodeID: "e1042",
	}
}

// dlpWorld is one flag-on diagnoser over the executor fixture.
type dlpWorld struct {
	*lpFixture
	fake      dlpFakeClaude
	diagnoser *bandDiagnoser
	after     func(context.Context, healthband.DueClaim, healthband.ClaimOutcome, healthband.Recorded)
	reports   []healthband.LocalPatchRecord
}

// newDLPWorld switches a diagnoser of the fixture's checkout on with plan's
// records; a nil harness takes the S13 configuration.
func newDLPWorld(t *testing.T, harness *config.HarnessConfig, plan ...healthband.LocalPatchRecord) *dlpWorld {
	t.Helper()
	if harness == nil {
		harness = lpS13Harness()
	}
	w := &dlpWorld{fake: installDLPFakeClaude(t), lpFixture: newLPFixture(t, harness)}
	logs := []healthband.RunLog{{RunID: 4242, Attempt: 1, Evidence: healthband.SanitizeCILog("step 3 failed\n", false, w.repo)}}
	w.diagnoser = newBandDiagnoser(w.repo, harness, false, bandStaticEvidence{logs: logs})
	cacheDir := t.TempDir()
	w.diagnoser.now = func() time.Time { return lpT0 }
	w.diagnoser.bsOptions = brainstorm.Options{CacheDir: func() (string, error) { return cacheDir, nil }}
	w.after = w.diagnoser.enableLocalPatch(w.patcher, plan, func(result healthband.LocalPatchRecord) {
		w.reports = append(w.reports, result)
	})
	return w
}

// claim runs one diagnose claim and then the AfterRecord hook, as phase B
// and C do, and returns the diagnose outcome.
func (w *dlpWorld) claim(t *testing.T, claim healthband.DueClaim) healthband.ClaimOutcome {
	t.Helper()
	outcome := w.diagnoser.Run(context.Background(), claim)
	w.after(context.Background(), claim, outcome, healthband.Recorded{ClaimID: claim.ID})
	return outcome
}

func (w *dlpWorld) bs(t *testing.T, id string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(w.repo, ".autopus", "brainstorms", id+".md"))
	require.NoError(t, err)
	return string(data)
}

// results are the store's result records in seq order.
func (w *dlpWorld) results() []healthband.LocalPatchRecord {
	var results []healthband.LocalPatchRecord
	for _, record := range w.ledger.records() {
		if record.Kind == healthband.LocalPatchKindResult {
			results = append(results, record)
		}
	}
	return results
}

// worktrees lists the band worktrees that git still registers.
func (w *dlpWorld) worktrees(t *testing.T) []string {
	t.Helper()
	var paths []string
	for _, line := range strings.Split(w.git(w.repo, "worktree", "list", "--porcelain"), "\n") {
		if path, ok := strings.CutPrefix(line, "worktree "); ok && strings.HasPrefix(path, w.lp()) {
			paths = append(paths, path)
		}
	}
	return paths
}
