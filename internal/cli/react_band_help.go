package cli

// Help text for `auto react band` (SPEC-SIGMABAND-001 REQ-21). The command in
// react_band.go reads these constants. The detector constants they state are
// fixed in pkg/healthband, and react_band_help_test.go checks the two agree.

const reactBandShort = "Score CI and canary history against a σ-band; diagnose anomalies read-only"

const reactBandLong = `auto react band keeps a local history of trusted CI runs and executed canary
runs under .autopus/metrics/, scores every series against its own baseline,
and responds by tier. It never changes a git ref, a worktree, or GitHub state,
and the diagnosis agent is read-only in every tier.

Evidence
  CI      completed push and schedule runs on the remote default branch, read
          with gh run list (default --limit 200). success counts 0; failure,
          timed_out, and startup_failure count 1; other conclusions are
          skipped. The highest attempt of a run replaces earlier attempts.
  canary  every executed auto canary run: verdict FAIL counts 1, PASS and
          WARN count 0. Dry runs and all-SKIPPED runs are not recorded.

Detector (fixed in v1: K=4, W=30, N_min=20, floor=1/K=0.25, ε=1e-9)
  Observations are cut into blocks of K from the newest one backwards. x is
  the failure share of the newest block; the baseline is up to W blocks
  before it. For n baseline blocks with values b:
    μ = Σb / n,  sd = sqrt(Σ(b - μ)² / (n - 1))
    z = (x - μ) / max(sd, 1/K)
  Fewer than K observations give no_current_block. n < N_min gives
  insufficient_samples and no z. The test is one-sided: z < 0 is tier 0.

Tiers (a boundary belongs to the upper tier: tier k means z ≥ k - ε)
  tier 0   z < 1   log; the first tier 0 closes an open episode
  tier 1   z ≥ 1   log
  tier 2   z ≥ 2   diagnose once per episode: one read-only provider run and
                   one BS-BAND-NNN brainstorm for /auto plan --from-idea
  tier 3   z ≥ 3   the same response as tier 2, recorded as tier 3
  Tier 3 is diagnosis-only: it opens no pull request, pushes nothing, and
  applies no patch. A repeat tier 2 or 3 inside an open episode is suppressed.

Exit status
  Every completed evaluation exits 0, including insufficient samples, a
  missing gh or provider, an unknown --series, and a locked store; reason
  codes appear in the output. Invalid flags and an unreadable or invalid
  autopus.yaml exit non-zero before anything runs. A metric store band cannot
  read or write, such as a symlinked .autopus/metrics, ends the run non-zero
  after the report. --dry-run writes nothing and calls no provider.

Configuration
  health_band.diagnosis_provider picks the diagnosis provider first; without
  it band uses orchestra.judge, then the first configured provider. Upgrade
  every auto binary that reads this autopus.yaml before you set health_band:
  older binaries decode autopus.yaml strictly and reject the unknown key.

Scheduling
  band runs no daemon or scheduler. Run it from cron, for example
    0 7 * * * cd /path/to/repo && auto react band >>"$HOME/band.log" 2>&1
  or register the same command with /auto schedule in a coding CLI session.
  See docs/health-band.md for the files, reason codes, and reset steps.`
