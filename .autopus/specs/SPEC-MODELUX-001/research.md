# SPEC-MODELUX-001 리서치

## 기존 코드 분석
- 등급·해석: `pkg/config/quality_tier.go`(`canonicalAgentNames` L17-21, `qualityTiers` L53, `AgentTier` L76-91), `quality_provider.go`(`EffectiveMode` L56-66, `ForProvider` L79-84), `native_balanced.go`(`NativeBalancedAgentCandidate` L9-31, `historicalBalancedDefaults` L52-67). ultra 기본값은 `defaults.go:132-153`(fable 7, opus 9), balanced는 `role_model_policy_balanced.go:21-40`의 rung(fable 7, sonnet 9)이다.
- Claude projection(`pkg/adapter/claude/claude_agent_content.go:114-148`): 표준 배치면 정확한 ID와 thinking, 아니면 tier alias와 effort(fable max, opus는 ultra면 max·아니면 high, sonnet은 ultra면 high·아니면 medium, haiku 없음)다. fallback 원천은 `content/agents/*.md`의 `model:`이고, Codex 템플릿 literal도 같은 값이다(`pkg/content/agent_transformer.go:107`).
- Codex: 요청 값은 `codex_profile.go:105-124`(ultra는 opus 미만을 opus로 올린다), supervisor는 L78-83이다. 생성은 `applyPreparedFiles`가 catalog를 probe하고(`codex.go:108-111`, `codex_catalog.go:20-31`), render context가 `ResolveCodexProfile`(`codex_profile_resolve.go:38-110`)로 Effective 값을 쓴다. probe가 실패하면 표준 배치가 아닌 칸은 `gpt-5.6-sol`, 표준 배치 칸은 요청 값(미확인)이다. 표준 배치에서 대체가 필요하면 생성을 거부한다(`codex_native_placement.go:29-47`). 고정 catalog seam은 `codex.WithModelCatalog`, `platformAdapterOptions`(`platform_catalog.go:15-18,51-56`), drift snapshot(`doctor_drift_content.go:60-80`)이다.
- OMP: profile이 비면 모델을 묶지 않는다(`omp_model_integration.go:38-40`). 있으면 `omp`로 catalog를 읽고(`omp_model_integration_probe.go:26`), degrade(`DegradeUnservedDerivedOMPRoutes`)와 config 되읽기(`omp_model_integration_activation.go:56`)를 거친다. 접힘은 `omp_native_agents.go:12-47`이다.
- CLI: `quality.go:136-190`(278줄), 도구별 편집기 `quality_tool_interactive.go:19-77`, `""` fallback 표 `quality_tool_targets.go:82-94`(C5), 공유 reader `quality_omp_interactive.go:25-27`, writer `quality_config.go:24-75`, `quality_tool_persist.go:46-83`, `quality_config_verify.go:20-75`, `platform_omp_config.go:87`이 있다. loader는 decode 전에 `${NAME}`만 펼친다(`loader.go:151-161`).
- update(`update.go`, 291줄): `--auto`→yes(L37-39), `--check`는 `--self` 분기에서만 쓰인다(L44-50). 설정 프롬프트·디자인 파일은 L191-208, supervisor migration `config.Save`는 L211-219, 생성 루프는 L226이다. workspace는 `update_workspace.go:28-30,277-300`이다. `promptChoice`는 `os.Stdin`을 직접 읽는다(`prompts.go:38`).
- init(`init.go`, 253줄): huh wizard L99-131, `saveConfigWithGenerationRollback` L186-190, 생성 `init_helpers.go:30`. schema·loader는 `schema.go:24-35,235-239`, strict decode는 `loader.go:75`·`loader_strict.go:105`다.

## PRD 보정
PRD의 C1–C7은 그대로 따른다. 아래는 SPEC 작성(C8–C16)과 review 반영(C17–C20)에서 확인한 보정이다.

| # | PRD 내용 | 확인한 사실 | 근거 | 반영 |
|---|---|---|---|---|
| C8 | binding "없음" 행은 `quality.default`만 쓴다 | `Validate`는 `quality.default`가 presets에 있어야 통과한다 | `schema.go:235-239` | absent면 프리셋도 쓴다 |
| C9 | 기존 section writer가 quality 밖 byte를 지킨다 | 문서 전체를 다시 encode한다 | probe A1 | 블록 splice |
| C10 | workspace는 리포마다 묻는다(A6) | workspace는 `--yes`나 `--auto`를 요구하고 하위에 `--yes`나 `--plan`을 넘긴다 | `update_workspace.go:28-30,277-300` | 묻지 않는다 |
| C11 | 구버전이 `quality.providers`를 strict decode로 거부한다 | strict decode는 v0.50.113, providers는 v0.50.88부터다. 그 이전 바이너리는 조용히 무시한다 | `git tag --contains` | 결론은 같고 이유를 고친다 |
| C12 | balanced historical은 2종 | 출하 balanced는 4종, ultra는 2종 | git 이력 | registry 6행, runtime standard는 HB2·HB5 |
| C13 | fallback drift test가 필요하다 | Codex literal은 content에서 생성된다 | `agent_transformer.go:107` | content를 직접 읽는다 |
| C14 | (없음) | `promptChoice`는 `os.Stdin`을 직접 읽고, 기존 update test 다수는 `--yes`·`SetIn`이 없다 | `prompts.go:38`, grep | 주입 probe, 공유 reader |
| C15 | `init.go`만 300줄 주의 | update.go 291, quality.go 278, init.go 253 | `wc -l` | 새 파일, 호출 3줄 이하 |
| C16 | Claude alias는 slug로 표시 | 생성 파일은 alias와 정확한 ID를 구별한다 | `claude_agent_content.go:114-120` | raw 값 표시 |
| C17 | Codex 셀은 catalog probe 전 요청 값(A2) | 생성은 catalog로 대체·거부하고, probe가 실패하면 `gpt-5.6-sol`을 쓴다 | `codex_catalog.go:20-31`, `codex_profile_resolve.go:52-62` | run 단위 snapshot과 같은 해석 함수 |
| C18 | `--check`는 구성 단계 전에 끝난다 | `--self` 없는 `--check`는 프로젝트 update를 계속한다 | `update.go:44-50` | 비대화형으로 다룬다 |
| C19 | OMP 칸은 첫 후보(실제 모델) | 첫 후보 뒤에 catalog degrade와 config 되읽기가 있다 | `omp_model_integration*.go` | "요청 route"로 범위를 명시 |
| C20 | (없음) | `saveQualitySection`의 유일한 호출처인 도구별 편집기도 펼쳐진 `cfg.Quality`를 다시 쓴다 | `quality_tool_persist.go:46-83` | 새 writer에 위임 |

## Plan Intent Ledger
출처는 direct `auto plan`의 Plan Intent Ledger다(planning context, 2026-10-10). 셀은 근거로만 요약했다.

| Field | Status | SPEC 반영 |
|---|---|---|
| goal | answered | Outcome Lock, REQ-MUX-13, REQ-MUX-15, REQ-MUX-16 |
| selection unit (그룹 4개, 고급) | answered | REQ-MUX-01, REQ-MUX-03, REQ-MUX-04 |
| stored value (상대 등급), storage (`quality.presets` + binding, `custom`) | answered | REQ-MUX-07, REQ-MUX-09. binding은 `quality.default`만 쓴다(C11) |
| update behavior | answered | REQ-MUX-13, REQ-MUX-14 |
| table (5개 플랫폼, `플랫폼 기본값`) | answered | REQ-MUX-05. OMP는 요청 route로 범위를 명시했다(C19) |
| release v0.50.125, constraints | answered | header, REQ-MUX-23. 새 secret·승인·유료 API CI 없음, 생성 루트 수작업 금지 |
| scope_boundary | assumed | non-goals. 틀리면 OMP 쓰기를 별도 config 경계로 설계해야 한다 |
| done_evidence | assumed | Must S1–S14, S19–S21. 틀리면 완료 증거 목록이 바뀐다 |
| brownfield_impact | assumed → 코드로 검증 | 기존 코드 분석, PRD 보정 |

## Question Audit
- question_transport: AskUserQuestion · question_count: 4 (선택 단위, 저장 값, update 동작, 출하 순서) · unresolved_fields: none

## Outcome Lock
- User-visible outcome: 대화형 `auto init`, `auto update`, `auto quality`가 에이전트×플랫폼 표를 보여 준다. Claude Code·Codex 칸은 이번 run의 생성기가 같은 catalog snapshot으로 쓸 실제 값이고, OMP는 요청 route, OpenCode·Antigravity는 `플랫폼 기본값`이다. 사용자는 그룹 4개나 '고급'으로 바꾸고, `*`가 붙은 적용 후 표를 확인한 뒤 `y`일 때만 저장한다. 같은 run의 생성 결과는 적용 후 표와 같다. update는 매번 묻고 Enter 한 번이면 아무것도 쓰지 않는다. 비대화형은 묻지도 바꾸지도 않는다. 예전 기본 프리셋은 확인 뒤에만 갱신되고, 사용자가 고친 프리셋의 값은 바뀌지 않는다. 환경 변수 값은 파일에 써지지 않는다.
- Mandatory requirements: REQ-MUX-01 ~ REQ-MUX-19.
- Explicit non-goals: `spec.md` `## Outcome Boundary`의 목록과 같다.
- Completion evidence: Must S1–S14, S19–S21 PASS, `auto eval harness run`, coverage 85% 이상, 300줄 이하, `evidence/manual-smoke.txt`, `auto spec validate --strict` 통과.

## Visual Planning Brief
명령 흐름도, update 질문, override 표, 미리보기는 `plan.md`에 있다. 아래 화면은 `wireframe intent: assumed`다. 사용자 confirm 전이므로 risk와 reviewer focus로 둔다.

```text
역할 그룹마다 등급을 고르세요 (Enter = 그대로)
설계·판단  planner, architect, spec-writer, security-auditor              현재: fable
  1) fable  2) opus  3) sonnet  4) haiku  [Enter] 그대로   >
구현       executor, debugger, deep-worker, devops, frontend-specialist, perf-engineer   현재: 혼합 (fable 2 · sonnet 4)
  1) fable  2) opus  3) sonnet  4) haiku  [Enter] 혼합 그대로   > 3
```

```text
고급: 에이전트별 등급 (agent=tier, 빈 줄로 끝)
> debugger=opus
  debugger = opus      구현: 혼합 (opus 1 · sonnet 5)
> tester=gpt
  unknown tier "gpt" (choose one of: fable, opus, sonnet, haiku)
```

## 설계 결정
| 결정 | 선택 | 기각한 대안 | 근거와 trade-off |
|---|---|---|---|
| 쓰기 원천 (F-001) | env 확장 없는 raw quality decode. raw와 loader 값이 다르거나 alias·merge가 있으면 거부 | `${` 문자열 검사, 펼쳐진 `cfg.Quality` 재직렬화, alias를 따라가 검사 | 원천을 바꾸면 직접 placeholder, 바깥 anchor 참조, merge가 한 규칙으로 막힌다. 대신 참조를 쓰는 드문 파일은 손으로 고쳐야 한다 |
| preset 쓰기 | 기존 encoder 출력에서 quality 블록만 원본에 splice | 기존 `saveQualitySection`, `config.Save` | 블록 밖 서식을 지킨다(A1). 블록 안 서식과 주석은 잃고, 값만 보존한다 |
| Codex 칸 | 생성기 해석 함수와 run 단위 snapshot 공유 | 요청 값만 표시, temp dir 생성으로 표 계산 | 표 = 생성 결과가 구조상 성립한다. probe 1회(최대 5초)가 늘지만, update와 `--apply`에서는 생성과 공유하므로 늘지 않는다 |
| OMP 칸 | 요청 route와 명시 머리말 | `omp` probe·되읽기 재현 | OMP 생성은 subprocess와 config 되읽기가 필요하고, custom은 OMP를 바꾸지 않는다. 실제 route는 생성 때 기존 안내가 알린다 |
| registry | 한 표에 runtime standard 표시 | 목록 하나에 다 넣기 | 판별 범위는 넓히고 런타임은 그대로 둔다 |
| 질문 키 | 글자 키와 기존 대상 이름 | 숫자 | 기존 scripted test와 습관을 그대로 둔다 |
| Claude 칸 | frontmatter raw 값 | slug로 통일 | 생성 diff와 칸 diff가 일치한다(C16) |
| fallback | embedded content agent source | Go 표 + drift test | 생성기와 같은 원천이다 |
| TTY·init seam | 주입 probe와 `runInitWizard` 변수, 실제 터미널은 수동 smoke | PTY 라이브러리 | 새 의존성이 없다. huh 경계는 S21이 사람 눈으로 닫는다 |
| 보호 범위 | 이 기능의 쓰기와 그 프롬프트에서 닿는 편집기, 값 단위 | "모든 흐름"의 byte 불변 | block 재encode와 init 재실행이 byte 불변과 충돌한다(R03, R09) |

## Technology Stack Decision
| Mode | Selected stack | Resolved versions | Source refs | Checked at | Rejected alternatives |
|---|---|---|---|---|---|
| brownfield | 기존 Go 모듈, `bufio`, `text/tabwriter`, yaml.v3 Node, 기존 quality writer와 Codex catalog seam | `go.mod`(`go 1.26`, toolchain go1.26.9), 새 의존성 없음 | `go.mod`, `platform_catalog.go` | 2026-10-10 | huh 폼 편집기, PTY 라이브러리, 새 YAML 키 |

## Minimality Decision Matrix
| Ladder step | Evidence | Decision | Receipt item |
|-------------|----------|----------|--------------|
| actual need | 2모드뿐이고, 에이전트별 조정은 도구별 편집기에만 있다. GAP 1, 표와 생성의 catalog 차이, F-001 | proceed | 그룹, 표, update 단계, GAP 1, 안전한 writer |
| existing code/helper/pattern | `parseQualityToolTierAssignment`, `withQualityToolPreset`, `saveQualityDefault`, `replaceAutopusConfigSection`, `verifyQualityLineEdit`, `validateQualityYAML`, `atomicWriteQualityConfig`, `chooseQualityTarget` reader, `captureDriftGenerationSnapshot`, `driftAdapterFor`, `codex.WithModelCatalog`, `OMPNativeAgentRoute`, `LoadAgentSourcesFromFS` | reuse | 검증, 사본, writer 검증, snapshot 공유, 기존 편집기 |
| stdlib/native | `bufio`, `text/tabwriter`, `strings`, `maps.Equal`, `reflect.DeepEqual`, `os.Getenv` | use | driver, 표, raw 비교, CI probe |
| existing dependency | yaml.v3 Node(Line, Kind, Alias), cobra, huh(init 기존 단계), testify | reuse | 블록 경계, alias 검출, quality option |
| new dependency or new abstraction | 새 의존성 0. 새 abstraction은 registry 표, `EffectiveAgentTier`, Claude projection export, `codex.ResolveAgentProfile`, `saveQualityBlock`, 구성 core, snapshot 전달 함수 3개 | accepted | 각각 REQ-MUX-10, 02, 05, 08, 17, 19의 실제 요구다. A1과 C17이 기존 경로로는 부족함을 보였다 |
| minimum sufficient verification | `go test ./pkg/config/... ./pkg/adapter/... ./pkg/content/... ./internal/cli/...`, 실제 `Generate` oracle, catalog fixture 4종, scripted 행렬, `auto eval harness run`, coverage 85% 이상, 수동 smoke 1회 | required checks | security(F-001), data-loss(atomic, 블록 밖 byte), validation(키 제한, legacy decode), deterministic-oracle(golden) gate를 줄이지 않는다 |

## Semantic Invariant Inventory
| ID | source clause | invariant type | affected outputs | acceptance IDs |
|----|---------------|----------------|------------------|----------------|
| INV-MUX-01 | "Enter 한 번이면 그대로" | keep path: 파일 전체 zero-byte | autopus.yaml SHA-256 | S2, S6, S10, S11, S20 |
| INV-MUX-02 | "적용 전 실제 모델 표" | Claude·Codex 칸 = 같은 run, 같은 snapshot의 생성 값. OMP 칸 = 요청 route | 표 raw 값, 실제 `Generate` 결과 | S4, S5, S12, S14, S19, S20 |
| INV-MUX-03 | "사용자 의도를 조용히 바꾸지 않는다" | customized 값(agents, description) 불변. 이 기능의 쓰기와 그 프롬프트의 편집기 기준 | decode한 프리셋 값 | S7, S8, S9 |
| INV-MUX-04 | "--yes, CI, non-TTY: never ask" | 비대화형 no-read | Read 횟수, 안내 줄 수 | S10 |
| INV-MUX-05 | "Groups exist only in the UI" | grouping not persisted | quality key 경로 | S1 |
| INV-MUX-06 | "혼합 표시, 그룹 등급 선택" | grouping and ordering | 그룹 표시 문자열 | S2, S3 |
| INV-MUX-07 | "기존 스키마에 저장" | binding decision | 쓰는 key, writer 종류 | S6, S7 |
| INV-MUX-08 | "quality 밖 byte 불변" | block 쓰기의 블록 밖 byte 보존 | quality 블록 밖 줄 | S9, S12, S13 |
| INV-MUX-09 | "런타임 해석 의미를 바꾸지 않는다" | runtime compatibility | `NativeBalancedAgentCandidate`, 기존 test | S8, S10, S12 |
| INV-MUX-10 | "오래 기록된 프리셋 안전 갱신" | exact-match classification | 판정 결과 | S8, S9 |
| INV-MUX-11 | "older binaries must still load the file" | schema compatibility | 쓰는 key, legacy strict decode | S1, S13, S15 |
| INV-MUX-12 | review F-001 "펼쳐진 값이 기록된다" | no env-expanded value written | 파일 bytes, raw decode 비교 | S13 |

## Feature Coverage Map
| Outcome slice | Covered by | Status |
|---------------|------------|--------|
| happy path: 그룹·고급 편집, 미리보기, 확인, 저장 | REQ-MUX-01~07 / S1-S3, S6 | covered |
| 표와 생성 결과(catalog 대체·없음·거부, override, OMP 요청 route, 오래된 디스크 파일) | REQ-MUX-05, 18, 19 / S4, S5, S14, S19, S20 | covered |
| GAP 1 판별·갱신·보호(도구별 편집기 guard 포함) | REQ-MUX-10~12 / S8, S9 | covered |
| 진입점 init, update, quality를 실제 명령으로 | REQ-MUX-13, 15, 16 / S10-S12, S21 | covered |
| security: 환경 변수 값 비노출, 참조 거부 | REQ-MUX-08 / S13 | covered |
| error/recovery: EOF, 3회 오입력, 취소, 쓰기 거부·실패 시 원본 유지 | REQ-MUX-06, 08, 13 / S6, S10, S13 | covered |
| 구버전 호환과 비대화형(non-TTY, `--yes`, `--auto`, `--check`, CI, preview, workspace) | REQ-MUX-09, 10, 14 / S8, S10, S13 | covered |
| UX와 docs: override 순서, 읽기 전용 표, locale, help·README·CHANGELOG | REQ-MUX-20~23 / S15-S18 | covered (Should) |

## Completion Debt
| Item | Blocks | Required resolution |
|------|--------|---------------------|
| None | - | - |

작성 시점에 알려진 debt는 없다. Must REQ, snapshot 공유, raw writer, registry ultra 행, update 계약 test, 실제 명령 종단 test, 수동 smoke 기록 중 하나라도 빠지면 debt가 되어 sync 완료를 막는다. 이 저장소 `autopus.yaml`의 실제 갱신은 사용자 선택이라 debt가 아니다.

## Evolution Ideas

These are optional improvements and do not block sync completion.

| Idea | Why not required now | Promotion trigger |
|------|----------------------|-------------------|
| OMP `role_model_policy` 통합, OMP 칸의 catalog 반영 값 | scope_boundary 밖이다. 13개 역할이 `task`로 접히고 probe에 `omp` 실행이 필요하다 | OMP도 같은 구성을 따르거나 실제 route를 보기를 요청할 때 |
| 에이전트별 provider 모델 ID 고정 | 상대 등급이 저장 단위라는 결정이 있다 | 특정 모델 고정 요청이 반복될 때 |
| custom의 base mode(ultra effort 사다리 상속), provider별 그룹 구성 | 새 YAML 키가 필요하다 | effort 하락이나 provider별 구성이 실제 요청이 될 때 |
| 비대화형 `auto quality refresh-defaults` | 비대화형은 바꾸지 않는다는 결정이 있다 | CI만 쓰는 사용자의 갱신 요청 |
| HB1·HB4 런타임 표준 해석, historical 런타임 해석 제거 | 지금 바꾸면 실제 모델이 바뀐다 | 갱신 채택이 충분해졌을 때 |
| 디스크 생성 파일의 drift 표시, `show --agents` JSON, 그룹별 비용 힌트 | Outcome Lock에 필요 없다 | 지원 문의나 자동화 요청 |

## Sibling SPEC Decision
| Decision | Reason | Sibling SPEC IDs |
|----------|--------|------------------|
| none | Primary SPEC 하나가 Outcome Lock을 닫는다. GAP 1 갱신과 writer는 같은 구성 단계와 미리보기를 공유한다. OMP 통합은 Evolution Idea다. task 15개, 파일 약 28개로 sibling 기준에 못 미친다 | None |

## Reference Discipline
| Reference | Type | Verification |
|-----------|------|--------------|
| `pkg/config/` `quality_tier.go`, `quality_provider.go`, `native_balanced.go`, `defaults.go`, `role_model_policy_balanced.go`, `codex_profile.go`, `codex_profile_resolve.go`, `schema.go`, `loader.go`, `loader_strict.go`, `quality_policy.go`, `omp_native_agents.go`, `role_model_policy*.go` | existing | Read, grep (줄 번호는 이 문서와 spec.md에 인용) |
| `pkg/adapter/claude/claude_agent_content.go`, `pkg/adapter/codex/codex.go`, `codex_catalog.go`, `codex_native_placement.go`, `pkg/adapter/omp/omp_model_integration*.go`, `pkg/content/agent_transformer*.go`, `content/embed.go:11` | existing | Read, grep |
| `internal/cli/` `quality*.go`, `platform_omp_config.go:87`, `platform_catalog.go`, `doctor_drift_content.go`, `codex_catalog_runtime.go`, `prompts.go`, `update.go`, `update_workspace.go`, `update_freshness.go:227`, `init.go`, `init_helpers.go:30`, `platform_lifecycle.go:128-150`, `tui/wizard_steps.go`, `tui/locale.go` | existing | Read, `wc -l`, probe A1 |
| registry·호환 근거 commit c95806e2, c21f7316, 0cb6296b, aaeae52d, 001830fd, fbe502c0, a0355acf, 726fa47a, e02b5ab4 | existing (git) | `git show`, `git tag --contains` |
| `ClassifyBuiltinQualityPreset`, `EffectiveAgentTier`, `ClaudeAgentProfile`, `ClaudeAgentEffort`, `codex.ResolveAgentProfile`, `codex.ResolveSupervisorProfile`, `saveQualityBlock`, `composeCodexCatalogProbe`, `updateHarnessPlatformWith`, `generatePlatformFilesWith`, `applyQualityHarnessWith`, `runInitWizard`, `tui.QualityRoleGroups`, `quality_compose_*.go`, `update_compose.go`, `init_compose.go`, `codex_resolve.go`, `show --agents`, `evidence/manual-smoke.txt` | [NEW] planned addition | 부재 확인(grep) |

## Reviewer Brief
- Intended scope: 세 진입점의 공유 구성 단계(그룹, 고급, 생성기 해석과 같은 표, 확인 뒤 저장), GAP 1 판별·갱신·보호, 환경 변수 값을 쓰지 않는 블록 writer, 비대화형 계약, run 단위 Codex catalog snapshot.
- Explicit non-goals: OMP 통합과 OMP 칸의 catalog 반영, 모델 ID 고정, 새 YAML 키, 기본값·런타임 해석 변경, 기존 편집기 제거, 비대화형 갱신, 기존 migration 저장과 init 재실행 정책, 블록 안 서식 보존.
- Self-verified: rev 0 review finding 9건 처리(`spec.md` `## Review Resolution (rev 1)`), Traceability Matrix, invariant 12개와 구체 값, A3 registry, PRD 보정 C8–C20.
- Reviewer should focus on: raw writer의 F-001 차단 완전성, Codex snapshot 공유가 생성과 같은 값을 내는지, 블록 경계, update 위치와 비대화형 판정, 보호 범위. 그리고 Completion Debt only.

## Self-Verify Summary
- Q-CORR-01 | status: PASS | attempt: 2 | files: spec.md, plan.md, research.md | reason: `--check`를 코드대로 고쳤다(`--self`만 먼저 끝난다). 인용한 경로와 줄을 다시 확인했다
- Q-CORR-02 | status: PASS | attempt: 2 | files: spec.md, plan.md, research.md | reason: 새 함수, seam, 파일, evidence를 모두 [NEW]로 표시했다
- Q-CORR-03 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: EARS 문장은 parser 형식이고, 시나리오는 bare Given/When/Then/And다
- Q-CORR-04 | status: PASS | attempt: 2 | files: research.md | reason: Reference Discipline이 existing, git 근거, [NEW]를 나누고 생성 루트를 원천과 구별한다
- Q-COMP-01 | status: PASS | attempt: 1 | files: spec.md, plan.md, acceptance.md, research.md | reason: 네 문서가 계약, 계획, oracle, 근거를 맡는다
- Q-COMP-02 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: init과 `--apply`를 실제 명령과 실제 adapter로 도는 S11, S12와 수동 smoke S21로 바꿨다
- Q-COMP-03 | status: PASS | attempt: 1 | files: spec.md | reason: 조건, 기대 결과, 관측 지점을 적었다
- Q-COMP-04 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: 칸의 뜻을 플랫폼별로 정하고, 표 = 생성 결과를 snapshot 공유로 닫았다. 보호는 값 단위와 명시 범위로 고쳤다
- Q-COMP-05 | status: PASS | attempt: 2 | files: research.md, acceptance.md | reason: INV-MUX-02, 03, 08을 다시 정의하고 INV-MUX-12를 더했다. oracle은 실제 `Generate`와 catalog fixture 4종, 오래된 디스크 파일이다
- Q-COMP-06 | status: PASS | attempt: 2 | files: spec.md, research.md | reason: Traceability Matrix와 Reviewer Brief를 rev 1 범위로 고쳤다
- Q-COMP-07 | status: PASS | attempt: 1 | files: research.md | reason: Completion Debt와 Evolution Ideas가 분리되고 Evolution Ideas에 ID가 없다
- Q-COMP-08 | status: PASS | attempt: 2 | files: plan.md | reason: A1은 실행 증거가 있는 PASS다. A2와 A3는 snapshot 공유와 catalog fixture를 포함해 not-run 이유를 적었다
- Q-FEAS-01 | status: PASS | attempt: 2 | files: spec.md, plan.md | reason: 표는 생성기의 해석 함수와 같은 snapshot을 쓰고, 생성도 기존 고정 catalog seam으로 그 snapshot을 쓴다
- Q-FEAS-02 | status: PASS | attempt: 1 | files: spec.md, plan.md | reason: 변경은 `pkg/config`, `pkg/adapter/claude`, `pkg/adapter/codex`, `internal/cli`, 원천 문서다
- Q-FEAS-03 | status: PASS | attempt: 2 | files: plan.md, acceptance.md | reason: 종단 증거가 stub이 아닌 실제 adapter `Generate`와 수동 smoke로 바뀌었다
- Q-STYLE-01 | status: PASS | attempt: 1 | files: spec.md | reason: REQ 문장에 모호어가 없다
- Q-STYLE-02 | status: PASS | attempt: 1 | files: spec.md, acceptance.md | reason: Priority는 Must/Should만 쓰고 EARS type과 분리했다
- Q-STYLE-03 | status: PASS | attempt: 1 | files: acceptance.md | reason: step은 bare keyword로 시작하는 완결된 문장이다
- Q-SEC-01 | status: PASS | attempt: 2 | files: spec.md | reason: Trust Model이 clone한 autopus.yaml과 loader의 env 확장 경계를 정의한다. writer는 펼친 값을 원천으로 쓰지 않는다
- Q-SEC-02 | status: PASS | attempt: 2 | files: spec.md, acceptance.md | reason: raw decode 비교와 alias·merge 거부가 직접·간접 placeholder를 모두 막는다. F-001 입력 그대로의 S13으로 확인한다
- Q-SEC-03 | status: PASS | attempt: 1 | files: spec.md | reason: 새 로그나 artifact가 없다. 수동 smoke 기록에는 secret이 없는 temp project만 쓴다
- Q-COH-01 | status: PASS | attempt: 1 | files: spec.md | reason: 에이전트별 모델 구성이라는 한 흐름으로 모인다
- Q-COH-02 | status: PASS | attempt: 1 | files: research.md | reason: Outcome Lock에 필요한 작업은 모두 Primary SPEC에 있다
- Q-COH-03 | status: PASS | attempt: 1 | files: research.md | reason: sibling이 없고 근거를 적었다
