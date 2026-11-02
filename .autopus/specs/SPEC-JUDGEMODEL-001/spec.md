# SPEC-JUDGEMODEL-001: The SPEC Review Judge Runs a Model of Its Own

**Status**: draft
**Created**: 2026-11-02
**Domain**: JUDGEMODEL
**Priority**: MEDIUM
**Change Class**: feature (two new `autopus.yaml` keys, a changed judge argv at runtime, new CLI flags, and new review tab selects; `feature` is a high-risk class and the surface spans `pkg/config` and `internal/cli`, so the lean profile does not apply)
**Source**: operator request of 2026-11-02 (the judge should run, for example, claude on opus while the claude reviewer runs sonnet, chosen in the `auto quality` review tab because the commands are long)
**Target module**: `autopus-adk/` (this repository)
**Module ownership**: `.autopus/specs/SPEC-JUDGEMODEL-001/**`, `pkg/config/schema_spec.go`, `pkg/config/schema.go`, `[NEW] pkg/config/schema_review_judge_model*.go`, the files of `internal/cli` listed under Generated And Changed Surfaces, `README.md`, `docs/README.ko.md`, `CHANGELOG.md`, `content/skills/{spec-review,using-autopus,adaptive-quality}.md` and their copies under `templates/codex/skills/` and `templates/gemini/skills/`, the TUIUX goldens they regenerate, and section 96 of `.autopus/specs/SPEC-TUIUX-001/evidence/base-goldens-recapture.md`
**Depends on**: SPEC-REVMODEL-001 (review tab, `auto quality review`, line splice writer), SPEC-REVIEWRO-001 (read-only judge assembly), SPEC-REVIEWPOLICY-001 (review plan block)

## Purpose

`spec.review_gate.judge` names a provider only. At runtime `assembleSpecReviewJudge` reuses that provider's reviewer config, so the judge always runs the reviewer's model and effort: a project that reviews with claude on `claude-sonnet-5-5` cannot let the claude judge arbitrate on `claude-opus-5-5`. The only workaround, a second `orchestra.providers` entry, changes the reviewer set and the quorum.

This SPEC adds two optional keys, `spec.review_gate.judge_model` and `spec.review_gate.judge_effort`. When they are set, the judge call runs the judge provider's entry with only its model and effort replaced, encoded the way the review tab writes a provider's model; reviewer calls do not change. The plan block and the review receipt name the judge model, `auto quality review judge` takes `--model` and `--effort`, and the review tab lets the operator pick the judge model right after the judge.

## Background

- `ReviewGateConf.Judge` (`pkg/config/schema_spec.go`) is a provider name. `planSpecReviewProviders` (`internal/cli/spec_review_readiness.go`) calls `specReviewJudgeAssembly`, which reuses the matching reviewer config or resolves the judge alone through `resolveProviders`, the read-only gate, the Codex capability resolution, the review timeout, and the read-only projection (`internal/cli/spec_review_readonly.go`). The loop passes the result as `OrchestraConfig.JudgeConfig`, and `runStructuredSpecReviewJudge` executes it with `Role: "judge"`.
- The review tab encodes a model per backend (SPEC-REVMODEL-001 CLI Model Contract): an OMP provider's `model` selector `<provider>/<model>:<thinking>`; claude `--model` and `--effort` (no effort for haiku, which drops the flag); codex `-m` and `-c model_reasoning_effort="<effort>"` with `model_policy: pinned`; agy `--model <id>`, the id carrying its effort. `planReviewArgsEdit` changes only those values and appends a missing pair. The lists it offers are claude's four tier models, the run's Codex catalog, `agy models`, and the attested OMP catalog of the provider's family.
- The strict config loader rejects an unknown key, so an `auto` release that does not know a key refuses the whole file. SPEC-REVMODEL-001 REQ-RVM-15 keeps the tab inside the v0.50.125 schema for that reason.
- `auto quality review judge none` writes `judge: ""`; the line splice writer replaces a scalar's value span or appends a missing key at the end of its mapping, and `verifyReviewLineEdit` refuses any edit whose result loads to anything but the planned values.

## Outcome Boundary

- Outcome Lock: with `judge_model` (and optionally `judge_effort`) set, `auto spec review` runs the configured judge on that model while every reviewer keeps its own; an invalid judge model is refused before any readiness probe or model call; the plan block and `review-receipt.json` name the judge model; `auto quality review judge <provider|none> [--model <model>|default] [--effort <effort>] [--dry-run]` sets, changes, and clears it with the tab's lists, and `auto quality review [show] [--json]` shows it; the review tab offers the judge model right after a new judge and through a `<judge> — judge model 고르기` option, previews it, writes only the changed keys with comments kept, and removes the keys again when the judge model goes back to the reviewer model. Without the keys every output and every call is byte-for-byte today's.
- Mandatory requirements: REQ-JM-SCOPE-01 through REQ-JM-VERIFY-01 below.
- Explicit non-goals: a judge model for `orchestra.judge` or for orchestra commands (brainstorm, plan, review, secure); judge models of providers other than the configured judge (`--judge <other>` keeps that provider's reviewer model); switching the judge alone to another backend; attesting the judge model against a catalog at review time; a live check of the judge model.
- Completion evidence: the tests named in `acceptance.md`; `go test ./pkg/config ./pkg/spec ./internal/cli -count=1`; the source-lines gate on the changed Go files; `golangci-lint` on the changed packages; `auto spec validate --strict`.

## Definitions

- Judge model: the pair (`judge_model`, `judge_effort`). Per backend of the judge provider's entry: OMP — `judge_model` is `<provider>/<model>` and `judge_effort` its thinking level, or `judge_model` already carries `:<thinking>`; claude and codex CLI — a model id and an effort of `low`, `medium`, `high`, `xhigh`, `max`, or none; agy — a listed id, which carries its effort, and no `judge_effort`.
- Applies: the judge model applies to a review when the review's judge (`--judge`, else the plan's) equals `spec.review_gate.judge` and `judge_model` is not empty.
- Judge model text: `<model> · <effort>` for a CLI judge, `<model>:<effort>` for an OMP judge, `<model>` without an effort.
- Reviewer model: the model and effort of the judge provider's own `orchestra.providers` entry.

## Requirements

### Scope

**REQ-JM-SCOPE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL change only the files under Module ownership and SHALL NOT edit the generated roots `.claude/**`, `.codex/**`, `.gemini/**`, `.opencode/**`, `.omp/**`, or `.agents/**`.

### Configuration

**REQ-JM-CFG-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL accept the optional keys `spec.review_gate.judge_model` and `spec.review_gate.judge_effort`, both `omitempty`, so a configuration that does not set them loads, saves, and runs exactly as before and a saved default configuration holds neither key.

**REQ-JM-CFG-02**
Priority: Must
Type: Unwanted
IF `judge_model` or `judge_effort` is set while `spec.review_gate.judge` is empty, `judge_effort` is set while `judge_model` is empty, or either value starts with `-` or holds whitespace or a control character, THEN THE SYSTEM SHALL fail configuration load with an error that names the dotted key.

### Runtime

**REQ-JM-RUN-01**
Priority: Must
Type: Event-driven
WHEN `auto spec review` assembles a judge to which the judge model applies, THEN THE SYSTEM SHALL resolve the judge from its `orchestra.providers` entry separately from the reviewers, replace only its model and effort with the review tab's encoding for the entry's backend (an OMP entry's `model`; claude `--model`/`--effort`; codex `-m` and `model_reasoning_effort` with the model policy pinned; agy `--model`), recompute the model family of an OMP entry whose provider name has none, and pass the result through the same read-only gate, Codex capability resolution, review timeout, and read-only projection as before, leaving every reviewer config unchanged.

**REQ-JM-RUN-02**
Priority: Must
Type: Unwanted
IF a judge model that applies cannot run on the judge's backend — its binary has no CLI Model Contract row, a CLI judge gets a `<provider>/<model>` selector, an OMP judge gets a value that is not one, the selector's model family differs from the judge provider's, the thinking level is given twice with different values, an agy judge gets `judge_effort`, a claude or codex judge gets an effort outside `low`–`max`, or the args planner refuses the value — THEN THE SYSTEM SHALL fail the review before any readiness probe or model call with an error that starts `spec review: ` and names the key, the value, the judge, and its backend.

**REQ-JM-RUN-03**
Priority: Must
Type: Event-driven
WHEN the review plan block is printed and `judge_model` is configured, THEN THE SYSTEM SHALL print after the judge line `  judge model: <judge model text> — spec.review_gate.judge_model` when it applies, and `  judge model: <judge>의 리뷰어 model — spec.review_gate.judge_model은 judge <configured judge>에만 씁니다` when the review runs another judge; without `judge_model` or without a judge the block is unchanged.

**REQ-JM-RUN-04**
Priority: Must
Type: Event-driven
WHEN a review whose judge model applied writes `review-receipt.json`, THEN THE SYSTEM SHALL record `judge.model` and `judge.effort` (each omitted when empty) beside `judge.provider`, and SHALL write no such field otherwise.

### Command

**REQ-JM-CLI-01**
Priority: Must
Type: Event-driven
WHEN `auto quality review judge <provider> --model <model> [--effort <effort>]` runs, THEN THE SYSTEM SHALL resolve the model exactly as `auto quality review model` resolves one for that provider's working backend (the same list source, the `:<thinking>` and `--effort` rules, the agy group and suffix rules, and the effort default seeded by the current judge model, else the reviewer model), plan the edit with the tab's planner, print the settings that change and the judge model note, and write `spec.review_gate.judge`, `judge_model`, and `judge_effort` only where they change, with `--dry-run` writing nothing.

**REQ-JM-CLI-02**
Priority: Must
Type: Event-driven
WHEN `--model default` is given, or the judge changes to another provider or to `none` without `--model`, THEN THE SYSTEM SHALL clear the judge model, removing the `judge_model` and `judge_effort` lines from `autopus.yaml` so the file again holds only keys older releases read.

**REQ-JM-CLI-03**
Priority: Must
Type: Unwanted
IF `--effort` is given without `--model`, `--model` is given with `none`, `--model default` is given with `--effort`, the model or effort is not one the judge provider's list offers, or `judge_model` or `judge_effort` is written as a `${NAME}` placeholder, THEN THE SYSTEM SHALL exit 1 with one line naming the refusal (and the accepted values for a refused model) and write nothing.

**REQ-JM-CLI-04**
Priority: Must
Type: Event-driven
WHEN `auto quality review [show]` runs, THEN THE SYSTEM SHALL print `spec.review_gate.judge_model = <value>` and `spec.review_gate.judge_effort = <value>` after the judge line for each key the file sets, as written, and `--json` SHALL carry `judge_model` and `judge_effort` (omitted when empty).

### Review Tab

**REQ-JM-TUI-01**
Priority: Must
Type: Event-driven
WHEN the review tab asks the judge and the working judge takes part, THEN THE SYSTEM SHALL add `<judge> — judge model 고르기` as the last option; WHEN that option or another provider is chosen, THEN THE SYSTEM SHALL ask `<judge> judge model` with `그대로 — <judge model text or 리뷰어 model과 같음>`, `리뷰어 model과 같음` while a judge model is set, and the models the provider's model select offers for its working backend, followed by `<judge> judge thinking (<selector>)` or `<judge> judge effort` exactly where that select asks it; choosing another provider first resets the judge model, and keeping the judge asks nothing more.

**REQ-JM-TUI-02**
Priority: Must
Type: Event-driven
WHEN an edit changes the judge model, THEN THE SYSTEM SHALL show `spec.review_gate.judge_model <old> → <new>` and `spec.review_gate.judge_effort <old> → <new>` after the judge line (old values as written, an empty model read `리뷰어 model`, an empty effort `없음`), the note `judge model은 auto spec review의 judge 호출에만 쓰입니다. judge_model을 모르는 이전 auto는 이 autopus.yaml을 읽지 못합니다` when the new model is set, the judge cell `judge (<judge model text>)` in the after table, and in the menu summary `judge <judge> (<judge model text>)`; and SHALL write with the line splice writer, appending a missing key at the end of `spec.review_gate`, replacing a value span, removing the line of a key that becomes empty, keeping every other byte, and refusing what the writer refuses.

**REQ-JM-TUI-03**
Priority: Must
Type: Unwanted
IF `judge_model` or `judge_effort` is written as a `${NAME}` placeholder, THEN THE SYSTEM SHALL print the value refusal line for that key instead of the judge select and leave the judge and the judge model as they are.

**REQ-JM-COMPAT-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL write `judge_model` and `judge_effort` only for an edit that sets a judge model, amending SPEC-REVMODEL-001 REQ-RVM-15 for these two keys alone, and SHALL say in the note, the help, and both READMEs that an `auto` release without the keys refuses such a file.

### Documentation

**REQ-JM-DOC-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL document the keys, the runtime rule, the command flags, and the tab selects in `auto quality --help`, `auto quality review --help`, `README.md`, `docs/README.ko.md`, the CHANGELOG, and the review sections of the `spec-review`, `using-autopus`, and `adaptive-quality` skill sources, re-recording the TUIUX goldens those sources regenerate.

### Verification

**REQ-JM-VERIFY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL prove every Must requirement with tests over temporary projects and recording backends that never run a provider binary or touch the operator's configuration.

## Generated And Changed Surfaces

- `pkg/config/schema_spec.go` (the two fields), `pkg/config/schema.go` (the validation call), `[NEW] pkg/config/schema_review_judge_model.go`.
- `[NEW] internal/cli/spec_review_judge_model.go`: applies-rule, per-backend application and refusal, plan line, receipt fields; hooks in `spec_review_readonly.go` (`assembleSpecReviewJudge`), `spec_review_readiness.go` (`specReviewProviderPlan.JudgeModel`), `spec_review_plan_print.go` (one call), `spec_review_lean.go` (runtime evidence), `spec_review_receipt.go` (`model`, `effort`, evidence field), `spec_review_runtime.go` (one call).
- `[NEW] internal/cli/quality_compose_review_judge.go` (tab selects), `[NEW] internal/cli/quality_compose_review_judgemodel.go` (composition value, attestation, targets, change lines, cells, line removal), `[NEW] internal/cli/quality_review_judge.go` (command flags and resolution); `quality_compose_review.go`, `quality_compose_review_plan.go`, `quality_compose_review_raw.go`, `quality_compose_review_write.go`, `quality_compose_review_splice.go`, `quality_compose_review_verify.go`, `quality_compose_review_lines.go`, `quality_compose_review_menu.go`, `quality_compose_review_check.go`, `quality_compose_review_clitab.go`, `quality_compose_review_text.go`, `quality_review.go`, `quality_review_edit.go`, `quality_review_model.go`, `quality_help.go`.
- `README.md`, `docs/README.ko.md`, `CHANGELOG.md`, the three skill sources and their Codex and Gemini template copies, the regenerated TUIUX NPD goldens and recapture section 96.

## Related SPECs

- SPEC-REVMODEL-001: the review tab, `auto quality review`, the CLI Model Contract, and the line splice writer this SPEC extends; REQ-JM-COMPAT-01 amends its REQ-RVM-15.
- SPEC-REVIEWRO-001: the read-only gate and projection the judge still passes.
- SPEC-REVIEWPOLICY-001: the plan block that gains the judge model line.

## Traceability Matrix

| Requirement | Plan task | Acceptance | Invariants |
|---|---|---|---|
| REQ-JM-SCOPE-01 | T7 | S10 | INV-J06 |
| REQ-JM-CFG-01, CFG-02 | T1 | S1 | INV-J01 |
| REQ-JM-RUN-01 | T2 | S2 | INV-J02, INV-J03 |
| REQ-JM-RUN-02 | T2 | S3 | INV-J04 |
| REQ-JM-RUN-03, RUN-04 | T2 | S4 | INV-J05 |
| REQ-JM-CLI-01, CLI-02 | T4 | S5 | INV-J01, INV-J07 |
| REQ-JM-CLI-03, CLI-04 | T4 | S6 | INV-J04 |
| REQ-JM-TUI-01, TUI-02 | T3, T5 | S7, S8 | INV-J07 |
| REQ-JM-TUI-03 | T5 | S9 | INV-J04 |
| REQ-JM-COMPAT-01 | T3, T4 | S5, S8 | INV-J01 |
| REQ-JM-DOC-01 | T6 | S10 | INV-J06 |
| REQ-JM-VERIFY-01 | T7 | S2, S7 | INV-J06 |
