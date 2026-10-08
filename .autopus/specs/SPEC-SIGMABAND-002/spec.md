# SPEC-SIGMABAND-002: σ-band 3σ local patch on an isolated worktree

**Status**: draft
**Created**: 2026-10-06
**Revised**: 2026-10-08 (rev 10: the CD-3 verify findings L2, M5, M7, and N1–N5 and the rev 9 spec-review findings F-001–F-011 applied, three probe assumptions recorded as verified, see Review Resolution; rev 9: the CD-3 security design review findings H1–H3, M1–M7, and L1–L4 applied; rev 8: Risk-First probes A1 and A3 executed and folded in, closing Completion Debt CD-1 and CD-2; rev 7: operator decision on OQ-1, the band-only subprocess provider `health_band.local_patch_provider`; rev 6: refreshed against the merged SPEC-SIGMABAND-001 code at main `1943e596`, SPEC-PANERM-001, and SPEC-EDITGUARD-001, and resolves the open rev 5 findings; rev 5: one recovery state table, a separate recovery step with its own lock, claim-unique diagnosis worktrees, chained-lease oracle; rev 2 rescoped the cap to local patch only)
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
쓴다. git은 checkout·apply·commit·format-patch 동안 repository가 지정한 어떤 명령도 실행하지 않고, promisor remote에서 객체를 받아 오지 않으며, git-lfs와 자동 maintenance를 실행하지 않고, replace ref를 따르지 않으며, tracked symlink를 링크로 풀지 않는다. 저장소 root 아래 `.git/` 밖 파일은 BS와 band 기록 파일 외에는 바뀌지 않고 `.git/` 안에는 REQ-14가
허용한 변경만 생기며, test는 자동으로 돌리지 않는다.

## Outcome Boundary

- Outcome Lock: `health_band.allow_local_patch: true`이면, SPEC-SIGMABAND-001이 tier 3으로 연 episode 중 confined 진단이 성공한 것은 최대 한 번
  (모든 guard를 통과하고 저장소당 보존 상한인 남은 key 5개에 이르지 않았으면 정확히 한 번, 상한에 이르면 `local_patch_skipped:cap_reached`) 로컬 산출물을 받는다. 산출물은 로컬 branch `autopus/band/<key>`, worktree `<lp>/<key>/worktree/`,
  patch 파일 `<lp>/<key>.patch`이다. 여기서 `<lp>` = `<UserCacheDir>/autopus/local-patches/<repo-hash>`, `<key>` =
  `<series-slug>-<h8>-<episode-id>-<c8>`이고 `<c8>`은 claim id의 앞 8 hex다. 3σ BS에는 pointer가 기록되고, 원격에는 아무것도 생기지 않는다.
  orchestra provider가 모두 OMP backend인 저장소도 `health_band.local_patch_provider: claude`로 같은 산출물을 받고, orchestra는
  설정된 backend를 유지한다.
- Mandatory requirements: REQ-01–REQ-15 (Priority Must).
- Explicit non-goals: push, fetch, PR 생성·갱신, 원격 ref, GitHub 쓰기 API, CI 실행, test·build 자동 실행, 에이전트 쓰기 권한, tier 2로 열린 뒤
  tier 3으로 오른 episode의 patch, OMP backend의 worktree confinement, orchestra 명령의 provider backend 변경, SPEC-SIGMABAND-001이 소유한 동작. 리뷰어가 worktree를 IDE로 열거나
  그 안에서 명령이나 에이전트를 실행하거나 branch를 push할 때의 실행도 범위 밖이며, BS의 reviewer warning이 이를 알린다.
- Completion evidence: acceptance S1–S15 통과, Completion Debt CD-3 해소(설계 리뷰 지적 H1–H3, M1–M7, L1–L4는 rev 9에, verify 지적 L2, M5, M7, N1–N5와 spec review 지적 F-001–F-011은 rev 10에 반영, security-auditor 재리뷰 통과로 닫힘; CD-1, CD-2는 2026-10-08 probe A3, A1로 해소), 모든 run에서 원격 쓰기 0건, 저장소 root
  아래 `.git/` 밖 파일 변경은 BS와 `.autopus/metrics/` 기록 파일뿐.

## Requirements

Priority 열은 Must만 쓴다. 각 문장은 `pkg/spec` parser 문법을 따르며 strict validate의 `ParseEARS`로 15개 모두 인식된다.

| ID | Priority | Source | EARS requirement |
|----|----------|--------|------------------|
| REQ-01 | Must | FR-17, F-052, decisions 2026-10-06 and 2026-10-08 | THE SYSTEM SHALL add `AllowLocalPatch bool` with the tag `yaml:"allow_local_patch,omitempty"` and `LocalPatchProvider string` with the tag `yaml:"local_patch_provider,omitempty"` to `HealthBandConf` in `pkg/config/schema_health_band.go`, amend SPEC-SIGMABAND-001 REQ-15's key list with both keys at this SPEC's sync while `allow_draft_pr` stays rejected, omit each key from generated and saved `autopus.yaml` files while it holds its default, and document that a binary without this SPEC rejects a file that sets either key. |
| REQ-02 | Must | FR-07, F-049, F-072 | WHEN SPEC-SIGMABAND-001's phase A plans an evaluated tier-3 position IF `health_band.allow_local_patch` is true, THEN THE SYSTEM SHALL decide it exactly as the Local Patch Decision Table defines, record a `local_patch` claim that depends on the diagnose claim of a row-5 position, append those records to `.autopus/metrics/localpatch-events.jsonl` under the store lock after 001's `Commit` and before its `Unlock`, and write nothing into SPEC-SIGMABAND-001's files beyond the amendments listed in Related SPECs. |
| REQ-03 | Must | F-039, F-057, PANERM, decision 2026-10-08, probe A1, CD-3 M4, N5 | WHERE `health_band.allow_local_patch` is true, THEN THE SYSTEM SHALL run every SPEC-SIGMABAND-001 diagnosis and every patch request with a band worktree at the base SHA as the only working directory, on the subprocess claude that the Local Patch Provider Contract selects, projected by `applyReadOnlyProviderPolicy` with the confined option that adds `--restricted`, `--output-format stream-json`, and `--verbose` to the shared projection, without the variables of `bandProviderUnsetEnv` and without any `GIT_*` variable, record every selection that does not end at that subprocess claude, an OMP-backed claude reached without `health_band.local_patch_provider` included, as `unavailable(provider_unconfined)`, and record each request's requested and actual model, marking `model_substituted` without failing a diagnosis that claude answered on another model and ending with `failed:patch_model_refused` a patch request that claude answered on another model and with `failed:patch_model_unverified` one whose stream has no `init` event or an `assistant` event on a model other than the `init` model. |
| REQ-04 | Must | F-049, F-050, F-068, F-073 | WHEN a `local_patch` claim executes, THEN THE SYSTEM SHALL end it with `failed:record_unavailable` when its diagnosis has no `prep` record, otherwise with the step-1 code of that `prep` record when the code is not ok, otherwise with `failed:<code>` of a `worktree_failed` stage record (`worktree_failed`, or `git_config_unsafe:<key>` from the check inside the new worktree), otherwise with `failed:no_bs` when the diagnose outcome has no BS ID, otherwise with `failed:diagnosis_unavailable` when `diagnosis_status` is not ok, and only otherwise start the patch stage, so that no failed preparation or diagnosis leads to a patch request, apply, or commit. |
| REQ-05 | Must | F-043, F-071, F-075, CD-3 H3, M1, L2, L4, F-001–F-004, F-009 | WHEN band runs git for a local patch, THEN THE SYSTEM SHALL run only the commands of the Git Execution Policy through its own allowlist, leave SPEC-SIGMABAND-001's `checkBandCommand` unchanged, and apply that policy to every git command (scrubbed environment with `GIT_ATTR_NOSYSTEM=1`, `GIT_LFS_SKIP_SMUDGE=1`, `GIT_NO_LAZY_FETCH=1`, and `GIT_NO_REPLACE_OBJECTS=1`, refusal of a git older than 2.44 and of a partial clone, hooks off through `core.hooksPath=/dev/null`, `core.fsmonitor=false`, `core.symlinks=false`, `core.attributesFile=/dev/null`, `core.useReplaceRefs=false`, `maintenance.auto=false`, the git-lfs driver blanked, the fixed band user, author, and committer identity, refusal of a non-empty `info/attributes`, of every configured filter, diff, or merge driver outside the git-lfs allowlist, and of every `lfs.extension` or `lfs.customtransfer` setting, the same refusals checked in the user's checkout, inside the new worktree before its checkout, and again before the first object write, and no network command) so that checkout, apply, commit, and format-patch run no command that the repository or its configuration selects, git-lfs included, and THE SYSTEM SHALL never run tests, builds, or the proposed change. |
| REQ-06 | Must | F-016, F-055, F-056 | WHEN band builds the commit message, THEN THE SYSTEM SHALL build and validate it in-process with `lore.BuildCommit` and `lore.Validate` before any `git apply`, refuse with `lore_unsupported_required:<trailer>` when a required trailer is not one of the five that `lore.Validate` recognizes, and confirm after the commit that the commit object's message equals the `-F` file bytes. |
| REQ-07 | Must | F-039, F-047, F-048, probe A1 | WHEN band requests a patch, THEN THE SYSTEM SHALL build the prompt as the Patch Prompt Contract defines, bound the provider stream at 8 MiB while the provider runs and the `result` text taken from that stream at 1 MiB, take the diff from that raw text without redaction or line removal, and keep the reply in memory until the Patch Policy has accepted it. |
| REQ-08 | Must | F-040, F-058, decision 2026-10-06, EDITGUARD, CD-3 H1, H2, M2, M3, L1, N1, N3, N4, F-009 | WHEN a proposed diff is evaluated, THEN THE SYSTEM SHALL decode every path as the Patch Policy defines and accept the diff only when the Patch Policy accepts every decoded path (including the dotenv and credential entries, the structural deny rules, the folded collision check, the refusal of a path with a `filter` attribute at the base, and the edit guard check of item 9, whose fault denies), every mode together with the base entry that `git ls-tree -t` reports for each touched path and directory prefix, and every added line, counted by its hunk header, which holds no code point of the Patch Policy item 7 set, and the change stays within 10 files, 400 changed lines, and 64 KiB, before any command writes to the object database. |
| REQ-09 | Must | decision 2026-10-06, F-070, F-080, CD-3 F-005, F-006, F-010 | WHEN the Patch Policy accepts a diff, THEN THE SYSTEM SHALL apply it with `git apply --index` in the claim's worktree under `<lp>/<key>/worktree/`, confirm that the worktree's index tree equals the expected tree recorded in `apply_intent`, commit it, confirm that the recorded commit has the base SHA as its only parent and the expected tree as its tree, create the local branch `autopus/band/<key>` at that commit OID with `git update-ref --no-deref`, write the canonical `git format-patch` output of `<base-sha>..<commit-oid>` to `<lp>/<key>.patch` after recording its SHA-256, and keep the worktree for human review. |
| REQ-10 | Must | decision 2026-10-06, CD-3 M1 | THE SYSTEM SHALL never push, fetch, fetch an object lazily from a promisor remote, create or update a pull request, call a GitHub write API, or create a remote ref in the local patch flow, so that every artifact stays local until a human pushes it. |
| REQ-11 | Must | F-060, F-064, F-068, F-076, F-080, F-083, CD-3 M6, M7, L3, F-001, F-005, F-007, F-010, F-011 | IF any guard or step of the local patch flow or of a confined diagnosis fails, or a later run finds a claim interrupted, THEN THE SYSTEM SHALL handle exactly the artifacts that the claim's intent records name, at the paths derived from `<lp>` and the claim's key and never at a path that a record stores, under the one Cleanup Rules set, end a live failure with the code of its Local Patch Flow step, an interrupted claim with the state of the Recovery State Table, and a claim whose records disagree with that derivation with `failed:record_invalid`, leave without a `result` in recovery a claim whose key lock another process holds, stop without a further record, in the live run and in recovery, wherever the re-read under the store lock that follows the key lock finds the claim's `result`, write exactly one `result` record for every other claim, keep the SPEC-SIGMABAND-001 BS, never touch an artifact that no intent record of the claim names or that a user changed or added, ignored files, index flags, and symbolic refs included, and exit 0. |
| REQ-12 | Must | F-032, F-068, F-072, CD-3 M5 | THE SYSTEM SHALL give a diagnose claim the 990 s budget and a `local_patch` claim the 810 s budget of the Step Timeouts table while `allow_local_patch` is true, execute each `local_patch` claim right after phase C has recorded its diagnose claim, count both budgets in the lease chain of SPEC-SIGMABAND-001's `Plan` through the `PlanOptions` and `ExecuteOptions` hooks that plan task T8 adds in `pkg/healthband/catchup.go` and `claims.go`, start no step group whose deadline the claim's remaining lease does not cover, and stop a command that outlives its step group with SIGTERM 5 s before the group's deadline and SIGKILL at it. |
| REQ-13 | Must | FR-23, decisions 2026-10-06 and 2026-10-08 | THE SYSTEM SHALL document the flag, the `local_patch_provider` key with the subscription claude CLI deployment, the artifact locations, the reviewer warning, the subprocess claude requirement, and the upgrade-before-enable note in the `auto react band` help text, `docs/health-band.md`, and `CHANGELOG.md`, qualify their tier-3 diagnosis-only statements with the flag, and put the Local Patch pointer lines with the reviewer warning in every 3σ BS of a `local_patch` claim. |
| REQ-14 | Must | F-070, F-077, CD-3 M5, L2, F-008, F-009 | THE SYSTEM SHALL keep every worktree and patch file of this SPEC under `<UserCacheDir>/autopus/local-patches/<repo-hash>/`, outside the repository, refuse with `cache_unavailable` when that directory cannot be created with mode 0700, exists as anything but a directory of the current user without group or other permission bits, has a symlink component below the user cache directory, or has a real path that equals or lies inside the repository's top level, its common directory, or any registered worktree, create every patch, diff, and temp file that band writes there with mode 0600 through `O_CREAT`, `O_EXCL`, and `O_NOFOLLOW` below an `os.Root` opened at the real user cache directory, skip a new claim with `local_patch_skipped:cap_reached` and refuse every other flag-on worktree with `cap_reached` while 5 kept keys remain there, change no file under the repository root outside `.git/` except the BS file and the files under `.autopus/metrics/` that SPEC-SIGMABAND-001 and this SPEC own, and change inside `.git/` only `refs/heads/autopus/band/<key>` with its reflog, the `worktrees/<name>/` entry of the claim's worktree, and new objects, with nothing under `lfs/`. |
| REQ-15 | Must | operator decision 2026-10-08 (OQ-1), probe A1 | WHERE `health_band.allow_local_patch` is true, THEN THE SYSTEM SHALL select the provider of every diagnosis and every patch request by the Local Patch Provider Contract, run a `claude` named by `health_band.local_patch_provider` as a CLI subprocess whatever its `orchestra.providers.claude.backend` says, on the model that the contract's model rule names, use SPEC-SIGMABAND-001's selection only while that key is empty and only when it resolves to a subprocess claude, give `unavailable(provider_unconfined)` in every other case, a key naming another provider included, and leave the provider backend of every orchestra command unchanged. |

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
are checked top down and the first match wins; row 7 is the default. Decisions go to
`.autopus/metrics/localpatch-events.jsonl` (kind `decision`).

| Order | Condition | Decision | Reason |
|-------|-----------|----------|--------|
| 1 | the run has `--no-agent` | skipped | `local_patch_skipped:no_agent` |
| 2 | the event carries `superseded_in_batch` (001 batch rule, `decideSeries`, `catchup.go:117-144`) | skipped | `local_patch_skipped:superseded_in_batch` |
| 3 | `localpatch-state.json` holds a `local_patch` claim for the episode | skipped | `local_patch_skipped:episode_already_patched` |
| 4 | the event's action is `diagnose` and the retention count has reached 5 | skipped | `local_patch_skipped:cap_reached` |
| 5 | the event's action is `diagnose`, so it opens the episode at tier 3 | claim, `depends_on` = that event's diagnose claim | - |
| 6 | the episode's opening tier is 2 | skipped | `local_patch_skipped:bs_not_tier3` |
| 7 | any other tier-3 position, such as a later position of an episode whose tier-3 opening got no claim (rows 1, 2, and 4, or the flag was off) | skipped | `local_patch_skipped:no_opening_claim` |

Opening tier: 001's `Episode` (`pkg/healthband/types.go:175-181`) keeps `max_tier` but not the opening tier, so the table takes
the `tier` of the opening event when this plan holds it, else 2 when 001's checkpoint gives the episode `max_tier` 2, else 2
when this SPEC's records hold a `bs_not_tier3` decision for the episode, else 3.

Retention count (CD-3 M5): the number of distinct keys that have a `<lp>/<key>/` directory or a `<lp>/<key>.patch` file, read
by one `os.ReadDir` of `<lp>` (local IO under the store lock, no git), plus the row-5 claims already decided in this plan.
Kept worktrees of done claims and artifacts that a Cleanup Rule kept both count, and a human frees a slot by removing them.
Local Patch Flow step 1 counts again, by the `os.ReadDir` alone, before every flag-on worktree, a tier-2 diagnosis included,
because another checkout of the repository may fill `<lp>` after phase A (CD-3 M5).
While the flag is true, band resolves `<lp>` (the `git rev-parse` of the `<repo-hash>` definition) before `store.Lock`.

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
records, and `result` record. A claim holds its key lock from step 1 until its last `result` record, and every `result` is
write-once (Data Contracts).

| Order | Step | Code on failure |
|-------|------|-----------------|
| 1 | Inside the diagnose claim (`bandDiagnoser.Run`, `internal/cli/react_band_diagnose.go:108`): git 2.44 or later (Git Execution Policy item 1); `<lp>` available with the REQ-14 modes and the real-path rule (Data Contracts, Derived paths); no artifact at `<lp>/<key>/`, `<lp>/<key>.patch`, `<lp>/<key>.diff`, `<lp>/<key>.lock`, or `refs/heads/autopus/band/<key>`, a symbolic ref included; a retention count below 5 for this and every other flag-on worktree, a tier-2 diagnosis included (CD-3 M5); then the key lock `<lp>/<key>.lock` (`filelock.Acquire`, `pkg/filelock/lock.go:37`, with a zero wait, which tries once, creates a 0600 file, and refuses a symlink), held until the claim's last `result` record (the `local_patch` claim's when one exists) and then unlinked, so it exists before `prep` (CD-3 M7); unsafe configuration check in the user's checkout (item 3, partial clones included), base SHA (item 4), checkout preflight (below the table); then, under the store lock, a re-read of this SPEC's records and, when it finds no `result` for the claim id of `<key>`, one `prep` record with the diagnose claim id, its `lease_until`, the base SHA, and the code (ok or the failure). A key lock that another process holds or a `result` found by the re-read stops the claim with no record of this SPEC. The first four codes end the step before the key lock and write their `prep` without it, which is safe because no artifact can exist yet and a `result` is write-once. Every stop and failure makes the diagnosis report `unavailable(worktree_unavailable)`, and 001 writes the evidence-only BS | `git_version_unsupported:<version>`, `cache_unavailable`, `artifact_exists`, `cap_reached`, `git_config_unsafe:<key>`, `base_unavailable`, `worktree_too_large`, `disk_insufficient` |
| 2 | Inside the diagnose claim: `stage` `worktree_intent` (path); `git worktree add --no-checkout --detach <worktree> <base-sha>`; item 3 again inside the new worktree (`git -C <worktree> config --list --show-scope --show-origin`, which evaluates every `include` and `includeIf` condition, `gitdir:` included, for the worktree's own git directory) before any file is checked out (CD-3 F-001); the checkout `git -C <worktree> reset --hard --no-recurse-submodules --quiet`, the command that a default `git worktree add` runs as its child; each command stopped as Step Timeouts and Lease states; then `stage` `worktree_done` with `status_sha256`, the SHA-256 of the `git status --porcelain -z --untracked-files=all --ignored` output followed by the `git diff --no-ext-diff --no-textconv --binary` output (CD-3 M5), or `stage` `worktree_failed` with its code; then the confined diagnosis with cwd `<worktree>` (REQ-03) and the BS, with the BS Record lines when a `local_patch` claim exists; a diagnosis without a `local_patch` claim then applies the Cleanup Rules to its worktree and writes its `result` | `git_config_unsafe:<key>`, `worktree_failed`, 001 REQ-12 reasons |
| 3 | `local_patch` claim: result check in the order of REQ-04 | `record_unavailable`, the `prep` code, `worktree_failed`, `no_bs`, `diagnosis_unavailable` |
| 4 | `refs/heads/autopus/band/<key>` still absent, as a ref and as a symbolic ref (`git symbolic-ref -q --no-recurse`, CD-3 F-005); it can only appear here if something created it after step 1 | `branch_exists` |
| 5 | Lore message drafted and validated (REQ-06), so a configuration that cannot take the band commit stops before the provider call | `lore_unsupported_required:<trailer>`, `lore_rejected` |
| 6 | Patch request (REQ-07) on the provider of the Local Patch Provider Contract; the reply stays in memory; a `model_refusal_fallback` event in its stream, a stream without a `system` `init` event, or an `assistant` event whose `message.model` differs from the `init` `model` ends the claim (Provider Contract item 7); then the final message, the step-5 draft plus the body line `Patch model: <init model>`, validated again, and `stage` `message` with its SHA-256 | `patch_provider_unconfined`, `patch_model_refused`, `patch_model_unverified`, `lore_rejected`, 001 REQ-12 reasons |
| 7 | Patch Policy over the raw reply (REQ-08), ending with `git apply --numstat --summary -z --check`; no command writes objects before this step passes | `no_patch`, `patch_invalid`, `path_denied`, `path_denied:<class>`, `patch_content_denied`, `patch_content_denied:control_char`, `patch_too_large` |
| 8 | Git Execution Policy item 3 again, inside the worktree, because the configuration can change during the patch request (CD-3 L4); then the expected tree: a band temp index (`GIT_INDEX_FILE`, Git Execution Policy item 1) reads the base tree, takes the diff with `git apply --cached`, and `git write-tree` gives the expected tree; `stage` `apply_intent` with the diff's SHA-256 and the expected tree; only then the diff is written to `<lp>/<key>.diff`; `git apply --index` in the worktree; the worktree's index tree (`git write-tree`) equals the expected tree; `stage` `apply_done` | `git_config_unsafe:<key>`, `patch_invalid` |
| 9 | `git commit --no-verify --cleanup=verbatim -F <msg>`; `stage` `commit_done` with the OID that `git rev-parse HEAD` prints; from here on every command names that recorded OID and never `HEAD`; `git cat-file commit <commit-oid>` shows exactly one parent, the base SHA, and the `apply_intent` expected tree as its tree (CD-3 F-006), and its message (after the header's blank line) equals the `-F` file bytes | `commit_failed`, `commit_tree_mismatch`, `commit_message_altered` |
| 10 | `stage` `branch_intent` with the commit OID; `git update-ref --no-deref refs/heads/autopus/band/<key> <commit-oid> ""`, which creates the ref only while it is absent and never writes through a symbolic ref (CD-3 F-005); `stage` `branch_done` | `branch_failed` |
| 11 | The canonical format-patch (below the table) of `<base-sha>..<commit-oid>` into memory, at most 1 MiB; `stage` `patch_intent` with the path and the SHA-256 of those bytes; the bytes to `<lp>/<key>.patch.tmp-<c8>`; rename to `<lp>/<key>.patch`; `stage` `patch_done` with the same SHA-256 | `patch_file_failed` |
| 12 | Done: keep the worktree, branch, and patch file; delete `<lp>/<key>.diff`; write the `result`. Any failure applies the Cleanup Rules to this claim's intent records; `git worktree prune` is never run | - |

- Claim commit: the OID that `commit_done` records, or, without `commit_done`, the commit whose parent is the `prep` base
  SHA, whose message hashes to the `message` stage, and whose tree is the `apply_intent` expected tree. Recovery finds it
  even when a crash came before `commit_done`; a commit that fails the message comparison is still known by `commit_done`;
  a commit a user amended to another tree is never the claim commit.
- Objects: steps 1–7 write no object. Steps 8–11 write objects; after a failure there they stay unreachable in the shared
  object database until the user's own `git gc`, and no ref points to them. The temp index and the `-F` message file live in a
  band-owned directory that `os.MkdirTemp` creates with mode 0700 in the OS temp directory; the temp index is deleted at the
  end of step 8 and the directory after step 9.
- Commit message via `lore.BuildCommit` (`pkg/lore/writer.go:11`): subject `fix(band): <series-slug> anomaly local patch
  (<episode-id>)`, or `fix(band): <series-slug> 이상 대응 로컬 패치 (<episode-id>)` when the commit-message language is `ko`;
  body with tier, z, BS ID, and the line `Patch model: <init model>` that step 6 adds; the `pkg/lore` sign-off (`writer.go:8`); the `-F` file is the `BuildCommit` output plus one
  newline. Required trailers outside Constraint, Rejected, Confidence, Directive, Tested stop step 5 (`pkg/lore/query.go:87`
  `hasField` recognizes only these five). Values: Constraint `generated by auto react band; local patch needs human review`,
  Rejected `write-capable agent; read-only proposal only`, Confidence `low`, Directive `review the patch file before running
  anything`, Tested `none; nothing was run`; `Related: <BS-ID>` is always added. Author and committer are
  `autopus-band <band@autopus.invalid>` (Git Execution Policy item 2), so no band commit carries the user's identity, even when the user's configuration sets
  `author.*` or `committer.*`, which git takes over `user.*` (CD-3 L2).
- Checkout preflight (CD-3 M5): `git ls-tree -r -l -z <base-sha>` gives the entry count and the summed blob sizes (an LFS
  pointer counts with its pointer size, since band checks out pointers and never runs git-lfs); more than 200,000 entries
  or 2 GiB gives `worktree_too_large`, and free space (`Bavail` times `Bsize` of `syscall.Statfs` on `<lp>`) below the sum
  over the blobs of each size rounded up to a multiple of `Bsize` (the file system's `f_bsize`), plus 512 MiB, gives
  `disk_insufficient`, because every non-empty file takes whole blocks (CD-3 N2). Availability: in the auditor's CD-3
  verify probe a `git worktree add` of 150,000 files took 16.5 s (APFS, git 2.50.1), so a repository near the 200,000-entry
  cap can exceed the 30 s setup deadline and end `worktree_failed`, a limit on availability and not on safety.
- Canonical format-patch (CD-3 F-010): `git format-patch --stdout --no-signature --no-thread --no-numbered
  --no-cover-letter --no-notes --no-attach --no-add-header --no-to --no-cc --no-from --no-base --no-signoff
  --subject-prefix=PATCH --full-index --no-textconv --no-ext-diff <base-sha>..<commit-oid>`. In git 2.50.1 these flags
  removed every effect of a hostile `format.signatureFile=/etc/hosts`, `format.headers`, `format.thread=deep`,
  `format.coverLetter=true`, `format.notes=true`, `format.useAutoBase=whenAble`, `format.signOff=true`, and `format.to`
  (rev 10 probe), so no file outside the commit enters the patch. Band never compares a patch file with a fresh
  format-patch: Cleanup Rule 2 and the Recovery State Table compare it with the `patch_intent` hash, so a git upgrade or a
  configuration change between runs cannot reclassify an unmodified patch file.

## BS Record

With a `local_patch` claim, `brainstorm.Write` (`pkg/brainstorm/id.go:101`) receives the pointer through a `[NEW]`
`Request.LocalPatch` field (`render.go:31-40`, plan task T8), and `Render` puts these lines at the end of the BS `## 추천 방향`
section, so the BS sections and the structural validator (`validate.go:26-27`) stay unchanged and the BS stays write-once:

```text
Local patch (3σ, local only, if produced): branch autopus/band/<key>, worktree <lp>/<key>/worktree/, patch file <lp>/<key>.patch, outside this repository.
The outcome, the changed files, and the model that wrote the patch are in the run output and in .autopus/metrics/localpatch-events.jsonl under claim <claim-id>; nothing was pushed.
Reviewer warning: this local branch holds a patch that an AI model derived from untrusted CI logs and that nothing has run. Read the whole patch file before you open the worktree in an IDE, run any command or agent in it, or push the branch, because repository hooks and tool configuration files run on checkout, commit, and build, and the edit guard does not cover that worktree.
```

`render.go` also holds three fixed diagnosis-only sentences that a local patch would contradict: the `- When:` line
(`render.go:180`), the Outcome Lock non-goal (`:196`), and the `## 추천 방향` sentence (`:208-209`). With a `local_patch` claim
they read: `- When: detected {date}; tier 3 with health_band.allow_local_patch may leave one local patch outside this
repository (see 추천 방향); band pushes nothing.`, `- Explicit non-goals: changes unrelated to {series_code}; band itself
pushes nothing and changes no GitHub state.`, and `Tier 3 with health_band.allow_local_patch adds at most one local branch,
worktree, and patch file and opens no pull request.` A BS without a `local_patch` claim keeps 001's text.

Every flag-on diagnosis BS, with or without a `local_patch` claim, also gets through a `[NEW]` `Request.DiagnosisModel` field
one line after the pointer lines, if any, at the end of `## 추천 방향` (Local Patch Provider Contract item 7; `none` stands
for an argv without `--model`): `Diagnosis model: requested <requested>, actual <actual>.`, or after a substitution
`Diagnosis model: requested <requested>, actual <actual>; model_substituted (refusal category <category>).` The patch
request runs after the BS is written and the BS stays write-once, so its models appear only in the `result` record and the run output.

`<lp>` is written as an absolute path. The BS lives in `.autopus/brainstorms/` and the records in `.autopus/metrics/`; both
are gitignored and always blocked from staging (`internal/cli/check_rules_hygiene.go:25-30`). The branch is a local ref;
`git push --all` would publish it, which the docs state.

Run output (CD-3 L4): for every `local_patch` claim of the run, the text output of `auto react band` and its JSON envelope
(`[NEW]` `local_patches[]`) show the `result` status, the patch file path, the changed paths with the added and removed
line counts of `git apply --numstat` (`files[]`), the requested and actual model of the patch request, and this warning:

```text
This patch was derived by an AI model from untrusted CI logs; read the whole patch file before running anything.
```

The BS is written before the patch request and stays write-once, so it carries the warning and the pointer, while the
outcome, the file list, and the model appear in the run output and the `result` record.

## Git Execution Policy

1. Environment: every git subprocess starts from `orchestra.EnvironWithout(os.Environ(), []string{"GIT_*"})`
   (`pkg/orchestra/provider_env.go:34`) and gets `GIT_TERMINAL_PROMPT=0`, `GIT_EDITOR=:`, `GIT_PAGER=cat`,
   `GIT_ATTR_NOSYSTEM=1` (no system attributes file), `GIT_LFS_SKIP_SMUDGE=1` (git-lfs downloads nothing, should one ever start),
   `GIT_NO_LAZY_FETCH=1` (no lazy fetch of a missing object from a promisor remote), and `GIT_NO_REPLACE_OBJECTS=1` (no
   replace ref changes the objects that the checks, the commit, and format-patch read; CD-3 F-004). Git 2.44 added
   `GIT_NO_LAZY_FETCH`, so Local
   Patch Flow step 1 refuses an older or unparsable `git version` with `git_version_unsupported:<version>`, such as
   `git_version_unsupported:2.43`. Only the three temp-index commands of Local Patch Flow step 8 also get
   `GIT_INDEX_FILE=<band temp file>`. No fetch, push, or other network command runs, so no transport or credential
   configuration is used. A lazy fetch is git's own child process, which no band argv recorder sees: in the auditor's CD-3
   probe a default `git worktree add` of a partial clone started 4 fetch or upload-pack children (trace2 `child_start`). Under `GIT_NO_LAZY_FETCH=1` the same add exited 128 with 0
   such children and left no worktree or admin entry (CD-3 verify probe), and Local Patch Flow step 1 refuses a partial
   clone before any add.
2. Flags on every git command: `-c core.hooksPath=/dev/null`, `-c core.attributesFile=/dev/null`, `-c core.fsmonitor=false`,
   `-c core.untrackedCache=false`, `-c core.symlinks=false` (a tracked symlink is checked out as a plain file that holds its
   link text, so no file of a band worktree leads out of it; CD-3 H3; the CD-3 verify probe saw the flag reach the
   `reset --hard` checkout child of `git worktree add`), `-c core.ignoreStat=false` and `-c core.sparseCheckout=false` (no
   band command sets an assume-unchanged or skip-worktree bit; CD-3 F-007), `-c core.useReplaceRefs=false` (CD-3 F-004),
   `-c commit.gpgSign=false`, `-c user.name=autopus-band`, `-c user.email=band@autopus.invalid`,
   `-c author.name=autopus-band`, `-c author.email=band@autopus.invalid`, `-c committer.name=autopus-band`,
   `-c committer.email=band@autopus.invalid` (git takes `author.*` and `committer.*` over `user.*`, so a repository or global
   `author.email` cannot reach a band commit; CD-3 L2), `-c gc.auto=0` and `-c maintenance.auto=false` (no automatic gc and
   no `git maintenance run --auto`, detached or not, which `git commit` would otherwise start in the shared object database;
   CD-3 F-003 and the worktree-safety rule), and `-c filter.lfs.process=`, `-c filter.lfs.clean=`, `-c filter.lfs.smudge=`,
   and `-c filter.lfs.required=false`, which blank the git-lfs driver, so band never starts git-lfs, LFS-tracked files stay
   pointer files, and nothing is written under `$GIT_COMMON_DIR/lfs/` (CD-3 F-009). In git 2.50.1 a later `-c` value wins
   and an empty `process`, `clean`, or `smudge` value runs no filter, while `required=true` then fails the checkout (rev 10
   probe). No git-lfs path is ever built into a command, so no generated filter value exists for the shell to re-read
   (CD-3 F-002). `format-patch` and every diff also pass `--no-textconv --no-ext-diff`.
3. Unsafe configuration, checked against `git config --list --show-scope --show-origin` and the repository files three times:
   in the user's checkout at Local Patch Flow step 1; inside the new worktree after `git worktree add --no-checkout` and
   before its checkout at step 2, because `include` and `includeIf` conditions such as `gitdir:**/worktrees/**` evaluate for
   the worktree's own git directory and can set a driver that the first check never saw (CD-3 F-001); and inside the
   worktree again at step 8:
   - `$GIT_COMMON_DIR/info/attributes` holding any line that is not blank or a comment gives `git_config_unsafe:info_attributes`.
     This refusal is mandatory, not a second layer: probe A3 showed that a linked worktree still reads that file and runs the
     filter it selects under every flag and variable of items 1–2, so no flag can replace it.
   - Any configured `filter.<driver>.clean`, `.smudge`, or `.process` gives `git_config_unsafe:filter.<driver>.<kind>` unless the
     driver is `lfs` and the raw value matches, byte for byte, the expression
     `^(git-lfs|/[A-Za-z0-9._/-]+/git-lfs) (filter-process|clean -- %f|smudge -- %f)$` (single spaces, no newline, tab, quote, `$`,
     backtick, or other shell metacharacter, because git runs filter commands through the shell). Item 2 blanks even an
     accepted `lfs` driver, so this allowlist is a second layer that keeps a hostile value out of any command that could
     miss the blanks, and band never resolves or runs the binary. An interpreter such as `/usr/bin/python3 tools/clean.py`
     is never allowed. The rule covers every scope that `--show-scope` reports, so a non-LFS
     filter that only global or system configuration sets is refused even when no attribute source selects it: an intended
     fail-closed limitation (probe A3), because a tracked `.gitattributes` at the base SHA can select any configured driver
     by name; such a user gets `git_config_unsafe:filter.<driver>.<kind>` and removes the filter or keeps the flag off.
   - Any configured `diff.<driver>.textconv` or `.command`, or `merge.<driver>.driver`, gives `git_config_unsafe:<key>`.
   - Any `lfs.extension.<name>.*` or `lfs.customtransfer.<name>.*` setting gives `git_config_unsafe:<key>`.
   - `core.alternateRefsCommand` set at all gives `git_config_unsafe:core.alternateRefsCommand`.
   - A partial clone, that is any `remote.<name>.promisor` set to true, any `remote.<name>.partialclonefilter`, or
     `extensions.partialclone`, gives `git_config_unsafe:<key>`, because its checkout fetches missing objects (CD-3 M1).
   With global and system attribute files disabled by items 1–2 and `info/attributes` empty, only tracked `.gitattributes` files
   select drivers, only the git-lfs allowlist can be configured, and item 2 blanks it, so no repository-selected command
   runs. Probe A3 (git 2.50.1) wrote 0 markers under this policy against 15 distinct markers from the same sources without
   it. A finding of the second or third check gives `git_config_unsafe:<key>` before any file is checked out or any object
   is written (CD-3 L4, F-001). Safe baseline for cleanup and recovery (CD-3 F-001): items 1–2 apply to every band git
   command, recovery included and whatever the flag; Cleanup Rule 3 runs item 3 inside a worktree before any command that
   reads its files; the branch and patch-file rules read only refs, objects, and the patch file.
4. Base SHA: `<default>` is the default branch that 001's `fetchCI` resolved (`internal/cli/react_band_ingest.go:83-111`; today
   a local variable that T8 exposes as `[NEW]` `bandCIFetch.DefaultBranch`, `:33-41`), else, under `--no-fetch` or a failed
   lookup, `git symbolic-ref --short refs/remotes/origin/HEAD` without its `origin/` prefix; it must pass
   `git check-ref-format --branch`. The base is `git rev-parse --verify refs/remotes/origin/<default>^{commit}`, otherwise
   `refs/heads/<default>`; none of these gives `base_unavailable`. No fetch runs, so the base is the last fetched state the
   user already has.
5. Worktree: `git worktree add --no-checkout --detach <worktree> <base-sha>` and the checkout of Local Patch Flow step 2 at
   `<lp>/<key>/worktree/`, for a tier-3 claim and for a diagnosis
   without a `local_patch` claim alike (each with its own `<key>`); the latter is cleaned up right after its diagnosis. With
   `core.symlinks=false`, a reviewer's own git, which runs without that flag, reports each tracked symlink of the worktree
   as a type change; the docs say so.
6. Runner: the executor runs only the git commands that this SPEC names (this policy, the Local Patch Flow, the Patch
   Policy, and the Cleanup Rules), through its own allowlist and a process-group runner modeled on `execBandRunner.Run`
   (`internal/cli/react_band_gh.go:73-84`) that stops a command as Step Timeouts and Lease states. 001's `checkBandCommand` (`:90-113`), which
   refuses every git mutation, stays unchanged, so a flag-off run keeps 001 S14's zero-mutation recorder.
7. Nothing else: the user's worktrees, index, HEAD, stash, and every ref except `refs/heads/autopus/band/<key>` stay unchanged.

## Local Patch Provider Contract

While `health_band.allow_local_patch` is true, this contract replaces item 1 (selection) and item 5 (working directory) of
SPEC-SIGMABAND-001's Provider Read-Only Contract for every diagnosis and every patch request, adds `--restricted`, `--verbose`, and
`--output-format stream-json` to its item 3 and `GIT_*` to its item 6, and keeps items 2 and 4 (REQ-03, REQ-15). With the flag off, 001's contract applies unchanged and
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
   (`pkg/config/claude_provider.go:42`: binary `claude`, `--print`, `--model claude-fable-5-1`, `--effort max`) under one
   model rule: when an `orchestra.providers.claude` entry with a `backend` has a `model` that `orchestra.SplitModelSelector`
   (`pkg/orchestra/model_family.go:9`) accepts with provider `anthropic` and a model ID that matches
   `^claude-[a-z0-9][a-z0-9.-]*$`, that model ID replaces the `--model` value, its thinking suffix is dropped, and
   `--effort max` stays (this repository: `anthropic/claude-opus-5-5:max` gives `--model claude-opus-5-5`); every other case
   keeps `claude-fable-5-1`. An OMP entry's `binary` (default `omp`) and `tools` have no subprocess meaning and are never
   read, and its `model` is read only by this rule, so this is no fallback of the OMP provider: band never runs the OMP
   entry. Rule 2 takes `providerConfigFromEntry` of the entry, as 001 does. The result has
   no `Backend`, so `orchestra.RunSingleProvider` runs the projected argv as a subprocess
   (`pkg/orchestra/provider_backend_route.go:26-30`), and band's execution config registers no OMP route.
3. Projection: `applyReadOnlyProviderPolicy` with `[NEW]` `readOnlyPolicyOptions.Confined` (plan task T6) sets
   `--output-format stream-json` (replacing a configured value), adds `--verbose` and `--restricted` before the last item
   `--tools=Read,Grep,Glob`, and refuses a provider that has a `Backend` or a name other than `claude`,
   because the shared projection passes an OMP-backed provider through as-is (`orchestra_readonly_policy.go:59-63`); a refusal
   gives `unavailable(provider_unconfined)`. Then `bandReadOnlyControls` and a check for `--restricted`, `--verbose`, and
   `--output-format stream-json` run fail-closed, and a projection without one of them gives
   `unavailable(provider_unconfined)`. An argv item outside the claude allowlist (`:148-185`), `--bare` and a configured
   `--verbose` included (the bool-flag list has no `--verbose`, `:150-153`), still gives `unavailable(provider_policy_rejected)`.
4. The patch request resolves its provider by this contract again; anything but a confined subprocess claude ends the claim
   `failed:patch_provider_unconfined` (Local Patch Flow step 6). A missing binary, a timeout, a non-zero exit, and empty output
   keep 001's REQ-12 reasons.
5. Orchestra stays unchanged: `resolveProviders` (`internal/cli/orchestra_config.go:92`) and every orchestra command keep
   `orchestra.providers.<name>.backend`; only band reads `local_patch_provider`, and band never writes `autopus.yaml`.
6. Expected deployment: the subscription-authenticated `claude` CLI (Configuration). Band never projects `--bare`, whose
   authentication reads only `ANTHROPIC_API_KEY` or an `apiKeyHelper` and never the subscription login, while `--safe-mode`
   keeps authentication working (claude 2.1.289 help). Probe A1 (2026-10-08, claude 2.1.289) confirmed it: a `--restricted`
   session signed in through the subscription login (`init` `apiKeySource` `none`), offered only `Glob`, `Grep`, and `Read`
   with no MCP server, and its permission layer, not the model, denied a `Read` outside the worktree (`permission_denials`).
   Git Execution Policy item 2 checks every tracked symlink out as a plain file, so no link in the worktree leads a `Read`
   outside it (CD-3 H3), and a tracked `.claude/settings.json` or `.mcp.json` of the worktree (`apiKeyHelper`, hooks, `env`)
   has no effect: `--restricted` ignores project and local settings, `--safe-mode` disables hooks, and `--strict-mcp-config`
   loads no `.mcp.json` server (S7 live check).
7. Stream and model record: claude can retry a refused request on another model and say so only in its event stream (probe
   A1 run 2: a `system` event with subtype `model_refusal_fallback`, `original_model` `claude-fable-5-1`, `fallback_model`
   `claude-opus-4-8`, `api_refusal_category` `cyber`; the text output showed nothing). A confined request therefore sets
   `MaxOutputBytes` to `[NEW]` `bandProviderStreamBytes` + 1 (8 MiB, room for the JSON escaping of a 1 MiB text and for tool
   events) in place of 001's `ProviderCaptureBytes` + 1 (`react_band_diagnose.go:258`), and band parses the captured stream
   as JSON lines (a line that is not a JSON object is skipped), keeping only the `system` `init` event's `model`, every
   `model_refusal_fallback` event, the `message.model` of every `assistant` event, and the last `result` event. A stream without a `result` event whose `subtype` is
   `success` and whose `is_error` is false is empty output under item 4 (for a diagnosis 001's `provider_empty_output`,
   `:39,272`), and so is a diagnosis stream past the bound; a patch-request stream past the bound gives `patch_too_large`.
   The `result` text is the reply: 001's 1 MiB `NewHeadBuffer` capture (`:268`) and sanitization take it for a diagnosis,
   Patch Policy item 1 for a patch. Per request, `requested` is the `--model` value, `actual` the last `fallback_model`, else
   the last `assistant` `message.model` that differs from the `init` `model`, else the `init` `model`, and `refusal_category` the last `api_refusal_category`; they go into the `result` record's
   `models[]` and, for the diagnosis, the BS (BS Record), and `model_substituted` is true when any `model_refusal_fallback`
   event occurred or an `assistant` model differs from the `init` model. A substitution during a diagnosis is recorded and the diagnosis proceeds unchanged. A
   `model_refusal_fallback` event during the patch request ends the claim `failed:patch_model_refused` at Local Patch Flow
   step 6, with its `models[]` entry recorded and before any Patch Policy check, because a safeguard refused that prompt on
   the configured model and band applies code only from the model that the operator selected (CD-3 M4). A patch-request
   stream without a `system` `init` event, or with an `assistant` event whose `message.model` differs from the `init`
   `model`, ends the claim `failed:patch_model_unverified` at the same point (a `model_refusal_fallback` event, when present, gives
   `patch_model_refused` instead), because band cannot tell which model wrote the
   reply (CD-3 N5); in probe A1 every `assistant` event named the `init` model (run 3) or, after the fallback event, the
   fallback model (run 2). A diagnosis records the same values and proceeds.

## Patch Prompt Contract

The patch request is a second provider prompt rendered by `promptlayer.Render` (`pkg/promptlayer/layer.go:89`); its manifest
goes into the `result` record. Its snapshot layer is 001's `healthband.EvaluationLayer` (`pkg/healthband/prompt.go:113`) of
the event that holds the diagnose claim. Its ephemeral layers take the sanitized texts the diagnose claim holds in memory
(the diagnosis output and the `RunLog` evidence), never a re-read of the BS file. 001's `evidenceLayer` (`prompt.go:164`) is
not reused, because it fixes the `untrusted-evidence` info string.

| Layer ID | Kind | Content | Cache eligible |
|----------|------|---------|----------------|
| `band.patch_instructions.v1` | stable | role; the diff is a proposal that band checks and applies outside the session, which edits no file (plan mode); "answer with exactly one diff fence"; no commands; the nonce rule | true |
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
untracked files of the user's checkout, such as `.env`, are not in it. The proposal framing is load-bearing: in probe A1
run 1 the plan-mode session declined to print a throwaway diff and gave no fence, while run 2, which framed the diff as a
proposal, answered with one well-formed fence.

## Patch Policy

The items run in order, and the first item that fails gives the code.

1. The reply is the `result` text of the provider stream (Local Patch Provider Contract item 7), captured at
   `healthband.ProviderCaptureBytes` (1 MiB, `pkg/healthband/sanitize.go:16`) as 001's diagnosis capture is
   (`react_band_diagnose.go:253-276`); a stream past its 8 MiB bound or a text capture that dropped bytes gives
   `patch_too_large`. The reply holds exactly one ```` ```diff ```` fence (else `no_patch`), and the diff is at most 64 KiB
   (else `patch_too_large`).
2. Paths are decoded before any check: a path in a `diff --git`, `---`, `+++`, `rename`, or `copy` header that starts with `"` is a
   C-quoted string whose escapes (`\a \b \f \n \r \t \v \\ \"` and three-digit octal) are decoded to bytes; every other path is
   taken as is. Every check below runs on the decoded bytes, and the decoded path set must equal the NUL-separated path set that
   `git apply --numstat --summary -z --check` reports (it writes no object), else `patch_invalid`. Added and removed lines
   are counted per hunk from its header `@@ -a[,b] +c[,d] @@`, where an omitted count is 1: the hunk body is the next lines
   until `b` old and `d` new lines are consumed, a line that starts with a space counts once on each side, `-` on the old
   side, `+` on the new side, and `\ No newline at end of file` on neither; every `+` line of a body is an added line, even
   when it reads `+++`, and no line outside a body is content (CD-3 N4). Each file's added and removed counts must equal its
   `--numstat` counts, else `patch_invalid`. Band then reads the base entry of every touched path and of each of its
   directory prefixes with `git --literal-pathspecs ls-tree -t -z <base-sha> -- <paths>`, whose `-t` also lists the trees
   that ls-tree recurses into (CD-3 H2, N3), and the `filter` attribute of every touched path at the base with
   `git check-attr -z --source=<base-sha> filter -- <paths>`.
3. Any rename, copy, deletion, mode change, symlink (120000), gitlink (160000), binary patch, new file whose mode is not 100644,
   absolute path, `..`, decoded path that is not valid UTF-8, or decoded path with a code point of the item-7 set, TAB and CR
   included (CD-3 N1), gives `patch_invalid`. The allowed base entries are a 100644 or 100755 blob for a modified path, no
   entry for an added path, and no entry or a tree (040000) for a directory prefix; any other entry that `ls-tree -t`
   reports gives `patch_invalid` (CD-3 N3). A touched path whose `filter` attribute is set at the base, `lfs` included,
   gives `path_denied:filter`, because item 2 of the Git Execution Policy blanks the git-lfs driver and an edit of an LFS
   pointer file would commit raw content (CD-3 F-009). The base mode decides, not the headers: a
   modeless hunk on a tracked symlink or gitlink has no mode line, so `git apply --numstat --summary -z --check` reports
   nothing, yet in the auditor's CD-3 probe such a hunk retargeted a tracked `link.go` to `../../../../../../../.ssh/id_rsa`
   through `git apply --index`.
4. Collisions (CD-3 M2): every decoded path and each of its directory prefixes is folded (NFC normalization, then Unicode
   case folding, so an NFD and an NFC spelling fold alike). Two distinct paths or prefixes of the diff with one fold, or a
   path or prefix whose fold equals that of a different tracked path or directory at the base SHA
   (`git ls-tree -r -z --name-only <base-sha>` with its directory prefixes), give `path_denied:case_collision`. In the
   auditor's CD-3 probe, `pkg/n.go` and `pkg/N.go` added by one diff both entered the index while APFS kept one file.
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
   `.pnpmfile.cjs`, `Package.swift`, `gulpfile.*`, `Gruntfile.*`, `magefile.go`, `Dangerfile.*`. Structural rules (CD-3 M3),
   also after folding: any path segment that starts with `.`, with no exception (every dot entry above falls under it and
   stays listed for clarity); any file name matching `*.conf.*`, `*rc.*`, or `.*rc` (tool configuration; `*rc.*` also
   denies a name such as `src.go`, an intended fail-closed limitation); the Gradle included-build directories
   `build-logic/` and `buildSrc/`; `.yarn/` and `.pnp.*`; and every modification of a path whose base mode is 100755, an
   executable that a hook, a script, or a reviewer may run. Each gives `path_denied`.
7. Added lines (counted as item 2 counts): first a check of the raw added lines (CD-3 H1, N1), because 001's `Sanitize`
   strips such characters silently while `git apply` applies the raw diff. An added line that is not valid UTF-8 or that
   holds a code point of general category Cc, Cf, Zl, Zp, or Co, or with the property Default_Ignorable_Code_Point, gives
   `patch_content_denied:control_char`, with two exceptions: TAB, and a CR that is the one byte right before the LF that
   ends the line in the diff text (a CRLF line ending passes, while a lone CR, an embedded CR, and CR CR LF do not). The set
   holds U+2028 and U+2029, the Hangul fillers U+3164, U+115F, U+1160, and U+FFA0, U+00AD, U+034F, U+180E, U+2061–U+2064,
   the variation selectors U+FE00–U+FE0F, the tag block U+E0000–U+E007F, and every bidi control, zero-width mark, C0, DEL,
   and C1 control that rev 9 listed. Go's `unicode` package has no derived Default_Ignorable_Code_Point table, so the
   check is `unicode.In` over `Cc`, `Cf`, `Zl`, `Zp`, `Co`, `Other_Default_Ignorable_Code_Point`, and `Variation_Selector`,
   a superset of the derived property; in Go 1.26 (Unicode 17.0.0) it holds every code point named here and the whole tag
   block, and no letter such as `a`, U+00E9, or U+D55C (rev 10 probe). Then `healthband.Sanitize`
   (`pkg/healthband/sanitize.go:88`) over the raw added lines, which applies 001's band
   secret forms (`bandSecretPatterns`, `sanitize_secrets.go:16`) before `promptlayer.SanitizeContent`, reports neither
   `secret_risk` nor `injection_risk`, and no added line holds a run of 40 or more base64 or hex characters
   (`patch_content_denied`).
8. Size: at most 10 files and 400 changed lines (added + removed, counted as item 2 counts) (`patch_too_large`).
9. Edit guard parity: band applies the diff with git from the CLI process, which no pre-tool hook sees, and the band worktree
   holds no `.autopus/*-manifest.json` or `.autopus/runtime/fix-locks/` record (both gitignored, `.gitignore:11,30`;
   `docs/edit-guard.md:92`). The policy therefore calls `editguard.Decide` (`pkg/editguard/decide.go:116`) with a `Call`
   (`:24-36`) whose `Cwd` is the top level of the user's checkout and whose `Targets` are the decoded paths, which classifies
   them where the manifests and fix locks live; a deny gives `path_denied:<class>` (`guard_state`, `fix_lock`, or
   `generated_surface`). A guard fault, which the guard's own contract turns into an allow with a non-empty
   `Decision.Diagnostic` (`decide.go:40-45,116-139`), gives `path_denied:guard_fault` here, so band fails closed where an
   agent edit would proceed (CD-3 L1); items 5–6 stay the primary path rules.

## Data Contracts

- `.autopus/metrics/localpatch-events.jsonl` (`autopus.band_localpatch.v1`, written under SPEC-SIGMABAND-001's store lock
  through its helpers `appendStoreFile`, `writeStoreFileAtomic`, and `readStoreLines` with `openStoreFile`'s no-follow open
  (`pkg/healthband/store_compact.go:129-258`), and compacted with the same rules, never dropping a record of a non-terminal
  claim) holds five record kinds:
  - `decision`: `seq`, `series`, `episode_id`, `evaluation_seq`, `decision` (`claim` or `skipped`), `reason`.
  - `claim`: `seq`, `claim_id`, `owner`, `lease_until`, `series`, `episode_id`, `depends_on` (the diagnose claim id), `key`,
    `worktree_path`, `patch_path`, `branch`; written in phase A. A stored path or branch is only compared with its derived
    value (Derived paths below) and never used.
  - `prep`: `seq`, `series`, `episode_id`, `diagnose_claim_id`, `lease_until` (both from 001's `DueClaim`,
    `pkg/healthband/catchup.go:30-42`), `claim_id` (when a `local_patch` claim exists), `key`, `base_sha` (once resolved),
    `code` (`ok` or a step-1 code); at most one per flag-on diagnosis, appended at the end of step 1 in the same store-lock
    section as the re-read that found no `result`, and before any artifact exists.
  - `stage`: `seq`, `claim_id` (the `local_patch` claim id, else the diagnose claim id), `phase` (`worktree_intent`,
    `worktree_done`, `worktree_failed`, `message`, `apply_intent`, `apply_done`, `commit_done`, `branch_intent`,
    `branch_done`, `patch_intent`, `patch_done`), and per phase `path` (`worktree_intent`, `patch_intent`), `status_sha256`
    (`worktree_done`), `code` (`worktree_failed`), `message_sha256` (`message`), `diff_sha256` and `tree` (`apply_intent`),
    `tree` (`apply_done`), `commit_oid` (`commit_done`, `branch_intent`), or `patch_sha256` (`patch_intent`, `patch_done`). Every `*_intent` is written before the command that creates the
    artifact, so every artifact that may exist is named by a durable record first.
  - `result`: `seq`, `claim_id`, `status` (`done` or `failed:<code>`), `bs_id`, `base_sha`, `commit_sha`, `branch`,
    `worktree_path`, `patch_path`, `prompt_manifest`, `recovered`, `kept[]` (artifacts kept by a cleanup rule, with reason),
    `models[]` (one entry per confined request of the claim that returned a stream: `request` (`diagnosis` or `patch`),
    `requested`, `actual`, `refusal_category`), `model_substituted`, and `files[]` (`path`, `added`, and `removed` from
    `git apply --numstat`, once step 7 has passed); a diagnosis without a `local_patch` claim writes one
    too, keyed by its diagnose claim id. A `result` is write-once: its append checks under the store lock that the claim has
    no `result` yet and is dropped otherwise, so a live claim and recovery never both end one claim (CD-3 M7, F-011).
- `.autopus/metrics/localpatch-state.json` (`autopus.band_localpatch_state.v1`): claims per series, written after the events and
  replayed like SPEC-SIGMABAND-001's checkpoint. SPEC-SIGMABAND-001 binaries never read these two files. Their codes may hold
  `:` and `.` (`failed:git_config_unsafe:filter.lfs.process`), which 001's `validResultStatus` would refuse
  (`wal_validate.go:31-34`); no such code reaches 001's events, whose diagnosis status only gains
  `unavailable(provider_unconfined)` and `unavailable(worktree_unavailable)`.
- Derived paths (CD-3 L3): recovery and the Cleanup Rules never use a path or branch that a record stores. They take `<lp>`
  from the current `os.UserCacheDir` and `<repo-hash>`, require the record's `key` to match `^[a-z0-9-]+$`, to end in
  `-<c8>` of the claim id that the `<key>` definition names, and to equal the key that the `<key>` rule gives for the
  record's series and episode ID,
  derive `<lp>/<key>/worktree/`, `<lp>/<key>.patch`, `<lp>/<key>.diff`, `<lp>/<key>.lock`, and
  `refs/heads/autopus/band/<key>` from it, and compare every stored path with its derived value. Any mismatch, a changed
  user cache directory included, ends the claim `failed:record_invalid` and touches nothing. `<lp>` itself (CD-3 F-008): band
  opens the real path of `os.UserCacheDir` (`filepath.EvalSymlinks`) as an `os.Root`, creates or opens `autopus`,
  `local-patches`, and `<repo-hash>` through that root with `Lstat` showing a directory and never a symlink at each
  component, and opens every file below `<lp>` through it; the real path of `<lp>` must neither equal nor lie inside the
  real path of the repository's top level, its common directory, or any worktree that `git worktree list --porcelain -z`
  lists, else `cache_unavailable` at step 1 and `record_invalid` in recovery.
- Cleanup Rules (one set for the live run and for recovery; only artifacts named by the claim's intent records, at their
  derived paths, are considered;
  every git call has a 30 s timeout; the rules run in the order 3, 1, 2, so the worktree check still finds the diff file):
  3. Worktree: only the admin entry whose path equals the derived worktree path, which the `worktree_intent` path must
     equal, is considered. The `locked` check comes
     first and alone decides an entry that still holds the `locked` file of an interrupted `git worktree add` (content
     `initializing`, beside an `index.lock`): it is kept with reason `worktree_incomplete`, because band cannot prove that a
     partial checkout holds only its own files, and the HEAD test below cannot tell it apart, since its admin HEAD already
     equals the base SHA (probe A3). `git worktree remove --force` refuses a locked entry (exit 128), so the docs name
     `git worktree remove --force --force <path>` for a human to run; band never passes `--force` twice and never runs
     `git worktree unlock`. In the CD-3 verify probe, SIGTERM to the process group of a 150,000-file `git worktree add` made
     git remove its partial worktree and admin entry, and SIGKILL left the `locked` entry with partial files. Next, an entry
     whose admin directory holds no `index` file is removed with `git worktree remove --force <path>` when its worktree
     directory holds only its `.git` file (an add without checkout, step 2) and is otherwise kept with reason
     `worktree_incomplete` (a checkout that a stop interrupted). Before any command that reads the worktree's files, item 3
     of the Git Execution Policy runs inside it, and an unsafe result keeps it with reason `git_config_unsafe` (CD-3 F-001).
     Any other entry is removed with `git worktree remove --force <path>` only when its HEAD is the base SHA or the claim
     commit, `git ls-files -v -z` shows no assume-unchanged (lowercase tag) or skip-worktree (`S`) entry (CD-3 F-007), and
     `git status --porcelain -z --untracked-files=all --ignored` prints nothing, or prints output that, followed by the
     `git diff --no-ext-diff --no-textconv --binary` output, hashes to the `worktree_done` `status_sha256` (a repository
     whose `text` or `eol` attributes show ` M` on a fresh checkout; CD-3 M5), or, while HEAD is the base SHA and the index
     tree is the `apply_intent` expected tree, prints only staged entries (`M ` or `A `) of the diff's paths. Any index
     flag and any untracked (`??`), ignored (`!!`), or unstaged entry keep it with reason `worktree_modified` (CD-3 M6: in the auditor's
     CD-3 probe `remove --force` deleted an ignored `secret.env` that `--untracked-files=all` alone does not list), and a
     HEAD that is neither commit keeps it with reason `head_unrecognized`. After a removal, an empty `<lp>/<key>/` is
     removed as well.
  1. Branch: a symbolic ref at that name (`git symbolic-ref -q --no-recurse`) is kept with reason `branch_moved`; otherwise
     `git update-ref --no-deref -d refs/heads/autopus/band/<key> <claim-commit>`, which deletes only that ref and only while
     it points at the claim commit (CD-3 F-005); a branch at another OID is kept with reason `branch_moved`.
  2. Patch and diff files: `<lp>/<key>.patch` or its `.tmp-<c8>` is deleted only when its SHA-256 equals the `patch_intent`
     hash, so a file a reviewer edited is kept with reason `patch_modified` and no git version or configuration change can
     reclassify an unmodified file (CD-3 F-010); `<lp>/<key>.diff` is deleted only when its SHA-256 equals the `apply_intent` hash.
- Recovery step: runs in every non-dry-run band run in which `.autopus/metrics/localpatch-events.jsonl` exists, whatever the
  flag, so a claim interrupted while the flag was on is still handled after the flag is turned off. Its place in `execute`
  (`internal/cli/react_band.go:139-179`) is after `fetchCI` and before the `store.Lock` of phase A. It never runs under
  `--dry-run`, which loads the log through `planDry` without a lock or a write (001 REQ-14), and a run without that file
  creates no `.recovery.lock`. It takes `.autopus/metrics/.recovery.lock` (`filelock.Acquire`, `pkg/filelock/lock.go:37`,
  wait at most 5 s; otherwise the run reports reason `recovery_locked` in its JSON envelope and text output and recovers
  nothing). It reads this SPEC's records under the store lock (local file IO only), then, without the store lock, handles
  every `local_patch` claim and every flag-on diagnosis (`prep` record) whose `lease_until` has passed without a `result`
  (never retried) by the Recovery State Table, with a budget of 120 s per claim, and appends each `result` under the store
  lock. Per claim it first derives the paths (Derived paths) and takes the key lock (`filelock.Acquire` on
  `<lp>/<key>.lock`, zero wait); a claim whose lock another process still holds, such as a live claim that a suspended or
  stuck process keeps past its lease, gets no result, the run reports reason `recovery_key_locked`, and a later run handles
  it (CD-3 M7). With the key lock held, recovery re-reads the claim's records under the store lock and acts on that fresh
  read, never on its first snapshot: a `result` that appeared in between, from a live claim that finished and released the
  lock after the first read, ends recovery's handling of that claim with no action and no record (CD-3 M7, F-011). After
  the claim's `result`, recovery unlinks the lock file. Phase A only reads these results.
- Recovery State Table (last durable record of the claim → what recovery finds → action → claim state):

| Last durable record | Found | Action | State |
|---------------------|-------|--------|-------|
| any | a `key` or stored path that differs from its derived value (Derived paths) | none | `failed:record_invalid` |
| any | `<lp>/<key>.lock` held by another process | skip the claim; run reason `recovery_key_locked` | no `result` yet; a later run handles it |
| any | a `result` of the claim at the re-read after the key lock | none; unlink the key lock | that `result` stays the only one |
| `decision`, `claim`, or `prep` | no artifact | none | `failed:interrupted` |
| `worktree_intent` | an admin entry with the `locked` file of an interrupted add, whatever its HEAD | keep it, `worktree_incomplete` in `kept[]` | `failed:interrupted` |
| `worktree_intent` | a worktree at the path with or without its checkout, or none | Cleanup Rule 3 | `failed:interrupted` |
| `worktree_failed` | an admin entry at the path, or none | Cleanup Rule 3 | `failed:<code>` of that record |
| `worktree_done` (diagnosis or patch request in flight) or `message` | worktree at the base SHA | Cleanup Rule 3 | `failed:interrupted` |
| `apply_intent` | worktree at the base SHA whose index tree is the base tree or the expected tree | Cleanup Rules 3, 2 | `failed:interrupted` |
| `apply_done` or `commit_done` | HEAD at the base SHA or at the claim commit | Cleanup Rules 3, 2 | `failed:interrupted` |
| `branch_intent` or `branch_done` | branch absent or at the claim commit | Cleanup Rules 3, 1, 2 | `failed:interrupted` |
| `patch_intent` | `<lp>/<key>.patch` whose SHA-256 equals the `patch_intent` hash, branch and worktree HEAD at the claim commit, clean worktree | delete the diff file, keep the rest | `done`, `recovered: true` |
| `patch_intent` | anything short of that complete set | Cleanup Rules 3, 1, 2 | `failed:interrupted` |
| `patch_done` | anything | delete the diff file when its hash matches; keep the rest and list in `kept[]` each artifact that differs from its record | `done`, `recovered: true` |
| any row above | an artifact a user changed | keep it, list it in `kept[]` with its reason | the state of that row |

## Step Timeouts and Lease

| Step group | Steps | Deadline |
|------------|-------|----------|
| diagnosis setup | 1–2 up to `worktree_done` (checks with the checkout preflight, the key lock and the store-locked re-read, three records, `git worktree add --no-checkout`, the check inside the worktree, the checkout, and the status hash) | 30 s |
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
hook, whose row-5 claims `Plan` chains with 810 s right after their diagnose claim. A diagnose and its `local_patch` first in a
run get 990 s and 1,800 s. A checkout of the base that does not fit the 30 s setup deadline ends `worktree_failed`.

Stopping (CD-3 M5): a command that outlives its step group gets SIGTERM on its process group 5 s before the group's deadline
and SIGKILL at the deadline, so the stop stays inside the budget. The CD-3 verify probe confirmed that SIGTERM makes
`git worktree add` remove its partial worktree and admin entry and that SIGKILL leaves a `locked` entry; since rev 10 the
long command is the step-2 checkout, whose interrupted entry has no `index` file and is kept as `worktree_incomplete`
(Cleanup Rule 3).

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
| [NEW] `pkg/healthband/patchpolicy.go`, `patchprompt.go`, `commitmsg.go`, `gitpolicy.go` | Patch Policy (with the base-mode read, folded collisions, structural rules, the control-character check, and the fail-closed `editguard.Decide` check), Patch Prompt Contract, Lore message, Git Execution Policy (with the version, partial-clone, and step-8 re-checks) |
| [NEW] `internal/cli/react_band_localpatch.go` | executor of the Local Patch Flow with its own git allowlist and runner |
| [NEW] `internal/cli/react_band_localpatch_provider.go` | Local Patch Provider Contract: selection order, band-only subprocess form, confined projection and its control checks (T7) |
| existing (001) `pkg/config/schema_health_band.go:8-14` | gains `AllowLocalPatch bool` with `yaml:"allow_local_patch,omitempty"` and `LocalPatchProvider string` with `yaml:"local_patch_provider,omitempty"` (T1) |
| existing (001) `pkg/healthband/episode.go:21-27`, `catchup.go:12-22,51-58,73-111`, `claims.go:84-117` | `local_patch` budget in `claimBudgets`; `[NEW]` `PlanOptions` budget override and `LocalPatch` hook with `[NEW]` `Plan.LocalPatch`; `[NEW]` `ExecuteOptions.AfterRecord` (T8) |
| existing (001) `internal/cli/react_band.go:139-255` | recovery step between `fetchCI` and `store.Lock`; `<lp>` resolved before `store.Lock`; decision and claim records after `Commit`; the hooks in `phaseA` and `phaseB`; the `[NEW]` `local_patches[]` run output (T8) |
| existing (001) `internal/cli/react_band_diagnose.go:54-62,72-86,140-276` | `[NEW]` provider work directory apart from the BS directory, steps 1–2 inside `Run`, the flag-on provider from the Local Patch Provider Contract in place of `selectBandProvider` and `resolveProvider`, `GIT_*` added to the unset list, the worktree path redacted by the output sanitizer, the two new unavailable reasons (T8) |
| existing (001) `internal/cli/react_band_ingest.go:33-41,83-111` | `[NEW]` `bandCIFetch.DefaultBranch` (T8) |
| existing (001) `pkg/brainstorm/render.go:31-40,180,196,208-209` | `[NEW]` `Request.LocalPatch`: pointer lines and the three local-patch sentences (T8) |
| existing (001) `internal/cli/react_band_help.go:9-58`, `docs/health-band.md:223-228`; existing `CHANGELOG.md` | flag, `local_patch_provider` with the subscription claude CLI deployment, artifact locations, reviewer warning, subprocess claude requirement, upgrade note (T8 help text, T9 docs) |
| existing `internal/cli/orchestra_readonly_policy.go:11-15,53-88,209-217` | `[NEW]` `readOnlyPolicyOptions.Confined`, which adds `--restricted`, `--verbose`, and `--output-format stream-json` to the claude projection and refuses a provider with a `Backend` or a name other than `claude` (T6) |

## Related SPECs

- SPEC-SIGMABAND-001 (implemented, merged): plan task T8 owns this SPEC's edits to the 001 files of 생성 파일 상세. While the
  flag is true, this SPEC amends: the diagnosis provider selection (REQ-15) and its cwd, projection, and environment (REQ-03); the diagnosis status values
  `unavailable(provider_unconfined)` and `unavailable(worktree_unavailable)`; the diagnose budget (990 s) and the lease chain,
  which also counts `local_patch` budgets, so claim `owner`/`lease_until` values differ from a flag-off run (REQ-12); the BS
  pointer lines and three sentences (BS Record); the run's text output and JSON envelope, which gain `local_patches[]` (BS Record); REQ-22, whose redaction does not apply to the raw patch reply, which never
  reaches a prompt, BS, or terminal (REQ-07); and the run order, which gains the recovery step between 001's network step and
  phase A whenever this SPEC's log exists. It never changes 001's action enum, decision table, evaluation fields other than
  claim leases, BS sections, or `checkBandCommand`. At sync it adds `allow_local_patch` and `local_patch_provider` to 001
  REQ-15's key list (REQ-01) and amends the text that still names the retired draft PR path: 001 `spec.md` Related SPECs, `docs/health-band.md:226-228`, and the `HealthBandConf`
  comment (`schema_health_band.go:3-7`).
- SPEC-REVIEWRO-001 (implemented): owns the shared claude projection, which already carries `--permission-mode plan`,
  `--safe-mode`, `--no-session-persistence`, `--disable-slash-commands`, `--strict-mcp-config`, and the last item
  `--tools=Read,Grep,Glob` (`orchestra_readonly_policy.go:209-217`); this SPEC adds only the confined option, which appends
  `--restricted` and `--verbose`, sets `--output-format stream-json`, and refuses a provider with a `Backend` (the projection
  passes OMP providers as-is, `:59-63`), and the claude bool-flag list (`:150-153`) needs no change because the projection,
  not the user's argv, adds both flags.
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
| REQ-03 | T6, T7, T8 | S7, S13, S15 | INV-06, INV-12 |
| REQ-04 | T7 | S3, S6 | INV-02 |
| REQ-05 | T4, T7 | S4, S6, S7 | INV-03, INV-06 |
| REQ-06 | T7 | S5 | INV-08 |
| REQ-07 | T5, T7 | S8, S15 | INV-06, INV-12 |
| REQ-08 | T3 | S5 | INV-05, INV-10 |
| REQ-09 | T7 | S4 | INV-07 |
| REQ-10 | T4, T7, T10 | S12 | INV-04 |
| REQ-11 | T2, T7 | S5, S9 | INV-09 |
| REQ-12 | T2, T7, T8 | S2, S3, S9 | INV-02 |
| REQ-13 | T8, T9 | S4, S11 | INV-01, INV-07 |
| REQ-14 | T2, T7 | S3, S4 | INV-02, INV-07 |
| REQ-15 | T6, T7, T8 | S7, S13, S14 | INV-11 |

## Review Resolution

Rev 10 (2026-10-08) applies the security-auditor's CD-3 verify of rev 9 (repro `cd3v-probe/probe.sh`, git 2.50.1) and the
rev 9 spec review (codex, F-001–F-011; F-011 is M7). CD-3 stays open until the verify of rev 10 passes:

| Item | Resolution | Where |
|------|------------|-------|
| verified assumptions | `GIT_NO_LAZY_FETCH=1` on a blob:none partial clone: `git worktree add` exits 128 with 0 fetch children and leaves nothing; SIGTERM to the process group during a 150,000-file add: git removes the worktree and its admin entry, while SIGKILL leaves `locked` and partial files; `-c core.symlinks=false` reaches the `reset --hard` child; the claude settings and MCP marker check stays a T10 live item | Git Execution Policy items 1–2, Cleanup Rule 3, Step Timeouts and Lease, plan.md A3 |
| L2 author and committer | `-c author.name`, `author.email`, `committer.name`, and `committer.email` beside `user.*`, which they outrank | Git Execution Policy item 2, REQ-05, S4, S6 |
| M7, F-011 key lock and fresh read | the key lock is taken before `prep`; the live claim writes `prep`, and recovery acts, only after a store-locked re-read under the key lock finds no `result`; `result` is write-once | Local Patch Flow step 1, Data Contracts, Recovery State Table, REQ-11, S9 |
| M5 retention and eol worktrees | step 1 counts again before every flag-on worktree (`cap_reached`, tier 2 included); `worktree_done` records `status_sha256` over status and diff, and an equal hash allows removal | Local Patch Flow steps 1–2, Decision Table, Cleanup Rule 3, REQ-14, S3, S5 |
| N1 property-based characters | Cc (TAB and the CRLF rule excepted), Cf, Zl, Zp, Co, and Default_Ignorable_Code_Point, checked with Go `unicode` tables; the same set, TAB and CR included, for paths | Patch Policy items 3 and 7, REQ-08, S5 |
| N2 free space | Σ roundup(size, `f_bsize`) over the blobs plus 512 MiB | Local Patch Flow checkout preflight, S3 |
| N3 base entries | `git ls-tree -t -z`; allowed: none, 100644, 100755, and 040000 only as a prefix | Patch Policy items 2–3, S5 |
| N4 added lines | hunk-header counting; per-file counts equal `git apply --numstat`; a `+++` content line is an added line | Patch Policy items 2, 7, and 8, S5 |
| N5 unverified patch model | no `init` event, or an `assistant` `message.model` other than the `init` model, ends a patch request `failed:patch_model_unverified` | Provider Contract item 7, Local Patch Flow step 6, REQ-03, S15 |
| availability | a 150,000-file add took 16.5 s, so a repository near the 200,000-entry cap can exceed the 30 s setup deadline (`worktree_failed`) | Local Patch Flow checkout preflight, Step Timeouts and Lease |
| commit body model | the `init` model of the patch request goes into the commit body as `Patch model: <model>`; the message is drafted at step 5 and finished at step 6 | Local Patch Flow steps 5–6, S4, S15 |
| F-001 includeIf | `git worktree add --no-checkout`, item 3 inside the new worktree, then the checkout; Cleanup Rule 3 runs item 3 before reading a worktree; items 1–2 bind recovery whatever the flag | Git Execution Policy items 3 and 5, Local Patch Flow step 2, Cleanup Rule 3, REQ-04, REQ-05, S6, S9 |
| F-002 generated LFS value | resolved by removal: band blanks the git-lfs driver and never builds a git-lfs path into a command; a configured value still has to pass the byte-exact allowlist | Git Execution Policy items 2–3, S6 |
| F-003 maintenance | `-c maintenance.auto=false` beside `-c gc.auto=0` | Git Execution Policy item 2, S6 |
| F-004 replace refs | `GIT_NO_REPLACE_OBJECTS=1` and `-c core.useReplaceRefs=false` | Git Execution Policy items 1–2, S6 |
| F-005 symbolic refs | `update-ref --no-deref` for create and delete; a symbolic ref at the band name gives `branch_exists` or `branch_moved` | Local Patch Flow steps 4 and 10, Cleanup Rule 1, S5, S9 |
| F-006 commit tree and range | the recorded commit must have the base as its only parent and the expected tree (`commit_tree_mismatch`); format-patch takes `<base-sha>..<commit-oid>` | Local Patch Flow steps 9 and 11, REQ-09, S4, S5 |
| F-007 index flags | `-c core.ignoreStat=false` and `-c core.sparseCheckout=false`; an assume-unchanged or skip-worktree entry keeps the worktree | Git Execution Policy item 2, Cleanup Rule 3, S9 |
| F-008 cache path | `os.Root` walk from the real user cache directory with no symlink component; `<lp>` outside the top level, the common directory, and every worktree | Data Contracts Derived paths, REQ-14, S3 |
| F-009 git-lfs writes | chosen: band never runs git-lfs (blanked driver) and refuses a touched path with a `filter` attribute (`path_denied:filter`), which is smaller than listing LFS storage writes in REQ-14; RR-1 closes by construction | Git Execution Policy item 2, Patch Policy item 3, REQ-14, S5, S6, research.md |
| F-010 format-patch | canonical flags, checked against hostile `format.*` settings; artifacts compared by the `patch_intent` hash, never by a fresh format-patch | Local Patch Flow step 11, Cleanup Rule 2, Recovery State Table, S4, S9 |

Rev 9 (2026-10-08) applies the CD-3 security design review (security-auditor; repro `cd3-probe/probe.sh`, git 2.50.1).
CD-3 stays open until the re-review passes:

| Item | Resolution | Where |
|------|------------|-------|
| H1 invisible and control characters | a byte check of the raw added lines runs before `Sanitize`: invalid UTF-8, bidi controls U+202A–U+202E and U+2066–U+2069, zero-width and bidi marks, ESC and other C0 controls except TAB, DEL, C1 controls, and every CR except the one right before a line's LF give `patch_content_denied:control_char`; a CRLF line ending passes (decided here) | Patch Policy item 7, REQ-08, S5 |
| H2 modeless hunks on non-regular entries | the base entry of every touched path and directory prefix comes from `git ls-tree -z <base-sha>`; anything but a 100644 or 100755 blob (no entry for an add) or a non-tree prefix gives `patch_invalid` | Patch Policy items 2–3, REQ-08, S5 |
| H3 symlinks out of the worktree | `-c core.symlinks=false` on every band git command checks tracked links out as text files; a reviewer's own git shows them as type changes, which the docs state | Git Execution Policy items 2 and 5, Provider Contract item 6, REQ-05, S7 |
| M1 lazy fetch | `GIT_NO_LAZY_FETCH=1` with git 2.44 or later, else `git_version_unsupported:<version>`; partial clones refused at step 1; S6 and S12 count network children by trace2 `child_start` events, not by an argv recorder | Git Execution Policy items 1 and 3, REQ-05, REQ-10, S6, S12 |
| M2 collisions | decoded paths and their directory prefixes, folded by NFC and case, collide neither with each other nor with tracked paths | Patch Policy item 4, REQ-08, S5 |
| M3 structural denies | dot segments with no exception, `*.conf.*`, `*rc.*`, `.*rc`, `build-logic/`, `buildSrc/`, `.yarn/`, `.pnp.*`, and modifications of 100755 paths give `path_denied` | Patch Policy item 6, REQ-08, S5 |
| M4 refused patch model | a `model_refusal_fallback` during the patch request gives `failed:patch_model_refused`, which replaces rev 8's never-fail rule for the patch request; a diagnosis keeps record-and-continue | REQ-03, Local Patch Flow step 6, Provider Contract item 7, S15 |
| M5 bounded checkout and retention | SIGTERM 5 s before a step group's deadline, then SIGKILL; a `git ls-tree -r -l` preflight against 200,000 entries, 2 GiB, and free disk; a per-repository cap of 5 kept keys gives `local_patch_skipped:cap_reached` | Step Timeouts and Lease, Local Patch Flow step 1, Decision Table row 4, Cleanup Rule 3, Outcome Lock, REQ-12, REQ-14, S3 |
| M6 ignored files | a worktree is removed only when `git status --porcelain --untracked-files=all --ignored` is empty, or lists only the claim's staged paths before its commit; otherwise `worktree_modified` | Cleanup Rule 3, REQ-11, S9 |
| M7 live claim against recovery | the key lock `<lp>/<key>.lock` is held for the claim's lifetime; recovery skips a key that it cannot lock at once (`recovery_key_locked`) | Local Patch Flow step 2, Data Contracts Recovery step, Recovery State Table, REQ-11, S9 |
| L1 guard fault | an `editguard.Decide` allow with a `Diagnostic` gives `path_denied:guard_fault` | Patch Policy item 9, REQ-08, S5 |
| L2 identity, hooks, attributes, file modes | `user.name=autopus-band`, `user.email=band@autopus.invalid`, `core.hooksPath=/dev/null`, `core.attributesFile=/dev/null`; `<lp>` 0700 and files 0600 through `O_CREAT`, `O_EXCL`, and `O_NOFOLLOW` | Git Execution Policy item 2, Local Patch Flow commit message, REQ-05, REQ-14, S4, S6 |
| L3 untrusted record paths | every path re-derived from `<lp>` and a key that matches `^[a-z0-9-]+$` and ends in the claim id's `<c8>`; a mismatch gives `failed:record_invalid` | Data Contracts Derived paths, Recovery State Table, REQ-11, S9 |
| L4 re-check and disclosure | item 3 runs again at step 8; `filter.lfs.process` is pinned to the resolved absolute path; the run output shows status, files, and models with the untrusted-CI-logs warning; the write-once BS, written before the patch request, carries the warning and points there | Git Execution Policy items 2–3, Local Patch Flow step 8, BS Record, S4, S6 |

Rev 8 (2026-10-08) folds in the executed Risk-First probes A1 and A3 (plan.md), which close Completion Debt CD-1 and CD-2:

| Item | Resolution | Where |
|------|------------|-------|
| A1 PASS | subscription sign-in without an API key (`apiKeySource` `none`); only `Glob`, `Grep`, `Read`, no MCP; an out-of-worktree `Read` denied by the permission layer; one diff fence | Local Patch Provider Contract item 6, plan.md A1 |
| A1 new risk: silent model fallback | stream-json with `--verbose`; the `init` model and every `model_refusal_fallback` event parsed; requested and actual model in `models[]` and the BS; `model_substituted` recorded, never a failure; 8 MiB stream bound, 1 MiB on the `result` text | REQ-03, REQ-07, Provider Contract items 3 and 7, Patch Policy item 1, Data Contracts, BS Record, S15 |
| A1 model rule | an OMP entry's `anthropic/claude-*` selector gives the subprocess `--model` (this repository: `claude-opus-5-5`); otherwise `claude-fable-5-1` | REQ-15, Provider Contract item 2, S13 |
| A1 run 1 gave no fence | the patch instructions frame the diff as a proposal that band applies outside the session | Patch Prompt Contract, S8 |
| A3 PASS | 0 markers under the policy vs 15 without it; temp-index tree equals the `apply --index` tree (crlf, ident, lfs, rename, mode); user index, HEAD, and stash unchanged; S6 fixture codes as listed | Git Execution Policy item 3, plan.md A3 |
| A3 deviations | the `info/attributes` refusal is mandatory; an interrupted add is classified by its `locked` file alone and removed only by a human with `--force --force`; global or system non-LFS filters are refused as an intended fail-closed limitation | Git Execution Policy item 3, Cleanup Rule 3, Recovery State Table, S6, S9 |
| residual risks | real git-lfs network behavior and a tracked `.lfsconfig`, and `GIT_ATTR_NOSYSTEM` against a real `/etc/gitattributes`, stay unverified; not blockers | research.md Completion Debt (RR-1, RR-2) |

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
