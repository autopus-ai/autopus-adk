# SPEC-MODELUX-001: 에이전트별 모델 구성 UX — 역할 그룹, 플랫폼별 실제 모델 표, 오래된 프리셋 안전 갱신

**Status**: approved
**Created**: 2026-10-10
**Revision**: 1 (spec review REJECT 1회 반영, `## Review Resolution (rev 1)`)
**Domain**: MODELUX
**Module**: autopus-adk
**Release**: v0.50.125 (`release-prep.sh --apply` 전에 merge)
**Source PRD**: `prd.md` (같은 디렉터리). PRD의 사실 보정 C1–C7을 따른다. 이 SPEC을 쓰면서 코드, git 이력, probe로 확인한 보정 C8–C20은 `research.md` `## PRD 보정`에 있다. 둘이 다르면 이 SPEC이 우선한다.

## 목적

지금 사용자가 고를 수 있는 것은 전역 모드 balanced/ultra 두 개다. 에이전트별 조정은 `auto quality` 안의 도구별 16행 `agent=tier` 편집기(`internal/cli/quality_tool_interactive.go`)에서만 되고, init·update에서는 닿지 않는다. 플랫폼마다 실제로 무엇이 도는지 한 화면에서 볼 수도 없다. 게다가 init은 그 시점의 기본 프리셋 전체를 파일에 쓰고 loader는 코드 기본값과 병합하지 않는다. 그래서 예전 버전이 쓴 프리셋은 새 등급(fable)과 새 에이전트(16개 체계)를 영영 받지 못한다(GAP 1). 이 저장소의 `autopus.yaml`도 그렇다(balanced = HB2, ultra = HU1).

이 SPEC은 세 진입점에 같은 구성 단계를 넣는다. 구성 단계는 다섯 가지로 이루어진다. UI 전용 역할 그룹 4개, '고급' 에이전트별 편집, 생성기와 같은 해석(Codex catalog 포함)으로 계산한 에이전트×플랫폼 표, 명시 확인 뒤에만 쓰는 저장, 정확 일치로만 판별하는 GAP 1 갱신이다.

## Outcome Boundary

- Outcome Lock 원문은 `research.md` `## Outcome Lock`에 있다. 요약은 다음과 같다. 대화형 터미널에서 `auto init`, `auto update`, `auto quality`가 에이전트×플랫폼 표를 보여 준다. 칸의 뜻은 플랫폼마다 다르다. Claude Code와 Codex 칸은 이번 run의 생성기가 같은 catalog snapshot으로 쓸 실제 `model`·`effort`다. OMP 칸은 profile이 정한 요청 route다(catalog 확인 전). OpenCode와 Antigravity 칸은 `플랫폼 기본값`이다. 사용자는 역할 그룹 4개에 등급을 하나씩 고르거나, '고급'에서 에이전트 하나씩 바꾼다. 바뀐 칸에 `*`가 붙은 적용 후 표를 보고 `y`로 승인할 때만 기존 스키마에 저장된다. 같은 run의 생성 결과는 적용 후 표와 똑같다. 대화형 update는 매번 묻고, Enter 한 번이면 아무것도 쓰지 않는다. 비대화형은 묻지도 바꾸지도 않는다. 예전 기본 프리셋은 "이전 버전 기본값"으로 보이고 확인해야만 갱신된다. 사용자가 고친 프리셋의 값은 이 기능의 어떤 쓰기에서도 바뀌지 않는다.
- Mandatory requirements: REQ-MUX-01 ~ REQ-MUX-19 (Must). REQ-MUX-20 ~ REQ-MUX-23은 Should다.
- Explicit non-goals:
  - OMP `role_model_policy` 통합, 그리고 OMP 칸의 catalog 반영 값(실제 실행 모델)
  - 에이전트별 provider 모델 ID 고정
  - 새 YAML 키(그룹, base mode, provider별 구성)
  - balanced/ultra 코드 기본값 변경
  - 런타임 해석 의미 변경. balanced historical 런타임 해석, 그리고 HB1·HB4를 런타임 표준 배치로 넓히는 것을 포함한다.
  - 기존 도구별 편집기와 OMP pin 편집기 제거
  - OpenCode·Antigravity 에이전트별 모델
  - 비대화형 GAP 1 갱신 명령
  - 디스크에 이미 있는 생성 파일과의 diff를 미리 보여 주는 것
  - update의 기존 migration 저장(`config.Save`)이 파일 전체를 다시 쓰는 동작 변경, 기존 파일 위 `auto init` 재실행 정책
  - quality 블록 안의 서식과 주석 보존. 값 보존은 REQ-MUX-12가 정한다.
- Completion evidence: `acceptance.md`의 Must 시나리오(S1–S14, S19–S21) PASS, `auto eval harness run` 통과, 신규·변경 패키지 coverage 85% 이상, 신규·변경 소스 파일 300줄 이하, 수동 터미널 smoke 기록(`evidence/manual-smoke.txt`, S21), `auto spec validate --strict` 통과.

## Trust Model

- 신뢰: main의 코드와 embedded content(`content/`, `templates/`).
- 반쯤 신뢰: `autopus.yaml`. 사용자 파일이지만 clone한 저장소에서 온 것일 수 있다. anchor, alias, merge key, `${NAME}` placeholder를 담을 수 있다. loader는 decode 전에 문서 전체 텍스트의 `${NAME}`을 환경 변수 값으로 바꾼다(`pkg/config/loader.go:151-161`). 그래서 loader가 읽은 config를 다시 직렬화해 파일에 쓰면 운영자의 환경 변수 값(token 등)이 tracked 파일에 남을 수 있다(F-001). 이 SPEC의 writer는 loader가 펼친 값을 쓰기 원천으로 쓰지 않는다(REQ-MUX-08).
- 비신뢰 입력: 운영자 키 입력(어휘로 검증한다), 출력할 YAML의 프리셋 이름과 description(제어 문자를 escape한다), Codex catalog JSON(파싱에 실패하면 기존 catalog_unknown 규칙을 따른다).
- 출력은 터미널과 `autopus.yaml`, 생성 파일뿐이다. 새 로그나 artifact는 없다.

## Requirements

### REQ-MUX-01 역할 그룹은 UI 전용이다
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL define exactly four UI-only role groups in the Role Group Contract order that cover every name returned by `config.CanonicalAgentNames()` exactly once, and SHALL never write a group name or any group-derived key to autopus.yaml.

- 정의는 `[NEW] internal/cli/quality_compose_groups.go`에 둔다. 정규 에이전트가 늘거나 줄면 커버리지 test가 실패한다.

### REQ-MUX-02 시작 구성은 실제 생성 등급이다
Priority: Must · EARS: EventDriven

WHEN a composition session starts, THEN THE SYSTEM SHALL take each canonical agent's starting tier from `[NEW] config.QualityConf.EffectiveAgentTier` under the global quality view, returning the balanced rung tier where `NativeBalancedAgentCandidate` applies and otherwise `AgentTier` with the fallback tier declared by that agent's embedded source.

- global view는 provider override를 지운 view다(`ForProvider("")`와 같다). override가 있는 열은 REQ-MUX-18이 따로 계산한다.
- fallback 원천은 `content/agents/<agent>.md`의 `model:`이다. Claude 생성기는 이 값을 `applyClaudeAgentProfile`의 `currentModel`로 넘긴다. Codex 템플릿의 literal 인자도 `TransformAgentForCodex`(`pkg/content/agent_transformer.go:107`)가 같은 값으로 만든다(C13). 표와 시작 구성은 `[NEW] internal/cli/quality_compose_sources.go`가 `pkgcontent.LoadAgentSourcesFromFS(contentfs.FS, "agents")`로 읽은 tier와 `MapEffort(effort, "codex")`를 쓴다.
- 예: balanced를 HB2로 기록한 프로젝트에서 debugger는 YAML에 `sonnet`이지만 런타임은 표준 배치(fable)다. 그래서 시작 등급은 `fable`이고, 손대지 않고 끝내면 쓰기는 0이다.

### REQ-MUX-03 혼합 그룹
Priority: Must · EARS: EventDriven

WHEN the members of one group hold different tiers, THEN THE SYSTEM SHALL display the group as `혼합` with per-tier counts in `config.QualityTiers()` order, keep every member's tier unless the operator selects a tier for that group, and set every member to the selected tier when one is selected.

- 표시 예: `혼합 (fable 2 · sonnet 4)`. 개수가 0인 등급은 쓰지 않는다. 멤버 등급이 모두 같으면 그 등급 이름만 쓴다.

### REQ-MUX-04 고급: 에이전트별 편집
Priority: Must · EARS: EventDriven

WHEN the operator opens 고급, THEN THE SYSTEM SHALL accept `agent=tier` lines validated by `parseQualityToolTierAssignment`, print a rejected line's error from that function without discarding earlier edits, finish on an empty line or EOF, and recompute every group display after each accepted edit.

### REQ-MUX-05 구성 표
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL render the composition table with one row per canonical agent in group order, a tier column, and one column per installed platform in the fixed order Claude Code, Codex, OMP, OpenCode, Antigravity, computing each Claude Code and Codex cell through the resolution function that platform's generator calls with the command run's single Codex catalog snapshot, and SHALL keep no second tier-to-model mapping.

- Claude: 지금 `pkg/adapter/claude`의 unexported `claudeAgentProfile`·`claudeAgentEffort`(`claude_agent_content.go:114-148`)를 `[NEW] config.QualityConf.ClaudeAgentProfile`와 `[NEW] config.ClaudeAgentEffort`로 옮긴다. adapter의 두 함수는 새 함수를 부르는 한 줄로 남긴다. Claude에는 catalog가 없다.
- Codex: `[NEW] codex.ResolveAgentProfile(quality, snapshot, agent, fallbackTier, declaredEffort)`와 `[NEW] codex.ResolveSupervisorProfile(quality, snapshot)`를 만든다. 지금 `codexRenderContext.resolveAgent`, `resolve`, `resolveCodexPlacement`(`codex_catalog.go:90-106`, `codex_native_placement.go:29-47`)가 하는 판단을 그대로 옮긴 순수 함수다. 템플릿 render context는 이 함수를 부르고 진단 출력만 더한다.
- snapshot: 표를 그리는 명령 run은 Codex catalog를 한 번만 읽는다. 읽기는 기존 `codexruntime.ProbeModelCatalog`(5초)로 하고, test는 `[NEW] composeCodexCatalogProbe` 변수로 바꾼다. 읽기에 실패하면 snapshot은 비어 있는 채 "읽음"으로 표시된다. 이것은 adapter가 스스로 probe에 실패했을 때와 같은 결과를 낸다(`codex_catalog.go:20-31`). 같은 run의 Codex 생성은 기존 `captureDriftGenerationSnapshot`·`driftAdapterFor`(`doctor_drift_content.go:60-80`)와 `codex.WithModelCatalog`(`platform_catalog.go:51-56`)로 같은 snapshot을 쓴다. 그래서 표와 생성이 다른 catalog를 볼 수 없다.
- OMP: profile이 비면 생성기가 모델을 묶지 않으므로(`omp_model_integration.go:38-40`) `플랫폼 기본값`이다. profile이 있으면 요청 route를 보인다. 요청 route는 `SelectedRoleModelProfileForQuality` → `OMPNativeAgentForRole` → `OMPNativeAgentRoute`의 첫 후보이고, 열 머리말에 `요청 route · catalog 확인 전`을 쓴다. OMP 생성은 `omp` 실행 파일로 catalog를 읽고 쓴 config를 다시 읽어 검증하므로(`omp_model_integration_probe.go:26`, `omp_model_integration_activation.go:56`) 표 계산에서 재현하지 않는다(non-goal). catalog 때문에 route가 낮아지면 기존 OMP degraded-route 안내가 생성 때 알린다.
- 비용: 네트워크와 subprocess는 Codex catalog probe 1회뿐이다(Codex가 설치된 경우). update와 `--apply`는 생성이 같은 snapshot을 쓰므로 probe 횟수가 늘지 않는다. 같은 config와 snapshot이면 같은 bytes를 낸다. 변경 표시는 색이 아니라 `*`다.
- YAML에서 온 프리셋 이름과 description은 제어 문자를 escape해서 출력한다.

### REQ-MUX-06 미리보기와 명시 확인
Priority: Must · EARS: EventDriven

WHEN a change is pending, THEN THE SYSTEM SHALL render the after-table from a detached candidate config with the same catalog snapshot, mark every changed cell with `*`, list every autopus.yaml key the write changes and every effect outside the table, and write only after an explicit `y` or `yes` answer to a prompt whose default is no.

- 후보 config는 `withQualityToolPreset`처럼 map을 복사한 분리 사본이다. 원본 config와 파일은 확인 전까지 바뀌지 않는다. 확인 전에 정해야 하는 선택(REQ-MUX-20의 override 유지 여부)은 모두 후보에 먼저 반영한 뒤 미리보기를 그린다.
- 표 밖 영향은 네 가지다. Codex 기본 세션 profile의 전후 값(`ManagesSupervisorModel()`일 때만), `quality.presets.custom 덮어씀`(custom이 있고 현재 binding이 아닐 때만), custom이 balanced 표준 배치와 ultra effort 사다리를 받지 않는다는 안내(C6), 그리고 block writer를 쓸 때의 `quality 블록을 다시 씁니다: 블록 안 주석 N줄과 서식은 유지되지 않습니다(값은 그대로)`다.
- 확인 질문은 `적용할까요? [y/N] · e) 계속 편집`이다. `e`는 후보를 유지한 채 편집 메뉴(`g`, `a`)로 돌아간다. `n`, 빈 줄, EOF, 그 밖의 답은 모두 취소다. 취소 출력은 `취소했습니다. 설정을 바꾸지 않았습니다.`이다.

### REQ-MUX-07 Binding
Priority: Must · EARS: EventDriven

WHEN a confirmed composition is persisted, THEN THE SYSTEM SHALL choose the write by the Binding Contract and write nothing when the edited composition equals the starting composition.

- 갱신(`u`)은 표가 그대로여도 프리셋을 고쳐 쓰는 명시 동작이다. 그래서 "같으면 쓰지 않음"은 그룹·고급 편집 경로에만 적용한다(REQ-MUX-11).

### REQ-MUX-08 quality writer
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL build every written quality value from the raw `quality` node decoded without environment expansion, SHALL refuse the write when that raw decode differs from the loaded quality section or when the quality subtree contains an alias node or a merge key, SHALL write `quality.default` alone through `saveQualityDefault` and every other change through `[NEW] saveQualityBlock`, and SHALL splice only the top-level `quality` block into the original bytes, so that no environment-expanded value is written and every byte outside that block stays identical.

- 이유(F-001): `project_name: &note '${PRESET_NOTE}'`와 `quality.presets.balanced.description: *note`처럼 quality 밖 anchor를 참조하면 quality 블록 안에는 `${`가 없다. 그런데 loader가 펼친 `cfg.Quality`를 다시 쓰면 펼쳐진 값이 파일에 남는다. 그래서 문자열 검사가 아니라 쓰기 원천을 바꾼다.
- 사전 검사(line writer와 block writer 공통, 쓰기 전):
  1. raw decode: 원본 bytes를 yaml.v3 Node로 읽고, root `quality` 값 node를 env 확장 없이 `config.QualityConf`로 decode한다. 원본은 `pruneRetiredConfig`(SPEC-PANERM-001 REQ-10)를 적용한 뒤의 bytes다.
  2. 불일치 거부: raw decode가 loader가 읽은 quality(`config.LoadPreview`)와 deep-equal이 아니면 쓰지 않는다. 설정된 환경 변수의 `${NAME}`이 quality 값에 직접 있든 alias·merge로 들어오든 모두 여기 걸린다.
  3. 참조 거부: quality subtree 안에 alias node(`*x`)나 merge key(`<<`)가 있으면 쓰지 않는다. 다시 encode하면 참조가 사본으로 바뀌어 의미가 달라진다. 환경 변수가 설정되지 않아 2번을 통과하는 참조도 여기서 막는다.
  4. 거부 출력: `quality 블록에 환경 변수 값이나 YAML 참조(alias·merge)가 섞여 있어 자동으로 쓰지 않습니다. autopus.yaml을 직접 고치세요.` 파일은 그대로다.
  설정되지 않은 변수의 `${NAME}`이 quality 값에 직접 있으면(참조 없음) 2번과 3번을 통과한다. 이때는 raw 값이 그대로 다시 쓰이므로 placeholder 글자가 보존된다.
- block 쓰기 순서: (1) 사전 검사를 통과한 raw quality에 이번 변경(binding, 갱신, 도구별 편집기의 프리셋과 binding)만 더해 후보를 만든다. (2) `replaceAutopusConfigSection`으로 후보를 encode하고 검증한다. (3) encode 결과에서 같은 경계 규칙으로 quality 블록을 잘라 원본 범위에 넣는다. (4) `verifyQualityLineEdit(원본, 결과, "quality")`와 `validateQualityYAML(결과, 후보)`(후보는 raw 기반)를 통과해야 한다. (5) atomic replace. 어느 단계든 실패하면 원본 bytes는 그대로다.
- 블록 경계(R05): root mapping에서 `quality` 다음에 오는 key node의 줄 직전까지가 후보 범위다. 다음 key가 없으면 파일 끝까지이고, 문서 표시 줄(`---`, `...`)이 먼저 오면 그 줄 직전까지다. 후보 범위 끝에 이어진 빈 줄과 1열 주석 줄은 다음 key의 머리 주석으로 보고 블록 밖에 둔다. quality 자식 사이에 있는 1열 주석(뒤에 들여쓴 자식이 이어짐)은 블록 안이다. root가 flow mapping이면 쓰지 않는다. 원본이 CRLF면 새 블록도 CRLF다. 블록이 없으면 파일 끝에 붙인다.
- 이유(C9, probe A1): 기존 `saveQualitySection`은 문서 node tree 전체를 `yaml.Marshal`로 다시 쓴다. 손으로 쓴 2칸 fixture에서 quality 밖의 빈 줄 3개가 사라지고 들여쓰기가 4칸이 됐다. `saveQualitySection`의 유일한 호출처인 도구별 편집기(`quality_tool_persist.go:46`)도 같은 F-001 노출이 있으므로(C20), `saveQualitySection`은 `saveQualityBlock`에 위임한다.
- 범위: 기존 update 저장(`persistUpdateConfigMigrations`, 언어 설정과 supervisor migration의 `config.Save`)은 지금처럼 동작한다(non-goal). init은 새 config를 `config.Save`로 쓴다.

### REQ-MUX-09 쓰는 키 제한
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL write only `quality.default`, `quality.presets.<name>.description`, and `quality.presets.<name>.agents` from the composition flows, and SHALL NOT add or modify `quality.providers`, `quality.supervisor_model_policy`, `role_model_policy`, or any key the v0.4.3 quality schema lacks.

- 이 키들은 v0.4.3부터 `QualityConf{Default, Presets{Description, Agents}}`에 있다. `custom`은 평범한 프리셋 이름이라 모든 출하 바이너리가 같은 뜻으로 읽는다.
- `quality.providers`를 쓰지 않는 이유는 C11에서 바로잡았다. 이 키를 strict decode로 거부하는 바이너리는 없다. 대신 v0.50.88 이전 바이너리는 이 키를 조용히 무시하므로 팀원마다 다른 모델이 돈다.
- 예외는 둘이다. REQ-MUX-20(Should)은 확인을 받은 뒤 `quality.providers.<p>` 항목 하나를 지울 수 있다. 구성 프롬프트에서 들어간 기존 도구별 편집기는 지금처럼 자기 프리셋과 `quality.providers.<tool>`을 쓴다(REQ-MUX-16).

### REQ-MUX-10 GAP 1 판별
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL classify each written `balanced` and `ultra` preset as `current` when its agents map exactly equals the current code default, `historical` when it exactly equals a Historical Preset Registry row of that preset, and `customized` otherwise including partial maps and invalid tiers, ignoring descriptions, while `isHistoricalBalancedDefault` keeps returning true only for the rows marked runtime standard.

- registry는 `[NEW] pkg/config/quality_preset_history.go`의 표 하나다. `historicalBalancedDefaults`(`native_balanced.go:52-67`)의 두 항목은 이 표의 HB5·HB2 행으로 옮기고 runtime standard로 표시한다. HB1·HB4는 판별에만 쓴다. 그래서 `NativeBalancedAgentCandidate`의 결과는 어떤 입력에서도 이 SPEC 전과 같다(C12).
- 파일에 그 프리셋이 없으면 `absent`다. 표시는 `현재 기본값`, `이전 버전 기본값 (갱신 가능)`, `사용자 수정 (N개 다름)`이다. N은 정규 에이전트 중 기록 값(없음 포함)이 현재 기본값과 다른 수다.
- 등록 행마다 test로 네 가지를 고정한다. 정규 에이전트 이름만 쓴다. 16개 이하다. 등급이 모두 유효하다. 현재 기본값과 다르다.

### REQ-MUX-11 historical 갱신
Priority: Must · EARS: EventDriven

WHEN a written built-in preset is historical and the composition step is interactive, THEN THE SYSTEM SHALL offer 최신 기본값으로 갱신, show the per-agent diff and the after-table, and rewrite every historical built-in preset's agents and description to the current code default only after explicit confirmation.

- `u` 한 번이 historical인 built-in 프리셋을 모두 갱신하고 한 번에 쓴다. 미리보기는 프리셋마다 줄을 나눈다. 묶이지 않은 프리셋, 그리고 런타임이 이미 표준 배치인 HB2·HB5 balanced는 `모델 변화 없음, YAML만 정리`로 보인다.
- builtin `ultra` OMP profile은 `quality.presets.ultra`를 읽는다(`builtinAgentTiers`). 그래서 ultra 갱신이 OMP 요청 route를 바꾸면 그 칸에도 `*`가 붙는다.

### REQ-MUX-12 사용자 수정 보호
Priority: Must · EARS: Unwanted

IF a written built-in preset is customized, THEN THE SYSTEM SHALL keep its `agents` map and `description` value unchanged in every write this feature makes and in every per-tool editor reachable from the composition prompt, label it `사용자 수정 (N개 다름)`, and persist any composition that would otherwise bind that name as `custom`.

- 보존 단위는 값이다(R03). block writer는 quality 블록을 다시 encode하므로 그 프리셋의 서식(들여쓰기, 따옴표, key 순서, 주석)은 바뀔 수 있고, 미리보기가 이를 알린다(REQ-MUX-06). 유지 경로는 파일 bytes가, line writer 경로는 `default:` 줄 밖 bytes가 그대로다.
- 도구별 편집기(R09): `readQualityToolPresetName`이 받은 이름이 customized built-in(`balanced`, `ultra`)이면 `사용자 수정 프리셋 <name>은 덮어쓰지 않습니다. 다른 이름을 입력하세요.`를 쓰고 다시 묻는다. 3번 연속이면 취소한다. current나 historical인 built-in 이름은 지금처럼 허용한다(명시한 이름 지정이다).
- 범위 밖: 기존 파일 위에서 `auto init`을 다시 실행하면 파일 전체를 새로 쓴다(기존 정책, PRD §8). 손 편집도 범위 밖이다.

### REQ-MUX-13 update 대화형 단계
Priority: Must · EARS: EventDriven

WHEN the project update path of `auto update` runs with the TTY probe true, without `--yes`, `-y`, `--auto`, `--check`, `--plan`, `--preview`, or `--dry-run`, and with `CI` not truthy, THEN THE SYSTEM SHALL show the current composition table with GAP 1 labels after the existing project settings prompts and before the Codex supervisor migration and platform generation, ask one question whose default keeps autopus.yaml byte-identical, and pass a confirmed change and the run's catalog snapshot into the same run's generation.

- 위치는 `update.go`에서 디자인 파일 생성 뒤, `migrateLegacyCodexSupervisorPolicy` 앞이다. `[NEW] internal/cli/update_compose.go`의 함수 하나를 3줄 이하로 부른다(`update.go`는 지금 291줄이다). 생성 루프의 `updateHarnessPlatform` 호출은 snapshot을 받는 `[NEW] updateHarnessPlatformWith`로 바꾼다(줄 수는 같다). 확인한 쓰기 뒤에는 `cfg.Quality`를 쓴 값으로 바꾼다. 그래야 뒤의 supervisor migration `config.Save`와 생성이 바뀐 구성을 쓴다.
- `--check`(R07): `checkOnly`는 `--self` 분기에서만 쓰인다(`update.go:44-50`). 그래서 `--self` 없는 `--check`는 지금처럼 프로젝트 update를 계속한다. 이 기능은 그 경우를 비대화형으로 다룬다.
- CI truthy는 값이 비어 있지 않고, 대소문자 무관하게 `0`이나 `false`가 아닌 경우다(A4).
- 잘못된 답에는 선택지를 다시 보여 준다. 3번 연속이면 `입력이 올바르지 않아 구성을 유지합니다.`를 쓰고 유지한다. 읽기 오류와 EOF도 유지다. 이 단계 때문에 update가 실패하지는 않는다. 쓰기가 거부되거나 실패하면 원본을 그대로 두고 경고 1줄을 남긴 뒤 update를 계속한다.
- freshness re-exec 자식(`update_freshness.go:227`)은 같은 stdin을 받는다. 질문은 자식에서만 나온다.

### REQ-MUX-14 비대화형은 묻지도 바꾸지도 않는다
Priority: Must · EARS: Unwanted

IF the TTY probe is false, `--yes`, `-y`, `--auto`, or `--check` is set, a preview flag is set, `CI` is truthy, or the run is a `--workspace` update, THEN THE SYSTEM SHALL NOT read stdin for the composition step, SHALL leave autopus.yaml byte-identical with respect to this feature, and SHALL print at most one notice line naming the historical presets outside preview runs.

- workspace: preview가 아닌 workspace update는 `--yes`나 `--auto`를 요구하고(`update_workspace.go:28-30`), 하위 update에 `--yes`나 `--plan`을 넘긴다(`:277-300`). 그래서 리포마다 묻는 경로는 없다(C10).
- 안내 문구(ko)는 `참고: ultra, balanced 프리셋이 이전 버전 기본값입니다. 대화형 auto update 또는 auto quality에서 갱신할 수 있습니다.`이다. historical이 없으면 아무것도 쓰지 않는다. preview 출력은 바꾸지 않는다. 비대화형 run은 표를 그리지 않으므로 catalog snapshot도 만들지 않고, 생성은 지금처럼 adapter가 직접 probe한다.

### REQ-MUX-15 init 세 번째 선택지
Priority: Must · EARS: EventDriven

WHEN `auto init` runs its interactive wizard, THEN THE SYSTEM SHALL offer balanced, ultra, and 역할별 직접 구성 in the quality step, start the shared editor from the current balanced default for the third choice, show the composition table of the chosen result before platform files are written, generate platform files with the same catalog snapshot, and keep the `--yes`, non-TTY, and `--quality` paths unchanged.

- `tui.buildQualityStep`에 세 번째 option(value `[NEW] tui.QualityRoleGroups`, 값 `role-groups`)을 더한다. wizard가 끝나면 `[NEW] internal/cli/init_compose.go`가 줄 기반 driver를 돌린다. 확인(`y`)하면 Binding Contract에 따라 메모리의 cfg를 고치고, 기존 `saveConfigWithGenerationRollback`이 파일을 쓴 뒤 `[NEW] generatePlatformFilesWith`가 snapshot으로 생성한다. 취소하면 balanced 기본값으로 진행한다.
- 종단 test seam(R08): `init.go`의 TTY 판정은 공유 TTY probe를 쓰고, wizard 호출은 `[NEW] runInitWizard` 변수(기본값 `tui.RunInitWizard`)를 거친다. test는 wizard 결과만 대신하고 나머지(driver, 저장, 실제 생성)는 실제 명령으로 돈다. huh에서 driver로 넘어가는 실제 터미널 경계는 수동 smoke(S21)로 확인한다.
- balanced나 ultra를 고르면 표를 정보로만 보여 주고 질문은 더하지 않는다(A5). `init.go`(253줄)에는 3줄 이하만 더한다.

### REQ-MUX-16 auto quality 진입점
Priority: Must · EARS: EventDriven

WHEN `auto quality` runs without arguments and with interactive input, THEN THE SYSTEM SHALL open the composition step first and keep the existing global preset menu and the Claude Code, Codex, and OMP editors reachable from the same prompt, and SHALL regenerate harness files with the run's catalog snapshot through `[NEW] applyQualityHarnessWith` after a confirmed change when `--apply` is set.

- 입력 판정은 지금과 같다. 실제 stdin이 TTY가 아니면 기존 오류 `interactive quality selection requires a TTY; use an explicit quality or profile command`를 낸다.
- 같은 질문이 기존 대상 이름 `global`, `claude-code`, `codex`, `omp`를 받으면, `chooseQualityTarget`이 그 대상을 고른 것처럼 같은 함수로 바로 보낸다. 새 동작은 글자 키 `u`, `g`, `a`, `t`로 고른다. 그래서 기존 scripted test 입력(`claude-code\n…`, `codex\n…`, `global\n1\n`)은 바꾸지 않고 통과한다.
- `applyQualityHarnessWith`는 기존 `applyQualityHarness`와 같은 출력(`quality.applied_platforms = N`)을 내지만 실제 adapter를 snapshot으로 만든다. 기존 `qualityPlatformUpdater` 변수와 그것을 쓰는 기존 명령은 그대로다.
- 인자가 있는 하위 명령(`auto quality <preset>`, `provider`, `supervisor`, `show`)은 그대로다.

### REQ-MUX-17 공유 core와 driver
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL implement composition state, groups, classification, binding, and table rendering once without UI dependencies, and SHALL drive init, update, and quality through one line-based prompt driver that reads one shared `bufio.Reader` set on the command input and takes injectable TTY, CI, Codex catalog, and init wizard seams.

- 공유 reader: `chooseQualityTarget`처럼 `cmd.SetIn(bufio.NewReader(in))`로 한 번만 감싼다. 이어지는 `bufio.NewReader` 호출은 같은 reader를 돌려받는다.
- seam 기본값은 TTY `isStdinTTY()`, CI `os.Getenv("CI")`, catalog `codexruntime.ProbeModelCatalog`, wizard `tui.RunInitWizard`다. test는 `[NEW]` package 변수를 바꿔 넣고 `t.Cleanup`으로 되돌린다. 기존 update test는 대부분 `--yes`도 `SetIn`도 쓰지 않고 probe도 바꾸지 않으므로 지금처럼 비대화형으로 돈다(C14).
- init의 기존 huh 단계는 그대로 쓴다. 줄 기반 driver에는 Windows 터미널의 huh hang 우회(`tui/wizard_steps.go::RunInitWizard`)가 필요 없다.

### REQ-MUX-18 provider override 열
Priority: Must · EARS: StateDriven

WHERE `quality.providers.claude` or `quality.providers.codex` names a preset, THEN THE SYSTEM SHALL compute that platform's column from the override, label the column header `개별 설정: <preset>`, and leave the override unchanged.

### REQ-MUX-19 생성 결과 = 적용 후 표
Priority: Must · EARS: EventDriven

WHEN a confirmed composition is applied in the same run by update generation, `auto quality --apply`, or init generation, THEN THE SYSTEM SHALL produce Claude Code and Codex agent files whose `model` and `effort` values equal the after-table cells computed with that run's catalog snapshot, regardless of what previously generated files contained.

- `*`는 시작 config와 후보 config를 같은 binary와 같은 snapshot으로 계산한 두 표의 차이다(R06). 디스크에 이미 있는 생성 파일(이전 버전 산출물, 손으로 고친 파일)과의 차이는 약속하지 않는다. 재생성은 그런 파일도 표 값으로 맞춘다(S20). 확인 없이 끝난 run의 생성 결과는 처음 표와 같다.
- Claude 셀은 frontmatter에 쓰일 값 그대로다. 정확한 ID(`claude-fable-5-1`)와 tier alias(`fable`)를 구별한다(C16). Codex 셀은 snapshot으로 해석한 Effective 값이다(C17).

### REQ-MUX-20 override가 새 구성을 따를지 묻기
Priority: Should · EARS: StateDriven

WHERE a provider override exists and the edited composition changes the global binding, THEN THE SYSTEM SHALL ask whether that provider follows the new composition before rendering the preview, apply the answer to the candidate config so that the after-table and the changed-key list show it, and on confirmation remove only that `quality.providers.<p>` entry through the existing provider writer.

- 순서(R04): 편집 → override 질문(기본값 아니오) → 미리보기 → 최종 확인 한 번이다. 승인한 표와 생성 결과가 어긋나지 않는다.

### REQ-MUX-21 읽기 전용 표
Priority: Should · EARS: EventDriven

WHEN `auto quality show --agents` runs, THEN THE SYSTEM SHALL print the composition table read-only on any stdin, including non-TTY, after the existing `show` lines, which stay unchanged.

### REQ-MUX-22 locale
Priority: Should · EARS: Ubiquitous

THE SYSTEM SHALL render group labels, prompts, notices, and `플랫폼 기본값` in Korean when `tui.DetectSystemLang()` returns `ko` and in English otherwise, while every value written to autopus.yaml stays locale-independent.

### REQ-MUX-23 문서
Priority: Should · EARS: Ubiquitous

THE SYSTEM SHALL document the role groups, table columns, binding rules, update prompt contract, GAP 1 classes, and OMP independence in CLI help, README, and the v0.50.125 CHANGELOG, editing `content/` and `templates/` sources and regenerating with `make generate-templates` instead of editing generated roots.

## Contracts

### Role Group Contract

| 순서 | 그룹 (ko / en) | 에이전트 (표시 순서) |
|---|---|---|
| 1 | 설계·판단 / Design·Judgment | planner, architect, spec-writer, security-auditor |
| 2 | 구현 / Implementation | executor, debugger, deep-worker, devops, frontend-specialist, perf-engineer |
| 3 | 검증·리뷰 / Verification·Review | reviewer, validator, tester, ux-validator |
| 4 | 탐색·보조 / Exploration·Support | explorer, annotator |

- 그룹 질문은 `1) fable 2) opus 3) sonnet 4) haiku [Enter] 그대로`이다(`config.QualityTiers()` 순서). 숫자와 등급 이름을 받는다. 잘못된 답에는 다시 묻고, 3번 연속이면 그 그룹을 그대로 둔다.

### Table Cell Contract

| 열 | 표시 조건 | 칸 값(raw)의 원천 | 표기 |
|---|---|---|---|
| 등급 | 항상 | `EffectiveAgentTier`(REQ-MUX-02) | 프리셋에 없으면 `기본값(<tier>)`, 잘못된 값이면 `잘못된 값→<tier>` |
| Claude Code | platforms에 `claude-code` | Claude 열 view의 `ClaudeAgentProfile(agent, fallback)` | `<model> · <effort>`. 정확한 ID는 `claude-`를 뗀다(`fable-5-1`). alias는 그대로(`opus`). effort가 없으면 `-` |
| Codex | `codex` | `ForProvider(codex)` view와 run snapshot으로 `codex.ResolveAgentProfile`의 Effective | `gpt-`를 뗀다(`6.1-sol · xhigh`). Fallback이 있으면 표시를 붙인다(아래) |
| OMP | `omp` | profile이 비면 없음. 있으면 `OMPNativeAgentRoute` 첫 후보(요청 route) | 비면 `플랫폼 기본값`. 있으면 `<native> · <model>:<thinking>`(provider 접두사와 `claude-`/`gpt-`를 뗀다, `task · fable-5-1:max`) |
| OpenCode | `opencode` | 없음(에이전트 파일에 model이 없다, C1) | `플랫폼 기본값` |
| Antigravity | `antigravity-cli` | 없음 | `플랫폼 기본값` |
| footer | `codex` 설치 | catalog 상태, Fallback 칸 수, `ManagesSupervisorModel()`이면 `codex.ResolveSupervisorProfile` | `Codex catalog: 읽음` 또는 `읽지 못함`, `Codex 기본 세션: 6-astra · ultra` |

- Codex Fallback 표시: `model_unavailable`과 `effort_unavailable`은 `(대체)`, `runtime_default`는 model이 없으면 `Codex 기본값`이고 effort만 없으면 `<model> · -`, 읽지 못한 catalog에서 표준 배치가 아닌 칸은 `(catalog 없음)`, 표준 배치 칸은 `(미확인)`, 표준 배치 거부(`ErrCodexNativePlacementUnavailable`)는 `생성 거부`다. `생성 거부`가 있으면 미리보기와 질문 화면에 `Codex 생성이 실패합니다`를 쓴다.
- raw 값과 표기는 따로 둔다. 생성 결과 비교와 `*` 판정은 raw 값으로 한다.
- 설치된 플랫폼 중 에이전트별 모델을 쓰는 곳이 없으면 footer에 그 사실을 쓴다.

### Binding Contract

| 결과 구성 (그룹·고급 편집) | 같은 이름의 built-in 프리셋 | 쓰기 | writer |
|---|---|---|---|
| 시작 구성과 같음 | — | 없음(`변경 없음`) | — |
| 최신 `balanced` 기본값과 같음 | current | `quality.default: balanced` | line |
| 최신 `balanced` 기본값과 같음 | historical | `quality.presets.balanced` ← 최신 기본값, `quality.default: balanced` | block |
| 최신 `balanced` 기본값과 같음 | absent | 위와 같다. C8: `Validate`는 `quality.default`가 presets에 있어야 통과한다(`schema.go:235-239`) | block |
| 최신 `balanced` 기본값과 같음 | customized | `quality.presets.custom` ← 결과 16개, `quality.default: custom` | block |
| 최신 `ultra` 기본값과 같음 | 위 네 행과 같은 규칙 | `ultra` | line 또는 block |
| 그 밖 | — | `quality.presets.custom` ← 정규 에이전트 16개 전부와 description `역할 그룹 구성`, `quality.default: custom` | block |

- `quality.default`가 이미 결과의 이름이면 그 줄은 쓰지 않는다. 다른 프리셋(예전 `custom`, 사용자 프리셋 `fast`)은 지우지 않고 값도 바꾸지 않는다.
- `u` 갱신: historical인 built-in 프리셋마다 `quality.presets.<name>`의 agents와 description을 최신 기본값으로 바꾼다. `quality.default`는 그대로이고, writer는 block이다.
- 모든 쓰기는 REQ-MUX-08의 사전 검사를 먼저 통과해야 한다.

### Update Prompt Contract

| 조건 | 질문 | stdin 읽기 | autopus.yaml (이 기능 기준) | 출력 |
|---|---|---|---|---|
| TTY probe true, `--yes`/`-y`/`--auto`/`--check` 없음, preview 없음, CI 아님 | 1회, 기본값 유지 | 예 | Enter, EOF, 3회 오입력이면 그대로. 확인한 변경만 쓴다 | 표, 프리셋 상태, 질문 |
| TTY probe false | 없음 | 아니오 | 그대로 | historical 안내 1줄 이하 |
| `--yes`, `-y`, `--auto` | 없음 | 아니오 | 그대로 | 안내 1줄 이하 |
| `--check` (`--self` 없음, 프로젝트 update는 계속) | 없음 | 아니오 | 그대로 | 안내 1줄 이하 |
| `CI` truthy | 없음 | 아니오 | 그대로 | 안내 1줄 이하 |
| `--plan`, `--preview`, `--dry-run` | 없음 | 아니오 | 그대로 | 이 기능 출력 없음 |
| `--self` (`--self --check` 포함) | 없음(구성 단계 전에 `runSelfUpdate`로 끝남) | 아니오 | 그대로 | 없음 |
| `--workspace` (항상 `--yes` 또는 preview) | 없음 | 아니오 | 그대로 | 리포마다 안내 1줄 이하 |

- 질문 화면은 표, 프리셋 상태 줄, 그리고 `구성을 바꿀까요? [Enter] 유지 · u) 최신 기본값으로 갱신 · g) 역할 그룹 · a) 고급`이다. `u`는 historical이 있을 때만 보인다. `auto quality`는 여기에 `t) 도구별 설정`과 기존 대상 이름을 더 받는다.
- 입력 횟수: 유지 경로는 1회, 그룹 경로는 6회 이하다(`g`, 그룹 4개, `y`).

### Historical Preset Registry (가정 A3 해소)

`git log -p -- pkg/config/defaults.go`와 `git tag --contains`로 복원했다(2026-10-10). 여기서 "출하"는 릴리스 태그에 들어간 배치를 말한다.

| 행 | 프리셋 | agents (정확히 이 맵) | 출하 태그 | 도입 commit | runtime standard |
|---|---|---|---|---|---|
| HB1 | balanced | planner, architect, spec-writer, security-auditor = opus. executor, tester, reviewer, debugger, devops = sonnet. validator, explorer = haiku (11개) | v0.4.3–v0.40.14 (133개) | c95806e2 | 아니오 |
| HB2 | balanced | planner, architect, spec-writer, security-auditor = opus. executor, tester, reviewer, debugger, devops, validator, explorer = sonnet (11개) | v0.40.15–v0.50.103 (160개) | c21f7316 | 예 (지금 `historicalBalancedDefaults[1]`) |
| HB4 | balanced | architect, deep-worker, executor, planner, security-auditor, spec-writer = opus. 나머지 10개 = sonnet (16개) | v0.50.105–v0.50.114 (10개) | aaeae52d | 아니오 |
| HB5 | balanced | architect, planner, security-auditor = fable. debugger, deep-worker, executor, reviewer, spec-writer = opus. 나머지 8개 = sonnet (16개) | v0.50.115 (1개) | 001830fd | 예 (지금 `historicalBalancedDefaults[0]`) |
| HU1 | ultra | planner, executor, validator, tester, reviewer, architect, spec-writer, security-auditor, debugger, explorer, devops = opus (11개) | v0.4.3–v0.50.103 (293개) | c95806e2 | 해당 없음 |
| HU2 | ultra | 정규 에이전트 16개 = opus | v0.50.105–v0.50.114 (10개) | aaeae52d | 해당 없음 |

- 현재 기본값: balanced는 fbe502c0(v0.50.116–v0.50.124)부터의 `balancedRungByAgent` 배치(fable 7, sonnet 9), ultra는 001830fd(v0.50.115–v0.50.124)부터의 배치(fable 7, opus 9)다.
- 제외: 0cb6296b(2026-08-10, executor만 opus로 올린 11개 balanced)는 같은 날 aaeae52d로 대체되어 어느 태그에도 단독으로 들어가지 않았다(v0.50.104 태그는 없다). 이 배치를 쓴 파일은 customized로 보고 고치지 않는다.
- 이 저장소의 `autopus.yaml:107-138`은 balanced = HB2, ultra = HU1이고, `quality.default: ultra`, `supervisor_model_policy: quality`다.

## Existing Test Changes

바꾸는 기존 test는 0건으로 예상한다. 근거는 다섯 가지다.

1. 기존 `auto quality` scripted 입력은 모두 대상 이름(`claude-code`, `codex`, `global`)으로 시작한다. REQ-MUX-16이 그 이름을 기존 흐름으로 보낸다. 도구별 편집기 test는 기본 프리셋 이름(Enter)을 쓰므로 R09 guard에 걸리지 않는다.
2. 기존 update test는 TTY probe를 바꾸지 않으므로 비대화형이다. 현재 기본 프리셋에는 historical이 없으므로 안내도 나오지 않는다. 비대화형 run은 snapshot을 만들지 않으므로 생성 경로도 그대로다.
3. `isHistoricalBalancedDefault`의 답, `claudeAgentEffort` 이름, Codex 템플릿 함수의 결과는 그대로다(해석을 옮기기만 한다).
4. `saveQualitySection`을 직접 부르는 test는 없다(호출처는 `quality_tool_persist.go:46` 하나). 도구별 편집기 test fixture에는 placeholder와 alias가 없다.
5. `wizard_steps_test.go`는 quality step이 nil이 아닌지만 본다.

T15에서 전체 suite로 확인한다. 바뀌는 test가 생기면 요구사항으로 올리지 않는다. 이 절에 적고 review를 받는다.

## 생성 파일 상세

- `[NEW] pkg/config/quality_preset_history.go`(+test): registry 6행, `[NEW] ClassifyBuiltinQualityPreset`, runtime standard 표시. `native_balanced.go`의 `isHistoricalBalancedDefault`는 runtime standard 행만 본다.
- `[NEW] pkg/config/quality_effective.go`(+test): `EffectiveAgentTier`, `ClaudeAgentProfile`, `ClaudeAgentEffort`. `pkg/adapter/claude/claude_agent_content.go`는 위임만 한다.
- `[NEW] pkg/adapter/codex/codex_resolve.go`(+test): `ResolveAgentProfile`, `ResolveSupervisorProfile`. `codex_catalog.go`와 `codex_native_placement.go`의 render context는 이 함수를 부른다.
- `[NEW] internal/cli/` 파일: `quality_compose_sources.go`, `quality_compose_groups.go`, `quality_compose_table.go`, `quality_compose_bind.go`, `quality_config_block.go`, `quality_compose_driver.go`, `quality_compose_text.go`, `quality_compose_snapshot.go`(`composeCodexCatalogProbe`, `updateHarnessPlatformWith`, `generatePlatformFilesWith`, `applyQualityHarnessWith`), `update_compose.go`, `init_compose.go`, `quality_compose_entry.go`와 각 `_test.go`. 파일마다 300줄 이하다.
- 기존 파일 변경: `internal/cli/update.go`, `init.go`, `quality.go`(호출 3줄 이하와 `show --agents` flag), `quality_tool_persist.go`(`saveQualitySection` 위임), `quality_tool_interactive.go`(R09 guard), `internal/cli/tui/wizard_steps.go`(세 번째 option).
- 문서: `README.md`, `CHANGELOG.md`(v0.50.125), CLI help. 생성 표면 문구는 `content/`·`templates/`에서 고치고 `make generate-templates`로 만든다. 수동 smoke 기록은 `evidence/manual-smoke.txt`다.

## Related SPECs

- SPEC-PROVIDER-QUALITY-001(completed): `EffectiveMode` 우선순위(`@AX:ANCHOR`), `NormalizeQualityProvider`, 도구별 편집기를 재사용한다. 구성 흐름은 `quality.providers`를 쓰지 않는다.
- SPEC-INITUX-001(completed): quality 단계에 세 번째 선택지를 더한다. 플래그로 단계를 건너뛰는 규칙은 그대로다.
- SPEC-OSSUX-001(completed): profile 단계는 그대로다.
- SPEC-PANERM-001 REQ-10: 새 writer도 퇴역 키 prune을 따른다.
- OMP 계열(SPEC-OMP-005 등): native agent 접힘과 profile 해석을 읽기만 한다.
- Sibling: 없다(`research.md` `## Sibling SPEC Decision`).

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-MUX-01 | T5 | S1 | INV-MUX-05 |
| REQ-MUX-02 | T2, T4, T5 | S2 | INV-MUX-01, INV-MUX-06 |
| REQ-MUX-03 | T5 | S2, S3 | INV-MUX-06 |
| REQ-MUX-04 | T5, T9 | S3 | INV-MUX-06 |
| REQ-MUX-05 | T2, T3, T4, T6 | S4, S5, S19 | INV-MUX-02 |
| REQ-MUX-06 | T6, T7, T9 | S6 | INV-MUX-01, INV-MUX-02 |
| REQ-MUX-07 | T7 | S6, S7 | INV-MUX-07 |
| REQ-MUX-08 | T8 | S13 | INV-MUX-08, INV-MUX-12 |
| REQ-MUX-09 | T7, T8 | S1, S13 | INV-MUX-11 |
| REQ-MUX-10 | T1 | S8 | INV-MUX-09, INV-MUX-10 |
| REQ-MUX-11 | T1, T7 | S9 | INV-MUX-10 |
| REQ-MUX-12 | T1, T7, T12 | S8, S9 | INV-MUX-03 |
| REQ-MUX-13 | T3, T10 | S10, S20 | INV-MUX-01, INV-MUX-02 |
| REQ-MUX-14 | T10 | S10 | INV-MUX-04 |
| REQ-MUX-15 | T3, T11 | S11, S21 | INV-MUX-02 |
| REQ-MUX-16 | T3, T12 | S12 | INV-MUX-02 |
| REQ-MUX-17 | T9 | S10, S11, S12 | INV-MUX-04 |
| REQ-MUX-18 | T6 | S4 | INV-MUX-02 |
| REQ-MUX-19 | T3, T6, T10, T12 | S5, S14, S19, S20 | INV-MUX-02 |
| REQ-MUX-20 | T13 | S15 | INV-MUX-11 |
| REQ-MUX-21 | T13 | S16 | INV-MUX-02 |
| REQ-MUX-22 | T9, T13 | S17 | - |
| REQ-MUX-23 | T14 | S18 | - |
| Brownfield 회귀 | T15 | S10, S12 | INV-MUX-09 |

## Review Resolution (rev 1)

| Finding | 처리 | 위치 |
|---------|------|------|
| MUX-R01 / F-001 (security) quality 밖 placeholder anchor를 alias로 참조하면 펼쳐진 값이 써진다 | 쓰기 원천을 env 확장 없는 raw quality decode로 바꿨다. raw와 loader 값이 다르거나 alias·merge가 있으면 거부한다. line writer에도 같은 사전 검사를 둔다. 같은 writer를 쓰던 도구별 편집기도 위임한다(C20) | Trust Model, REQ-MUX-08, S13 |
| MUX-R02 표와 생성 값 불일치(Codex catalog, OMP degrade) | Codex 칸은 생성기의 해석 함수와 run 단위 catalog snapshot으로 계산하고, 같은 run의 생성이 같은 snapshot을 쓴다. OMP 칸은 요청 route로 범위를 명시했다. 대체·거부·catalog 없음 표기를 정했다 | REQ-MUX-05, REQ-MUX-19, Table Cell Contract, S5, S19 |
| MUX-R03 customized 프리셋 byte 불변과 block 재encode 충돌 | 보존 단위를 값(agents, description)으로 바꿨다. byte 불변은 유지 경로(파일 전체), block 밖, line writer의 `default:` 밖으로 한정했다. 미리보기가 서식 변화를 알린다 | REQ-MUX-06, REQ-MUX-12, INV-MUX-03, S9, S13 |
| MUX-R04 override 질문이 승인 뒤에 나온다 | 질문을 미리보기 전으로 옮겨 후보에 반영한다 | REQ-MUX-06, REQ-MUX-20, S15 |
| MUX-R05 quality 자식 사이 1열 주석이 블록을 끊는다 | 경계를 다음 root key node로 정하고, 그 앞의 빈 줄·1열 주석만 블록 밖에 둔다 | REQ-MUX-08, S13 |
| MUX-R06 표 diff와 디스크 diff 기준이 다르다 | `*`는 같은 binary·snapshot으로 계산한 두 표의 차이다. 디스크 파일과의 diff는 약속하지 않고, 재생성 결과가 표와 같다는 것을 약속한다 | REQ-MUX-19, INV-MUX-02, S14, S20 |
| MUX-R07 (Q-CORR-01) 단독 `--check` 설명이 코드와 다르다 | `--self`만 구성 단계 전에 끝난다. `--self` 없는 `--check`는 프로젝트 update를 계속하고 구성 단계는 비대화형이다 | REQ-MUX-13, REQ-MUX-14, Update Prompt Contract, S10 |
| MUX-R08 (Q-COMP-02) init과 `--apply`의 종단 검증 부족 | 실제 init 명령(`runInitWizard` seam만 대체)과 실제 adapter로 도는 `--apply`, 수동 터미널 smoke를 Must로 넣었다 | REQ-MUX-15, REQ-MUX-16, S11, S12, S21 |
| MUX-R09 기존 도구별 편집기가 customized built-in을 덮어쓸 수 있다 | 이름 단계에 guard를 두고, 보호 범위를 이 기능의 쓰기와 그 프롬프트에서 닿는 편집기로 명시했다 | REQ-MUX-12, S9 |
| Q-COMP-04/05, Q-FEAS-01/03 | INV-MUX-02, INV-MUX-03, INV-MUX-12를 다시 정의했다. oracle은 같은 snapshot으로 돈 실제 adapter `Generate`와 비교한다 | research.md, acceptance.md |
