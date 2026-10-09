# Health band: σ-band response to CI and canary signals

`auto react band` keeps a bounded local history of CI and canary results under
`.autopus/metrics/`, scores each series against its own recent baseline with a
block-based mean ± σ detector, and responds by tier. Tier 1 is logged. Tier 2
and tier 3 get one read-only diagnosis per anomaly, written as a `BS-BAND-NNN`
brainstorm that `/auto plan --from-idea` can triage. With the default
configuration the command never changes a git ref, a worktree, or GitHub
state, and the diagnosis agent is read-only in every tier. With
`health_band.allow_local_patch: true`, an episode that opens at tier 3 can
also get one local patch outside the repository (see
[Local patch](#local-patch-tier-3-opt-in)); band still changes no GitHub state.

Defined by SPEC-SIGMABAND-001; the local patch flow by SPEC-SIGMABAND-002.
`auto react check` and `auto react apply` are unchanged, and no hook runs band.

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
- A metric store that git tracks (any path in the index that is
  `.autopus/metrics/` or lies in it on disk, however the index spells it) came
  with the repository, not from this machine's runs. Before it reads or locks
  the store, band checks this with a read-only
  `git -c core.fsmonitor=false ls-files -z -- ':(icase,glob).autopu*/metric*' ':(icase,glob).autopu*/metric*/**'`,
  run without an inherited `GIT_LITERAL_PATHSPECS`, `GIT_GLOB_PATHSPECS`,
  `GIT_NOGLOB_PATHSPECS`, or `GIT_ICASE_PATHSPECS`, records `store_tracked`,
  and changes nothing, under `--dry-run` as well. git lists every path under
  any `.autopu*/metric*` spelling, and band judges each by identity: a listed
  path counts when its entry at the store's depth is the same file as
  `.autopus/metrics/`. So `.autopus/METRICS/` and `.autopus/metricſ/` (a long
  s, U+017F) count on a file system that resolves them to the store, such as
  APFS, and a tracked symlink at the store path counts, while another
  directory such as `.autopus/metrics-archive/` does not. When band cannot
  settle identity, because a stat of either path fails or the listing is cut
  short or above 64 KiB, the store counts as tracked, whatever git's exit
  status: a project that tracks another `.autopu*/metric*` path is therefore
  refused until `.autopus/metrics/` exists. `core.fsmonitor=false` keeps git
  from starting an fsmonitor command that the repository's own config names.
  Run `git rm -r --cached .autopus/metrics`, or the same for the spelling
  `git ls-files` shows, to untrack it. Without git on `PATH`, outside a git
  repository, or in a repository git refuses to read (for example one it
  reports as dubious ownership), git lists nothing, and band treats the store
  as untracked: there is no index it can read that could have brought store
  files in.

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
patterns, band redacts JSON members and single-quoted Python dict items whose
key names a credential (password, secret, token, AccessKey, Authorization,
Cookie, ...), URL credentials (`scheme://user:pass@`, `scheme://:pass@`, and a
token as the user of an http(s) URL, `https://<token>@host` and
`https://<token>:@host`), `Authorization: token|Basic|Bearer|Digest` headers,
also with the header name or the value quoted (`"Authorization": Bearer ...`,
`Authorization: 'Bearer ...'` in a JS object), `Cookie:` and
`Set-Cookie:` header values, JSON Web Tokens, PEM, PGP, and SSH2 private key
blocks, `AccountKey=` and `SharedAccessKey=` values, and prefixed tokens such
as `glpat-`, `ghp_`, `gho_`, `github_pat_`, `sk-`, and `pypi-`; each records
`secret_risk`. A filtered series id is still repository text, so outside the
BS title line and the next-step command it is written as inline code. Events
and state hold numbers, filtered ids, reason codes, and manifest hashes only.

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

**Tier 3 is diagnosis-only** while `health_band.allow_local_patch` is false,
the default. Tier 3 is recorded as tier 3 in the event and in the BS file, and
it gets exactly the tier 2 response: one read-only diagnosis. It opens no pull
request, creates no branch or worktree, and applies no patch. With
`health_band.allow_local_patch: true`, an episode that opens at tier 3 can also
get one local patch: a local branch, a worktree, and a patch file outside the
repository ([Local patch](#local-patch-tier-3-opt-in)). Even then band opens no
pull request and pushes nothing. There is no draft pull request path:
`health_band.allow_draft_pr` stays an unknown key that strict decoding rejects.

## Diagnosis

- One provider is chosen: `health_band.diagnosis_provider`, otherwise
  `orchestra.judge`, otherwise the lexicographically first configured provider.
  No other provider is tried after the choice. While
  `health_band.allow_local_patch` is true, the confined provider rule of
  [Local patch](#local-patch-tier-3-opt-in) replaces this choice for every
  diagnosis, a tier-2 diagnosis included.
- The provider is projected through the shared read-only policy and refused
  unless the projection keeps every read-only control: claude needs
  `--permission-mode plan` and `--tools=Read,Grep,Glob`, codex needs
  `--sandbox read-only`, gemini needs `--mode plan` and `--sandbox`, and an
  OMP-backed provider runs only through its routed backend with a read-only
  sandbox and the tools glob, grep, and read. The call times out after 600 s.
- The provider starts without `GH_TOKEN`, `GITHUB_TOKEN`,
  `GH_ENTERPRISE_TOKEN`, `GITHUB_ENTERPRISE_TOKEN`, the GitHub Actions tokens
  `ACTIONS_ID_TOKEN_REQUEST_TOKEN`, `ACTIONS_ID_TOKEN_REQUEST_URL`, and
  `ACTIONS_RUNTIME_TOKEN`, and the AWS, Google Cloud, and Azure credential
  variables, including `AWS_WEB_IDENTITY_TOKEN_FILE`, every `AWS_CONTAINER_*`
  variable, and `AWS_BEARER_TOKEN_BEDROCK`, on the subprocess path and the OMP
  route alike. A provider that authenticates only through one of them (for
  example Claude on Bedrock or Vertex AI) is therefore unavailable to band.
  Its output is bounded while it runs: band keeps the first 1 MiB and drops
  the rest.
- When no provider can run, the BS file is still written with the evidence and
  `diagnosis_status: unavailable(<reason>)`. The reasons are
  `provider_unconfigured`, `provider_unsupported`, `provider_policy_rejected`,
  `provider_policy_incomplete`, `provider_backend_unavailable`,
  `provider_missing`, `provider_timeout`, `provider_exit_nonzero`, and
  `provider_empty_output`; with `health_band.allow_local_patch` set, also
  `provider_unconfined` and `worktree_unavailable`.
- The BS file is `<project>/.autopus/brainstorms/BS-BAND-NNN.md`, numbered one
  above the highest `BS-BAND-*` id in the whole repository tree (meta root,
  modules, and nested repositories) under one per-user allocation lock. It
  follows the `/auto idea` brainstorm format, and its last step is
  `/auto plan --from-idea BS-BAND-NNN "<series> tier <k> anomaly response"`.
  The file is private to its owner (mode 0600), and the BS directory is
  checked again right before each create.
- The ID scan ignores an entry named like a BS that is a symlink or not a
  regular file, and an ID that an existing file holds is never tried. When the
  highest entry leaves fewer than five IDs above it, the scan also ignores,
  highest first, entries that fail the BS format validator until it finds a
  valid BS. When fewer than five free IDs remain above that BS, the BS takes
  the lowest free ID instead (for example `BS-BAND-001`). So no planted file
  such as `BS-BAND-999999999.md`, a valid BS or not, can block every later BS.
  Band names each ignored path on stderr, and after a lowest-free fallback
  each file above `BS-BAND-999999994` as the holder of the range end.
- A BS that cannot be written ends the claim `failed:<reason>` and the episode
  has no BS: `failed:bs_lock_timeout` (the per-user allocation lock stayed busy
  for 30 s), `failed:bs_id_exhausted` (five consecutive IDs were taken),
  `failed:bs_scope_too_deep` (the project sits more than 8 component levels
  below the top of its repository chain, where a scan from that top would not
  see its IDs, so allocating there could collide), `failed:bs_invalid`, and
  `failed:bs_write_failed`.

## Local patch (tier 3, opt-in)

`health_band.allow_local_patch: true` (default false, SPEC-SIGMABAND-002) gives
an episode that opens at tier 3 at most one local patch, and only after its
confined diagnosis succeeded. Band asks a read-only provider for a proposed
diff, checks it, applies and commits it in an isolated worktree outside the
repository, and keeps three artifacts:

| Artifact | Location |
| --- | --- |
| Local branch | `autopus/band/<key>` |
| Worktree | `<lp>/<key>/worktree/` |
| Patch file | `<lp>/<key>.patch` (mode 0600) |

`<lp>` is `<user cache directory>/autopus/local-patches/<repo-hash>`, so the
artifacts live in autopus/local-patches under the user cache directory
(`~/Library/Caches` on macOS, `$XDG_CACHE_HOME` or `~/.cache` on Linux). Band
creates it with mode 0700 and refuses it (`cache_unavailable`) when it cannot,
when it is not a directory of yours without group or other permissions, when a
component below the user cache directory is a symlink, or when it resolves
into the repository, its git directory, or one of its worktrees. `<repo-hash>` is
the first 12 hex digits of the SHA-256 of the absolute common git directory,
so every worktree of one repository shares it, and `<key>` is
`<series-slug>-<h8>-<episode-id>-<c8>`, where `<c8>` is the first 8 hex digits
of the claim id. Inside the repository band writes only the BS file and the
records under `.autopus/metrics/`, and band never pushes, fetches, or opens a
pull request: a human reviews the patch file and pushes the branch by hand.
The branch is an ordinary local ref, so `git push --all` would publish it.
Band never runs tests, builds, or the proposed change.

Every 3σ BS of a local patch claim gets pointer lines to these paths at the end
of its `## 추천 방향` section, with this warning:

> Reviewer warning: this local branch holds a patch that an AI model derived from untrusted CI logs and that nothing has run. Read the whole patch file before you open the worktree in an IDE, run any command or agent in it, or push the branch, because repository hooks and tool configuration files run on checkout, commit, and build, and the edit guard does not cover that worktree.

The patch request runs after the BS is written, so the outcome, the changed
files with their added and removed line counts, and the requested and actual
model of the patch request appear in the run output (`local_patches[]` with
`--format json`) and in `.autopus/metrics/localpatch-events.jsonl`, with this
warning:

```text
This patch was derived by an AI model from untrusted CI logs; read the whole patch file before running anything.
```

With `--format json`, every path under your home directory is masked as `~`,
so `local_patches[].patch_path` reads `~/Library/Caches/autopus/...` on macOS;
replace `~` with your home directory to open it. The text output prints the
full path.

### Confined provider

Only a subprocess claude provider can run a confined diagnosis. While the flag
is true, every diagnosis, a tier-2 diagnosis included, and every patch request
runs on a claude CLI subprocess whose only working directory is a band
worktree of tracked content at the base commit, so untracked files of your
checkout, such as `.env`, are not in it. The shared read-only projection
(`--permission-mode plan`, `--safe-mode`, `--strict-mcp-config`, and
`--tools=Read,Grep,Glob`) gains `--restricted`, `--verbose`, and
`--output-format stream-json`, and the provider starts with only an allowlist
of inherited variables (`PATH`, `HOME`, `USER`, `LOGNAME`, `SHELL`, `TMPDIR`,
`LANG`, `LC_*`, `TERM`, `TZ`, `XDG_*`, `CLAUDE_CONFIG_DIR`,
`CLAUDE_CODE_OAUTH_TOKEN`, `ANTHROPIC_API_KEY`, the proxy variables in both
letter cases, `SSL_CERT_FILE`, `SSL_CERT_DIR`, and `NODE_EXTRA_CA_CERTS`).
Every other variable is dropped: no other credential, no `GIT_*` variable, no
other `ANTHROPIC_*` variable (such as `ANTHROPIC_BASE_URL`, which would send
the key elsewhere), and no agent-session variable (such as `CLAUDECODE`,
`CLAUDE_CODE_*`, or `MCP_*`) reaches it, so authenticate with the subscription
login, `CLAUDE_CODE_OAUTH_TOKEN`, or `ANTHROPIC_API_KEY`. With
`ANTHROPIC_API_KEY` set, claude can use that key instead of the subscription
login, so unset it in band's environment when the subscription should pay for
band's requests. A provider chosen this way is the only one tried, and it is
chosen before any worktree exists:

1. `health_band.local_patch_provider`, trimmed, when it is not empty. With
   that key set, band runs the claude named by health_band.local_patch_provider
   as a CLI subprocess whatever its orchestra backend, while orchestra keeps
   that backend for reviews, plans, brainstorms, and secure runs. Only `claude`
   can be confined; any other name gives `unavailable(provider_unconfined)`.
2. Otherwise the provider of the default choice
   (`health_band.diagnosis_provider`, then `orchestra.judge`, then the first
   configured provider), used only when it is `claude` and
   `orchestra.providers.claude` has no `backend`.
3. Otherwise `unavailable(provider_unconfined)`: an OMP-backed claude, codex,
   gemini, an unconfigured name, or no name. Band then creates no worktree, and
   the local patch claim ends `failed:diagnosis_unavailable`.

The expected deployment is the claude CLI signed in with a Claude subscription
(claude auth login); it needs no API key. An API-key-only deployment exports
ANTHROPIC_API_KEY for the band run instead. Band itself sets no API key and
never passes `--bare`, which reads only an API key and never the subscription
login. A repository whose orchestra providers are all `backend: omp` sets:

```yaml
health_band:
  allow_local_patch: true
  local_patch_provider: claude
```

with the `claude` CLI on `PATH` (`claude auth status` reports `authMethod`
`claude.ai`), while `orchestra.providers.claude` keeps `backend: omp`.

The model is the `--model` of a claude entry without a `backend`, or, for an
OMP entry, the model ID of an `anthropic/claude-*` selector
(`anthropic/claude-opus-5-5:max` gives `--model claude-opus-5-5`), otherwise
`claude-fable-5-1`. claude can retry a refused request on another model and say
so only in its event stream, so band records the requested and the actual
model of every request; each flag-on diagnosis BS states them on a
`Diagnosis model:` line. A diagnosis answered on another model is recorded as
`model_substituted` and kept. A patch request that claude retried on another
model (a `model_refusal_fallback` event) ends `failed:patch_model_refused`, and
one whose stream does not prove its model (no `init` event, an `init` model
other than the requested `--model`, or an `assistant` event without a model or
on a model other than the `init` model) ends `failed:patch_model_unverified`. A patch request without `--model`, or with an
alias such as `sonnet` that claude expands to a full model ID, therefore always
ends `failed:patch_model_unverified`: configure a full model ID such as
`claude-opus-5-5`.

### What git runs

Band runs git only from its own command allowlist, never through a shell. Every
command starts without any inherited `GIT_*` variable, with
`GIT_ATTR_NOSYSTEM=1`, `GIT_LFS_SKIP_SMUDGE=1`, `GIT_NO_LAZY_FETCH=1`, and
`GIT_NO_REPLACE_OBJECTS=1`, and with `-c` flags that turn off hooks
(`core.hooksPath=/dev/null`), fsmonitor, the global attributes file
(`core.attributesFile=/dev/null`), replace refs, automatic gc and maintenance,
and signing, check tracked symlinks out as plain files (`core.symlinks=false`),
and fix the user, author, and committer identity to
`autopus-band <band@autopus.invalid>`. So no hook, filter, diff or merge
driver, or other command that the repository or its configuration names runs
during checkout, apply, commit, or format-patch.

Band never runs git-lfs. The git-lfs driver is blanked, so LFS-tracked files
stay pointer files in a band worktree, and a patch that touches a path with a
`filter` attribute at the base, `lfs` included, ends
`failed:path_denied:filter`.

A band worktree checks every tracked symlink out as a plain file that holds the
link text, so no file there leads out of the worktree. Your own git, which runs
without that flag, reports each tracked symlink of the worktree as a type
change (`T` in `git status`); that is expected and not part of the patch.

In your checkout, inside the new worktree before its checkout, and again before
the first object write, band refuses the following with
`git_config_unsafe:<key>`, before any file is checked out or any object is
written:

- an `info/attributes` file of the common git directory that holds any line
  besides blank lines and comments (`git_config_unsafe:info_attributes`);
- any configured `filter.<driver>.clean`, `.smudge`, or `.process` other than
  the plain git-lfs commands (`git-lfs filter-process`, `git-lfs clean -- %f`,
  or `git-lfs smudge -- %f`, optionally with an absolute path to `git-lfs`), in
  every scope: a non-LFS filter that only your global or system configuration
  sets is refused even when no attribute selects it, an intended fail-closed
  limitation, so remove that filter or keep the flag off;
- any `diff.<driver>.textconv` or `.command`, `merge.<driver>.driver`,
  `lfs.extension.*` or `lfs.customtransfer.*` setting,
  `core.alternateRefsCommand`, and a config-defined `hook.<name>.command`;
- a partial clone (`remote.<name>.promisor`, `remote.<name>.partialclonefilter`,
  or `extensions.partialclone`), because its checkout would fetch missing
  objects from the promisor remote;
- a configuration git cannot list (`git_config_unsafe:config_unreadable`).

Band also refuses git older than 2.44 (`git_version_unsupported:<version>`,
such as `git_version_unsupported:2.43`), because git 2.44 added
`GIT_NO_LAZY_FETCH`. The base is the commit of the last fetched
`refs/remotes/origin/<default branch>`, otherwise `refs/heads/<default branch>`
(`base_unavailable` when neither exists); band fetches nothing.

Band also refuses (`failed:path_denied`) a patch to a source that a build or
an IDE sync of the base runs: the directory of a Rust proc-macro crate
(`[lib] proc-macro = true`), a custom `build = "<path>"` script with its
directory (for a script in the crate root, and for `build.rs`, the root's other
`.rs` files), every crate that a build script or a proc-macro crate depends on
by path, transitively, every `[patch]` and `[replace]` path, and every directory
that a Gradle settings file names with `includeBuild`. Band reads these from the
`Cargo.toml`, `settings.gradle`, and `settings.gradle.kts` files of the base
commit, never of the patch, and reads at most 1,024 of them, 256 KiB each and
8 MiB in all, shallowest first. A manifest past a bound, one that is not a
regular file or does not parse, and a settings file with an `includeBuild` that
does not name its directory with a plain string denies its own directory tree
instead, the whole repository for one at the root. Code that a build reaches
another way, such as `#[path]` or `include!`, an included build that a settings
plugin or an applied script declares, or a script that `package.json` or a
Bazel rule runs, is not covered: read the whole patch before any build.

### Limits

- Retention cap: at most 5 kept keys per repository. Every `<lp>/<key>/`
  directory or `<lp>/<key>.patch` file counts, the kept worktree of a done
  claim and an artifact that a cleanup rule kept included. With 5 kept keys, a
  new tier-3 opening is recorded as `local_patch_skipped:cap_reached`, and
  every other flag-on diagnosis, a tier-2 diagnosis included, reports
  `unavailable(worktree_unavailable)` with the code `cap_reached`. Remove
  reviewed local patches to free a slot. Two checkouts of one repository that
  run band at the same moment can overshoot the cap.
- Size: before the checkout band counts the base tree. More than 200,000
  entries or 2 GiB gives `worktree_too_large`, and free space below the
  checkout's size plus 512 MiB gives `disk_insufficient`. The checkout must also
  finish within the 30 s setup deadline: a 150,000-file checkout took 16.5 s on
  APFS, so a repository near the 200,000-entry cap can end `worktree_failed`, a
  limit on availability and not on safety.

### Failures and cleanup

Every refusal and failure ends the claim `failed:<code>` with one `result`
record, keeps the BS, and exits 0. Band removes what the claim created, but
never an artifact that it cannot prove is its own and unchanged. `kept[]` of
the `result` names each kept artifact with its reason: `worktree_modified` (an
edited, staged, untracked, or ignored file, an index flag, or a file inside a
gitlink directory), `head_unrecognized`, `branch_moved`, `patch_modified`,
`git_config_unsafe`, or `worktree_incomplete`.

A checkout that the live run stopped at its deadline, or that failed, is
removed at once with `git worktree remove --force` and takes no retention slot;
only when git refuses that removal is the entry kept as `worktree_incomplete`.
Otherwise `worktree_incomplete` remains for a checkout that a crash of band
interrupted, which the next run's recovery finds: band cannot prove that a
partial checkout holds only its own files, and a `git worktree add`
interrupted that way leaves its entry locked. Look at it, then remove it
yourself:

```sh
git worktree remove --force --force <path>
```

Band itself never passes `--force` twice and never runs `git worktree unlock`
or `git worktree prune`.

Recovery runs in every band run but `--dry-run` while
`.autopus/metrics/localpatch-events.jsonl` exists, whatever the flag, so a claim
interrupted while the flag was on is still cleaned up after it is turned off. It
holds `.autopus/metrics/.recovery.lock`; while another process holds that lock,
the run reports `recovery_locked`, and a claim whose key lock a live process
still holds is reported as `recovery_key_locked` and handled by a later run.
When git cannot resolve the repository or `<lp>`, or a git call is stopped at
its timeout, the run reports `recovery_skipped` and a later run recovers. A
`result` record with `recovered: true` is one that recovery wrote, failed or
done; a result that the live run wrote never carries it.

### Reviewing a local patch

```sh
less <lp>/<key>.patch                             # read the whole patch first
git push origin autopus/band/<key>                # only after review, by hand
git worktree remove --force <lp>/<key>/worktree   # then free the retention slot
git branch -D autopus/band/<key>
rm <lp>/<key>.patch
```

`--force` is needed when the base tracks a symlink, which your git reports as
a type change in the band worktree.

### Codes

| Stage | Codes |
| --- | --- |
| Decision (`local_patch_skipped:<reason>`) | `no_agent`, `superseded_in_batch`, `episode_already_patched`, `cap_reached`, `bs_not_tier3`, `no_opening_claim` |
| Preparation (the diagnosis reports `unavailable(worktree_unavailable)`) | `git_version_unsupported:<version>`, `cache_unavailable`, `artifact_exists`, `cap_reached`, `git_config_unsafe:<key>`, `base_unavailable`, `worktree_too_large`, `disk_insufficient` |
| Worktree | `worktree_failed`, `git_config_unsafe:<key>` |
| Before the patch request | `record_unavailable`, `no_bs`, `diagnosis_unavailable`, `branch_exists`, `lore_unsupported_required:<trailer>`, `lore_rejected` |
| Patch request | `patch_provider_unconfined`, `patch_model_refused`, `patch_model_unverified` |
| Patch policy | `no_patch`, `patch_invalid`, `path_denied`, `path_denied:<class>` (`filter`, `case_collision`, `guard_state`, `fix_lock`, `generated_surface`, `guard_fault`), `patch_content_denied`, `patch_content_denied:control_char`, `patch_content_denied:confusable`, `patch_too_large` |
| Recovery run reasons | `recovery_locked`, `recovery_key_locked`, `recovery_skipped` |

The content checks are heuristics: besides control and invisible characters,
`control_char` covers every space other than U+0020 and the blank symbols
U+2800 and U+1D159, `confusable` an identifier that mixes Latin, Cyrillic, or
Greek letters outside a whole-line comment, and `patch_content_denied`
injection phrases and runs of 40 or more base64, base64url, or hex characters,
so a 40-character identifier is refused as well. Passing them proves nothing:
read the whole patch.
| Apply, commit, branch, patch file | `commit_failed`, `commit_tree_mismatch`, `commit_message_altered`, `branch_failed`, `patch_file_failed` |
| Any step or recovery | `lease_exhausted`, `interrupted`, `record_invalid` |

## Configuration

```yaml
health_band:
  diagnosis_provider: codex
  allow_local_patch: true       # tier-3 local patch; default false
  local_patch_provider: claude  # band-only subprocess claude; default empty
```

`health_band` has three optional keys: `health_band.diagnosis_provider`,
`health_band.allow_local_patch` (default false), and
`health_band.local_patch_provider` (default empty; band reads it only while
`allow_local_patch` is true). Like `diagnosis_provider`, `local_patch_provider`
is not validated at load: a name that cannot be confined is a runtime
`unavailable(provider_unconfined)`, so orchestra commands keep loading the
file. Generated and saved `autopus.yaml` files omit each key while it holds
its default, so a project that never sets them is unaffected, and
`health_band.allow_draft_pr` is still an unknown key.

Upgrade every auto binary that reads this autopus.yaml before you set
health_band. Older binaries decode `autopus.yaml` strictly and reject the
unknown key, which breaks every command on that machine or CI runner. Upgrade
every auto binary that reads this autopus.yaml before you set
health_band.allow_local_patch or health_band.local_patch_provider: a binary
without SPEC-SIGMABAND-002 rejects a file that sets either key.

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
| `.autopus/metrics/localpatch-events.jsonl` | Local patch log: decisions, claims, preparation, stages, and one `result` per claim (`autopus.band_localpatch.v1`); SPEC-SIGMABAND-001 binaries never read it |
| `.autopus/metrics/localpatch-state.json` | Local patch claims per series, written after the log (`autopus.band_localpatch_state.v1`) |
| `.autopus/metrics/.recovery.lock` | Lock of the local patch recovery step; created only while the local patch log exists |

The store keeps the newest 512 observations per series and the newest 2,048
events. A malformed, unknown-schema, or invalid line is skipped and counted.

The store is local-only runtime state. `auto init` and `auto update` add
`.autopus/metrics/` to `.gitignore` (an existing project gets the pattern on its
next `auto update`), sync commit plans exclude it, and
`auto check --hygiene --staged` blocks a staged store file even when a source
change is staged with it.

To reset the history, delete `.autopus/metrics/`. CI history returns on the next
fetch; canary history does not. There is no schema migration.
