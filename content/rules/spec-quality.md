---
name: spec-quality
description: SPEC self-verify checklist applied by spec-writer and spec-review before provider review
category: workflow
skillScoped: true
---

# SPEC Quality Checklist

Use this checklist after `spec.md`, `plan.md`, `acceptance.md`, and `research.md` are drafted, and before the SPEC enters `.claude/skills/spec-review/SKILL.md`.

## How To Apply

1. Read all four SPEC files as one document set.
2. For every checklist item, record `PASS`, `FAIL`, or `N/A` with a short reason in natural language.
3. Keep a visible `## Self-Verify Summary` in `research.md` using `Q-* | status | attempt | files | reason` so reviewers can confirm what changed between retries.
4. If an item fails, fix every affected file required by that dimension. Do not limit edits to the file where the symptom first appeared.
5. Retry the full checklist up to 2 times.
6. If failures remain after the second retry, append `## Open Issues` to the end of `spec.md` and list each remaining `Q-* | category | scope | attempt | reason`.

Prompt-state SPECs MUST name the prompt layer manifest contract and classify stable, snapshot, and ephemeral context so cache invalidation is observable without exposing raw secrets.

Keep `plan.md`, `research.md`, and `acceptance.md` each at or under the 200-line review injection cap (the authoring cap). A file over the cap is injected under structure-preserving compression fallback and the review is labeled with degraded observation coverage, so staying under the cap is what lets a well-formed SPEC be fully observed without an override. `auto spec validate <dir> --strict` emits a warning-level finding naming the file, its line count, and the cap, and still exits zero, so authors see the truncation risk before review.

The five primary dimensions map 1:1 to `FindingCategory` values in `pkg/spec/types.go`:
- `correctness`
- `completeness`
- `feasibility`
- `style`
- `security`

## Change Contract Applicability

This checklist reviews a four-document SPEC set. It does not apply to a compact change contract, and its absence is not a finding on that path.

- `auto spec change <SPEC-ID> --class <class> --ac <AC-ID,...> --surface <path,...> --verify "<command>"` writes one `change.md` for low-risk work: `test_only`, `docs_only`, `small_ui`, `bugfix_existing_contract`. `auto spec gates` then reports `spec_authoring: not_applicable` with the low-risk reason, and `risk_first_probe: not_applicable`.
- `feature`, `multi_domain`, and `security_or_data` work is high risk. It needs this full checklist plus at least one executable `## Risk-First Integration Probe` row; `spec_authoring` and `risk_first_probe` are both `required`.
- Risk is never decided by file count. An auth, billing, data, migration, or security path raises the class to `security_or_data`; production code spanning two module roots raises it to `multi_domain`; a new exported API or contract raises it to `feature`.
- A declared class contradicted by the surface — `test_only` including non-test source, `docs_only` including code, `small_ui` including non-UI source — is `escalate_to_full_spec`. Author the full set and answer this checklist.
- Reviewing a compact contract means checking the referenced SPEC id, the referenced acceptance-criteria ids, the declared surface against the real diff, and the per-criterion evidence rows. `security`, `validation`, `data_loss`, and `deterministic_oracle` stay `required` on both paths, as do `accessibility` and `ux_verification` for a UI surface.

## N/A Status Guidance

`N/A` is a first-class status alongside `PASS` and `FAIL`, surfaced as its own count by the orchestra parser, `auto spec self-verify --status N/A`, and the persisted `## Checklist Summary`. Use it when a dimension genuinely does not apply — never to avoid analysis.

| Dimension | Allowed scenarios | Required reason content |
|-----------|------------------|-------------------------|
| **correctness** (Q-CORR-*) | SPEC modifies only documentation/markdown; no code, configuration, or runtime contract referenced | Identify the doc-only scope; confirm no `[NEW]` runtime symbol claims |
| **completeness** (Q-COMP-*) | Rare. Only when the slice is owned by an approved sibling SPEC decision or is an explicit Outcome Lock non-goal | Reference the sibling SPEC ID or the non-goal that owns the slice |
| **feasibility** (Q-FEAS-*) | SPEC is purely conceptual and runtime/module ownership is explicitly out of scope | State the gating step that will establish feasibility |
| **style** (Q-STYLE-*) | Rare. A section deliberately uses non-standard formatting validated by a linter exception | Cite the linter rule and the rationale |
| **security** (Q-SEC-*) | No trust boundary, secret, credential, privileged path, or external input parsing | Explain why no real attack surface is involved |

Every `N/A` entry carries a non-empty reason, concise enough to survive the 200-rune sanitizer: an empty reason renders as `-` in `review.md` and is indistinguishable from a placeholder.

- Do NOT mark `N/A` to silence a checklist item you have not analyzed.
- Do NOT mark `N/A` for security on a SPEC that touches user input, file paths, secrets, or external network calls — even a doc-only SPEC quoting untrusted prompt evidence triggers Q-SEC-01.
- Do NOT mix `N/A` and `PASS` for the same dimension across providers without explaining the divergence in the merged finding list.

## correctness

### Q-CORR-01 — Existing references are real

- PASS 기준: `[NEW]` 마커가 없는 경로, 타입, 함수, 규칙 이름이 현재 코드베이스에서 실제로 확인된다.
- FAIL 기준: 기존 참조가 존재하지 않거나, 다른 이름/다른 위치를 실제 경로처럼 단정한다.
- Example: `pkg/spec/types.go`의 상수 이름을 인용했다면 실제 선언을 직접 확인한다.

### Q-CORR-02 — Planned additions are marked as planned additions

- PASS 기준: 아직 존재하지 않는 새 파일, 타입, 함수, 섹션은 `[NEW]`로 표기되고 기존 참조 검증 대상에서 제외된다.
- FAIL 기준: 미래 변경을 기존 참조처럼 서술하거나, `[NEW]` 없는 신규 항목을 정합성 PASS 근거로 사용한다.
- Example: `[NEW] pkg/foo/bar.go`는 허용되지만, 같은 경로를 기존 파일처럼 단정하면 FAIL이다.

### Q-CORR-03 — Claimed syntax matches real parser and validator behavior

- PASS 기준: EARS, Gherkin, Priority, validator 제약에 대한 설명과 예시가 실제 파서/validator가 받아들이는 형식과 맞는다.
- FAIL 기준: 문서가 지원하지 않는 구문을 공식 형식처럼 제시하거나, 예시가 실제 parser contract와 어긋난다.
- Example: acceptance 예시는 parser가 인식할 수 있는 `Given/When/Then` 형태를 사용한다.

### Q-CORR-04 — Reference Discipline separates existing and planned references

- PASS 기준: 실제 코드베이스에 존재하는 path/symbol/config는 `rg`/Read 등으로 확인했고, 아직 만들 항목은 `[NEW]` planned addition으로 표시한다. generated surface와 source of truth도 구분한다.
- FAIL 기준: 존재하지 않는 파일, 함수, CLI flag, generated copy를 기존 구현처럼 단정하거나, `[NEW]` 표시 없이 future reference를 품질 근거로 사용한다.
- Example: `internal/cli/spec_review_loop.go`는 existing reference로 검증하고, 새 테스트 파일/함수는 `[NEW] internal/cli/spec_review_loop_convergence_test.go::Test...`처럼 표시한다.

## completeness

### Q-COMP-01 — The four SPEC files form one complete package

- PASS 기준: `spec.md`, `plan.md`, `acceptance.md`, `research.md`가 각각 목적과 역할을 갖고 서로 보완된다.
- FAIL 기준: 문서 중 하나가 비어 있거나, 핵심 결정·검증 근거가 빠져 다른 문서가 역할을 대신 떠안는다.

### Q-COMP-02 — Requirements, plan, and acceptance stay traceable

- PASS 기준: 요구사항이 구현 태스크와 수락 기준에서 빠짐없이 다뤄지며, 빠진 항목이 있으면 `Open Issues` 또는 out-of-scope로 명시된다.
- FAIL 기준: REQ가 구현 태스크나 acceptance에서 근거 없이 사라지거나, 어떤 문서가 어떤 요구를 검증하는지 추적할 수 없다.
- Example: 추적성 FAIL은 `spec.md`와 `acceptance.md`를 함께 수정해야 할 수 있다.

### Q-COMP-03 — EARS type, trigger, and observability are explicit

- PASS 기준: 각 요구사항의 EARS type, 조건, 기대 결과, 관측 지점이 문서상에서 분명하다.
- FAIL 기준: type만 적고 조건이나 기대 결과가 없거나, 검증 가능한 관측 지점 없이 행동만 선언한다.
- Example: self-verify loop를 요구한다면 `research.md`의 `Self-Verify Summary`나 `spec.md`의 `Open Issues`처럼 어느 문서와 어떤 흔적으로 확인하는지 함께 적는다.

### Q-COMP-04 — Outcome Lock is fully covered by the Primary SPEC

- PASS 기준: `research.md`의 `## Outcome Lock`이 사용자 가시 결과, mandatory requirements, explicit non-goals, completion evidence를 고정하고, Primary SPEC의 requirements/plan/Must acceptance가 이를 닫는다.
- FAIL 기준: 스캐폴드, 설정, 문서, 일부 wiring만 다루고도 완전한 기능처럼 보이게 하거나, Completion Debt를 `Out of Scope`, `Evolution Ideas`, 막연한 future work로 숨긴다.
- Example: CLI 플래그 추가만으로 backend/API/UX 동작이 필요한 Outcome Lock을 닫을 수 없다면, Primary SPEC에 해당 동작을 포함하거나 `Completion Debt`로 남겨 sync completion을 막아야 한다.

### Q-COMP-05 — Semantic invariants are mapped to oracle acceptance

- PASS 기준: `research.md`의 `## Semantic Invariant Inventory`에 있는 각 invariant가 최소 하나의 요구사항, `plan.md` 태스크, 그리고 Must `acceptance.md` 시나리오에 추적된다. 알고리즘, paired/grouped/deduped/ordered 데이터, parser/report row, numeric formula는 concrete expected value나 tolerance가 있는 oracle acceptance로 검증된다.
- FAIL 기준: invariant가 research prose에만 남거나, requirement/plan/acceptance 중 하나에서 사라지거나, acceptance가 heading/file existence/exit success 같은 structural check만 수행한다.
- Fix guidance: FAIL이면 `spec.md`, `plan.md`, `acceptance.md`를 함께 점검하고 수정한다. 요구사항 누락은 `spec.md`, implementation ownership 누락은 `plan.md`, observable oracle 누락은 `acceptance.md`에서 보완해야 한다.
- Example: 원 요청이 common `task_id`로 baseline과 other rows를 pair하라고 했다면 acceptance는 서로 다른 harness label의 overlapping `task_id` 입력과 예상 Cohen's d 또는 McNemar 값을 포함해야 한다.

### Q-COMP-06 — Reviewer Brief and Traceability Matrix constrain review scope

- PASS 기준: `spec.md`에는 `## Traceability Matrix`가 있고 각 requirement가 plan task, acceptance scenario, semantic invariant로 연결된다. `research.md`에는 `## Reviewer Brief`가 있어 intended scope, explicit non-goals, self-verified evidence, reviewer focus를 짧게 제시한다.
- FAIL 기준: reviewer가 전체 SPEC을 다시 discovery해야 할 정도로 scope/non-goals/추적 근거가 없거나, review focus가 없어 optional deeper-layer 탐색을 유도한다.
- Example: review focus가 `correctness, convergence safety, regression risk`이면 reviewer는 새로운 제품 scope 제안보다 해당 blocker 검증에 집중해야 한다.

### Q-COMP-07 — Completion Debt and Evolution Ideas are separated

- PASS 기준: `research.md`가 `## Completion Debt`와 `## Evolution Ideas`를 분리한다. Completion Debt는 Outcome Lock, Must acceptance, 보안/데이터 무결성, 필수 workflow를 막는 항목만 포함하고 sync completion을 막는다. Evolution Ideas는 optional improvement로만 남고 SPEC ID, task ID, acceptance ID, sibling SPEC, follow-up SPEC를 자동 생성하지 않는다.
- FAIL 기준: 필수 누락 작업을 Evolution Ideas로 낮추거나, 선택 개선 제안에 SPEC ID/acceptance/task를 붙여 자동 후속 작업으로 승격하거나, reviewer-discovered future idea를 REQUEST_CHANGES 근거로 삼게 만든다.
- Example: "더 다양한 export 포맷"이 Outcome Lock 밖의 개선이면 Evolution Ideas에만 기록하고, 사용자가 명시적으로 요청하기 전까지 `/auto plan`이나 sibling SPEC을 추천하지 않는다.

### Q-COMP-08 — Risk-First Integration Probe records honest verdicts

- PASS 기준: `plan.md`에 `## Risk-First Integration Probe`가 있고, 1-3개 행이 각각 `PASS` / `FAIL` / `not-run` status와 non-empty reason을 갖는다. `PASS` 행은 실제 실행에서 얻은 evidence ref를 참조하고, plan 진술은 `requirement_invariant` / `implementation_assumption` / `verified_fact`로 분류된다.
- FAIL 기준: 섹션이 없거나, `not-run`을 이유 없이 남기거나 PASS로 승격하거나, evidence 없는 행을 `PASS`로 표기하거나, 구현 가정을 요구사항이나 검증된 사실과 섞어 서술한다.
- Fix guidance: 지금 실행 가능한 probe는 `Phase 1.9: Risk-First Probe Gate`에서 실행해 evidence를 남기고, 실행 불가한 행은 `not-run` + 이유로 유지한다. 각 게이트의 적용 여부는 `required | not_applicable | blocked` + 이유로 기록한다.
- Example: doc-only SPEC은 섹션을 유지하고 `not-run | no integration boundary` 한 행만 남긴다. composition-root boot, browser -> BFF -> API round trip, logout/cancel/account-switch 같은 in-flight 상태 경계는 fan-out 전에 실제로 probe해야 한다.

## feasibility

### Q-FEAS-01 — Scope matches the real implementation layer

- PASS 기준: 문서/프롬프트 변경인지, 런타임 코드 변경인지, 후속 SPEC 분리 대상인지가 실제 구현 경로와 맞는다.
- FAIL 기준: 문서만 바꿔서는 보장할 수 없는 동작을 즉시 구현된 것처럼 약속하거나 source of truth를 혼동한다.

### Q-FEAS-02 — Proposed changes fit the owning repo and module boundaries

- PASS 기준: 변경 대상 경로가 실제 repo 구조와 module ownership에 맞고, generated surface와 source of truth를 구분한다.
- FAIL 기준: 존재하지 않는 디렉토리를 편집 대상으로 잡거나, generated copy를 source of truth처럼 다룬다.
- Example: `content/`를 source of truth로 바꾸는지, 설치된 `.codex/` 복사본을 바꾸는지 혼동하면 FAIL이다.

### Q-FEAS-03 — Verification steps are runnable and proportionate

- PASS 기준: 제안한 검증 명령, 수동 점검, 또는 문서 검토 절차가 현재 저장소에서 실제로 수행 가능하다.
- FAIL 기준: 존재하지 않는 명령을 제시하거나, 문서-only 변경에 불필요한 대규모 검증을 강제한다.
- Example: markdown-only 변경이면 포맷, 경로, 라인 수, 교차 참조 점검으로 충분할 수 있다.

## style

### Q-STYLE-01 — Requirement text avoids ambiguous wording

- PASS 기준: REQ description은 `should`, `might`, `could`, `possibly`, `maybe`, `perhaps` 같은 모호어 없이 단정적으로 작성된다.
- FAIL 기준: 모호어가 REQ description에 들어가 validator 경고를 유발하거나, 의미가 흐려진다.
- Example: Priority 값은 description이 아니라 별도 메타 라인에 둔다.

### Q-STYLE-02 — Priority and EARS type remain separate axes

- PASS 기준: Priority는 `Must`, `Should`, `Nice`만 사용하고, EARS type과 같은 규칙으로 섞지 않는다.
- FAIL 기준: Priority와 EARS type을 하나의 FAIL 규칙에 묶거나, `P0`, `P1`, `P2`, `Could` 같은 별칭을 허용한다.
- Example: `Ubiquitous / Priority: Must`는 가능하지만, 둘을 같은 검사 축으로 취급하면 FAIL이다.

### Q-STYLE-03 — Sentences and Gherkin steps are readable and parseable

- PASS 기준: REQ/AC 문장은 완결된 문장으로 끝나며, acceptance 예시는 bare `Given`, `When`, `Then`, `And`, `But` 형식을 따른다.
- FAIL 기준: 문장이 미완성으로 끝나거나, bullet/강조 마크업이 step keyword를 가려 parser와 사람 모두가 읽기 어렵다.
- Example: `Given 사용자가 로그인했다.`는 PASS이고 `- **Given**: ...`는 권장 형식이 아니다.

## security

### Q-SEC-01 — Trust boundaries and prompt-input risk are addressed

- PASS 기준: 외부 입력, prompt injection, provider output, 혹은 untrusted 문서가 품질 루프에 들어오면 그 경계와 완화 방안을 적는다.
- FAIL 기준: 외부 입력이 있는데도 trusted local document처럼 취급하거나, 공격 표면 언급 없이 흐린다.
- N/A 기준: 로컬 저장소의 human-maintained markdown만 다루고 외부 입력 경로가 없다.

### Q-SEC-02 — Sensitive paths and secrets stay protected

- PASS 기준: 비밀값, 토큰, credential, 절대 경로, privileged 파일 접근이 필요하면 노출/오용 방지를 명시한다.
- FAIL 기준: 민감값을 예시로 노출하거나, path traversal 성격의 경로를 검증 없이 허용한다.
- N/A 기준: 비밀값과 privileged path를 전혀 다루지 않는 문서-only SPEC이다.

### Q-SEC-03 — Logging and retained artifacts do not create avoidable risk

- PASS 기준: 로그, audit note, retained artifact를 요구한다면 포맷 안정성, retention, diff noise, secret leakage를 함께 고려한다.
- FAIL 기준: 구조화 기록을 요구하면서 포맷 보장 수단이 없거나, 불필요한 영구 artifact를 남겨 후속 작업을 오염시킨다.
- N/A 기준: 별도 로그/아티팩트를 만들지 않고 결과를 문서 내부에서만 정리한다.

## Appendix — cohesion

This appendix is a secondary axis and remains separate from the five primary dimensions above.

### Q-COH-01 — One SPEC, one cohesive change story

- PASS 기준: SPEC이 하나의 명확한 문제와 소수의 밀접한 변경 대상으로 수렴한다.
- FAIL 기준: unrelated concerns를 한 SPEC에 묶어 reviewer나 executor가 경계를 파악하기 어렵다.

### Q-COH-02 — Follow-on work does not bypass Outcome Lock

- PASS 기준: Outcome Lock에 필요한 후속 런타임/플랫폼 작업은 Primary SPEC에 포함되거나 Completion Debt로 남아 sync completion을 막는다. Outcome Lock 밖의 개선은 Evolution Ideas로만 남는다.
- FAIL 기준: 현재 문서가 해결하지 못하는 필수 문제를 같은 iteration 안에서 암묵적으로 해결된 것처럼 남기거나, optional idea를 자동 follow-up SPEC으로 예약한다.

### Q-COH-03 — Sibling SPEC boundaries are exceptional and bounded

- PASS 기준: sibling SPEC가 있으면 `## Sibling SPEC Decision`이 허용 사유를 명시하고, sibling은 최대 2개이며, 각 SPEC가 독립적으로 구현 가능한 outcome slice와 acceptance 책임을 갖는다. sibling의 sibling은 만들지 않는다.
- FAIL 기준: 너무 작게 쪼개져 실질 동작 없이 스캐폴드만 만들거나, optional improvement/reviewer idea를 sibling으로 만들거나, 재귀 sibling으로 workflow를 끝나지 않게 한다.
