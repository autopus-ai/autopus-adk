---
id: SPEC-ORCH-018
title: Orchestra Debate Stability & Yield-Rounds Mode
status: completed
note: Superseded by SPEC-PANERM-001 (the orchestra pane backend was retired on 2026-10-07)
priority: P0
target_module: autopus-adk
created: 2026-03-31
---

# SPEC-ORCH-018: Orchestra Debate Stability & Yield-Rounds Mode

## Problem Statement

The Orchestra interactive debate mode has multiple stability issues observed during a real `auto orchestra brainstorm --strategy debate` run with 3 providers (claude, codex, gemini):

1. **Pane recreation destroys context**: `sendPromptWithRetry()` in `surface_manager.go:151-180` immediately calls `recreatePane()` when `SendLongText` fails, instead of retrying on the existing pane first. This destroys conversation context accumulated in Round 1.

2. **Incomplete result capture**: Gemini completed in 2.53s but its full output (SCAMPER + ICE tables) may not have been fully captured by `ReadScreen` with scrollback, leading to incomplete rebuttal context in Round 2.

3. **Round 2 entry failure**: After pane recreation, codex didn't reach a ready state before prompt injection. `pollUntilPrompt` has only a 10s timeout (`interactive_debate.go:230`) which is insufficient for provider restart.

4. **Judge runs as subprocess without project context**: `runJudgeRound()` in `interactive_debate_helpers.go:48-76` executes the judge as a subprocess via `runProvider()`. The subprocess has no project context, making judgment superficial. The main Claude Code session should act as judge instead.

5. **No project context in brainstorm prompt**: `buildBrainstormPrompt()` in `orchestra_brainstorm.go:54-78` only includes the raw feature description. `topicIsolationInstruction` in `debate.go:95` actively prevents providers from reading project files. Providers brainstorm in a vacuum.

## Root Cause Analysis

### sendPromptWithRetry Flow (current, broken)

```
SendLongText(existing pane) -> FAIL
  -> recreatePane() immediately (context lost!)
  -> retry SendLongText 3x on NEW pane -> may also fail (session not ready)
```

### Desired Flow

```
SendLongText(existing pane) -> FAIL
  -> wait for prompt visibility (pollUntilPrompt with longer timeout)
  -> retry SendLongText on SAME pane 2x with backoff (2s, 4s)
  -> ONLY if all retries fail -> recreatePane as last resort
  -> waitForSessionReady on new pane -> then retry
```

### Key Files

| File | Lines | Issue |
|------|-------|-------|
| `pkg/orchestra/surface_manager.go` | 151-180 | Immediate pane recreation on first failure |
| `pkg/orchestra/interactive_debate.go` | 225-230 | pollUntilPrompt 10s timeout too short |
| `pkg/orchestra/interactive.go` | 245-288 | ReadScreen scrollback depth not configurable |
| `pkg/orchestra/interactive_debate_helpers.go` | 48-76 | Judge runs as subprocess |
| `internal/cli/orchestra_brainstorm.go` | 54-78 | No project context in prompt |
| `pkg/orchestra/debate.go` | 93-95 | topicIsolationInstruction blocks file reading |

## Requirements

### P0 -- Must Have

**R1 (Retry-first on SendLongText failure)**:
WHEN `SendLongText` fails on an existing pane, THE SYSTEM SHALL retry on the same pane up to 2 times with exponential backoff (2s, 4s) before attempting pane recreation as a last resort. Pane recreation SHALL only occur after all same-pane retries are exhausted.

**R2 (pollUntilPrompt timeout increase)**:
WHEN entering Round 2 or later, THE SYSTEM SHALL wait for the provider's prompt pattern to become visible with a timeout of 30s (increased from 10s), with 3s polling interval.

**R3 (ReadScreen scrollback depth)**:
WHEN collecting provider results via `ReadScreen`, THE SYSTEM SHALL request scrollback with a minimum of 500 lines to capture long outputs (e.g., SCAMPER + ICE tables from Gemini). The scrollback depth SHALL be configurable via `OrchestraConfig.ScrollbackLines` with a default of 500.

**R4 (--no-judge flag)**:
WHEN the `--no-judge` flag is passed to any orchestra command, THE SYSTEM SHALL skip the judge phase entirely and output only the raw provider responses. The flag SHALL be added to `OrchestraConfig` as `NoJudge bool`.

**R5 (--yield-rounds mode)**:
WHEN the `--yield-rounds` flag is passed to `auto orchestra brainstorm`, THE SYSTEM SHALL:
- Run Round 1 and collect results
- Output a JSON object to stdout containing: round number, pane IDs (surface refs), provider responses, session ID
- Keep panes alive (do NOT clean up)
- Exit with code 0
- The flag SHALL be added to `OrchestraConfig` as `YieldRounds bool`

**R6 (auto orchestra collect command)**:
THE SYSTEM SHALL provide an `auto orchestra collect --session-id <ID> --round <N>` command that:
- Reads the screens of existing panes identified by session ID
- Outputs collected results as JSON to stdout
- Does NOT clean up panes

**R7 (auto orchestra cleanup command)**:
THE SYSTEM SHALL provide an `auto orchestra cleanup --session-id <ID>` command that:
- Cleans up all panes for a given session
- Removes the session persistence file
- Outputs cleanup confirmation to stderr

### P1 -- Should Have

**R8 (brainstorm --context flag)**:
WHEN the `--context` flag is passed to `auto orchestra brainstorm`, THE SYSTEM SHALL:
- Load project context files: `ARCHITECTURE.md`, `.autopus/project/product.md`, `.autopus/project/structure.md`
- Inject a summarized project context section into the brainstorm prompt before the SCAMPER framework
- Replace `topicIsolationInstruction` with a context-aware instruction: "Use the project context below to ground your ideas in the actual codebase. Focus on the given topic."

**R9 (Structured JSON output for --no-judge)**:
WHEN `--no-judge` is active, THE SYSTEM SHALL output results in this JSON format:

```json
{
  "strategy": "debate",
  "rounds": 2,
  "round_history": [
    {
      "round": 1,
      "responses": [
        {"provider": "claude", "output": "...", "duration_ms": 147000, "timed_out": false},
        {"provider": "gemini", "output": "...", "duration_ms": 2530, "timed_out": false},
        {"provider": "codex", "output": "...", "duration_ms": 120000, "timed_out": false}
      ]
    }
  ],
  "panes": {"claude": "surface:453", "gemini": "surface:455", "codex": "surface:454"},
  "session_id": "orch-abc123"
}
```

**R10 (idea.md skill update for main-session judge)**:
The `/auto idea` skill's Step 3 and Step 4 SHALL be updated to:
- Step 3: Call `auto orchestra brainstorm --no-judge --yield-rounds` and parse the JSON output
- Step 3.5 (new): Main session reviews Round 1 results, prepares enriched rebuttal prompt with project context
- Step 3.6 (new): Main session sends rebuttal prompt to each pane via `cmux set-buffer/paste-buffer` (or via `auto orchestra inject`)
- Step 3.7 (new): Call `auto orchestra collect --session-id <ID> --round 2` to get Round 2 results
- Step 3.8 (new): Call `auto orchestra cleanup --session-id <ID>`
- Step 4: Main session does ICE scoring with full project context (replaces subprocess judge)

### P2 -- Could Have

**R11 (auto orchestra inject command)**:
THE SYSTEM SHALL provide an `auto orchestra inject --session-id <ID> --provider <name> "prompt"` command that sends a prompt to a specific provider's pane, abstracting the cmux `set-buffer`/`paste-buffer` details.

**R12 (Session persistence file)**:
WHEN `--yield-rounds` creates a session, THE SYSTEM SHALL exclusively create session metadata in the private `/tmp/autopus-orchestra-sessions/{ID}.json` path (directory `0700`, file `0600`) containing pane IDs, provider configs, and round history, for use by `collect`, `inject`, and `cleanup` commands. For patch-release compatibility, reads and removals SHALL fall back to the legacy `/tmp/autopus-orch-session-{ID}.json` basename only when the private entry does not exist, and SHALL reject symlinks, non-regular entries, identity changes, or a persisted ID that differs from the requested ID.

## File Inventory

| File | Change Type | Description |
|------|-------------|-------------|
| `pkg/orchestra/surface_manager.go` | Modify | R1: retry-first logic in sendPromptWithRetry |
| `pkg/orchestra/interactive_debate.go` | Modify | R2: pollUntilPrompt timeout 10s->30s, R5: yield-rounds exit point |
| `pkg/orchestra/interactive.go` | Modify | R3: ReadScreen scrollback depth parameter |
| `pkg/orchestra/interactive_debate_helpers.go` | Modify | R4: skip judge when --no-judge |
| `internal/cli/orchestra_brainstorm.go` | Modify | R4: --no-judge flag, R5: --yield-rounds flag, R8: --context flag |
| `internal/cli/orchestra.go` | Modify | R4: pass NoJudge to config, R5: pass YieldRounds to config |
| `pkg/orchestra/types.go` | Modify | Add NoJudge, YieldRounds, ScrollbackLines, ContextMode fields |
| `pkg/orchestra/debate.go` | Modify | R8: conditional topic isolation |
| `internal/cli/orchestra_collect.go` | New | R6: collect command |
| `internal/cli/orchestra_cleanup.go` | New | R7: cleanup command |
| `internal/cli/orchestra_inject.go` | New (P2) | R11: inject command |
| `pkg/orchestra/session.go` | New | R12: session persistence |
| `pkg/orchestra/yield.go` | New | R5/R9: yield-rounds JSON output |
| `.claude/skills/autopus/idea.md` | Modify | R10: main-session judge flow |

## Backward Compatibility

- All new flags (`--no-judge`, `--yield-rounds`, `--context`) are opt-in. Default behavior is unchanged.
- `ScrollbackLines` defaults to 500 (previously unlimited/default tmux scrollback). This may return more data than before but does not change semantics.
- `pollUntilPrompt` timeout change (10s -> 30s) only affects Round 2+ entry timing. Round 1 is unaffected.
- The `sendPromptWithRetry` retry-first change preserves the existing pane recreation as a fallback, so no behavior is removed.
