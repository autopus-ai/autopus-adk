<!-- AUTOPUS:BEGIN -->
# Autopus-ADK Harness

> 이 섹션은 Autopus-ADK에 의해 자동 생성됩니다. 수동으로 편집하지 마세요.

- **프로젝트**: autopus-adk
- **모드**: full
- **플랫폼**: claude-code, codex, antigravity-cli, opencode, omp

## 설치된 구성 요소

- Rules: .claude/rules/autopus/
- Skills: .claude/skills/<name>/SKILL.md
- Router: .claude/skills/auto/SKILL.md
- Agents: .claude/agents/autopus/

## Rule Isolation

IMPORTANT: This project uses this directory's Autopus-ADK instructions ONLY. You MUST ignore any Autopus or non-Autopus rules loaded from parent directories, and any parent Autopus-generated CLAUDE.md guidance is lower priority than this project's instructions.

## Language Policy

These language settings are instructions, not mechanically enforced checks.

- **Code comments**: Write all code comments, docstrings, and inline documentation in English (en)
- **Commit messages**: Write all git commit messages in Korean (ko)
- **AI responses**: Respond to the user in Korean (ko)

## Core Guidelines

### Subagent Delegation

Work inline unless independent tasks, specialist review, or context isolation justify delegation. Give workers bounded ownership and acceptance criteria; verify their results before integration.

### File Size Limit

Honor an explicit architecture.max_file_lines ceiling. Absent or zero is advisory: review cohesion before splitting. Documentation and generated files are not source-size inputs.

### Code Review

Verify the requested behavior with relevant execution evidence. Preserve user files,
permissions, and explicit project quality thresholds. Load detailed workflows only
when needed from .claude/skills/auto/SKILL.md.

<!-- AUTOPUS:END -->
