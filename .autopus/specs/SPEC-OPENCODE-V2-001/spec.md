---
id: SPEC-OPENCODE-V2-001
status: implemented
---

# OpenCode V2 adapter compatibility

The installed 2.0.10 runtime changed plugin execution, native delegation and
configuration contracts. Preserve V1 generation while selecting V2 from a bounded
CLI version probe or explicit constructor version; reject unsupported future
major versions before generating files. Missing/unreadable versions retain the
legacy offline V1 contract and do not certify runtime compatibility.

Required outcomes:
- Generate a V2 hook plugin using the exact native setup/tool-hook contract.
  Before-hook failure blocks execution; after hooks observe shell completion.
  Resolve session/command working directory, bound subprocess lifetime/output,
  and dispose registrations. Keep the V1 renderer available.
- Preserve plugin options, ordering, user settings and the effective V1/V2
  configuration shape. Validation recognizes native objects and explicit disables.
- Emit V2 subagent invocation/schema/lifecycle guidance without converting user
  text outside the managed AGENTS.md section. Retain V1 task instructions.
- Verify generated JS behavior and real installed plugin loading/native hook
  invocation without requiring provider credentials. Model-authentication 401
  remains a separately unverified boundary.

No global credential changes, global plugin removal, release publication or
claim of successful live model-native delegation is included.
