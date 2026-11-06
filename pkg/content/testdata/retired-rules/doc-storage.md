---
name: doc-storage
description: Storage locations for SPEC, brainstorm, and generated documents across the workspace
category: workflow
skillScoped: true
---

# Document Storage Rules

IMPORTANT: All documents MUST be stored in the correct location based on their scope. Misplaced documents cause sync failures and version control gaps.

## Storage Matrix

| Document Type | Location | Git Repo | Example |
|---------------|----------|----------|---------|
| Project context | Root | meta repo | `ARCHITECTURE.md`, `.autopus/project/*` |
| Harness bootstrap config | Root | meta repo | `CLAUDE.md`, `autopus.yaml`, `opencode.json`, `.mcp.json`, `.autopus/context/constraints.yaml` |
| Generated harness/runtime surface | Local working copy only | Do not commit | `.claude/`, `.codex/`, `.gemini/`, `.opencode/`, `.autopus/plugins/`, `.autopus/*-manifest.json` |
| Cross-module SPEC | Root `.autopus/specs/` | meta repo | SPECs affecting 2+ modules |
| Module-specific SPEC | `{module}/.autopus/specs/` | module repo | SPECs affecting a single module |
| Brainstorm/runtime output | Local working copy only | Do not commit | `.autopus/brainstorms/`, `.autopus/orchestra/`, `.autopus/runtime/` |
| CHANGELOG | Root | meta repo | `CHANGELOG.md` |
| Module CHANGELOG | `{module}/CHANGELOG.md` | module repo | Module-specific changes |

## Module Detection

WHEN creating a SPEC or BS, determine the target module by:

1. Check which `pkg/`, `cmd/`, `internal/`, `src/`, `app/` paths are referenced
2. Match those paths to the submodule that contains them
3. If paths span 2+ modules → cross-module → root
4. If no code paths → use the module closest to the described feature

## ID Uniqueness

SPEC IDs and BS IDs MUST be globally unique across the entire workspace.

- Before creating a new ID, scan ALL locations: `.autopus/specs/SPEC-*` AND `*/.autopus/specs/SPEC-*`
- Same for BS: `.autopus/brainstorms/BS-*` AND `*/.autopus/brainstorms/BS-*`
- ID collision is a hard error — never create a duplicate

## Sync Commit Rules

WHEN `/auto sync` runs:

1. **Module commit** (Phase A): SPEC files within `{TARGET_MODULE}` are committed to the module's git repo
2. **Meta commit** (Phase B): Canonical root documents and reviewed bootstrap config (`AGENTS.md`, `ARCHITECTURE.md`, `CLAUDE.md`, `autopus.yaml`, `opencode.json`, `.mcp.json`, `.autopus/context/constraints.yaml`, `.autopus/project/`, `.autopus/specs/`, `.autopus/learnings/pipeline.jsonl`, and human-maintained root Markdown) are committed to the meta repo

Both phases run in sequence. Phase B is skipped if no root files changed.

커밋 전에 `auto sync verify`(read-only, git 변경 없음)를 실행합니다. dirty 경로를 commit 후보 / 차단된 generated·runtime 경로 / 미분류로 분할하고, 안전한 후보만 `git -C <repo> add -- <paths>` 형태로 출력합니다. 멀티 repo 워크스페이스는 Phase A(module)와 Phase B(meta)로 나뉘고, `autopus.yaml`을 가진 단일 repo는 module phase 없이 한 그룹이 됩니다. 지원하지 않는 배치에서는 `unsupported topology:` 진단으로 멈추며, 이는 분류 결과가 아닙니다. `auto check --hygiene --staged`는 generated/runtime 위생만 보므로 대체물이 아닙니다.

`auto sync verify --spec SPEC-ID`는 워크스페이스 전체에서 해당 SPEC의 host 하나를 찾아 그 SPEC이 소유한 dirty 경로만 계획하고 나머지를 보고합니다. hook이나 CI에서는 `--strict`로 경계·소유권·차단·미분류 경고를 exit code로 승격합니다.

## Context Document Rotation

IMPORTANT: The context catalog documents MUST stay compact current-state maps, not append-only ledgers. The catalog lists documents eligible for command-profile selection, rotation, and weight measurement; it does not imply that every document loads in every `/auto` session. WHEN `/auto sync` updates a catalog document, per-SPEC completion history rotates into per-document archive files instead of accumulating.

### Context Catalog

The context catalog contains seven profile-eligible documents. Each has a per-document byte cap for lossless rotation; the caps sum to 100000 bytes.

| Document | Per-doc cap (bytes) |
|----------|---------------------|
| `.autopus/project/product.md` | 18000 |
| `ARCHITECTURE.md` | 16000 |
| `.autopus/project/scenarios.md` | 20000 |
| `.autopus/project/workspace.md` | 12000 |
| `.autopus/project/tech.md` | 10000 |
| `.autopus/project/structure.md` | 18000 |
| `.autopus/project/canary.md` | 6000 |

### Keep vs Move

Keep current-state facts and executable scenario wiring in the context catalog document. Move completion history and over-detail into that document's archive losslessly (move, never summarize, rewrite, or delete).

| Keep (current fact, context catalog) | Move (to archive, lossless) |
|--------------------------------------|------------------------------|
| Latest-state description of what a capability does now | Completion dates (`completed YYYY-MM-DD`) |
| Active boundaries, ownership, command routing | Module commit hashes (`@abc1234`), per-SPEC completion attribution |
| Package-level structure map | Full directory trees |
| In `scenarios.md`, top-level `Build` plus every runnable scenario `Command`, `Verify`, and `Status` | Scenario completion history that can move without removing or rewriting the runnable body |
| Active canary configuration | Verification narrative (follow-up verification, review-loop hardening) |

Runnable scenario bodies MUST remain executable in `scenarios.md`; never replace them with index-only entries. Completion history and full directory trees that rotate to archives MUST remain lossless.

### Rotation Rules

WHEN `/auto sync` updates a context catalog document:

1. Retain only current-state facts in the document. Append the removed history and over-detail to that document's own archive file at `.autopus/project/archive/<doc>-history-<year>H<half>.md`, where `<doc>` is the document base name without extension (for example, `product` for `product.md`).
2. Choose the half-year bucket from the record's `completed|implemented YYYY-MM-DD` date tag: H1 covers January–June, H2 covers July–December. An undated record inherits the half-year of the nearest dated record above it.
3. Prepend one `Archived-From: <doc>@<date>` header line to every moved record, keeping the original text intact — for example, `Archived-From: product.md@2026-06-15`.
4. Rotation is idempotent: a record already carrying an `Archived-From:` header is never moved or duplicated again.
5. Add a history pointer line to each compacted document that references its own archive file — for example, `History: .autopus/project/archive/product-history-2026H1.md`.

WHEN `/auto sync` records a changelog entry:

1. Keep only the most recent half-year in `CHANGELOG.md`. Move older entries into `CHANGELOG-<year>H<half>.md` without discarding any entry.
2. Take each entry's half-year from its heading ISO date (`completed|implemented|in progress YYYY-MM-DD`); an undated entry inherits the half-year of the nearest dated entry above it.

### Advisory Guards

`auto doctor`는 context 카탈로그 문서 무게(합계 120000 바이트 또는 단일 문서 20000 바이트 초과), 설치 표면 드리프트와 orphan manifest, learnings/canary/memindex evidence 신선도(30일 초과)를 비차단 경고로 보고합니다. 모두 advisory이며 하네스 상태를 실패시키지 않고, rotation이나 `auto update`, `generate-templates`, `auto learn record`, `auto canary`, `auto mem rebuild` 같은 후속 조치를 힌트로만 제시합니다. `auto learn query --spec SPEC-ID`로 특정 SPEC의 항목만 조회할 수 있습니다.

## Anti-Patterns

- Do NOT store module-specific SPECs at the root level
- Do NOT store cross-module SPECs inside a single module
- Do NOT create BS or SPEC IDs without checking global uniqueness first
- Do NOT commit root documents to a submodule repo (they are outside its git tree)
- Do NOT append per-SPEC completion history to a context catalog document; rotate it into the document's archive file
- Do NOT summarize or delete rotated history; move it verbatim so record counts are conserved
