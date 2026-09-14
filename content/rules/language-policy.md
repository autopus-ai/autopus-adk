---
name: language-policy
description: Language policy for code comments, commit messages, and AI responses
category: workflow
alwaysApply: true
---

# Language Policy

IMPORTANT: Follow the project's configured language for each surface. Every agent checks the configuration before producing output.

- `code_comments` — code comments, docstrings, inline documentation
- `commit_messages` — git commit messages
- `ai_responses` — responses to the user

An unconfigured surface defaults to English.

Nothing enforces this mechanically: no hook, linter, or CI step inspects language, and the pre-commit Lore check validates only the commit type prefix and sign-off trailers. A violation surfaces as a review finding, so do not rely on a gate to catch it.
