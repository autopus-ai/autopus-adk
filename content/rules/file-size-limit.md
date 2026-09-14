---
name: file-size-limit
description: Project-specific source size policy and cohesion review
category: structure
globs:
  - "**/*.go"
  - "**/*.ts"
  - "**/*.tsx"
  - "**/*.js"
  - "**/*.jsx"
  - "**/*.py"
  - "**/*.rs"
---

# Source Size Policy

Honor an explicitly configured `architecture.max_file_lines` ceiling. A positive
value is enforced by `auto check --arch`; absent or `0` leaves source size advisory.
Do not split cohesive code merely to satisfy an undeclared universal threshold.

When a file grows, review its responsibilities and interfaces. Split by a real
independent concern, not by arbitrary line ranges or boilerplate categories.

The checker counts code lines, including blank lines, imports, and trailing
comments, but excludes comment-only lines. Strings containing comment markers
remain code. Tests follow the same policy; generated code, documentation, and
configuration files are not source-size gate inputs.

Repository-specific CI commands may declare stricter limits. Those explicit
limits remain authoritative; an advisory harness check does not waive them.
