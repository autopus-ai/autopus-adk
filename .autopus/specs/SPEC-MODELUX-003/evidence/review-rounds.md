# SPEC-MODELUX-003 review round 반영표

`research.md`에서 옮겼다(2026-10-14). 각 행은 그 round의 finding ID와 이 SPEC에서 고친 자리다.

## Review Round 1
| ID | 반영 |
|---|---|
| F-001 | task 규칙, 접힘 검증 native 한정, autopus 검증(REQ-MUX3-11, Route Contract, T3, S21) |
| F-002 | Claim Set Contract, 전환·release·clean·실패 복원(REQ-MUX3-10, 15, T5, S22) |
| F-003 | Agent Set Contract에 production 소비처 전부, degraded 규칙(REQ-MUX3-12, T6, S21) |
| F-004 | S9는 `config get` 한 번의 17 key record, RPC 17회는 `RunWithInput` fake |
| F-005 | S13 기준 run은 v0.50.125 바이너리로 agent_source 줄 없는 파일 |
| F-006 | REQ-MUX3-09에 Generate 쓰기 loop와 Update resolver 두 자리 |
| F-007 | 27개 목록과 기본값 전환 단언 4개 파일(위 `### 기존 test`) |
| F-008 | OMP 탭 pre-plan과 blocker 처리(REQ-MUX3-13, S11) |
| F-009 | 그룹 effort Select는 haiku가 아닌 멤버로 |
| F-010 | 기각 근거를 attestation 비용으로, 주석 2곳 갱신(T4, T5) |
| F-011 | header: branch `feat/modelux-003`, v0.50.126 |
| F-012 | REQ-MUX3-09: 디스크 checksum과 manifest 기록 checksum 비교 |
| F-013 | Efforts Preservation Contract(REQ-MUX3-20, T10, S20) |
| F-014 | 폼 test 입력 규칙, 불변 범위 축소(REQ-MUX3-16, S16) |
| F-015 | OMP Render Mode Contract, 템플릿 문장, 실제 Generate oracle(REQ-MUX3-21, T8, S14) |
| F-016 | metadata 없는 catalog 규칙(REQ-MUX3-13, S10) |
| F-017 | S17을 plain 세션과 `PI_CONFIG_FILES` 세션으로 나눔 |
| F-018 | thinking 질문 제거, reviewer golden, S23과 Completion Debt |
| F-019 | 백업 사실 정정, 기존 transaction 백업 재사용 |
| F-020 | REQ-MUX3-02, 06, 11을 `WHERE … THEN` 형식으로 |
## Review Round 2
| ID | 반영 |
|---|---|
| F-002 | 섞인 상태에서 `task.disabledAgents`만 바뀐 판정, 사용자 소유 전환·표시·재claim(Claim Set Contract, S24) |
| F-008 | 탭이 더하는 catalog 읽기는 pre-plan 1회, 기존 경로의 횟수는 단언하지 않음(REQ-MUX3-13) |
| F-013 | 모든 도구 같게의 `custom`에도 편집 전 binding 프리셋 efforts를 옮김(Efforts Preservation Contract, S20 넷째) |
| F-021 | S22(e)는 내용을 바꿔 실제 transaction을 열고 `ompRootedWriteFailureHook`으로 마지막 write를 실패시킴 |
| F-022 | S21 task pin 사본은 11개 역할 + task = 12개 key |
## Review Round 3
| ID | 반영 |
|---|---|
| F-002 | 사용자 소유 표시는 C가 비어 release해도 표시만 남은 ledger로 남고 `auto clean`만 지운다(Claim Set Contract, S24 첫 사본) |
| F-008 | 설계 결정 행을 pre-plan 1회·횟수 미단언으로 맞춤 |
| F-023 | `composeBindTarget` built-in 분기: P에 efforts가 있으면 다른 built-in 대신 `custom`, 같은 이름 Historical만 갱신(Efforts Preservation Contract, S20 다섯째) |
## Review Round 4
| ID | 반영 |
|---|---|
| F-002 | 첫 claim 판정(이미 있는 다른 값은 처음부터 사용자 소유·표시, 같은 값은 preimage와 같게 claim)과 표시만 남은 ledger의 읽기 전용 판단(Claim Set Contract, S8 UC·UC2) |
