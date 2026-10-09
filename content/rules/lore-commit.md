---
name: lore-commit
description: Lore commit format rules for structured, traceable commit messages
category: workflow
condition: '\bgit\s+commit\b'
scope: tool:bash
interruptMode: tool-only
astCondition: 'exec.Command("git", "commit", $$$ARGS)'
---

# Lore Commit

IMPORTANT: All commits MUST use Lore format. Never use plain commit messages or Co-Authored-By trailers.

## Format

```
<type>(<scope>): <subject>

<body>

Constraint: <invariant or design boundary>
Confidence: <low|medium|high>
Scope-risk: <local|module|system>
Reversibility: <trivial|moderate|difficult>
Directive: <follow-up guidance>
Tested: <what was verified>
Not-tested: <what remains unverified>
Related: <SPEC-ID, issue, or related change>

🐙 Autopus <noreply@autopus.co>
```

## Types

| Type | Description |
|------|-------------|
| feat | New feature |
| fix | Bug fix |
| refactor | Code improvement without behavior change |
| test | Add or modify tests |
| docs | Documentation |
| chore | Build, config changes |
| perf | Performance improvement |

## Rules

- `auto check --lore` enforces a valid Lore type prefix, the Autopus sign-off, and the absence of every trailer in `lore.forbidden_trailers`.
- Structured Lore trailers use the `Constraint` / `Rejected` / `Confidence` / `Scope-risk` / `Reversibility` / `Directive` / `Tested` / `Not-tested` / `Related` protocol.
- Default `autopus.yaml` requires `Constraint` when Lore trailer validation is enabled.
- `Why` / `Decision` / `Alternatives` trailers are legacy guidance and are no longer the source of truth.
- Sign with `🐙 Autopus <noreply@autopus.co>`
- NEVER add `Co-Authored-By` trailers. `lore.forbidden_trailers` defaults to `[Co-Authored-By]`, so the commit-msg hook rejects them, and generated Claude Code settings clear its commit attribution to match. Set `forbidden_trailers: []` to allow them.
- When committing from Codex, build the full Lore message first and use `git commit -F <message-file>` so trailers and sign-off are preserved exactly.
- The installed `auto` can lag the source it was built from. When its validator rejects a type this rule names (for example `merge(<scope>)`), commit with a type it accepts and say so in the body instead of bypassing the hook: `--no-verify` also skips the sign-off and trailer checks.

## Merge and Squash Commits

GitHub PR merges land on the default branch as commits too — they MUST carry Lore format, or the branch history silently drifts out of compliance.

- Squash merge: edit the squash commit message before confirming. The PR title becomes the subject — give the PR a Lore-valid title (`<type>(<scope>): <subject>`), and paste the Lore body + trailers + `🐙 Autopus <noreply@autopus.co>` sign-off into the squash message body (GitHub's default bullet list of commit subjects is NOT a valid Lore body).
- Merge commit: the auto-generated `Merge pull request #N ...` subject is not Lore-valid. Prefer squash merge, or use `merge(<scope>): <subject>` locally with full trailers when a merge commit is required.
- The same rule applies to release/RC integration merges (for example `merge(release): ...`).
