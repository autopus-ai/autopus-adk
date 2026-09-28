---
name: auto
description: Autopus 명령 라우터 — oh-my-pi helper
compatibility: omp
---

# Autopus 명령 라우터

`$ARGUMENTS`

## Task Triage
- For natural-language implementation requests, assess evidence before loading a planning workflow or dispatching workers. Do not reroute explicit `plan`, `review`, or other routes, or replace requested `--team`, `--multi`, or `--solo` behavior.
- `inline`: a bounded, evidenced low-risk change stays in this session with targeted implementation and tests; do not add a default planner or preload unrelated skills.
- `guided`: incomplete evidence or medium risk calls for focused inspection, then implementation and the verification appropriate to the discovered surface. Inspect uncertainty before treating it as a requirement for a full plan.
- `planned`: confirmed cross-module work, high risk, or requirements still unclear after inspection use planning and required review. A plan does not itself authorize parallel dispatch.
- When explicit task facts are available, optional read-only `auto workflow triage --facts-json <file> --format json` gives deterministic advice, not an authorization or proof of runtime capacity. Do not fabricate facts to obtain a cheaper route.
- If the CLI is unavailable, apply the same conservative evidence-based policy and continue; do not block solely on the advisory command. A trivial question or non-implementation request needs no triage CLI call.
- Parallel work requires independent ready slices, disjoint ownership, and observed native capacity. Honor `--solo`; file count alone never triggers workers.
- Preserve mandatory security, UX, and coverage gates from the selected workflow and project policy. Triage cannot waive them or replace acceptance evidence.
- Inherit the requested model and reasoning settings. Do not lower the model or effort to make a route appear cheaper.

## Router Contract

Treat the payload above as the complete text supplied after `/auto`.
Route to exactly one matching detail skill from this exact map: `auto-setup`, `auto-status`, `auto-goal`, `auto-update`, `auto-plan`, `auto-go`, `auto-fix`, `auto-review`, `auto-sync`, `auto-idea`, `auto-map`, `auto-why`, `auto-verify`, `auto-secure`, `auto-test`, `auto-qa`, `auto-dev`, `auto-canary`, `auto-doctor`.
Preserve `--model <provider/model>` and `--variant <value>` exactly as supplied.
For `go`, preserve at most one `--execution-owner omp|orca` pair exactly; omission defaults to `omp`, and aliases or fuzzy correction are forbidden.
Do not fuzzy-correct an unknown subcommand; report it as unsupported and show the exact map.
This is an emitted routing contract, not an OMP runtime parser or model-quality claim.
