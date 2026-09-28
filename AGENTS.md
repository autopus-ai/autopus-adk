<!-- AUTOPUS:BEGIN -->
# Autopus-ADK Harness

> 이 섹션은 Autopus-ADK에 의해 자동 생성됩니다. 수동으로 편집하지 마세요.

- **프로젝트**: autopus-adk
- **모드**: full
- **플랫폼**: claude-code, codex, antigravity-cli, opencode, omp

## Installed Components

Use the current tool's native skill and agent catalog. Load the requested workflow,
not every installed instruction. `auto skill list` and `auto skill info <name>`
provide optional built-in guidance without enlarging the default catalog.

- Claude: .claude/
- Codex: .codex/
- Antigravity: .agents/plugins/autopus/
- OpenCode: .opencode/
- Shared skills: .agents/skills/
- OMP: .omp/

Detailed generated-file ownership is recorded in `.autopus/<platform>-manifest.json`.

## Language Policy

- **Code comments**: en
- **Commit messages**: ko
- **AI responses**: ko

These are instructions, not mechanically enforced language checks.

## Autopus Branding

For explicit `/auto` or `@auto` workflows, start with this banner and end with `🐙`.
Ordinary responses need no wrapper. Worker summaries remain concise evidence receipts.

```text
🐙 Autopus ─────────────────────────
  프로젝트: {project-name} | 모드: {mode}
  SPEC: {draft}개 draft · {approved}개 approved · {implemented}개 구현중 · {completed}개 완료
  다음: {next-step recommendation}
```

## Document Storage

Product code is separate from `.autopus/` harness state. Root project context and
cross-module SPECs belong to the meta repository; module-specific SPECs and
CHANGELOG changes belong to the owning module. SPECs live under the owner's
`.autopus/specs/`. Generated harness files and brainstorm/runtime output stay local
and are not commit candidates.

Allocate SPEC/BS IDs uniquely across root and module repositories. Before committing,
run `auto sync verify` to classify ownership and generated/runtime exclusions.
`auto check --hygiene --staged` is not a substitute for that ownership check.

## Native Execution

Use this runtime's native tool schemas, permissions, and model configuration.
Codex: invoke @auto or $codex-auto and load only the selected route.
Workers share cwd/filesystem unless actual isolation is established. The worker
ceiling is codex.agents.max_concurrent_threads; auto doctor distinguishes requested
from observed capacity. Configuration writes do not prove effective capacity.
OpenCode: invoke /auto <route> or /auto-<route>. Work inline by default;
use native subagents only for justified independent or isolated work.


## Core Guidelines

### Execution

Work inline by default. Delegate independent slices, necessary specialist review,
or work needing context isolation—not work merely spanning more files or lines.
Give workers owned/forbidden paths and acceptance criteria. Parallel writers need
disjoint ownership; dependencies and shared mutable state require sequencing.

Choose execution depth from scope, risk, uncertainty and known acceptance.
Small verified low-risk fixes stay inline; inspect missing facts before adding
steps. Plan high-risk or unclear work, and reassess after failure or scope growth.
Routing never waives applicable gates or changes the user's model preferences.

### Verification

Reproduce bugs and write meaningful tests before behavior changes. Run the affected
behavior and applicable checks before declaring completion. Honor explicit project
quality thresholds; do not invent universal file-size or coverage limits. Keep
permissions, user-owned files, and security/data-loss boundaries intact. Obtain
approval for destructive or production actions.
For greenfield dependencies or requested migrations, verify versions with primary
sources. Preserve existing brownfield major versions unless migration is requested.

### Worker Results

Return `owned_paths`, `changed_files`, `verification`, `blockers`, and
`next_required_step`. Report only observed execution, including failures. The parent
verifies evidence and integrates results; a worker's completion claim is not proof.

### Review and Completion

Separate initial finding discovery from verification of fixes. Do not repeat
unchanged reviews. Finish requested actionable work rather than stopping at a
phase boundary; report a concrete blocker when external input is truly required.
Read workflow details only as needed through native skill discovery.

## OpenCode V2 native contract

Use the advertised native tool catalog. Delegation uses subagent, not a shell
command. The required input fields are agent, description, and prompt:

```json
{"agent":"executor","description":"Implement scoped change","prompt":"Read the assigned context, change only owned files, and return verification."}
```

Only configured subagent-mode agents are eligible. New children need complete
instructions; they do not inherit the parent's conversation. Foreground calls
wait for completion. Use background: true only for independent work; completion
is delivered to the parent. Do not poll or treat the initial running status as
completion. To continue that same child, pass its returned sessionID; it must
belong to the current parent. Do not invent task IDs or cancellation tools.

Omit model unless the user explicitly requested an override. Native model
references use provider/model#variant; translate an Autopus --variant flag to
that suffix rather than forwarding --variant to the V2 CLI. Use shell with its
workdir field for commands. Plugin wiring uses the effective plugins setting;
registration alone does not prove hook execution. If the advertised schema
differs, stop that dispatch and report the mismatch rather than guessing.

<!-- AUTOPUS:END -->
