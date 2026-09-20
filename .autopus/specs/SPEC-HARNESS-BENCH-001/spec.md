---
id: SPEC-HARNESS-BENCH-001
status: implemented
---
# Single-agent instruction and skill exposure pilot

Measure native versus compact-default versus full-catalog Codex surfaces using
12 frozen local regression-repair tasks, three arms, one trial per task/arm.
Baseline commit a05ce69df9dc03493b8c5de0ab9299459195e8d8. This pilot does not
represent new-feature delivery, long-running tasks, or multi-agent productivity.

Primary outcome: independent existing acceptance tests AND scope integrity.
Secondary: elapsed agent time, reported input/output tokens, cached-input subset,
operational failure, timeout and command action counts. USD cost unknown. Human
corrections zero by design, not evidence of lower human maintenance burden.

All arms use Codex 0.155.1, gpt-6-astra medium, same process deadline, permissions,
frozen task prompt and oracle. Provider-side model revision/cache cannot be
pinned; record that limitation. Rotate all six arm permutations twice, serial
execution, no favorable task selection or outcome-driven retry. All attempts
remain in reports. Existing system/account instructions are shared controls.

Native: no project harness. Reduced: existing split default (38 project skills).
Current/full: explicit full catalog (71 project skills), not the product default.
Root instructions equal between harness arms. Hooks, multi-agent and memory
features disabled in every arm: this isolates published instruction/skill exposure.
