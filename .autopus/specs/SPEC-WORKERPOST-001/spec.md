# SPEC-WORKERPOST-001: Worker Completion Contract — Explicit Postconditions, Four States, and SHA-Verified Push

**Status**: draft
**Created**: 2026-11-02
**Revised**: 2026-11-02 (codex review round 1, judge REVISE, 12 accepted findings)
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

This SPEC gives the task input an explicit, versioned completion contract, judges every check as `satisfied`, `unsatisfied`, `not_required`, or `unknown` through ordered decision tables, verifies a push by comparing the exact remote ref's SHA with the expected commit, reads history only from evidence the task cannot forge, never reports `unknown` as success, and keeps the prompt inference only as a negation-aware fallback for payloads without a contract.

## Background

- The worker receives the task payload as opaque JSON inside the A2A `tasks/send` params; `pkg/worker/a2a` merges signed control-plane metadata into it and `handleTask` decodes it into `taskPayloadMessage`. The backend sends no completion contract today, so every current task takes the fallback path.
- The postcondition check runs after a successful provider execution in `executeWithParallel` and `executePipelineWithParallel` (`pkg/worker/loop_exec.go`). A returned error fails the task, and the report travels as the `postconditions.json` artifact. No code outside `pkg/worker` reads the artifact or matches its error text (searched in this repository, `autopus/`, and `autopus-desktop/` on 2026-11-02).
- Worker tasks may run in a linked worktree on a `worker-<task id>` branch that shares refs, objects, and configuration with the main checkout. Codex worker tasks can write the shared git directory, so the configuration, the object store, replace refs, graft and shallow files, and the commit-graph can all change during execution.

## Outcome Boundary

- Outcome Lock: a task payload can state which git actions the task requires and forbids, the remote, the exact branch ref, and the expected commit; the worker reports each postcondition as `satisfied`, `unsatisfied`, `not_required`, or `unknown` by the first matching row of its decision table, completes the task only when no check is `unsatisfied` or `unknown`, verifies a push by the SHA of the exact ref on the remote URL pinned before execution, reads ancestry only from trusted history, reads a failed lookup as `unknown` and never as absence, never writes a ref, configuration, or object or reaches the network except through the pinned lookup, and, for payloads without a contract, never requires an action the prompt negates and never reports success without SHA evidence.
- Mandatory requirements: REQ-WPOST-SCOPE-01 through REQ-WPOST-VERIFY-01 below.
- Explicit non-goals: performing or retrying commits or pushes; tags and refs outside `refs/heads/`; verifying a commit made on a branch HEAD does not end on; attributing pushes of commits the task did not leave on HEAD; changing the A2A transport, the control-plane signature, or the backend that would send the contract; changing `pkg/pipeline`.
- Completion evidence: hermetic tests in `pkg/worker` (temporary repositories with a bare remote, `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`) for every Must requirement, the reproductions failing before the fix, the whole `pkg/worker` package passing, and `go run ./cmd/source-lines --max 300 --ext .go` clean over the changed Go files.

## Definitions

- Contract: the value of the `completion_contract` key of the task payload, an object with `version` (integer), `required_actions` and `forbidden_actions` (arrays of strings), `remote`, `ref`, `expected_commit`, and `push_match` (strings). A payload without the key is in fallback mode; a present key whose value is null, not an object, holds an unknown key, or holds a field of another type (null included) is an invalid contract.
- Action: `commit`, `push`, or `branch`, matched case-insensitively after trimming.
- Requirement level: `required`, `forbidden`, `none`, and, in fallback mode only, `ambiguous`.
- Branch name: a name `git check-ref-format --branch` accepts for a literal name: `refs/heads/<name>` passes git's refname rules (no `..`, no `@{`, no control character, space, `~`, `^`, `:`, `?`, `*`, `[`, or `\`, no empty component, no component starting with `.` or ending with `.lock`, no trailing `.`), and the name neither starts with `-` nor is `HEAD`. Non-ASCII letters and characters such as `+` are valid.
- Lookup result: every git lookup ends present (exit 0), absent, or failed. Absent is only the command's documented not-found answer (`rev-parse --verify -q HEAD` exit 1, `remote get-url` exit 2, `cat-file -e` exit 1, an exact `for-each-ref` that lists nothing) or git reporting the directory is not a repository; everything else, timeouts included, is failed.
- Baseline: captured in the directory the provider runs in, after any worktree is assigned and immediately before the provider starts: the lookup result and SHA of HEAD; the lookup result and URL of the target remote from `git remote get-url` (the pinned URL, where a relative local path, one without `scheme://` whose first `:` does not come before its first `/`, is made absolute against `git rev-parse --show-toplevel` as git itself resolves it); the lookup result of the local branch of the contract ref; and, when push is forbidden, the branch heads of the pinned URL (the head snapshot, absent when that lookup fails).
- Expected commit: `expected_commit` when the contract gives it, else the commit HEAD points at after execution while HEAD is on a branch.
- Trusted history: every git command runs with `--no-replace-objects`, `-c core.commitGraph=false`, `GIT_NO_LAZY_FETCH=1`, and, for commands other than `ls-remote`, `GIT_ALLOW_PROTOCOL=none`, so parents come from commit objects in the local object store and nothing is fetched. History is untrusted while a graft file (`info/grafts`) exists; in a shallow repository a negative answer is untrusted.
- Gained commits: the output of `git rev-list <HEAD> ^<baseline commit>` under trusted history.
- Neutral directory: a fresh empty temporary directory with `GIT_CEILING_DIRECTORIES` set to its parent, so git reads no repository configuration there.
- Clause: a run of the lowercased prompt between `.`, `;`, `!`, `?`, and line breaks. Tokens: words of `[a-z0-9_]` with an optional apostrophe suffix (a trailing `'s` dropped), runs of Hangul, and the separators `,` and `/`.
- Negated mention: an action word (`commit`, `commits`, `committed`, `committing`, `push`, `pushes`, `pushed`, `pushing`, `branch`, `branches`) that either follows, in the same clause, a negation cue (`not`, `no`, `never`, `without`, `nor`, `don't`, `dont`, `doesn't`, `didn't`, `mustn't`, `shouldn't`, `won't`, `can't`, `cannot`, `avoid`, `avoiding`, `refrain`, `skip`, `forbid`, `prohibit`) with only fillers, separators, cues, and action words in between, or is followed within four tokens, with no comma or action word in between, by `forbidden`, `prohibited`, `disallowed`, `banned`, `not` plus one of `allowed`, `permitted`, `needed`, `required`, `necessary`, a token starting with `금지`, or `하지` plus a token starting with `마`, `말`, or `않`. Fillers: `a`, `an`, `the`, `any`, `your`, `our`, `this`, `these`, `those`, `it`, `them`, `change`, `changes`, `work`, `git`, `to`, `be`, `being`, `been`, `yet`, `ever`, `anything`, `new`, `create`, `creating`, `make`, `making`, `perform`, `performing`, `run`, `running`, `do`, `doing`, `try`, `attempt`, `need`, `force`, `from`, `or`, `and`.
- Affirmative mention: the token `commit`, `push`, or `branch` (the words the pre-fix inference matched) that is not a negated mention.

## Decision Tables

Each check walks its table top to bottom; the first row whose condition holds sets the status. "Failed" and "absent" are lookup results.

Commit required (also `ambiguous`):

| Row | Condition | Status |
|---|---|---|
| C1 | HEAD's lookup failed | unknown |
| C2 | the baseline HEAD lookup failed | unknown |
| C3 | the baseline HEAD is absent | unknown |
| C4 | HEAD is absent | unknown |
| C5 | HEAD equals the baseline commit | unsatisfied, `HEAD did not advance after execution` |
| C6 | history is untrusted or the ancestry lookup failed | unknown |
| C7 | the baseline commit is an ancestor of HEAD | satisfied |
| C8 | the repository is shallow | unknown |
| C9 | any other state | unsatisfied |

Commit forbidden:

| Row | Condition | Status |
|---|---|---|
| F1 | HEAD's lookup failed | unknown |
| F2 | the baseline HEAD lookup failed | unknown |
| F3 | the baseline HEAD and HEAD are both absent | satisfied |
| F4 | the baseline HEAD is absent | unsatisfied |
| F5 | HEAD is absent | unknown |
| F6 | HEAD equals the baseline commit | satisfied |
| F7 | history is untrusted or the ancestry lookup failed | unknown |
| F8 | the baseline commit is an ancestor of HEAD (a fast-forward pull counts) | unsatisfied |
| F9 | any other state | unknown |

Push required (also `ambiguous`):

| Row | Condition | Status |
|---|---|---|
| P1 | no target ref: the contract names none, or in fallback mode HEAD's lookup failed or REQ-WPOST-LEGACY-02 selects none | unknown |
| P2 | the remote's baseline lookup failed or was absent | unknown |
| P3 | no `expected_commit` is given and HEAD's lookup failed, HEAD is absent, or HEAD is detached | unknown |
| P4 | the `ls-remote` lookup failed | unknown |
| P5 | the exact target ref is not on the remote | unsatisfied |
| P6 | its SHA equals the expected commit | satisfied |
| P7 | `push_match` is `exact` | unsatisfied |
| P8 | the tip's local lookup failed or the tip is absent from the local object store | unknown |
| P9 | history is untrusted or the ancestry lookup failed | unknown |
| P10 | the expected commit is an ancestor of the tip | satisfied |
| P11 | the repository is shallow | unknown |
| P12 | any other state | unsatisfied |

Push forbidden:

| Row | Condition | Status |
|---|---|---|
| Q1 | HEAD's lookup failed | unknown |
| Q2 | the baseline HEAD lookup failed | unknown |
| Q3 | the remote's baseline lookup failed | unknown |
| Q4 | the remote was absent at the baseline and its lookup now fails | unknown |
| Q5 | the remote was absent at the baseline and is present now | unknown |
| Q6 | the remote was absent at the baseline and still is | satisfied |
| Q7 | the baseline HEAD and HEAD are both absent | satisfied |
| Q8 | the baseline HEAD is absent | unknown |
| Q9 | HEAD is absent | unknown |
| Q10 | HEAD equals the baseline commit | satisfied |
| Q11 | history is untrusted, or listing the gained commits or the remote heads failed | unknown |
| Q12 | a remote branch points at a gained commit it did not point at in the head snapshot, and the repository is not shallow | unsatisfied |
| Q13 | a remote branch points at a gained commit, and the head snapshot is absent or the repository is shallow | unknown |
| Q14 | any other state | satisfied |

Branch required (also `ambiguous`, one check per extracted name in fallback mode):

| Row | Condition | Status |
|---|---|---|
| B1 | no target ref | unknown |
| B2 | the branch lookup failed | unknown |
| B3 | the branch is absent | unsatisfied |
| B4 | `expected_commit` is given and the branch points elsewhere | unsatisfied |
| B5 | any other state | satisfied |

Branch forbidden:

| Row | Condition | Status |
|---|---|---|
| G1 | no target ref | unknown |
| G2 | the branch lookup failed | unknown |
| G3 | the branch is absent | satisfied |
| G4 | the branch was present at the baseline | satisfied |
| G5 | the branch's baseline lookup failed | unknown |
| G6 | any other state | unsatisfied |

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
THE SYSTEM SHALL accept the `completion_contract` object in the task payload, read an absent or zero `version` as 1, default `remote` to `origin` and `push_match` to `exact`, accept any branch name in `ref` and normalize a bare one to `refs/heads/<name>`, lowercase `expected_commit`, and give every action in neither list the level `none`.

**REQ-WPOST-INPUT-02**
Priority: Must
Type: Unwanted
IF the contract is invalid, or has a `version` other than 1, an action outside `commit`, `push`, and `branch`, an action in both lists, a `remote` not matching `^[A-Za-z0-9][A-Za-z0-9._-]*$`, a `ref` outside `refs/heads/` or with an invalid branch name, an `expected_commit` that is not 40 or 64 hexadecimal characters, or a `push_match` other than `exact` and `contains`, THEN THE SYSTEM SHALL fail the task before any provider runs with one error starting `invalid completion_contract: ` that names every problem, each non-string list element by its index while the string elements are still checked as actions, and each sender value first stripped of URL credentials, then clipped to 64 bytes and quoted.

**REQ-WPOST-INPUT-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL derive no requirement from the prompt while the payload carries the `completion_contract` key, whatever its value.

### States And Outcome

**REQ-WPOST-STATE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL record each check in `postconditions.json` with `name`, `requirement`, `status` (`satisfied`, `unsatisfied`, `not_required`, or `unknown`), and `detail`, plus `remote`, `ref`, `expected`, `observed`, and `match` where they apply; SHALL give the report `version` 2, `active` true, `source` (`contract` or `legacy_prompt`), and `outcome`, the first status present in the order `unsatisfied`, `unknown`, `satisfied`, `not_required`; SHALL list one check per action and, in fallback mode, one branch check per extracted name; and SHALL report an action of level `none` as `not_required`.

**REQ-WPOST-STATE-02**
Priority: Must
Type: Event-driven
WHEN the outcome is `unsatisfied`, THEN THE SYSTEM SHALL fail the task with an error starting `postcondition failed: `; WHEN the outcome is `unknown`, THEN THE SYSTEM SHALL fail the task with an error starting `postcondition unverified: `; both errors SHALL list every check that is not `satisfied` or `not_required`, an `unknown` one marked `(unverified)` beside an `unsatisfied` one, and only the outcomes `satisfied` and `not_required` SHALL let the task complete.

### Lookups And History

**REQ-WPOST-LOOKUP-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL classify every lookup as present, absent, or failed as defined in Lookup result, SHALL read a local branch with an exact `for-each-ref` lookup rather than a revision that git expands, and SHALL let no decision table row that requires absence match a failed lookup.

**REQ-WPOST-PIN-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL pin the target remote's URL at the baseline, make a relative local path absolute against the work tree's top level, read a failure to resolve it as a failed remote lookup, and query only the pinned URL afterwards.

**REQ-WPOST-HISTORY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL answer every ancestry question and compute the gained commits only under trusted history, treat any answer as untrusted while a graft file exists, and treat a negative answer, a gained commit found on a remote branch included, as untrusted in a shallow repository.

### Checks

**REQ-WPOST-COMMIT-01**
Priority: Must
Type: Event-driven
WHEN `commit` is required, THEN THE SYSTEM SHALL report the status of the first matching row of the commit required table.

**REQ-WPOST-COMMIT-02**
Priority: Must
Type: Event-driven
WHEN `commit` is forbidden, THEN THE SYSTEM SHALL report the status of the first matching row of the commit forbidden table.

**REQ-WPOST-PUSH-01**
Priority: Must
Type: Event-driven
WHEN `push` is required, THEN THE SYSTEM SHALL run `git ls-remote -- <pinned URL> <ref>` in a neutral directory, take the SHA of the line whose ref name equals the target ref exactly, and report the status of the first matching row of the push required table.

**REQ-WPOST-PUSH-02**
Priority: Must
Type: Unwanted
IF the remote holds the target branch at a SHA other than the expected commit, or holds only refs whose names end with the target, THEN THE SYSTEM SHALL NOT report the push `satisfied` on that evidence.

**REQ-WPOST-PUSH-03**
Priority: Must
Type: Unwanted
IF a comparison needs a commit that is not in the local object store, THEN THE SYSTEM SHALL report the check `unknown` without fetching the commit.

**REQ-WPOST-PUSH-04**
Priority: Must
Type: Event-driven
WHEN `push` is forbidden, THEN THE SYSTEM SHALL report the status of the first matching row of the push forbidden table, listing the remote heads with `git ls-remote --heads -- <pinned URL>` in a neutral directory.

**REQ-WPOST-BRANCH-01**
Priority: Must
Type: Event-driven
WHEN `branch` is required, THEN THE SYSTEM SHALL report the status of the first matching row of the branch required table.

**REQ-WPOST-BRANCH-02**
Priority: Must
Type: Event-driven
WHEN `branch` is forbidden, THEN THE SYSTEM SHALL report the status of the first matching row of the branch forbidden table.

### Read-Only Verification

**REQ-WPOST-READONLY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL run only the exact forms of `rev-parse`, `symbolic-ref`, `remote get-url`, `for-each-ref`, `ls-remote`, `merge-base --is-ancestor`, `cat-file -e`, and `rev-list` that the checks use during baseline capture and verification, refusing any other subcommand, form, or operand starting with `-` before a process starts; SHALL run them with the trusted-history options and with `GIT_TERMINAL_PROMPT=0`, `GIT_OPTIONAL_LOCKS=0`, and `LC_ALL=C`, dropping inherited `GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`, `GIT_OBJECT_DIRECTORY`, `GIT_ALTERNATE_OBJECT_DIRECTORIES`, `GIT_COMMON_DIR`, `GIT_NAMESPACE`, `GIT_GRAFT_FILE`, `GIT_REPLACE_REF_BASE`, `GIT_SHALLOW_FILE`, `GIT_CEILING_DIRECTORIES`, and `LANGUAGE`; and SHALL NOT write any ref, configuration, or object, or fetch, whatever the outcome.

### Fallback Mode

**REQ-WPOST-LEGACY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL in fallback mode give `commit` and `push` the level `required` for an affirmative mention without a negated one, `ambiguous` for both, and `none` otherwise; SHALL give `branch` a level by the same rule only for a prompt in which the pre-fix patterns (`refs/heads/<name>`, `branch [named|called] <name>`, `origin/<name>`, `origin <name>`) extract a branch name, else `none`; and SHALL NOT derive a forbidden level from the prompt.

**REQ-WPOST-LEGACY-02**
Priority: Must
Type: Event-driven
WHEN fallback mode judges a push, THEN THE SYSTEM SHALL use the remote `origin`, the target ref of the current branch for a HEAD on a branch not starting with `worker-` that is among the extracted names or with no name extracted, else of the single extracted name, else no target, with the expected commit taken from HEAD and the `exact` match; and WHEN it judges a branch, THEN THE SYSTEM SHALL check every extracted name with the branch required table.

**REQ-WPOST-LEGACY-03**
Priority: Must
Type: Unwanted
IF a fallback check of level `ambiguous` reaches `unsatisfied`, THEN THE SYSTEM SHALL report it `unknown`; and IF the prompt has no affirmative or negated mention, or the baseline HEAD lookup was absent, THEN THE SYSTEM SHALL emit no report and no error, as before this change, while a failed baseline HEAD lookup still yields a report.

### Report Hygiene

**REQ-WPOST-REPORT-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL keep credentials out of the report and the task error: fields it sets name a remote only by its configured name, never by URL, and every git message and contract value it quotes has the user part of each URL or scp-like address, with or without a password or a scheme, replaced with `<redacted>`; a git message is also clipped to its first line and 200 bytes.

### Documentation

**REQ-WPOST-DOC-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL document the contract fields and invalid values, the four states and the outcome rule, the decision tables, the pinned remote, trusted history, the read-only guarantee, the fallback grammar with its limits, and a read-only and a commit-and-push example in `docs/worker-completion-contract.md`.

### Verification

**REQ-WPOST-VERIFY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL prove every Must requirement with tests that build temporary repositories and a bare remote under the test's own temporary directory, run their git commands with `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_NOSYSTEM=1`, and inject the same environment into the verifier, and SHALL keep the reproductions as regression tests.

## Generated And Changed Surfaces

- `pkg/worker/postconditions.go`: report, outcome, error, and the verification entry point.
- `[NEW] pkg/worker/postconditions_contract.go`, `[NEW] pkg/worker/postconditions_contract_decode.go`: the contract message, strict decoding, validation, normalization, and the plan.
- `[NEW] pkg/worker/postconditions_legacy.go`: the fallback inference and its negation grammar.
- `[NEW] pkg/worker/postconditions_git.go`: the read-only git runner, its environment, and redaction.
- `[NEW] pkg/worker/postconditions_baseline.go`: lookup results, baseline capture, URL pinning, trusted history.
- `[NEW] pkg/worker/postconditions_checks.go`, `[NEW] pkg/worker/postconditions_push.go`: the decision tables.
- `pkg/worker/loop_task.go` (payload field, contract rejection), `pkg/worker/loop_exec.go` (plan, verification, and the baseline moved after worktree assignment), `pkg/worker/operator_metadata.go` (plan carried in `taskRunMeta`), `pkg/worker/loop.go` (verifier seam), `pkg/worker/adapter/codex_worker_sandbox.go` (comment only).
- `[NEW] docs/worker-completion-contract.md`.

## Related SPECs

- None. The worker's earlier fail-closed postconditions came from commit `b8a5436e` without a SPEC.

## Traceability Matrix

| Requirement | Plan task | Acceptance | Invariants |
|---|---|---|---|
| REQ-WPOST-SCOPE-01 | T7 | S12 | INV-P08 |
| REQ-WPOST-INPUT-01, INPUT-03 | T1 | S9 | INV-P05 |
| REQ-WPOST-INPUT-02 | T1, T5 | S9 | INV-P05, INV-P07 |
| REQ-WPOST-STATE-01, STATE-02 | T4 | S1, S2, S11 | INV-P01 |
| REQ-WPOST-LOOKUP-01 | T2 | S4, S6, S7 | INV-P01 |
| REQ-WPOST-PIN-01 | T2 | S3 | INV-P04 |
| REQ-WPOST-HISTORY-01 | T2, T3 | S5, S7 | INV-P09 |
| REQ-WPOST-COMMIT-01, COMMIT-02 | T3 | S6 | INV-P01 |
| REQ-WPOST-PUSH-01, PUSH-02 | T3 | S2, S3, S5 | INV-P01, INV-P04 |
| REQ-WPOST-PUSH-03 | T2, T3 | S5, S10 | INV-P03 |
| REQ-WPOST-PUSH-04 | T3 | S1, S7 | INV-P01, INV-P04 |
| REQ-WPOST-BRANCH-01, BRANCH-02 | T3 | S7 | INV-P01 |
| REQ-WPOST-READONLY-01 | T2 | S10 | INV-P03 |
| REQ-WPOST-LEGACY-01, LEGACY-02, LEGACY-03 | T2, T4 | S1, S8 | INV-P02, INV-P06 |
| REQ-WPOST-REPORT-01 | T4 | S9, S11 | INV-P07 |
| REQ-WPOST-DOC-01 | T6 | S12 | INV-P08 |
| REQ-WPOST-VERIFY-01 | T7 | S1, S2 | INV-P01, INV-P02 |
