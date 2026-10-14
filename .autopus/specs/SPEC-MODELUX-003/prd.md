# PRD: 에이전트별 effort 편집과 OMP 기본값을 Autopus 에이전트로

- **SPEC-ID**: SPEC-MODELUX-003
- **Mode**: Short
- **Source**: SPEC-MODELUX-002 smoke 뒤 사용자 피드백과 결정 표(planning context, 2026-10-12), Q1 답(2026-10-13), review round 1(F-001–F-020), `v0.50.125` 코드, omp 18.6.1 probe
- **Status**: Draft (revision 5)
- **Date**: 2026-10-13
- **Target module**: `autopus-adk`
- **Release**: v0.50.126, branch `feat/modelux-003`. v0.50.125는 SPEC-MODELUX-001·002로 이미 발행됐다

## 1. 문제

사용자 말: "좋았어. 다만 effort도 수정할 수 있게 해줘야지. 그리고 omp도 내가 봤을 땐 기존 에이전트를 없애버리고, 우리 autopus 식으로 에이전트를 생성해서 이걸 디폴트로 해야 할 것 같아."

1. 구성 폼은 등급만 바꾼다. effort는 (mode, 등급)과 에이전트 선언으로 자동으로 정해지고 저장할 곳이 없다(`pkg/config/schema.go:24-27`).
2. OMP는 bundled agent 5개를 쓴다. Autopus 16개 역할이 task·scout·reviewer·security-reviewer·sonic으로 접혀 역할마다 model과 thinking을 따로 줄 수 없다(`pkg/config/omp_native_agents.go:12-27`).

## 2. 목표

- Claude Code·Codex 탭에서 그룹·에이전트마다 그 도구가 받는 effort를 고르고 `quality.presets.<name>.efforts`에 저장한다. 값이 없으면 지금 자동 effort다. 다른 편집과 갱신은 저장된 effort를 지우지 않는다. OMP 탭은 thinking을 고른다.
- OMP 기본값을 Autopus 에이전트로 바꾼다: `.omp/agents/<agent>.md` 16개 생성(model 없음), scout·security-reviewer·sonic은 `task.disabledAgents`, reviewer는 OMP `/review`와 맞는 Autopus reviewer, task는 유지. 프로필이 있으면 route는 16개 이름과 task를 key로 한 `task.agentModelOverrides`로 기존 활성화·증명·되읽기 경로를 탄다. 생성되는 OMP 문서와 dispatch도 Autopus 이름을 쓴다.
- (S17 뒤 2026-10-14) autopus 모드 OMP 탭은 프로필·family를 묻지 않는다. 역할 그룹(또는 에이전트별)마다 provider가 섞인 증명된 설치 model이나 `OMP 세션 모델 그대로`를 고르고 thinking은 그 model이 지원하는 단계만이다. 결과는 프로필 없는 pin으로 저장되고 pin된 에이전트만 override key를 받는다. 기존 프로필 설정은 바꾸지 않은 에이전트의 route를 pin으로 옮겨 변환한다.
- `auto update`가 기존 프로젝트를 옮기고, `role_model_policy.agent_source: native`가 v0.50.126 동안 이전 방식을 되살린다.

## 3. 사용자

- 실제 터미널에서 `auto init`·`auto update`·`auto quality`를 쓰는 사용자(주 사용자)
- OMP를 Claude·Codex와 같은 Autopus 역할 단위로 쓰려는 사용자
- 사용자 `.omp/agents` 파일이나 `.omp/config.yml` 설정을 가진 사용자(보존 대상)

## 4. 결정 (사용자와 확정, 다시 묻지 않음)

| 항목 | 결정 | 이 SPEC에서 정한 세부 |
|---|---|---|
| effort 저장 | `quality.presets.<name>.efforts.<agent>`, 없으면 자동, 옛 바이너리는 못 읽음을 한 줄로 | 다섯 단계, load 검증, 명시 effort는 표준 배치를 떠남, 모든 복사·갱신 경로가 보존(REQ-MUX3-01, 02, 20) |
| effort UI | 도구 탭별, 그룹·에이전트, 유효 값만 | Claude는 문서의 다섯 단계(haiku는 대상에서 뺌), Codex는 snapshot의 model 단계, OMP는 model이 보고한 thinking(REQ-MUX3-03, 13) |
| OMP 에이전트 | 16개 manifest 소유 생성, override key는 16개 이름, frontmatter에 model 금지, thinking은 effort에서 | route가 없는 에이전트만 `thinkingLevel` = 선언 effort, reviewer `output`은 18.6.1 golden, task는 task·planner pin 또는 프로필 planner route(REQ-MUX3-07, 08, 11) |
| native built-in | 셋은 `task.disabledAgents`, reviewer는 대체, task 유지 | project `.omp/config.yml`에 프로필 key와 합친 claim 집합 하나로, 사용자 소유면 쓰지 않음(REQ-MUX3-10) |
| rollback | 이전 방식을 한 릴리스 opt-in으로 | `role_model_policy.agent_source: native`, v0.50.127 제거는 Evolution Idea(REQ-MUX3-06, 15) |
| 프로필 없는 프로젝트(Q1) | 세션 모델 상속 | 자동 route는 non-goal |
| OMP 탭(S17 뒤, 2026-10-14) | 프로필·family 없이 멀티프로바이더, 첫 선택지 `OMP 세션 모델 그대로`, 증명된 model만 | pins-only `role_model_policy.agents`, metadata 없는 catalog는 built-in 선언으로 증명, 기존 프로필 설정은 변환(REQ-MUX3-13, 22, 23) |
| 출하 | 새 secret·승인·유료 CI 없음 | v0.50.126(결정 표의 v0.50.125는 이미 발행), sibling 없음, 새 의존성 없음 |

## 5. 범위 밖

프로필 없는 프로젝트의 등급 기반 OMP route(Q1), native 경로 제거, frontmatter concrete model과 OMP role alias, `모든 도구 같게` 탭의 effort, 줄 driver 변경, OMP writer 재encode 변경, 프로필 claim key와 claim 밖 편집의 conflict 규칙 변경(`task.disabledAgents`만 바뀐 경우는 사용자 소유로 넘긴다), OpenCode·Antigravity effort, OMP 버전별 `/review` schema 추적.

## 6. 성공 기준

- OMP 탭에서 설계·판단 anthropic, 구현 openai, 탐색·보조 세션 모델을 고르면 override가 pin된 10개 에이전트와 task뿐이고 프로필·family 단계가 없다(S10). 기존 프로필 설정을 고쳐도 바꾸지 않은 에이전트의 route가 그대로다(S25).
- 실제 터미널 smoke(S17)에서 Claude 탭 effort를 적용하고 같은 run의 생성 파일이 미리보기와 같으며, OMP `/agents`에 Autopus 16개와 task가 있고 셋이 꺼져 있다. 동의 뒤 S23에서 이름별 override와 `/review` verdict가 확인된다.
- `efforts`가 없는 파일의 생성 결과와 줄 driver 출력이 v0.50.125와 같다(S1, S16).
- override key가 16개 이름과 task이고 값이 같은 run의 plan과 같다(S9, S21). 생성된 OMP 문서에 native agent 선택 문장이 없다(S14).
- 사용자 파일과 사용자 소유 key의 bytes가 그대로이고, claim 집합 전환과 실패 뒤 bytes가 맞다(S8, S22). native 왕복이 v0.50.125 OMP 표면으로 돌아간다(S13).

## 7. 위험과 가정

- 프로필 없는 `agents`는 v0.50.125 이하가 거부한다. pins-only가 기존 plan·쓰기·활성화·claim 경로를 그대로 탄다는 것은 구현 확인 전이다(T15). 변환하면 ultra의 quality 연동과 family diversity 힌트가 고정된다.
- plain OMP 세션의 발견·대체·숨김과 이름별 dispatch, `/review` verdict는 정적 근거와 설정 되읽기(P3)로만 확인했다. S17·S23이 기록되기 전에는 Completion Debt CD-1·CD-2가 남는다.
- 합친 claim 집합의 merge·release·clean은 기존 ledger의 새 사용이다(probe A3, T5 첫 작업). ledger에 owner 필드를 더할 때 v0.50.125가 그 파일을 읽는지 미확인이다.
- 기본값이 바뀌어 기존 test 27개 파일 중 일부가 바뀐다(단언 4개 파일, 나머지는 fixture·option·폼 입력 규칙).
- RPC 되읽기가 5회에서 17회로 는다. `efforts`나 `agent_source`를 쓴 파일은 v0.50.113–v0.50.125가 읽지 못한다.

## 8. 사용자에게 남은 질문

- Q1 (해결 2026-10-13): 프로필이 없으면 세션 모델을 상속한다. 등급 기반 자동 route는 하지 않는다.
- 남은 질문은 없다. 단, 결정 표의 출하 버전 v0.50.125는 그 사이 발행돼 v0.50.126으로 옮겼다.
