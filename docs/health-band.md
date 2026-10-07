# Health band: σ-band response to CI and canary signals

`auto react band` keeps a bounded local history of CI and canary results under
`.autopus/metrics/`, scores each series against its own recent baseline with a
block-based mean ± σ detector, and responds by tier. Tier 1 is logged. Tier 2
and tier 3 get one read-only diagnosis per anomaly, written as a `BS-BAND-NNN`
brainstorm that `/auto plan --from-idea` can triage. The command never changes
a git ref, a worktree, or GitHub state, and the diagnosis agent is read-only in
every tier.

Defined by SPEC-SIGMABAND-001. `auto react check` and `auto react apply` are
unchanged, and no hook runs band.

## Running it

```sh
auto react band --dry-run       # plan only: no write, no lock, no provider call
auto react band                 # fetch CI runs, evaluate, diagnose due episodes
auto react band --no-agent      # evaluate; BS files carry evidence only
auto react band --format json   # one band.<series> check per series
```

| Flag | Effect |
| --- | --- |
| `--project-dir <dir>` | Project to evaluate (default: the current directory) |
| `--no-fetch` | Skip the gh fetch and evaluate the stored series |
| `--no-agent` | Run no provider; the BS file records `diagnosis_status: skipped(no_agent)` |
| `--dry-run` | Report the planned action without writing a file, taking the lock, or calling a provider |
| `--series <id>` | Evaluate one series such as `ci.failure_rate:CI`; an unknown id reports `series_not_found` |
| `--limit <n>` | Runs to fetch, 1 to 1000 (default 200) |
| `--json`, `--format text\|json` | Output format |

Text output prints one row per series, sorted by series id, with n/N_min, x,
μ, sd_eff, z, tier, action, and episode.

Every completed evaluation exits 0, including insufficient samples, a missing
gh or provider, an unknown series, and a locked store. The reason codes are in
the output. Only an invalid flag or an unreadable or invalid `autopus.yaml`
exits non-zero.

## Evidence

### CI runs

Band resolves the host and `<owner/repo>` from the `origin` remote and calls
gh with `GH_REPO` and `GH_HOST` set to them, replacing inherited values:

```sh
gh auth status --hostname <host>
gh api repos/<owner>/<repo> --hostname <host> --jq .default_branch
gh run list -R <owner/repo> --limit <n> --json databaseId,attempt,conclusion,status,headBranch,event,workflowName,createdAt
```

- Only completed runs whose event is `push` or `schedule` on the remote
  default branch become observations. Pull request, `workflow_dispatch`,
  `workflow_run`, and other events never count, and neither do other branches.
- `success` counts 0. `failure`, `timed_out`, and `startup_failure` count 1.
  Every other conclusion, such as `cancelled`, is left out.
- Each run id keeps one observation. A higher attempt replaces a lower one.
- The series is `ci.failure_rate:<workflow>`.

Without gh, without authentication, on a failed or slow fetch (30 s), without
an `origin` remote, with a non-GitHub remote, or with an unknown default branch,
band skips CI ingest with `gh_missing`, `gh_unauthenticated`, `gh_fetch_failed`,
`no_remote`, `remote_not_github`, or `default_branch_unknown` and still
evaluates the stored series.

### Canary runs

Every executed `auto canary` run appends one observation: verdict FAIL counts
1, PASS and WARN count 0. A dry run and a run whose every check is SKIPPED add
nothing. The series is `canary.failure_rate:<target>`, where the target is the
sorted, de-duplicated lowercase API and frontend hosts with default ports
removed, joined by `+`, or `local` when no URL is given. The canary's
`latest.json`, stdout, JSON envelope, and exit code are unchanged; a failed
append is one stderr warning.

### Untrusted text

Workflow names and canary hosts are filtered to `[A-Za-z0-9 ._:+-]` and at most
80 characters; a name that changed gets `#<first 8 hex of its SHA-256>` and the
reason `identifier_sanitized`. CI logs, react reports, and provider output are
redacted for secrets and local paths before any cut and reach a prompt or a BS
file only inside a fenced untrusted-evidence block. Events and state hold
numbers, filtered ids, reason codes, and manifest hashes only.

## Detector

The constants are fixed in v1: K=4, W=30, N_min=20, floor=1/K=0.25, ε=1e-9.
There is no override.

1. Observations are ordered by observed time and a tie-break: the numeric run
   id for CI, a store-wide sequence for canary.
2. Non-overlapping blocks of K are cut backwards from the newest observation;
   the oldest `count mod K` observations are left out. The current block is the
   newest K observations and the baseline is the up to W blocks before it. A
   block value is b = failures / K, and x is the current block value.
3. Fewer than K observations give `no_current_block`. Fewer than N_min baseline
   blocks give `insufficient_samples`: no z, action log.
4. With n baseline blocks:

   ```text
   μ = Σb / n
   sd = sqrt(Σ(b - μ)² / (n - 1))
   z = (x - μ) / max(sd, 1/K)
   ```

   sd = 0 adds `zero_variance`, and 0 < sd < 1/K adds `variance_floor_applied`.
   The test is one-sided: z < 0 adds `below_baseline` and gives tier 0.
5. The tier is the largest k in {1, 2, 3} with z ≥ k - ε, otherwise 0, so a
   boundary belongs to the upper tier.

Example: twenty baseline blocks of 0.0 and a current block with two failures
in four runs give x = 0.5, μ = 0, sd = 0, and z = (0.5 - 0) / 0.25 = 2.0, which
is tier 2 with `zero_variance`.

Evaluating a position uses only observations at or before it, and recorded
evaluations never change. A late observation or a superseding attempt changes
only later evaluations and is counted as `late_observation`.

## Tiers, actions, and episodes

| Tier | No open episode | Open episode |
| --- | --- | --- |
| none (`no_current_block`, `insufficient_samples`) | log | log |
| 0 (z < 1) | log | log, and the episode closes (`episode_closed`) |
| 1 (z ≥ 1) | log | log |
| 2 (z ≥ 2) or 3 (z ≥ 3) | diagnose, and an episode opens | suppressed (`episode_already_diagnosed`), and the episode's max tier rises |

An episode is one anomaly: it opens at a series' first tier 2 or higher
evaluation, stays open through tier 1 and tierless evaluations, and closes at
the first tier 0. Each episode is diagnosed at most once and gets at most one BS
file. Its id is `e` plus the opening sample key.

A series evaluated for the first time scores only its newest position, so the
first run never diagnoses old incidents. Later runs score every newer position
oldest first. When one run finds due actions for several episodes of a series,
only the newest episode's action runs and the rest are recorded as `suppressed`
with `superseded_in_batch`.

**Tier 3 is diagnosis-only.** Tier 3 is recorded as tier 3 in the event and in
the BS file, and it gets exactly the tier 2 response: one read-only diagnosis.
It opens no pull request, creates no branch or worktree, and applies no patch.
A draft pull request path for tier 3 is planned separately (SPEC-SIGMABAND-002)
behind its own flag; this release rejects `health_band.allow_draft_pr` as an
unknown key.

## Diagnosis

- One provider is chosen: `health_band.diagnosis_provider`, otherwise
  `orchestra.judge`, otherwise the lexicographically first configured provider.
  No other provider is tried after the choice.
- The provider is projected through the shared read-only policy and refused
  unless the projection keeps every read-only control: claude needs
  `--permission-mode plan` and `--tools=Read,Grep,Glob`, codex needs
  `--sandbox read-only`, gemini needs `--mode plan` and `--sandbox`, and an
  OMP-backed provider runs only through its routed backend with a read-only
  sandbox and the tools glob, grep, and read. The call times out after 600 s.
- When no provider can run, the BS file is still written with the evidence and
  `diagnosis_status: unavailable(<reason>)`. The reasons are
  `provider_unconfigured`, `provider_unsupported`, `provider_policy_rejected`,
  `provider_policy_incomplete`, `provider_backend_unavailable`,
  `provider_missing`, `provider_timeout`, `provider_exit_nonzero`, and
  `provider_empty_output`.
- The BS file is `<project>/.autopus/brainstorms/BS-BAND-NNN.md`, numbered one
  above the highest `BS-BAND-*` id in the whole repository tree (meta root,
  modules, and nested repositories) under one per-user allocation lock. It
  follows the `/auto idea` brainstorm format, and its last step is
  `/auto plan --from-idea BS-BAND-NNN "<series> tier <k> anomaly response"`.

## Configuration

```yaml
health_band:
  diagnosis_provider: codex
```

`health_band.diagnosis_provider` is the only key in this release and is
optional. Generated and saved `autopus.yaml` files omit `health_band` while it
holds its default, so a project that never sets it is unaffected.

Upgrade every auto binary that reads this autopus.yaml before you set
health_band. Older binaries decode `autopus.yaml` strictly and reject the
unknown key, which breaks every command on that machine or CI runner.

## Scheduling

Band runs no daemon, hook, or scheduler; schedule it yourself. A daily cron
entry:

```sh
0 7 * * * cd /path/to/repo && auto react band >>"$HOME/band.log" 2>&1
```

Cron starts with a minimal `PATH`, so make sure it reaches `auto`, `gh`, and the
provider CLI, or use absolute paths. Inside a coding CLI session,
`/auto schedule` can register the same command as a recurring task. Running
band again with no new observation appends nothing and leaves the checkpoint
byte-identical.

## Files

| Path | Content |
| --- | --- |
| `.autopus/metrics/ci-runs.jsonl` | CI observations (`autopus.metric_observation.v1`) |
| `.autopus/metrics/canary-runs.jsonl` | Canary observations |
| `.autopus/metrics/band-events.jsonl` | Write-ahead log: one `evaluation` event per evaluated position and one `action_result` event per executed action (`autopus.band_evaluation.v1`) |
| `.autopus/metrics/band-state.json` | Checkpoint written after the events (`autopus.band_state.v1`) |
| `.autopus/metrics/.lock` | Cross-process lock |
| `.autopus/metrics/pending/<claim-id>.json` | A result that could not take the lock; the next run appends it once |

The store keeps the newest 512 observations per series and the newest 2,048
events. A malformed, unknown-schema, or invalid line is skipped and counted.

The store is local-only runtime state. `auto init` and `auto update` add
`.autopus/metrics/` to `.gitignore` (an existing project gets the pattern on its
next `auto update`), sync commit plans exclude it, and
`auto check --hygiene --staged` blocks a staged store file even when a source
change is staged with it.

To reset the history, delete `.autopus/metrics/`. CI history returns on the next
fetch; canary history does not. There is no schema migration.
