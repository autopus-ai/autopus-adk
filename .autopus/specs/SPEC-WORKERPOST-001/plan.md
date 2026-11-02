# SPEC-WORKERPOST-001 Implementation Plan

Status: draft

## Tasks

- T1: `[NEW] pkg/worker/postconditions_contract.go` — `completionContractMessage`, requirement levels, the normalized plan (`taskPostconditions`), validation, the ported branch-name rule, redacted quoting; `[NEW] pkg/worker/postconditions_contract_decode.go` — strict decoding of the raw value (null, non-object, wrong types, unknown keys, every problem collected); `taskPayloadMessage.CompletionContract` (raw JSON) in `loop_task.go`, which rejects an invalid contract before any provider runs and stores the plan in `taskRunMeta`.
- T2: `[NEW] pkg/worker/postconditions_git.go` — the read-only git runner (exact-form allowlist, trusted-history options, `GIT_NO_LAZY_FETCH=1`, `GIT_ALLOW_PROTOCOL=none` for local commands, fixed locale, filtered environment, timeout, neutral directory for `ls-remote`, credential redaction); `[NEW] pkg/worker/postconditions_baseline.go` — lookup results, baseline capture, URL pinning with relative paths made absolute, exact branch lookup, history trust; `[NEW] pkg/worker/postconditions_legacy.go` — clause and token scan, negation cues, levels, branch-name extraction (the pre-fix patterns), fallback push target.
- T3: `[NEW] pkg/worker/postconditions_checks.go` — the commit and branch decision tables, fallback branch checks, trusted ancestry; `[NEW] pkg/worker/postconditions_push.go` — the push tables (`exact`, `contains`, forbidden over `ls-remote --heads` against the head snapshot and the trusted gained commits).
- T4: `pkg/worker/postconditions.go` — report version 2, outcome order, the two error sentinels, the ambiguous softening; `loop_exec.go` builds the plan, captures the baseline after worktree assignment, and verifies through `WorkerLoop.postconditionGit`; `operator_metadata.go` carries the plan; `loop.go` adds the seam.
- T5: tests — the reproductions first (they fail on `109c7ea4`), then S1 to S11 in `pkg/worker/postconditions_*_test.go` with a shared hermetic fixture (digests of git directories, a commit-graph rewriter, a configuration breaker, a partial clone); existing postcondition tests move to the new status words without losing an assertion.
- T6: `[NEW] docs/worker-completion-contract.md`; the comment in `pkg/worker/adapter/codex_worker_sandbox.go` that says every worker task must commit.
- T7: `go test ./pkg/worker/...`, `go vet ./pkg/worker/...`, the source-lines gate over the changed Go files, and the S12 path check.

## Implementation Strategy

The plan is computed once per task in `handleTask`: a contract yields levels, remote, ref, expected commit, and match with no prompt inference; no contract yields the fallback levels from the prompt. The plan rides in `taskRunMeta`, so both execution paths and the existing direct callers keep their signatures; a zero `taskRunMeta` falls back to inference from the prompt, which is the pre-fix behaviour for prompts without negations.

Every git command goes through one runner that refuses anything outside the read-only allowlist before starting a process and that reads history from commit objects only: replace refs and the commit-graph are off, lazy fetch is off twice over, and a graft file or a shallow boundary downgrades the answers they could falsify. Every lookup ends present, absent, or failed, and each check walks an ordered decision table in which failed lookups match before any row that needs absence. The baseline is read in the directory the provider runs in, after the worktree is assigned and the queue slot taken; the pre-contract code read it before both, so a HEAD that moved in the main checkout while a task queued could pass as the task's own commit. The baseline records the remote URL through `git remote get-url` before the provider runs; push lookups pass that URL to `git ls-remote` in a neutral directory, so configuration the task changes (a new URL, an `insteadOf` rewrite, a credential helper) cannot redirect the lookup. Exact comparison needs no local objects. `contains` uses local ancestry only after the remote tip is found locally. A relative remote path is pinned against the work tree's top level, as git itself resolves it.

A forbidden push is blamed only on a remote head that moved during execution to a commit HEAD gained; the baseline snapshots the remote heads for that, so a commit the task pulled is not taken for a push. `unknown` maps to a failed task with its own error prefix, never to completion. The fallback keeps the exact affirmative words of the pre-fix regular expressions, so it can only lose requirements the prompt negates, never gain new ones.

## Visual Planning Brief

```mermaid
flowchart TD
    A["task payload"] --> B{"completion_contract?"}
    B -->|present| C["validate + normalize"]
    C -->|invalid| D["fail task: invalid completion_contract (no provider run)"]
    C -->|valid| E["plan: levels, remote, ref, expected, match"]
    B -->|absent| F["fallback: clause/token scan, negation cues"] --> E
    E --> G["baseline: HEAD, pinned remote URL, ref existed"]
    G --> H["provider runs"]
    H --> I["checks via read-only runner"]
    I --> J["commit: HEAD advanced by trusted ancestry"]
    I --> K["push: ls-remote pinned URL in neutral dir, exact ref SHA vs expected"]
    I --> L["branch: local ref vs baseline"]
    J & K & L --> M{"outcome"}
    M -->|unsatisfied| N["postcondition failed: ..."]
    M -->|unknown| O["postcondition unverified: ..."]
    M -->|satisfied / not_required| P["task completed"]
```

## Feature Completion Scope

This single SPEC closes the Outcome Lock: T1 gives the input contract and its rejection, T2 the read-only runner, pinned baseline, and the fallback, T3 the four-state checks, T4 the report, errors, and wiring, T5 the evidence for every Must requirement, T6 the documentation. Sending the contract from the backend is outside this repository and outside the Outcome Lock; until a sender adopts it, every task takes the fallback, which this SPEC also fixes. No Completion Debt.

## Risk-First Integration Probe

| assumption_id | class | risk | boundary | input | oracle | isolation | status | reason | evidence |
|---------------|-------|------|----------|-------|--------|-----------|--------|--------|----------|
| P1 | implementation_assumption | high | `git ls-remote` target selection | bare remote with `refs/heads/feature/x` and `refs/heads/other/feature/x`; repository-level `url.<fake>.insteadOf <bare>` | pattern `feature/x` lists both refs; `ls-remote -- <bare URL> refs/heads/feature/x` from a fresh directory under `GIT_CEILING_DIRECTORIES` lists one line and ignores the rewrite that `git remote get-url origin` applies inside the repository | scratch repositories, `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_NOSYSTEM=1` | PASS | the pre-fix pattern lookup can match a suffix ref, and only a lookup outside the repository is immune to configuration the task can change | scratch probe 2026-11-02, git 2.50.1 (Apple Git-155); kept as `TestPushContract_SuffixRefAndRepointedRemoteFail` |
| P2 | implementation_assumption | high | trusted history | orphan `U`, commit `C2`: a `git replace --graft U C2` ref; an `info/grafts` line `U C2`; a commit-graph patched so `A0` lists `U` as parent; a blob-less partial clone missing a pushed tip | replace ref fools plain `merge-base` and not `--no-replace-objects`; the graft file fools both; the patched graph makes `rev-list U ^A0` empty and `-c core.commitGraph=false` restores `U`; `cat-file -e` fetches the missing tip unless `GIT_NO_LAZY_FETCH=1` or `GIT_ALLOW_PROTOCOL=none` is set | scratch repositories | PASS | replace refs, the commit-graph, and lazy fetch are switched off per command; a graft file cannot be, so its presence makes history untrusted | scratch probes 2026-11-02, git 2.50.1; kept as `TestPushContract_ContainsNeedsTrustedAncestry`, `TestTrust_CommitGraphCannotHideAGainedCommit`, `TestTrust_LazyFetchIsBlocked` |
| P3 | implementation_assumption | medium | authentication failure | `ls-remote` of `http://user:secret-token@127.0.0.1:<port>/r.git` against a server answering 401, `GIT_TERMINAL_PROMPT=0`, no credential helper | exit 128 within one second, stderr `fatal: Authentication failed for 'http://127.0.0.1:<port>/r.git/'` with the userinfo removed | local HTTP server, scratch directory | PASS | an auth failure ends promptly as a lookup error, which the check maps to `unknown`; git already strips the userinfo, and the redaction covers other messages | scratch probe 2026-11-02, git 2.50.1; kept as `TestPushContract_LookupFailuresAreUnknown` |

## Verification

```sh
go test -timeout 40m -p 2 ./pkg/worker/...
go vet ./pkg/worker/...
go run ./cmd/source-lines --max 300 --ext .go $(git diff --name-only origin/main -- '*.go')
git diff --name-only origin/main
```
