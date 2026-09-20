# Acceptance

- AC1: evidenced small low-risk fix -> inline; missing evidence -> guided;
  single-file security/high risk and repeated failures -> planned.
- AC2: high risk cannot be downgraded by requested inline; model stays inherited
  and mandatory gate preservation is explicit in all decisions.
- AC3: parallel only for independent disjoint owned work inside known parent
  scope with native availability; unknown, overlap or solo remain serial.
- AC4: strict bounded input and read-only CLI preserve input/project files;
  report caller-declared provenance without capability certification.
- AC5: existing platform routers expose the same compact policy, respect user
  route choices, and avoid unrelated skill loading or compulsory triage calls.
- AC6: relevant tests, candidate execution, review and generated-surface checks
  have recorded outcomes. Routing speed/quality improvement is not inferred
  from deterministic tests or the earlier single-agent exposure pilot.

## Executed evidence, 2026-09-20

AC1-AC6 PASS for the implementation scope; no performance promotion is claimed.

- Core race tests cover positive inline evidence, missing facts, single-file
  sensitive paths, explicit lower/higher requests, retries, size thresholds,
  independent ownership, scope containment, overlap, solo and strict decoding.
- CLI race tests cover the route outcomes, provenance, read-only behavior,
  malformed/duplicate/oversized input, file identity, formats and repeated flags.
- Real candidate CLI examples: small-fix -> inline/serial; uncertain ->
  guided/serial; sensitive -> planned/serial; independent -> planned/parallel.
- Generated router tests passed for Claude, Codex, Gemini, OpenCode and OMP.
  OpenCode/OMP custom renderers now import the canonical short policy, rather
  than relying on a template they do not actually render. Explicit route/model
  intent is retained and old unconditional subagent-first wording is removed.
- Domain race suite passed: taskroute, content, OpenCode, OMP and templates.
  OMP complete suite took 259.779s. Focused OMP routing/workflow smoke also passed.
- vet passed for taskroute, CLI, OMP, OpenCode and templates. Candidate build,
  generated-surface parity, diff check and sync verify passed.
- Review TR-001 closed: aggregate ownership bounded to 256 before comparisons,
  with 256/257 boundary tests. TR-002 closed: all planned routes require
  review_changes; security_review remains additive for sensitive work.
- Live example exposed module-directory scope classification loss; fixed locally
  in taskroute, with tests for different modules, same-module directory/file and
  two source files in one generic container. Shared gates classifier unchanged.

Logs: /tmp/task-route-domain.log, /tmp/task-route-cli.log,
/tmp/task-route-final-core-cli.log, /tmp/task-route-surface-green.log,
/tmp/task-route-omp-focused.log, /tmp/task-route-vet.log.
CLI receipts: /tmp/autopus-task-routing-20260920/.
Copied local-only evidence: .autopus/runtime/task-routing/2026-09-20/.

No new model call, automatic worker dispatch, required-gate exemption, model
preset change, release, commit or push is performed by this change. Existing
benchmark artifacts and local generated OMP drift remain preserved.
