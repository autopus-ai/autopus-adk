# SPEC-SIGMABAND-002: σ-band 3σ draft PR on an isolated branch

**Status**: draft
**Created**: 2026-10-06
**Domain**: SIGMABAND
**Module**: autopus-adk
**Sibling of**: SPEC-SIGMABAND-001 (consumes its tier-3 episodes, BS IDs, write-ahead log, and claims)
**PRD**: `../SPEC-SIGMABAND-001/prd.md` (FR-14, FR-15, §5.2, user decision D3)

## 목적

사용자 결정 D3(3σ는 별도 브랜치 draft PR, 기본 OFF 플래그)는 유지된다. 2026-10-06 사용자 결정으로 이 경로는 SPEC-SIGMABAND-001에서
분리되었다. 분리 사유는 보안 경계다. 이 경로에서는 신뢰할 수 없는 CI 로그의 영향을 받은 에이전트 제안 patch가 사람의 리뷰 전에
원격 브랜치에 도달한다. 이 SPEC은 001 rev 2의 설계(read-only patch 제안, 격리 worktree, hook 비활성화, 프로세스 안 Lore 검증,
CI skip 범위 검사, tag·submodule push 차단)를 이어받는다. 아직 닫히지 않은 항목은 Completion Debt로 남아 승인과 sync를 막는다.

## Outcome Boundary

- Outcome Lock: `health_band.allow_draft_pr: true`이면, SPEC-SIGMABAND-001의 tier-3 episode 중 진단 BS가 있는 것은 episode마다 한 번
  `autopus/band/<series-slug>-<h8>-<episode-id>` draft PR을 받는다. PR은 read-only patch 제안을 격리 worktree에 적용해 만든다. 사람이
  리뷰하기 전에는 repository 코드도 hook도 실행되지 않는다. guard나 CI 검사가 안전을 증명하지 못하면 거부하고 001의 진단 결과만 남긴다.
- Mandatory requirements: REQ-01–REQ-10 (Priority Must).
- Explicit non-goals: merge, approve, auto-merge, 기본 브랜치 push, tag push, 에이전트 쓰기 권한, 제안의 build·test·실행,
  SPEC-SIGMABAND-001이 이미 소유한 모든 동작(수집, 탐지, 진단, BS).
- Completion evidence: acceptance S1–S11 통과, Completion Debt CD-1–CD-4 해소, security-auditor 리뷰 통과, flag OFF에서 git/gh 변경 0건.

## Requirements

Priority 열은 Must만 쓴다. 각 문장은 `pkg/spec` parser 문법을 따르며 실제 `ParseEARS`로 10개 모두 인식됨을 확인했다.

| ID | Priority | Source | EARS requirement |
|----|----------|--------|------------------|
| REQ-01 | Must | FR-17, D3 | THE SYSTEM SHALL add `allow_draft_pr` (default false) to the `health_band` namespace of SPEC-SIGMABAND-001, strictly decoded and omitted from generated and saved `autopus.yaml` files while false, and document that binaries without this SPEC reject a file that enables it. |
| REQ-02 | Must | FR-07, FR-08, F-033, F-034 | WHEN SPEC-SIGMABAND-001 evaluates a tier-3 position of an open episode IF `health_band.allow_draft_pr` is true, THEN THE SYSTEM SHALL attach one `draft_pr` claim to that evaluation event only while the episode holds no `draft_pr` claim and its diagnose claim is `done` with a BS ID or belongs to the same run, keep the 001 action value unchanged, and otherwise record `draft_pr_deferred:diagnose_in_flight`, `draft_pr_skipped:episode_without_bs`, or `draft_pr_skipped:episode_already_drafted`. |
| REQ-03 | Must | FR-14, F-020 | WHEN a draft_pr claim executes, THEN THE SYSTEM SHALL require the episode's BS file, then execute the Draft PR Guard Contract steps in order: enumerate open band PRs, create the detached worktree from the remote default branch, pass the CI Skip-Coverage Scan, request one fenced unified diff from the read-only provider running in that worktree, apply a diff that passed every pre-mutation guard with `git apply --index`, commit, push the single band ref, open the PR with `gh pr create --draft`, and remove the worktree. |
| REQ-04 | Must | FR-15, F-021, F-028 | IF a pre-mutation guard fails, THEN THE SYSTEM SHALL run no commit, push, or PR step; IF a mutation step fails, THEN THE SYSTEM SHALL run no later step; in both cases THE SYSTEM SHALL record `draft_pr_guard:<code>`, keep the 001 BS outcome, force-remove the band-owned worktree, and exit 0. |
| REQ-05 | Must | F-002, F-016, F-024, F-025 | WHEN band runs git or gh in the draft PR flow, THEN THE SYSTEM SHALL apply the Git Execution Policy to every git command, pass `--no-verify` on commit and push, validate the Lore message in-process with `lore.Validate` and the project Lore configuration before committing, put `[skip ci]` in the message, push with `-c push.followTags=false --no-follow-tags --recurse-submodules=no` and the single band refspec, invoke gh exactly as the gh Invocation Table defines, never build, test, or run the proposed change, and state in the PR body that checks stay skipped until a human pushes a commit without the skip instruction. |
| REQ-06 | Must | F-002, F-011 | WHEN a proposed diff is evaluated, THEN THE SYSTEM SHALL accept it only when every path passes the allowlist and deny list of the Draft PR Guard Contract, no added line carries a secret or injection marker, the change stays within 10 files and 400 changed lines, and the staged path set after `git apply --index` equals the parsed path set. |
| REQ-07 | Must | F-002, F-036, F-037 | WHEN a draft_pr claim has created its worktree, THEN THE SYSTEM SHALL run the CI Skip-Coverage Scan before any patch request, refuse with `draft_pr_guard:ci_skip_unverifiable:<detail>` whenever a default-branch workflow declares a trigger outside the skip-safe set, a non-Actions CI or deploy configuration file exists, the default-branch head carries a check suite from an app other than `github-actions` or any commit status, or any lookup is incomplete, and keep the 001 BS outcome. |
| REQ-08 | Must | F-017 | WHEN band checks for open band PRs, THEN THE SYSTEM SHALL enumerate every open pull request with `gh api --paginate`, stop when a head starts with `autopus/band/<series-slug>-<h8>-`, and refuse with `draft_pr_guard:open_pr_lookup_incomplete` whenever the enumeration fails or ends before the last page. |
| REQ-09 | Must | F-032 | THE SYSTEM SHALL give each draft_pr claim the 1,230 s budget of the Step Timeouts table inside SPEC-SIGMABAND-001's chained lease rule, so a draft_pr executed after a diagnose of the same run gets 930 + 1,230 = 2,160 s. |
| REQ-10 | Must | FR-23 | THE SYSTEM SHALL document the flag, the skipped-checks note, the CI scan proof scope and its operator precondition, and the upgrade-before-enable note in the `auto react band` help text, `docs/health-band.md`, and `CHANGELOG.md`. |

## Draft PR Guard Contract

Branch: `autopus/band/<series-slug>-<h8>-<episode-id>`, where `<h8>` is the first 8 hex digits of the SHA-256 of the sanitized
series ID (`ci.failure_rate:CI` → `c6d37d0a`) and the slug is lowercase with runs outside `[a-z0-9]` turned into `-`, at most 40 characters.

| Order | Step | Code on failure | Stage |
|-------|------|-----------------|-------|
| 1 | Enumerate open PRs (REQ-08); stop when a head starts with `autopus/band/<series-slug>-<h8>-` | `open_pr_exists`, `open_pr_lookup_incomplete` | pre-mutation |
| 2 | `git fetch origin <default>`; `git worktree add --detach <tmp> origin/<default>` | `remote_unavailable` | pre-mutation, isolated |
| 3 | CI Skip-Coverage Scan | `ci_skip_unverifiable:<detail>` | pre-mutation |
| 4 | Patch request to the read-only provider of SPEC-SIGMABAND-001 with cwd `<tmp>` (second and last provider call of the episode) | 001 REQ-12 reasons | pre-mutation |
| 5 | The reply holds exactly one ```` ```diff ```` fence | `no_patch` | pre-mutation |
| 6 | No absolute path, `..`, rename, copy, deletion, mode change, symlink, or `GIT binary patch` | `patch_invalid` | pre-mutation |
| 7 | Every path passes the allowlist and the deny list below | `path_denied` | pre-mutation |
| 8 | Added lines through `SanitizeContent` report neither `secret_risk` nor `injection_risk` | `patch_content_denied` | pre-mutation |
| 9 | At most 10 files and 400 changed lines (added + removed) | `patch_too_large` | pre-mutation |
| 10 | `git apply --index --check`, `git apply --index`, and `git diff --cached --name-only` equal to the parsed path set | `patch_invalid` | pre-mutation, isolated |
| 11 | `lore.BuildCommit` with the trailer values below, then `lore.Validate(message, projectLoreConfig)` returns no error | `lore_unfillable:<trailer>`, `lore_rejected` | pre-mutation |
| 12 | `git commit --no-verify -F <msg>` under the Git Execution Policy | `commit_failed` | mutation |
| 13 | `git push --no-verify --no-follow-tags --recurse-submodules=no origin HEAD:refs/heads/autopus/band/<series-slug>-<h8>-<episode-id>` with `-c push.followTags=false`; a target equal to the default branch is refused | `push_rejected` | mutation |
| 14 | `gh pr create -R <owner/repo> --draft --base <default> --head <band-branch> --title <subject> --body-file <file>` | `gh_failed` + `orphan_branch:<ref>` | mutation |
| 15 | `git worktree remove --force <tmp>` and `git worktree prune` on every exit path; `--force` appears only here, only for the band-owned path | - | cleanup |

- Allowlist: extensions `.go .ts .tsx .js .jsx .mjs .cjs .py .rs .java .kt .rb .php .cs .swift .c .h .cc .cpp .hpp`.
- Deny list (any path segment or file name): `.github/`, `.gitlab/`, `.circleci/`, `.buildkite/`, `.husky/`, `.githooks/`,
  `.lefthook/`, `.autopus/`, `.omp/`, `.agents/`, `.cursor/`, `.windsurf/`, `scripts/`, `vendor/`, `node_modules/`, every
  `GeneratedSurfacePrefixes` and `GeneratedSurfaceExactPaths` entry, `Makefile`, `GNUmakefile`, `Dockerfile*`, `docker-compose*`,
  `package.json`, `go.mod`, `go.sum`, `AGENTS.md`, `CLAUDE.md`, `GEMINI.md`, `.gitattributes`, `.gitmodules`,
  `.pre-commit-config.yaml`, `lefthook.yml`, `lefthook.yaml`, `.gitlab-ci.yml`, `azure-pipelines.yml`, `Jenkinsfile`, `.env*`,
  `*.pem`, `*.key`, `id_rsa*`, `*credentials*`, `*.p12`, `*.pfx`.
- Never called: `gh pr merge`, `gh pr review`, any `--auto`, `--admin`, `--tags`, `--mirror`, `git stash`, `git checkout` or
  `git reset` in the user worktree, `react apply`, any repository hook, and any build, test, or run of the proposed change.
- Commit message via `lore.BuildCommit`: subject `fix(band): <series-slug> anomaly draft (<episode-id>)`, or
  `fix(band): <series-slug> 이상 대응 초안 (<episode-id>)` when the commit-message language is `ko`; body with tier, z, BS ID, and
  a final `[skip ci]` line; the `pkg/lore` sign-off; trailers below for every configured required trailer.

| Trailer | Value (accepted by the `lore.Validate` enums, `pkg/lore/validator.go:9-11`) |
|---------|-------|
| Constraint | `generated by auto react band; draft PR needs human review` |
| Related | `<BS-ID>` |
| Confidence | `low` |
| Scope-risk | `local` |
| Reversibility | `trivial` |
| Directive | `review the diff before running checks` |
| Tested | `none; checks are skipped until human review` |
| Not-tested | `every check until a human removes the skip` |
| Rejected | `write-capable agent; read-only proposal only` |
| any other required trailer | not filled; step 11 stops with `lore_unfillable:<trailer>` |

## Git Execution Policy

Every git command of the flow runs with `-c core.hooksPath=<hooks-off>` (an empty band-owned directory, equivalent to
`/dev/null` and portable to Windows) and `-c core.fsmonitor=false`. Probe evidence (carried from 001 rev 2): a default commit
logs 2 trace2 `child_class` hook events, a commit with hooks off and `--no-verify` logs 0. Open as Completion Debt CD-1: the
full set of configuration keys that execute commands inside the patched worktree regardless of `core.hooksPath` (for example
`core.fsmonitor`, `filter.<driver>.clean`/`smudge`/`process`, `diff.<driver>.textconv`, `core.sshCommand`, `credential.helper`,
`gpg.program`, `include.path` and `includeIf` with relative paths) and the rule that either neutralizes each key or refuses with
`git_config_unsafe:<key>` before step 10.

## CI Skip-Coverage Scan

Runs as Guard step 3 on the band worktree, which is the remote default branch; a band patch never touches `.github/`.

1. Workflows: every `.github/workflows/*.yml` and `*.yaml`; the top-level `on` key (or the YAML 1.1 `true` key) as a string,
   list, or map. Proposed skip-safe set (Completion Debt CD-2 must confirm it against the GitHub event documentation): `push`
   and `pull_request` (GitHub skips both for a commit or pull-request head carrying `[skip ci]`), and `schedule`,
   `workflow_dispatch`, `workflow_call`, `merge_group`, `release`, `repository_dispatch`. Every other trigger, including
   `pull_request_target`, `pull_request_review`, `pull_request_review_comment`, `issue_comment`, `workflow_run`, `create`,
   `check_run`, `check_suite`, `status`, `deployment`, `deployment_status`, and `page_build`, gives
   `ci_skip_unverifiable:trigger:<name>@<file>`; an unparsable file gives `ci_skip_unverifiable:parse:<file>`.
2. Non-Actions CI and deploy configuration at the default branch: `.circleci/`, `.travis.yml`, `.gitlab-ci.yml`,
   `azure-pipelines.yml`, `Jenkinsfile`, `.buildkite/`, `bitbucket-pipelines.yml`, `.drone.yml`, `appveyor.yml`, `.cirrus.yml`,
   `codemagic.yaml`, `.woodpecker.yml`, `.woodpecker/`, `cloudbuild.yaml`, `buildspec.yml`, `netlify.toml`, `vercel.json`,
   `railway.json`, `railway.toml`, `render.yaml`, `fly.toml` give `ci_skip_unverifiable:config:<path>`.
3. Observed CI on the default-branch head: check suites enumerated with `gh api --paginate` and counted against
   `total_count` (Completion Debt CD-3); any app other than `github-actions` gives `ci_skip_unverifiable:check_suite:<slug>`;
   any commit status gives `ci_skip_unverifiable:status:<context>`; an error, a timeout, or a count below `total_count` gives
   `ci_skip_unverifiable:scan_incomplete` (Completion Debt CD-4 covers every other lookup).
4. Outcome: a refusal is a pre-mutation guard; the 001 BS stays; the episode made 1 provider call. Example (probe evidence
   carried from 001): `autopus-ai/autopus-adk` uses only `push`, `pull_request`, `schedule`, `workflow_call`,
   `workflow_dispatch`, but its head carries a `railway-app` check suite, so band refuses with
   `ci_skip_unverifiable:check_suite:railway-app`.
5. Proof scope: CI observable through workflow files, CI and deploy configuration files, check suites, and commit statuses.
   Enabling the flag asserts that no other system builds pushed branches; REQ-10 documents this precondition.

## gh Invocation Table

Extends the table of SPEC-SIGMABAND-001 (verified with `--help` of gh 2.98.0: `pr list` and `pr create` accept `-R`; `api`
takes the path plus `--hostname`). Each call has a 30 s timeout (`gh pr create` 60 s) and runs with `GH_REPO` and `GH_HOST` set.

| Purpose | Command |
|---------|---------|
| Open PRs | `gh api --paginate repos/<owner>/<repo>/pulls?state=open&per_page=100 --hostname <host> --jq '.[].head.ref'` |
| Draft PR | `gh pr create -R <owner/repo> --draft --base <default> --head <band-branch> --title <subject> --body-file <file>` |
| Check suites | `gh api --paginate repos/<owner>/<repo>/commits/<sha>/check-suites --hostname <host>` |
| Commit statuses | `gh api repos/<owner>/<repo>/commits/<sha>/status --hostname <host>` |

## Step Timeouts and Lease

Draft_pr budget = 30 (open PRs) + 120 (fetch) + 60 (worktree add) + 2×30 (scan) + 600 (patch request) + 60 (apply and commit)
+ 120 (push) + 60 (PR create) + 60 (worktree remove and prune) + 60 (margin) = 1,230 s. SPEC-SIGMABAND-001's chained lease
rule adds the budgets of every earlier claim of the same run.

## 생성 파일 상세

| Path | Role |
|------|------|
| [NEW] `pkg/config/schema_health_band.go` (field added) | `AllowDraftPR bool` with `omitempty` next to 001's `DiagnosisProvider` |
| [NEW] `pkg/healthband/draftpr_claim.go` | REQ-02 claim attachment on top of 001's decision table |
| [NEW] `pkg/healthband/patchguard.go`, `commitmsg.go`, `ciscan.go`, `gitpolicy.go` | diff parsing and path, content, size guards; Lore message and trailer values; CI Skip-Coverage Scan; Git Execution Policy |
| [NEW] `internal/cli/react_band_draftpr.go` | git and gh steps of the Guard Contract |
| `internal/cli/react_band_gh.go` (from 001) | the four commands of the table above |
| `docs/health-band.md`, `CHANGELOG.md` | flag, precondition, skipped-checks note |

## Related SPECs

- SPEC-SIGMABAND-001 (parent of this sibling): must be implemented first; this SPEC adds a claim kind and never changes 001's
  action enum, decision table, BS format, or provider contract.
- SPEC-REVIEWRO-001: the patch request uses 001's read-only provider contract, including `--tools=Read,Grep,Glob`.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-01 | T1 | S1, S10 | INV-01 |
| REQ-02 | T2 | S2 | INV-02 |
| REQ-03 | T6 | S3 | INV-03, INV-04 |
| REQ-04 | T6 | S4 | INV-04 |
| REQ-05 | T4, T6 | S3, S7 | INV-03 |
| REQ-06 | T3 | S4 | INV-05 |
| REQ-07 | T5 | S5, S6 | INV-06 |
| REQ-08 | T7 | S8 | INV-07 |
| REQ-09 | T2 | S9 | INV-02 |
| REQ-10 | T8 | S11 | INV-01 |
