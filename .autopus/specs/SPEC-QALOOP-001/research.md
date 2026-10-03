# SPEC-QALOOP-001 — Research

## Source: "토스 FE 개발자들의 검증법" (Toss FE webinar, 2026-08-25)

Korean auto-captions were pulled with yt-dlp, and the whole talk was read.
Findings that shaped this SPEC:

1. **Code-derived generation failed.** The team first treated code as the
   single source of truth: infer screen flows from it, build scenarios, then
   generate E2E tests. "It didn't work... code contains everything, but too
   much; what matters is the intent of the person who built or designed it,
   and that intent is not visible in code." → P-1, and the reason generation
   is anchored to acceptance criteria (REQ-6).
2. **Conversational recording.** The agent drives a real device from natural
   language ("start a new user, log in, hand it to me"). The human clicks what
   the agent cannot, and the system captures those clicks too. "Turn what we
   did into E2E code" then generates the test. → REQ-14..16.
3. **Replay bundle for self-repair.** On failure, the video, device logs, and
   every API call as JSONL are saved, and the agent is told "look at this and
   fix it". The cited case: a CTA label changed from "임무를 수행하라" to
   "가서 살펴보세요", and the agent repaired the locator. → REQ-9 `test_drift`,
   REQ-13. Our refinement: the heal may change the locator, but an assertion
   about that label is an oracle and goes to a person (REQ-11).
4. **Deterministic, surface-only.** Tests minimise dependencies, check only
   what the user experiences, and remove anything that sometimes works. →
   flaky class with one re-run (REQ-9), role-first locators.
5. **Testable environment.** They had to request test APIs (auth, seeding) from
   service teams. → `value_env`, plus the `environment` class that stops
   instead of "fixing" code.
6. **Real-user logs as scenario source.** They mined ELK action logs by user
   number. Out of scope here; discovery (REQ-17) is the web-app analogue that
   needs no log infrastructure.
7. **Focus on high-value surfaces.** Payments and shopping first, because one
   escape costs a lot. This is why criteria-level coverage (REQ-6 output)
   matters more than a raw test count.

Toss Tech, "AI-driven UI test automation": a three-tier readiness check and
click fallbacks for React timing, unique test users per run, and a centralised
consent handler. Playwright's auto-waiting covers the readiness tiers.
`value_env` and per-run data are the project's responsibility.

## Current state (2026-10-03, autopus-adk 55fe979f)

- Nothing in Go generates scenarios. `regen.Synthesize` returns fixed starter
  packs (`pkg/qa/regen/synthesize.go:9`, "NOT behavioral extraction").
- `qa scenario init` writes `expect_title: EDIT-ME`
  (`pkg/qa/scenario/starter.go:48`). The v1 vocabulary is read-only by design
  (`pkg/qa/scenario/types.go:44-58`) because it compiles to `@explore`, which
  the gui-explore mutation guard protects.
- Acceptance criteria reach QA only through hand-written `qamesh-check` fenced
  blocks (`pkg/qa/compile/compiler.go:80`).
- `--feedback-to` writes a repair prompt bundle
  (`pkg/qa/evidence/feedback_prompt.go`). No code reads it, so no loop exists.
- `pkg/qualityloop/classify.go` classifies improvement candidates by reason
  code. It does not read Playwright output, so it cannot separate drift from a
  product bug.
- The capture fixture already records per-step console, network, screenshot,
  and trace evidence. The replay bundle only has to cite it.

## Agent CLI flags (verified locally 2026-10-03)

- `claude -p`, `--output-format text`, `--permission-mode acceptEdits`
- `codex exec --skip-git-repo-check --sandbox read-only|workspace-write -o <file>`
  (`-` reads the prompt from stdin)
- `agy -p`, `--mode accept-edits|plan`
- `opencode run [message...]`
