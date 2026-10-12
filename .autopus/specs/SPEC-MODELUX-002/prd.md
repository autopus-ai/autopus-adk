# PRD: 에이전트 모델 구성 UX v2 (화살표 선택 폼 · 도구별 실제 모델 · OMP 지정)

- **SPEC-ID**: SPEC-MODELUX-002
- **Mode**: Short
- **Source**: SPEC-MODELUX-001 실제 터미널 smoke(S21) 뒤 사용자 피드백과 결정 표(planning context, 2026-10-12), worktree HEAD `44773355` 코드 확인
- **Status**: Draft
- **Date**: 2026-10-12
- **Target module**: `autopus-adk`
- **Release**: v0.50.125, SPEC-MODELUX-001과 같은 PR #196

## 1. 문제

SPEC-MODELUX-001이 init, update, quality에 넣은 구성 단계는 줄 입력이다. 실제 터미널에서 사용자는 화살표를 눌렀고 그 글자가 프롬프트에 찍혔다. 그룹 선택지는 `fable/opus/sonnet/haiku` 등급 이름뿐이라 Claude 모델로 읽혔고, Codex와 OMP를 따로 정할 길이 구성 단계에 없었다. 사용자 말: "사용성이 좋진 않네.. 화살표와 엔터로 수정할 수 있게 해야 할 것 같은데. 그리고 클로드만 나오고 코덱스는 지정 못하던데.. omp도 그렇고.."

같은 smoke에서 결함 네 가지가 더 보였다(`research.md` `## S21 실제 터미널 smoke 관찰`).
1. update가 감지해 더한 플랫폼(antigravity-cli, opencode, omp)이 표에 없었다.
2. S21의 "autopus.yaml SHA 불변" oracle은 감지 저장 때문에 틀렸다.
3. init과 update 맨 앞에 OSC 11 query(`ESC]11;?ESC\`)가 나간다.
4. 그룹 선택지가 등급 이름뿐이다.

## 2. 목표

- 대화형 구성 단계의 모든 질문을 init wizard와 같은 모양의 huh 폼(화살표 + Enter)으로 한다. scripted·pipe 입력은 SPEC-MODELUX-001 줄 driver를 그대로 쓴다.
- 도구 탭으로 `모든 도구 같게`, Claude Code, Codex, OMP를 고른다. 선택지마다 그 도구의 실제 모델과 effort가 보인다.
- Claude·Codex 도구별 결과는 기존 per-provider binding(`quality.providers.<p>` + `quality.presets.<p>-custom`)에 저장한다. 새 YAML key는 없다.
- OMP는 init, update, quality 모두에서 지정할 수 있게 한다. update·quality는 기존 OMP writer, init은 같은 plan 검사를 거친 메모리 config와 init의 저장을 쓰고, catalog·계열 증명·placeholder 규칙은 그대로다. init에서 catalog를 못 읽으면 안내하고 계속한다.
- update 표에 감지 플랫폼 열을 넣고, 유지 경로의 oracle을 "quality 블록 bytes 불변"으로 고친다.

## 3. 사용자

- 실제 터미널에서 `auto update`를 자주 돌리는 기존 사용자(주 사용자)
- Codex나 OMP를 Claude와 다르게 쓰려는 사용자
- `--yes`, CI, scripted test(SPEC-MODELUX-001 계약 그대로)

## 4. 결정 (사용자와 확정, 다시 묻지 않음)

| 항목 | 결정 | 이 SPEC에서 정한 세부 |
|---|---|---|
| 입력 | 모든 대화형 단계를 huh 폼으로. 비TTY·scripted는 줄 driver 유지. 폼은 seam으로 test | 폼 판정·실행 seam 2개, huh accessible mode test(REQ-MUX2-01, 17) |
| 도구별 | 도구 탭 먼저. 설치된 도구만. OpenCode·Antigravity는 고르지 않음. 도구별 실제 모델 이름 | 라벨은 후보 config의 표 칸(REQ-MUX2-05, 06) |
| 저장 | 기존 per-provider binding, 새 key 없음 | 상대 등급 저장, 이름은 기존 `<provider>-custom`(REQ-MUX2-07) |
| OMP | 이번에 지정 가능, 기존 OMP 경로 재사용. init에서도 지정(2026-10-12 추가 결정): 메모리 config로 plan·검증, init의 `config.Save`가 쓰고, catalog가 없으면 안내 후 계속 | 기존 OMP 편집기의 폼 판. update·quality는 `applyOMPProfile`과 cfg 동기화, init은 `[NEW] planOMPProfileForConfig`와 placeholder 검사(REQ-MUX2-10, 11, 22, 23) |
| 출하 | v0.50.125, 범위를 좁게, 새 secret·승인·유료 CI 없음 | sibling 없음, 새 의존성 없음 |

## 5. 범위 밖

줄 driver 개선, 새 YAML key, OMP를 quality 등급에 묶기, 표 OMP 칸의 실제 route, update에서 이번 run에 처음 감지된 OMP 지정, OMP pin 해제 UI, 여러 도구 일괄 쓰기, OMP writer의 문서 전체 재encode 변경, huh·bubbletea v2 이전(OSC 11 근본 해결), OpenCode·Antigravity 에이전트별 모델.

## 6. 성공 기준

- 실제 터미널 smoke(S20)에서 화살표·Enter로 Codex 탭을 적용하고, 같은 run의 Codex 생성 파일이 미리보기와 같다.
- 유지 경로에서 quality 블록과 `role_model_policy` bytes 변경 0, 비대화형 stdin 읽기와 폼 실행 0회.
- 기존 test 수정 0건, 줄 입력 출력이 HEAD 44773355와 bytes가 같다.
- 라벨 = 같은 snapshot으로 돈 실제 `Generate` 값(S14).
- init에서 고른 OMP pin이 init이 만든 autopus.yaml에 그대로 있고, catalog를 못 읽으면 안내 뒤 init이 끝까지 돈다(S21, S22).

## 7. 위험과 가정

- 기존 update test가 플랫폼 감지를 고정하지 않으면 CLI가 설치된 기계에서 표 단언이 바뀔 수 있다(미확인, R1).
- huh accessible mode를 여러 form에 이어 쓸 때 입력 소비가 1byte reader로 안정적인지 실행 확인 전이다(probe A2).
- OMP 쓰기 뒤 cfg 동기화가 update와 `--apply`의 뒤 저장·생성을 모두 덮는지 실행 확인 전이다(probe A3).
- Claude 라벨은 slug(`opus-5-5`), 표 칸은 frontmatter 값(`opus`)이다. 사용자 confirm 대상인 표시 결정이다.
- OSC 11 query는 bubbletea 초기화의 기존 동작이라 이번에는 기록만 한다.
- init에서 OMP를 고르면 init 생성 단계에 OMP model integration 실패 경로가 생기고, 실패하면 init 전체가 rollback된다(plan이 직전에 같은 catalog를 확인한다).
- init 종단 test에서 OMP 생성을 실제 `omp` 없이 돌리려면 OMP descriptor에 test 전용 runner seam(`[NEW] platformOMPModelRunner`)이 필요하다. adapter에는 `WithModelIntegrationRunner`가 이미 있다. 설정 되읽기에 답하는 가짜를 만들 수 있는지는 미확인이다.

## 8. 사용자에게 남은 질문

- 없음. init의 OMP 지정은 사용자가 정했다(2026-10-12, REQ-MUX2-22, REQ-MUX2-23).
