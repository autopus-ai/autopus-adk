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
| `--series <id>` | Evaluate only this series, such as `ci.failure_rate:CI` (repeatable); an unknown id reports `series_not_found` |
| `--limit <n>` | Runs to fetch, 1 to 1000 (default 200) |
| `--json`, `--format text\|json` | Output format |

Text output prints one row per series, sorted by series id, with n/N_min, x,
μ, sd_eff, z, tier, action, and episode, for the newest position the run
evaluated; an absent value prints `-`. Indented lines below a row name its
reason codes and the diagnose claim of the run. A series without a newer
observation reports its checkpoint key instead.

```text
ci.failure_rate:CI n=20/20 x=0.500000 μ=0.000000 sd_eff=0.250000 z=2.000000 tier=2 action=diagnose episode=e1042
  reasons: zero_variance
  claim: diagnose 1042 done bs=BS-BAND-001 diagnosis=ok
```

`--format json` adds one `band.<series>` check per series. Its fields hold the
values of `data.series`, numbers as their JSON text, and an absent value is an
absent key. Under `--dry-run` the action is reported as `planned_action` and
the claim as `planned`.

Every completed evaluation exits 0, including insufficient samples, a missing
gh or provider, an unknown series, and a locked store. The reason codes are in
the output. An invalid flag or an unreadable or invalid `autopus.yaml` exits
non-zero before anything runs.

Two store conditions are intentional fail-closed exceptions to that rule,
because evaluating a store band cannot trust would record wrong results and
diagnose phantom incidents:

- A metric store band cannot read or write, such as a symlinked
  `.autopus/metrics/`, a store path swapped for a symlink or FIFO, or a store
  file above 64 MiB, completes no evaluation.
- A metric store that git tracks (any file under `.autopus/metrics/` in the
  index) came with the repository, not from this machine's runs. Band checks
  this with a read-only `git ls-files` before it reads or locks the store,
  records `store_tracked`, and changes nothing, under `--dry-run` as well. Run
  `git rm -r --cached .autopus/metrics` to untrack it.

In both cases band prints the report it has and exits non-zero, and
`--format json` carries the error in the envelope.

## Evidence

### CI runs

Band resolves the host and `<owner/repo>` from the `origin` remote and calls
gh with `GH_REPO` set to `<owner/repo>`, replacing an inherited value:

```sh
gh auth status --hostname <host>
gh api repos/<owner>/<repo> --hostname <host> --jq .default_branch
gh run list -R <owner/repo> --limit <n> --json databaseId,attempt,conclusion,status,headBranch,event,workflowName,createdAt
```

The host check (`gh auth status`) runs without an injected `GH_HOST`, because
gh treats `GH_HOST` as a configured host and would otherwise check an
environment token such as `GH_ENTERPRISE_TOKEN` against whatever host `origin`
names. An inherited `GH_HOST` is kept for that call only when it already names
the same host. Only the calls after a passed check get `GH_HOST=<host>`. So a
host other than `github.com` counts as GitHub only when gh knows it from its
hosts config (`gh auth login --hostname <host>`) or from an inherited
`GH_HOST` equal to it; a host known only through an environment token is
`remote_not_github`. A `localhost` origin or an IP literal is never GitHub
(`remote_not_github`, no gh call). Every gh call also gets
`GH_PROMPT_DISABLED=1`, `GH_PAGER=cat`, and `NO_COLOR=1`, loses `GH_FORCE_TTY`
and `CLICOLOR_FORCE`, and runs in its own process group, which a timeout kills
as a whole.

- Only completed runs whose event is `push` or `schedule` on the remote
  default branch become observations. Pull request, `workflow_dispatch`,
  `workflow_run`, and other events never count, and neither do other branches.
- `success` counts 0. `failure`, `timed_out`, and `startup_failure` count 1.
  Every other conclusion, such as `cancelled`, is left out.
- Each run id keeps one observation. A higher attempt replaces a lower one.
- The series is `ci.failure_rate:<workflow>`. It stays keyed by the filtered
  workflow name (plus `#<h8>` when filtering changed it), not by the workflow
  id, so a renamed workflow starts a new series.
- A series keeps its newest 512 observations. Once it holds 512, a fetched run
  older than the oldest kept one is not stored again, so `--limit` above 512
  re-adds nothing that compaction dropped; a higher attempt of a kept run is
  still taken.

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
file only inside a fenced untrusted-evidence block. Besides the shared
patterns, band redacts JSON members whose key names a credential (password,
secret, token, AccessKey, ...), URL credentials (`scheme://user:pass@`),
`Authorization: token|Basic|Bearer|Digest` headers, JSON Web Tokens, PEM, PGP,
and SSH2 private key blocks, `AccountKey=` and `SharedAccessKey=` values, and
prefixed tokens such as `glpat-`, `ghp_`, `gho_`, `github_pat_`, and `sk-`;
each records `secret_risk`. A filtered series id is still repository text, so
outside the BS title line and the next-step command it is written as inline
code. Events and state hold numbers, filtered ids, reason codes, and manifest
hashes only.

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
- The provider starts without `GH_TOKEN`, `GITHUB_TOKEN`,
  `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`, and the AWS, Google Cloud,
  and Azure credential variables, on the subprocess path and the OMP route
  alike. A provider that authenticates only through one of them (for example
  Claude on Bedrock or Vertex AI) is therefore unavailable to band. Its output
  is bounded while it runs: band keeps the first 1 MiB and drops the rest.
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
  The file is private to its owner (mode 0600), and the BS directory is
  checked again right before each create.
- The ID scan ignores an entry named like a BS that is a symlink or not a
  regular file. When the highest entries leave no ID in range, it also ignores,
  highest first, entries that fail the BS format validator, so one planted
  `BS-BAND-999999999.md` cannot block every later BS. Band names each ignored
  path on stderr.
- A BS that cannot be written ends the claim `failed:<reason>` and the episode
  has no BS: `failed:bs_lock_timeout` (the per-user allocation lock stayed busy
  for 30 s), `failed:bs_id_exhausted` (five consecutive IDs were taken),
  `failed:bs_scope_too_deep` (the project sits more than 8 component levels
  below the top of its repository chain, where a scan from that top would not
  see its IDs, so allocating there could collide), `failed:bs_invalid`, and
  `failed:bs_write_failed`.

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
