# PRD: 에이전트별 effort 편집과 OMP 기본값을 Autopus 에이전트로

- **SPEC-ID**: SPEC-MODELUX-003
- **Mode**: Short
- **Source**: SPEC-MODELUX-002 S20 smoke 뒤 사용자 피드백과 결정 표(planning context, 2026-10-12), worktree HEAD `93dbb564` 코드 확인, omp 18.6.1 probe
- **Status**: Draft
- **Date**: 2026-10-12
- **Target module**: `autopus-adk`
- **Release**: v0.50.125, SPEC-MODELUX-001·002와 같은 PR #196

## 1. 문제

사용자 말: "좋았어. 다만 effort도 수정할 수 있게 해줘야지. 그리고 omp도 내가 봤을 땐 기존 에이전트를 없애버리고, 우리 autopus 식으로 에이전트를 생성해서 이걸 디폴트로 해야 할 것 같아."

1. 구성 폼은 등급만 바꾼다. effort는 (mode, 등급)과 에이전트 선언으로 자동으로 정해지고 저장할 곳이 없다(`pkg/config/schema.go:24-27`).
2. OMP는 bundled agent 5개를 쓴다. Autopus 16개 역할이 task·scout·reviewer·security-reviewer·sonic으로 접혀, 역할마다 model과 thinking을 따로 줄 수 없다(`pkg/config/omp_native_agents.go:12-27`).

## 2. 목표

- Claude Code·Codex 탭에서 그룹·에이전트마다 그 도구가 받는 effort를 고르고 `quality.presets.<name>.efforts`에 저장한다. 값이 없으면 지금 자동 effort다. OMP 탭은 thinking을 고른다.
- OMP 기본값을 Autopus 에이전트로 바꾼다: `.omp/agents/<agent>.md` 16개 생성(model 없음), scout·security-reviewer·sonic은 `task.disabledAgents`, reviewer는 Autopus reviewer, task는 유지. 프로필이 있으면 route는 16개 이름(+task) key의 `task.agentModelOverrides`로 기존 활성화·증명·되읽기 경로를 탄다.
- `auto update`가 기존 프로젝트를 옮기고, `role_model_policy.agent_source: native`가 한 릴리스 동안 이전 방식을 되살린다.

## 3. 사용자

- 실제 터미널에서 `auto init`·`auto update`·`auto quality`를 쓰는 사용자(주 사용자)
- OMP를 Claude·Codex와 같은 Autopus 역할 단위로 쓰려는 사용자
- 사용자 `.omp/agents` 파일이나 `.omp/config.yml` 설정을 이미 가진 사용자(보존 대상)

## 4. 결정 (사용자와 확정, 다시 묻지 않음)

| 항목 | 결정 | 이 SPEC에서 정한 세부 |
|---|---|---|
| effort 저장 | `quality.presets.<name>.efforts.<agent>`, 없으면 자동, 옛 바이너리는 읽지 못함을 미리보기 한 줄로 | 값 `low`·`medium`·`high`·`xhigh`·`max`, load 검증, 명시 effort는 표준 배치를 떠남(REQ-MUX3-01, 02) |
| effort UI | 도구 탭별, 그룹·에이전트, 그 도구의 유효 값만 | Claude는 문서의 다섯 단계(haiku 제외), Codex는 run snapshot의 model 단계, OMP는 model이 보고한 thinking(REQ-MUX3-03, 13) |
| OMP 에이전트 | 16개 manifest 소유 생성, override key는 16개 이름, frontmatter에 model 금지, thinking은 effort에서 | `thinkingLevel` = 선언 effort, reviewer는 OMP `/review` verdict schema 포함, 옛 native pin은 접히던 역할로 펼침(REQ-MUX3-07, 08, 11) |
| native built-in | scout·security-reviewer·sonic은 `task.disabledAgents`, reviewer는 같은 이름으로 대체, task 유지 | project `.omp/config.yml` claim, 사용자 소유면 쓰지 않고 degraded. task를 끄면 이름 없는 dispatch가 실패한다(정적 확인, REQ-MUX3-10) |
| rollback | 이전 방식을 한 릴리스 opt-in으로 | `role_model_policy.agent_source: native`, 다음 릴리스 제거는 Evolution Idea(REQ-MUX3-06, 15) |
| 출하 | v0.50.125, PR #196, 새 secret·승인·유료 CI 없음 | sibling 없음, 새 의존성 없음 |

## 5. 범위 밖

프로필 없는 프로젝트의 등급 기반 OMP route(Q1), native 경로 제거, frontmatter concrete model과 OMP role alias, `모든 도구 같게` 탭의 effort, 줄 driver 변경, OMP writer 재encode 변경, OpenCode·Antigravity effort, OMP 버전별 `/review` schema 추적.

## 6. 성공 기준

- 실제 터미널 smoke(S17)에서 Claude 탭 effort를 적용하고 같은 run의 생성 파일이 미리보기와 같으며, OMP `/agents`에 Autopus 16개와 task가 있고 셋이 꺼져 있다.
- `efforts`가 없는 파일의 생성 결과가 HEAD 93dbb564와 bytes가 같다(S1).
- override key가 16개 이름 + task이고 각 값이 같은 run의 plan과 같다(S9).
- 사용자 `.omp/agents` 파일과 사용자 소유 `task.disabledAgents`의 bytes가 그대로다(S8). native 왕복이 HEAD 93dbb564 OMP 표면으로 돌아간다(S13).

## 7. 위험과 가정

- plain OMP 세션의 발견·대체·숨김·이름별 override 적용은 정적 근거와 설정 되읽기(P3)로만 확인했다. 18.6.1에 읽기 전용 목록이 없어 S17이 TUI `/agents`로 닫는다.
- 프로필 밖 `task.disabledAgents` claim은 기존 claim 경로의 새 사용이다(probe A3, T5 첫 작업).
- 기본값이 바뀌어 기존 test 일부가 바뀐다. 규칙(fixture 한 줄 또는 기본값 전환 단언)과 목록(T1)으로 review한다.
- RPC 되읽기가 5회에서 17회로 는다. `efforts`나 `agent_source`를 쓴 파일은 v0.50.113–v0.50.124가 읽지 못한다.

## 8. 사용자에게 남은 질문

- Q1 (해결 2026-10-13): 프로필이 없으면 세션 모델을 상속한다. 등급 기반 자동 route는 하지 않는다.
