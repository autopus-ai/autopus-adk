# SPEC-STATUSEVIDENCE-001 Acceptance Criteria

Every scenario uses a temporary git project. Checkpoints that a resume could read are made by the real pipeline engine with a scripted backend, so they carry the engine's own output records. Status sees the machine through an injected probe (installed CLIs, Desktop helper, home), never the real `PATH` or home, except S12, which puts trap executables on `PATH` on purpose.

## Test Scenarios

### S1: AC-SEV-001 - A declared done SPEC without evidence is not verified
Priority: Must
Given a project whose only SPEC declares `**Status**: done` and has no pipeline state
When `auto status` and `auto status --json` run
Then the row reads `SPEC-DONE-001  ✓ done  documented_only`, the declared line still reads `전체: 1/1 완료`, and the evidence line reads `Evidence: 0 verified · 1 documented_only`
And the JSON has `status` `done`, `completed` true, `evidence.label` `documented_only`, verification `none`, no execution, `summary.verified` 0, and `summary.evidence.documented_only` 1 (test `TestStatusEvidence_DocumentedDoneWithoutReceiptIsNotVerified`).

### S2: AC-SEV-002 - Verification goes stale when code or requirements change, not when the verified tree is committed
Given a SPEC whose engine-completed run recorded a baseline over an edited working tree
When status runs, then after committing that tree, after a later edit, after undoing it, after a status-line edit, and after an acceptance criterion is added
Then the label and state read `verified`/`fresh`, `verified`/`fresh`, `stale`/`stale` with `working tree changed since verification: pkg/app/app.go`, `verified`/`fresh`, `verified`/`fresh`, and `stale`/`stale` with `SPEC documents changed since verification`
And without the baseline the run reads `unverified`/`unknown` with `no verification baseline`
And only a completed run records a baseline, mode 0600; outside git the run prints `Pipeline verification baseline not recorded:`; a write that fails leaves the previous baseline byte for byte (tests `TestStatusEvidence_VerificationGoesStaleWhenCodeChanges`, `TestPipelineBaseline_RecordedOnlyForCompletedRuns`, `TestPipelineBaseline_WriteFailureIsReportedAndAtomic`, `TestPipelineBaseline_FingerprintIgnoresCommitsAndCatchesEdits`, `TestPipelineBaseline_StatusLinesDoNotChangeTheSpecFingerprint`).

### S3: AC-SEV-003 - A stopped run is resumable exactly when --continue admits it
Given an engine-made checkpoint: attempt 1 cancelled in validate; attempt 2 resumed, restoring plan, test_scaffold, and implement, failed validation, dispatched the fix to implement, failed the same way, and stopped on the breaker
When status runs
Then it reads `run: <run> · task <task> · attempt 2, resumed from <run 1> · route full · platform codex`, `result: blocked (circuit_break_no_progress) · finished <time> · base HEAD <12> · .autopus/pipeline-state/SPEC-RUN-001.yaml`, `phases: 1 executed · 2 restored · 1 failed · 1 pending · 3 dispatches (7 cumulative)`, `resumable: yes — the engine admits it; restores plan, test_scaffold, implement; dispatches again none`, and `resume command: auto pipeline run SPEC-RUN-001 --continue --platform codex`
And for each of unchanged, HEAD moved, SPEC changed, an implement output tampered, a broken dependency, an unknown checkpoint version, a changed route, and no git, status's admission agrees with the real `auto pipeline run <SPEC> --continue --dry-run`, refusing with the same reason, and the canonical checkpoint is left byte for byte
And a newer `<SPEC>.blocked.yaml` adds `last attempt blocked before dispatch:` with its blocker (tests `TestStatusEvidence_FailedRunIsResumableUntilHEADMoves`, `TestStatusEvidence_AdmissionAgreesWithContinue`, `TestStatusEvidence_RefusedResumeIsReported`, `TestAdmitResume_AgreesWithTheEngine`).

### S4: AC-SEV-004 - Work without a SPEC is offered beside planning
Given a project with no SPEC and the platforms `claude-code` and `codex`
When status runs
Then the next actions include `/auto fix "<problem>" (claude-code) · $codex-auto-fix "<problem>" (codex)` and `/auto plan "<outcome>" (claude-code) · $codex-auto-plan "<outcome>" (codex)`, each with `runs_in` `agent_session`, `cwd` the absolute project directory, and `auto_run` false (test `TestStatusNext_NoSpecOffersWorkWithoutASpec`).

### S5: AC-SEV-005 - JSON and text carry the same meaning
Given a stopped run, a verified run, an approved SPEC, a reviewed draft with a session, an installed codex, and an installed Desktop helper
When `auto status` and `auto status --json` run
Then every field of the JSON contract appears in the text as the contract spells it, and the next actions are resume, implement, fix, plan, and desktop_connect, all with `auto_run` false (test `TestStatusEvidence_JSONAndTextAgree`).

### S6: AC-SEV-006 - A restored phase that a fix dispatches again counts as executed
Given a resume that restored plan, test_scaffold, and implement, whose validation failed, whose fix dispatched implement, and whose validation and review then passed
When the run completes
Then the line is exactly `Pipeline complete: 3 phases executed, 2 restored from checkpoint, 4 dispatches`, and a fresh run reads `5 phases executed, 0 restored from checkpoint, 5 dispatches` (test `TestPipelineRunSummary_SeparatesExecutedAndRestored`).

### S7: AC-SEV-007 - Desktop is optional
Given no Desktop helper, then the helper without a worker config, then a worker config
When status runs
Then it reads `Desktop: not installed (optional; the ADK works standalone)` with no Desktop command, then offers `cd <dir> && auto connect` as `optional · in <dir> · runs in terminal · provider: none`, then reads `Desktop: connected (worker config found)` with no connect action (test `TestStatusNext_DesktopIsOptional`).

### S8: AC-SEV-008 - Pipeline commands follow what is installed
Given only `codex` configured and, in turn, `claude, codex, agy`; only `omp`; only `opencode`; nothing
When status runs for a SPEC whose run cannot resume
Then the platform line reads `claude — first installed of claude → codex → agy` with the note naming `--platform codex` and a new run with `--platform claude`; then `omp — … runs only with --platform omp` with `--platform omp`; then `none — no pipeline platform is installed (claude, codex, agy, or omp)` with no pipeline command and `auto doctor` first, its purpose naming the missing pipeline platform, then naming that no AI CLI is installed (test `TestStatusNext_PipelinePlatformFollowsWhatIsInstalled`).

### S9: AC-SEV-009 - Review freshness is proven by content, never by modification time
Given a receipt whose `finished_at` is 10:00 and whose file is dated 12:00
When it records the current SPEC fingerprint, then a status-line edit, then a plan change
Then the review reads `review: REVISE (gate blocked, loop revisions_exhausted, run orch-1) at 2026-11-01T10:00:00Z — fresh`, stays fresh, then reads `stale` with `SPEC requirements changed after the review`
And a receipt without a fingerprint reads `unknown`, stays `unknown` after a change that kept its old modification time, reads `stale` with `SPEC documents written after the review: plan.md` after a newer write, and `unknown` when spec.md is gone
And sessions and foreign receipts read as in REQ-SEV-REVIEW-02 (tests `TestStatusEvidence_ReviewFreshnessComparesContent`, `TestStatusEvidence_ReviewWithoutFingerprintIsNeverFresh`, `TestStatusEvidence_ReviewSessionIsReadTolerantly`).

### S10: AC-SEV-010 - Init leads with standalone use and states each step's facts
Given `auto init` with platforms `claude-code` and `codex`
When it prints its summary
Then the steps are plan, fix, and the optional Desktop connection, each followed by `in <dir> · runs in … · provider: … · calls: … · writes: …`, and no line before the Desktop step names Desktop (tests `TestInitGuidance_StandaloneFirst`, `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`).

### S11: AC-SEV-011 - The pipeline names its platform and why
Given `--platform codex`, then `codex` and `agy` on `PATH`, then only `agy`, then nothing
When the platform is resolved
Then the reasons read `--platform`, `first installed of claude → codex → agy` twice (agy as `gemini (agy)`), and `no AI CLI installed; falls back to claude` (tests `TestPipelinePlatformSelection_Reasons`, `TestPipelineRunCmd_DryRunKeepsTheLastRealRun`).

### S12: AC-SEV-012 - Status writes nothing and runs nothing but read-only git
Given trap executables for every AI CLI, orca, npx, node, and the Desktop helper on `PATH`, and a logging git wrapper
When status runs in text and JSON
Then no trap ran, every git call is rev-parse, diff, ls-files, status, or check-ignore, and every project file outside `.git` keeps its mode, content, and time; the branch touches only the paths of the module ownership; no changed Go file exceeds 300 counted lines (test `TestStatusEvidence_StatusRunsNothingButReadOnlyGit`).

### S13: AC-SEV-013 - Commands carry only validated values
Given a receipt whose platform is `codex; touch PWN` and whose blocker holds an escape sequence and a newline, a SPEC directory `SPEC-X;touch PWN`, and a project directory named `it's a project`
When status runs
Then the resume reads admitted with no command and the note `the run's recorded platform is not one auto pipeline run accepts`, no action command holds `PWN` or names that SPEC, the new run reads `cd '<dir with '\''>' && auto pipeline run SPEC-INJECT-001 --platform codex`, and the blocker shows no control character (test `TestStatusNext_CommandsCarryOnlyValidatedValues`).

### S14: AC-SEV-014 - Hidden changes and invalid baselines are never fresh
Given a verified SPEC
When a tracked file is marked assume-unchanged or skip-worktree and edited, when only its executable bit changes under `core.fileMode=false`, and when the baseline loses `tree_paths`, carries a wrong aggregate, a malformed entry or field, or names another SPEC
Then the first reads `unverified` with `git hides changes to tracked files: 1 marked assume-unchanged or skip-worktree: pkg/app/other.go`, the second `stale`, and the rest `unverified` with `verification baseline is invalid: <what>`, and no baseline is recorded over a hidden change (tests `TestStatusEvidence_HiddenChangesAreNeverFresh`, `TestStatusEvidence_InvalidBaselineIsNeverFresh`, `TestPipelineBaseline_RefusesATreeGitWouldHide`).

### S15: AC-SEV-015 - A dry run keeps the last real run
Given a completed run with its baseline
When `auto pipeline run <SPEC> --dry-run --platform codex` runs
Then it prints `Pipeline platform: codex — --platform`, writes `.autopus/pipeline-state/dry-run/<SPEC>.yaml`, leaves the checkpoint and baseline byte for byte, and status still reads the completed run as verified (tests `TestPipelineRunCmd_DryRunKeepsTheLastRealRun`, `TestPipelineRunCmd_AuthorizedCompactContractDispatchesFewerPhases`).

### S16: AC-SEV-016 - The execution owner comes from the run's receipt
Given an OMP run whose receipt records backend `orca`, `rpc-canonical-full`, or no dispatch, and an owner sidecar rewritten to `omp`
When status runs
Then the resume commands end `--execution-owner orca`, without an owner, and without an owner with the note that the owner is unknown (test `TestStatusEvidence_OwnerComesFromTheRunReceipt`).

### S17: AC-SEV-017 - The newest state root wins
Given runs of a module SPEC started from its module and later from the workspace root
When status runs, and again after the workspace run is dated an hour earlier
Then it reads the workspace run with the module root in `other_roots`, then the module run with `older evidence also in: <workspace>` (test `TestStatusEvidence_NewestStateRootWins`).

### S18: AC-SEV-018 - Every command names its absolute directory
Given a module SPEC whose module configures `codex` while the workspace configures `claude-code`, and status run with `--dir` from another directory
When the actions are listed
Then every `cwd` is absolute, every terminal `shell` starts with `cd <cwd> && `, the module SPEC's implement action runs in the module as `$codex-auto-go SPEC-MOD-001`, and the workspace's fix action stays `/auto fix "<problem>"` (test `TestStatusNext_EveryCommandNamesItsDirectory`).

## Oracle Acceptance Notes

Every oracle is a fixed string, count, label, or reason, or — for S3 and S5 — the agreement between two independent outputs: status against the real `--continue`, and the text against the JSON contract table. Engine-made checkpoints carry random run IDs, so those oracles read the IDs from the receipt rather than recomputing them.
