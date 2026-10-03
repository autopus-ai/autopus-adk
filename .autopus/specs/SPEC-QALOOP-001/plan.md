# SPEC-QALOOP-001 — Plan

Three slices. Slice A defines every type the other two consume, so it lands
first. B and C then run in parallel in separate worktrees with disjoint
ownership. Every file stays under the 300-line ceiling
(`architecture.max_file_lines`).

## Slice A — Intent schema and agent runner (REQ-1..5, 8)

Owned paths:
- `pkg/qa/acceptance/` (new) — `Parse(path) ([]Criterion, []Problem, error)`
  and `ParseSpec(projectDir, specID)`.
- `pkg/qa/scenario/`:
  - v2 types: `IntentSource`, `SpecID`, `RecordingRef`, action fields on
    `Step`, `Target`, `FillAction`, `PressAction`, `SelectAction`, `Ac`,
    `Confirm`.
  - v2 validation, v2 compile, and `@journey` / `@baseline` tags.
  - Step map writer `StepMap{SchemaVersion, Spec, Lines map[int]StepRef}`
    written as `<id>.spec.map.json`.
  - `CandidatesDirRel = .autopus/qa/scenarios/candidates`.
  - `LoadCandidates`.
  - `ParseBytes` for in-memory validation of agent output.
- `pkg/qa/testscenario/` (new) — types, strict loader, validator, the
  candidates directory, and `CommandAllowlist`.
- `pkg/qa/compile/` — read `.autopus/qa/test-scenarios/*.yaml` as a candidate
  source.
- `pkg/qa/agentexec/` (new):
  - `Target`, `Mode` (generate|edit), `Request`, `Response`, and
    `Runner.Run(ctx, Request)`.
  - Argv table and `AUTOPUS_QA_AGENT_ARGV` override.
  - A `LookPath` seam and an `exec` seam for tests.
  - The setup-gap error `qa_agent_cli_missing`.

v1 compile output must stay byte-identical; golden-test it before touching
`compile.go`.

## Slice B — Triage, loop, replay (REQ-9..13)

Owned paths:
- `pkg/qa/triage/` (new) — `Classify(input) Verdict`, where the input is
  manifest status, setup gap, failure text, step maps, and the re-run result.
- `pkg/qa/loop/` (new):
  - git helpers: clean check, branch, changed paths, untracked baseline,
    revert, commit.
  - Class policy allowlists, heal guard, fingerprint, iteration driver, and
    report writer.
  - The driver depends on `run.Execute` through an interface so it can be
    tested with fakes.
- `pkg/qa/evidence/` — Replay section in `feedback_prompt.go` (a new file if
  `feedback_prompt.go` would exceed 300 lines).
- `internal/cli/qa_loop*.go` — the `auto qa loop` command, registered in
  `qa.go`.

## Slice C — Generation, promotion, recording, discovery (REQ-6, 7, 14..17)

Owned paths:
- `pkg/qa/generate/` (new) — prompt builder, fenced-YAML extraction, validation
  against the criteria, candidate writer, and coverage.
- `pkg/qa/promote/` (new) — promotion rules.
- `pkg/qa/record/` (new) — codegen JS parser, JSONL parser, and
  `ToScenario`.
- `pkg/qa/discover/` (new) — crawler script template, crawl JSON decoding,
  baseline scenario builder, and origin allowlist check.
- `internal/cli/`:
  - `qa_scenario_generate.go`, `qa_scenario_promote.go`, `qa_record.go`, and
    `qa_discover.go`.
  - Registration in `qa.go`, and in `qa_scenario.go` for generate and promote.

## Integration (main session, after merging)

- `pkg/qa/scaffold`: the gui-journey example (REQ-18).
- `content/skills/qa-autopilot.md` and the agent-pipeline verification
  reference (REQ-19).
- Docs and CHANGELOG.
- Full `go test ./...`, lint, the file-size gate, and the live AC-QALOOP-019
  run on a sample app.
