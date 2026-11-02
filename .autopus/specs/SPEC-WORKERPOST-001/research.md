# SPEC-WORKERPOST-001 Research

## Existing Code Analysis

- `pkg/worker/postconditions.go` (`109c7ea4`): `detectTaskPostconditions` sets `CommitRequired`, `PushRequired`, and `BranchRequired` from the regular expressions `\bgit\s+commit\b|\bcommit\b`, `\bgit\s+push\b|\bpush\b`, and `\bbranch\b`, with no notion of negation. Branch names come from `refs/heads/<name>`, `branch [named|called] <name>`, and `origin[/ ]<name>`, where the last two require a `/` or `-` in the name. The commit check compares HEAD with the baseline SHA only. The push check runs `git ls-remote --exit-code --heads origin <branch>` and passes on exit 0, so it proves only that some ref ending in `<branch>` exists. Statuses are `passed` and `failed`; a failed git command is `failed`, never distinguished from a negative answer.
- `pkg/worker/loop_exec.go`: `executeWithParallel` captured the baseline before the queue slot and the worktree; both execution paths verify after a successful provider run; an error fails the task and the report is appended as `postconditions.json`.
- `pkg/worker/loop_task.go`: `taskPayloadMessage` is decoded from the payload that `pkg/worker/a2a` merges with signed control-plane metadata; unknown keys are ignored by `encoding/json`, so an older worker ignores a contract it does not know.
- `pkg/worker/parallel/worktree.go`: a worker worktree sits on `worker-<task id>`; `branchesForVerification` excluded such branches, so a push prompt without a named branch failed in every worktree.
- `pkg/worker/adapter/codex_worker_sandbox.go`: Codex worker tasks get `--add-dir <git common dir>`, so a task can rewrite configuration, objects, refs, and history files of the repository the check reads.
- Consumers: only `pkg/worker` reads the report or the error (repository-wide search, plus `autopus/` and `autopus-desktop/` on 2026-11-02); the backend's `A2ATask` has no completion field.

## Outcome Lock

- User-visible outcome: a task can carry an explicit completion contract; every postcondition is `satisfied`, `unsatisfied`, `not_required`, or `unknown` by the first matching row of its decision table; a push passes only on the exact ref's SHA at the URL pinned before execution; ancestry comes only from trusted history; a failed lookup is `unknown`, never absence; `unknown` fails the task as unverified; verification writes nothing and fetches nothing; without a contract, negated actions are not required and no check succeeds without SHA evidence.
- Mandatory requirements: REQ-WPOST-SCOPE-01 through REQ-WPOST-VERIFY-01 in `spec.md`.
- Explicit non-goals: performing or retrying git writes, tags, judging commits HEAD does not end on, sending the contract from the backend, `pkg/pipeline`.
- Completion evidence: S1 to S12 in `acceptance.md`, the reproductions failing on `109c7ea4`, the whole `pkg/worker` package passing, the source-lines gate.

## Visual Planning Brief

`plan.md` carries the flow. In prose: the payload yields one plan (contract or fallback); the baseline records lookup results for HEAD, the pinned remote URL, and the target branch; after the provider runs, each action walks its decision table through a read-only runner that reads trusted history only; the outcome order is unsatisfied, unknown, satisfied, not_required; only the last two complete the task.

## Probe Evidence (2026-11-02, git 2.50.1)

- Reproduction 1: `Review only; do not commit or push` on `109c7ea4` returned `postcondition failed: commit: HEAD did not advance after execution`.
- Reproduction 2: `feature/stale` pushed at `A`, local commit `B` not pushed, prompt `commit and git push origin feature/stale`: `109c7ea4` returned no error.
- Reproduction 3: with one queue slot held, a task with the prompt `commit your work` that commits nothing, while the main checkout gains a commit during the wait, returned no error on `109c7ea4`.
- Reproduction 4: a push only to `refs/heads/other/feature/x` satisfied the prompt `git push origin feature/x` on `109c7ea4`. Reproductions 3 and 4 ran as throwaway tests in a `git archive 109c7ea4` export in the session scratchpad, deleted afterwards.
- Lazy fetch: in a blob-less partial clone, `git cat-file -e <missing commit>` fetched it from the promisor (objects 4 to 8, exit 0); `GIT_NO_LAZY_FETCH=1`, `--no-lazy-fetch`, and `GIT_ALLOW_PROTOCOL=none` each left it missing (exit 1, objects unchanged).
- Commit-graph: after patching a `commitGraph.generationVersion=1` graph so that `A0` lists orphan `U` as its parent, `git rev-list U ^A0` printed nothing; with `-c core.commitGraph=false` it printed `U`. `merge-base --is-ancestor` was not fooled in that probe, but the option is applied to every command.
- Relative remote: `origin` set to `../remote.git` resolves against the work tree's top level (a subdirectory works, a linked worktree resolves it against its own top).
- `git rev-parse --verify -q refs/heads/ghost` printed the SHA of `refs/tags/refs/heads/ghost` (DWIM expansion); `for-each-ref` with the exact name lists nothing.
- Lookup exits: unborn `rev-parse --verify -q HEAD` 1; missing `remote get-url` 2; missing `cat-file -e` 1; not a repository 128 with `not a git repository`.
- `git rev-list --output=<file> -1 HEAD` creates the file, so a subcommand name alone does not make a command read-only.
- `git check-ref-format --branch` accepts `feature+fix`, non-ASCII names, `@`, and `a./b`, and rejects `feature.lock/x`, `-dash`, and `HEAD`.

## Trust Boundary

The contract arrives in the same unsigned payload as the prompt, so it is no more trusted than the prompt: it states what the check demands and grants nothing, and the worker never performs the actions it names. Everything in the repository after execution is task-controlled: configuration, objects, refs, replace refs, graft and shallow files, the commit-graph, and promisor settings. The check therefore pins the remote URL before the provider runs and queries it from a neutral directory, so configuration written during the task (URL, `insteadOf`, credential helper, `core.sshCommand`) never reaches the lookup. It reads parents only from commit objects (replace objects and the commit-graph off), distrusts all history while a graft file exists and negative answers in a shallow repository, and never fetches, so a promisor setting cannot reach the network. A commit object is content-addressed, so a locally present tip carries its true parents. Global configuration and the worker's own environment are trusted, as they are for every git command the worker runs.

## Design Decisions

- `exact` is the default push contract because it needs no local objects and no ancestry; `contains` is opt-in for shared branches where later pushes are expected.
- The expected commit defaults to HEAD on a branch: the task's result is the commit it ends on. A detached HEAD without `expected_commit` is `unknown`.
- `unknown` fails the task with its own prefix instead of a new A2A status: the transport has no unverified status, and failing keeps the brief's rule that unverifiable is not success.
- An invalid contract fails before the provider runs. Only an absent key selects the fallback: null, a non-object, a wrong field type, and an unknown key are invalid, because a misspelt key would otherwise drop a requirement silently.
- Branch names follow `git check-ref-format --branch`, ported to Go and cross-checked against git, rather than a narrower pattern that refused valid names.
- Lazy fetch is blocked twice: `GIT_NO_LAZY_FETCH=1` (git 2.44+) and, for local commands, `GIT_ALLOW_PROTOCOL=none`, which older git honours; `--no-lazy-fetch` would fail every command on git older than 2.44.
- Every check is an ordered decision table: overlapping conditions get one result, failed lookups come first, and an appearing remote is `unknown` before an unchanged HEAD can make a forbidden push `satisfied`.
- The fallback keeps the exact pre-fix affirmative words and only removes negated ones; an `ambiguous` action with no evidence is `unknown`, not a demand to act.
- Forbidden checks judge only what the task leaves on HEAD. A forbidden commit is any forward move of HEAD. A forbidden push compares the remote heads with a baseline snapshot, so a commit the task pulled is not taken for a push.
- The baseline is read just before the provider starts, after the queue slot and the worktree (reproduction 3).
- Local reads and the remote lookup have separate timeouts (30 s and 15 s). One 5 s limit for both let a slow local read on a loaded machine pass for a failed lookup; either timeout is still a failed lookup.

## Technology Stack Decision

Go standard library and the `git` CLI the worker already requires. No new module. Version and source: the repository's `go.mod`; git 2.31 or later for `rev-parse --path-format` (2.44 or later for `GIT_NO_LAZY_FETCH`, with `GIT_ALLOW_PROTOCOL` covering older git), checked 2026-11-02 against git 2.50.1.

## Minimality Decision Matrix

| Option | Chosen | Why |
|---|---|---|
| Versioned `completion_contract` object in the payload | yes | One key whose presence switches inference off; old workers ignore it |
| Flat top-level fields | no | Absent and empty lists become indistinguishable |
| New A2A status for unverified | no | Transport and backend change; the error prefix and report carry the state |
| Fetch before comparing | no | Writes objects and refs; exact comparison needs none |
| Call `git check-ref-format` per contract | no | A process per task for a rule that ports in twenty lines and is cross-checked in tests |
| Drop the fallback | no | The backend sends no contract yet; every task would lose its checks |

## Semantic Invariant Inventory

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-P01 | unverifiable is not success; a failed lookup is not absence | state mapping | task status, error prefix, outcome | S1, S2, S4, S5, S6, S7 |
| INV-P02 | no requirement from a negated or absent mention | inference monotonicity | planned levels | S1, S8 |
| INV-P03 | verification never writes or fetches | effect boundary | invoked commands, git directory digests | S10 |
| INV-P04 | the push target is the exact ref at the pinned URL | target identity | push status, observed SHA | S2, S3, S5, S7 |
| INV-P05 | the contract is authoritative and validated first | precedence and rejection | planned levels, provider calls | S9 |
| INV-P06 | prompts without a mention keep the pre-fix silence | compatibility | artifact presence | S8 |
| INV-P07 | no credential in report, error, or event | redaction | artifact, error text, host events | S9, S11 |
| INV-P08 | changes stay in owned paths | scope | branch diff | S12 |
| INV-P09 | ancestry comes only from trusted history | trust boundary | commit and push statuses | S5, S7 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---|---|---|
| Contract input, decoding, and rejection | T1, S9 | covered |
| Four states and outcome | T4, S1, S2, S11 | covered |
| Lookups, pinning, trusted history | T2, S3, S4, S5, S6, S7 | covered |
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
| git `ls-remote`, `merge-base --is-ancestor`, `rev-list`, `rev-parse`, `for-each-ref`, `check-ref-format`, replace refs, graft and shallow files, commit-graph, partial clone | external tool behaviour | probes in Probe Evidence and P1 to P3 |
| files marked `[NEW]` in `spec.md` | planned additions | `go test ./pkg/worker/...` |

## Reviewer Brief

- Intended scope: R1 of the brief inside `pkg/worker`, plus its document.
- Self-verified: four reproductions, the probes above, a mutation check that disabling each trust defence fails its test, an invariant per outcome slice.
- Focus: whether any decision table row turns a failed lookup into `satisfied`; whether any task-writable file (configuration, objects, history files, promisor settings) can still steer a lookup or an ancestry answer; whether the fallback can require something the pre-fix code did not; whether the allowlist admits a writing or fetching form.

## Residual Risks

- Forbidden actions are judged on HEAD only: a task that commits or pushes on another branch and returns HEAD to the baseline is not detected. A forbidden commit also rejects a fast-forward pull.
- The head snapshot is taken when the task's slot starts; a remote head that another actor moves during execution to a commit the task then pulls is blamed on the task.
- The fallback grammar is a heuristic: conditional instructions read as negations and noun mentions (`push notifications`) still read as requests; a prompt naming several branches while HEAD is on none of them now gives an `unknown` push where the pre-contract check passed.
- A git that reports "not a repository" in another wording would turn an absent HEAD into `unknown` (fail closed); `LC_ALL=C` keeps the wording stable.
- Global git configuration and askpass programs of the worker environment are trusted; an SSH remote may wait on its own prompts until the timeout.

## Review Response (codex round 1, 2026-11-02, judge REVISE, 12 accepted)

| Finding | Resolution |
|---|---|
| WPOST-R01 lazy fetch | `GIT_NO_LAZY_FETCH=1` and `GIT_ALLOW_PROTOCOL=none` for local commands; REQ-WPOST-PUSH-03, READONLY-01; partial-clone oracle in S5/S10 |
| WPOST-R02 forbidden-push rev-list trust | Gained commits only under trusted history; graft gives `unknown`, shallow match gives `unknown` (REQ-WPOST-HISTORY-01, Q11 to Q13); S7 rows |
| WPOST-R03 commit-graph | `-c core.commitGraph=false` on every command; Trusted history definition; commit-graph oracle in S7 |
| WPOST-R04 credentials in contract errors | Values are redacted before clipping and quoting (REQ-WPOST-INPUT-02, REPORT-01); S9 checks the error, task result, and host events |
| WPOST-R05 absent vs failed | Lookup result definition and REQ-WPOST-LOOKUP-01; failed baseline or final lookups map to `unknown` rows; S4, S6, S7 |
| WPOST-R06 relative remote | REQ-WPOST-PIN-01 pins a relative path against the work tree's top level; S3 |
| WPOST-R07 null and wrong types | Contract definition: only an absent key falls back; strict decoding with every problem reported; S9 |
| WPOST-R08 branch names | Branch name definition ports `check-ref-format --branch`; S9 cross-checks 35 names against git |
| WPOST-R09 precedence | Decision Tables with first-match rows; REQ-WPOST-COMMIT/PUSH/BRANCH point at them; S7 overlap rows |
| WPOST-R10 missing oracles | S5 shallow, S7 unknown baseline and overlaps, S9 aggregation, S10 digests, S11 mixed outcome |
| WPOST-R11 self-verify dimensions | Q-FEAS, Q-STYLE, Q-SEC, Q-COH lines below |
| WPOST-R12 BRANCH-01 parsed as Optional | Split into REQ-WPOST-BRANCH-01 and -02; every requirement's parsed EARS type checked against its declared type |

## Review Response (codex round 2, 2026-11-02, judge REVISE, 2 residual)

| Finding | Resolution |
|---|---|
| R04 residual: a user part without a password (`secret-token@host:repo.git`, `TOKEN@github.com/org/repo.git`) leaked | Every user part before `@` of a scheme-less or scp-like address is redacted before clipping, `git@host:` included (REQ-WPOST-REPORT-01); S9 checks the error, task result, and host events |
| R07 residual: list decoding stopped at the first non-string element | Every element is decoded; each non-string one is named by index and the strings still reach the action checks (REQ-WPOST-INPUT-02); S9 row with `["deploy","push",3,false]` |

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: every cited function and pattern was read at `109c7ea4`; git behaviour is backed by the probes and P1 to P3
- Q-CORR-02 | status: PASS | attempt: 2 | files: spec.md, plan.md | reason: new files carry `[NEW]` in Generated And Changed Surfaces and the tasks
- Q-CORR-03 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: `ParseEARS` assigns each of the 24 requirement sentences its declared type, and no other line parses as a requirement
- Q-CORR-04 | status: PASS | attempt: 2 | files: research.md | reason: Reference Discipline separates existing code, the other repository, tool behaviour, and `[NEW]` files
- Q-COMP-01 | status: PASS | attempt: 2 | files: all four | reason: contract and tables in spec, tasks and probes in plan, oracles in acceptance, evidence, decisions, and review response here
- Q-COMP-02 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: all 24 requirements appear in the matrix; every table row with `unknown` has a failure or trust oracle
- Q-COMP-03 | status: PASS | attempt: 2 | files: spec.md | reason: decision tables give one status per state; outcome order, error prefixes, and report keys are named outputs
- Q-COMP-04 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: Outcome Lock states the tables, the pinned target, trusted history, failed lookups, the read-only rule, and the fallback limits
- Q-COMP-05 | status: PASS | attempt: 2 | files: research.md, spec.md, acceptance.md | reason: INV-P01 to INV-P09 map to requirements, tasks, and Must oracles with exact values
- Q-COMP-06 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: Traceability Matrix carries the invariant column; Reviewer Brief names four focus areas
- Q-COMP-07 | status: PASS | attempt: 2 | files: research.md | reason: Completion Debt is empty; residual risks are stated properties; Evolution Ideas carry no identifiers
- Q-COMP-08 | status: PASS | attempt: 2 | files: plan.md | reason: three probe rows with PASS status and evidence
- Q-FEAS-01 | status: PASS | attempt: 2 | files: plan.md | reason: payload decoding, execution wiring, and the checks all live in `pkg/worker`, the layer that owns completion
- Q-FEAS-02 | status: PASS | attempt: 2 | files: spec.md, plan.md | reason: ownership is `pkg/worker`, the module document, and this SPEC; generated roots and other modules are excluded
- Q-FEAS-03 | status: PASS | attempt: 2 | files: plan.md | reason: the verification commands match the repository layout and ran on this branch
- Q-STYLE-01 | status: PASS | attempt: 2 | files: spec.md | reason: no should, might, could, possibly, maybe, or perhaps in requirement text; every status condition sits in exactly one table row
- Q-STYLE-02 | status: PASS | attempt: 2 | files: spec.md | reason: Priority and Type are separate lines; Must only
- Q-STYLE-03 | status: PASS | attempt: 2 | files: acceptance.md | reason: bare Given/When/Then/And steps; tables sit between When and Then
- Q-SEC-01 | status: PASS | attempt: 2 | files: research.md, spec.md | reason: Trust Boundary covers task-writable configuration, objects, history files, and promisor settings; each has a requirement and an oracle
- Q-SEC-02 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: exact command forms, no fetch, digest-proven absence of writes, credentials redacted before any value is quoted
- Q-SEC-03 | status: PASS | attempt: 2 | files: spec.md | reason: the report names remotes, never URLs; contract errors reach the task result and host events only redacted
- Q-COH-01 | status: PASS | attempt: 2 | files: spec.md | reason: one problem: the worker's completion judgement
- Q-COH-02 | status: PASS | attempt: 2 | files: research.md | reason: review findings were fixed inside this SPEC; follow-ups are Evolution Ideas without identifiers
- Q-COH-03 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: no sibling SPEC
