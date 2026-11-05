# SPEC-HARNESS-BENCH-002 리서치

## User Decisions (rev 2)

2026-11-04 사용자 결정(main session 전달). 원칙은 "알려진 편향은 공개가 아니라 제거한다"이다. (1) 외부 OSS의 cutoff 뒤 PR을 기계적 규칙으로 6개 고르고 harness는 snapshot에 pinned `auto init`(REQ-HB2-03, 04, 05, 18). (2) `null` 금지, machine-readable usage, 사전등록 전 probe(REQ-HB2-10, 19). (3) 동결 template·strip 규칙으로 prompt 도출(REQ-HB2-16). (4) seeded 순서, 같은 machine 직렬(REQ-HB2-07). (5) trial별 격리 config home(REQ-HB2-17). (6) blind 채점(REQ-HB2-09). (7) 추가 provider·agent는 처치(REQ-HB2-20). 규모(6×1)와 R1은 유지한다.

## T0 결과와 결정 (rev 7)

- T0(2026-11-05, `scratchpad/rfp1/RESULTS.md`, `rfp2/RESULTS.md`): RFP-1 조건부 PASS, RFP-2 FAIL. CD-4 PASS(`result.modelUsage`가 subagent 포함, `result.usage`는 top-level만). CD-1은 PR #267(agy stream-json, claude json) 조건, CD-6은 `SSL_CERT_FILE=/etc/ssl/cert.pem` 조건(없으면 `auto spec review`가 `gpt-5.6-sol`로 대체). CD-3 codex PASS(`type:"web_search"` item). CD-5 (b)(c)(d) PASS(144/145 `lsof` 귀속, 1건 fail-closed 거부, CLI group 0.071초 소멸).
- **D1**(CD-2: macOS agy 인증은 Keychain 전용, 파일 경로는 새 secret뿐): benchmark의 review panel은 claude+codex다. gemini/agy를 overlay에서 빼고 처치 정의, Bias Ledger(RB-9), claim 문구에 이탈로 적는다. quorum은 제품 규칙대로 2/2다.
- **D2**(CD-5: codex 자체 Seatbelt는 바깥 Seatbelt 안에서 `forbidden-sandbox-reinit`): broker는 codex를 `feature_provider.sb` 없이 돌리고 codex 자체 `--sandbox read-only`에 맡긴다. env allowlist, broker home, cwd 검사, process group kill, agent 밖 receipt는 유지하고, review codex의 경계를 RB-8에 정직하게 적는다. claude(review 포함)는 `feature_provider.sb` 아래 둔다.
- 투영 review argv(T0 캡처): codex `exec --json --sandbox read-only -m gpt-6-astra -c model_reasoning_effort="max" --ephemeral --ignore-user-config --ignore-rules --output-schema … --output-last-message …`(stdin prompt), claude `--print --model claude-opus-5-5 --effort max --permission-mode plan --safe-mode --no-session-persistence --disable-slash-commands --strict-mcp-config --tools=Read,Grep,Glob`. 이 캡처는 `4f21f309` 기준이라 claude에 `--output-format json`이 없고, #267 뒤에는 붙는다.

## 기존 코드 분석

origin/main `4f21f309`(2026-11-06에도 같음)에서 Read와 `rg`로 확인했다. PR #267(head `0dfcce9c`, open) 뒤의 동작은 `git show 0dfcce9c:<path>`로 확인했고, pinned `auto`의 C가 #267을 포함한다는 전제로만 쓴다.

- `scripts/benchmarks/harness/`: `workspace.snapshot`은 harness 파일을 root에서만 지운다(`workspace.py:49-54`). `grader.allowlist`(`grader.py:41-45`), `grader_argv`는 `prepared['warm_cache']`를 읽는다(`:115-119`). `judge`는 bare test 이름만 비교하고(`:247-274`, `set(expected_tests)` `:254`) `grade`는 늘 그 판정을 돌려준다(`:278-285`). `prepare_grader.py:244`는 package 인자를 `./` 형식으로 넘긴다. `grader.sb`는 `(allow default)`(`:22`) 위에 network·mach-lookup을 거부하고(`:23-24`) 쓰기는 grade root와 `/dev/null`만(`:25-28`), 읽기는 home·temp 아래만 거부하며(`:42-51`) Keychain을 닫고(`:55-58`) process-info·signal은 자기만(`:68-71`). `grader_argv`는 `-json`을 스스로 붙이고 명령의 `-json`은 거부한다(`grader.py:120-128`). `workspace.initialize`는 처음 세 git 명령에만 hook을 끈다(`workspace.py:121-128`). `prepare_grader.prepare`는 download·verify 뒤 sandbox 밖에서 `go test -count=1 -run '^$'`를 돈다(`:133`). `test_golden.py:99-106`은 golden이 load한 module 집합이 `RUNNER_FILES`와 같다고 본다. 그 밖에 `golden.Refusal`·`Steps`, `golden_protocol.runner_digest`, `process_tree.Tree`, `observe.unique_object`를 재사용한다.
- `autopus.yaml`(`4f21f309`): `:29-32` language `en/ko/ko`, `:52-56` review_gate, `:79-97` orchestra providers(claude 출력 형식 없음, agy text). #267(`0dfcce9c:autopus.yaml:79-90`)은 claude `--output-format json`, agy stream-json이다. ultra preset은 7 agent `fable`, 나머지 `opus`(`pkg/config/defaults.go:136-144`). 2명 panel quorum은 `DefaultMinProviders(2) = 2`(`pkg/spec/provider_health.go:236-241`). project MCP는 context7, full mode sequential-thinking(`templates/claude/mcp.json.tmpl`). `internal/cli/init.go:139-142`는 host locale로 `language.*`를, `:150-153`은 provider 2개 미만이면 gate를 끈다.
- `auto-workflows.md.tmpl:93`은 `$ARGUMENTS` 어디서든 global flag를 떼고 `:739-747`은 plan flag를 뗀 나머지를 `FEATURE_DESC`로 쓴다. `content/agents/planner.md:6`, `spec-writer.md:6`은 `WebSearch`, `WebFetch`를 선언한다. review 경로: 투영 `orchestra_readonly_policy.go:224-226`·`:236-258`, catalog probe `codex_catalog_runtime.go:16`·`:43-58`, readiness `spec_review_readiness.go:62-81`·`provider_readiness.go:102-105`, fallback `codex_profile_resolve.go:52-61`. 언어 `claude_markers.go:148-157`, `tui/locale.go:11-21`. version 문자열 `version.go:82-84`. provider cwd는 auto process cwd(`spec_review_loop.go:108-109`), timeout은 process group SIGKILL(`pkg/orchestra/command_unix.go:18-25`).
- `pkg/spec/types.go:193`(Revision 0 = 첫 review), `review_persist.go:39`, `internal/cli/spec_review_loop.go:53`. `pkg/orchestra/usage.go:39-63`은 `4f21f309`에서 agy를 `Supported=false`로 두고 #267이 `parseAgyUsage`·claude envelope을 더한다. #267의 결과 뒤 reap는 `provider_completion.go:11-17`, `provider_runner.go:160-166`, reasoning·thinking 부분집합은 `usage_parse.go:84-86`, `:103-106`이다. `.goreleaser.yaml:8-27`은 `-trimpath`, `-buildvcs=false`, `CGO_ENABLED=0`, `GOENV=off`, version ldflags다.
- 로컬 CLI(model 호출 없음): Claude Code 2.1.314, codex-cli 0.163.0, agy 1.5.0(T0, D1로 미사용), auto 0.50.133. `claude --help`에 `--disallowedTools`·`--tools`·`--strict-mcp-config`, `codex --help`에 `--search`(켤 때만 web search), `agy --help`에 `--output-format`·`--settings`가 있고 web tool flag는 없다.

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
| 실현성 blocker 선확인, 사전등록과 usage gate | T0, RFP-1/2, CD-1~7, REQ-01/02/19, T1, T10, S1 | covered |
| 외부 corpus, prompt, oracle, calibration | REQ-03/04/16/18/24, T2, T3, S2–S4, S19 | covered |
| 두 arm, 순서, config home, 환경 | REQ-05/06/07/17/22, T4, S5–S8 | covered |
| 격리, egress, trusted receipt, tool 정책, blind 채점, outcome 분류 | REQ-08/09/21/23/25, T5, T6, S8, S9, S18 | covered |
| 지표, 판정, 보고서, README, 편향 절 | REQ-10/11/12/13/14/20, T7–T9, S10–S15 | covered |
| 실제 pilot과 공개, 보존 | T11–T13, S17, REQ-15, T14, S16 | covered |

## Completion Debt

| Item | Blocks | Required resolution |
|------|--------|---------------------|
| CD-1: claude review·agy usage 관측 | 사전등록(REQ-HB2-19) | 닫힘(조건). T0에서 json 출력에 usage가 있다. 조건: `auto_source_commit`이 PR #267을 포함한다. agy는 D1로 빠졌다 |
| CD-2: 격리 home 인증 | 모든 trial | D1로 정리. claude는 Keychain 항목에서 `claudeAiOauth`만 꺼낸 파일로, codex는 `auth.json`으로 동작한다(T0). agy는 Keychain 전용이라 panel에서 뺐다. 갱신 뒤 수명과 강제 회전은 CD-7 |
| CD-3: review 검색 관측 | 외부 조회 통제(REQ-HB2-21) | 닫힘. codex 투영 argv의 `type:"web_search"` item. agy는 D1로 해당 없음 |
| CD-6: catalog probe와 readiness | 처치 동일성(REQ-HB2-10) | 닫힘(조건). `SSL_CERT_FILE=/etc/ssl/cert.pem`(REQ-HB2-22) 아래 0.80초, `gpt-6-astra` 유지, readiness 성공 |
| CD-4: subagent token | token 지표 | 닫힘. `result.modelUsage` |
| CD-5: 격리와 broker 동작 | REQ-HB2-08, 25 | D2로 정리. (b)(c)(d) PASS. codex 중첩 sandbox 불가는 D2와 RB-8로, 최소 추가 경로·env는 REQ-HB2-08·22에 넣었다 |
| CD-7: D1·D2와 #267 포함 pinned `auto`로 RFP-2 재실행 | 사전등록(T11)과 모든 run | 열림. RFP-2 oracle 전부 PASS(인증 수명, 기준선 위반 0, broker review quorum 2/2와 read tool 성공, DAC 잠금, same-sandbox signal, `modelUsage` model 집합). 실패하면 차단하고 사용자가 정한다. 새 secret·계정·승인 절차로 닫지 않는다 |

## Evolution Ideas

These are optional improvements and do not block sync completion.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| K≥3 반복과 corpus 확장 | 사용자가 6×1로 정했다 | 사용자 요청 |
| ablation arm, Codex·OMP 주 agent arm | Outcome Lock 밖이다 | 사용자 요청 |
| agy 파일 인증이 생기면 3명 panel 복원 | macOS agy 인증이 Keychain 전용이다(D1) | agy가 파일 token을 지원 |
| go/types 기반 unexported 판정, loopback 허용 grader profile | 과한 제외는 편향이 아니고, loopback 허용은 로컬 서비스를 노출한다 | 적격 task 부족(RFP-3) |

## Sibling SPEC Decision

| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | 결과 하나, module 하나, 15 task·약 35 소스 파일(test 포함)로 기준(25·40) 아래 | None |

## Reference Discipline

| Reference | Type | Verification |
|-----------|------|--------------|
| `scripts/benchmarks/harness/{run,workspace,grader,prepare_grader,golden,golden_protocol,process_tree,observe,report,test_golden}.py`, `grader.sb`, `README.md`, `surface_driver/main.go` | existing | Read, `grep -n`(행 번호는 `## 기존 코드 분석`) |
| `pkg/orchestra/usage.go`, `pkg/spec/{types,review_persist}.go`, `pkg/orchestra/command_unix.go`, `pkg/spec/provider_health.go`, `pkg/config/defaults.go`, `templates/claude/mcp.json.tmpl`, `internal/cli/spec_review_integrity.go`, PR #267 `0dfcce9c`의 `pkg/orchestra/{provider_completion,provider_runner,provider_envelope,usage_parse}.go`, `internal/cli/{init,spec_review_loop,orchestra_readonly_policy,codex_catalog_runtime,spec_review_readiness,provider_readiness}.go`, `internal/cli/tui/locale.go`, `pkg/adapter/claude/claude_markers.go`, `pkg/config/codex_profile_resolve.go`, `pkg/version/version.go`, `templates/claude/commands/auto-workflows.md.tmpl`, `content/agents/{planner,spec-writer}.md`, `autopus.yaml`, `.goreleaser.yaml`, `go.mod` | existing (read-only) | Read |
| `agy --help`, `auto init --help`, `claude --help`, `codex --help` 출력 | existing (local CLI) | 2026-11-04 실행 |
| Tier 1·2 repository와 license, 동결 model 집합의 공개 cutoff, Claude Code 내장 subagent model | external, unverified | T2·T11(GitHub API, vendor 문서), CD-7 |
| `feature*.py`, `feature_goapi/`, `feature_agent.sb`, `feature_provider.sb`, `feature_grader.sb`, `feature_prereg.json`, `FEATURE_RUNNER_FILES`, `test_feature_*.py`, 보고서 | [NEW] planned addition | 아직 없음 |

## Bias Ledger

| 편향 | 상태 | 이유 |
|---|---|---|
| corpus·harness 누출 | 제거 | Autopus와 무관한 외부 OSS, ADK 의존성 repo 제외, 모든 깊이의 harness 파일 대칭 제거 |
| model 학습(해답 노출) | 제거(vendor 진술 전제) | 동결 model 집합(ultra preset `fable`·`opus`, 내장 subagent, panel 포함) cutoff 최댓값 뒤 생성·commit, 참조된 모든 issue도 그 뒤. 집합 밖 model은 `model_substituted`. 진술 자체의 정확성은 RB-2 |
| 외부 조회 | 제거(CD-7 전제) | claude 거부 tool 대칭 적용, pid 귀속 egress, broker가 sandbox 밖에서 본 모든 codex model call(review 포함)의 `web_search` event 관측 시 claim 차단 |
| prompt 선택·전달 | 제거 | 동결 template과 strip 규칙, 같은 `TASK.md`, flag 없는 argv |
| 순서·시간 | 기대값만 제거 | seeded 균형 순서, interleave, 같은 machine, CLI snapshot. 실제 불균형은 RB-1 |
| 설정·환경·credential 누출 | 제거 | 격리 config home(broker home은 agent가 못 읽음, master 되쓰기 없음), `/` 기준 읽기 거부와 session canary preflight(`--output` 위치와 무관), grader도 `/` 기준 거부(`feature_grader.sb`), agent workspace에서 git 미실행, 빈 MCP, `env -i` allowlist, default-deny 읽기, download 전용 module cache, 두 arm 모두 영어 출력(한국어는 token을 더 써 token 비교를 기울인다) |
| oracle 구성·채점 | 제거 | 단일 구성 규칙, `(package, test)`, blind 경로, 통제 위반은 fail이 아니라 claim 차단(비대칭 없음) |
| token 관측 불가 | 제거 또는 차단 | `null` 금지, probe gate, CD-1, CD-4 |
| 처치 confound(추가 provider·agent·review, codex catalog probe·readiness 명령) | 측정 대상 | REQ-HB2-20, 제품 설정 그대로. model 대체는 통제 위반(CD-6) |
| RB-1: provider 서버 측 model revision, 계정 공유 prompt cache, 약 36시간 부하 변화 | 잔여 | 고객 측에서 pin하거나 끌 수 없다. n=6에서는 실제 불균형이 남을 수 있다 |
| RB-2: 공개 cutoff는 vendor 진술이고 출시 뒤 갱신될 수 있다 | 잔여 | 검증할 수 없다. 미공개 cutoff는 출시일로 대신해 보수적으로 줄였다 |
| RB-3: `auto` shim 메시지(review 횟수·시간)는 sandbox 안에서 나와 위조할 수 있다 | 잔여(부차) | R1이 읽는 receipt는 모두 sandbox 밖에서 만든다(REQ-HB2-25). 이 메시지는 부차 지표에만 쓴다 |
| RB-4: filter와 loopback 거부가 공개 API test가 있는 작은 PR, listener 없는 package 쪽으로 기운다 | 외적 타당성 | 두 arm에 같아 arm 편향은 아니다. HTTP 중심 repo가 빠지는 것을 보고서 한계 절에 쓴다 |
| RB-5: overlay가 언어를 en으로 둔다(ADK repo 설정은 ko). 출력 형식은 #267의 제품 값 그대로다. native에는 언어 지시가 없다 | 처치 정의·잔여 | native가 영어로 답한다는 것은 영어 prompt에 기댄 가정이다 |
| RB-6: 재시작 금지는 절차로만 강제된다 | 잔여 | 다른 출력 경로의 숨은 session은 runner가 알 수 없다. `prior_sessions` 공개 규칙으로 줄인다 |
| RB-7: test-only 자산이 `testdata` 밖에 있으면 production으로 분류된다 | 잔여(대칭) | 두 arm 모두 floor 쪽으로 같게 작용한다. calibration은 통과할 수 있다 |
| RB-8(D2): review codex의 shell 도구 경계는 codex 자체 `--sandbox read-only`이고 이 모드는 사용자가 읽을 수 있는 파일을 모두 읽는다 | 잔여(harness 쪽) | 바깥 통제는 pid 귀속 egress, agent 밖 receipt·broker home, trial 동안 다른 저장소 DAC 잠금, `command_execution` 경로 검사, `host_solution_copy` preflight다. 난독화한 명령이나 알려지지 않은 host 사본 읽기는 남고, 이 경로는 harness arm에만 있어 harness 쪽으로 기울 수 있다 |
| RB-9(D1): review panel이 claude+codex다(제품 기본은 gemini 포함, quorum 2/2) | 선언된 이탈 | 측정 대상은 제품 기본값과 다르다. claim 문구와 보고서 처치 절에 적는다 |
| RB-10: project MCP(context7, sequential-thinking)가 두 arm 모두 꺼진다 | 선언된 이탈 | egress allowlist 아래 `npx`로 뜰 수 없다. claim 문구에 적는다 |

## Reviewer Brief

- **Intended scope**: feature mode runner, 외부 corpus 선택, oracle 구성, 사전등록, 격리, blind 채점, 보고서, README 가드, 1회 pilot 공개.
- **Explicit non-goals**: 유의성, 추가 arm, 제품 코드 변경, 외부 코드 commit, Python adapter, 새 secret·승인 절차.
- **Self-verified**: Traceability(REQ 25개), INV ↔ acceptance, 정확한 값(S4, S7 hex 재계산, S10, S13, S14, S19), 인용 행 번호, existing/[NEW]/unverified 구분, probe 3행.
- **Reviewer should focus on**: Bias Ledger의 `제거`가 통제로 뒷받침되는가, D1·D2 잔여(RB-8~10)의 정직성, CD-7 oracle의 충분성, usage 완결·합계 계약, outcome taxonomy와 R1의 정합, Completion Debt only.

## Self-Verify Summary

- Q-CORR-01 | status: PASS | attempt: 7 | files: research.md, spec.md, plan.md | reason: rev 7 인용 행(`grader.sb:22-28`·`:42-51`·`:68-71`, `grader.py:120-128`, `workspace.py:121-128`, `provider_health.go:236-266`, `defaults.go:136-144`, #267 `0dfcce9c` 파일)을 다시 읽어 확인했다
- Q-CORR-02 | status: PASS | attempt: 3 | files: spec.md, plan.md, research.md | reason: [NEW]와 external unverified를 분리했다
- Q-CORR-03 | status: PASS | attempt: 3 | files: spec.md, acceptance.md | reason: EARS 문장과 bare Given/When/Then
- Q-CORR-04 | status: PASS | attempt: 3 | files: research.md | reason: Reference Discipline 3분류
- Q-COMP-01 | status: PASS | attempt: 3 | files: all | reason: 네 문서와 prd.md를 rev 7로 맞췄다
- Q-COMP-02 | status: PASS | attempt: 3 | files: spec.md, plan.md, acceptance.md | reason: REQ 25개가 task와 scenario에 연결된다
- Q-COMP-04 | status: PASS | attempt: 3 | files: research.md, plan.md | reason: OPS와 공개 포함, CD-7만 열린 blocker
- Q-COMP-05 | status: PASS | attempt: 3 | files: research.md, acceptance.md | reason: INV 18개가 정확한 값 scenario에 연결된다
- Q-COMP-06 | status: PASS | attempt: 3 | files: spec.md, research.md | reason: Traceability Matrix(REQ 25개)와 Reviewer Brief를 rev 7로 갱신했다
- Q-COMP-07 | status: PASS | attempt: 3 | files: research.md | reason: CD마다 해소 경로와 차단 조건, Evolution Ideas에 ID 없음
- Q-COMP-08 | status: PASS | attempt: 3 | files: plan.md | reason: RFP 3행, RFP-1 PASS(evidence 있음), RFP-2는 T0 FAIL 뒤 재실행 not-run, RFP-3 not-run
- Q-FEAS-01 | status: PASS | attempt: 3 | files: spec.md, plan.md | reason: runner 계층만 바꾸고 grader·prepare_grader·report는 그대로
- Q-STYLE-01 | status: PASS | attempt: 3 | files: spec.md, acceptance.md | reason: Priority·Type 분리, bare step keyword
- Q-SEC-01 | status: PASS | attempt: 3 | files: spec.md | reason: 외부 코드 build·실행은 sandbox 안에서만, prompt는 argv 밖
- Q-SEC-02 | status: PASS | attempt: 5 | files: spec.md, acceptance.md | reason: trusted receipt(fail closed), pid 귀속 egress, 되쓰기 없는 credential, redaction, canary, grader `/` 기준 거부, agent workspace git 미실행, D2 잔여 명시
