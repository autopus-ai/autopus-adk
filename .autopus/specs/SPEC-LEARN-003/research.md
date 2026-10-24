# SPEC-LEARN-003 리서치

## Outcome Lock

- User-visible outcome: 수정 패턴을 기록할 때 그 교훈이 실행으로 확인됐는지(`deterministic`) 아닌지(`llm_only`)와 범위(`process`·`project`)가 함께 남는다. 조회는 실행 근거가 있는 교훈을 앞에 두고, 정리는 근거 없는 의견을 더 일찍 지우며, 요약은 템플릿 반영 후보와 intake 대상을 보여 준다. Claude Code만이 아니라 Codex·OpenCode·OMP·Gemini도 같은 명령으로 학습을 조회·기록·정리한다.
- Mandatory requirements: REQ-LRN3-01 ~ 08.
- Explicit non-goals: 자동 반영, 유사도 조회, skillevolve·qualityloop 재작업, OpenCode 조건부 rule, 이전 버전의 새 필드 보존.
- Completion evidence: Must S1–S9 PASS, strict validate, 300줄, 생성 결과 grep.

## 배경

- Harness²(Xu et al., arXiv 2610.14224v1, 2026-10-09, Google Cloud AI Research)의 근거.
  - §2.3: 교훈은 실행 비교로 뒷받침될 때만 남기고, 출력을 골랐다는 사실만으로는 전이 가능하다고 보지 않는다.
  - Table 3: 범용·분야로 나눈 제안을 빼면 WorkBuddy −2.9, JobBench −3.7. 세 ablation 중 가장 크다.
  - App. D.2: 라이브러리는 산문 요약이 아니라 실행 가능한 산출물을 보관한다.
  - App. E.6: 편집 모델이 쓴 검사는 편집 모델의 가정을 잡지 못하고, 채점되지 않은 실행에서 뽑은 짐작이 규칙이 된다.
- 현재 코드(main d11bf15b + PR #214, 2026-10-24 Read·grep).
  - 저장소 schema `pkg/learn/types.go:32-50`. 근거 gate 없이 위생 검사만 한다(`pkg/learn/record.go`).
  - Claude 템플릿만 조회·기록한다(`templates/claude/commands/auto-workflows.md.tmpl` Phase 0.8, fix Step 0·5, sync Target 4.5).
  - Codex 템플릿은 learnings를 command profile이 요구할 때만 넣는다고 적었지만, 어느 profile도 요구하지 않았다(`templates/codex/skills/auto-go.md.tmpl:50`).
  - Gemini 템플릿에는 learn 명령이 없다.
  - OpenCode는 `pkg/adapter/opencode/opencode_specs.go`의 SkillPath로, OMP는 `pkg/adapter/omp/omp_skills.go`로 Codex 템플릿을 쓴다.
  - intake는 expected·actual이 있는 항목을 격리 후보로 만들고 사람이 승격한다(SPEC-HARNEVAL-002). 어떤 workflow도 부르지 않는다.
  - 저장소 decoder는 엄격 모드가 아니다(`pkg/learn/store.go:98`).

## Visual Planning Brief

UI 변경은 없다. 데이터 흐름 Mermaid는 `plan.md` `## Visual Planning Brief`에 있다(`wireframe intent: not applicable`).

## Semantic Invariant Inventory

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-LRN3-01 | "근거 종류는 exit code에서 도출" | `deterministic` ⇔ command≠"" ∧ before≠0 ∧ after=0. 호출자가 종류를 직접 쓰는 경로는 없다 | `evidence.kind`, 조회 점수 | S1, S3, S9 |
| INV-LRN3-02 | "기존 항목은 이전과 같다" | 새 필드 없는 항목의 JSONL 줄·점수·정리·요약·intake fingerprint가 이 SPEC 전과 같다 | 저장소 bytes, CLI 출력 | S1, S5, S7 |
| INV-LRN3-03 | "scope가 없으면 project" | 읽는 모든 곳에서 빈 scope = `project` | 조회 필터, 요약 개수 | S2, S4 |
| INV-LRN3-04 | "거부되면 아무것도 쓰지 않는다" | scope·근거 명령 거부 시 저장소 SHA-256 불변 | 저장소 bytes | S2 |
| INV-LRN3-05 | "자동 반영 없음" | summary와 템플릿은 후보와 intake를 보여 주거나 제안만 하고 rule·skill·template·golden set을 바꾸지 않는다 | summary 출력, 생성 템플릿 문구 | S6, S8 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| 근거 입력·도출·검사 | REQ-LRN3-01, 02, 04 / S1, S2, S9 | covered |
| 범위 | REQ-LRN3-03, 05 / S2, S4 | covered |
| 조회 가산 | REQ-LRN3-05 / S3 | covered |
| llm_only 정리 | REQ-LRN3-06 / S5 | covered |
| 요약 후보·intake | REQ-LRN3-07 / S6 | covered |
| 다섯 플랫폼 템플릿 | REQ-LRN3-08 / S8 | covered |
| 호환 | REQ-LRN3-02 / S7 | covered |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | — | 모든 Must가 결정적 test와 생성 결과 grep으로 닫힌다. model 호출이 필요한 live smoke는 없다 |

## Evolution Ideas
These are optional improvements and do not block sync completion.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| `/auto sync`가 `Template candidates`를 ADK 템플릿 변경 제안(PR 초안)으로 만들기 | 자동 반영은 non-goal이고, 후보가 쌓인 실측이 아직 없다 | process·deterministic 후보가 여러 프로젝트에서 반복된다 |
| skillevolve가 stub 대신 실제 skill 수정안을 만들고 이 SPEC의 근거로 replay 입력을 채우기 | 생성기와 quality-index 입력 생산자가 없다(PR #214 조사) | skillevolve를 실제로 쓰겠다는 결정 |
| 운영 코드가 부르지 않는 `pkg/qualityloop`를 이 근거 체계로 흡수하거나 삭제하기 | 범위 밖 정리 작업이다 | 다음 정리 SPEC |
| OpenCode V2 plugin의 `ctx.session.hook("context")`로 lore-commit 등 trigger rule을 조건부로 전달하기 | OpenCode가 늘 로드하는 분량은 22.7KB이고, 그중 약 8KB가 trigger rule이다. bundle을 읽은 정적 근거뿐이라 실행 검증이 필요하다 | loopback fake provider로 system push를 확인 |
| 근거 명령을 sandbox에서 재실행해 `deterministic`을 확인하기 | 지금은 "확인됐다고 기록됨"이다. 재실행은 비용·안전 결정이 필요하다 | 지어낸 exit code가 실제로 관찰된다 |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| `pkg/learn`, `pkg/harneval/intake`, `pkg/adapter/opencode`, `pkg/adapter/omp`, `templates/*` 경로·함수·줄 번호 | existing | Read, grep(2026-10-24, 350ce05c) |
| Harness² 논문 수치·절 | external | arXiv PDF 2610.14224v1 본문과 부록 B–G를 읽음 |
| `evidence.go`, `summary_evidence.go`, `Evidence`, `EvidenceKind`, `PrunePolicy`, `PruneResult`, `PrunePolicyExcept`, `QueryScored`의 scope·bonus, CLI flag 다섯 | [NEW] | 이 branch에서 구현, test로 확인 |

## Reviewer Brief

- Intended scope: learn 저장소의 근거·범위 필드, 조회 가산·필터, llm_only 정리, 요약 줄, 다섯 플랫폼 `auto-go`·`auto-fix`·`auto-sync` 템플릿.
- Explicit non-goals: 자동 반영, 유사도 조회, skillevolve·qualityloop, OpenCode 조건부 rule, 이전 버전 보존.
- Self-verified: 기존 출력 고정 test 통과(S7), 생성 결과 grep(S8), decoder 비엄격(A1), intake fingerprint 입력(A2).
- Reviewer should focus on: 근거 종류 도출식, 거부 시 무기록, `--llm-only-days` 기본 30이 기존 항목에 닿지 않는지, 템플릿이 exit code를 지어내지 말라고 지시하는지, 그리고 Completion Debt only.

## Open Risks

- R1: `deterministic`은 템플릿이 넘긴 exit code를 믿는다. 지어낸 값은 막지 못한다(Trust Model). 재실행 확인은 Evolution Idea다.
- R2: 이전 버전 `auto`(v0.50.127 이하)가 `auto learn prune`으로 저장소를 다시 쓰면 `scope`·`evidence`가 사라진다. 항목은 남는다(fail-soft).
- R3: 다섯 플랫폼 템플릿이 learnings를 늘 조회하면 prompt가 최대 2000 토큰 늘어난다. 파일이 없으면 건너뛰고, Codex는 optional recall로 예산 축소 대상에 둔다.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: 인용한 경로·줄 번호를 350ce05c에서 Read·grep으로 확인했다
- Q-CORR-02 | status: PASS | attempt: 1 | files: plan.md, research.md | reason: 새 파일·타입·함수·flag는 [NEW]로 표시했다
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md | reason: 요구사항은 EventDriven·Ubiquitous EARS 형식이다
- Q-CORR-04 | status: PASS | attempt: 1 | files: research.md | reason: Reference Discipline이 existing, external, [NEW]를 나눈다
- Q-COMP-01 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: Must 8개가 모두 시나리오를 가진다
- Q-COMP-02 | status: PASS | attempt: 1 | files: acceptance.md | reason: 거부 경로(S2, S4), 보호 항목(S5), 호환(S7)을 다뤘다
- Q-COMP-03 | status: PASS | attempt: 1 | files: plan.md | reason: task 9개에 소유 파일을 적었다
- Q-COMP-04 | status: PASS | attempt: 1 | files: spec.md, research.md | reason: non-goal과 Evolution Idea를 나눴다
- Q-COMP-05 | status: PASS | attempt: 1 | files: research.md, acceptance.md | reason: Feature Coverage Map의 모든 slice가 covered다
- Q-COMP-06 | status: PASS | attempt: 1 | files: spec.md | reason: Traceability Matrix가 REQ 8개를 task·시나리오·invariant에 잇는다
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: live 확인이 필요한 항목이 없어 Completion Debt는 None이다
