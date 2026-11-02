# SPEC-WORKERPOST-001: Worker Completion Contract — Explicit Postconditions, Four States, and SHA-Verified Push

**Status**: draft
**Created**: 2026-11-02
**Domain**: WORKERPOST
**Priority**: HIGH
**Change Class**: security_or_data (the check decides whether a worker task that commits or pushes is reported complete)
**Source**: R1 of the ADK reliability brief (baseline `4a5fa659`), re-verified on `origin/main` `109c7ea4`
**Target module**: `autopus-adk/` (this repository)
**Module ownership**: `.autopus/specs/SPEC-WORKERPOST-001/**`

## Purpose

The retained worker decides whether a task is complete by guessing git postconditions from the words `commit`, `push`, and `branch` in the prompt (`pkg/worker/postconditions.go`). Two defects follow, both reproduced on `109c7ea4` with temporary repositories and a bare remote:

1. The prompt `Review only; do not commit or push` is read as requiring a commit, so a read-only task that obeyed its instructions fails with `postcondition failed: commit: HEAD did not advance after execution`.
2. The push check passes when `git ls-remote --heads origin <branch>` finds the branch at all. A branch pushed earlier at an older commit, with the new local commit never pushed, passes. The same pattern lookup also matches `refs/heads/other/<branch>`.

This SPEC gives the task input an explicit, versioned completion contract, judges every check as `satisfied`, `unsatisfied`, `not_required`, or `unknown`, verifies a push by comparing the exact remote ref's SHA with the expected commit, never reports `unknown` as success, and keeps the prompt inference only as a negation-aware fallback for payloads that carry no contract.

## Background

- The worker receives the task payload as opaque JSON inside the A2A `tasks/send` params; `pkg/worker/a2a` merges signed control-plane metadata into it and `handleTask` decodes it into `taskPayloadMessage`. The backend sends no completion contract today, so every current task takes the fallback path.
- `verifyExecutionPostconditions` runs after a successful provider execution in `executeWithParallel` and `executePipelineWithParallel` (`pkg/worker/loop_exec.go`). A returned error fails the task, and the report travels as the `postconditions.json` artifact. No code outside `pkg/worker` reads the artifact or matches its error text (searched in this repository, `autopus/`, and `autopus-desktop/` on 2026-11-02).
- Worker tasks may run in a linked worktree on a `worker-<task id>` branch that shares refs, objects, and configuration with the main checkout. Codex worker tasks can write the shared git directory, so repository configuration can change during execution.

## Outcome Boundary

- Outcome Lock: a task payload can state which git actions the task requires and forbids, the remote, the exact branch ref, and the expected commit; the worker reports each postcondition as `satisfied`, `unsatisfied`, `not_required`, or `unknown`, completes the task only when no check is `unsatisfied` or `unknown`, verifies a push by the SHA of the exact ref on the remote URL recorded before execution, never runs a git command that writes, and, for payloads without a contract, never requires an action the prompt negates and never reports success without SHA evidence.
- Mandatory requirements: REQ-WPOST-SCOPE-01 through REQ-WPOST-VERIFY-01 below.
- Explicit non-goals: performing or retrying commits or pushes; tags and refs outside `refs/heads/`; verifying a commit made on a branch HEAD does not end on; attributing pushes of commits the task did not leave on HEAD; changing the A2A transport, the control-plane signature, or the backend that would send the contract; changing `pkg/pipeline`.
- Completion evidence: hermetic tests in `pkg/worker` (temporary repositories with a bare remote, `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`) for every Must requirement, the two reproductions failing before the fix and passing after it, the whole `pkg/worker` package passing, and `go run ./cmd/source-lines --max 300 --ext .go` clean over the changed Go files.

## Definitions

- Contract: the optional `completion_contract` object of the task payload: `version` (integer), `required_actions` and `forbidden_actions` (arrays of action names), `remote` (remote name), `ref` (branch ref), `expected_commit` (commit SHA), `push_match` (`exact` or `contains`).
- Action: `commit`, `push`, or `branch`, matched case-insensitively after trimming.
- Requirement level: `required`, `forbidden`, `none`, and, in fallback mode only, `ambiguous`.
- Baseline: captured in the directory the provider runs in, after any worktree is assigned and immediately before the provider starts: HEAD's commit (absent when HEAD does not resolve to a commit), the fetch URL of the target remote from `git remote get-url` (the pinned URL), whether the local branch named by the contract ref exists (present, absent, or unknown), and, when push is forbidden, the branch heads of the pinned URL (the head snapshot, absent when that lookup fails).
- Expected commit: `expected_commit` when the contract gives it, else the commit HEAD points at after execution while HEAD is on a branch.
- HEAD advanced: HEAD's commit after execution differs from the baseline commit and the baseline commit is its ancestor.
- Trusted ancestry: `git --no-replace-objects merge-base --is-ancestor` over local objects, when the repository has no graft file; in a shallow repository only a positive answer is trusted.
- Neutral directory: a fresh empty temporary directory with `GIT_CEILING_DIRECTORIES` set to its parent, so git reads no repository configuration there.
- Fallback mode: the payload has no `completion_contract` key; requirements come from the prompt as defined in REQ-WPOST-LEGACY-01.
- Clause: a run of the lowercased prompt between `.`, `;`, `!`, `?`, and line breaks. Tokens: words of `[a-z0-9_]` with an optional apostrophe suffix (a trailing `'s` dropped), runs of Hangul, and the separators `,` and `/`.
- Negated mention: an action word (`commit`, `commits`, `committed`, `committing`, `push`, `pushes`, `pushed`, `pushing`, `branch`, `branches`) that either follows, in the same clause, a negation cue (`not`, `no`, `never`, `without`, `nor`, `don't`, `dont`, `doesn't`, `didn't`, `mustn't`, `shouldn't`, `won't`, `can't`, `cannot`, `avoid`, `avoiding`, `refrain`, `skip`, `forbid`, `prohibit`) with only fillers, separators, cues, and action words in between, or is followed within four tokens, with no comma or action word in between, by `forbidden`, `prohibited`, `disallowed`, `banned`, `not` plus one of `allowed`, `permitted`, `needed`, `required`, `necessary`, a token starting with `금지`, or `하지` plus a token starting with `마`, `말`, or `않`. Fillers: `a`, `an`, `the`, `any`, `your`, `our`, `this`, `these`, `those`, `it`, `them`, `change`, `changes`, `work`, `git`, `to`, `be`, `being`, `been`, `yet`, `ever`, `anything`, `new`, `create`, `creating`, `make`, `making`, `perform`, `performing`, `run`, `running`, `do`, `doing`, `try`, `attempt`, `need`, `force`, `from`, `or`, `and`.
- Affirmative mention: the token `commit`, `push`, or `branch` (the words the pre-fix inference matched) that is not a negated mention.

## Requirements

### Scope

**REQ-WPOST-SCOPE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL confine the change to `pkg/worker/**`, `[NEW] docs/worker-completion-contract.md`, and `.autopus/specs/SPEC-WORKERPOST-001/**`, SHALL NOT edit the generated roots `.claude/**`, `.codex/**`, `.gemini/**`, `.opencode/**`, `.omp/**`, or `.agents/**`, and SHALL NOT change `pkg/pipeline/**` or `internal/cli/**`.

### Task Input

**REQ-WPOST-INPUT-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL accept the optional `completion_contract` object in the task payload, read an absent or zero `version` as 1, default `remote` to `origin` and `push_match` to `exact`, normalize a bare branch name in `ref` to `refs/heads/<name>`, lowercase `expected_commit`, and give every action in neither list the level `none`.

**REQ-WPOST-INPUT-02**
Priority: Must
Type: Unwanted
IF the contract has a `version` other than 1, an action outside `commit`, `push`, and `branch`, an action in both lists, a `remote` not matching `^[A-Za-z0-9][A-Za-z0-9._-]*$`, a `ref` outside `refs/heads/` or with an invalid branch name, an `expected_commit` that is not 40 or 64 hexadecimal characters, or a `push_match` other than `exact` and `contains`, THEN THE SYSTEM SHALL fail the task before any provider runs with an error starting `invalid completion_contract: ` that names every problem, quoting each sender value clipped to 64 bytes.

**REQ-WPOST-INPUT-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL derive no requirement from the prompt when the payload carries a contract.

### States And Outcome

**REQ-WPOST-STATE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL record each check in `postconditions.json` with `name`, `requirement`, `status` (`satisfied`, `unsatisfied`, `not_required`, or `unknown`), and `detail`, plus `remote`, `ref`, `expected`, `observed`, and `match` when they apply; SHALL give the report `version` 2, `active` true, `source` (`contract` or `legacy_prompt`), and `outcome` (the first present of `unsatisfied`, `unknown`, `satisfied`, then `not_required`); SHALL list one check per action; SHALL report a check whose level is `none` as `not_required`. Fallback mode lists one branch check per extracted branch name.

**REQ-WPOST-STATE-02**
Priority: Must
Type: Event-driven
WHEN the outcome is `unsatisfied`, THEN THE SYSTEM SHALL fail the task with an error starting `postcondition failed: `; WHEN the outcome is `unknown`, THEN THE SYSTEM SHALL fail the task with an error starting `postcondition unverified: `; both errors SHALL list every check that is not `satisfied` or `not_required`, and only the outcomes `satisfied` and `not_required` SHALL let the task complete.

### Commit

**REQ-WPOST-COMMIT-01**
Priority: Must
Type: Event-driven
WHEN `commit` is required, THEN THE SYSTEM SHALL report `satisfied` when HEAD advanced by trusted ancestry, `unsatisfied` with `HEAD did not advance after execution` when HEAD equals the baseline commit, `unsatisfied` when HEAD moved to a commit the baseline is not an ancestor of, and `unknown` when there is no baseline commit, HEAD does not resolve, or ancestry is not trusted.

**REQ-WPOST-COMMIT-02**
Priority: Must
Type: Event-driven
WHEN `commit` is forbidden, THEN THE SYSTEM SHALL report `satisfied` when HEAD equals the baseline commit or when neither the baseline nor the final state has a HEAD commit, `unsatisfied` when HEAD advanced (a fast-forward pull counts) or when a HEAD commit appeared in a work directory that had none, and `unknown` when HEAD moved to a commit that does not descend from the baseline or git fails.

### Push

**REQ-WPOST-PUSH-01**
Priority: Must
Type: Event-driven
WHEN `push` is required, THEN THE SYSTEM SHALL run `git ls-remote -- <pinned URL> <ref>` in a neutral directory, read the SHA of the line whose ref name equals the target ref exactly, and report `satisfied` when that SHA equals the expected commit, or, under `contains`, when the remote tip is present locally and the expected commit is its ancestor by trusted ancestry.

**REQ-WPOST-PUSH-02**
Priority: Must
Type: Unwanted
IF the exact target ref is absent from the remote, its SHA differs from the expected commit under `exact`, or under `contains` the remote tip is present locally and trusted ancestry shows it does not contain the expected commit, THEN THE SYSTEM SHALL report the push `unsatisfied`, and SHALL NOT treat the existence of a branch with that name, or of a ref that only ends with it, as evidence.

**REQ-WPOST-PUSH-03**
Priority: Must
Type: Unwanted
IF a required push has no target ref, its remote had no URL at the baseline, HEAD is detached or unresolvable while the contract gives no `expected_commit`, the lookup fails for any reason (unreachable remote, authentication, timeout), or a `contains` comparison lacks the remote tip locally, finds a graft file, or gets a negative answer in a shallow repository, THEN THE SYSTEM SHALL report the push `unknown`.

**REQ-WPOST-PUSH-04**
Priority: Must
Type: Event-driven
WHEN `push` is forbidden, THEN THE SYSTEM SHALL report `satisfied` when HEAD equals the baseline commit (neither resolving counts as equal), when the remote had no URL at the baseline and has none after execution, or when every `refs/heads/` entry of `git ls-remote --heads -- <pinned URL>` that points at a commit of `git rev-list <baseline>..HEAD` pointed at that same commit in the head snapshot; `unsatisfied` naming the ref when such an entry moved there during execution; and `unknown` when the lookup fails, such an entry exists but the head snapshot is absent, the remote gained a URL during execution, there is no baseline commit while HEAD resolves, or HEAD no longer resolves although the baseline had a commit.

### Branch

**REQ-WPOST-BRANCH-01**
Priority: Must
Type: Event-driven
WHEN `branch` is required, THEN THE SYSTEM SHALL report `unknown` without a target ref, `satisfied` when the local branch of the target ref exists and, if the contract gives `expected_commit`, points at it, and `unsatisfied` otherwise; WHEN `branch` is forbidden, THEN THE SYSTEM SHALL report `unknown` without a target ref, `unsatisfied` when that local branch exists after execution but was absent at the baseline, `unknown` when git fails or the baseline state of that branch is unknown while it exists, and `satisfied` otherwise.

### Read-Only Verification

**REQ-WPOST-READONLY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL run only the exact forms of the git subcommands `rev-parse`, `symbolic-ref`, `remote get-url`, `ls-remote`, `merge-base --is-ancestor`, `cat-file -e`, and `rev-list` that the checks use during baseline capture and verification, refusing any other subcommand, form, or operand starting with `-` before a process starts; SHALL pass `--no-replace-objects`, set `GIT_TERMINAL_PROMPT=0`, and drop inherited `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_OBJECT_DIRECTORY`, `GIT_ALTERNATE_OBJECT_DIRECTORIES`, `GIT_COMMON_DIR`, `GIT_NAMESPACE`, `GIT_GRAFT_FILE`, `GIT_REPLACE_REF_BASE`, `GIT_SHALLOW_FILE`, and `GIT_CEILING_DIRECTORIES`; and SHALL NOT commit, push, fetch, or change refs, configuration, or objects, whatever the outcome.

### Fallback Mode

**REQ-WPOST-LEGACY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL in fallback mode give `commit` and `push` the level `required` when the prompt has an affirmative mention and no negated mention, `ambiguous` when it has both, and `none` otherwise; SHALL give `branch` a level by the same rule only when the pre-fix patterns (`refs/heads/<name>`, `branch [named|called] <name>`, `origin/<name>`, `origin <name>`) extract a branch name, else `none`; and SHALL NOT derive a forbidden level from the prompt.

**REQ-WPOST-LEGACY-02**
Priority: Must
Type: Event-driven
WHEN fallback mode judges a push, THEN THE SYSTEM SHALL use the remote `origin` pinned at the baseline, the target ref of the current branch when HEAD is on a branch not starting with `worker-` and that branch is among the extracted names or none were extracted, else of the single extracted name, else no target, with the expected commit taken from HEAD and the `exact` match; and WHEN it judges a required or ambiguous branch, THEN THE SYSTEM SHALL check that every extracted name exists as a local branch.

**REQ-WPOST-LEGACY-03**
Priority: Must
Type: Unwanted
IF a fallback check whose level is `ambiguous` would be `unsatisfied`, THEN THE SYSTEM SHALL report it `unknown`; and IF the prompt has no affirmative or negated mention, or the work directory had no HEAD commit at the baseline, THEN THE SYSTEM SHALL emit no report and no error, as before this change.

### Report Hygiene

**REQ-WPOST-REPORT-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL keep credentials out of the report and the task error: fields it sets name a remote only by its configured name, never by URL, and every git message it quotes has the userinfo of each URL replaced with `<redacted>` and is clipped to its first line and 200 bytes.

### Documentation

**REQ-WPOST-DOC-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL document the contract fields, the four states and the outcome rule, the per-action semantics, the pinned remote, the read-only guarantee, the fallback grammar with its limits, and a read-only and a commit-and-push example in `docs/worker-completion-contract.md`.

### Verification

**REQ-WPOST-VERIFY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL prove every Must requirement with tests that build temporary repositories and a bare remote under the test's own temporary directory, run their git commands with `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_NOSYSTEM=1`, and inject the same environment into the verifier, and SHALL keep the reproductions of both defects as regression tests.

## Generated And Changed Surfaces

- `pkg/worker/postconditions.go`: report, outcome, error, and the verification entry point.
- `[NEW] pkg/worker/postconditions_contract.go`: the contract message, validation, normalization, and the plan.
- `[NEW] pkg/worker/postconditions_legacy.go`: the fallback inference and its negation grammar.
- `[NEW] pkg/worker/postconditions_git.go`: the read-only git runner and baseline capture.
- `[NEW] pkg/worker/postconditions_checks.go`: the commit and branch checks and trusted ancestry.
- `[NEW] pkg/worker/postconditions_push.go`: the required and forbidden push checks.
- `pkg/worker/loop_task.go` (payload field, contract rejection), `pkg/worker/loop_exec.go` (plan, verification, and the baseline moved after worktree assignment), `pkg/worker/operator_metadata.go` (plan carried in `taskRunMeta`), `pkg/worker/loop.go` (verifier seam), `pkg/worker/adapter/codex_worker_sandbox.go` (comment only).
- `[NEW] docs/worker-completion-contract.md`.

## Related SPECs

- None. The worker's earlier fail-closed postconditions came from commit `b8a5436e` without a SPEC.

## Traceability Matrix

| Requirement | Plan task | Acceptance | Invariants |
|---|---|---|---|
| REQ-WPOST-SCOPE-01 | T7 | S12 | INV-P08 |
| REQ-WPOST-INPUT-01, INPUT-03 | T1 | S9 | INV-P05 |
| REQ-WPOST-INPUT-02 | T1, T5 | S9 | INV-P05 |
| REQ-WPOST-STATE-01, STATE-02 | T4 | S1, S2, S11 | INV-P01 |
| REQ-WPOST-COMMIT-01, COMMIT-02 | T3 | S6 | INV-P01 |
| REQ-WPOST-PUSH-01, PUSH-02 | T3 | S2, S3, S5 | INV-P01, INV-P04 |
| REQ-WPOST-PUSH-03 | T3 | S4, S5 | INV-P01 |
| REQ-WPOST-PUSH-04 | T3 | S1, S7 | INV-P01, INV-P04 |
| REQ-WPOST-BRANCH-01 | T3 | S7 | INV-P01 |
| REQ-WPOST-READONLY-01 | T2 | S10 | INV-P03 |
| REQ-WPOST-LEGACY-01, LEGACY-02, LEGACY-03 | T2, T4 | S1, S8 | INV-P02, INV-P06 |
| REQ-WPOST-REPORT-01 | T4 | S11 | INV-P07 |
| REQ-WPOST-DOC-01 | T6 | S12 | INV-P08 |
| REQ-WPOST-VERIFY-01 | T7 | S1, S2 | INV-P01, INV-P02 |
