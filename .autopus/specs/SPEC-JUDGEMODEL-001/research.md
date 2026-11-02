# SPEC-JUDGEMODEL-001 Research

## Existing Code Analysis

- `internal/cli/spec_review_readonly.go` `assembleSpecReviewJudge`: a judge that names an assembled reviewer reuses that reviewer's projected config; otherwise it resolves the judge alone (`resolveProviders` → `checkReadOnlyProvider` → `resolveCodexProviderCapabilities` → `configureSpecReviewProviders` → `applySpecReviewExecutionTimeout` → `applyReadOnlyProviderPolicy` → `stampSpecReviewSandboxEvidence`). The projection must stay the final argv mutation (claude's inline `--tools=` goes last).
- `internal/cli/spec_review_judge.go` `specReviewJudgeConfig` returns `OrchestraConfig.JudgeConfig` when set; the loop always sets it from the assembled plan, so the assembly decides the judge argv. `omp_review_backend.go` reads the selector from the request's `Config.Model`, the subprocess backend the request's `Config.Args`.
- `internal/cli/codex_catalog_runtime.go` re-resolves a codex entry's model only for `model_policy: quality`; a pinned entry is left alone.
- `internal/cli/quality_compose_review_args.go` `planReviewArgsEdit`/`reviewArgsEdit.apply`: the tab's per-row args encoding; `quality_compose_review_cli.go` `reviewArgsRowFor`, `reviewCLIModelList`; `quality_compose_review_choices.go` `reviewModelChoices`; `quality_review_model.go` `setOMPModel`/`setCLIModel`/`agyModel`: the command's resolution with the tab's lists.
- `internal/cli/quality_compose_review_plan.go` (`reviewComposition`, `reviewBaseline`, `planReviewComposition`), `quality_compose_review_write.go` (`reviewTargets`, `saveReviewComposition`, `applyReviewCompositionMemory`), `quality_compose_review_splice.go` (`planReviewSpan`), `quality_compose_review_verify.go` (`verifyReviewLineEdit`), `quality_compose_review_lines.go` (change lines, after table), `quality_compose_review_menu.go` (summary), `quality_compose_review.go` (`reviewJudge`).
- `internal/cli/spec_review_plan_print.go` `printSpecReviewPlan`, `spec_review_receipt.go` `specReviewJudgeReceipt`, `spec_review_runtime.go` `syncReviewedSpecStatusWithReceipt`.
- `pkg/config/loader.go`: strict decode, then `HarnessConfig.Validate`; `schema_review_budget.go` is the shape of a review gate validator with dotted keys.

## Outcome Lock

- User-visible outcome: a project sets the judge's own model and effort in the review tab or with `auto quality review judge <provider> --model <model> [--effort <effort>]`; `auto spec review` runs the judge on it and every reviewer on its own model, prints it in the plan block, and records it in the receipt; an unrunnable value fails before any call; clearing returns the file to keys older releases read.
- Mandatory requirements: REQ-JM-SCOPE-01 through REQ-JM-VERIFY-01 in `spec.md`.
- Explicit non-goals: orchestra command judges, other providers' judge models, judge-only backend switches, catalog attestation at review time, a judge live check.
- Completion evidence: S1–S10 of `acceptance.md`, package test runs, lint, the source-lines gate, strict SPEC validation.

## Visual Planning Brief

`plan.md` carries the Mermaid flow. In prose: the loader checks the keys' shape; the review applies them to a copy of the judge's entry before the read-only gate when the judge is the configured one; the plan block and the receipt name the result; the command and the tab share one composition value, one resolver per backend, and the line splice writer, which can now remove a key's line.

## Design Decisions

- Judge model keys under `spec.review_gate`, not a second provider entry: a second entry would change the reviewer set, the quorum denominator, `orchestra.commands` (Migration 3 fills them with every provider), and the readiness preflight.
- Two keys, `judge_model` and `judge_effort`, one meaning on every backend (model, then effort or thinking); an OMP selector carrying `:<thinking>` is also accepted as written by hand. The command and the tab write the split form.
- The judge model applies only to the configured judge. `--judge <other>` is a different arbitrator, and a model chosen for claude says nothing about codex; the plan block says so instead of guessing.
- Applied inside `assembleSpecReviewJudge`, before the read-only gate: every consumer of the judge config sees the same judge, the projection stays last, and a codex judge with a pinned policy is not re-resolved by the quality profile.
- Encoding by the tab's own planner (`planReviewArgsEdit`), so the judge argv is exactly what the tab would write for that provider: claude's empty effort drops `--effort` (haiku), codex keeps its effort when none is given, a missing flag is appended as a pair.
- Validation in three layers: shape at load (cheap, always), backend fit at review time (no catalog read, so a review never pays an OMP catalog refresh or `agy models`), and attestation against the lists when the command or the tab writes it. A hand-written alias such as `opus` therefore runs; a value the backend cannot carry never does.
- The plan block and the receipt name the effective judge model, not the keys as written: an OMP judge model may carry its thinking inline, alone or beside an equal `judge_effort`, and all three forms ran the same selector, so they print `<base>:<thinking>` once and record the base selector with the thinking level. Show and the tab's change lines stay with the text as written, as for every other review key.
- Clearing removes the key lines instead of writing `""`: a file that once had a judge model and went back is again loadable by releases without the keys (REQ-JM-COMPAT-01).
- The tab asks the judge model only after a new judge or through `<judge> — judge model 고르기`, so a pass that keeps the judge asks exactly the selects it asked before.

## Technology Stack Decision

Go standard library and the packages already used by the review tab (`gopkg.in/yaml.v3`, `pkg/config`, `pkg/orchestra`). No new module and no manifest change.

## Minimality Decision Matrix

| Option | Chosen | Why |
|---|---|---|
| `judge_model`/`judge_effort` under `spec.review_gate` | yes | One provider entry per provider; only the judge call changes |
| A second provider entry for the judge | no | Changes reviewers, quorum, commands, and readiness |
| One `judge_model` key holding `model · effort` | no | Needs a new separator per backend; effort has its own vocabulary |
| Ask the judge model in every pass | no | One more select in every pass of every operator, for a setting most never change |
| Write `""` to clear | no | Leaves a key that older releases refuse |

## Semantic Invariant Inventory

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-J01 | absent keys keep review requests, plan block, receipt, and show output byte for byte (the tab's new option and the help are outside), and only a set judge model writes them | default and compatibility | loaded config, saved file, written lines, receipt keys | S1, S2, S4, S5, S6, S8 |
| INV-J02 | only the judge call changes | isolation | reviewer argv and model, judge argv and model | S2 |
| INV-J03 | the judge argv differs from the reviewer argv only in model and effort values, and the projection stays last | argv equivalence | judge argv | S2 |
| INV-J04 | a value that cannot run or cannot be edited safely never reaches a call or the file | fail before effect | refusals, call counts, file bytes | S3, S6, S9 |
| INV-J05 | what ran is named once, in the effective form, whichever form the keys use | evidence | plan line, receipt fields | S4 |
| INV-J06 | sources only, documented, verified without real providers | scope and verification | changed paths, docs, tests | S10 |
| INV-J07 | the tab and the command write the same edit for the same choice | single writer | change lines, file text | S5, S7, S8 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---|---|---|
| Keys and validation | T1, S1 | covered |
| Runtime judge config per backend | T2, S2 | covered |
| Refusal before any call | T2, S3 | covered |
| Plan line and receipt | T2, S4 | covered |
| Command flags, clearing, show | T4, S5, S6 | covered |
| Tab selects, preview, writer | T3, T5, S7, S8, S9 | covered |
| Documentation | T6, S10 | covered |
| Orchestra command judges, judge live check | explicit non-goals | out of scope by Outcome Lock |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

## Evolution Ideas

Optional improvements. They do not block sync completion and carry no identifiers.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| Include the judge model in the tab's live check | The live check contract counts review providers only; a judge call changes its label and bound | An operator reports a judge model the list offered but the provider refused |
| A judge model for `orchestra.judge` and orchestra debate commands | Different config surface and judge path (`pkg/orchestra/debate_judge.go`) | Requested for brainstorm or plan |
| Insert the new keys right after the `judge` line | The writer appends missing keys at the mapping's end by contract | Operators find the keys hard to spot |

## Sibling SPEC Decision

| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | One change story: give the review judge a model of its own, end to end | None |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| `internal/cli/spec_review_{readonly,readiness,judge,loop,lean,plan_print,receipt,runtime}.go`, `codex_catalog_runtime.go`, `omp_review_backend.go`, `orchestra_readonly_policy.go` | existing | read on 2026-11-02 at origin/main 27297e2a |
| `internal/cli/quality_compose_review*.go`, `quality_review*.go`, `quality_help.go`, `pkg/config/{schema,schema_spec,schema_review_budget,loader}.go` | existing | read on 2026-11-02 at origin/main 27297e2a |
| SPEC-REVMODEL-001 CLI Model Contract, Line Splice Writer Contract, REQ-RVM-15 | existing contract | read on 2026-11-02 |
| every file marked `[NEW]` in `spec.md` and `plan.md` | planned addition on branch `feat/review-judge-model` | `go test` over the packages |

## Reviewer Brief

- Intended scope: two review gate keys, the judge assembly, the plan line and receipt, the `judge` subcommand flags, and the review tab's judge model selects.
- Explicit non-goals: see the Outcome Lock.
- Self-verified: the Traceability Matrix, three executed probes, and the acceptance oracles as tests.
- Reviewer should focus on: whether the judge argv can differ from the reviewer argv in anything but model and effort; whether any path applies a judge model chosen for one provider to another; whether a refused value can reach a readiness probe or a call; and whether the line removal can drop anything but the key's own line.

## Residual Risks

- An older `auto` refuses a file that sets a judge model (strict loader). The note, the help, and the READMEs say so, and clearing removes the keys.
- Review time checks only that the backend can carry the value; a hand-written model the provider does not serve fails at the judge call as a judge failure, as a hand-written reviewer model fails today.
- PR #255 (SPEC-REVIEWREUSE-001) fingerprints the assembled judge config; because the model is applied during assembly, a changed judge model changes that fingerprint, which a merge must keep.

## Review Response (codex, 2026-11-02, REVISE, 3 findings)

| Finding | Resolution |
|---|---|
| F-001 byte identity vs the new tab option | Outcome Lock and REQ-JM-CFG-01 scope byte identity to review requests, plan block, receipt, and `auto quality review [show] [--json]`; the tab's option and select and the help texts are named as outside it; S2 and S7 say so; `TestSpecReviewJudgeModel_AbsentOrOtherJudgeKeepsTheReviewerConfig` checks the receipt's judge keys |
| F-002 OMP inline thinking in plan and receipt | Effective judge model defined (base selector and one thinking level; a mismatch is refused); REQ-JM-RUN-03/04 and S4 give exact values for the split, inline-only, and inline-plus-equal forms; `TestSpecReviewJudgeModel_OMPFormsNormalize` fails on the raw recording; show and change lines keep the text as written (`TestReviewJudgeModel_InlineThinkingShowsOnce`) |
| F-003 test files outside Module ownership | Module ownership covers the listed `internal/cli` files, tests included; Generated And Changed Surfaces lists every new test and the two changed regression tests; T7 and S10 check the change set against Module ownership |

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: every cited function, key, and message was read on 2026-11-02 at 27297e2a
- Q-CORR-02 | status: PASS | attempt: 1 | files: spec.md, plan.md | reason: new files carry `[NEW]` in the surfaces and the tasks
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: Event-driven requirements use WHEN ..., THEN; Unwanted use IF ..., THEN; scenarios use bare Given/When/Then/And
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline separates existing code, the extended contract, and `[NEW]` additions
- Q-COMP-01 | status: PASS | attempt: 1 | files: all four | reason: contract, tasks and probes, oracles, and decisions each live in their file
- Q-COMP-02 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: 16 requirements map to S1–S10, refusals and clearing included
- Q-COMP-03 | status: PASS | attempt: 2 | files: spec.md | reason: each requirement names its trigger and its output text; the effective judge model fixes one plan and receipt form
- Q-COMP-04 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: the Outcome Lock covers runtime, command, tab, evidence, and compatibility, with byte identity scoped to what stays unchanged
- Q-COMP-05 | status: PASS | attempt: 2 | files: research.md, acceptance.md | reason: INV-J01 to INV-J07 map to oracles with fixed strings and argv, the three OMP forms included
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: Traceability Matrix with invariants; the Reviewer Brief names the focus
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: Completion Debt is empty; residual risks are stated
- Q-COMP-08 | status: PASS | attempt: 2 | files: plan.md | reason: B1 to B3 executed as the named tests on 2026-11-02
- Q-FEAS-02 | status: PASS | attempt: 2 | files: spec.md, plan.md | reason: Module ownership lists every changed path, the internal/cli test files and the two changed regression tests included; T7 checks the change set against it
