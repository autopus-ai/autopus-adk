# SPEC-SIGMABAND-002: σ-band 3σ local patch on an isolated worktree

**Status**: draft
**Created**: 2026-10-06
**Revised**: 2026-10-06 (rev 5: one recovery state table, a separate recovery step with its own lock, claim-unique diagnosis worktrees, chained-lease oracle; rev 2 rescoped the cap to local patch only)
**Domain**: SIGMABAND
**Module**: autopus-adk
**Sibling of**: SPEC-SIGMABAND-001 (consumes its tier-3 episodes, diagnosis, BS, write-ahead log, and claims)
**PRD**: `../SPEC-SIGMABAND-001/prd.md` (FR-14, FR-15, §5.2, user decision D3); deviations are listed in PRD Deviations

## 목적

사용자 결정 D3(3σ 대응 상한)은 2026-10-06 사용자 결정으로 "별도 브랜치 draft PR"에서 "로컬 patch만"으로 바뀌었다. 3σ(기본 OFF
플래그)에서는 저장소 밖 사용자 cache 디렉토리에 격리 worktree를 만들고, 로컬 branch를 만들고, read-only provider가 제안한 patch를
적용한 뒤 `.patch` 파일을 쓴다. 3σ BS에는 그 위치를 가리키는 pointer만 남는다. band는 push·fetch·PR·원격 쓰기를 하지 않는다. 사람이
검토한 뒤 직접 push한다. 신뢰 경계는 남아 있다. 에이전트가 작성한 코드가 사용자 저장소의 branch와 object에 놓이기 때문이다. 그래서
두 provider는 모두 band worktree 안에서만 읽을 수 있다. git은 checkout·apply·commit·format-patch 동안 repository가 지정한 어떤 명령도
실행하지 않는다. 저장소 root 아래 `.git/` 밖 파일은 BS와 band 기록 파일 외에는 바뀌지 않고 `.git/` 안에는 REQ-14가 허용한 변경만 생기며, test는
자동으로 돌리지 않는다.

## Outcome Boundary

- Outcome Lock: `health_band.allow_local_patch: true`이면, SPEC-SIGMABAND-001이 tier 3으로 연 episode 중 confined 진단이 성공한 것은 최대 한 번
  (모든 guard 통과 시 정확히 한 번) 로컬 산출물을 받는다. 산출물은 로컬 branch `autopus/band/<key>`, worktree `<lp>/<key>/worktree/`,
  patch 파일 `<lp>/<key>.patch`이다. 여기서 `<lp>` = `<UserCacheDir>/autopus/local-patches/<repo-hash>`, `<key>` =
  `<series-slug>-<h8>-<episode-id>-<c8>`이고 `<c8>`은 claim id의 앞 8 hex다. 3σ BS에는 pointer가 기록되고, 원격에는 아무것도 생기지 않는다.
- Mandatory requirements: REQ-01–REQ-14 (Priority Must).
- Explicit non-goals: push, fetch, PR 생성·갱신, 원격 ref, GitHub 쓰기 API, CI 실행, test·build 자동 실행, 에이전트 쓰기 권한, tier 2로 열린 뒤
  tier 3으로 오른 episode의 patch, SPEC-SIGMABAND-001이 소유한 동작. 리뷰어가 worktree를 IDE로 열거나 그 안에서 명령을 실행하거나
  branch를 push할 때의 실행도 범위 밖이며, BS의 reviewer warning이 이를 알린다.
- Completion evidence: acceptance S1–S12 통과, Completion Debt CD-1–CD-3 해소, security-auditor 리뷰 통과, 모든 run에서 원격 쓰기 0건, 저장소 root
  아래 `.git/` 밖 파일 변경은 BS와 `.autopus/metrics/` 기록 파일뿐.

## Requirements

Priority 열은 Must만 쓴다. 각 문장은 `pkg/spec` parser 문법을 따르며 실제 `ParseEARS`로 14개 모두 인식됨을 확인했다.

| ID | Priority | Source | EARS requirement |
|----|----------|--------|------------------|
| REQ-01 | Must | FR-17, F-052, decision 2026-10-06 | THE SYSTEM SHALL add `allow_local_patch` (default false) to SPEC-SIGMABAND-001's `HealthBandConf`, amend SPEC-SIGMABAND-001 REQ-15's key list at this SPEC's sync while `allow_draft_pr` stays rejected, omit the key from generated and saved `autopus.yaml` files while it is false, and document that a binary without this SPEC rejects a file that enables it. |
| REQ-02 | Must | FR-07, F-049, F-072 | WHEN SPEC-SIGMABAND-001 opens an episode at tier 3 IF `health_band.allow_local_patch` is true, THEN THE SYSTEM SHALL record a `local_patch` claim that depends on that episode's diagnose claim in `.autopus/metrics/localpatch-events.jsonl`, decide every other tier-3 position exactly as the Local Patch Decision Table defines, and write nothing into SPEC-SIGMABAND-001's files beyond the amendments listed in Related SPECs. |
| REQ-03 | Must | F-039, F-057 | WHERE `health_band.allow_local_patch` is true, THEN THE SYSTEM SHALL run every SPEC-SIGMABAND-001 diagnosis and every patch request with a band worktree at the base SHA as the only working directory and a claude provider projected with `--restricted`, `--strict-mcp-config`, and `--tools=Read,Grep,Glob`, and record every other provider as `unavailable(provider_unconfined)`. |
| REQ-04 | Must | F-049, F-050, F-073 | WHEN a `local_patch` claim executes, THEN THE SYSTEM SHALL end it with the step-1 code of its `prep` record when that code is not ok, otherwise with `failed:worktree_failed` when its `stage` records hold `worktree_failed`, otherwise with `failed:no_bs` when the diagnose result has no BS ID, otherwise with `failed:diagnosis_unavailable` when `diagnosis_status` is not ok, and only otherwise start the patch stage, so that no failed preparation or diagnosis leads to a patch request, apply, or commit. |
| REQ-05 | Must | F-043, F-071, F-075 | WHEN band runs git for a local patch, THEN THE SYSTEM SHALL apply the Git Execution Policy to every git command (scrubbed environment with `GIT_ATTR_NOSYSTEM=1` and `GIT_LFS_SKIP_SMUDGE=1`, hooks off, `core.fsmonitor=false`, an empty `core.attributesFile`, refusal of a non-empty `info/attributes`, of every configured filter, diff, or merge driver outside the git-lfs allowlist, and of every `lfs.extension` or `lfs.customtransfer` setting, and no network command) so that checkout, apply, commit, and format-patch run no command that the repository or its configuration selects beyond the allowlisted git-lfs binary, and THE SYSTEM SHALL never run tests, builds, or the proposed change. |
| REQ-06 | Must | F-016, F-055, F-056 | WHEN band builds the commit message, THEN THE SYSTEM SHALL build and validate it in-process with `lore.BuildCommit` and `lore.Validate` before any `git apply`, refuse with `lore_unsupported_required:<trailer>` when a required trailer is not one of the five that `lore.Validate` recognizes, and confirm after the commit that the commit object's message equals the `-F` file bytes. |
| REQ-07 | Must | F-039, F-047, F-048 | WHEN band requests a patch, THEN THE SYSTEM SHALL build the prompt as the Patch Prompt Contract defines, take the diff from the raw reply without redaction or line removal, and keep the reply in memory until the Patch Policy has accepted it. |
| REQ-08 | Must | F-040, F-058, decision 2026-10-06 | WHEN a proposed diff is evaluated, THEN THE SYSTEM SHALL decode every path as the Patch Policy defines and accept the diff only when the Patch Policy accepts every decoded path (including the dotenv and credential entries), mode, and added line and the change stays within 10 files, 400 changed lines, and 64 KiB, before any command writes to the object database. |
| REQ-09 | Must | decision 2026-10-06, F-070 | WHEN the Patch Policy accepts a diff, THEN THE SYSTEM SHALL apply it with `git apply --index` in the claim's worktree under `<lp>/<key>/worktree/`, confirm that the staged path set equals the parsed set, commit it, create the local branch `autopus/band/<key>` at that commit, write `git format-patch` output to `<lp>/<key>.patch`, and keep the worktree for human review. |
| REQ-10 | Must | decision 2026-10-06 | THE SYSTEM SHALL never push, fetch, create or update a pull request, call a GitHub write API, or create a remote ref in the local patch flow, so that every artifact stays local until a human pushes it. |
| REQ-11 | Must | F-060, F-064, F-068, F-076, F-080 | IF any guard or step of the local patch flow or of a confined diagnosis fails, or a later run finds a claim interrupted, THEN THE SYSTEM SHALL handle exactly the artifacts that the claim's intent records name, under the one Cleanup Rules set and the Recovery State Table of the Data Contracts, end the claim in the state that table names, write the `result` record, keep the SPEC-SIGMABAND-001 BS, never touch an artifact that no intent record of the claim names or that a user changed, and exit 0. |
| REQ-12 | Must | F-032, F-072 | THE SYSTEM SHALL give a diagnose claim the 990 s budget and a `local_patch` claim the 810 s budget of the Step Timeouts table while `allow_local_patch` is true, execute each `local_patch` claim right after its diagnose claim, and count both budgets in SPEC-SIGMABAND-001's chained lease rule for every later claim of the run through the lease-budget edits that plan task T8 owns in SPEC-SIGMABAND-001's `pkg/healthband/episode.go` and `catchup.go`. |
| REQ-13 | Must | FR-23, decision 2026-10-06 | THE SYSTEM SHALL document the flag, the artifact locations, the reviewer warning, and the upgrade-before-enable note in the `auto react band` help text, `docs/health-band.md`, and `CHANGELOG.md`, and put the Local Patch pointer lines with the reviewer warning in every 3σ BS of a `local_patch` claim. |
| REQ-14 | Must | F-070, F-077 | THE SYSTEM SHALL keep every worktree and patch file of this SPEC under `<UserCacheDir>/autopus/local-patches/<repo-hash>/`, outside the repository, refuse with `cache_unavailable` when that directory cannot be created, change no file under the repository root outside `.git/` except the BS file and the files under `.autopus/metrics/` that SPEC-SIGMABAND-001 and this SPEC own, and change inside `.git/` only `refs/heads/autopus/band/<key>` with its reflog, the `worktrees/<name>/` entry of the claim's worktree, and new objects. |

`<repo-hash>` = first 12 hex digits of the SHA-256 of `git rev-parse --path-format=absolute --git-common-dir`, so every worktree
of one repository shares one directory. `<key>` = `<series-slug>-<h8>-<episode-id>-<c8>`, where `<c8>` is the first 8 hex digits of
the random claim id (the `local_patch` claim id, or the diagnose claim id for a diagnosis without one), so two checkouts of one
repository, two series, or a run after the store was deleted never share a `<key>`.

## Local Patch Decision Table

SPEC-SIGMABAND-001's phase A calls this table for each evaluated tier-3 position while the flag is true. Rows are checked top
down and the first match wins; row 6 is the default. Decisions go to `.autopus/metrics/localpatch-events.jsonl` (kind `decision`).

| Order | Condition | Decision | Reason |
|-------|-----------|----------|--------|
| 1 | the run has `--no-agent` | skipped | `local_patch_skipped:no_agent` |
| 2 | the episode is not the newest touched in this run (001 batch rule) | skipped | `local_patch_skipped:superseded_in_batch` |
| 3 | the episode already has a `local_patch` claim | skipped | `local_patch_skipped:episode_already_patched` |
| 4 | this position opens the episode at tier 3 (001 action `diagnose`) | claim, `depends_on` = the diagnose claim | - |
| 5 | the episode opened at tier 2 and now reaches tier 3 | skipped | `local_patch_skipped:bs_not_tier3` |
| 6 | any other tier-3 position, such as a later position of an episode whose tier-3 opening got no claim (rows 1–2, or the flag was off) | skipped | `local_patch_skipped:no_opening_claim` |

Execution order in phase B: the diagnose claim runs first and SPEC-SIGMABAND-001 records its result right after it, including the
BS ID; the `local_patch` claim runs next and reads that result (REQ-04). Both run under one owner, so the BS ID is bound before
the patch stage.

## Local Patch Flow

Every `prep` and `stage` record is appended under SPEC-SIGMABAND-001's store lock with a wait of at most 60 s inside the claim's
margin; a failed append ends the claim `failed:record_unavailable` before the next artifact step, so no artifact exists without
its intent record. A diagnosis without a `local_patch` claim runs steps 1–2 with its own `<key>` and intent records.

| Order | Step | Code on failure |
|-------|------|-----------------|
| 1 | Inside the diagnose claim: `<lp>` available, no artifact at `<lp>/<key>/`, `<lp>/<key>.patch`, `<lp>/<key>.diff`, or `refs/heads/autopus/band/<key>`, unsafe configuration check (Git Execution Policy item 3), base SHA (item 4); then one `prep` record with the base SHA and the code (ok or the failure); a failure makes the diagnosis report `unavailable(worktree_unavailable)` and 001 writes the evidence-only BS | `cache_unavailable`, `artifact_exists`, `git_config_unsafe:<key>`, `base_unavailable` |
| 2 | Inside the diagnose claim: `stage` `worktree_intent` (path), `git worktree add --detach <worktree> <base-sha>`, then `stage` `worktree_done` or `worktree_failed`; then the confined diagnosis with cwd `<worktree>` (REQ-03) and the BS with the pointer lines; a diagnosis without a `local_patch` claim then applies the Cleanup Rules to its worktree | `worktree_failed`, 001 REQ-12 reasons |
| 3 | `local_patch` claim: result check in the order of REQ-04 | the `prep` code, `worktree_failed`, `no_bs`, `diagnosis_unavailable` |
| 4 | `refs/heads/autopus/band/<key>` still absent; it can only appear here if something created it after step 1 | `branch_exists` |
| 5 | Lore message built and validated (REQ-06); `stage` `message` with its SHA-256 | `lore_unsupported_required:<trailer>`, `lore_rejected` |
| 6 | Patch request (REQ-07); the reply stays in memory | `patch_provider_unconfined`, 001 REQ-12 reasons |
| 7 | Patch Policy over the raw reply (REQ-08), ending with `git apply --numstat --summary -z --check`; no command writes objects before this step passes | `no_patch`, `patch_invalid`, `path_denied`, `patch_content_denied`, `patch_too_large` |
| 8 | Accepted diff written to `<lp>/<key>.diff`, `stage` `apply_intent` (its SHA-256); `git apply --index`; staged path set equal to the parsed set; `stage` `apply_done` with the index tree from `git write-tree` | `patch_invalid` |
| 9 | `git commit --no-verify --cleanup=verbatim -F <msg>`; `stage` `commit_done` with `git rev-parse HEAD`; the commit object's message (after the header's blank line in `git cat-file commit HEAD`) equals the `-F` file bytes | `commit_failed`, `commit_message_altered` |
| 10 | `stage` `branch_intent` with the commit OID; `git update-ref refs/heads/autopus/band/<key> <commit-oid> ""`, which creates the ref only while it is absent; `stage` `branch_done` | `branch_failed` |
| 11 | `stage` `patch_intent` (path); `git format-patch --no-textconv --no-ext-diff --stdout <base-sha>..HEAD` to `<lp>/<key>.patch.tmp-<c8>`; rename to `<lp>/<key>.patch`; `stage` `patch_done` | `patch_file_failed` |
| 12 | Done: keep the worktree, branch, and patch file; delete `<lp>/<key>.diff`. Any failure applies the Cleanup Rules to this claim's intent records; `git worktree prune` is never run | - |

- Claim commit: the commit whose parent is the `prep` base SHA and whose message hashes to the `message` stage; recovery finds it
  this way even when a crash came before `commit_done`.
- Objects: steps 1–7 write no object. A failure at steps 8–11 leaves unreachable objects in the shared object database until the
  user's own `git gc`; no ref points to them.
- Commit message via `lore.BuildCommit`: subject `fix(band): <series-slug> anomaly local patch (<episode-id>)`, or
  `fix(band): <series-slug> 이상 대응 로컬 패치 (<episode-id>)` when the commit-message language is `ko`; body with tier, z, and
  BS ID; the `pkg/lore` sign-off; the `-F` file is the `BuildCommit` output plus one newline. Required trailers outside
  Constraint, Rejected, Confidence, Directive, Tested stop step 5 (`pkg/lore/query.go:87` `hasField` recognizes only these five).
  Values: Constraint `generated by auto react band; local patch needs human review`, Rejected `write-capable agent; read-only
  proposal only`, Confidence `low`, Directive `review the patch file before running anything`, Tested `none; nothing was run`;
  `Related: <BS-ID>` is always added.

## BS Record

With a `local_patch` claim, SPEC-SIGMABAND-001 renders these pointer lines at the end of the BS `## 추천 방향` section when it
writes the BS, so the BS format and its validator stay unchanged and the BS stays write-once:

```text
Local patch (3σ, local only, if produced): branch autopus/band/<key>, worktree <lp>/<key>/worktree/, patch file <lp>/<key>.patch, outside this repository.
The outcome is recorded in .autopus/metrics/localpatch-events.jsonl under claim <claim-id>; nothing was pushed.
Reviewer warning: this local branch holds an agent-proposed patch that nothing has run. Read the whole patch file before you open the worktree in an IDE, run any command in it, or push the branch, because repository hooks and tool configuration files run on checkout, commit, and build.
```

`<lp>` is written as an absolute path. The BS lives in `.autopus/brainstorms/` (gitignored, always blocked from staging) and the
records in `.autopus/metrics/` (local-only by SPEC-SIGMABAND-001 REQ-16). The branch is a local ref; `git push --all` would
publish it, which the docs state.

## Git Execution Policy

1. Environment: every git subprocess starts with every `GIT_*` variable removed and gets `GIT_TERMINAL_PROMPT=0`, `GIT_EDITOR=:`,
   `GIT_PAGER=cat`, `GIT_ATTR_NOSYSTEM=1` (no system attributes file), and `GIT_LFS_SKIP_SMUDGE=1` (git-lfs downloads nothing).
   No fetch, push, or other network command runs, so no transport or credential configuration is used.
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
4. Base SHA: `git rev-parse --verify refs/remotes/origin/<default>^{commit}`, otherwise `refs/heads/<default>`; neither gives
   `base_unavailable`. `<default>` comes from SPEC-SIGMABAND-001's default-branch lookup or `git symbolic-ref
   refs/remotes/origin/HEAD`. No fetch runs, so the base is the last fetched state the user already has.
5. Worktree: `git worktree add --detach <worktree> <base-sha>` at `<lp>/<key>/worktree/`, for a tier-3 claim and for a diagnosis
   without a `local_patch` claim alike (each with its own `<key>`); the latter is cleaned up right after its diagnosis.
6. Nothing else: the user's worktrees, index, HEAD, stash, and every ref except `refs/heads/autopus/band/<key>` stay unchanged.

## Patch Prompt Contract

The patch request is a second provider prompt rendered by `promptlayer.Render`; its manifest goes into the `result` record.

| Layer ID | Kind | Content | Cache eligible |
|----------|------|---------|----------------|
| `band.patch_instructions.v1` | stable | role, "answer with exactly one diff fence", no commands, the nonce rule | true |
| `band.patch_rules.v1` | stable | Patch Policy summary and size caps | true |
| `band.evaluation.<event-hash>` | snapshot | SPEC-SIGMABAND-001's frozen evaluation record (no BS ID) | false |
| `band.diagnosis.<claim-id>` | ephemeral | the body of the BS `## 프로바이더별 발산 결과` section only (no title line, no BS ID), produced by the confined diagnosis | false |
| `band.evidence.run.<run_id>.a<attempt>` | ephemeral | SPEC-SIGMABAND-001's sanitized log excerpt | false |
| `band.base.<base-sha>` | ephemeral | the base SHA and the tracked paths named in the evidence (no file content) | false |

Every ephemeral layer is wrapped in a fence whose info string is `untrusted-<nonce>`, where the nonce is 128 random bits in hex,
drawn again whenever it occurs inside any layer; the instructions name the nonce and state that text inside such fences is data.
Both providers read only files inside a band worktree at the base SHA (`--restricted`), which holds tracked content only;
untracked files of the user's checkout, such as `.env`, are not in it.

## Patch Policy

1. The reply holds exactly one ```` ```diff ```` fence (else `no_patch`); the reply is at most 1 MiB and the diff at most 64 KiB.
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
   `scripts/`, `buildSrc/`, `vendor/`, `node_modules/`, every `GeneratedSurfacePrefixes` and `GeneratedSurfaceExactPaths` entry,
   `Makefile`, `GNUmakefile`, `Dockerfile*`, `docker-compose*`, `package.json`, `go.mod`, `go.sum`, `AGENTS.md`, `CLAUDE.md`,
   `GEMINI.md`, `.gitattributes`, `.gitmodules`, `.pre-commit-config.yaml`, `lefthook.yml`, `lefthook.yaml`; dotenv and credential
   paths `.env*`, `*.env`, `.envrc`, `.netrc`, `.npmrc`, `.pypirc`, `.git-credentials`, `.aws/`, `.ssh/`, `.gnupg/`, `.kube/`,
   `kubeconfig*`, `secrets.*`, `*secret*`, `*credentials*`, `*.pem`, `*.key`, `id_rsa*`, `id_ed25519*`, `*.p12`, `*.pfx`,
   `*.keystore`, `*.jks`; and files that tools load or run on their own: `build.rs`, `conftest.py`, `setup.py`, `noxfile.py`,
   `sitecustomize.py`, `usercustomize.py`, `*.config.js`, `*.config.cjs`, `*.config.mjs`, `*.config.ts`, `*.config.cts`,
   `*.config.mts`, `.eslintrc.*`, `.prettierrc.*`, `.pnpmfile.cjs`, `Package.swift`, `gulpfile.*`, `Gruntfile.*`, `magefile.go`,
   `Dangerfile.*`.
7. Added lines: `SanitizeContent` over the raw added lines reports neither `secret_risk` nor `injection_risk`, and no added line
   holds a run of 40 or more base64 or hex characters (`patch_content_denied`).
8. Size: at most 10 files and 400 changed lines (added + removed) (`patch_too_large`).

## Data Contracts

- `.autopus/metrics/localpatch-events.jsonl` (`autopus.band_localpatch.v1`, written under SPEC-SIGMABAND-001's store lock and
  compacted with the same rules, never dropping a record of a non-terminal claim) holds five record kinds:
  - `decision`: `seq`, `series`, `episode_id`, `evaluation_seq`, `decision` (`claim` or `skipped`), `reason`.
  - `claim`: `seq`, `claim_id`, `owner`, `lease_until`, `series`, `episode_id`, `depends_on` (the diagnose claim id), `key`,
    `worktree_path`, `patch_path`, `branch`; written in phase A.
  - `prep`: `seq`, `series`, `episode_id`, `claim_id` (when a `local_patch` claim exists), `base_sha` (once resolved), `code`
    (`ok` or a step-1 code); exactly one per diagnosis, written after step 1 and before any artifact exists.
  - `stage`: `seq`, `claim_id`, `phase` (`worktree_intent`, `worktree_done`, `worktree_failed`, `message`, `branch_intent`,
    `branch_done`, `patch_intent`, `patch_done`), and per phase `path`, `message_sha256`, or `intended_oid`. Every `*_intent` is
    written before the command that creates the artifact, so every artifact that may exist is named by a durable record first.
  - `result`: `seq`, `claim_id`, `status` (`done` or `failed:<code>`), `bs_id`, `base_sha`, `commit_sha`, `branch`,
    `worktree_path`, `patch_path`, `prompt_manifest`, `recovered`, `kept[]` (artifacts kept by a cleanup rule, with reason).
- `.autopus/metrics/localpatch-state.json` (`autopus.band_localpatch_state.v1`): claims per series, written after the events and
  replayed like SPEC-SIGMABAND-001's checkpoint. SPEC-SIGMABAND-001 binaries never read these two files.
- Cleanup Rules (one set for the live run and for recovery; only artifacts named by the claim's intent records are considered;
  every git call has a 30 s timeout):
  1. Branch: `git update-ref -d refs/heads/autopus/band/<key> <claim-commit>`, which deletes only while the branch points at the
     claim commit; a branch at another OID is kept with reason `branch_moved`.
  2. Patch and diff files: `<lp>/<key>.patch` or its `.tmp-<c8>` is deleted only when its first line is `From <claim-commit>`;
     `<lp>/<key>.diff` is deleted only when its SHA-256 equals the `apply_intent` hash; anything else is kept with reason
     `patch_modified`.
  3. Worktree: only the admin entry whose path equals the `worktree_intent` path is considered. It is removed with
     `git worktree remove --force <path>` (`--force --force` when the admin entry holds the `locked` file that an interrupted
     `git worktree add` leaves) when its HEAD is the base SHA or the claim commit, it has no unstaged or untracked change
     (`git status --porcelain --untracked-files=all`), and its index tree is the HEAD tree, the `apply_done` tree, or the tree of
     the base plus `<lp>/<key>.diff` computed with a temporary index. Otherwise it is kept with reason `worktree_modified`, or
     `head_unrecognized` when HEAD is neither commit.
- Recovery step: runs at the start of every band run, after SPEC-SIGMABAND-001's network step and before its phase A, under its
  own lock `.autopus/metrics/.recovery.lock` (`pkg/filelock`, wait at most 5 s, otherwise reason `recovery_locked` and no
  recovery in that run). It reads this SPEC's records under the store lock (local file IO only), then, without the store lock,
  handles every claim whose `lease_until` has passed without a `result` (never retried) by the Recovery State Table, with a
  budget of 120 s per claim, and appends each `result` under the store lock. Phase A only reads these results.
- Recovery State Table (last durable record of the claim → what recovery finds → action → claim state):

| Last durable record | Found | Action | State |
|---------------------|-------|--------|-------|
| `decision`, `claim`, or `prep` | no artifact | none | `failed:interrupted` |
| `worktree_intent` | worktree at the path, possibly locked, or none | Cleanup Rule 3 | `failed:interrupted` |
| `worktree_done` or `message` (diagnosis or patch request in flight) | clean worktree at the base SHA | Cleanup Rule 3 | `failed:interrupted` |
| `apply_intent` | worktree at the base SHA with an index of the base tree or of the base plus the diff | Cleanup Rules 2–3 | `failed:interrupted` |
| `apply_done` or `commit_done` | HEAD at the base SHA or at the claim commit | Cleanup Rules 2–3 | `failed:interrupted` |
| `branch_intent` or `branch_done` | branch absent or at the claim commit | Cleanup Rules 1–3 | `failed:interrupted` |
| `patch_intent` or `patch_done` | `<lp>/<key>.patch` with `From <claim-commit>`, branch and worktree HEAD at the claim commit, clean worktree | delete the diff file, keep the rest | `done`, `recovered: true` |
| `patch_intent` | anything short of that complete set | Cleanup Rules 1–3 | `failed:interrupted` |
| any | an artifact a user changed | keep it, list it in `kept[]` with its reason | as the row above |

## Step Timeouts and Lease

While the flag is true, a diagnose claim's budget is SPEC-SIGMABAND-001's 930 s plus 60 s for the worktree = 990 s. A
`local_patch` claim's budget = 600 (patch request) + 60 (apply and commit) + 30 (branch and format-patch) + 60 (cleanup) + 60
(margin) = 810 s. Each `local_patch` claim runs right after its diagnose claim, and SPEC-SIGMABAND-001's chained lease rule
counts both budgets for every later claim of the run, so a diagnose and its local_patch first in a run get 990 s and 1,800 s.

## PRD Deviations

| PRD text | This SPEC | Reason |
|----------|-----------|--------|
| FR-14 draft PR, push, `gh pr create --draft` | local branch, worktree, and `.patch` file only; no push, no PR | user decision 2026-10-06 (3σ cap = local patch only) |
| FR-14 "commit through the repository's normal hooks" | hooks off, in-process `lore.Validate` | hooks and commit-msg tooling would run repository code (F-002) |
| FR-14 branch name without hash | `autopus/band/<series-slug>-<h8>-<episode-id>` (local) | slug prefixes collide (F-017) |
| FR-15 "create no branch, commit, push, or PR" on a guard failure | a failure removes the branch, worktree, and patch file it created; nothing remote ever exists | local cleanup replaces remote rollback |
| §5.2 `git apply --check` | `git apply --numstat --summary -z --check` plus staged-set equality after `git apply --index` | the commit tree must equal the guarded patch (F-011) |

## 생성 파일 상세

Labels: `[NEW]` = added by this SPEC; `[PLANNED by SIGMABAND-001]` = planned by the sibling and changed here after it exists.

| Path | Role |
|------|------|
| [NEW] `pkg/healthband/localpatch_decision.go`, `localpatch_wal.go`, `localpatch_recovery.go` | Local Patch Decision Table, own write-ahead log and checkpoint, Recovery rules |
| [NEW] `pkg/healthband/patchpolicy.go`, `patchprompt.go`, `commitmsg.go`, `gitpolicy.go` | Patch Policy, Patch Prompt Contract, Lore message, Git Execution Policy |
| [NEW] `internal/cli/react_band_localpatch.go` | executor of the Local Patch Flow |
| [PLANNED by SIGMABAND-001] `pkg/config/schema_health_band.go` | gains `AllowLocalPatch bool` with `omitempty` |
| [PLANNED by SIGMABAND-001] `pkg/healthband/episode.go`, `catchup.go` | per-kind lease-budget table (diagnose 990 s, local_patch 810 s while the flag is true), owned by plan task T8 |
| [PLANNED by SIGMABAND-001] `internal/cli/react_band.go`, `react_band_diagnose.go`, `react_band_help.go` | recovery-step call before phase A, phase hooks, confined diagnosis cwd and projection, help text |
| [PLANNED by SIGMABAND-001] `pkg/brainstorm/render.go` | pointer lines in `## 추천 방향` |
| [PLANNED by SIGMABAND-001] `docs/health-band.md`; existing `CHANGELOG.md` | flag, artifact locations, reviewer warning |
| existing `internal/cli/orchestra_readonly_policy.go` | gains a confined projection option (claude only: `--restricted`, `--strict-mcp-config`, `--tools=Read,Grep,Glob`) after SPEC-REVIEWRO-001 |

## Related SPECs

- SPEC-SIGMABAND-001: must be implemented and merged first; plan task T8 then owns this SPEC's edits to its files, including the
  lease-budget table in `pkg/healthband/episode.go` and `catchup.go`. While the flag is true, this SPEC amends: the diagnosis cwd and projection
  (REQ-03); the diagnosis status values `unavailable(provider_unconfined)` and `unavailable(worktree_unavailable)`; the diagnose
  budget (990 s) and the lease chain, which also counts `local_patch` budgets, so claim `owner`/`lease_until` values differ
  from a flag-off run (REQ-12); the BS `## 추천 방향` pointer lines (BS Record); and REQ-22, whose redaction does not apply to
  the raw patch reply, which never reaches a prompt, BS, or terminal (REQ-07); and the run order, which gains the recovery step between
  001's network step and phase A. It never changes 001's action enum, decision
  table, evaluation fields other than claim leases, or BS format. At sync it amends 001 REQ-15's key list (REQ-01).
- SPEC-REVIEWRO-001: owns the shared claude `--tools=Read,Grep,Glob` projection; this SPEC adds the confined option on top of it.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-01 | T1 | S1, S10 | INV-01 |
| REQ-02 | T2, T8 | S2, S3 | INV-02 |
| REQ-03 | T6, T8 | S7 | INV-06 |
| REQ-04 | T7 | S3, S6 | INV-02 |
| REQ-05 | T4, T7 | S4, S6 | INV-03 |
| REQ-06 | T7 | S5 | INV-08 |
| REQ-07 | T5 | S8 | INV-06 |
| REQ-08 | T3 | S5 | INV-05 |
| REQ-09 | T7 | S4 | INV-07 |
| REQ-10 | T7, T10 | S12 | INV-04 |
| REQ-11 | T7 | S5, S9 | INV-09 |
| REQ-12 | T2, T8 | S2, S9 | INV-02 |
| REQ-13 | T9 | S4, S11 | INV-01, INV-07 |
| REQ-14 | T7 | S4 | INV-07 |

## Review Resolution

Rev 2 applied the user decision of 2026-10-06 (3σ cap = local patch only); rev 3 resolves the rev 2 review.

| Finding | Resolution | Where |
|---------|------------|-------|
| F-068 (rev 5) | resolved: one Recovery State Table; recovery is its own step with its own lock, 30 s git timeouts, and a 120 s budget, outside 001's phase A; prep and stage appends take the store lock | Data Contracts, Local Patch Flow, S9 |
| F-076 (rev 5) | resolved: diagnoses without a claim get a claim-unique `<key>`, step-1 existence checks, and intent records | Git Execution Policy item 5, Local Patch Flow steps 1–2, S7 |
| F-079 | resolved: the complete row checks the files, not `patch_done`; a pre-existing branch yields only `artifact_exists` | Recovery State Table, S5, S9 |
| F-080 | resolved: `apply_intent`/`apply_done`/`commit_done` records and the claim-commit rule cover HEAD on a new commit and staged-only states | Local Patch Flow steps 8–9, Cleanup Rule 3, S9 |
| F-072 (rev 5) | resolved: the lease table lives in 001's `episode.go`/`catchup.go` (T8); S9 checks a five-claim chain and S2 checks lease values | 생성 파일 상세, S2, S9 |
| F-068 (rev 4) | superseded by the rev 5 row | - |
| F-072 (rev 4) | resolved: plan task T8 owns the lease-budget edits in 001's `episode.go` and `catchup.go`; 001 lands first | REQ-12, plan.md T8 |
| F-073 (rev 4) | resolved: one `prep` record after step 1; `worktree_failed` is a `stage` record after the command; REQ-04 order covers both | REQ-04, Local Patch Flow steps 1–3, S3 |
| F-076 | resolved: claim-unique `<key>` with `<c8>`; an existing artifact at the key refuses with `artifact_exists` and is never touched | Local Patch Flow step 1, REQ-11, S5 |
| F-077 | resolved: REQ-14 separates files outside `.git/` from the allowed `.git/` changes | REQ-14, S4 |
| F-078 | resolved: the git-lfs allowlist is a byte-exact expression without shell metacharacters | Git Execution Policy item 3, S6 |
| F-017, F-036, F-037, F-041, F-042, F-044, F-053, F-059, F-062, F-068 (remote part), F-069 | resolved by rescope (user decision 2026-10-06): no push, PR, remote ref, or CI run exists | REQ-10, S12 |
| F-045 | resolved by rescope: no fetch runs; the base is the last fetched remote-tracking ref | Git Execution Policy item 4, S6 |
| F-039 | resolved: both providers confined to a worktree of tracked content; dotenv and credential paths denied; records local | REQ-03, Patch Policy item 6, S7 |
| F-043 | resolved (rev 3): global and system attribute files disabled, `info/attributes` must be empty, every configured filter, diff, or merge driver outside the exact git-lfs allowlist refused | REQ-05, Git Execution Policy items 1–3, S6 |
| F-068 (local part) | resolved (rev 3): base SHA, artifact paths, message hash, and created OID are durable before their artifacts; recovery completes or removes with expected-OID branch deletion | Data Contracts, REQ-11, S9 |
| F-070 | resolved (rev 3): worktree and patch file live under the user cache directory, outside the repository; only the BS and records change inside it | REQ-14, Git Execution Policy item 5, S4 |
| F-071 | resolved (rev 3): `GIT_LFS_SKIP_SMUDGE=1`, `lfs.extension` and `lfs.customtransfer` refused, so git-lfs neither downloads nor runs extra programs | REQ-05, Git Execution Policy items 1 and 3, S6 |
| F-072 | resolved (rev 3): REQ-02 and Related SPECs state the lease amendment; S2 compares the fields that must match | REQ-02, REQ-12, S2 |
| F-073 | resolved (rev 3): preparation failures write a `prep` record whose code the claim takes first; new diagnosis status values listed as amendments | REQ-04, Local Patch Flow steps 1–3, S3, S6 |
| F-074 | resolved (rev 3): default row 6 | Decision Table, S3 |
| F-075 | resolved (rev 3): the allowlist is a token rule that accepts the bare word or an absolute path with base name `git-lfs` | Git Execution Policy item 3, S6 |
| F-058 (regressed) | resolved (rev 3): C-quoted paths decoded before every check and cross-checked with `git apply --numstat -z --check`, which writes no object | Patch Policy item 2, S5 |
| F-049, F-050, F-067 | resolved: claim depends on the diagnose claim, whose result with the BS ID comes first; first-match order | Decision Table, REQ-04, S3 |
| F-056, F-060 | resolved: commit object message vs `-F` bytes; steps 1–7 write no object | Local Patch Flow, S5 |
| F-064 | resolved: Self-Verify Summary re-judged after rev 3 | research.md |
| F-040, F-047, F-048, F-055 | carried: tool-loaded files denied, raw diff, fenced prompt layers, five-trailer rule | Patch Policy, Patch Prompt Contract, REQ-06 |
