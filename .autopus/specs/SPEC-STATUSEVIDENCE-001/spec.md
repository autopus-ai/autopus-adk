# SPEC-STATUSEVIDENCE-001: Status Separates Declared SPEC State from Execution Evidence and Names the Next Command

**Status**: draft
**Created**: 2026-11-02
**Domain**: STATUSEVIDENCE
**Priority**: HIGH
**Change Class**: feature (new `auto status --json` evidence fields are a machine-read contract, so the lean profile does not apply)
**Source**: ADK improvement brief of 2026-11-01, requirement R5 (untrusted requirements input, summarized here)
**Target module**: `autopus-adk/` (this repository)
**Module ownership**: `internal/cli/status*.go`, `internal/cli/init*.go` (guidance text), `internal/cli/pipeline_run*.go`, `pkg/pipeline/resume_admission*.go` (a read-only entry to the engine's resume admission), the `auto status` row and the `auto pipeline run` paragraph of `README.md` and `docs/README.ko.md`, `.autopus/specs/SPEC-STATUSEVIDENCE-001/**`, the `init-yes.stdout` and `init-pipe.stdout` TUIUX goldens, and section 18 of `.autopus/specs/SPEC-TUIUX-001/evidence/base-goldens-recapture.md`
**Depends on**: SPEC-PIPELINE-RESUME-001 (pipeline receipt lineage, failure decisions, restored phases, output records)

## Purpose

`auto status` counts a SPEC as complete when its `spec.md` says `done`, `completed`, or `implemented`, which is a document claim, not a verified run. `auto pipeline run` reports `Pipeline complete: N phases executed` with N including phases restored from a checkpoint, and a dry run overwrites the checkpoint of the last real run. `auto init` tells every user without a worker config to connect Autopus Desktop first, although the ADK works on its own, and `auto status` with no SPEC sends every user to `/auto plan`, even for a small fix that needs no SPEC. The pipeline picks its platform from the first installed of `claude`, `codex`, `agy` without saying so or relating it to the project's configured platforms.

This SPEC makes status keep three kinds of evidence apart — the declared SPEC state, the last pipeline execution, and the last SPEC review — marks evidence stale or unknown when what it judged has changed or cannot be compared, decides resumability with the engine's own admission, separates executed from restored phases, and names next commands that match what is installed and routed, each with its absolute directory, where it runs, which provider it uses, and what it calls and writes. Commands carry only validated values, and nothing status recommends runs by itself.

## Background

- Pipeline runs persist `.autopus/pipeline-state/<SPEC>.yaml` (`pipeline_checkpoint.v2`) with the run receipt: run and task IDs, `resumed_from`, `prior_attempts`, `terminal_state`, `blocker`, failures with decisions, `resume.restored_phases`, `dispatch_count`, transitions, provider receipts, the checkpoint's `git_commit_hash` and `snapshot_hash`, and the output records later phases read. A run blocked before dispatch while a canonical checkpoint exists writes `<SPEC>.blocked.yaml` and keeps the checkpoint. A dry run overwrote the checkpoint with terminal `dry_run`.
- `auto pipeline run --continue` refuses a checkpoint whose git hash differs from HEAD; the engine then runs `planResume`: identity, version, snapshot, dependency closure, route, and output-record verification, refusing when a working-tree-writing phase cannot be restored. Every dispatch starts with an in-progress transition of attempt one or more; a restore records attempt zero; a routed fix can dispatch a restored phase again.
- The owner sidecar `<SPEC>.execution-owner.json` is rewritten by every OMP invocation before admission; the receipt's provider receipts record backend `orca` for orca dispatches and `rpc-*` for the native OMP DAG.
- `auto spec review` writes `{SPEC_DIR}/review-receipt.json` (`spec_review_promotion_receipt.v1`) with no content fingerprint; a passing review promotes `spec.md` after `finished_at` is taken and before the receipt is written, and `/auto sync` later rewrites the same status line. PR #252, not on this base, adds `{SPEC_DIR}/review-session.json` (`spec_review_session.v1`).
- `git diff` skips files marked assume-unchanged or skip-worktree and, with `core.fileMode` off, executable-bit changes.
- Codex invokes routes as `$codex-auto-<route>` right after init, or as `@auto <route>` once the local plugin is installed (init printed the retired `$auto plan`); Claude Code, Antigravity CLI, OpenCode, and Oh My Pi use `/auto <route>`. Small low-risk work without a SPEC goes through `/auto fix`.
- The Desktop runtime helper is resolved from `AUTOPUS_DESKTOP_RUNTIME_HELPER` or packaged locations, never `PATH`; `auto connect` delegates to it, and a Desktop connection writes `~/.config/autopus/worker.yaml`.

## Outcome Boundary

- Outcome Lock: `auto status` text and `--json` show, per SPEC, the declared status, the last pipeline execution with every field of the JSON contract below, whether `--continue` would be admitted as the engine decides it, the verification state of the last completed run against a validated baseline, and the last SPEC review, and never count a declared-done SPEC as verified; a dry run never replaces the last real run's evidence; `auto pipeline run` reports executed and restored phases and dispatches separately and names its platform and why; status and init name next commands that match the installed AI CLIs, pipeline platforms, configured platforms, and Desktop state, lead with standalone use, offer work without a SPEC beside `/auto plan`, and say for each command its absolute directory, where it runs, which provider it uses, and what it calls and writes; commands carry only validated values; status writes nothing, runs nothing but read-only git, and runs no recommended command.
- Mandatory requirements: REQ-SEV-SCOPE-01 through REQ-SEV-VERIFY-01 below.
- Explicit non-goals: a new dashboard or state store; changing pipeline platform selection itself; changing what `auto spec review` writes; detecting Desktop installation in `auto init`; Windows-specific Desktop paths; probing provider authentication or making any model call from status.
- Completion evidence: the tests named in `acceptance.md`, `go test ./internal/cli/... ./pkg/pipeline/...`, the TUIUX goldens re-recorded with section 18 of the recapture log, and the source-lines gate on the changed Go files.

## Definitions

- Declared status: the normalized `spec.md` status; `done`, `completed`, and `implemented` are documented-done. JSON `completed` keeps this meaning.
- State root: a directory whose `.autopus/pipeline-state` may hold a SPEC's evidence — the root holding the SPEC's `.autopus/specs`, and the status directory when no other scanned SPEC has the same ID. The root whose newest evidence time (receipt start or finish, baseline `recorded_at`, else file times) is latest is chosen; other roots with evidence are reported as `other_roots`.
- Executed and restored phases: a phase whose final status is `done` is executed when this run dispatched it (an in-progress transition of attempt one or more) and restored otherwise. Dispatches: `dispatch_count` and `cumulative_dispatch_count`.
- Stop reason: for a stopped run (`blocked`, `failed_budget`, `cancelled`, `partial_preserved`), the last failure's decision when it is one that ends a run in that terminal state (`circuit_break_no_progress` or `blocked` for `blocked`, `budget_exhausted` for `failed_budget`, `cancelled` for `cancelled`), else the terminal state.
- Resume admission: the CLI's HEAD check, then `pipeline.AdmitResume`, which runs the engine's `planResume` without dispatching or writing, with the current SPEC snapshot, HEAD, and route.
- Execution owner: for an `omp` run, `orca` when a provider receipt reports backend `orca`, `omp` when one reports an `rpc-` backend, else `unknown`.
- Verification baseline: `<state>/<SPEC>.verification.json`, schema `pipeline_verification_baseline.v1`: `spec_id`, `run_id`, `task_id`, `base_head` (HEAD when the run completed), `spec_fingerprint`, `tree_fingerprint`, `tree_paths`, `recorded_at`. It is valid only when every required field is present and well-formed, `spec_id` names the SPEC, every `tree_paths` entry is a known digest form outside `.autopus`, and `tree_fingerprint` equals the hash of `tree_paths`.
- SPEC fingerprint: SHA-256 over `spec.md`, `plan.md`, `acceptance.md`, and `research.md` without the lines before their first H2 that start with `status:` or `**Status**`.
- Tree fingerprint: `sha256:` over every path outside any `.autopus` directory whose working-tree content differs from `base_head` (`git -c core.fileMode=true diff --name-only --no-renames`) or that is untracked and not ignored, each with the SHA-256 of its content plus the executable bit, its symlink target, a submodule's commit, or `deleted`. A tree with a tracked file marked assume-unchanged or skip-worktree, more than 5,000 paths, or more than 1 GiB of changed content has no fingerprint.
- Evidence label, by precedence: `unfinished` (no terminal state), `failed` (stopped), `verified` (verification `fresh`), `stale`, `unverified` (verification `unknown`), `documented_only` (declared done, no evidence), `none`. A dry run is not execution evidence.
- Action directory and configuration: a SPEC's pipeline actions run in its chosen state root, its implement and review actions in the root holding its `.autopus/specs`, and the other actions in the status directory, each given as an absolute path. A module with its own `autopus.yaml` supplies the platforms and review providers of its SPECs' actions.
- Pipeline platform: the platform pipeline commands use — the auto-selected one of `claude`, `codex`, `agy` (`gemini`); else `omp` when installed, named with `--platform omp`; else none, and no pipeline command is offered.

## Requirements

### Scope

**REQ-SEV-SCOPE-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL build status evidence only from existing sources and the engine's read-only resume admission, SHALL add no state store beyond the verification baseline, and SHALL make status write no project file, run no program but read-only git, call no model or server, and never run a recommended command.

### Declared State and Execution Evidence

**REQ-SEV-DECL-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL report the declared status apart from an evidence label and count every label, `verified` among them, apart from the declared done count in the text summary and the `summary` JSON.

**REQ-SEV-EXEC-01**
Priority: Must
Type: Event-driven
WHEN a SPEC has execution evidence, THEN THE SYSTEM SHALL report every execution field of the JSON contract in JSON and in text, with the stop reason of a stopped run.

**REQ-SEV-EXEC-02**
Priority: Must
Type: Event-driven
WHEN the last run stopped, THEN THE SYSTEM SHALL report the resume admission — admitted with the phases it restores and dispatches again, or refused with the CLI's or engine's reason — and the checks it covered, and, when admitted, the resume command pinned to the run's platform and execution owner, or a note why no command is offered.

**REQ-SEV-EXEC-03**
Priority: Must
Type: Unwanted
IF `<SPEC>.blocked.yaml` is newer than the canonical checkpoint, THEN THE SYSTEM SHALL report that the last attempt was blocked before dispatch, with its blocker.

**REQ-SEV-EXEC-04**
Priority: Must
Type: Event-driven
WHEN `auto pipeline run --dry-run` persists a receipt, THEN THE SYSTEM SHALL write it under `.autopus/pipeline-state/dry-run/` and SHALL leave the canonical checkpoint and the verification baseline unchanged.

**REQ-SEV-EXEC-05**
Priority: Must
Type: Event-driven
WHEN more than one state root holds a SPEC's evidence, THEN THE SYSTEM SHALL report the newest root's evidence, name the other roots, and run the SPEC's pipeline commands in the chosen root.

**REQ-SEV-EXEC-06**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL read an OMP run's execution owner from that run's receipt, never from the owner sidecar, and SHALL report `unknown` when the receipt records no dispatch.

### Verification Baseline

**REQ-SEV-BASE-01**
Priority: Must
Type: Event-driven
WHEN `auto pipeline run` ends `completed` in a git work tree whose changes git does not hide, THEN THE SYSTEM SHALL replace the verification baseline atomically with mode 0600, and SHALL report a failure on stderr without failing the run and without altering the previous baseline.

**REQ-SEV-BASE-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL report verification `fresh` only when the SPEC fingerprint and the tree digests recomputed against `base_head` equal the recorded ones; `stale` with `working tree changed since verification: <paths>` or `SPEC documents changed since verification`; `unknown` with the reason when the last completed run left no baseline, the baseline belongs to another run, git hides changes, or the tree cannot be compared; and `none` without a completed run or baseline.

**REQ-SEV-BASE-03**
Priority: Must
Type: Unwanted
IF a baseline is not valid, THEN THE SYSTEM SHALL report verification `unknown` with `verification baseline is invalid: <what>`.

### Review Evidence

**REQ-SEV-REVIEW-01**
Priority: Must
Type: Event-driven
WHEN a SPEC directory holds `review-receipt.json`, THEN THE SYSTEM SHALL report its verdict, gate status, loop status, run ID, and finish time, and its freshness: by content when the receipt records `spec_fingerprint`; without one, `stale` when a SPEC document was written after the receipt file and `unknown` otherwise; and `unknown` when a SPEC document cannot be read; never `fresh` by modification time.

**REQ-SEV-REVIEW-02**
Priority: Must
Type: Unwanted
IF `review-session.json` or `review-receipt.json` is unparseable or of an unknown schema, THEN THE SYSTEM SHALL report it as `unknown` with the reason and continue; a `spec_review_session.v1` file reports its state, attempt count, and distinct call count.

### Pipeline Run Output

**REQ-SEV-RUN-01**
Priority: Must
Type: Event-driven
WHEN `auto pipeline run` completes, THEN THE SYSTEM SHALL print `Pipeline complete: <E> phases executed, <R> restored from checkpoint, <D> dispatches`, counting executed and restored phases as defined above.

**REQ-SEV-RUN-02**
Priority: Must
Type: Event-driven
WHEN `auto pipeline run` resolves its platform, THEN THE SYSTEM SHALL print on stderr `Pipeline platform: <platform> — <reason>`, naming `gemini (agy)` for agy, the reason being `--platform`, `first installed of claude → codex → agy`, or `no AI CLI installed; falls back to claude`.

### Next Commands

**REQ-SEV-NEXT-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL list next actions in this order — `auto init` without `autopus.yaml`, `auto doctor` without a pipeline platform, at most three SPEC actions ranked resume, new run, re-verify stale, implement, review, re-verify unverified, then `/auto fix` and `/auto plan`, then the optional Desktop connection — each with `auto_run: false`.

**REQ-SEV-NEXT-02**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL report the installed AI CLIs, the configured platforms, and the pipeline platform with its reason, whether `auto pipeline run` selects it unprompted, and a note when it is not configured for the project.

**REQ-SEV-NEXT-03**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL present the Desktop connection as optional — status offers `auto connect` only when the helper is installed and no worker config exists — and SHALL make init list new work and a small change first and the Desktop connection last, each with the directory, place, provider, calls, and writes status gives the same action.

**REQ-SEV-NEXT-04**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL give every action its absolute directory, give a terminal action a shell line that changes into that directory quoted, build commands only from a SPEC ID the pipeline accepts, a platform `auto pipeline run` accepts, and an owner of `omp` or `orca`, offer no command built from anything else, and show receipt and file-system strings without control characters.

**REQ-SEV-JSON-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL carry in `auto status --json` exactly the fields of the JSON contract below, each with the text counterpart it names.

### Verification

**REQ-SEV-VERIFY-01**
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL prove every Must requirement with tests that use temporary repositories, checkpoints made by the real engine, an injected machine probe, and executed admission probes against `--continue`, and SHALL keep every changed Go file within 300 counted lines.

## JSON Contract

Fields under the existing `data` envelope. `opt` fields are omitted when empty; text lines are indented under the SPEC row or in the named section.

| Field | Type | Presence | Text counterpart |
|---|---|---|---|
| `summary.verified`, `summary.evidence.<label>` | int | always | `Evidence: <n> verified · <n> <label>` (non-zero labels) |
| `specs[].completed`, `specs[].status` | bool, string | always | row icon and declared status |
| `specs[].evidence.label` | enum of the seven labels | always | row label column |
| `execution.source`, `root`, `other_roots` | string, abs path, [abs path] | with evidence; `other_roots` opt | `result: … · <source>`; `older evidence also in: <roots>` |
| `execution.error` | string | opt | `run: <error> · <source>` |
| `execution.run_id`, `task_id`, `attempt`, `resumed_from` | string, string, int, string | with a receipt; `resumed_from` opt | `run: <run_id> · task <task_id> · attempt <n>[, resumed from <id>]` |
| `execution.route`, `platform`, `execution_owner` | string; owner `omp`/`orca`/`unknown` | with a receipt; owner for `omp` | `· route <route> · platform <platform>[ (owner <owner>)]` |
| `execution.terminal`, `stop_reason`, `finished_at`, `base_head` | string, string, RFC 3339, commit id | terminal always; others opt | `result: <terminal>[ (<stop_reason>)] · finished <time> · base HEAD <first 12>` |
| `execution.phases.{executed,restored,failed,pending}`, `dispatches`, `cumulative_dispatches` | int | always | `phases: … executed · … restored · … failed · … pending · <n> dispatches (<n> cumulative)` |
| `execution.blocker`, `blocked_attempt.{source,blocker,finished_at}` | string | opt | `blocker: …`; `last attempt blocked before dispatch: …` |
| `execution.resume.{admitted,reason,restores,reruns,checked,command,command_note}` | bool, string, [string] ×3, string, string | stopped runs; `command`, `command_note` opt | `resumable: yes — <summary>` or `resumable: no — <reason>`; `resume command: …`; `admission checked: …` |
| `verification.{state,reasons,run_id,base_head,recorded_at}` | enum, [string], string ×3 | state always | `verification: <state> (run …, base HEAD <12>, recorded …) — <reasons>` unless `none` |
| `review.{state,source,run_id,verdict,gate_status,loop_status,finished_at,reasons}` | strings | review opt | `review: <verdict> (gate …, loop …, run …) at <time> — <state>: <reasons>` |
| `review.session.{state,attempts,calls,reason}` | string, int, int, string | opt | `review session: <state> · <n> attempts · <n> calls` or `unknown — <reason>` |
| `environment.{dir,installed_clis,config_found,config_error,configured_platforms,review_gate,review_providers}` | abs path, [string], bool, string, [string], bool, [string] | always; `config_error` opt | `Directory:`, `AI CLIs on PATH:`, `Configured platforms:` |
| `environment.pipeline_platform.{platform,reason,auto_selectable,configured,note}` | string, string, bool, bool, string | always; `note` opt | `auto pipeline run platform: <platform or none> — <reason>`; `note: …` |
| `environment.desktop.{state,helper_installed,worker_config,note}` | enum, bool, bool, string | always | `Desktop: <state> (<note>)` |
| `next_actions[].{id,spec_id,cwd,command,shell,forms,purpose,runs_in,provider,external_calls,writes,optional,auto_run}` | strings, `forms` [{platforms,command}], bools | always; `spec_id`, `shell`, `forms` opt | `<n>. <shell or forms>`, then `<purpose>`, then `[optional · ]in <cwd> · runs in … · provider: … · calls: … · writes: …` |

## Generated And Changed Surfaces

- `[NEW] internal/cli/status_evidence.go`, `[NEW] status_evidence_roots.go`, `[NEW] status_evidence_resume.go`, `[NEW] status_evidence_verify.go`, `[NEW] status_evidence_review.go`, `[NEW] status_evidence_render.go`: evidence, roots, admission, freshness, and rendering.
- `[NEW] internal/cli/status_next.go`, `[NEW] status_next_command.go`, `[NEW] status_next_env.go`: actions, command validation and quoting, platform and Desktop facts.
- `[NEW] internal/cli/pipeline_run_baseline.go`, `[NEW] pipeline_run_fingerprint.go`, `[NEW] pipeline_run_dryrun.go`; `pipeline_run.go`, `pipeline_run_flags.go`, `pipeline_run_resume.go`.
- `[NEW] pkg/pipeline/resume_admission.go`: `AdmitResume`, a read-only entry to `planResume`.
- `internal/cli/status.go`, `status_json.go`, `init.go`: wiring, probe injection, guidance; README rows; TUIUX goldens.

## Related SPECs

- SPEC-PIPELINE-RESUME-001: the receipt fields and resume admission this SPEC reads.
- SPEC-TUIUX-001: the init goldens this SPEC re-records.

## Traceability Matrix

| Requirement | Plan task | Acceptance | Invariants |
|---|---|---|---|
| REQ-SEV-SCOPE-01 | T7 | S12 | INV-S06 |
| REQ-SEV-DECL-01 | T1, T3 | S1 | INV-S01 |
| REQ-SEV-EXEC-01, EXEC-03 | T1, T3 | S3, S5 | INV-S03 |
| REQ-SEV-EXEC-02, EXEC-06 | T1 | S3, S16 | INV-S04 |
| REQ-SEV-EXEC-04 | T5 | S15 | INV-S01 |
| REQ-SEV-EXEC-05 | T1 | S17 | INV-S05 |
| REQ-SEV-BASE-01, BASE-02, BASE-03 | T2 | S2, S14 | INV-S01, INV-S02 |
| REQ-SEV-REVIEW-01, REVIEW-02 | T1 | S9 | INV-S02 |
| REQ-SEV-RUN-01, RUN-02 | T5 | S6, S11 | INV-S03 |
| REQ-SEV-NEXT-01, NEXT-04 | T4 | S4, S13, S18 | INV-S05, INV-S08 |
| REQ-SEV-NEXT-02 | T4 | S8 | INV-S05 |
| REQ-SEV-NEXT-03 | T4, T6 | S7, S10 | INV-S05 |
| REQ-SEV-JSON-01 | T3 | S5 | INV-S07 |
| REQ-SEV-VERIFY-01 | T7 | S3, S12 | INV-S06 |
