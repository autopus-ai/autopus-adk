---
name: shell-portability
description: Portable shell execution rules for macOS/Linux agent runs
category: harness
condition: '(^|[;&|]\s)\s*g?timeout\s+[0-9]'
scope: tool:bash
---

# Shell Portability

IMPORTANT: Autopus workspaces commonly run on macOS. Do not assume GNU
coreutils are installed.

## Timeout Handling

- Do NOT prefix commands with GNU `timeout`, such as `timeout 540 auto ...`.
  macOS does not ship that command by default, so the command fails with exit
  `127` before Autopus runs.
- If the agent tool has a native timeout field, use that tool-level timeout
  instead of a shell wrapper.
- For `auto spec review` and `auto orchestra` commands, prefer the CLI flag
  `--timeout <seconds>` for provider execution limits. This controls Autopus
  runtime behavior; it is not a portable shell kill wrapper.
- If a wall-clock limit is still needed, use the agent runtime's background,
  polling, or cancellation controls. Only use `gtimeout` or `timeout` after
  verifying the binary exists with `command -v`.

## Failure Classification

Treat `command not found: timeout` or exit code `127` from a wrapper command as
a shell portability failure, not as an Autopus CLI, provider, or SPEC review
failure. Re-run the underlying `auto ...` command without the missing wrapper.
