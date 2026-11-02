# SPEC-WORKERPOST-001 Research

## Existing Code Analysis

- `pkg/worker/postconditions.go` (`109c7ea4`): `detectTaskPostconditions` sets `CommitRequired`, `PushRequired`, and `BranchRequired` from the regular expressions `\bgit\s+commit\b|\bcommit\b`, `\bgit\s+push\b|\bpush\b`, and `\bbranch\b`, with no notion of negation. Branch names come from `refs/heads/<name>`, `branch [named|called] <name>`, and `origin[/ ]<name>`, where the last two require a `/` or `-` in the name. The commit check compares HEAD with the baseline SHA only. The push check runs `git ls-remote --exit-code --heads origin <branch>` and passes on exit 0, so it proves only that some ref ending in `<branch>` exists. Statuses are `passed` and `failed`; a failed git command is `failed`, never distinguished from a negative answer.
- `pkg/worker/loop_exec.go`: `executeWithParallel` captures the baseline before the worktree is assigned (worktrees share refs and configuration with the main checkout), and both execution paths call `verifyExecutionPostconditions(workDir, prompt, baseline)` after a successful provider run; an error fails the task and the report is appended as `postconditions.json`.
- `pkg/worker/loop_task.go`: `taskPayloadMessage` is decoded from the payload that `pkg/worker/a2a` merges with signed control-plane metadata; unknown keys are ignored by `encoding/json`, so an older worker ignores a contract it does not know.
- `pkg/worker/parallel/worktree.go`: a worker worktree sits on `worker-<task id>`; `branchesForVerification` excluded such branches, so a push prompt without a named branch failed in every worktree.
- `pkg/worker/adapter/codex_worker_sandbox.go`: Codex worker tasks get `--add-dir <git common dir>`, so a task can rewrite the repository configuration the pre-fix check then trusted.
- Consumers: only `pkg/worker` reads the report or the error (repository-wide search, plus `autopus/` and `autopus-desktop/` on 2026-11-02); the backend's `A2ATask` has no completion field.

## Outcome Lock

- User-visible outcome: a task can carry an explicit completion contract; every postcondition is `satisfied`, `unsatisfied`, `not_required`, or `unknown`; a push passes only on the exact ref's SHA at the URL recorded before execution; `unknown` fails the task as unverified; verification never writes; without a contract, negated actions are not required and no check succeeds without SHA evidence.
- Mandatory requirements: REQ-WPOST-SCOPE-01 through REQ-WPOST-VERIFY-01 in `spec.md`.
- Explicit non-goals: performing or retrying git writes, tags, judging commits HEAD does not end on, sending the contract from the backend, `pkg/pipeline`.
- Completion evidence: S1 to S12 in `acceptance.md`, the reproductions failing on `109c7ea4`, the whole `pkg/worker` package passing, the source-lines gate.

## Visual Planning Brief

`plan.md` carries the flow. In prose: the payload yields one plan (contract or fallback); the baseline pins HEAD and the remote URL; after the provider runs, each action gets one check through a read-only runner; the outcome order is unsatisfied, unknown, satisfied, not_required; only the last two complete the task.

## Probe Evidence (2026-11-02, git 2.50.1)

- Reproduction 1: `verifyExecutionPostconditions(repo, "Review only; do not commit or push", baseline)` on `109c7ea4` returned `postcondition failed: commit: HEAD did not advance after execution`.
- Reproduction 2: `feature/stale` pushed at `A`, local commit `B` not pushed, prompt `commit and git push origin feature/stale`: `109c7ea4` returned no error.
- Reproduction 3, found while wiring: with one queue slot held, a task with the prompt `commit your work` that commits nothing, while the main checkout gains a commit during the wait, returned no error on `109c7ea4` (baseline read before the wait, worktree created after it).
- Reproduction 4: a push only to `refs/heads/other/feature/x` satisfied the prompt `git push origin feature/x` on `109c7ea4`.
- `git rev-list --output=<file> -1 HEAD` creates the file (git 2.50.1), so a subcommand name alone does not make a command read-only; the allowlist fixes each form and refuses operands that start with `-`.
- Reproductions 3 and 4 ran as throwaway tests in a `git archive 109c7ea4` export in the session scratchpad, which was deleted afterwards.
- P1 to P3 of `plan.md` ran in scratch repositories with `GIT_CONFIG_GLOBAL=/dev/null` and `GIT_CONFIG_NOSYSTEM=1`.

## Trust Boundary

The contract arrives in the same unsigned payload as the prompt, so it is no more trusted than the prompt: it only states what the check demands and grants nothing, and the worker never performs the actions it names. Repository state after execution is task-controlled. The check therefore reads the remote URL before the provider runs and queries it from a neutral directory, so repository configuration written during the task (URL, `insteadOf`, credential helper, `core.sshCommand`) never reaches the lookup; it disables replace objects and distrusts ancestry when a graft file exists. Commit objects are content-addressed, so a locally present remote tip carries its true parents. Global configuration and the worker's own environment are trusted, as they are for every git command the worker runs.

## Design Decisions

- `exact` is the default push contract because it needs no local objects and no ancestry; `contains` is opt-in for shared branches where later pushes are expected.
- The expected commit defaults to HEAD on a branch: the task's result is the commit it ends on. A detached HEAD without `expected_commit` is `unknown`, because nothing then says which commit should be on the remote.
- `unknown` fails the task with its own prefix instead of a new A2A status: the transport has no unverified status, and failing keeps the brief's rule that unverifiable is not success while letting operators tell the two apart.
- An invalid contract fails before the provider runs: its result could never be judged, and running it would spend a model call and could publish work.
- The fallback keeps the exact pre-fix affirmative words and only removes negated ones, so it never requires more than before; a prompt that both asks for and declines an action is `ambiguous`, whose missing evidence is `unknown` rather than a demand to act.
- Forbidden checks judge only what the task leaves on HEAD; parallel workers share the repository, so new commits on other branches cannot be attributed. A forbidden commit is any forward move of HEAD, which keeps the rule local and deterministic. A forbidden push compares the remote heads with a snapshot taken at the baseline, because rebasing onto or pulling a commit the remote already held is common and is not a push.
- The baseline moved from the start of `executeWithParallel` to just before the provider starts, after the queue slot and the worktree: reading it earlier let a HEAD that moved in the main checkout while the task queued count as the task's commit.

## Technology Stack Decision

Go standard library and the `git` CLI the worker already requires. No new module. Version and source: the repository's `go.mod`; git 2.31 or later for `rev-parse --path-format`, checked 2026-11-02 against git 2.50.1.

## Minimality Decision Matrix

| Option | Chosen | Why |
|---|---|---|
| Versioned `completion_contract` object in the payload | yes | One key whose presence switches inference off; old workers ignore it |
| Flat top-level fields | no | Absent and empty lists become indistinguishable |
| New A2A status for unverified | no | Transport and backend change; the error prefix and report carry the state |
| Fetch before comparing | no | Writes objects and refs; exact comparison needs none |
| Drop the fallback | no | The backend sends no contract yet; every task would lose its checks |

## Semantic Invariant Inventory

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-P01 | unverifiable is not success | state mapping | task status, error prefix, outcome | S1, S2, S4, S5, S6, S7 |
| INV-P02 | no requirement from a negated or absent mention | inference monotonicity | planned levels | S1, S8 |
| INV-P03 | verification never writes | effect boundary | invoked subcommands, refs | S10 |
| INV-P04 | the push target is the exact ref at the pinned URL | target identity | push status, observed SHA | S2, S3, S5, S7 |
| INV-P05 | the contract is authoritative and validated first | precedence and rejection | planned levels, provider calls | S9 |
| INV-P06 | prompts without a mention keep the pre-fix silence | compatibility | artifact presence | S8 |
| INV-P07 | no credential in report or error | redaction | artifact, error text | S11 |
| INV-P08 | changes stay in owned paths | scope | branch diff | S12 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---|---|---|
| Contract input and rejection | T1, S9 | covered |
| Four states and outcome | T4, S1, S2, S11 | covered |
| Commit checks | T3, S6 | covered |
| Push exact and contains | T3, S2, S3, S4, S5 | covered |
| Forbidden push and branch | T3, S7 | covered |
| Read-only guarantee | T2, S10 | covered |
| Fallback inference | T2, S1, S8 | covered |
| Documentation | T6, S12 | covered |
| Backend sending the contract | outside this repository | non-goal |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

## Evolution Ideas

Optional improvements. They carry no identifiers and do not block completion.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| The backend fills the contract from the run's intent | Another repository; the worker accepts it today | A dispatcher knows the required actions |
| A repository identity field compared with the pinned URL | URL forms differ (ssh, https, insteadOf) | Workers serve several repositories per workspace |
| Tag refs and annotated-tag peeling | No task pushes tags today | A release task needs a tag postcondition |

## Sibling SPEC Decision

| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | One change story: the worker's completion judgement | None |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| `pkg/worker/postconditions.go`, `loop_exec.go`, `loop_task.go`, `operator_metadata.go`, `worktree_safety.go`, `parallel/worktree.go`, `adapter/codex_worker_sandbox.go`, `a2a/server_task_runtime.go` | existing | read on 2026-11-02 at `109c7ea4` |
| `autopus/backend/internal/symphony/worker_a2a.go`, `worker_a2a_route.go`, `scaffold.go` | existing, other repository | read-only on 2026-11-02 to confirm no contract is sent and how prompts phrase commits |
| git `ls-remote`, `merge-base --is-ancestor`, `rev-parse --git-path`, replace refs, graft file | external tool behaviour | probes P1 to P3 |
| files marked `[NEW]` in `spec.md` | planned additions | `go test ./pkg/worker/...` |

## Reviewer Brief

- Intended scope: R1 of the brief inside `pkg/worker`, plus its document.
- Self-verified: both reproductions, three probes, an invariant per outcome slice.
- Focus: whether any path turns a failed or skipped lookup into `satisfied`; whether the fallback can require something the pre-fix code did not; whether repository configuration written by the task can still steer the push lookup; whether the allowlist admits a writing form of an allowed subcommand.

## Residual Risks

- Forbidden actions are judged on HEAD only: a task that commits or pushes on another branch and returns HEAD to the baseline is not detected. A forbidden commit also rejects a fast-forward pull.
- The head snapshot is taken when the task's slot starts; a remote head that another actor moves during execution to a commit the task then pulls is blamed on the task, and a failed snapshot turns any such head into `unknown`.
- The fallback grammar is a heuristic. Conditional instructions (`if tests fail, do not push`) read as negations, and noun mentions (`push notifications`) still read as requests, as before this change; the contract is the remedy.
- Fallback compatibility: a prompt that names several branches while HEAD is on none of them (for example on a `worker-` branch) now gives an `unknown` push, where the pre-contract check passed whenever every named branch existed on `origin`. That pass was the defect this SPEC removes, but such tasks now fail as unverified until their sender adopts the contract.
- Global git configuration and askpass programs of the worker environment are trusted; an SSH remote may still wait on its own prompts until the timeout.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: every cited function and pattern was read at `109c7ea4`; git behaviour is backed by probes P1 to P3
- Q-CORR-02 | status: PASS | attempt: 1 | files: spec.md, plan.md | reason: new files carry `[NEW]` in Generated And Changed Surfaces and the tasks
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: Event-driven requirements use `WHEN ..., THEN`; Unwanted use `IF ..., THEN`; scenarios use bare Given/When/Then/And
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline separates existing code, the other repository, tool behaviour, and `[NEW]` files with their verification
- Q-COMP-01 | status: PASS | attempt: 1 | files: all four | reason: contract in spec, tasks and probes in plan, oracles in acceptance, evidence and decisions here
- Q-COMP-02 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: all 20 requirements appear in the matrix; each check has an unsatisfied and an unknown oracle
- Q-COMP-03 | status: PASS | attempt: 1 | files: spec.md | reason: states, outcome order, error prefixes, and report keys are named outputs
- Q-COMP-04 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Outcome Lock states the four states, the pinned target, the read-only rule, and the fallback limits
- Q-COMP-05 | status: PASS | attempt: 1 | files: research.md, spec.md, acceptance.md | reason: INV-P01 to INV-P08 map to requirements, tasks, and Must oracles with exact values
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Traceability Matrix carries the invariant column; Reviewer Brief names four focus areas
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: Completion Debt is empty; residual risks are stated properties; Evolution Ideas carry no identifiers
- Q-COMP-08 | status: PASS | attempt: 1 | files: plan.md | reason: three probe rows with PASS status and evidence
