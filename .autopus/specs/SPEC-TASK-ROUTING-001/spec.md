---
id: SPEC-TASK-ROUTING-001
status: implemented
---
# Evidence-based execution depth

Choose inline, guided or planned execution from explicit scope, risk,
uncertainty and acceptance facts. Model quality selection remains separate;
user model/effort choices and existing safety/acceptance gates are preserved.

A small low-risk known-contract fix can use inline implementation and focused
verification. Missing facts require focused inspection, not invented certainty.
High-risk, multi-domain, unresolved requirements or repeated failures require
planning. Explicit user routes and solo/team intent remain authoritative;
a request for less process cannot waive mandatory risk checks.

Parallel work is orthogonal: require declared independent slices, bounded
ownership within parent scope, no overlap and available native capability.
Unknown scope/requirements/acceptance or --solo means serial. The decision is
advisory and based on caller declarations and path inference; it neither
launches workers nor certifies readiness or bypasses gate evaluation.

Integrate the short policy into existing platform routers and root guidance.
Do not create an additional always-loaded skill or change model presets.
