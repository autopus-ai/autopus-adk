# SPEC-SIGMABAND-002: σ-band 3σ local patch on an isolated worktree

**Status**: draft
**Created**: 2026-10-06
**Revised**: 2026-10-08 (rev 7: operator decision on OQ-1, the band-only subprocess provider `health_band.local_patch_provider`; rev 6: refreshed against the merged SPEC-SIGMABAND-001 code at main `1943e596`, SPEC-PANERM-001, and SPEC-EDITGUARD-001, and resolves the open rev 5 findings; rev 5: one recovery state table, a separate recovery step with its own lock, claim-unique diagnosis worktrees, chained-lease oracle; rev 2 rescoped the cap to local patch only)
**Domain**: SIGMABAND
**Module**: autopus-adk
**Sibling of**: SPEC-SIGMABAND-001 (implemented and merged; this SPEC consumes its tier-3 episodes, diagnosis, BS, write-ahead log, and claims)
**PRD**: `../SPEC-SIGMABAND-001/prd.md` (FR-14, FR-15, §5.2, user decision D3); deviations are listed in PRD Deviations

## 목적

사용자 결정 D3(3σ 대응 상한)은 2026-10-06 사용자 결정으로 "별도 브랜치 draft PR"에서 "로컬 patch만"으로 바뀌었다. 3σ(기본 OFF
플래그)에서는 저장소 밖 사용자 cache 디렉토리에 격리 worktree를 만들고, 로컬 branch를 만들고, read-only provider가 제안한 patch를
적용한 뒤 `.patch` 파일을 쓴다. 3σ BS에는 그 위치를 가리키는 pointer만 남는다. band는 push·fetch·PR·원격 쓰기를 하지 않는다. 사람이
검토한 뒤 직접 push한다. 신뢰 경계는 남아 있다. 에이전트가 작성한 코드가 사용자 저장소의 branch와 object에 놓이기 때문이다. 그래서
두 provider는 모두 band worktree 안에서만 읽을 수 있는 subprocess claude다. orchestra provider가 모두 OMP backend인 저장소에서는
`health_band.local_patch_provider: claude`가 band에서만 claude를 CLI subprocess로 실행하고, orchestra는 설정된 backend를 그대로
쓴다. git은 checkout·apply·commit·format-patch 동안 repository가 지정한 어떤 명령도 실행하지 않는다. 저장소 root 아래 `.git/` 밖 파일은 BS와 band 기록 파일 외에는 바뀌지 않고 `.git/` 안에는 REQ-14가
허용한 변경만 생기며, test는 자동으로 돌리지 않는다.

## Outcome Boundary

- Outcome Lock: `health_band.allow_local_patch: true`이면, SPEC-SIGMABAND-001이 tier 3으로 연 episode 중 confined 진단이 성공한 것은 최대 한 번
  (모든 guard 통과 시 정확히 한 번) 로컬 산출물을 받는다. 산출물은 로컬 branch `autopus/band/<key>`, worktree `<lp>/<key>/worktree/`,
  patch 파일 `<lp>/<key>.patch`이다. 여기서 `<lp>` = `<UserCacheDir>/autopus/local-patches/<repo-hash>`, `<key>` =
  `<series-slug>-<h8>-<episode-id>-<c8>`이고 `<c8>`은 claim id의 앞 8 hex다. 3σ BS에는 pointer가 기록되고, 원격에는 아무것도 생기지 않는다.
  orchestra provider가 모두 OMP backend인 저장소도 `health_band.local_patch_provider: claude`로 같은 산출물을 받고, orchestra는
  설정된 backend를 유지한다.
- Mandatory requirements: REQ-01–REQ-15 (Priority Must).
- Explicit non-goals: push, fetch, PR 생성·갱신, 원격 ref, GitHub 쓰기 API, CI 실행, test·build 자동 실행, 에이전트 쓰기 권한, tier 2로 열린 뒤
  tier 3으로 오른 episode의 patch, OMP backend의 worktree confinement, orchestra 명령의 provider backend 변경, SPEC-SIGMABAND-001이 소유한 동작. 리뷰어가 worktree를 IDE로 열거나
  그 안에서 명령이나 에이전트를 실행하거나 branch를 push할 때의 실행도 범위 밖이며, BS의 reviewer warning이 이를 알린다.
- Completion evidence: acceptance S1–S14 통과, Completion Debt CD-1–CD-3 해소, security-auditor 리뷰 통과, 모든 run에서 원격 쓰기 0건, 저장소 root
  아래 `.git/` 밖 파일 변경은 BS와 `.autopus/metrics/` 기록 파일뿐.

## Requirements

Priority 열은 Must만 쓴다. 각 문장은 `pkg/spec` parser 문법을 따르며 strict validate의 `ParseEARS`로 15개 모두 인식된다.

| ID | Priority | Source | EARS requirement |
|----|----------|--------|------------------|
| REQ-01 | Must | FR-17, F-052, decisions 2026-10-06 and 2026-10-08 | THE SYSTEM SHALL add `AllowLocalPatch bool` with the tag `yaml:"allow_local_patch,omitempty"` and `LocalPatchProvider string` with the tag `yaml:"local_patch_provider,omitempty"` to `HealthBandConf` in `pkg/config/schema_health_band.go`, amend SPEC-SIGMABAND-001 REQ-15's key list with both keys at this SPEC's sync while `allow_draft_pr` stays rejected, omit each key from generated and saved `autopus.yaml` files while it holds its default, and document that a binary without this SPEC rejects a file that sets either key. |
| REQ-02 | Must | FR-07, F-049, F-072 | WHEN SPEC-SIGMABAND-001's phase A plans an evaluated tier-3 position IF `health_band.allow_local_patch` is true, THEN THE SYSTEM SHALL decide it exactly as the Local Patch Decision Table defines, record a `local_patch` claim that depends on the diagnose claim of a row-4 position, append those records to `.autopus/metrics/localpatch-events.jsonl` under the store lock after 001's `Commit` and before its `Unlock`, and write nothing into SPEC-SIGMABAND-001's files beyond the amendments listed in Related SPECs. |
| REQ-03 | Must | F-039, F-057, PANERM, decision 2026-10-08 | WHERE `health_band.allow_local_patch` is true, THEN THE SYSTEM SHALL run every SPEC-SIGMABAND-001 diagnosis and every patch request with a band worktree at the base SHA as the only working directory, on the subprocess claude that the Local Patch Provider Contract selects, projected by `applyReadOnlyProviderPolicy` with the confined option that adds `--restricted` to the shared projection, without the variables of `bandProviderUnsetEnv` and without any `GIT_*` variable, and record every selection that does not end at that subprocess claude, an OMP-backed claude reached without `health_band.local_patch_provider` included, as `unavailable(provider_unconfined)`. |
| REQ-04 | Must | F-049, F-050, F-068, F-073 | WHEN a `local_patch` claim executes, THEN THE SYSTEM SHALL end it with `failed:record_unavailable` when its diagnosis has no `prep` record, otherwise with the step-1 code of that `prep` record when the code is not ok, otherwise with `failed:worktree_failed` when its `stage` records hold `worktree_failed`, otherwise with `failed:no_bs` when the diagnose outcome has no BS ID, otherwise with `failed:diagnosis_unavailable` when `diagnosis_status` is not ok, and only otherwise start the patch stage, so that no failed preparation or diagnosis leads to a patch request, apply, or commit. |
| REQ-05 | Must | F-043, F-071, F-075 | WHEN band runs git for a local patch, THEN THE SYSTEM SHALL run only the commands of the Git Execution Policy through its own allowlist, leave SPEC-SIGMABAND-001's `checkBandCommand` unchanged, and apply that policy to every git command (scrubbed environment with `GIT_ATTR_NOSYSTEM=1` and `GIT_LFS_SKIP_SMUDGE=1`, hooks off, `core.fsmonitor=false`, an empty `core.attributesFile`, refusal of a non-empty `info/attributes`, of every configured filter, diff, or merge driver outside the git-lfs allowlist, and of every `lfs.extension` or `lfs.customtransfer` setting, and no network command) so that checkout, apply, commit, and format-patch run no command that the repository or its configuration selects beyond the allowlisted git-lfs binary, and THE SYSTEM SHALL never run tests, builds, or the proposed change. |
| REQ-06 | Must | F-016, F-055, F-056 | WHEN band builds the commit message, THEN THE SYSTEM SHALL build and validate it in-process with `lore.BuildCommit` and `lore.Validate` before any `git apply`, refuse with `lore_unsupported_required:<trailer>` when a required trailer is not one of the five that `lore.Validate` recognizes, and confirm after the commit that the commit object's message equals the `-F` file bytes. |
| REQ-07 | Must | F-039, F-047, F-048 | WHEN band requests a patch, THEN THE SYSTEM SHALL build the prompt as the Patch Prompt Contract defines, bound the reply capture at 1 MiB while the provider runs, take the diff from the raw reply without redaction or line removal, and keep the reply in memory until the Patch Policy has accepted it. |
| REQ-08 | Must | F-040, F-058, decision 2026-10-06, EDITGUARD | WHEN a proposed diff is evaluated, THEN THE SYSTEM SHALL decode every path as the Patch Policy defines and accept the diff only when the Patch Policy accepts every decoded path (including the dotenv and credential entries and the edit guard check of item 9), mode, and added line and the change stays within 10 files, 400 changed lines, and 64 KiB, before any command writes to the object database. |
| REQ-09 | Must | decision 2026-10-06, F-070, F-080 | WHEN the Patch Policy accepts a diff, THEN THE SYSTEM SHALL apply it with `git apply --index` in the claim's worktree under `<lp>/<key>/worktree/`, confirm that the worktree's index tree equals the expected tree recorded in `apply_intent`, commit it, create the local branch `autopus/band/<key>` at that commit, write `git format-patch` output to `<lp>/<key>.patch`, and keep the worktree for human review. |
| REQ-10 | Must | decision 2026-10-06 | THE SYSTEM SHALL never push, fetch, create or update a pull request, call a GitHub write API, or create a remote ref in the local patch flow, so that every artifact stays local until a human pushes it. |
| REQ-11 | Must | F-060, F-064, F-068, F-076, F-080, F-083 | IF any guard or step of the local patch flow or of a confined diagnosis fails, or a later run finds a claim interrupted, THEN THE SYSTEM SHALL handle exactly the artifacts that the claim's intent records name under the one Cleanup Rules set, end a live failure with the code of its Local Patch Flow step and an interrupted claim with the state of the Recovery State Table, write the `result` record, keep the SPEC-SIGMABAND-001 BS, never touch an artifact that no intent record of the claim names or that a user changed, and exit 0. |
| REQ-12 | Must | F-032, F-068, F-072 | THE SYSTEM SHALL give a diagnose claim the 990 s budget and a `local_patch` claim the 810 s budget of the Step Timeouts table while `allow_local_patch` is true, execute each `local_patch` claim right after phase C has recorded its diagnose claim, count both budgets in the lease chain of SPEC-SIGMABAND-001's `Plan` through the `PlanOptions` and `ExecuteOptions` hooks that plan task T8 adds in `pkg/healthband/catchup.go` and `claims.go`, and start no step group whose deadline the claim's remaining lease does not cover. |
| REQ-13 | Must | FR-23, decisions 2026-10-06 and 2026-10-08 | THE SYSTEM SHALL document the flag, the `local_patch_provider` key with the subscription claude CLI deployment, the artifact locations, the reviewer warning, the subprocess claude requirement, and the upgrade-before-enable note in the `auto react band` help text, `docs/health-band.md`, and `CHANGELOG.md`, qualify their tier-3 diagnosis-only statements with the flag, and put the Local Patch pointer lines with the reviewer warning in every 3σ BS of a `local_patch` claim. |
| REQ-14 | Must | F-070, F-077 | THE SYSTEM SHALL keep every worktree and patch file of this SPEC under `<UserCacheDir>/autopus/local-patches/<repo-hash>/`, outside the repository, refuse with `cache_unavailable` when that directory cannot be created, change no file under the repository root outside `.git/` except the BS file and the files under `.autopus/metrics/` that SPEC-SIGMABAND-001 and this SPEC own, and change inside `.git/` only `refs/heads/autopus/band/<key>` with its reflog, the `worktrees/<name>/` entry of the claim's worktree, and new objects. |
| REQ-15 | Must | operator decision 2026-10-08 (OQ-1) | WHERE `health_band.allow_local_patch` is true, THEN THE SYSTEM SHALL select the provider of every diagnosis and every patch request by the Local Patch Provider Contract, run a `claude` named by `health_band.local_patch_provider` as a CLI subprocess whatever its `orchestra.providers.claude.backend` says, use SPEC-SIGMABAND-001's selection only while that key is empty and only when it resolves to a subprocess claude, give `unavailable(provider_unconfined)` in every other case, a key naming another provider included, and leave the provider backend of every orchestra command unchanged. |

`<repo-hash>` = first 12 hex digits of the SHA-256 of `git rev-parse --path-format=absolute --git-common-dir`, so every worktree
of one repository shares one directory. `<key>` = `<series-slug>-<h8>-<episode-id>-<c8>`: `<series-slug>` is the series ID
lowercased with every run of characters outside `[a-z0-9]` replaced by one `-`, trimmed of `-`, and cut to 40 bytes;
`<h8>` is `healthband.H8` of the series ID (`pkg/healthband/identifier.go:10`); `<episode-id>` is the episode ID under the
slug rule; `<c8>` is the first 8 hex digits of the 32-hex claim id (`NewClaimID`, `claims.go:44`; `claimIDPattern`,
`wal_validate.go:14`) of the `local_patch` claim, or of the diagnose claim for a diagnosis without one. The key alphabet is
`[a-z0-9-]`, so `<key>` is one valid ref component and one file name on every file system, although 001 sample keys may hold
`.` and `_` and series IDs `:`, `+`, space, and `#`. Two checkouts of one repository, two series, or a run after the store was
deleted never share a `<key>`.

## Configuration

| Key | Type and default | Effect |
|-----|------------------|--------|
| `health_band.allow_local_patch` | bool, `false` | turns the local patch flow on (REQ-01) |
| `health_band.local_patch_provider` | string, empty | while `allow_local_patch` is true, names the provider that band runs as a CLI subprocess for every diagnosis and patch request, whatever `orchestra.providers.<name>.backend` says (REQ-15); only `claude` can be confined; ignored while the flag is false |
| `health_band.diagnosis_provider` | string, empty (SPEC-SIGMABAND-001) | unchanged; while the flag is true, band reads it only through rule 2 of the Local Patch Provider Contract |
| `health_band.allow_draft_pr` | none | stays an unknown key that strict decoding rejects |

Both new keys follow SPEC-SIGMABAND-001 REQ-15: `decodeStrict` (`pkg/config/loader_strict.go:105`) accepts them and still
rejects every other unknown `health_band` key, and generated and saved `autopus.yaml` files omit each key while it holds its
default. At this SPEC's sync, 001 REQ-15's key list becomes `diagnosis_provider`, `allow_local_patch`, and
`local_patch_provider`. Like `diagnosis_provider` (`schema_health_band.go:9-12`), `local_patch_provider` is not validated at
load: a name that cannot be confined is a runtime `unavailable(provider_unconfined)`, never a load failure, so orchestra
commands keep loading the file. A binary without this SPEC rejects a file that sets either key, so the docs say to upgrade
every binary that reads the file first.

Expected deployment for a repository whose orchestra providers are all `backend: omp` (this repository, `autopus.yaml:77-90`):

```yaml
health_band:
  allow_local_patch: true
  local_patch_provider: claude
```

with the `claude` CLI on PATH, signed in with a Claude subscription (`claude auth login`; `claude auth status` reports
`authMethod` `claude.ai`). Band sets no API key and needs none, and `orchestra.providers.claude` keeps `backend: omp` for
reviews, plans, brainstorms, and secure runs.

## Local Patch Decision Table

SPEC-SIGMABAND-001's `(*WAL).Plan` (`pkg/healthband/catchup.go:73-111`) calls this table through the `[NEW]`
`PlanOptions.LocalPatch` hook (plan task T8) once per evaluated tier-3 event, in plan order, while the flag is true. Rows
are checked top down and the first match wins; row 6 is the default. Decisions go to
`.autopus/metrics/localpatch-events.jsonl` (kind `decision`).

| Order | Condition | Decision | Reason |
|-------|-----------|----------|--------|
| 1 | the run has `--no-agent` | skipped | `local_patch_skipped:no_agent` |
| 2 | the event carries `superseded_in_batch` (001 batch rule, `decideSeries`, `catchup.go:117-144`) | skipped | `local_patch_skipped:superseded_in_batch` |
| 3 | `localpatch-state.json` holds a `local_patch` claim for the episode | skipped | `local_patch_skipped:episode_already_patched` |
| 4 | the event's action is `diagnose`, so it opens the episode at tier 3 | claim, `depends_on` = that event's diagnose claim | - |
| 5 | the episode's opening tier is 2 | skipped | `local_patch_skipped:bs_not_tier3` |
| 6 | any other tier-3 position, such as a later position of an episode whose tier-3 opening got no claim (rows 1–2, or the flag was off) | skipped | `local_patch_skipped:no_opening_claim` |

Opening tier: 001's `Episode` (`pkg/healthband/types.go:175-181`) keeps `max_tier` but not the opening tier, so the table takes
the `tier` of the opening event when this plan holds it, else 2 when 001's checkpoint gives the episode `max_tier` 2, else 2
when this SPEC's records hold a `bs_not_tier3` decision for the episode, else 3.

Phase A (`phaseA`, `internal/cli/react_band.go:183-204`): `Plan` returns the decisions and claims in `[NEW]` `Plan.LocalPatch`.
After `wal.Commit(plan)` and before `Unlock`, their records are appended in one fsynced write through the store helpers
(`appendStoreFile`, `pkg/healthband/store_compact.go:235`). When that append fails, no diagnosis of the run creates a worktree
or starts a provider: each reports `unavailable(worktree_unavailable)`, and the run exits non-zero after the report like a
001 store I/O failure (001 Review Resolution F2).

Phase B: `ExecuteClaims` (`pkg/healthband/claims.go:94-117`) runs the diagnose claim and records its result (phase C,
`RecordResult`, `claims.go:129-163`: an `action_result` event, or a pending file when the lock stays busy). The `[NEW]`
`ExecuteOptions.AfterRecord` hook then runs the `local_patch` claim before the next claim starts, with the diagnose
`ClaimOutcome` (BS ID, diagnosis status) and the sanitized diagnosis and run logs handed over in memory. Both run under one
owner, so the BS ID is bound before the patch stage.

## Local Patch Flow

Every `prep` and `stage` record is appended under SPEC-SIGMABAND-001's store lock, waiting at most `StoreLockWait` (5 s,
`pkg/healthband/store.go:33`) inside its step group's deadline; a failed append ends the claim `failed:record_unavailable`
before the next artifact step, so no artifact exists without its intent record. Before each step group of the Step Timeouts
table, the claim checks that its remaining lease covers that group's deadline plus the cleanup and margin deadlines;
otherwise it ends `failed:lease_exhausted` through the Cleanup Rules, so a live claim never acts after its `lease_until`,
the point from which recovery may act. A diagnosis without a `local_patch` claim runs steps 1–2 with its own `<key>`, intent
records, and `result` record.

| Order | Step | Code on failure |
|-------|------|-----------------|
| 1 | Inside the diagnose claim (`bandDiagnoser.Run`, `internal/cli/react_band_diagnose.go:108`): `<lp>` available, no artifact at `<lp>/<key>/`, `<lp>/<key>.patch`, `<lp>/<key>.diff`, or `refs/heads/autopus/band/<key>`, unsafe configuration check (Git Execution Policy item 3), base SHA (item 4); then one `prep` record with the diagnose claim id, its `lease_until`, the base SHA, and the code (ok or the failure); a failure makes the diagnosis report `unavailable(worktree_unavailable)` and 001 writes the evidence-only BS | `cache_unavailable`, `artifact_exists`, `git_config_unsafe:<key>`, `base_unavailable` |
| 2 | Inside the diagnose claim: `stage` `worktree_intent` (path), `git worktree add --detach <worktree> <base-sha>`, then `stage` `worktree_done` or `worktree_failed`; then the confined diagnosis with cwd `<worktree>` (REQ-03) and the BS, with the BS Record lines when a `local_patch` claim exists; a diagnosis without a `local_patch` claim then applies the Cleanup Rules to its worktree and writes its `result` | `worktree_failed`, 001 REQ-12 reasons |
| 3 | `local_patch` claim: result check in the order of REQ-04 | `record_unavailable`, the `prep` code, `worktree_failed`, `no_bs`, `diagnosis_unavailable` |
| 4 | `refs/heads/autopus/band/<key>` still absent; it can only appear here if something created it after step 1 | `branch_exists` |
| 5 | Lore message built and validated (REQ-06); `stage` `message` with its SHA-256 | `lore_unsupported_required:<trailer>`, `lore_rejected` |
| 6 | Patch request (REQ-07) on the provider of the Local Patch Provider Contract; the reply stays in memory | `patch_provider_unconfined`, 001 REQ-12 reasons |
| 7 | Patch Policy over the raw reply (REQ-08), ending with `git apply --numstat --summary -z --check`; no command writes objects before this step passes | `no_patch`, `patch_invalid`, `path_denied`, `patch_content_denied`, `patch_too_large` |
| 8 | Expected tree: a band temp index (`GIT_INDEX_FILE`, Git Execution Policy item 1) reads the base tree, takes the diff with `git apply --cached`, and `git write-tree` gives the expected tree; `stage` `apply_intent` with the diff's SHA-256 and the expected tree; only then the diff is written to `<lp>/<key>.diff`; `git apply --index` in the worktree; the worktree's index tree (`git write-tree`) equals the expected tree; `stage` `apply_done` | `patch_invalid` |
| 9 | `git commit --no-verify --cleanup=verbatim -F <msg>`; `stage` `commit_done` with `git rev-parse HEAD`; then the commit object's message (after the header's blank line in `git cat-file commit HEAD`) equals the `-F` file bytes | `commit_failed`, `commit_message_altered` |
| 10 | `stage` `branch_intent` with the commit OID; `git update-ref refs/heads/autopus/band/<key> <commit-oid> ""`, which creates the ref only while it is absent; `stage` `branch_done` | `branch_failed` |
| 11 | `stage` `patch_intent` (path); `git format-patch --no-textconv --no-ext-diff --stdout <base-sha>..HEAD` to `<lp>/<key>.patch.tmp-<c8>`; rename to `<lp>/<key>.patch`; `stage` `patch_done` with the patch file's SHA-256 | `patch_file_failed` |
| 12 | Done: keep the worktree, branch, and patch file; delete `<lp>/<key>.diff`; write the `result`. Any failure applies the Cleanup Rules to this claim's intent records; `git worktree prune` is never run | - |

- Claim commit: the OID that `commit_done` records, or, without `commit_done`, the commit whose parent is the `prep` base
  SHA, whose message hashes to the `message` stage, and whose tree is the `apply_intent` expected tree. Recovery finds it
  even when a crash came before `commit_done`; a commit that fails the message comparison is still known by `commit_done`;
  a commit a user amended to another tree is never the claim commit.
- Objects: steps 1–7 write no object. Steps 8–11 write objects; after a failure there they stay unreachable in the shared
  object database until the user's own `git gc`, and no ref points to them. The temp index is a band-owned file in the OS
  temp directory, deleted at the end of step 8.
- Commit message via `lore.BuildCommit` (`pkg/lore/writer.go:11`): subject `fix(band): <series-slug> anomaly local patch
  (<episode-id>)`, or `fix(band): <series-slug> 이상 대응 로컬 패치 (<episode-id>)` when the commit-message language is `ko`;
  body with tier, z, and BS ID; the `pkg/lore` sign-off (`writer.go:8`); the `-F` file is the `BuildCommit` output plus one
  newline. Required trailers outside Constraint, Rejected, Confidence, Directive, Tested stop step 5 (`pkg/lore/query.go:87`
  `hasField` recognizes only these five). Values: Constraint `generated by auto react band; local patch needs human review`,
  Rejected `write-capable agent; read-only proposal only`, Confidence `low`, Directive `review the patch file before running
  anything`, Tested `none; nothing was run`; `Related: <BS-ID>` is always added.

## BS Record

With a `local_patch` claim, `brainstorm.Write` (`pkg/brainstorm/id.go:101`) receives the pointer through a `[NEW]`
`Request.LocalPatch` field (`render.go:31-40`, plan task T8), and `Render` puts these lines at the end of the BS `## 추천 방향`
section, so the BS sections and the structural validator (`validate.go:26-27`) stay unchanged and the BS stays write-once:

```text
Local patch (3σ, local only, if produced): branch autopus/band/<key>, worktree <lp>/<key>/worktree/, patch file <lp>/<key>.patch, outside this repository.
The outcome is recorded in .autopus/metrics/localpatch-events.jsonl under claim <claim-id>; nothing was pushed.
Reviewer warning: this local branch holds an agent-proposed patch that nothing has run. Read the whole patch file before you open the worktree in an IDE, run any command or agent in it, or push the branch, because repository hooks and tool configuration files run on checkout, commit, and build, and the edit guard does not cover that worktree.
```

`render.go` also holds three fixed diagnosis-only sentences that a local patch would contradict: the `- When:` line
(`render.go:180`), the Outcome Lock non-goal (`:196`), and the `## 추천 방향` sentence (`:208-209`). With a `local_patch` claim
they read: `- When: detected {date}; tier 3 with health_band.allow_local_patch may leave one local patch outside this
repository (see 추천 방향); band pushes nothing.`, `- Explicit non-goals: changes unrelated to {series_code}; band itself
pushes nothing and changes no GitHub state.`, and `Tier 3 with health_band.allow_local_patch adds at most one local branch,
worktree, and patch file and opens no pull request.` A BS without a `local_patch` claim keeps 001's text.

`<lp>` is written as an absolute path. The BS lives in `.autopus/brainstorms/` and the records in `.autopus/metrics/`; both
are gitignored and always blocked from staging (`internal/cli/check_rules_hygiene.go:25-30`). The branch is a local ref;
`git push --all` would publish it, which the docs state.

## Git Execution Policy

1. Environment: every git subprocess starts from `orchestra.EnvironWithout(os.Environ(), []string{"GIT_*"})`
   (`pkg/orchestra/provider_env.go:34`) and gets `GIT_TERMINAL_PROMPT=0`, `GIT_EDITOR=:`, `GIT_PAGER=cat`,
   `GIT_ATTR_NOSYSTEM=1` (no system attributes file), and `GIT_LFS_SKIP_SMUDGE=1` (git-lfs downloads nothing); only the
   three temp-index commands of Local Patch Flow step 8 also get `GIT_INDEX_FILE=<band temp file>`. No fetch, push, or other
   network command runs, so no transport or credential configuration is used.
2. Flags on every git command: `-c core.hooksPath=<hooks-off>` (an empty band-owned directory), `-c core.attributesFile=<empty>`
   (an empty band-owned file), `-c core.fsmonitor=false`, `-c core.untrackedCache=false`, `-c commit.gpgSign=false`,
   `-c gc.auto=0` (no automatic gc in the shared object database, per the worktree-safety rule); `format-patch` and every diff
   also pass `--no-textconv --no-ext-diff`.
3. Unsafe configuration, checked before any worktree exists, against `git config --list --show-scope --show-origin` and the
   repository files:
   - `$GIT_COMMON_DIR/info/attributes` holding any line that is not blank or a comment gives `git_config_unsafe:info_attributes`.
   - Any configured `filter.<driver>.clean`, `.smudge`, or `.process` gives `git_config_unsafe:filter.<driver>.<kind>` unless the
     driver is `lfs` and the raw value matches, byte for byte, the expression
     `^(git-lfs|/[A-Za-z0-9._/-]+/git-lfs) (filter-process|clean -- %f|smudge -- %f)$` (single spaces, no newline, tab, quote, `$`,
     backtick, or other shell metacharacter, because git runs filter commands through the shell), and the executable resolves
     (PATH lookup for the bare word) to an absolute path outside the repository and every worktree. An interpreter such as
     `/usr/bin/python3 tools/clean.py` is never allowed.
   - Any configured `diff.<driver>.textconv` or `.command`, or `merge.<driver>.driver`, gives `git_config_unsafe:<key>`.
   - Any `lfs.extension.<name>.*` or `lfs.customtransfer.<name>.*` setting gives `git_config_unsafe:<key>`.
   - `core.alternateRefsCommand` set at all gives `git_config_unsafe:core.alternateRefsCommand`.
   With global and system attribute files disabled by items 1–2 and `info/attributes` empty, only tracked `.gitattributes` files
   select drivers, and only the git-lfs allowlist can be configured, so no repository-selected command runs.
4. Base SHA: `<default>` is the default branch that 001's `fetchCI` resolved (`internal/cli/react_band_ingest.go:83-111`; today
   a local variable that T8 exposes as `[NEW]` `bandCIFetch.DefaultBranch`, `:33-41`), else, under `--no-fetch` or a failed
   lookup, `git symbolic-ref --short refs/remotes/origin/HEAD` without its `origin/` prefix; it must pass
   `git check-ref-format --branch`. The base is `git rev-parse --verify refs/remotes/origin/<default>^{commit}`, otherwise
   `refs/heads/<default>`; none of these gives `base_unavailable`. No fetch runs, so the base is the last fetched state the
   user already has.
5. Worktree: `git worktree add --detach <worktree> <base-sha>` at `<lp>/<key>/worktree/`, for a tier-3 claim and for a diagnosis
   without a `local_patch` claim alike (each with its own `<key>`); the latter is cleaned up right after its diagnosis.
6. Runner: the executor runs only the commands this policy names, through its own allowlist and a process-group runner
   modeled on `execBandRunner.Run` (`internal/cli/react_band_gh.go:73-84`). 001's `checkBandCommand` (`:90-113`), which
   refuses every git mutation, stays unchanged, so a flag-off run keeps 001 S14's zero-mutation recorder.
7. Nothing else: the user's worktrees, index, HEAD, stash, and every ref except `refs/heads/autopus/band/<key>` stay unchanged.

## Local Patch Provider Contract

While `health_band.allow_local_patch` is true, this contract replaces item 1 (selection) and item 5 (working directory) of
SPEC-SIGMABAND-001's Provider Read-Only Contract for every diagnosis and every patch request, adds `--restricted` to its item 3
and `GIT_*` to its item 6, and keeps items 2 and 4 (REQ-03, REQ-15). With the flag off, 001's contract applies unchanged and
band never reads `local_patch_provider`.

1. Selection, first match; once a rule matches, no later rule and no other provider is tried (001's no-retry rule):
   1. `health_band.local_patch_provider`, trimmed, when non-empty. Band runs it as a CLI subprocess whatever
      `orchestra.providers.<name>.backend` says. Only `claude` can be confined (`--restricted` is a claude argv flag), so any
      other name gives `unavailable(provider_unconfined)`.
   2. Otherwise SPEC-SIGMABAND-001's selection (`selectBandProvider`, `internal/cli/react_band_diagnose.go:165-186`), used only
      when it names `claude` and `orchestra.providers.claude` exists without a `backend`.
   3. Otherwise `unavailable(provider_unconfined)`: an OMP-backed claude, codex, gemini, an unconfigured name, or no name.
2. Subprocess form: rule 1 takes the `orchestra.providers.claude` entry when it exists without a `backend`
   (`providerConfigFromEntry`, `internal/cli/orchestra_helpers.go:222`), else `config.DefaultClaudeProviderEntry()`
   (`pkg/config/claude_provider.go:42`: binary `claude`, `--print`, the default model policy). An OMP entry's `binary` (default
   `omp`), `model` selector, and `tools` have no subprocess meaning and are never read, so this is no fallback of the OMP
   provider: band never runs the OMP entry. Rule 2 takes `providerConfigFromEntry` of the entry, as 001 does. The result has
   no `Backend`, so `orchestra.RunSingleProvider` runs the projected argv as a subprocess
   (`pkg/orchestra/provider_backend_route.go:26-30`), and band's execution config registers no OMP route.
3. Projection: `applyReadOnlyProviderPolicy` with `[NEW]` `readOnlyPolicyOptions.Confined` (plan task T6) adds `--restricted`
   before the last item `--tools=Read,Grep,Glob` and refuses a provider that has a `Backend` or a name other than `claude`,
   because the shared projection passes an OMP-backed provider through as-is (`orchestra_readonly_policy.go:59-63`); a refusal
   gives `unavailable(provider_unconfined)`. Then `bandReadOnlyControls` and a `--restricted` check run fail-closed, and a
   projection without `--restricted` gives `unavailable(provider_unconfined)`. An argv item outside the claude allowlist
   (`:148-185`), `--bare` included, still gives `unavailable(provider_policy_rejected)`.
4. The patch request resolves its provider by this contract again; anything but a confined subprocess claude ends the claim
   `failed:patch_provider_unconfined` (Local Patch Flow step 6). A missing binary, a timeout, a non-zero exit, and empty output
   keep 001's REQ-12 reasons.
5. Orchestra stays unchanged: `resolveProviders` (`internal/cli/orchestra_config.go:92`) and every orchestra command keep
   `orchestra.providers.<name>.backend`; only band reads `local_patch_provider`, and band never writes `autopus.yaml`.
6. Expected deployment: the subscription-authenticated `claude` CLI (Configuration). Band never projects `--bare`, whose
   authentication reads only `ANTHROPIC_API_KEY` or an `apiKeyHelper` and never the subscription login, while `--safe-mode`
   keeps authentication working (claude 2.1.289 help). That a `--restricted` session signs in through the subscription login
   is probe A1 (Completion Debt CD-2).

## Patch Prompt Contract

The patch request is a second provider prompt rendered by `promptlayer.Render` (`pkg/promptlayer/layer.go:89`); its manifest
goes into the `result` record. Its snapshot layer is 001's `healthband.EvaluationLayer` (`pkg/healthband/prompt.go:113`) of
the event that holds the diagnose claim. Its ephemeral layers take the sanitized texts the diagnose claim holds in memory
(the diagnosis output and the `RunLog` evidence), never a re-read of the BS file. 001's `evidenceLayer` (`prompt.go:164`) is
not reused, because it fixes the `untrusted-evidence` info string.

| Layer ID | Kind | Content | Cache eligible |
|----------|------|---------|----------------|
| `band.patch_instructions.v1` | stable | role, "answer with exactly one diff fence", no commands, the nonce rule | true |
| `band.patch_rules.v1` | stable | Patch Policy summary and size caps | true |
| `band.evaluation.<event-hash>` | snapshot | 001's frozen evaluation record (`EventHash`, `prompt.go:126`; no BS ID) | false |
| `band.diagnosis.<claim-id>` | ephemeral | the sanitized diagnosis text that the diagnose claim passed to `brainstorm.Request.Diagnosis` (no title line, no BS ID) | false |
| `band.evidence.run.<run_id>.a<attempt>` | ephemeral | SPEC-SIGMABAND-001's sanitized log excerpt | false |
| `band.base.<base-sha>` | ephemeral | the base SHA and the tracked paths named in the evidence (no file content) | false |

Every ephemeral layer is wrapped in a fence whose info string is `untrusted-<nonce>`, where the nonce is 128 random bits in
hex, drawn again whenever it occurs inside any layer, and whose backtick run is longer than any run inside the layer (the
length rule of `healthband.Fence`, `pkg/healthband/sanitize.go:246`): a closing fence needs only backticks, so the nonce alone
cannot keep the text from closing the block. The instructions name the nonce and state that text inside such fences is data.
Both providers read only files inside a band worktree at the base SHA (`--restricted`), which holds tracked content only;
untracked files of the user's checkout, such as `.env`, are not in it.

## Patch Policy

1. The reply capture is bounded while the provider runs at `healthband.ProviderCaptureBytes` + 1 (1 MiB,
   `pkg/healthband/sanitize.go:16`), as 001's diagnosis capture is (`react_band_diagnose.go:253-276`); a capture that dropped
   bytes gives `patch_too_large`. The reply holds exactly one ```` ```diff ```` fence (else `no_patch`), and the diff is at most
   64 KiB (else `patch_too_large`).
2. Paths are decoded before any check: a path in a `diff --git`, `---`, `+++`, `rename`, or `copy` header that starts with `"` is a
   C-quoted string whose escapes (`\a \b \f \n \r \t \v \\ \"` and three-digit octal) are decoded to bytes; every other path is
   taken as is. Every check below runs on the decoded bytes, and the decoded path set must equal the NUL-separated path set that
   `git apply --numstat --summary -z --check` reports (it writes no object), else `patch_invalid`.
3. Any rename, copy, deletion, mode change, symlink (120000), gitlink (160000), binary patch, new file whose mode is not 100644,
   absolute path, `..`, decoded path with control characters, or decoded path that is not valid UTF-8 gives `patch_invalid`.
4. Decoded paths are compared after Unicode NFC normalization and case folding; a path that folds to a different tracked path
   gives `path_denied:case_collision`.
5. Allowlist: extensions `.go .ts .tsx .js .jsx .mjs .cjs .py .rs .java .kt .rb .php .cs .swift .c .h .cc .cpp .hpp`.
6. Deny list (any path segment or file name, after folding): `.github/`, `.gitlab/`, `.circleci/`, `.buildkite/`, `.husky/`,
   `.githooks/`, `.lefthook/`, `.autopus/`, `.omp/`, `.agents/`, `.cursor/`, `.windsurf/`, `.vscode/`, `.idea/`, `.devcontainer/`,
   `scripts/`, `buildSrc/`, `vendor/`, `node_modules/`, every `workflow.GeneratedSurfacePrefixes` and
   `workflow.GeneratedSurfaceExactPaths` entry (`pkg/workflow/drift_gate.go:17,22`), `Makefile`, `GNUmakefile`, `Dockerfile*`,
   `docker-compose*`, `package.json`, `go.mod`, `go.sum`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.gitattributes`, `.gitmodules`,
   `.pre-commit-config.yaml`, `lefthook.yml`, `lefthook.yaml`; dotenv and credential paths `.env*`, `*.env`, `.envrc`, `.netrc`,
   `.npmrc`, `.pypirc`, `.git-credentials`, `.aws/`, `.ssh/`, `.gnupg/`, `.kube/`, `kubeconfig*`, `secrets.*`, `*secret*`,
   `*credentials*`, `*.pem`, `*.key`, `id_rsa*`, `id_ed25519*`, `*.p12`, `*.pfx`, `*.keystore`, `*.jks`; and files that tools
   load or run on their own: `build.rs`, `conftest.py`, `setup.py`, `noxfile.py`, `sitecustomize.py`, `usercustomize.py`,
   `*.config.js`, `*.config.cjs`, `*.config.mjs`, `*.config.ts`, `*.config.cts`, `*.config.mts`, `.eslintrc.*`, `.prettierrc.*`,
   `.pnpmfile.cjs`, `Package.swift`, `gulpfile.*`, `Gruntfile.*`, `magefile.go`, `Dangerfile.*`.
7. Added lines: `healthband.Sanitize` (`pkg/healthband/sanitize.go:88`) over the raw added lines, which applies 001's band
   secret forms (`bandSecretPatterns`, `sanitize_secrets.go:16`) before `promptlayer.SanitizeContent`, reports neither
   `secret_risk` nor `injection_risk`, and no added line holds a run of 40 or more base64 or hex characters
   (`patch_content_denied`).
8. Size: at most 10 files and 400 changed lines (added + removed) (`patch_too_large`).
9. Edit guard parity: band applies the diff with git from the CLI process, which no pre-tool hook sees, and the band worktree
   holds no `.autopus/*-manifest.json` or `.autopus/runtime/fix-locks/` record (both gitignored, `.gitignore:11,30`;
   `docs/edit-guard.md:92`). The policy therefore calls `editguard.Decide` (`pkg/editguard/decide.go:116`) with a `Call`
   (`:24-36`) whose `Cwd` is the top level of the user's checkout and whose `Targets` are the decoded paths, which classifies
   them where the manifests and fix locks live; a deny gives `path_denied:<class>` (`guard_state`, `fix_lock`, or
   `generated_surface`). A guard fault allows by the guard's own contract, so items 5–6 stay the primary path rules.

## Data Contracts

- `.autopus/metrics/localpatch-events.jsonl` (`autopus.band_localpatch.v1`, written under SPEC-SIGMABAND-001's store lock
  through its helpers `appendStoreFile`, `writeStoreFileAtomic`, and `readStoreLines` with `openStoreFile`'s no-follow open
  (`pkg/healthband/store_compact.go:129-258`), and compacted with the same rules, never dropping a record of a non-terminal
  claim) holds five record kinds:
  - `decision`: `seq`, `series`, `episode_id`, `evaluation_seq`, `decision` (`claim` or `skipped`), `reason`.
  - `claim`: `seq`, `claim_id`, `owner`, `lease_until`, `series`, `episode_id`, `depends_on` (the diagnose claim id), `key`,
    `worktree_path`, `patch_path`, `branch`; written in phase A.
  - `prep`: `seq`, `series`, `episode_id`, `diagnose_claim_id`, `lease_until` (both from 001's `DueClaim`,
    `pkg/healthband/catchup.go:30-42`), `claim_id` (when a `local_patch` claim exists), `key`, `base_sha` (once resolved),
    `code` (`ok` or a step-1 code); exactly one per flag-on diagnosis, written after step 1 and before any artifact exists.
  - `stage`: `seq`, `claim_id` (the `local_patch` claim id, else the diagnose claim id), `phase` (`worktree_intent`,
    `worktree_done`, `worktree_failed`, `message`, `apply_intent`, `apply_done`, `commit_done`, `branch_intent`,
    `branch_done`, `patch_intent`, `patch_done`), and per phase `path` (`worktree_intent`, `patch_intent`), `message_sha256`
    (`message`), `diff_sha256` and `tree` (`apply_intent`), `tree` (`apply_done`), `commit_oid` (`commit_done`,
    `branch_intent`), or `patch_sha256` (`patch_done`). Every `*_intent` is written before the command that creates the
    artifact, so every artifact that may exist is named by a durable record first.
  - `result`: `seq`, `claim_id`, `status` (`done` or `failed:<code>`), `bs_id`, `base_sha`, `commit_sha`, `branch`,
    `worktree_path`, `patch_path`, `prompt_manifest`, `recovered`, `kept[]` (artifacts kept by a cleanup rule, with reason); a
    diagnosis without a `local_patch` claim writes one too, keyed by its diagnose claim id.
- `.autopus/metrics/localpatch-state.json` (`autopus.band_localpatch_state.v1`): claims per series, written after the events and
  replayed like SPEC-SIGMABAND-001's checkpoint. SPEC-SIGMABAND-001 binaries never read these two files. Their codes may hold
  `:` and `.` (`failed:git_config_unsafe:filter.lfs.process`), which 001's `validResultStatus` would refuse
  (`wal_validate.go:31-34`); no such code reaches 001's events, whose diagnosis status only gains
  `unavailable(provider_unconfined)` and `unavailable(worktree_unavailable)`.
- Cleanup Rules (one set for the live run and for recovery; only artifacts named by the claim's intent records are considered;
  every git call has a 30 s timeout; the rules run in the order 3, 1, 2, so the worktree check still finds the diff file):
  3. Worktree: only the admin entry whose path equals the `worktree_intent` path is considered. It is removed with
     `git worktree remove --force <path>` when its HEAD is the base SHA or the claim commit, it has no unstaged or untracked
     change (`git status --porcelain --untracked-files=all`), and its index tree is the HEAD tree or the `apply_intent`
     expected tree. An entry that still holds the `locked` file of an interrupted `git worktree add` is kept with reason
     `worktree_incomplete`, because band cannot prove that a partial checkout holds only its own files; the docs name
     `git worktree remove --force --force <path>` for it. Otherwise it is kept with reason `worktree_modified`, or
     `head_unrecognized` when HEAD is neither commit.
  1. Branch: `git update-ref -d refs/heads/autopus/band/<key> <claim-commit>`, which deletes only while the branch points at the
     claim commit; a branch at another OID is kept with reason `branch_moved`.
  2. Patch and diff files: `<lp>/<key>.patch` or its `.tmp-<c8>` is deleted only when its bytes equal a fresh
     `git format-patch` of the claim commit with the step-11 flags, so a file a reviewer edited is kept with reason
     `patch_modified`; `<lp>/<key>.diff` is deleted only when its SHA-256 equals the `apply_intent` hash.
- Recovery step: runs in every non-dry-run band run in which `.autopus/metrics/localpatch-events.jsonl` exists, whatever the
  flag, so a claim interrupted while the flag was on is still handled after the flag is turned off. Its place in `execute`
  (`internal/cli/react_band.go:139-179`) is after `fetchCI` and before the `store.Lock` of phase A. It never runs under
  `--dry-run`, which loads the log through `planDry` without a lock or a write (001 REQ-14), and a run without that file
  creates no `.recovery.lock`. It takes `.autopus/metrics/.recovery.lock` (`filelock.Acquire`, `pkg/filelock/lock.go:37`,
  wait at most 5 s; otherwise the run reports reason `recovery_locked` in its JSON envelope and text output and recovers
  nothing). It reads this SPEC's records under the store lock (local file IO only), then, without the store lock, handles
  every `local_patch` claim and every flag-on diagnosis (`prep` record) whose `lease_until` has passed without a `result`
  (never retried) by the Recovery State Table, with a budget of 120 s per claim, and appends each `result` under the store
  lock. Phase A only reads these results.
- Recovery State Table (last durable record of the claim → what recovery finds → action → claim state):

| Last durable record | Found | Action | State |
|---------------------|-------|--------|-------|
| `decision`, `claim`, or `prep` | no artifact | none | `failed:interrupted` |
| `worktree_intent` | an admin entry with the `locked` file of an interrupted add | keep it, `worktree_incomplete` in `kept[]` | `failed:interrupted` |
| `worktree_intent` | a finished worktree at the path, or none | Cleanup Rule 3 | `failed:interrupted` |
| `worktree_failed` | an admin entry at the path, or none | Cleanup Rule 3 | `failed:worktree_failed` |
| `worktree_done` or `message` (diagnosis or patch request in flight) | clean worktree at the base SHA | Cleanup Rule 3 | `failed:interrupted` |
| `apply_intent` | worktree at the base SHA whose index tree is the base tree or the expected tree | Cleanup Rules 3, 2 | `failed:interrupted` |
| `apply_done` or `commit_done` | HEAD at the base SHA or at the claim commit | Cleanup Rules 3, 2 | `failed:interrupted` |
| `branch_intent` or `branch_done` | branch absent or at the claim commit | Cleanup Rules 3, 1, 2 | `failed:interrupted` |
| `patch_intent` | `<lp>/<key>.patch` equal to a fresh format-patch of the claim commit, branch and worktree HEAD at the claim commit, clean worktree | delete the diff file, keep the rest | `done`, `recovered: true` |
| `patch_intent` | anything short of that complete set | Cleanup Rules 3, 1, 2 | `failed:interrupted` |
| `patch_done` | anything | delete the diff file when its hash matches; keep the rest and list in `kept[]` each artifact that differs from its record | `done`, `recovered: true` |
| any row above | an artifact a user changed | keep it, list it in `kept[]` with its reason | the state of that row |

## Step Timeouts and Lease

| Step group | Steps | Deadline |
|------------|-------|----------|
| diagnosis setup | 1–2 up to `worktree_done` (checks, three records, `git worktree add`) | 30 s |
| diagnosis-only cleanup | Cleanup Rule 3 and the `result` record after the BS | 30 s |
| patch request | 3–6 (result check, branch check, Lore message and its record, provider call bounded by `ProviderTimeout`) | 600 s |
| apply and commit | 7–9 | 60 s |
| branch and patch file | 10–11 | 30 s |
| cleanup | the Cleanup Rules after a failure | 60 s |
| margin | the `result` record (store lock wait at most `ResultLockWait`, 60 s, `store.go:34`) | 60 s |

While the flag is true, a diagnose claim's budget is SPEC-SIGMABAND-001's `DiagnoseBudget` (930 s,
`pkg/healthband/episode.go:12-19`) plus diagnosis setup and diagnosis-only cleanup = 990 s; a `local_patch` claim's budget is
600 + 60 + 30 + 60 + 60 = 810 s. 001's `Plan` chains only `ClaimBudget(ClaimKindDiagnose)` (`catchup.go:100-101`), so
registering a kind in `claimBudgets` (`episode.go:21-27`) alone does not add it to the chain. Plan task T8 therefore adds
`[NEW]` `PlanOptions` fields: a budget override that gives a diagnose claim 990 s while the flag is true, and the `LocalPatch`
hook, whose row-4 claims `Plan` chains with 810 s right after their diagnose claim. A diagnose and its `local_patch` first in a
run get 990 s and 1,800 s. A checkout of the base that does not fit the 30 s setup deadline ends `worktree_failed`.

## PRD Deviations

| PRD text | This SPEC | Reason |
|----------|-----------|--------|
| FR-14 draft PR, push, `gh pr create --draft` | local branch, worktree, and `.patch` file only; no push, no PR | user decision 2026-10-06 (3σ cap = local patch only) |
| FR-14 "commit through the repository's normal hooks" | hooks off, in-process `lore.Validate` | hooks and commit-msg tooling would run repository code (F-002) |
| FR-14 branch name without hash | `autopus/band/<series-slug>-<h8>-<episode-id>-<c8>` (local) | slug prefixes collide (F-017); `<c8>` keeps claims of two checkouts apart (F-076) |
| FR-15 "create no branch, commit, push, or PR" on a guard failure | a failure removes the branch, worktree, and patch file it created unless a Cleanup Rule keeps a changed or unprovable artifact, which `kept[]` names; nothing remote ever exists | local cleanup replaces remote rollback |
| §5.2 `git apply --check` | `git apply --numstat --summary -z --check` plus the expected-tree check after `git apply --index` | the commit tree must equal the guarded patch (F-011, F-080) |

## 생성 파일 상세

Labels: `[NEW]` = added by this SPEC; `existing (001)` = merged by SPEC-SIGMABAND-001 and changed here by plan task T1, T8,
or T9; `existing` = older code.

| Path | Role |
|------|------|
| [NEW] `pkg/healthband/localpatch_decision.go`, `localpatch_wal.go`, `localpatch_recovery.go` | Local Patch Decision Table with the Opening tier rule, own write-ahead log and checkpoint over 001's store helpers, Recovery step and State Table |
| [NEW] `pkg/healthband/patchpolicy.go`, `patchprompt.go`, `commitmsg.go`, `gitpolicy.go` | Patch Policy (with the `editguard.Decide` check), Patch Prompt Contract, Lore message, Git Execution Policy |
| [NEW] `internal/cli/react_band_localpatch.go` | executor of the Local Patch Flow with its own git allowlist and runner |
| [NEW] `internal/cli/react_band_localpatch_provider.go` | Local Patch Provider Contract: selection order, band-only subprocess form, confined projection and its control checks (T7) |
| existing (001) `pkg/config/schema_health_band.go:8-14` | gains `AllowLocalPatch bool` with `yaml:"allow_local_patch,omitempty"` and `LocalPatchProvider string` with `yaml:"local_patch_provider,omitempty"` (T1) |
| existing (001) `pkg/healthband/episode.go:21-27`, `catchup.go:12-22,51-58,73-111`, `claims.go:84-117` | `local_patch` budget in `claimBudgets`; `[NEW]` `PlanOptions` budget override and `LocalPatch` hook with `[NEW]` `Plan.LocalPatch`; `[NEW]` `ExecuteOptions.AfterRecord` (T8) |
| existing (001) `internal/cli/react_band.go:139-255` | recovery step between `fetchCI` and `store.Lock`; decision and claim records after `Commit`; the hooks in `phaseA` and `phaseB` (T8) |
| existing (001) `internal/cli/react_band_diagnose.go:54-62,72-86,140-276` | `[NEW]` provider work directory apart from the BS directory, steps 1–2 inside `Run`, the flag-on provider from the Local Patch Provider Contract in place of `selectBandProvider` and `resolveProvider`, `GIT_*` added to the unset list, the worktree path redacted by the output sanitizer, the two new unavailable reasons (T8) |
| existing (001) `internal/cli/react_band_ingest.go:33-41,83-111` | `[NEW]` `bandCIFetch.DefaultBranch` (T8) |
| existing (001) `pkg/brainstorm/render.go:31-40,180,196,208-209` | `[NEW]` `Request.LocalPatch`: pointer lines and the three local-patch sentences (T8) |
| existing (001) `internal/cli/react_band_help.go:9-58`, `docs/health-band.md:223-228`; existing `CHANGELOG.md` | flag, `local_patch_provider` with the subscription claude CLI deployment, artifact locations, reviewer warning, subprocess claude requirement, upgrade note (T8 help text, T9 docs) |
| existing `internal/cli/orchestra_readonly_policy.go:11-15,53-88,209-217` | `[NEW]` `readOnlyPolicyOptions.Confined`, which adds `--restricted` to the claude projection and refuses a provider with a `Backend` or a name other than `claude` (T6) |

## Related SPECs

- SPEC-SIGMABAND-001 (implemented, merged): plan task T8 owns this SPEC's edits to the 001 files of 생성 파일 상세. While the
  flag is true, this SPEC amends: the diagnosis provider selection (REQ-15) and its cwd, projection, and environment (REQ-03); the diagnosis status values
  `unavailable(provider_unconfined)` and `unavailable(worktree_unavailable)`; the diagnose budget (990 s) and the lease chain,
  which also counts `local_patch` budgets, so claim `owner`/`lease_until` values differ from a flag-off run (REQ-12); the BS
  pointer lines and three sentences (BS Record); REQ-22, whose redaction does not apply to the raw patch reply, which never
  reaches a prompt, BS, or terminal (REQ-07); and the run order, which gains the recovery step between 001's network step and
  phase A whenever this SPEC's log exists. It never changes 001's action enum, decision table, evaluation fields other than
  claim leases, BS sections, or `checkBandCommand`. At sync it adds `allow_local_patch` and `local_patch_provider` to 001
  REQ-15's key list (REQ-01) and amends the text that still names the retired draft PR path: 001 `spec.md` Related SPECs, `docs/health-band.md:226-228`, and the `HealthBandConf`
  comment (`schema_health_band.go:3-7`).
- SPEC-REVIEWRO-001 (implemented): owns the shared claude projection, which already carries `--permission-mode plan`,
  `--safe-mode`, `--no-session-persistence`, `--disable-slash-commands`, `--strict-mcp-config`, and the last item
  `--tools=Read,Grep,Glob` (`orchestra_readonly_policy.go:209-217`); this SPEC adds only the confined option, which appends
  `--restricted` and refuses a provider with a `Backend` (the projection passes OMP providers as-is, `:59-63`), and the claude
  bool-flag list (`:150-153`) needs no change because the projection, not the user's argv, adds it.
- SPEC-PANERM-001 (implemented): orchestra has no pane backend and no `OrchestraConfig.SubprocessMode` (no match in `pkg`,
  `internal`, or `cmd`); `orchestra.RunSingleProvider` (`pkg/orchestra/provider_runner_single.go:28`) runs a provider as a
  subprocess of its projected argv or on its routed backend. `--restricted` is a claude argv flag, so only the subprocess
  claude can be confined. An OMP-backed claude, the form this repository's `autopus.yaml:77-90` configures for every provider,
  is `unavailable(provider_unconfined)` while the flag is true unless `health_band.local_patch_provider` names claude, which
  band then runs as a CLI subprocess while orchestra keeps `backend: omp` (Local Patch Provider Contract; OQ-1 closed 2026-10-08).
- SPEC-EDITGUARD-001 (implemented): the edit guard neither protects the local patch flow nor is relied on by it. The band
  providers hold only Read, Grep, and Glob, so the guard's `Edit|Write|MultiEdit` matcher never fires; `--safe-mode` disables
  hooks and `--restricted` ignores project and local settings, so a tracked `.claude/settings.json` in the band worktree
  registers nothing; band applies the diff with `git apply` from the CLI process, which is no tool call; and the fresh worktree
  has no manifests or fix-lock records (`docs/edit-guard.md:92`). Patch Policy item 9 therefore asks `editguard.Decide`
  against the user's checkout, and the BS reviewer warning says that the guard does not cover the band worktree.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-01 | T1 | S1, S10 | INV-01 |
| REQ-02 | T2, T8 | S2, S3 | INV-02 |
| REQ-03 | T6, T7, T8 | S7, S13 | INV-06 |
| REQ-04 | T7 | S3, S6 | INV-02 |
| REQ-05 | T4, T7 | S4, S6 | INV-03 |
| REQ-06 | T7 | S5 | INV-08 |
| REQ-07 | T5 | S8 | INV-06 |
| REQ-08 | T3 | S5 | INV-05, INV-10 |
| REQ-09 | T7 | S4 | INV-07 |
| REQ-10 | T7, T10 | S12 | INV-04 |
| REQ-11 | T2, T7 | S5, S9 | INV-09 |
| REQ-12 | T2, T8 | S2, S9 | INV-02 |
| REQ-13 | T8, T9 | S4, S11 | INV-01, INV-07 |
| REQ-14 | T7 | S4 | INV-07 |
| REQ-15 | T6, T7, T8 | S7, S13, S14 | INV-11 |

## Review Resolution

Rev 7 (2026-10-08) applies the operator decision on OQ-1 (the OMP backend cannot be confined, and every orchestra provider of
this repository is `backend: omp`):

| Item | Resolution | Where |
|------|------------|-------|
| OQ-1 (operator decision 2026-10-08) | closed: band-only subprocess provider. `[NEW]` `health_band.local_patch_provider` (default empty, strict decode, omitted while empty) names the claude that band always runs as a CLI subprocess with the confined projection, whatever its orchestra backend; the order is the key, then 001's selection only for a subprocess claude, then `unavailable(provider_unconfined)`; orchestra keeps its backend; the 001 REQ-15 amendment covers both new keys; the subscription claude CLI is the expected deployment | REQ-01, REQ-03, REQ-13, REQ-15, Configuration, Local Patch Provider Contract, S1, S7, S10–S14, plan.md T1, T6–T9, research.md Decision Record and Open Questions |

Rev 6 (2026-10-08) refreshes this draft against the merged SPEC-SIGMABAND-001 code (main `1943e596`), SPEC-PANERM-001, and
SPEC-EDITGUARD-001, and resolves the findings that rev 5's review left open or regressed:

| Item | Resolution | Where |
|------|------------|-------|
| refresh: 001 is code | every reference that rev 5 labeled as planned by SPEC-SIGMABAND-001 names the merged file, symbol, and line; T8 still owns the edits | 생성 파일 상세, plan.md T8, research.md Reference Discipline |
| refresh: lease chain | `Plan` chains only the diagnose budget (`catchup.go:100-101`); T8 adds a `PlanOptions` budget override and the `LocalPatch` hook; budgets are group deadlines and appends wait at most 5 s | Step Timeouts and Lease, REQ-12 |
| refresh: claims and episodes | claim ids are 32 hex, so the fixture id grew to 32 hex with the same `<c8>`; 001 episodes keep no opening tier, so row 5 uses the Opening tier rule | Decision Table, acceptance.md |
| refresh: phase A and B | records follow `Commit` inside the lock; `ExecuteClaims` has no post-record hook, so T8 adds `ExecuteOptions.AfterRecord`; the diagnoser's one `projectDir` (cwd and BS directory) splits | Decision Table, 생성 파일 상세 |
| refresh: provider | SPEC-REVIEWRO-001 landed, so the confined option adds only `--restricted`; `bandProviderUnsetEnv` plus `GIT_*`; 1 MiB capture bound; OMP-backed providers unconfined (SPEC-PANERM-001) | REQ-03, REQ-07, Related SPECs |
| refresh: git runner and base | 001's `checkBandCommand` refuses git mutations, so the executor has its own allowlist; `fetchCI` keeps the default branch local, so T8 exposes it | Git Execution Policy items 4 and 6, REQ-05 |
| refresh: BS text | three diagnosis-only sentences of `render.go` would contradict a local patch and get local-patch forms | BS Record, S2 |
| refresh: prompt and secrets | nonce fences keep `Fence`'s length rule; the diagnosis layer comes from memory; added lines run through `healthband.Sanitize` with 001's band secret forms | Patch Prompt Contract, Patch Policy items 1 and 7, S8 |
| refresh: edit guard | interaction stated; Patch Policy item 9 asks `editguard.Decide` against the user's checkout | Patch Policy item 9, Related SPECs, S5 |
| refresh: key alphabet | the slug rule maps 001's identifier and sample-key characters to `[a-z0-9-]` | `<key>` definition |
| F-061 (regressed) | resolved by SPEC-SIGMABAND-001's implementation: `pkg/filelock`, `episode.go`, and `catchup.go` exist and are cited as existing code | 생성 파일 상세, research.md |
| F-068 (c) | resolved with 001's `DueClaim.LeaseUntil`: every flag-on diagnosis `prep` carries the diagnose claim id and lease, and a diagnosis-only claim gets a `result` and recovery | Data Contracts, S9 |
| F-068 (d) | resolved: a patch file is deleted only when it equals a fresh format-patch of the claim commit; `patch_done` carries its hash | Cleanup Rule 2, S9 |
| F-068 (b') | resolved: group deadlines, 5 s record appends, and a lease check before each group, so a live claim never acts after its lease | Local Patch Flow, Step Timeouts and Lease |
| F-068 (e) | resolved: `record_unavailable` is REQ-04's first check; `recovery_locked` is a run reason, not a record | REQ-04, Data Contracts, S9 |
| F-080 | resolved: the `stage` phase list and fields are complete; the claim commit is the `commit_done` OID or parent, message hash, and expected tree; rules run 3, 1, 2; `apply_intent` records the expected tree | Local Patch Flow steps 8–9, Data Contracts, S9 |
| F-081 | resolved: no recovery under `--dry-run`; recovery runs whenever this SPEC's log exists, whatever the flag; no lock file without it | Data Contracts, S1, S9 |
| F-082 | resolved: an entry left by an interrupted `git worktree add` is kept as `worktree_incomplete`, never force-removed | Cleanup Rule 3, Recovery State Table, S9 |
| F-083 | resolved: rows for `worktree_failed` and `patch_done`; REQ-11 ends live failures with step codes and interrupted claims with table states | REQ-11, Recovery State Table |
| F-084 | resolved: `apply_intent` is written before the diff file | Local Patch Flow step 8 |
| F-076 | resolved: the PRD Deviations branch name carries `-<c8>` | PRD Deviations |
| F-064 | resolved: absolute statements narrowed to what the Cleanup Rules do; Self-Verify Summary re-judged | PRD Deviations, research.md, plan.md |

Earlier revisions (rev 2 applied the user decision of 2026-10-06, 3σ cap = local patch only; rev 3–5 resolved their reviews):

| Finding | Resolution | Where |
|---------|------------|-------|
| F-068 (rev 5) | partly resolved in rev 5 (own recovery step, lock, timeouts, store-locked appends); the rest closed in rev 6 above | Data Contracts, Local Patch Flow, S9 |
| F-076 (rev 5) | diagnoses without a claim got a claim-unique `<key>`, step-1 existence checks, and intent records | Git Execution Policy item 5, Local Patch Flow steps 1–2, S7 |
| F-079 | resolved: the complete row checks the files, not `patch_done`; a pre-existing branch yields only `artifact_exists` | Recovery State Table, S5, S9 |
| F-080 (rev 5) | apply and commit records added; closed in rev 6 above | Local Patch Flow steps 8–9 |
| F-072 (rev 5) | resolved: the lease edits live in 001's `episode.go`/`catchup.go` (T8); S9 checks a five-claim chain and S2 checks lease values | 생성 파일 상세, S2, S9 |
| F-073 (rev 4) | resolved: one `prep` record after step 1; `worktree_failed` is a `stage` record after the command; REQ-04 order covers both | REQ-04, Local Patch Flow steps 1–3, S3 |
| F-077 | resolved: REQ-14 separates files outside `.git/` from the allowed `.git/` changes | REQ-14, S4 |
| F-078 | resolved: the git-lfs allowlist is a byte-exact expression without shell metacharacters | Git Execution Policy item 3, S6 |
| F-017, F-036, F-037, F-041, F-042, F-044, F-053, F-059, F-062, F-068 (remote part), F-069 | resolved by rescope (user decision 2026-10-06): no push, PR, remote ref, or CI run exists | REQ-10, S12 |
| F-045 | resolved by rescope: no fetch runs; the base is the last fetched remote-tracking ref | Git Execution Policy item 4, S6 |
| F-039 | resolved: both providers confined to a worktree of tracked content; dotenv and credential paths denied; records local | REQ-03, Patch Policy item 6, S7 |
| F-043, F-071, F-075 | resolved (rev 3): attribute sources disabled or required empty, drivers refused outside the exact git-lfs token rule, LFS downloads and extensions off | REQ-05, Git Execution Policy items 1–3, S6 |
| F-070 | resolved (rev 3): worktree and patch file live under the user cache directory, outside the repository | REQ-14, Git Execution Policy item 5, S4 |
| F-074 | resolved (rev 3): default row 6 | Decision Table, S3 |
| F-058 (regressed in rev 2) | resolved (rev 3): C-quoted paths decoded before every check and cross-checked with `git apply --numstat -z --check` | Patch Policy item 2, S5 |
| F-049, F-050, F-067 | resolved: the claim depends on the diagnose claim, whose result with the BS ID comes first; first-match order | Decision Table, REQ-04, S3 |
| F-056, F-060 | resolved: commit object message vs `-F` bytes; steps 1–7 write no object | Local Patch Flow, S5 |
| F-040, F-047, F-048, F-055 | carried: tool-loaded files denied, raw diff, fenced prompt layers, five-trailer rule | Patch Policy, Patch Prompt Contract, REQ-06 |
