package cli

// Help text for `auto react band` (SPEC-SIGMABAND-001 REQ-21, SPEC-SIGMABAND-
// 002 REQ-13). The command in react_band.go reads these constants. The
// detector constants they state are fixed in pkg/healthband, and
// react_band_help_test.go checks the two agree; the local patch sentences
// are docs/health-band.md's, which react_band_help_localpatch_test.go checks.

const reactBandShort = "Score CI and canary history against a σ-band; diagnose anomalies read-only"

const reactBandLong = `auto react band keeps a local history of trusted CI runs and executed canary
runs under .autopus/metrics/, scores every series against its own baseline,
and responds by tier. With the default configuration the command never
changes a git ref, a worktree, or GitHub state, and the diagnosis agent is
read-only in every tier. With health_band.allow_local_patch: true, an episode
that opens at tier 3 can also get one local patch outside the repository;
band still changes no GitHub state.

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
  Tier 3 is diagnosis-only while health_band.allow_local_patch is false, the
  default: it opens no pull request, pushes nothing, and applies no patch. A
  repeat tier 2 or 3 inside an open episode is suppressed.

Local patch (tier 3, opt-in)
  health_band.allow_local_patch: true gives an episode that opens at tier 3
  at most one local patch, and only after its confined diagnosis succeeded:
  a local branch autopus/band/<key>, a worktree <lp>/<key>/worktree/, and a
  patch file <lp>/<key>.patch, where <lp> is
  <user cache directory>/autopus/local-patches/<repo-hash>, so the
  artifacts live in autopus/local-patches under the user cache directory.
  Inside the repository band writes only the BS file and the records under
  .autopus/metrics/, and band never pushes, fetches, or opens a pull
  request: a human reviews the patch file and pushes the branch by hand.
  Band never runs tests, builds, or the proposed change. The BS ends its
  추천 방향 section with pointer lines to these paths and this warning:
    Reviewer warning: this local branch holds a patch that an AI model
    derived from untrusted CI logs and that nothing has run. Read the whole
    patch file before you open the worktree in an IDE, run any command or
    agent in it, or push the branch, because repository hooks and tool
    configuration files run on checkout, commit, and build, and the edit
    guard does not cover that worktree.
  The outcome, the changed files, and the requested and actual model of the
  patch request appear in the run output (local_patches[] with --format
  json) with this warning:
    This patch was derived by an AI model from untrusted CI logs; read the
    whole patch file before running anything.

  Only a subprocess claude provider can run a confined diagnosis. While the
  flag is true, every diagnosis, a tier-2 diagnosis included, and every
  patch request runs on a claude CLI subprocess with --restricted whose only
  working directory is a band worktree; any other provider gives
  unavailable(provider_unconfined). With health_band.local_patch_provider
  set, band runs the claude named by health_band.local_patch_provider as a
  CLI subprocess whatever its orchestra backend, while orchestra keeps that
  backend. The expected deployment is the claude CLI signed in with a Claude
  subscription (claude auth login); it needs no API key. An API-key-only
  deployment exports ANTHROPIC_API_KEY for the band run instead.

  A refused or failed claim ends failed:<code>, keeps the BS, removes what
  it created unless it cannot prove an artifact its own and unchanged, and
  exits 0. Recovery runs in every run but --dry-run while
  .autopus/metrics/localpatch-events.jsonl exists, whatever the flag. A
  checkout that a crash of band interrupted is kept as worktree_incomplete;
  look at it, then remove it with git worktree remove --force --force <path>.

Exit status
  Every completed evaluation exits 0, including insufficient samples, a
  missing gh or provider, an unknown --series, and a locked store; reason
  codes appear in the output. Invalid flags and an unreadable or invalid
  autopus.yaml exit non-zero before anything runs. A metric store band cannot
  read or write, such as a symlinked .autopus/metrics, ends the run non-zero
  after the report. --dry-run writes nothing and calls no provider.

Configuration
  health_band.diagnosis_provider picks the diagnosis provider first; without
  it band uses orchestra.judge, then the first configured provider. While
  health_band.allow_local_patch is true, the confined provider rule above
  replaces this choice. Upgrade every auto binary that reads this autopus.yaml
  before you set health_band: older binaries decode autopus.yaml strictly and
  reject the unknown key. Upgrade every auto binary that reads this
  autopus.yaml before you set health_band.allow_local_patch or
  health_band.local_patch_provider: a binary without SPEC-SIGMABAND-002
  rejects a file that sets either key.

Scheduling
  band runs no daemon or scheduler. Run it from cron, for example
    0 7 * * * cd /path/to/repo && auto react band >>"$HOME/band.log" 2>&1
  or register the same command with /auto schedule in a coding CLI session.
  See docs/health-band.md for the files, reason codes, and reset steps.`
