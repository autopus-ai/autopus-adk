# SPEC-HARNESS-BENCH-002 리서치

## User Decisions (rev 2)

2026-11-04 사용자 결정을 main session이 전달했다. 원칙은 "알려진 편향은 공개가 아니라 제거한다"이다. 목적은 harness가 실제로 효과가 있는지 배우는 것이다.

| # | 결정 | 반영 |
|---|---|---|
| 1 | ADK reverse-PR 대신 Autopus와 무관한 외부 OSS(Go 우선, 부족하면 Python)의 cutoff 뒤 PR을 기계적 규칙으로 6개 고른다. harness arm은 snapshot에 pinned `auto init`. license 기록, 외부 코드 비commit | REQ-HB2-03, 04, 05, 18 |
| 2 | 어떤 provider도 `null` 금지. machine-readable usage로 재고 parse 불가는 infra failure. 사전등록 전 세 provider probe, agy usage가 없으면 blocker | REQ-HB2-10, 19, RFP-1, CD-1 |
| 3 | prompt는 PR 제목·본문(또는 linked issue)에서 동결 template과 strip 규칙으로 도출한다 | REQ-HB2-16 |
| 4 | seeded 순서, 같은 machine 직렬 interleave, version 기록 | REQ-HB2-07 |
| 5 | 두 arm 모두 trial별 격리 config home | REQ-HB2-17 |
| 6 | blind 채점, arm mapping은 grade 기록 뒤 | REQ-HB2-09 |
| 7 | 추가 provider와 agent는 처치이고 이를 명시한다 | REQ-HB2-20 |

rev 3–5는 이 원칙을 review finding에 적용했고, 통제로 바꿀 수 없는 편향은 `잔여`로 낮췄다. 규모(6×1)와 R1은 유지한다.

## 기존 코드 분석

2026-11-04 origin/main `4f21f309`(= HEAD)에서 Read와 `rg`로 확인했다.

- `scripts/benchmarks/harness/`: `workspace.snapshot`은 harness 파일을 root에서만 지운다(`workspace.py:49-54`). `grader.allowlist`(`grader.py:41-45`), `grader_argv`는 `prepared['warm_cache']`를 읽는다(`:115-119`). `judge`는 bare test 이름만 비교하고(`:247-274`, `set(expected_tests)` `:254`) `grade`는 늘 그 판정을 돌려준다(`:278-285`). `prepare_grader.py:244`는 package 인자를 `./` 형식으로 넘긴다. `grader.sb`는 network·mach-lookup을 모두 거부하고(`:21-23`) home·temp 아래 읽기를 default-deny하며(`:42-53`) Keychain을 닫는다(`:55-58`). `prepare_grader.prepare`는 download·verify 뒤 sandbox 밖에서 `go test -count=1 -run '^$'`를 돈다(`:133`). `test_golden.py:99-106`은 golden이 load한 module 집합이 `RUNNER_FILES`와 같다고 본다. 그 밖에 `golden.Refusal`·`Steps`, `golden_protocol.runner_digest`, `process_tree.Tree`, `observe.unique_object`를 재사용한다.
- `autopus.yaml:29-32` language `en/ko/ko`, `:52-56` review_gate, `:78-97` orchestra providers. `internal/cli/init.go:139-142`는 host locale로 `language.*`를, `:150-153`은 provider 2개 미만이면 gate를 끈다.
- `auto-workflows.md.tmpl:93`은 `$ARGUMENTS` 어디서든 global flag를 떼고 `:739-747`은 plan flag를 뗀 나머지를 `FEATURE_DESC`로 쓴다. `content/agents/planner.md:6`, `spec-writer.md:6`은 `WebSearch`, `WebFetch`를 선언한다. review 경로: 투영 `orchestra_readonly_policy.go:224-226`·`:236-258`, catalog probe `codex_catalog_runtime.go:16`·`:43-58`, readiness `spec_review_readiness.go:62-81`·`provider_readiness.go:102-105`, fallback `codex_profile_resolve.go:52-61`. 언어 `claude_markers.go:148-157`, `tui/locale.go:11-21`. version 문자열 `version.go:82-84`. provider cwd는 auto process cwd(`spec_review_loop.go:108-109`), timeout은 process group SIGKILL(`pkg/orchestra/command_unix.go:18-25`).
- `pkg/spec/types.go:193`(Revision 0 = 첫 review), `review_persist.go:39`, `internal/cli/spec_review_loop.go:53`. `pkg/orchestra/usage.go:39-63`은 agy를 `Supported=false`로 둔다. `.goreleaser.yaml:8-27`은 `-trimpath`, `-buildvcs=false`, `CGO_ENABLED=0`, `GOENV=off`, version ldflags다.
- 로컬 CLI(model 호출 없음): Claude Code 2.1.314, codex-cli 0.163.0, agy 1.4.12, auto 0.50.133. `claude --help`에 `--disallowedTools`·`--tools`·`--strict-mcp-config`, `codex --help`에 `--search`(켤 때만 web search), `agy --help`에 `--output-format`·`--settings`가 있고 web tool flag는 없다.

## Plan Intent Ledger

direct `auto plan`에서 main session이 인라인으로 넘긴 ledger를 요약했다. 셀은 untrusted 근거다.

| Field | Status | Source | Confidence | Decision / Assumption | If Wrong | Plan Handoff |
|---|---|---|---|---|---|---|
| goal | answered | user | high | 비싼 harness 경로가 중간 규모 작업에서 비용만큼 품질을 사는지 배운다 | – | Outcome Lock |
| scope_boundary | answered | user | high | 6 task × arm당 1 run, Claude Code만, 유의성 주장 없음 | 더 큰 corpus | non-goals |
| constraints | answered (rev 2) | user | high | 두 arm 같은 model·effort, 사람 개입 0, 격리 config home, 편향 제거 | – | REQ-HB2-07, 17 |
| done_evidence | answered | user+main | high | push된 사전등록, script 생성 보고서, `claim_allowed`일 때만 README | – | S17 |
| brownfield_impact | answered (rev 2) | user | high | 외부 OSS corpus, BENCH-001 격리·채점 재사용, pilot·golden 불변 | – | REQ-HB2-03, 15 |

main session 결정(assumed)은 다음과 같다. **Q6** claim이 불허되면 README 변경은 0이다. **Q4** run당 180분이다. **Q5** native도 subagent를 쓸 수 있다. **Q3**은 rev 2 결정 1로 대체됐다.

## Question Audit

`question_transport=AskUserQuestion`, `question_count=1`, `unresolved_fields=[constraints.model, brownfield_impact.corpus_source, Q6]`. rev 2 사용자 결정이 앞의 둘을 해소했고, Q6은 assumed로 남는다.

## Outcome Lock

- **User-visible outcome**: maintainer가 `run.py --mode feature`로 사전등록된 12-run pilot을 실행한다. corpus는 외부 OSS의 cutoff 뒤 PR이다. 보고서(Markdown + JSON)가 commit된다. 보고서는 blind 채점 결과, `null` 없는 provider별 token, wall time, review defect, 처치와 남은 편향, R1 판정을 담는다. README 문구는 `claim_allowed: true`일 때만 넣는다.
- **Mandatory requirements**: REQ-HB2-01 ~ REQ-HB2-25.
- **Explicit non-goals**: spec.md `## Outcome Boundary`와 같다.
- **Completion evidence**: spec.md Completion evidence 1~7, acceptance S1–S19.

## Visual Planning Brief

`plan.md`의 flowchart를 쓴다. UX wireframe gate는 해당 없다(`wireframe intent: n/a`).

## 설계 결정

편향별 통제는 `## Bias Ledger`에 있다. 여기에는 그 밖의 선택 이유만 둔다.

1. **window는 merge 시각만으로 정하지 않는다.** merge 전에 공개된 diff와 issue 토론도 학습될 수 있다. **사전등록에는 식별자와 hash만 둔다.** `corpus_sha256`은 불변 field만 해시해 메타데이터를 다시 받아 순서를 다시 뽑을 수 없다.
2. **API 절과 보수적 unexported 판정.** 공개 API 이름이 없으면 두 arm 모두 floor effect가 생긴다. closure 이름 비교는 과하게 제외할 뿐 편향을 만들지 않는다.
3. **통제 위반은 fail이 아니라 claim 차단이다.** fail로 세면 위반한 arm만 손해를 봐 비대칭이 생긴다. `null`과 `not_applicable`도 같은 원칙이다: 읽지 못한 값은 claim 차단이다.
4. **prompt는 파일로, 검색은 tool 단위로 막는다.** router는 `$ARGUMENTS` 어디서든 flag를 떼고, proxy는 허용 host 안에서 도는 provider 쪽 검색을 막지 못한다.
5. **warm build cache 대신 cold compile.** 외부 코드를 sandbox 밖에서 실행하지 않는 비용이다.

## Technology Stack Decision

brownfield다. Python stdlib과 Go stdlib만 쓰고 새 의존성은 없다. 도구 version은 사전등록 시점 값을 pin한다.

## Minimality Decision Matrix

| Ladder step | Evidence | Decision | Receipt item |
|-------------|----------|----------|--------------|
| actual need | Outcome Lock, rev 2 결정 7개, rev 3–6 review finding | proceed | 편향 제거 pilot |
| existing code/helper/pattern | `workspace.*`, `grader.grader_argv`·`run_sandboxed`·`parse_events`·`telemetry_off`, `prepare_grader.new_grade`, `golden.Refusal`/`Steps`, `golden_protocol.*`, `process_tree.Tree`, `observe.unique_object` | reuse | 격리·채점·digest·거부. 채점과 calibration은 모두 `feature_grade.judge_pairs`다. `prepare_grader.prepare`, `grader.grade`, `grader.judge`는 쓰지 않는다 |
| stdlib/native | `subprocess`, `hashlib`, `json`, `socket`/`selectors`(proxy), `go/parser`·`go/ast`, `sandbox-exec`, `log stream` | use | 새 의존성 없음 |
| existing dependency | runner는 stdlib만 쓴다. ADK `go.mod`에 x/tools가 없다 | not sufficient | go/types 대신 closure 이름 비교 |
| new dependency or abstraction / new dependency or new abstraction | 의존성 추가 없음. 새 모듈은 300줄 한도와 관심사 분리 때문이고 framework는 없다 | accepted | `feature_*` |
| minimum sufficient verification | offline S1–S16·S18·S19(sandbox·proxy 실제 실행 포함), source-lines, T0·T10 probe, OPS S17 | required checks | security·validation·data-loss·deterministic-oracle gate는 줄이지 않는다 |

## Semantic Invariant Inventory

| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-001 | "native pass·harness fail task 0개" | paired matching | `reasons` reversal | S13 |
| INV-002 | "harness ≥ native + 2" | numeric formula | `inputs`, `margin_below_2` | S13 |
| INV-003 | "null 금지, parse 불가는 claim 차단, cached는 부분집합" | numeric formula / parser | provider 4필드, 합계, `usage_status` | S10, S14, S1 |
| INV-004 | "불변 field hash, seeded randomisation, 직렬, 같은 machine" | ordering | `corpus_sha256`, 순서, 종료 사유 | S7, S17 |
| INV-005 | "parent + solution + oracle overlay, (package, test)" | set composition | 채점 사본, 기대 쌍 | S9, S19 |
| INV-006 | "같은 구성에서 merge pass, parent fail" | paired comparison | calibration 행 | S3, S19 |
| INV-007 | "사전등록이 첫 trial 전에 origin/main" | state precondition | refusal, `prereg_commit` | S1, S17 |
| INV-008 | "prompt·PR 텍스트·코드·home·credential 없음, digest" | report row / redaction | 보고서 | S14, S17 |
| INV-009 | "claim_allowed true일 때만 README" | equality | README 줄 | S15, S17 |
| INV-010 | "첫 agent 시작~마지막 종료, init·grading 제외" | numeric formula | `elapsed_seconds` | S11 |
| INV-011 | "reported/unknown/n/a, harness 경로, rounds = Revision + 1" | grouping | `review`, `harness_path` | S5, S12 |
| INV-012 | "surface 0, config home 격리, default-deny, 환경 allowlist, sandbox 밖 receipt" | state precondition | audit code, 읽기·egress 실패, env key 집합 | S6, S8 |
| INV-013 | "기존 mode 불변" | regression | 기존 test, 미변경 파일 | S16 |
| INV-014 | "window 시작 뒤, filter, closure, merge 순 앞 6개" | ordering / filter | 선택 목록, 제외 code | S2 |
| INV-015 | "제목+본문+issue, 동결 template, 파일 전달" | parser / text transform | prompt 바이트, argv | S4 |
| INV-016 | "arm label 없이 채점, mapping은 나중에" | ordering / redaction | 접근 순서, `grades.json` | S9, S17 |
| INV-017 | "거부 tool 호출이 보이면 통제 위반" | parser | `control_violations` | S18 |
| INV-018 | "terminal state마다 outcome 하나와 R1 효과" | grouping | `operational_error`, R1 reasons | S13, S18 |

## Feature Coverage Map

| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| 실현성 blocker 선확인, 사전등록과 usage gate | T0, RFP-1/2, CD-1~6, REQ-01/02/19, T1, T10, S1 | covered |
| 외부 corpus, prompt, oracle, calibration | REQ-03/04/16/18/24, T2, T3, S2–S4, S19 | covered |
| 두 arm, 순서, config home, 환경 | REQ-05/06/07/17/22, T4, S5–S8 | covered |
| 격리, egress, trusted receipt, tool 정책, blind 채점, outcome 분류 | REQ-08/09/21/23/25, T5, T6, S8, S9, S18 | covered |
| 지표 | REQ-10/11/12, T7, T8, S10–S12 | covered |
| 판정, 보고서, README, 편향 절 | REQ-13/14/20, T9, S13–S15 | covered |
| 실제 pilot과 공개, 보존 | T11–T13, S17, REQ-15, T14, S16 | covered |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| CD-1: agy `--output-format json`에 token usage가 있는지 미확인 | 사전등록(REQ-HB2-19)과 모든 run | T0 RFP-1 PASS. 없으면 `agy_usage_unavailable`. lower bound로 진행하지 않는다. CD-1~6 모두 새 secret·계정·승인 절차로 닫지 않고, 닫히지 않으면 benchmark를 차단하고 사용자가 처치 변경 여부를 정한다 |
| CD-2: claude·agy가 Keychain 없이 복사한 인증 파일로 로그인되는지 미확인(macOS Claude Code는 Keychain 사용). trial 안 refresh token 회전이 master를 무효로 만드는지 | 모든 trial | RFP-2 PASS. fallback은 trial 전 sandbox 밖 master 갱신(REQ-HB2-17)뿐이고 되쓰기는 없다. 회전이 master를 무효로 하면 `provider_auth_failed`로 claim이 막히므로 차단으로 본다. 파일 경로가 없으면 남는 길은 새 token(`claude setup-token`)이나 Keychain 개방인데, 앞은 새 secret이고 뒤는 무관한 항목을 노출하므로 blocker다 |
| CD-3: 투영된 review argv(`--ignore-user-config`)의 codex·agy 검색·URL 조회를 event로 관측할 수 있는지 미확인. 설정으로는 끌 수 없다 | 외부 조회 통제(REQ-HB2-21) | 투영 argv 그대로 실행한 RFP-2 PASS. 관측할 수 없으면 `provider_search_uncontrolled`로 차단한다. 제품 투영 변경은 처치 변경이라 쓰지 않는다 |
| CD-6: `codex debug models` catalog probe(5초, `codex_catalog_runtime.go:16`)가 sandbox·proxy 아래 성공해 review model이 대체되지 않는지, readiness 명령이 성공하는지 미확인 | 처치 동일성(REQ-HB2-10 model 대체 금지) | RFP-2 PASS. probe가 실패하면 모든 harness run이 `model_substituted`가 되므로 차단하고 사용자가 정한다 |
| CD-4: top-level Claude Code receipt가 subagent token을 포함하는지 미확인 | token 지표와 claim | RFP-1 PASS(포함하거나 subagent별 receipt 경로가 있음). 없으면 `subagent_usage_unavailable` |
| CD-5: `/` 기준 거부 아래 동결 읽기 목록으로 세 CLI가 동작하는지, Seatbelt 거부를 `log stream`으로·proxy 연결을 `lsof`로 pid에 귀속할 수 있는지, broker 아래 review가 trial cwd·환경·종료 규칙으로 제품처럼 동작하는지 미확인 | 격리 위반 검출, trusted receipt(REQ-HB2-08, 25) | RFP-2 PASS. 하나라도 안 되면 관측을 믿을 수 없으므로 차단한다 |

## Evolution Ideas

These are optional improvements and do not block sync completion.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| K≥3 반복과 corpus 확장 | 사용자가 6×1로 정했다 | 사용자 요청 |
| ablation arm, Codex·OMP 주 agent arm | Outcome Lock 밖이다 | 사용자 요청 |
| 제품 측 agy usage 파서, ledger cached 분리 | 제품 변경은 측정 대상을 바꾼다 | 사용자 요청 |
| go/types 기반 unexported 판정, loopback 허용 grader profile | 과한 제외는 편향이 아니고, loopback 허용은 로컬 서비스를 노출한다 | 적격 task 부족(RFP-3) |

## Sibling SPEC Decision

| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | 결과 하나, module 하나, 15 task·약 34 소스 파일(test 포함)로 기준(25·40) 아래 | None |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| `scripts/benchmarks/harness/{run,workspace,grader,prepare_grader,golden,golden_protocol,process_tree,observe,report,test_golden}.py`, `grader.sb`, `README.md`, `surface_driver/main.go` | existing | Read, `grep -n`(행 번호는 `## 기존 코드 분석`) |
| `pkg/orchestra/usage.go`, `pkg/spec/{types,review_persist}.go`, `pkg/orchestra/command_unix.go`, `internal/cli/{init,spec_review_loop,orchestra_readonly_policy,codex_catalog_runtime,spec_review_readiness,provider_readiness}.go`, `internal/cli/tui/locale.go`, `pkg/adapter/claude/claude_markers.go`, `pkg/config/codex_profile_resolve.go`, `pkg/version/version.go`, `templates/claude/commands/auto-workflows.md.tmpl`, `content/agents/{planner,spec-writer}.md`, `autopus.yaml`, `.goreleaser.yaml`, `go.mod` | existing (read-only) | Read |
| `agy --help`, `auto init --help`, `claude --help`, `codex --help` 출력 | existing (local CLI) | 2026-11-04 실행 |
| Tier 1·2 repository와 license, 세 model 공개 cutoff, codex `web_search` key, agy web tool key·state 위치, Claude Code 파일 인증 | external, unverified | T2·T11(GitHub API, vendor 문서), T0 RFP-2 |
| `feature*.py`, `feature_goapi/`, `feature_agent.sb`, `feature_provider.sb`, `feature_prereg.json`, `FEATURE_RUNNER_FILES`, `test_feature_*.py`, 보고서 | [NEW] planned addition | 아직 없음 |

## Bias Ledger

| 편향 | 상태 | 이유 |
|---|---|---|
| corpus·harness 누출 | 제거 | Autopus와 무관한 외부 OSS, ADK 의존성 repo 제외, 모든 깊이의 harness 파일 대칭 제거 |
| model 학습(해답 노출) | 제거(vendor 진술 전제) | 모든 model cutoff 최댓값 뒤 생성·commit·issue. 진술 자체의 정확성은 RB-2 |
| 외부 조회 | 제거(CD-3, CD-5 해소 전제) | claude 거부 tool 대칭 적용, pid 귀속 egress, broker가 sandbox 밖에서 본 모든 codex·agy model call(review 포함) event 관측 시 claim 차단 |
| prompt 선택·전달 | 제거 | 동결 template과 strip 규칙, 같은 `TASK.md`, flag 없는 argv |
| 순서·시간 | 기대값만 제거 | seeded 균형 순서, interleave, 같은 machine, CLI snapshot. 실제 불균형은 RB-1 |
| 설정·환경·credential 누출 | 제거 | 격리 config home(broker home은 agent가 못 읽음, master 되쓰기 없음), `/` 기준 읽기 거부와 session canary preflight(`--output` 위치와 무관), 빈 MCP, `env -i` allowlist, default-deny 읽기, download 전용 module cache, 두 arm 모두 영어 출력(한국어는 token을 더 써 token 비교를 기울인다) |
| oracle 구성·채점 | 제거 | 단일 구성 규칙, `(package, test)`, blind 경로, 통제 위반은 fail이 아니라 claim 차단(비대칭 없음) |
| token 관측 불가 | 제거 또는 차단 | `null` 금지, probe gate, CD-1, CD-4 |
| 처치 confound(추가 provider·agent·review, codex catalog probe·readiness 명령) | 측정 대상 | REQ-HB2-20, 제품 설정 그대로. model 대체는 통제 위반(CD-6) |
| RB-1: provider 서버 측 model revision, 계정 공유 prompt cache, 약 36시간 부하 변화 | 잔여 | 고객 측에서 pin하거나 끌 수 없다. n=6에서는 실제 불균형이 남을 수 있다 |
| RB-2: 공개 cutoff는 vendor 진술이고 출시 뒤 갱신될 수 있다 | 잔여 | 검증할 수 없다. 미공개 cutoff는 출시일로 대신해 보수적으로 줄였다 |
| RB-3: `auto` shim 메시지(review 횟수·시간)는 sandbox 안에서 나와 위조할 수 있다 | 잔여(부차) | R1이 읽는 receipt는 모두 sandbox 밖에서 만든다(REQ-HB2-25). 이 메시지는 부차 지표에만 쓴다 |
| RB-4: filter와 loopback 거부가 공개 API test가 있는 작은 PR, listener 없는 package 쪽으로 기운다 | 외적 타당성 | 두 arm에 같아 arm 편향은 아니다. HTTP 중심 repo가 빠지는 것을 보고서 한계 절에 쓴다 |
| RB-5: overlay가 출력을 JSON으로, 언어를 en으로 바꾼다(제품 repo는 text, ko). native에는 언어 지시가 없다 | 처치 정의·잔여 | 측정용 설정이다. RFP-1이 review 동작 유지를 본다. native가 영어로 답한다는 것은 영어 prompt에 기댄 가정이다 |
| RB-6: 재시작 금지는 절차로만 강제된다 | 잔여 | 다른 출력 경로의 숨은 session은 runner가 알 수 없다. `prior_sessions` 공개 규칙으로 줄인다 |
| RB-7: test-only 자산이 `testdata` 밖에 있으면 production으로 분류된다 | 잔여(대칭) | 두 arm 모두 floor 쪽으로 같게 작용한다. calibration은 통과할 수 있다 |

## Reviewer Brief

- **Intended scope**: feature mode runner, 외부 corpus 선택, oracle 구성, 사전등록, 격리, blind 채점, 보고서, README 가드, 1회 pilot 공개.
- **Explicit non-goals**: 유의성, 추가 arm, 제품 코드 변경, 외부 코드 commit, Python adapter, 새 secret·승인 절차.
- **Self-verified**: Traceability(REQ 25개), INV ↔ acceptance, 정확한 값(S4, S7 hex 재계산, S10, S13, S14, S19), 인용 행 번호, existing/[NEW]/unverified 구분, probe 3행.
- **Reviewer should focus on**: Bias Ledger의 `제거`가 통제로 뒷받침되는가, 통제의 실현성(CD-2~CD-6, 특히 broker·pid 귀속, 투영 argv 검색 관측, catalog probe), outcome taxonomy와 R1의 정합, Completion Debt only.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 6 | files: research.md, spec.md | reason: rev 6 인용 행(`spec_review_loop.go:108-109`, `orchestra_readonly_policy.go:260-262`, `command_unix.go:18-25`)을 다시 읽어 확인했다
- Q-CORR-02 | status: PASS | attempt: 3 | files: spec.md, plan.md, research.md | reason: [NEW]와 external unverified를 분리했다
- Q-CORR-03 | status: PASS | attempt: 3 | files: spec.md, acceptance.md | reason: EARS 문장과 bare Given/When/Then
- Q-CORR-04 | status: PASS | attempt: 3 | files: research.md | reason: Reference Discipline 3분류
- Q-COMP-01 | status: PASS | attempt: 3 | files: all | reason: 네 문서와 prd.md를 rev 5로 맞췄다
- Q-COMP-02 | status: PASS | attempt: 3 | files: spec.md, plan.md, acceptance.md | reason: REQ 25개가 task와 scenario에 연결된다
- Q-COMP-04 | status: PASS | attempt: 3 | files: research.md, plan.md | reason: OPS와 공개 포함, CD-1~6은 T0 blocker
- Q-COMP-05 | status: PASS | attempt: 3 | files: research.md, acceptance.md | reason: INV 18개가 정확한 값 scenario에 연결된다
- Q-COMP-06 | status: PASS | attempt: 3 | files: spec.md, research.md | reason: Traceability Matrix(REQ 25개)와 Reviewer Brief를 rev 5로 갱신했다
- Q-COMP-07 | status: PASS | attempt: 3 | files: research.md | reason: CD마다 해소 경로와 차단 조건, Evolution Ideas에 ID 없음
- Q-COMP-08 | status: PASS | attempt: 3 | files: plan.md | reason: RFP 3행 not-run과 reason, RFP-1·2는 T0
- Q-FEAS-01 | status: PASS | attempt: 3 | files: spec.md, plan.md | reason: runner 계층만 바꾸고 grader·prepare_grader·report는 그대로
- Q-STYLE-01 | status: PASS | attempt: 3 | files: spec.md, acceptance.md | reason: Priority·Type 분리, bare step keyword
- Q-SEC-01 | status: PASS | attempt: 3 | files: spec.md | reason: 외부 코드 build·실행은 sandbox 안에서만, prompt는 argv 밖
- Q-SEC-02 | status: PASS | attempt: 5 | files: spec.md, acceptance.md | reason: trusted receipt(fail closed), pid 귀속 egress, 되쓰기 없는 credential, redaction, canary 검사
