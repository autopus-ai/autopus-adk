# SPEC-QALOOP-001 — Acceptance

Every criterion is checked by a Go test unless it is marked **live**. A live
criterion needs a real browser or agent CLI, and is checked during release
verification on a sample project.

### S1: AC-QALOOP-001 — Acceptance criteria parse

GIVEN this SPEC's own `acceptance.md`
WHEN the acceptance parser runs
THEN it yields every `AC-QALOOP-*` id in document order, each with non-empty
given/when/then text and a correct line number.
AND a criterion with no THEN line is reported as `missing_then`.

### S2: AC-QALOOP-002 — v1 unchanged

GIVEN the existing v1 scenario fixtures
WHEN they are compiled
THEN the output is byte-identical to the output before this SPEC.

### S3: AC-QALOOP-003 — v2 actions compile to @journey

GIVEN a v2 acceptance scenario with `fill` (via `value_env`), `click`, and
`expect_text` steps
WHEN it is compiled
THEN the spec is tagged `@journey`, contains no `@explore`, reads
`process.env["E2E_PASSWORD"]`, and its step map labels each line `action`,
`expect`, or `goto` with the right `ac`.

### S4: AC-QALOOP-004 — v2 intent rules enforced

GIVEN v2 scenarios that break the intent rules:
- an acceptance scenario with an expect step that has no `ac`,
- an `ac` that is not in `acceptance_refs`,
- a baseline scenario with a click,
- a fill with both `value` and `value_env`, and
- an action target with two locator kinds

WHEN validated
THEN each is rejected with its own reason code.

### S5: AC-QALOOP-005 — Test scenarios become candidates

GIVEN a test-scenarios file with one `command` case (`go test ./...`), one
case whose argv[0] is `curl`, one `gui` case, and one `manual` case
WHEN candidate compilation runs
THEN the go case becomes an executable candidate with source `test-scenarios`
and its `ac` as an acceptance ref.
AND the curl case is deferred with `qa_test_scenario_command_not_allowlisted`.

### S6: AC-QALOOP-006 — Generation validates agent output

GIVEN a fake agent CLI that prints one valid v2 scenario, one scenario that
cites an unknown `ac`, and one valid test-scenarios document
WHEN `auto qa scenario generate --spec SPEC-X --agent claude` runs with
`AUTOPUS_QA_AGENT_ARGV` pointing at the fake
THEN the valid documents land in the candidate directories, the invalid one is
reported and not written, and coverage lists each criterion as covered or
`uncovered`.

### S7: AC-QALOOP-007 — Promotion gates

GIVEN candidates:
- a valid acceptance scenario,
- a recording with an unconfirmed agent assertion, and
- a candidate whose id clashes with a different active file

WHEN `promote --all` runs
THEN only the first is promoted.
AND re-running promote with `--accept-agent-assertions` promotes the second.
AND the clash is never overwritten.

### S8: AC-QALOOP-008 — Agent runner argv and overrides

GIVEN each of the four targets and both modes
WHEN the runner builds argv
THEN it matches the REQ-8 table.
AND `AUTOPUS_QA_AGENT_ARGV` replaces it, with the prompt on stdin.
AND a missing binary yields `qa_agent_cli_missing`.

### S9: AC-QALOOP-009 — Triage classes

GIVEN failure fixtures for each class (connection refused, strict-mode
violation, a failing line mapped to a click step, a failing line mapped to an
expect step, a pass on re-run, and unmatched output)
WHEN triage runs
THEN each gets exactly its class and the matched signal.

### S10: AC-QALOOP-010 — Heal guard keeps the oracle

GIVEN a before/after scenario pair where the healer changed a click target
WHEN the guard runs
THEN it accepts.

GIVEN a pair where the healer changed an `expect_text` value, dropped an
`ac`, or edited a file under `src/`
WHEN the guard runs
THEN it rejects with `qa_loop_guard_rejected` and names the offending path or
step.

### S11: AC-QALOOP-011 — Product fix may not touch tests

GIVEN a product-fix iteration whose agent edits `src/app.ts` and
`e2e/login.spec.ts`
WHEN the guard runs
THEN it rejects.
AND the revert restores `e2e/login.spec.ts`, removes only files the agent
created, and leaves pre-existing untracked files intact.

### S12: AC-QALOOP-012 — Loop end to end with fakes

GIVEN a temp git repo with a fast-lane journey that fails until a file is
fixed, and a fake agent that fixes it
WHEN `auto qa loop --lane fast --agent claude --max-iterations 3` runs
THEN it ends `passed` after one fix commit on `autopus/qa-loop/<id>`.
AND the original branch is checked out again.
AND `report.json` records the triage, guard verdict, and commit.

GIVEN a fake agent that changes nothing
WHEN the same loop runs
THEN it stops with `no_progress`.

### S13: AC-QALOOP-013 — Loop refuses a dirty tree

GIVEN uncommitted tracked changes
WHEN `auto qa loop` runs
THEN it exits without creating a branch, with `qa_loop_dirty_worktree`.

### S14: AC-QALOOP-014 — Replay section in repair prompts

GIVEN a failed GUI manifest with a step map and capture artifacts
WHEN a feedback bundle is written
THEN the prompt contains a `## Replay` section naming the failing step, its
kind, its `ac`, and the trace and screenshot paths, and no raw artifact bytes.

### S15: AC-QALOOP-015 — Codegen import

GIVEN a codegen file with goto, getByRole click, getByLabel fill, press Enter,
`expect(getByText(...)).toBeVisible()`, `toHaveURL`, and one `page.mouse.click`
line
WHEN imported without `--allow-partial`
THEN it fails and lists the mouse line number.

WHEN imported with `--allow-partial`
THEN it writes a valid v2 recording candidate with those steps in order.

### S16: AC-QALOOP-016 — Agent JSONL import

GIVEN a recording JSONL with human actions, one human expect, and one agent
expect without `ac`
WHEN imported
THEN the agent expect is marked `confirm: required`.

### S17: AC-QALOOP-017 — Discovery emits a baseline candidate

GIVEN crawl output JSON for three pages
WHEN discovery converts it
THEN it writes one baseline candidate with three screens, no action steps, and
title and heading expectations.
AND the generated crawler script calls only `goto`.
AND an origin not allowed by any pack is refused unless passed with
`--origin`.

### S18: AC-QALOOP-018 — Init emits the gui-journey pack

GIVEN a project with a Playwright config
WHEN `auto qa init` runs on it
THEN the capture README carries a `gui-journey` example in lane
`browser-staging` that greps `@journey`.

### S19: AC-QALOOP-019 — live: end to end on a sample web app

GIVEN a small sample web app with a login form and a SPEC with three
acceptance criteria
WHEN `generate → promote --all → compile → loop` runs against it with the
claude agent
THEN scenarios are generated for every criterion, the journeys run, an
injected product bug is triaged `product_defect` and fixed on the loop branch,
and the final run passes.
