<!-- AUTOPUS:BEGIN -->
# Autopus-ADK Harness

> 이 섹션은 Autopus-ADK에 의해 자동 생성됩니다. 수동으로 편집하지 마세요.

- **프로젝트**: autopus-adk
- **모드**: full

## 스킬 디렉터리

- Legacy Gemini-compatible: .gemini/skills/
- Antigravity workspace plugin: .agents/plugins/autopus/
- Antigravity workspace hooks: .agents/hooks.json

## Core Guidelines

### Subagent Delegation

IMPORTANT: Delegate for a reason, not for size. Ordinary work — including multi-file edits — stays inline. Use a subagent only for genuinely independent slices with disjoint write ownership, for work whose read surface would crowd out the main session, or for a risky change that needs isolated review. Give each worker owned paths, forbidden scope, completion criteria, and the return format.

### Source Size

IMPORTANT: This project declares architecture.max_file_lines = 300, and "auto check --arch" enforces that ceiling on source code files. Split on a real independent concern, never by arbitrary line ranges.

SPEC Markdown under .autopus/specs/** is documentation, never a source-size input. Generated files (*_generated.go, *.pb.go), documentation (*.md), and configuration (*.yaml, *.json) are also excluded.

### Code Review

During review, verify:
- Source files respect the project's declared size policy, if one is declared (REQUIRED)
- SPEC Markdown files under .autopus/specs/** are not split or rejected for line count alone
- Delegated work was split for independence, context isolation, or risk — not for file or line count (SUGGESTED)

## Rules

@.agents/plugins/autopus/rules/lore-commit.md
@.agents/plugins/autopus/rules/file-size-limit.md
@.agents/plugins/autopus/rules/subagent-delegation.md
@.agents/plugins/autopus/rules/language-policy.md
@.agents/plugins/autopus/rules/techstack-freshness.md

<!-- AUTOPUS:END -->
