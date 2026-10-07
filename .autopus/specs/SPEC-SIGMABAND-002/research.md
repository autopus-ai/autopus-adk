# SPEC-SIGMABAND-002 리서치

## Decision Record

- User decision D3 (2026-10-06, AskUserQuestion): the 3σ autonomy cap was a draft PR on a separate branch behind a default-OFF flag.
- User decision (2026-10-06, AskUserQuestion): the 3σ path was split from SPEC-SIGMABAND-001, which treats tier 3 as diagnosis-only.
- User decision (2026-10-06, AskUserQuestion): the 3σ cap changed from "draft PR" to "local patch only"; band never pushes, never creates a PR, and never calls a remote write API; the human reviews and pushes manually. D3 now reads "local patch".
- Operator restatement (2026-10-08, refresh hand-off): 3σ produces a local patch on an isolated worktree only, no push and no PR, behind a flag that defaults to OFF. Rev 6 refreshes the draft against the merged code; no new user decision was taken.

## 기존 코드 분석

SPEC-SIGMABAND-001 is merged (main `1943e596`); rev 6 read its code instead of its plan:

- Phase structure (`internal/cli/react_band.go:139-255`): `guardStore`, `fetchCI`, then `--dry-run` `planDry` without a lock, or `store.Lock`, `phaseA` (`OpenWAL`, merge, `ReadSeries`, `Plan`, `Commit`), `Unlock`, and `phaseB` (`ExecuteClaims` with the diagnoser as runner).
- Claims and leases: `Plan` (`pkg/healthband/catchup.go:73-111`) chains `ClaimBudget(ClaimKindDiagnose)` only (`:100-101`); `claimBudgets` (`episode.go:21-27`) is consulted for that kind alone; `DiagnoseBudget` = 930 s (`:12-19`); claim ids are 32 hex (`claims.go:44`, `wal_validate.go:14`); `Episode` (`types.go:175-181`) keeps `max_tier`, not the opening tier; `ExecuteClaims` (`claims.go:94-117`) has no hook after `RecordResult`.
- Diagnoser (`react_band_diagnose.go`): one `projectDir` is both the provider cwd and the BS directory (`:72-86`, `:253-276`); `bandProviderUnsetEnv` strips GitHub, Actions OIDC and runtime, AWS (`AWS_CONTAINER_*`, Bedrock), Google Cloud, and Azure credentials (`:54-62`); the capture is bounded at `ProviderCaptureBytes` + 1 (`sanitize.go:16`); `bandReadOnlyControls` checks the claude controls (`:215-233`).
- git and gh: `checkBandCommand` admits only `git remote get-url origin`, the tracked-store `ls-files`, and four gh rows (`react_band_gh.go:90-113`); the default branch stays a local variable of `fetchCI` (`react_band_ingest.go:83-111`).
- BS: `render.go:180,196,208-209` hold diagnosis-only sentences; `validate.go:26-27` checks sections only; prompts use `EvaluationLayer` (`prompt.go:113`) and an unexported `evidenceLayer` with a fixed `untrusted-evidence` fence (`:164`).
- Cross-SPEC: SPEC-REVIEWRO-001's projection already appends `--strict-mcp-config`, `--safe-mode`, and `--tools=Read,Grep,Glob` (`orchestra_readonly_policy.go:209-217`); SPEC-PANERM-001 left subprocess and OMP backends only (no `SubprocessMode` in `pkg`, `internal`, `cmd`); SPEC-EDITGUARD-001 guards `Edit|Write|MultiEdit` tool calls and documents that a fresh worktree has no manifests (`docs/edit-guard.md:92`).

## Outcome Lock

- User-visible outcome: with `health_band.allow_local_patch: true`, every SPEC-SIGMABAND-001 episode opened at tier 3 with an ok confined diagnosis gets at most one local patch (exactly one when every guard passes): branch `autopus/band/<key>`, worktree `<lp>/<key>/worktree/`, patch file `<lp>/<key>.patch` with `<lp>` = `<UserCacheDir>/autopus/local-patches/<repo-hash>` outside the repository, a pointer in the 3σ BS, nothing remote, and no repository-selected command run by band.
- Mandatory requirements: REQ-01–REQ-14.
- Explicit non-goals: push, fetch, PR, remote refs, GitHub write APIs, CI, automatic tests or builds, agent write access, patches for episodes that reached tier 3 after a tier-2 BS, OMP backend confinement, reviewer-side execution (warned in the BS), anything SPEC-SIGMABAND-001 owns.
- Completion evidence: S1–S12 pass, Completion Debt CD-1–CD-3 resolved, security-auditor review passed, 0 remote writes in every run, and no file under the repository root outside `.git/` changes except the BS and `.autopus/metrics/` records, with only the REQ-14 changes inside `.git/`.

## Visual Planning Brief

`wireframe intent: not applicable` (CLI only). Decision flow and the step order are in `plan.md`; data flow:

```mermaid
flowchart LR
  E1["001 tier-3 opening"] --> DL[(localpatch-events.jsonl)] --> WT["band worktree at base SHA under the user cache dir (hooks off)"]
  WT --> DX["confined diagnosis -> BS with pointer lines"] --> PQ["confined patch request"] --> GD["Patch Policy + editguard.Decide + lore.Validate"]
  GD --> LP["local branch + .patch file in the user cache dir"] --> HR["human review and manual push"]
```

## 설계 결정

- Rescope (user decision 2026-10-06): removing publication removes the remote trust problems; the remaining boundary is agent-authored code placed in the user's repository.
- Confinement (F-039, PANERM): only a subprocess claude can be confined (`--restricted` is argv); an OMP-backed provider is reported unconfined instead of silently running unconfined.
- No repository-selected command (F-043, F-071, F-075): attribute sources off or empty, drivers refused outside the git-lfs token rule, LFS downloads and extensions off, no network command, and an own git allowlist so 001's mutation-free `checkBandCommand` stays as it is.
- Location (F-070): worktree and patch file live in the user cache directory; the BS keeps only a pointer.
- Binding (F-049, F-050, F-067, F-073, F-074): the claim depends on the diagnose claim; preparation codes come first; first-match decision order with a default row; the Opening tier rule replaces an opening tier 001 does not store.
- Recovery (F-068, F-080, F-081, F-082, F-083, F-084): every artifact is named by a write-ahead intent record; the claim commit is the recorded OID or parent, message hash, and expected tree; one Cleanup Rules set (order 3, 1, 2) keeps anything a user changed and any partial checkout band cannot prove; diagnoses without a claim are recovered too; recovery never runs under `--dry-run`.
- Ownership (F-072): plan task T8 owns the edits in 001's files; 001's `Plan` needs a hook, because registering a budget alone does not chain it.
- Edit guard (EDITGUARD): band's writes bypass the pre-tool hook by construction, so the Patch Policy asks the guard's own decision function against the user's checkout instead of assuming the guard applies.
- Budgets (F-032, F-072): diagnose 990 s (930 + 30 setup + 30 diagnosis-only cleanup), local_patch 810 s; appends wait at most 5 s inside group deadlines; a claim stops before a group its lease cannot cover.

## Minimality Decision Matrix

| Ladder step | Evidence | Decision | Receipt item |
|-------------|----------|----------|--------------|
| actual need | D3 as a local patch (Decision Record); 001 stops at diagnosis (`render.go:180`) | proceed | one local patch per tier-3 opening |
| existing code/helper/pattern | 001 store helpers, `Plan`, `ExecuteClaims`, `bandProviderUnsetEnv`, `Sanitize`, `EvaluationLayer`, `Fence`, `H8`, `applyReadOnlyProviderPolicy`, `editguard.Decide`, `lore.BuildCommit`/`Validate`, `orchestra.EnvironWithout`, `filelock.Acquire` | reuse | no second store, sanitizer, projection, or guard |
| stdlib/native | `os.UserCacheDir`, `crypto/sha256`, `crypto/rand`, git worktree, apply, update-ref, format-patch | use | artifacts, hashes, nonce, isolation |
| existing dependency | no new module; git and claude CLIs already required | reuse | none added |
| new dependency or abstraction / new dependency or new abstraction | two hooks in 001 (`PlanOptions.LocalPatch`, `ExecuteOptions.AfterRecord`), one projection option (`Confined`), seven `[NEW]` files | accepted | hooks keep 001 free of local-patch logic |
| minimum sufficient verification | S1–S12 with real git in temp repos, probes A1 and A3, security-auditor review, strict validate | required checks | `plan.md` Verification |

## Semantic Invariant Inventory

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-01 | "3σ (flag default OFF)" | state transition / default | local-patch files, git argv, config text, docs | S1, S10, S11 |
| INV-02 | one local patch per tier-3-opened episode, only after an ok diagnosis, bound to its BS ID | state transition / ordering | decision, claim, result records, leases | S2, S3, S9 |
| INV-03 | checkout, apply, commit, and format-patch run no repository-tracked code | guard / containment | trace2 events, marker files, index hash | S4, S6 |
| INV-04 | never push, fetch, PR, or remote write | containment | git and gh recorders, bare remote | S12 |
| INV-05 | Patch Policy over the raw diff, including dotenv and credential paths | parser / guard | guard codes, object counts | S5 |
| INV-06 | diagnosis and patch providers confined to the band worktree | containment / layer classification | provider argv, cwd, environment, manifest | S7, S8 |
| INV-07 | artifacts stay outside the repository and are pointed to from the BS | location / record | repository file hashes, branch, worktree, patch file, BS lines | S4, S11 |
| INV-08 | the commit message equals the validated `-F` bytes | integrity | commit object message, Lore codes | S4, S5 |
| INV-09 | a failure removes what the claim created unless a Cleanup Rule keeps it; recovery uses only durable records | cleanup / recovery | worktree, branch, patch file, result records, `kept[]` | S5, S9 |
| INV-10 | a path the edit guard denies in the user's checkout is denied in a patch | guard / parity | `path_denied:<class>` codes | S5 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| Flag, migration, 001 amendment | REQ-01 / T1 / S1, S10 | covered |
| Decisions, own WAL, BS binding, leases, recovery | REQ-02, REQ-04, REQ-11, REQ-12 / T2, T8 / S2, S3, S9 | covered |
| Happy path local patch outside the repository | REQ-05, REQ-06, REQ-09, REQ-14 / T7 / S4 | covered |
| Patch Policy, edit guard parity, failure cleanup | REQ-08, REQ-11 / T3, T7 / S5 | covered |
| Git Execution Policy | REQ-05 / T4 / S6 | completion-debt (CD-1) |
| Provider confinement and prompt | REQ-03, REQ-07 / T5, T6 / S7, S8 | completion-debt (CD-2) |
| Never publish | REQ-10 / T7, T10 / S12 | covered |
| Docs, BS warning | REQ-13 / T9 / S11 | covered |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| CD-1 | INV-03, S6, approval | run probe A3 against the Git Execution Policy and the Local Patch Flow git mechanics (attribute sources, filter, diff, merge, and LFS settings with a real git-lfs, inherited `GIT_*` variables, the temp-index expected tree, an interrupted worktree add) and fix every gap it shows |
| CD-2 | INV-06, S7, approval | run probe A1: subprocess claude with the shared projection plus `--restricted`, no read outside the worktree, no command or network tool, one diff fence |
| CD-3 | approval and sync | security-auditor review of the local boundary |

The earlier CI-scan, pagination, and ruleset items were only about remote publication and were dropped with the rescope.

## Open Questions

| ID | Question | If unanswered |
|----|----------|---------------|
| OQ-1 | This repository's `autopus.yaml:77-90` sets `backend: omp` for every provider, and only a subprocess claude can be confined, so `allow_local_patch: true` here gives `unavailable(provider_unconfined)` and no local patch. Should the operator switch `orchestra.providers.claude` to the subprocess backend for band, should a later SPEC add worktree confinement to the OMP backend, or should band get its own provider entry? | the flag stays usable only in projects with a subprocess claude; rev 6 records it as a non-goal and a docs sentence (REQ-13), not as a requirement |

## Evolution Ideas

These are optional improvements and do not block sync completion.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| local patches for episodes that escalate from tier 2 to tier 3 | tier-3 openings keep the BS binding simple | operators see escalations that need patches |
| optional publication as a draft PR | the user capped the 3σ path at a local patch | the user lifts the cap |
| running the proposal's tests in a sandbox | no automatic execution is the current boundary | a sandbox with no network and no secrets exists |

## Sibling SPEC Decision

| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| this SPEC is the approved sibling of SPEC-SIGMABAND-001 | security boundary: agent-authored code is written into the user's repository; user decisions 2026-10-06 (split, local patch cap) | SPEC-SIGMABAND-001 |

No further sibling; no recursive sibling.

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| `pkg/healthband/episode.go:12-27`, `catchup.go:12-22,30-42,51-58,73-144`, `claims.go:44,54-60,84-163`, `wal.go:57-68,104-134`, `wal_validate.go:14,31-34`, `types.go:136-153,175-181`, `store.go:33-34,170`, `store_compact.go:129-258`, `sanitize.go:16,79,88,246`, `sanitize_secrets.go:16`, `prompt.go:113,126,164`, `identifier.go:10` | existing (001, merged) | Read at main `1943e596`, 2026-10-08 |
| `internal/cli/react_band.go:139-255`, `react_band_diagnose.go:54-62,72-86,108,192-276`, `react_band_gh.go:73-113`, `react_band_ingest.go:33-41,83-111`, `react_band_help.go:9-58`, `pkg/brainstorm/render.go:31-40,180,196,208-209`, `id.go:101`, `validate.go:26-27`, `pkg/config/schema_health_band.go:3-14`, `docs/health-band.md:223-228`, `pkg/filelock/lock.go:37` | existing (001, merged) | Read at main `1943e596`, 2026-10-08 |
| `internal/cli/orchestra_readonly_policy.go:11-15,53,150-153,207-217`; `pkg/orchestra/provider_runner_single.go:28`, `provider_env.go:34`, `types.go:65-75`; `internal/cli/omp_review_backend.go:63`; `autopus.yaml:77-90` | existing (REVIEWRO, PANERM) | Read and rg; no `SubprocessMode` match in `pkg`, `internal`, `cmd` |
| `pkg/editguard/decide.go:24-36,116`; `docs/edit-guard.md:92`; `.gitignore:11,30`; `pkg/workflow/drift_gate.go:17,22`; `internal/cli/check_rules_hygiene.go:25-30` | existing (EDITGUARD) | Read; check-ignore output for the manifest and runtime paths |
| `pkg/lore/validator.go:9-11,15,64`; `query.go:87` (`hasField`: Constraint, Rejected, Confidence, Directive, Tested); `writer.go:8,11`; `pkg/promptlayer/context_scan.go:112`, `layer.go:89` | existing | Read 2026-10-08 |
| claude 2.1.289 `--restricted` (removes code-running tools and WebFetch, ignores user, project, and local settings, confines file tools to working directories), `--safe-mode`, `--strict-mcp-config`, `--tools`; git 2.50.1 `commit --cleanup` | external CLI | help output 2026-10-08, `probe-a8-evidence.txt` |
| Probe values: `probe-a3-evidence.txt` (`user_status_unchanged=yes head_unchanged=yes stash_after=0`), `probe-a6-evidence.txt` (`default_commit hook_child_events=2 markers=2`, `guarded_commit hook_child_events=0 markers=0`) | executed evidence in `/private/tmp/claude-502/-Users-bitgapnam-Documents-github-autopus-workspace-autopus-adk/0646ea10-1fbb-4a7d-8ece-c1136d78ddce/scratchpad/` | file names are SPEC-SIGMABAND-001 rev 2 probe IDs, not 001's current plan rows |
| Worktree-safety rule (no automatic gc and no prune while worktrees run) | existing harness rule | hook context 2026-10-06 |
| gitattributes(5), git-lfs-config(5), the `locked` file that a running worktree add holds, Go `os.UserCacheDir` | external docs | to be verified by probe A3, which is not-run (CD-1) |
| `PlanOptions.LocalPatch`, `Plan.LocalPatch`, `ExecuteOptions.AfterRecord`, `bandCIFetch.DefaultBranch`, `Request.LocalPatch`, `readOnlyPolicyOptions.Confined`, `pkg/healthband/localpatch_decision.go`, `localpatch_wal.go`, `localpatch_recovery.go`, `patchpolicy.go`, `patchprompt.go`, `commitmsg.go`, `gitpolicy.go`, `internal/cli/react_band_localpatch.go`, integration test | [NEW] planned addition | n/a |

## Reviewer Brief

- Intended scope: D3 as a local patch for SPEC-SIGMABAND-001's tier-3 openings via REQ-01–REQ-14; draft status; Completion Debt CD-1–CD-3 blocks sync; rev 6 is a refresh against merged code plus the closure of the rev 5 findings.
- Explicit non-goals: any remote write, automatic tests, agent write access, escalated episodes, OMP confinement, reviewer-side execution, anything 001 owns.
- Self-verified: every 001, REVIEWRO, PANERM, and EDITGUARD reference re-read at main `1943e596`; Traceability Matrix; Semantic Invariant Inventory; strict validate.
- Reviewer should focus on: the two 001 hooks and the lease chain, the Opening tier rule, the claim-commit and expected-tree rules, Cleanup Rule order and `worktree_incomplete`, recovery under `--dry-run` and flag-off runs, edit guard parity, OQ-1, and Completion Debt only.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 5 | files: spec.md, research.md, plan.md | reason: every existing path, symbol, and line re-read at main `1943e596`; no reference rests on 001's plan
- Q-CORR-02 | status: PASS | attempt: 5 | files: spec.md, research.md, plan.md | reason: references carry existing (001), existing, or [NEW] labels; no planned-by-001 label remains (F-061)
- Q-CORR-03 | status: PASS | attempt: 5 | files: spec.md | reason: strict validate parses all 14 requirements; trailer support matches `hasField`
- Q-CORR-04 | status: PASS | attempt: 6 | files: research.md | reason: references classified; external docs and the `locked` file behavior are marked as awaiting probe A3, not as verified
- Q-COMP-01 | status: PASS | attempt: 1 | files: all | reason: each document keeps its role
- Q-COMP-02 | status: PASS | attempt: 7 | files: spec.md, plan.md, acceptance.md | reason: every REQ maps to an owning task and a scenario; T8 lists every 001 edit with lines
- Q-COMP-03 | status: PASS | attempt: 7 | files: spec.md | reason: the Recovery State Table has rows for `worktree_failed`, an interrupted add, and `patch_done` (F-083); live failures keep step codes
- Q-COMP-04 | status: PASS | attempt: 6 | files: spec.md, research.md | reason: remaining work is blocking Completion Debt CD-1–CD-3 and the operator question OQ-1
- Q-COMP-05 | status: PASS | attempt: 7 | files: research.md, acceptance.md | reason: INV-01–INV-10 map to Must oracles; S9 follows the table row by row, including B's diagnosis-only crash and `worktree_incomplete`
- Q-COMP-06 | status: PASS | attempt: 3 | files: spec.md, research.md | reason: Traceability Matrix and Reviewer Brief bound the scope, including the rev 6 refresh items
- Q-COMP-07 | status: PASS | attempt: 5 | files: research.md | reason: CD-1–CD-3 block sync; OQ-1 is a question, not a requirement; Evolution Ideas carry no IDs
- Q-COMP-08 | status: PASS | attempt: 6 | files: plan.md, research.md | reason: A2 PASS cites existing evidence and the 2026-10-08 help re-read; A1 and A3 not-run with reasons
- Q-FEAS-01 | status: PASS | attempt: 3 | files: plan.md | reason: the 001 hooks are small and owned by T8; files near the 300-line ceiling may split along their seams
- Q-FEAS-02 | status: PASS | attempt: 5 | files: plan.md, spec.md | reason: recovery has its own step, lock, and timeouts; appends wait 5 s inside group deadlines (F-068 b')
- Q-FEAS-03 | status: PASS | attempt: 7 | files: acceptance.md | reason: each S9 expectation matches one row of the Recovery State Table; the recorded expected tree replaces a recovery-time recomputation (F-080)
- Q-STYLE-01 | status: PASS | attempt: 1 | files: spec.md | reason: no ambiguous words in requirements
- Q-STYLE-02 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: Priority uses Must only
- Q-STYLE-03 | status: PASS | attempt: 1 | files: acceptance.md | reason: bare Given/When/Then/And steps
- Q-SEC-01 | status: PASS | attempt: 6 | files: spec.md, acceptance.md | reason: subprocess claude confined; OMP unconfined by rule; nonce fences keep the length rule; confinement proof pending as CD-2
- Q-SEC-02 | status: PASS | attempt: 5 | files: spec.md | reason: untracked secrets are outside the worktree; provider env drops 001's credential list and `GIT_*`; added lines use 001's band secret forms
- Q-SEC-03 | status: PASS | attempt: 7 | files: spec.md | reason: intent records, the recorded claim commit and expected tree, and keep-on-doubt cleanup protect user data in the live run and in recovery (F-068, F-080, F-082)
- Q-COH-01 | status: PASS | attempt: 1 | files: spec.md | reason: one story: the 3σ local patch
- Q-COH-02 | status: PASS | attempt: 3 | files: research.md | reason: the boundary work sits in requirements, blocking Completion Debt, or OQ-1
- Q-COH-03 | status: PASS | attempt: 1 | files: research.md | reason: one approved sibling, no recursion
