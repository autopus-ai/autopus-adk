---
name: context7-docs
description: Context7 documentation fetch heuristics for library and framework work
category: workflow
skillScoped: true
---

# Context7 Documentation Auto-Fetch

IMPORTANT: Before technology/library/framework work, fetch current documentation via Context7 MCP. If Context7 is unavailable, returns no match, or the query fails, fall back to targeted web search. Subagents cannot call MCP tools — the main session MUST fetch and inject docs into subagent prompts.

## When to Fetch

- `/auto go` pipeline: Phase 1.8 (Doc Fetch), before implementation
- `/auto fix`: when the error involves an external library
- General work: when the SPEC, code imports, or the user's description names an external library/framework

## Technology Detection

Scan SPEC requirements, `plan.md` task text, file imports, the user's request, and error messages for library names. Match import statements against the project manifest (`go.mod`, `package.json`, `requirements.txt`/`pyproject.toml`) and skip standard-library modules.

## Fetch Procedure

For each detected technology, up to the limits below:

1. `mcp__context7__resolve-library-id` with the library name. No match → web fallback.
2. `mcp__context7__query-docs` with the resolved id and a topic that matches the task (API usage, configuration, migration). Empty or error → web fallback.
3. Web fallback: targeted search preferring official docs, release notes, migration guides, and API references. Mark the result as a web fallback source.

Cache every result for the pipeline run.

## Prompt Injection Format

```
## Reference Documentation

### {Library Name} (via Context7)
_Metadata: version={resolved version} | source_ref={library ID or official URL} | checked_at={YYYY-MM-DD}_

{trimmed documentation content}
```

For greenfield stack choices, documentation content alone is not enough: copy the version/source_ref/checked_at metadata into the `## Technology Stack Decision` section that `techstack-freshness` requires.

When trimming, keep in this order: API signatures and type definitions, configuration examples and common patterns, version-specific breaking changes, error handling patterns. Trim tutorials and introductory content first.

## Task-Specific Topic Query

Phase 1.8 fetches base docs per library with a topic such as "API overview and core patterns". A per-executor refinement query is optional and only worth it when the task clearly needs a different facet of the same library; merge it with the base docs, dedup, and skip injection when the content matches. Each refinement consumes one query slot.

## Caching

Base docs are fetched once per pipeline; refinement docs are cached per `{library-id}:{topic}`. All agents in the pipeline share the cache, which is discarded when the pipeline completes.

## Limits

| Limit | Value | Rationale |
|-------|-------|-----------|
| Max libraries per pipeline | 5 | Token cost control |
| Max per-library tokens | ~5000 for a single library, scaling down to ~2000 at 5 | Breadth over depth as count grows |
| Max total injected tokens | 10000 (hard cap) | Prevent prompt bloat |
| Max refinement queries | 3 per pipeline | Avoid excessive MCP calls |
| Max MCP retries | 1 per library | Fail over to web instead of looping |

## Error Handling

Every failure path — no match, empty result, MCP server unavailable, failed web search — logs and continues. Documentation is supplementary: never block the pipeline on a fetch failure.

## Anti-Patterns

- Do NOT let subagents call MCP tools directly; they cannot access them.
- Do NOT fetch documentation for standard-library modules.
- Do NOT inject untrimmed documentation.
- Do NOT skip straight to web search when Context7 MCP is available and applicable.
- Do NOT treat "latest docs fetched" as a resolved dependency version; record concrete version evidence separately.

## Ref

SPEC-CTX7-001
