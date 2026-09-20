# References and design constraints

Official Codex noninteractive documentation:
https://learn.chatgpt.com/docs/non-interactive-mode

Installed CLI 0.155.1 supports --ignore-user-config (credentials retained),
--ephemeral and JSON turn.completed usage. Its local CLI help was checked.
A calibration call wrote/read a marker successfully and emitted usage.

The existing telemetry harness comparison accepts supplied evidence; it does
not execute workloads. This pilot adds an executable benchmark harness.
Current product generation defaults to split; full is an explicit opt-in.

Pretrial review found grading could restore original source after a candidate
file deletion: fixed by seeded grading plus rejecting missing/symlink candidates.
It also found workspace-write alone does not isolate reads of solution copies;
named permissions and actual sandbox read-denial checks are required before
claiming solution isolation.

Actual permission preflight: 8/8 checks passed on Codex 0.155.1 Seatbelt. Named
bench profile denies root reads, grants exact workspace/cache/temp writes and
system/toolchain/module reads; .git/.codex stay read-only, network disabled.
No legacy --sandbox override is passed. Source sibling, protocol sibling,
symlink escape and original repository reads all failed with permission errors;
workspace operations and Go compile/test succeeded. This proves local command
confinement, not global context auto-loading isolation.

Visibility calibration (no model call) via debug prompt-input: native/reduced/full
project skill counts 0/38/71, with the same 17 external skills in all arms.
Reduced/full root AGENTS content is included. Debug rendering uses normal user
config because it lacks --ignore-user-config, so it is not claimed identical to
actual exec startup. Pilot exec ignores user config equally in all arms.
