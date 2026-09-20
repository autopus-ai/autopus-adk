# Changelog — autopus-adk

All notable changes to this project will be documented in this file.

## [Unreleased]

- **작업 근거에 따라 실행 절차를 선택한다** (2026-09-20): `auto workflow triage`가
  범위·위험·불확실성·검증 방법을 바탕으로 inline/guided/planned를 제안한다.
  작은 저위험 수정에 기본 계획·위임 절차를 추가하지 않고, 민감 변경과 반복
  실패는 상향한다. 병렬 추천은 독립성·부모 범위·소유권·실행 가능성을 별도로
  확인하며 모델 설정과 필수 게이트를 유지한다. 판단은 호출자 선언 기반의
  조언이고 실행이나 검증 완료를 뜻하지 않는다.

- **OpenCode V1·V2 생성 규약을 분리한다** (2026-09-20): 설치된 버전에 따라
  플러그인과 위임 지침을 생성한다. V2는 SDK 설치가 필요 없는 네이티브
  플러그인 객체와 `subagent` 규약을
  사용하며, 기존 V1 지원을 유지한다. 플러그인 옵션·명시적 비활성화·마커 밖
  사용자 문구를 보존하고, 버전 조회 실패로 V2 플러그인을 V1으로 덮어쓰지
  않도록 막는다. 검증 범위와 모델 인증 한계는 `docs/opencode-v2-compatibility.md`에 기록한다.

- **네이티브 작업자 lifecycle과 팀 사용량을 분리해 확인한다** (2026-09-20):
  `doctor agents`의 기본 동작은 설치 정보 조회이며 모델을 호출하지 않는다.
  명시적 live 모드에서 Codex app-server의 실제 위임과 OpenCode의 연결된
  세션 API를 검사한다. 생성·결과·취소·정리의 ID와 관측 순서를 확인하고,
  취소 ACK나 호스트 종료만으로 성공을 판정하지 않는다. 가져온 trace는 실제
  실행 증명으로 승격하지 않는다. Codex 0.155.1 실제 검사에서 네 단계 PASS를
  확인했다. OpenCode 1.18.7은 외부 플러그인을 끈 모드의 403과 일반
  모드의 UnknownError를 구분해 기록했으며, 전체 lifecycle은 미검증이다.

- **협업 관측의 큐 과부하와 비용 이중 계산을 막는다** (2026-09-20): Codex V2
  `subAgentActivity`를 부모 출처와 대조하고, 무관한 본문·추론 델타는 lifecycle
  큐에 넣지 않는다. 300개 델타가 RPC 대기 중 큐를 넘치게 하던 경로를 재현해
  수정했으며 기존 제한은 유지한다. `telemetry team`은 self-call만 합산하고
  parent-inclusive rollup, 중복, 서로 다른 소유자의 같은 호출 주장을 구분한다.
  누락은 null로, 관측한 비용은 부분합으로 남긴다. OpenCode 단가 기반 금액은
  추정 비용이며 실제 청구액으로 취급하지 않는다.

- **멀티에이전트 실행의 의존성과 반환 범위를 검사한다** (2026-09-20):
  공개 `ParallelRunner`가 `Phase.DependsOn`을 지키도록 고쳤다. 잘못된 그래프는
  실행 전에 거부하고, 선행 단계가 성공한 작업만 슬롯 한도 안에서 시작 대상으로
  선택한다. 실패한 선행 작업의 후속 실행을 막고, 마지막 완료와 취소가 겹쳐도
  취소를 성공으로 표시하지 않는다. 슬롯 기록은 실제 시작 순서나 워크트리 생성
  증거와 구분한다. 작업자 receipt는 선언한 범위 밖의 변경 파일을 거부하며,
  별도 validator는 호출자가 제공한 담당 범위보다 넓은 선언도 거부한다.
  기본 CLI의 순차 실행 방식과 marker 없는 기존 출력의 호환성은 유지한다.

- **플랫폼별 팀 실행 안내를 최신 지원 범위에 맞춘다** (2026-09-20): OpenCode가
  명시적인 `--team`을 기본 task 실행으로 바꾸던 안내를 제거했다. Antigravity도
  자체 멀티에이전트 지원과 현재 세션에서 검증된 ADK 연결 경로를 구분하도록
  정정했다. 새로운 vendor workflow 자동 호출은 추가하지 않았으며, 요청한 팀
  실행 경로를 확인할 수 없으면 `unsupported-mode`로 알린다.

- **하네스 노출·선택·비교를 검증 가능한 명령으로 제공한다** (2026-09-19):
  `auto skill audit`가 설정상 노출과 로컬 파일을 구분하고, 선택 이유·중복·누락·
  크기 및 명시적인 토큰 추정치를 보여 준다. 실제 세션 로딩은 `UNKNOWN`으로
  남긴다. `skill select`와 `skill policy-check`는 명시적 작업 종류·파일 조건·
  버전 정책을 긍정·부정·비호환 사례로 검사하며 스킬을 실행하거나 설치하지
  않는다. `telemetry harness`는 native/current/reduced 관측값을 같은 과제·모델·
  환경·인수 기준에서 비교한다. 실패·재시도 지출은 포함하고 누락은 null로
  남기며, 일부만 확인된 지출은 별도 부분합으로 표시한다. 실측 없이 승자나
  성능 개선을 선언하지 않는다.

- **재검증 계획을 읽기 전용으로 확인하고 불확실한 재사용을 막는다** (2026-09-19):
  `auto spec gates --read-only`는 설정과 기존 근거 파일을 바꾸지 않는다.
  `--no-reuse`는 명령·도구·환경·외부 상태가 달라졌을 때 기존 성공 결과를
  재사용하지 않도록 한다. 검증 지침은 통합 후 공통 검증과 변경된 범위의
  재검사를 구분하며 필수 안전·인수 검사를 유지한다. 프롬프트 생성은 중복
  레이어 ID를 거부해 구성 요소 삭제가 변경 비교에서 숨겨지는 결함을 막는다.

- **macOS 샌드박스 테스트에서 자식 커버리지를 보존한다** (2026-09-19):
  전체 race·coverage 검사에서 기존 deny-default fixture 두 건이 보안 단언을
  통과한 뒤 Go coverage 파일 쓰기 거부로 실패했다. 테스트 helper가 부모 Go
  작업의 coverage 디렉터리만 쓰기 허용하고 자식의 flag·환경에 같은 경로를
  전달한다. 운영 sandbox 정책과 secret 제거 단언은 유지하며 자식 counter도
  최종 profile에 포함한다.

- **adk CI 커버리지 게이트를 83에서 85로 올린다** (2026-09-15): 두 라운드의 테스트 추가로 CI linux 실측이 83.7% → 84.9% → **85.6%** 가 됐고, 0.6pp 여유를 확인한 뒤 게이트를 옮겼다. `ci.yaml`의 `COVERAGE_THRESHOLD`와 기본값, 그리고 `internal/companionmanifest/release_ci_stability_test.go`의 핀을 함께 `85`로 옮겼다(핀이 따로 남으면 게이트가 조용히 되돌려질 수 있다). 이로써 하네스가 프로젝트에 부과하는 기본 하한 85와 adk 자체 CI 게이트가 같은 숫자가 됐지만, 둘은 여전히 별개 기계다 — 기본값은 파이프라인 Gate 3을 먹이고 CI 숫자는 이 저장소의 Go 문장을 재며, 프로젝트는 자기 하한을 낮추거나 `0`으로 끌 수 있다.

- **커버리지 게이트 승격 2라운드** (2026-09-15): CI 실측 84.9%에서 게이트 85%를 여유 있게 넘기려고 남은 미커버 결정 로직을 덮었다. `pkg/qa/project` 22.7%→100%, `pkg/worker/controlplane` 69.6%→97.3%, `pkg/worker/mcpserver` 84.7%→92.7%, `pkg/selfupdate` 81.8%→87.4%, `pkg/companionmanifest` 83.1%→86.6%, `pkg/adapter/codex` 88.4%→90.4%, `pkg/adapter/omp` 86.4%→88.3%, `pkg/design` 86.6%→89.8%, `pkg/content` 90.9%→93.1%, `pkg/qa/run` 87.6%→89.0%, 그리고 `auto experiment` 의 git 기반 init/commit/reset/metric/record 경로. 단정 대상은 signed pair 트랜잭션의 fault 지점별 복구(커밋 마커 이후에는 새 쌍 유지), zip/tar 릴리스 추출의 체크섬·구조 거절, OMP readiness 판정의 이유 우선순위와 probe 실패 fan-out, Figma/외부 fetch의 사설 IP·리다이렉트 거절, QA 시그널 탐지 경계다.

  플랫폼 의존 경로는 skip guard로 격리했다. managed OMP network sandbox는 darwin 전용 build tag라 observe 준비 테스트 3건이 linux에서 실패했고, sandbox가 있는 플랫폼에서만 실행하도록 고쳤다. launchd 설치 경로는 실제 사용자 세션을 건드리므로 darwin에서 skip하고 linux에서만 실패 분기를 덮는다.

- **커버리지 게이트 승격을 위해 미커버 결정 로직을 테스트로 덮는다** (2026-09-15): CI 실측 83.7%에서 85% 게이트로 올리기 위해 미커버 상위 블록을 동작 단정 테스트로 덮었다. 파일 소유권을 나눠 병렬로 작업했고, 프로덕션 코드는 아래 한 건을 빼고 건드리지 않았다. 대상과 결과: `pkg/adapter` 68.2%→83.5%(트랜잭션 rollback/journal, prune 자격, symlink 탈출 거절), `pkg/adapter/omp` 84.6%→86.5%(config marker span 병합 fail-closed), `pkg/orchestra` 90.5%→91.3%와 `pkg/promptlayer` 85.4%→87.6%(judge freshness fail-closed, OMP context evidence/attestation 검증), `pkg/worker` 81.6%→85.3%(서비스 lifecycle 전이), `pkg/qa/scenario` 76.8%→93.7%, `pkg/qa/report` 80.0%→89.2%, `internal/cli` 81.0%→82.7%(react/verify/lsp/prompts, worker setup wizard, workflow merge, runtime observe endpoint admission), 그리고 `pkg/spec`·`pkg/pipeline`·`pkg/skillevolve`의 findings 병합·SPEC status 재작성·checkpoint→dashboard 매핑·promotion rollback. 로컬 merged 프로파일 85.8%→87.0%.

  테스트 작성 중 실제 결함 1건을 함께 고쳤다. `pkg/spec`의 인용 소스 추출이 URL을 걸러내려고 `://` 를 검사했지만, 정규식이 스킴을 이미 잘라낸 뒤 실행돼 `https://example.com/docs/guide.go` 가 `/example.com/docs/guide.go` 라는 가짜 소스 경로로 수집됐다. 매치 앞부분으로 스킴을 판정하도록 바꿨다.

- **native 자식 계약을 bundled 에이전트 실측으로 고치고 셸 가정을 정정한다** (2026-09-14): `omp-native-smoke`가 cutover 이후 계속 실패했다. 원인 두 가지를 pinned omp/17.2.7로 실측해 확정했다. ① `omp agents unpack` 기준 bundled `scout`은 `read/grep/glob/web_search/yield`로 **셸이 없고**, `reviewer`는 `bash`와 함께 `spawns: [scout]`을 선언한다. smoke는 모든 자식에게 `bash` 필수·`task` 금지를 요구했는데, 이는 삭제된 프로젝트 정의(`.omp/agents/<role>.md`)의 툴셋이었다. 자식 계약을 에이전트별로 나눠 `scout`은 `{glob,grep,hub,read,web_search,yield}` 필수·`{bash,edit,task,write}` 금지, `reviewer`는 `{bash,glob,grep,hub,read,task,web_search,yield}` 필수·`{edit,write}` 금지로 고쳤다(`hub`는 background 자식에 런타임이 주입하며, `lsp`/`ast_grep`은 런타임 게이트라 필수에서 제외). ② provider의 부모/자식 판별이 "task 도구가 있으면 부모"였다. `reviewer`가 spawn 가능해지면서 자식 요청도 `task`를 싣게 되어 reviewer 자식이 부모 staging으로 오인되고 빈 tool 결과로 stage 1이 거절됐다. 판별을 smoke 프롬프트 토큰 보유 여부로 바꿨다. 실측: pinned 바이너리로 `--- PASS` (receipt loopback 10, hub ops 6, residual 0, external network 0), `TestOMPRules_LiveSessionReceivesEverySourceRule`도 통과.

  셸 가정도 함께 정정했다. `explorer`는 `scout`(bash 없음)으로, `security-auditor`는 `security-reviewer`(bash 없음)로 collapse되는데 두 프롬프트는 테스트 명령과 보안 스캐너 실행을 무조건 지시하고 있었다. 셸이 있는 런타임에서는 실행하고, 없으면 테스트 파일·CI 설정·기존 리포트를 읽어 보고하고 실행은 셸을 가진 에이전트에게 넘기도록 바꿨다.

- **후보 빌드가 자기 표면을 검증하도록 freshness gate를 고친다** (2026-09-14): `auto update`의 freshness gate가 `stableCurrentVersion`을 통해 버전 문자열을 첫 `-`에서 잘라, 소스에서 빌드한 후보 `0.50.109-canary`를 **릴리스 0.50.109로 취급**했다. 그 결과 gate는 "더 새로운 official release(0.50.117)가 있다"고 판정해 릴리스 바이너리를 설치하고 re-exec했고, upgrade canary는 후보가 아니라 **이미 배포된 표면**을 검증했다. 릴리스 표면은 아직 `.omp/agents/*.md` 16개를 생성하므로 native agent cutover 이후의 `assert_absent`가 실패했다. 이제 git-describe 접미사(`-dirty`, `-20-gffda591a`, `-20-gffda591a-dirty`)만 base release로 인정하고, 그 밖의 pre-release 라벨은 stable로 보지 않아 gate가 네트워크 조회 없이 통과한다. 배포되지 않은 빌드에 "최신 릴리스보다 뒤처졌다"는 주장을 할 근거가 없기 때문이다. 실측: 수정 전 canary는 `legacy path remains: .../.omp/agents/executor.md`로 실패, 수정 후 `upgrade-canary: PASS mode=fixture`. 버전 헬퍼는 `internal/cli/update_freshness_version.go`로 분리해 300줄 한도를 유지했다.

- **native cutover가 제거한 표면을 단정하던 live smoke를 정리한다** (2026-09-14): `TestOMPNativeTaskHubLiveSmoke`가 `assertOMPNativeGeneratedAgentsInheritParent`로 `.omp/agents/{explorer,reviewer}.md`의 존재와 parent model 상속을 요구했다. OMP는 이제 bundled agent registry를 쓰고 프로젝트 정의를 생성하지 않으므로(`.omp/agents` 부재는 `omp_native_registry_test.go`가 이미 단정) 이 기대는 삭제한다. 남은 smoke는 실제 native task/hub 왕복을 그대로 검증한다.

- **훅과 dev 설치 경로의 버전 스큐를 드러낸다** (2026-09-14): `make install`의 `$(GOPATH)`는 make 변수로 정의된 적이 없어 `/bin/auto`로 전개됐고, 문서가 안내하는 dev 설치가 SIP에서 `Operation not permitted`로 죽고 있었다. `GOPATH ?= $(shell go env GOPATH)`로 툴체인에 묻고 `INSTALL_DIR`로 대상 위치를 열었다. 설치 후 `PATH`가 다른 `auto`를 먼저 찾으면 그 사실과 `AUTOPUS_BIN` 사용법을 출력한다. 서명된 릴리스 설치본(`~/.local/bin`)은 덮어쓰지 않는다. 생성되는 git 훅은 `AUTO_BIN="${AUTOPUS_BIN:-auto}"`를 앞에 두어 self-hosting 체크아웃이 검증 대상 빌드로 hygiene·Lore 검사를 돌릴 수 있게 했다. 이전에는 훅이 PATH의 옛 릴리스 바이너리로 검사해, 커밋되는 소스가 아닌 바이너리에 대해 판정하고 있었다. 회귀 테스트는 문자열 비교가 아니라 stub 바이너리를 심어 실제로 어느 바이너리가 argv를 받았는지 확인한다. 실측: 설치본 0.50.117(commit 620e29a4)이 HEAD보다 20커밋 뒤처져 `auto sync verify`가 단일 저장소 분류(#188) 없이 실패했고, HEAD 빌드로는 `(no changes)` + blocked-path 경고 17건으로 정상 동작했다.

- **300줄 한도를 선언해 실제 게이트로 만들고 초과 파일 3개를 분할한다** (2026-09-14): `autopus.yaml`에 `architecture.max_file_lines: 300`이 없어 300줄 규칙이 문서 주장일 뿐 `auto check --arch`가 권고로만 보고했다. 한도를 선언하고 초과 파일을 concern 단위로 분할했다 — `internal/cli/pipeline_run.go`(322줄)에서 flag value 검증과 플랫폼 탐지를 `pipeline_run_flags.go`로, `pkg/config/schema.go`(305줄)에서 orchestra·subprocess·provider 타입을 `schema_orchestra.go`로, `pkg/content/hooks.go`(304줄)에서 git 훅 스크립트 생성을 `hooks_git.go`로 옮겼다. 실측: 선언 후 320줄 코드 파일을 심으면 `[ERROR] ... exceeds project limit 300`과 종료 코드 1, 제거하면 0. 주석 전용 줄은 한도 계산에서 제외된다(`countSourceLines`).

- **커버리지 기본 하한 85%를 복원하고 선언값을 실행 경로에 배선한다** (2026-09-14): `workflow.coverage_threshold` 기본값을 `0`(게이트 없음)에서 `85`로 되돌렸다. 기본값 상수는 `config.DefaultCoverageThreshold` 한 곳이 소유하고, `DefaultFullConfig`·`applyMissingDefaults`·CLI fallback이 모두 그 값을 읽는다. 백필은 섹션 단위에서 필드 단위로 바꿨다 — `workflow:` 섹션이 없거나 비었으면 전체 기본값을, 섹션은 있고 `coverage_threshold` 키만 없으면 그 필드만 채운다. 명시한 `0`은 프로젝트의 opt-out이므로 덮어쓰지 않는다. 선언값이 실제로 도달하지 않던 결함도 고쳤다: 비테스트 코드에서 `cfg.Workflow.CoverageThreshold`를 읽는 호출자가 없어 임계값이 runner의 coverage-gap 훅까지 가지 않았고, `internal/cli/pipeline_run_coverage.go`의 `pipelineCoverageThreshold`가 `LoadPreview`로 해석해 `pipeline.RunConfig.CoverageThreshold`에 전달한다(설정을 읽으며 파일을 다시 쓰지 않고, 읽을 수 없으면 기본 하한을 유지해 "판단 불가"를 "게이트 없음"과 구분한다). tdd·review·verification·testing-strategy·ci-cd·agent-pipeline 스킬, tester·validator·reviewer·executor·explorer 에이전트, `references/agent-pipeline/{verification,completion}.md`, Claude `/auto` Gate 3 계약, README 2종의 조건부 문구("게이트를 설정했다면")를 "기본 85%, 프로젝트 선언값 우선, `0`은 해제"로 통일하고 템플릿을 재생성했다. adk 자체 CI 게이트는 `83`을 유지한다(main 실측 83.7%). 검증: `pkg/config`·`pkg/pipeline`·`pkg/content`·`pkg/adapter`·`pkg/workflow`·`internal/cli` 통과, 신규 계약 테스트 10개(기본값·부재 섹션·키 누락 섹션·명시 0·명시 70·설정 파일 부재·CLI resolver 4건).

- **코딩 도구별 모델을 TUI로 직접 지정한다** (2026-09-14): `auto quality`가 먼저 설정 대상을 묻고, 생성 에이전트가 모델을 갖는 도구(Claude Code, Codex, OMP)만 제시한다. 모델을 갖지 않는 Antigravity CLI와 OpenCode는 빼지 않고 세션 모델을 상속한다고 표시한다. Claude Code와 Codex는 에이전트별 티어와 그 티어의 구체 모델을 함께 보여 주고 `agent=tier` 편집을 받아 `quality.presets.<name>` + `quality.providers.<tool>`로 저장하므로 도구별로 다른 모델을 쓸 수 있고 `quality.default`는 그대로 유지된다. OMP는 계열 2택 외에 `custom`을 추가해 설치된 카탈로그에서 기본 에이전트별 모델과 thinking을 직접 고른다. 질문마다 해당 경로가 요구하는 capability를 표시하고, 카탈로그가 그 capability를 선언하지 않은 모델은 즉시 거절해 가능한 모델을 알려 준다. 티어 어휘는 `config.NormalizeQualityTier`/`QualityTiers` 한 곳이 소유하며, 모르는 에이전트·티어 입력은 앞선 편집을 버리지 않고 거절한다. 취소는 autopus.yaml을 바이트 단위로 보존한다. 실측: PTY에서 executor=fable·validator=haiku를 적용한 뒤 `.claude/agents/autopus/executor.md`가 `model: fable`, `validator.md`가 `model: haiku`로 생성되고 Codex executor는 `gpt-5.6-luna`를 유지했다. OMP custom은 실제 카탈로그 모양의 metadata-light 카탈로그에서 다섯 에이전트 모두를 지정해 미리보기까지 통과했다.

- **native 이름으로 지정한 모델 경로를 attestation이 수용한다** (2026-09-14): `role_model_policy.agents`의 기본 에이전트 이름(`task` 등)으로 지정한 경로가 operator-attested 카탈로그 정규화에서 `catalog_invalid`로 거절되던 결함을 고쳤다. capability를 역할 매트릭스에서만 읽던 자리를 공용 resolver로 바꿨다. 이는 네이티브 에이전트 전환이 남긴 경로 불일치이며, metadata-light 설치에서 native 이름 지정이 전부 실패했다.

- **OMP는 자체 기본 에이전트를 재사용한다** (2026-09-13): `.omp/agents/<role>.md` 16개를 더 이상 생성하지 않는다. OMP는 task 에이전트를 이름으로 정확히 찾으므로 같은 이름의 프로젝트 파일은 기본 에이전트를 영구히 가렸다. 16개 ADK 역할은 `task`·`scout`·`reviewer`·`security-reviewer`·`sonic` 다섯 개로 모이고, 모델 지정은 은퇴한 `modelRoles`의 `autopus_*` 별칭 대신 네이티브 `task.agentModelOverrides` 한 키로 옮겼다. 한 기본 에이전트에 여러 역할 경로가 겹치면 대표 역할(`task`←planner, `scout`←explorer, `sonic`←validator)이 결정하며 어느 항목이 적용됐는지 plan·explain·receipt의 policy key로 드러낸다. 16개 역할을 모두 적은 기존 설정은 그대로 동작하고, 대표 역할이 없는 조합만 `omp_native_agent_conflict`로 거절한다. 재생성·업데이트는 manifest에 기록된 옛 정의만 제거하며, 사용자가 수정한 파일은 삭제 전에 백업하고 사용자가 직접 만든 `.omp/agents/*.md`는 건드리지 않는다. 상태 점검은 없는 정의를 정상으로 보고, 기본 이름을 가리는 프로젝트 파일만 `native_agent_shadowed`로 알린다. 실제 OMP 세션 발견 결과는 20개(16개 중복 + 기본 4개)에서 기본 5개로 줄었고, 가려진 `reviewer.md`를 만들면 status가 `degraded shadowed=1`로 보고했다.

- **기본 하네스를 줄이고 필요한 지침만 선택한다** (2026-09-13): 스킬 compiler의 기본값을 기존 `split` 모드로 바꾸고 핵심·필수 참조 스킬과 `/auto` 경로를 배포한다. 전체 라이브러리와 추가 묶음은 명시적으로 선택한다. 파이프라인 상세 지침은 실제 생성·관리되는 `references/` 파일로 옮기고, Codex의 중복 Go 문안과 OMP의 자동 첨부 조정 문안을 제거했다. 루트 문서는 전체 에이전트 목록 대신 네이티브 탐색 경로를 안내하며, OpenCode는 워크플로 전용 규칙을 시작 지침에서 제외하되 파일은 유지한다. 격리된 5플랫폼 생성에서 루트 AGENTS.md는 11,195→4,122바이트, 스킬 진입점은 플랫폼별 70–72→37–39개였다. 이는 생성 파일 측정이며 전체 런타임 토큰 수와 구분한다.

- **저위험 실행에서 불필요한 단계를 생략하되 계약을 재검증한다** (2026-09-13): `auto pipeline run`은 단일 변경 계약과 실제 변경 경로를 재평가해 저위험으로 판정한 경우에만 implement→validate→review를 선택한다. 위조된 위험도, 중복·손상·범위 불일치 계약은 단계를 줄이지 않으며 재개 route가 다르면 실행 전에 거절한다. 테스트 우선 원칙은 구현 단계에 유지한다. 어노테이션은 명시적 opt-in으로 바꾸고 보안·검증·데이터 손실·결정적 오라클은 유지했다. 파일 크기는 `architecture.max_file_lines`의 양수 한도만 강제하며 기본은 권고다. 커버리지 하한은 기본 85%이며 프로젝트·워크플로가 선언한 기준이 우선한다. 일반 context 명령은 선택한 아키텍처 문서만 전달하고, 서명된 OMP canonical 전달·증거 핀은 변경하지 않았다. Dry-run은 실행 완료가 아닌 계획 결과로 표시한다.

- **네이티브 설정과 프로젝트 소유권을 보존한다** (2026-09-13): 유효한 Codex `features.multi_agent`의 명시적 true/false와 주석을 유지한다. Antigravity 생성·업데이트에서 전역 플러그인 설치와 사용하지 않는 명령 복제를 제거하고, 네이티브 플러그인 및 기존 Gemini 경로에 필요한 리소스를 배포한다. OMP 변환은 다른 제품의 이름을 OMP로 바꾸지 않아 비교 문서의 사실관계를 보존한다. 같은 Codex 0.153.4·Astra/max에서 버그 수정 2건을 각각 한 번 비교해 양쪽 모두 통과했으며, 입력 토큰 합계는 297,441→210,180, 소요시간 합계는 141.43→107.17초였다. 소규모 smoke 관측으로, 일반적인 품질·성능 향상을 입증하는 벤치마크는 아니다. 기존 설치본의 직접 수정은 사용자 선택에 따라 재생성하지 않았다.

- **Antigravity 정리에서 사용자 파일과 같은 이벤트의 훅을 보존한다** (2026-09-13): 정리 중 `.gemini/skills` 등의 공유 디렉터리를 통째로 지우던 경로를 manifest 기반 파일 제거로 바꿨다. 같은 이벤트에 추가한 사용자 훅은 실제 생성한 핸들러와 구분하며, 사용자 정의 MCP 설정과 권한은 유지한다. 수정된 관리 파일을 제거할 때는 공용 정리 절차가 원본을 백업한다. 사용자 파일 소실을 재현한 회귀와 반복 업데이트의 훅 중복 방지 회귀를 추가했다. 실제 설치본은 변경하지 않았다.

- **검증된 probe에서 방법 전환 checkpoint를 제한적으로 수용한다** (2026-09-10): 승인된 T0-C 계약에 따라 정확한 OMP 18.1.13 바이너리·B probe·`[remote,snapcompact]` 조합에서만 두 번째 pre-checkpoint를 허용한다. ID replay, 세 번째 pre, post 이후 pre, 변경된 transcript와 조회 중 끼어드는 프레임은 ACK 전에 거부한다. 일반 producer와 미검증 profile은 single-pre를 유지한다. stateful 및 동기 writer 회귀로 정상 교환, 21개 거절 조합, repeated-pre refusal의 unchanged/idle 증명을 검증했다. provider 오류가 gate 진단 접두어를 흉내 내는 경로도 차단하여 로컬 evidence 단계의 숫자·bool 진단만 출력한다. 18.1.13 핀은 이 수정의 무서명 probe 재개용이며 v2 오라클이나 릴리스 승격을 뜻하지 않는다.

- **OMP-007 probe가 반복 pre-checkpoint 경계에서 중단된 근거를 기록한다** (2026-09-10): 두 번째 실제 관측도 primary 호출 5개를 마친 뒤 sequence 6에서 중단됐으며, 보완한 진단은 `pre_ack_out_of_order`와 compaction 경과 시간 61ms를 기록했다. 부분 기록 8개는 cleanup 후 0600 파일로 보존됐다. 이는 단일 pre-checkpoint를 전제한 하네스 계약과의 충돌이지 감축률 0bp나 remote 승격 성공이 아니다. 첫 방법의 내부 실패 사유는 아직 관측되지 않았다. 두 번의 실패 뒤 추가 유료 재시도를 멈추고 임시 probe 핀을 17.2.7로 복원한다. 방법별 checkpoint와 replay를 구분하는 계약 개정·리뷰 전에는 oracle 구현과 릴리스를 진행하지 않는다.

- **SPEC-OMP-007의 계측 전용 probe 경로를 추가한다** (2026-09-09): 명시적 `--probe-dir`에서 primary usage와 compaction 방법·거부·metadata를 기록하고 report·attestation·태그 생성 전에 종료한다. 관측하지 못한 provider 시도별 비용은 `unknown`으로 남긴다. probe 완료 판정은 실제 보존된 40개 call·18개 attempt의 순서·세션·AB/BA 균형·필수 stats를 확인한다. 반출은 UID 종료 확인 후 동일 descriptor로 검증하며, 검증된 레코드를 임시 보관한 뒤 계정·isolation root·sudo 정리가 모두 성공했을 때만 새 retained 파일로 발행한다. 경로 교체, symlink/hardlink, 중복·누락 기록, cleanup 실패를 회귀 검증했다. `advance-omp-pin.sh --probe`는 성능 거부 후보의 관측 준비만 허용한다. 현재 18.1.13 핀은 이 관측 실행을 위한 임시 선택이며 v2 오라클이나 릴리스 승인을 뜻하지 않는다.

- **단일 저장소에서도 `auto sync verify`로 커밋 대상을 분류한다 (#188)** (2026-09-09): `autopus.yaml`이 있는 단일 Git 저장소와 linked worktree에서 제품 파일, 생성·런타임 경로, 안전하게 분류할 수 없는 경로를 구분한다. 멀티 저장소의 Phase A/B 분류는 유지한다. Git 저장소가 아닌 위치와 손상된 meta root는 `unsupported topology`와 종료 코드 2로 알리며, `--strict` 분류 실패의 종료 코드 1과 구분한다. 실제 생성 프로젝트에서 발견한 `.omp/` 및 Claude 권한 기록의 제외 누락도 수정했다. 생성 정책은 두 토폴로지를 설명하고 `auto check --hygiene --staged`가 동등한 대체 검증이 아님을 명시한다.

- **Codex 작업 에이전트 수를 설정하고 요청·관측 상태를 구분한다 (#189)** (2026-09-09): `autopus.yaml`의 `codex.agents.max_concurrent_threads`로 요청할 작업 에이전트 수를 지정한다(기본 4, 범위 1–64, 주 에이전트 제외). 재생성 시 명시한 값을 유지하고 이전 별칭과 feature-table 설정을 문서화된 `[agents] max_concurrent_threads_per_session` 키로 정리한다. 검증하지 않은 버전의 지원 여부를 추정하지 않으며, 문서 기준으로 생성한 경우 그 가정을 표시한다. doctor는 `requested`, `on_disk`, `loaded`, `effective`와 출처를 나누어 보고한다. 디스크 설정만으로 실행 중인 세션의 적용값을 추정하지 않으므로 `loaded`·`effective`는 관측할 수 없을 때 `unknown`이다. 활성 에이전트를 중단하거나 호스트 제한을 우회하지 않는다.

- **omp/18.1.13을 실측하고 핀을 omp/17.2.7로 되돌린다** (2026-09-07): 가드가 요구하던 "standalone cohort"는 저장소에 명령이 없었고 정책 identity가 Go 코드에 핀을 포함해 핀을 옮기지 않으면 측정할 수도 없었다. `advance-omp-pin.sh --measure`로 미측정 버전을 측정 목적으로 옮기고(`--dry-run`은 실행 계약만 검사), 측정 절차가 릴리스 canary 자체임을 runbook에 명시했다. 첫 시도는 42/42 기록 뒤 evidence 단계에서 실패했는데 판정 숫자가 어디에도 남지 않아 provider 호출 40건이 헛돌았다. error frame에 body-free `gate_diagnostic`을 실어 receipt가 한 줄로 출력하도록 고친 뒤 재측정했다. **결과: 같은 workload·같은 모델에서 omp/18.1.13은 compactions=2/2, median_reduction_bp=895/2000.** 18.1.5의 0bp가 upstream v18.1.8 수정으로 895bp가 됐지만 floor의 절반 미만이고 compaction 횟수는 17.2.7의 8회 대비 2회다. 핀은 17.2.7로 되돌렸고 18.1.13은 판정표에 숫자와 함께 거부 행으로 기록했다. 태그나 원격 상태는 만들어지지 않았다.

- **v0.50.117 (A28) 발행 완료** (2026-09-07): release `383826825`를 정식·immutable 상태로 발행했으며 자산 15개를 확인했다. R2 서명 태그 객체는 `edcd4eb98f4e52e8ff868d1f8440cfa0d0c5bc3a`, 소스는 `620e29a44d004cb199d5f1c22ae92878f9b6930e`, tree는 `fce0d047ae8cf5c519762fe4ebf530e7ccc01ed1`다. 실제 검증 cohort는 42/42 기록(omp/17.2.7, openai-codex/gpt-5.6-sol)이며 K3 evidence 태그는 `62fa28194cbe146d2f1377cab9659e2ce3849985`, report SHA256은 `6c7213d0476124fc707d46dcc0b4601084654c28e13df1a62506b871b347efce`, attestation SHA256은 `e0c48a051ce61cf0882c595dd15ac955a2f9c1cf1cf51825f2a9e11e972b7573`이다. 보호된 release run `34082844951`은 태그 CI·보안·증거 검증, 빌드·서명·공증, immutable 자산 검증과 Homebrew Cask 발행까지 전부 통과했다. tap head는 `f6b06e4ad58c`, ruleset `22415096`은 sealed이며 배포 정책은 `59291706`이다. 공개 upgrade canary run `34086372717`은 Darwin/arm64에서 공개 서명 설치본으로 `0.50.116 -> 0.50.117` 업그레이드를 `admitted`로 기록했다. 태그 생성 전 apply 두 번은 go.sum 누락 해시와 고정 OMP 카탈로그에 없는 운영자 모델 때문에 멈췄고, 두 원인 모두 preflight·wrapper 게이트로 고쳤다.

- **SPEC 리뷰 판정에 차단 사유와 루프 상태를 명시하고, 변경 없는 재리뷰를 중단한다** (2026-09-07, #187): REVISE·REJECT 판정은 `blocking_reasons`(finding id, severity, category, policy)를 함께 남기며, 차단 사유가 없는 REVISE는 PASS로 정규화한다. 차단 여부는 `minor` 같은 심각도 표기가 아니라 범주로 정한다. correctness·contract·data_loss·security finding은 심각도와 무관하게 차단하고, style·suggestion·improvement는 critical/major로 올리지 않는 한 advisory다. judge가 PASS로 수용한 finding은 런타임에서도 차단하지 않으며 hard blocker만 사유와 함께 남긴다. SPEC 입력이 직전 revision과 같고 이전 판정이 수정을 요구했다면 provider를 다시 호출하지 않고 `loop_status: awaiting_changes`로 끝낸다. `converged`·`revisions_exhausted`·`provider_unavailable`(quota·timeout)을 구분해 receipt와 CLI 요약에 출력한다. `--single-pass`는 정확히 한 번의 provider round만 실행하고, 명시한 `max_revisions: 0`은 기본값 대신 추가 revision 없음을 뜻한다.

- **저위험 변경은 4문서 대신 짧은 변경 계약이 기본이다** (2026-09-07, #187, #186): 새 `auto spec change <SPEC-ID> --class <class> --ac <id> --surface <path> --verify <plan>`은 기존 SPEC과 인수 조건 id를 참조하는 `change.md` 하나만 기록한다. 존재하지 않는 SPEC이나 AC id는 실제 id 목록과 함께 거절한다. 변경 클래스는 `test_only|docs_only|small_ui|bugfix_existing_contract|feature|multi_domain|security_or_data`이며 파일 수가 아니라 클래스·경로·새 계약 여부로 위험 등급을 정한다. 저위험은 `compact_contract`, security_or_data·multi_domain·새 API/계약이 있는 feature는 `escalate_to_full_spec`로 정식 SPEC과 risk-first probe를 요구한다. 선언한 클래스와 실제 surface가 어긋나면(test_only인데 production 소스 포함 등) 같은 이유로 escalate한다. `auto spec gates --change-class`와 `--json`은 `spec_authoring`·`risk_first_probe`를 포함한 gate별 `required|not_applicable|blocked` 판정과 이유를 출력한다. 보안·데이터 손실·접근성·race/coverage 안전 gate는 어떤 클래스에서도 유지한다. Claude/Codex/Gemini/OMP 파이프라인 문서와 executor·reviewer·spec-writer·planner 에이전트 지침에 같은 규칙과 병렬 작업 후 단일 병합 검증(인수 조건별 증거 기록)을 반영했다.

- **300줄 제한에서 주석 전용 줄을 제외한다** (2026-09-07): 파일 크기 게이트가 물리 줄 수 대신 코드 줄 수를 기준으로 판정한다. 새 `pkg/linecount`는 Chroma 렉서로 28개 확장자의 주석 전용 줄을 식별하며, 문자열·템플릿 리터럴·정규식·heredoc 안의 주석 표시는 내용으로 보고 그대로 센다. 빈 줄과 import, 코드 옆의 후행 주석이 있는 줄은 계속 센다. 렉서 실패나 미지원 확장자는 물리 줄 수로 되돌아가 파일을 실제보다 작게 보고하지 않는다. `auto check --gate`(worktree·staged 모두), 새 `go run ./cmd/source-lines` 게이트, `preflight-release.sh`, release prep hardening test, companionmanifest 소스 크기 테스트와 생성 규칙 문서가 같은 규칙을 적용한다. 물리 줄 수가 200 이하인 파일은 렉싱하지 않아 전체 트리 검사가 0.7초에 끝난다.

- **업데이트 대상과 진행 상태에 OMP를 명확히 표시한다** (2026-09-06): 기존에는 플랫폼 처리가 끝난 뒤의 `omp updated`만 표시해 대상·처리 중 상태를 알기 어려웠다. 이제 `auto update`가 전체 대상 목록을 먼저 보여 주고 각 플랫폼 처리 전에 진행 상태를 출력한다. 워크스페이스 사전 검증도 저장소·플랫폼별 시작을 알리며 OMP는 `OMP (Oh My Pi)`로 표시한다. 실제 업데이트 대상과 완료·실패 판정은 바꾸지 않는다. OpenCode와 OMP가 함께 있는 프로젝트, OMP 업데이트 실패, 쓰기 없는 workspace preflight를 회귀 테스트와 실제 CLI로 확인했다.

- **v0.50.116 (A27) 발행 완료** (2026-09-06): release `383500138`을 정식·immutable 상태로 발행했으며 예상 자산 15개를 확인했다. R2 서명 태그 객체는 `39101f97302267d052ed18b8aafa9ec23278c091`, 소스는 `fbe502c05f84d5eeb81b089b2344c47329ab4543`, tree는 `95ba04d8499b00af86f6794e38a4da1b6d497a6d`다. 실제 검증 cohort는 42/42 기록(20 task pairs, 40 provider calls, 294초), 14/14 gate 통과, token reduction 2335bp, compaction admission 8/20이었다(omp/17.2.7, openai-codex/gpt-5.6-sol). K3 evidence 태그는 `c3666d3f41179e2a8f9ca45fd08a130d64769f70`, report SHA256은 `d0f3cb4777f37a8e83da3f8640adb7338ca5e54ae372eaf15e530af41fa99d0e`, attestation SHA256은 `eed4aab7705dc460d84bee177066d3ab3d773dc09ab0800d201046bf4fddd4f6`이다. 보호된 릴리스 run `34019224937`은 태그 CI·보안·증거 검증, 빌드·서명·공증, immutable 자산 검증과 Homebrew Cask 발행까지 전부 통과했다. tap head는 `6a53f34d00bf53773973ec3925e8179c7f67165b`다. ruleset `22344447`은 sealed이며 배포 정책은 `59198991`이다. 발행 직후 첫 봉인 조회에는 불일치 진단이 있었으나, 원격 `bypass_actors: []`와 승인 전 strict `--sealed` 재검증 통과를 확인했다. 공개 v0.50.115→v0.50.116 canary run `34020839570`은 Darwin/arm64의 실제 릴리스 실행 파일과 공개 서명 installer를 검증하고 `admitted` receipt를 남겼다. 공개 arm64 archive digest는 `sha256:f3c7b2d148b370a7d200a3474ceb342f933c3adead08eca0044f63fbc8ed30bf`다.

- **표준 Balanced 역할 배치를 Claude Code와 Codex 네이티브 에이전트까지 통일한다** (2026-09-06): OMP의 기존 역할표를 재사용해 핵심 7개 역할(planner, architect, spec-writer, reviewer, security-auditor, debugger, deep-worker)은 Claude Fable 5.1/max와 Codex Astra/max로 생성한다. 나머지 Codex 역할은 Luna/max, Claude 구현·테스트는 Sonnet 5/max, 탐색·주석·검증은 Sonnet 5/high다. 기본 balanced 티어와 정확히 일치하는 이전 기본 배치는 새 표준을 따르며, 명시적으로 다른 역할 티어나 사용자 품질 프리셋은 보존한다. Claude YAML의 model/effort와 Codex TOML의 model/model_reasoning_effort를 실제 생성 파일에서 확인했다. Codex는 카탈로그를 읽지 못하면 요청 모델을 유지하고 미검증을 알리며, 관측한 카탈로그에서 모델·추론 강도가 거부되면 파일을 쓰기 전에 차단한다. 네이티브 멀티프로바이더 리뷰 기본값은 품질 모드와 관계없이 Fable 5.1/max와 Astra/max다. 정확한 과거 Claude 기본 argv만 갱신하며 사용자 pin·추가 플래그·backend:omp와 Ultra·주 세션 모델은 보존한다.

- **OMP 모델 설정을 `auto quality` 대화형 메뉴로 단순화한다** (2026-09-06): 긴 `platform omp profile apply` 명령 대신 `auto quality`에서 OMP, balanced/ultra, GPT/Claude를 차례로 고르고 16개 에이전트의 모델·추론 강도 표를 확인한 뒤 적용할 수 있다. `y`/`yes`를 입력하기 전에는 설정을 쓰지 않으며 Enter·`n`·EOF는 취소한다. 기존 프로필 검증과 적용·rollback 경로를 재사용하고, 공통 quality·멀티프로바이더 리뷰·에이전트별 지정값은 유지한다. OMP가 없는 프로젝트의 기존 quality 메뉴와 고급 명령도 유지한다. 실제 PTY에서 GPT 미리보기 취소와 Claude 적용·16역할 readback을 검증했다.

- **OMP readiness의 빠른 종료 시 RPC 응답 누락을 수정한다** (2026-09-06): 원격 CI의 mixed-install 검사에서 provider-free RPC 프레임이 누락됐다. `StdoutPipe`·`StderrPipe`를 직접 읽는 goroutine보다 `cmd.Wait()`가 먼저 끝나 pipe를 닫는 경합을 로컬 회귀 테스트로 재현했다. 빈 응답으로 실패한 실행은 0.04초여서 timeout을 늘려 해결할 문제가 아니었다. 출력 writer를 `exec.Cmd`에 연결해 `Wait`가 복사를 끝낸 뒤 반환하도록 바꿨다. process-group 종료, 출력 제한과 250ms `WaitDelay`는 유지한다. 수정 전 실패한 fast-exit 회귀 테스트는 수정 후 10회 실행의 총 320개 subprocess에서 원본 RPC 응답을 그대로 보존했다.

- **OMP balanced에 GPT·Claude 계열 선택과 에이전트별 모델 지정을 추가한다** (2026-09-06): `auto platform omp profile apply balanced --family gpt|claude`로 모드와 계열을 함께 선택하고, `--plan`으로 16개 에이전트의 요청·실제 모델, 추론 강도, 후보와 차단 사유를 설정 변경 없이 확인한다. `planner`·`architect`·`spec-writer`·`reviewer`·`security-auditor`·`debugger`·`deep-worker`는 GPT형에서 Astra `max`, Claude형에서 Fable 5.1 `max`를 사용한다. GPT형의 나머지는 Luna `max`; Claude형의 구현·테스트 그룹은 Sonnet 5 `max`, 탐색·주석·검증 그룹은 Sonnet 5 `high`다. 일반 reviewer도 선택한 계열을 따르며 별도 멀티프로바이더 리뷰의 모델·judge는 바꾸지 않는다. `--agent executor=openai-codex/gpt-6-astra:max`는 기존 후보 스키마를 재사용하는 `role_model_policy.agents`에 저장하고, `--agent executor=inherit`로 기본 배치에 복귀한다. 내장 프로필을 `profiles` 아래에 복제하지 않으며 명시적인 사용자 정의는 우선한다. 모델·추론 강도가 지원되지 않으면 적용 전에 차단하고, 명시적 fallback 체인이 없으면 native `retry.modelFallback`도 false로 기록한다. 의미 메타데이터가 없는 실제 OMP 카탈로그에서도 배포된 정확한 family 선언과 native selector·thinking의 교집합으로 재정의를 검증한다. `auto quality show`는 OMP 선택을 공통 quality와 별도로 표시한다. OMP ultra·독립 Claude/Codex 정책과 현재 워크스페이스 설정은 유지했다.

- **핀 거부를 판정 표로 바꾸고, doctor가 측정된 감소 판정을 말하게 한다** (2026-09-03): 업스트림이 며칠마다 올라가므로 `18.1.5`를 이름으로 거부한 가드는 확장되지 않았다 - 18.2.0이 그냥 지나쳐 또 cohort를 태울 상태였다. 세 상태를 구분하는 판정 표로 바꿨다: 사용 중(17.2.7), 측정됨·0bp(18.1.2/18.1.5), **미측정(그 외 전부)**. 거부를 해제하는 것은 편집이 아니라 측정이다. 그리고 더 실질적인 격차를 닫았다 - 릴리즈 핀은 **evidence 생산**을 지배하고 런타임은 운영자가 설치한 OMP를 쓴다(`observePipelineOMPVersion`은 자기 receipt와만 비교한다). 이 머신이 실제로 omp/18.1.10을 돌리고 doctor의 모든 capability가 통과하는데 압축 이득은 0으로 측정됐다. 이제 `auto doctor`가 `compaction.measured_reduction: reason=measured_zero_reduction`을 보고한다. 미측정 버전은 실패가 아니라 미지이므로 advisory이고 이유 문자열을 셋으로 구분한다.

- **SPEC 품질 게이트를 receipt 기반 applicability·exact-input 증거 재사용·repeat-discovery 감지·no-capture UX·lead-time telemetry로 완성한다 (#186 C/D/E)** (2026-09-06): 55efaf01의 A(probe gate)에 이어 나머지 제안을 runtime으로 닫는다. (C) `auto spec gates <SPEC> [--changed|--base]`가 변경 집합을 `doc_only|ui_only|security_or_data|multi_domain|general`로 결정적으로 분류하고 13개 gate(`risk_first_probe, build, unit_tests, integration, security, validation, data_loss, deterministic_oracle, accessibility, ux_verification, annotation, provider_review, doc_sync`)마다 `required|reusable|not_applicable|blocked` + 이유를 `{SPEC_DIR}/gate-applicability.json`에 쓴다. 필수 안전 gate 4개는 어떤 class에서도 `not_applicable`이 되지 않고, `accessibility`/`ux_verification`은 분류기가 UI 경로 부재를 판정했을 때만 NA다. `auto spec gates record --gate --status --inputs [--dynamic-deps]`가 입력 closure(정렬된 path+sha256, go.sum 같은 dynamic dep 포함)의 body-free evidence receipt를 남기고, `reusable`은 현재 트리에서 재계산한 closure가 정확히 같고 `status=pass`·`complete`·max-age(168h) 이내일 때만 부여된다 - 한 바이트 변경, dynamic dep 변경, 실패, partial, stale, 부재는 각각 이유를 달아 `required`다(회귀 테스트 matrix). 에이전트는 `reusable`을 스스로 부여하지 않는다. (C) SPEC 리뷰 verify 모드에서 prior checklist에 없지만 정규화 제목 또는 file:line이 이전 finding과 같은 것은 `repeat`으로 분류하고(scope lock 앞), 이전 revision과 SPEC 입력 해시가 같으면 `same_input_rereview`로 표시해 receipt에 `repeat_discoveries`/`repeat_discovery_count`/`same_input_rereview`/`discovery_repeat_detected`를 남긴다. 이 과정에서 `MergeFindingStatuses`가 provider 투표가 없는 finding을 전부 `open`으로 되살려 out_of_scope/deferred를 매 revision 부활시키던 결함과, verify merge가 prior와 내용이 같은 finding을 조용히 버리던 동작을 고쳤다. (D) `verify.capture: no-capture`(기본 `screenshot`, 그 외 값은 거부)를 두면 `pkg/qa/capture` conformance가 `dom_geometry, accessibility_tree, keyboard_navigation, state_transition` 네 oracle을 전부 요구하고 첫 누락을 `missing_no_capture_oracle:<kind>`로 실패시킨다; frontend-specialist/ux-validator/frontend-verify에 `No-Capture Contract` 분기를 넣어 screenshot 없이 UX PASS를 판정한다. (E) telemetry에 `milestone`(`first_vertical_slice`)·`action`(reread/rerun + reason)·`defect`(discovered/fixed phase, files, escaped, repeat)·`gate`·`estimate` 레코드와 phase `--depends-on` DAG를 추가하고, `auto telemetry leadtime [--run] [--baseline] [--json]`이 time-to-first-slice, phase DAG의 wall-clock critical path(병렬 phase 합산 금지), reread/rerun 사유별 횟수, discovery phase별 결함과 파일 수, repeat finding rate, escaped defects, 미해결 안전 gate(기록 없는 필수 gate는 `omitted`로 fail-closed), estimate 대비 실제를 body-free로 보고한다. `--baseline`은 first-slice/completion delta를 내고 escaped defects나 미해결 안전 gate가 늘면 `regression: true`로 exit 1이다. agent-pipeline/OMP/Claude/Codex/Gemini go 표면과 reviewer가 receipt를 소비하도록 갱신했고, plan/spec-writer 표면의 "`reusable` 무효" 문구는 receipt 규칙으로 바꿨다. OMP core skill ratchet은 345→392줄.

- **좌표를 A27 = v0.50.116으로 올린다** (2026-09-06): 발행된 A26(release `383249963`, tag object `058f0fd9`, source `77ae668b`, tree `e353a4bc`)을 상대로 `advance-release-coordinate.sh v0.50.115 A26 v0.50.116 A27`을 돌려 트리거·ref guard·asset 리터럴·ruleset·현재 릴리즈 검증기·Homebrew 브리지·phase resolver를 함께 옮겼다. 이력 파일은 덮지 않고 추가했다: `validate-source.sh`에 `A27_A26_ANCESTOR_SHA`와 `v0.50.116) A27` arm, `verify-public-key-lineage-coordinates.sh`에 A26 전임자 핀 11개(checksums `7b6bf3c5`, 아카이브 4개, darwin manifest 2개, tag object, commit, tree, release id)를 release `383249963` 자산에서 실측해 A27 분기를 더했다. tap 전임자는 `61ce41f9`/Cask blob `8223e90e`로 실측해 옮겼고 Formula는 그대로 동결이다. 지난 runbook이 지시한 대로 `upgrade-canary.yaml` 전임자 블록(release id, tag object, commit, tree, darwin arm64 digest `7765aa3e`)도 실측 표에 넣었다. 진단 라벨 drift도 함께 고쳤다: `verify-current-release.sh`·`build-omp-context-candidate.sh`·`publish-release-coordinates.sh`(tag annotation `A23` 포함)·`verify-release-prep-lock.sh`·`homebrew-formula-bridge-recovery.yaml`이 아직 A23/A24를 말하고 있었고, `preflight-release.sh`의 전임자 기본값은 두 좌표 뒤(`v0.50.113`)에 머물렀으며 마지막 안내는 실제로는 R2 서명되는 태그를 "unsigned"라고 적었다. OMP pin은 `omp/17.2.7` 그대로다.

- **brainstorm/idea provider를 read-only로 격리하고 worktree 변경을 `workspace_mutation_detected`로 막는다 (#108)** (2026-09-06): `auto orchestra brainstorm`(그리고 이를 호출하는 `idea`)의 native subprocess는 provider argv를 그대로 쓰고 `cmd.Dir`도 두지 않아 repo cwd를 상속했다 - Codex 기본값은 `--sandbox workspace-write`였고 pane 경로는 Claude에 `--dangerously-skip-permissions`까지 붙였다. 이제 `plan`에만 있던 read-only argv 투영(Claude `--permission-mode plan --safe-mode`, Codex `--sandbox read-only --ephemeral`, Gemini `--mode plan --sandbox`)을 brainstorm participant와 judge에도 적용하고, `--context`가 없으면 `autopus-brainstorm-*` 임시 디렉터리를 provider cwd로 써서 repo를 노출하지 않으며(`--context`면 repo cwd + read-only), read-only 실행에서는 pane도 bypass 플래그를 붙이지 않는다. 실행 전후 `git status --porcelain -z` 스냅샷을 비교해 (path,status) delta가 있으면 `workspace_mutation_detected: ... (N files): <paths>`로 실패시키고 파일은 되돌리지 않는다(기존 WIP 보존). provider receipt에 `command`/`cwd`/`pid`/`sandbox_mode`/`started_at`/`ended_at`, run receipt에 `workspace{root, snapshot_before/after{entries,sha256}, mutation_detected, changed_files, status}`와 `quorum_usable`을 기록하고 실패 진단 artifact에도 receipt를 내장한다. quorum 메시지의 usable 분자는 결정에 쓴 값(`countConfiguredUsable`)을 그대로 쓴다(Gemini 성공·Codex timeout·judge 성공은 `usable 1/2, required 2`). guard가 완료를 관측해야 하므로 brainstorm은 더 이상 자동 detach하지 않는다. 실제 sandbox 차단은 provider 자체 기능이므로 하네스는 argv와 mutation 감지까지 증명한다. 실행 fixture 테스트는 `TestOrchestraBrainstorm_`으로 `PROCESS_HEAVY_TESTS`에 격리했다.

- **`/auto` 전용 rule 4종을 `skill-scoped`로 재분류해 Claude 상시 컨텍스트에서 뺀다 (#185-A)** (2026-09-06): `spec-quality`·`doc-storage`·`context7-docs`·`techstack-freshness`(합계 38 KB, 보고자 기준 상시 로드의 52%)는 `/auto plan|go|idea|sync` 안에서만 참조되는데 `.claude/rules/autopus/`에 있어 매 세션 주입됐다. frontmatter `skillScoped: true`가 새 class `skill-scoped`를 만든다(우선순위 hook-fired > skill-scoped > paths-scoped > always; `condition`/`globs`/`alwaysApply`와 결합은 거부). 본문은 hook-fired와 같은 `.claude/hooks/autopus/conditional/<name>.md`로 옮기되 manifest/dispatcher 항목은 만들지 않고, Claude 산출 markdown(skills·agents·commands·CLAUDE.md)의 `.claude/rules/autopus/<name>.md` 참조는 `prepareFiles` 마지막 seam 한 곳에서 relocated 경로로 다시 쓴다. `auto update`는 기존 설치의 stale baseline 사본 4개를 prune한다(실측: `.claude/rules/autopus/` 54,930 → 15,757 bytes). `auto rules list`는 `skill-scoped`와 목적지를 보여 준다. 다른 플랫폼 배치는 그대로다(`@import`는 frontmatter를 벗기고 OMP는 `description`만 남긴다).

- **`auto rules fire|sticky`가 git이 아닌 프로젝트에서도 root를 찾고, `fire`도 미해결을 알린다 (#185-B)** (2026-09-06): 두 dispatcher는 `.claude`/`.git` 디렉터리만 root 증거로 받고 `$HOME`은 검사 전에 거부해, `autopus.yaml`만 있는 디렉터리(홈 포함)에서 `sticky`는 `project_root_unresolved`, `fire`는 아무 출력 없이 exit 0으로 끝났다. 이제 regular(non-symlink) `autopus.yaml`을 가진 디렉터리는 `$HOME`이어도 root로 인정한다(`~/.claude` 거부는 유지 - Claude 사용자 설정 디렉터리이지 프로젝트가 아니다). `fire`는 unresolved 시 stderr 한 줄 `conditional-rules project_root_unresolved`를 쓰고 exit 0을 유지하며, root는 있는데 manifest가 없는 경우는 전과 같이 조용하다.

- **`auto doctor`가 PATH의 `auto`가 Autopus Desktop 런처 shim임을 진단하고 관리 바이너리 경로를 안내한다 (#164)** (2026-09-06): Desktop이 `~/.local/bin/auto`를 `--autopus-managed-adk-cli-broker`로 exec하는 셸 shim으로 바꾸고 broker가 `managed_adk_broker_current_slot_rejected`(exit 126)로 거절하면 사용자는 CLI에 닿을 길을 안내받지 못했다. 새 check `doctor.launcher.desktop_shim`은 `exec.LookPath("auto")` 대상의 앞 4 KiB에서 런처 marker를 읽어(실행하지 않음) `[WARN]`으로 shim 경로, `~/Library/Application Support/co.autopus.desktop/managed-adk/current/auto` 존재 여부, 직접 실행/alias 우회, broker 거절 시 복구를 출력한다(advisory - 최종 verdict는 바꾸지 않음; JSON은 `severity: warning`, `status: warn`, envelope warning `desktop_managed_launcher`). README/README.ko에 같은 복구 절차를 적었다. 실제 Go 바이너리나 그 symlink는 shim으로 보지 않는다.

- **plan.md에 Risk-First Integration Probe를 필수화하고 fan-out 전 Phase 1.9 probe gate와 gate applicability 어휘를 넣는다 (#186 전진)** (2026-09-06): 통합 결함이 구현 fan-out 뒤에 늦게 발견되는 문제(제한 권한 API 기동, 브라우저→BFF→API 왕복, 요청 진행 중 logout 같은 상태 경계)에 대해 `auto spec new` scaffold가 `## Risk-First Integration Probe` 표(`assumption_id | class | risk | boundary | input | oracle | isolation | status | reason | evidence`, 1-3행, `class` ∈ `requirement_invariant|implementation_assumption|verified_fact`, `status` ∈ `PASS|FAIL|not-run`)를 만들고 `auto spec validate --strict`가 행 수·status enum·reason 필수·`PASS`의 evidence 필수를 결정적으로 검사한다(`not-run`은 PASS로 승격 불가). spec-writer 계약(3.35)과 spec-quality `Q-COMP-08`이 작성/자체검증을, Claude/Codex/Gemini/OMP의 `/auto go`와 agent-pipeline이 `Phase 1.9: Risk-First Probe Gate`(실행 가능한 `not-run` 행 실행, high/critical FAIL은 1회 재계획 뒤 사용자에게 표면화, `not-run`은 handoff에 한계로 명시)를 Phase 2 앞에 소비한다. 모든 phase gate는 `required | not_applicable | blocked` + 이유를 남기며, exact-input 증거 재사용 엔진이 없으므로 `reusable`은 의도적으로 유효값이 아니다. 필수 안전 gate는 `not_applicable`이 될 수 없고, 참조 소스가 없는 @AX annotation 같은 보조 단계는 `blocked` + fallback으로 보고한다. deterministic `--workflow` Route A/Team 계약은 바꾸지 않았다. 부수 발견: `skillScoped` frontmatter로 spec-quality가 200줄을 넘자 리뷰 프롬프트의 checklist가 200줄에서 head-trim되어 Q-COH-03 FAIL 기준이 조용히 빠졌다 - checklist 예산을 `ResolveAuxTotalBudget`(단일 예산 권위)로 돌려 canonical checklist를 전부 주입한다. 완료 SPEC `SPEC-ORCH-024`는 새 필수 섹션 때문에 `--strict`에서 실패하며 과거 문서는 소급 편집하지 않았다.

- **v0.50.115 (A26) 발행 완료** (2026-09-05): release `383249963`(immutable, 15 assets), tag object `058f0fd9`, source `77ae668b`, evidence tag `omp-context-evidence-v0.50.115`(20/40, 14/14 gate, reduction 2335bp, compaction 8/20, omp/17.2.7, openai-codex), K3 attestation, Homebrew Cask `61ce41f9`, ruleset `22334869` sealed. 기록: `docs/runbooks/release-v0.50.115.md`.

- **좌표를 A26 = v0.50.115로 올린다** (2026-09-05): 발행된 A25(release `382345734`, tag object `7667c522`, source `a6d199fb`, tree `61dad3b6`)를 상대로 `advance-release-coordinate.sh v0.50.114 A25 v0.50.115 A26`을 돌려 트리거·ref guard·asset 리터럴·ruleset·현재 릴리즈 검증기·Homebrew 브리지·phase resolver를 함께 옮겼다. 이력 파일은 덮지 않고 추가했다: `validate-source.sh`에 `A26_A25_ANCESTOR_SHA`와 `v0.50.115) A26` arm, `verify-public-key-lineage-coordinates.sh`에 A25 전임자 핀 11개(checksums, 아카이브 4개, darwin 아카이브 내부 manifest 2개, tag object, commit, tree, release id)를 release `382345734`에서 실측해 A26 분기를 더했다. tap 전임자는 `7ea4a82e`/`01f5123f`로 실측해 옮겼고 Formula는 그대로 동결이다. `upgrade-canary.yaml`의 전임자 tree 핀이 A22의 tree(`79d97a64`)로 두 번의 이동 동안 굳어 있던 것을 A25 tree로 바로잡았다. Go/셸 테스트는 A26을 shipped로, A25는 accumulate 단언으로 남긴다. `release_lineage_phases_test.go`가 300줄을 넘어 좌표 표 대조 테스트를 `release_lineage_phase_table_test.go`로 분리했다. OMP 핀은 `omp/17.2.7` 그대로다.

- **process-heavy flaky의 근본 원인을 고치고 pkg/processprobe를 격리 목록에서 뺀다** (2026-09-05): `go test ./...` 병렬 실행에서만 터지던 `TestOutputLimited_StreamOverflowTerminatesProcessGroup`·`TestOutputSuccessDoesNotTerminateProcessGroup`은 macOS가 처음 실행되는 실행 파일마다 최초 실행 평가(syspolicyd/XProtect)를 시스템 전역으로 직렬화해(파일당 약 190ms) 그 큐 대기가 테스트 시계에 들어가는 것이 원인이었다(K=24 동시 실행 재현: 24/24 red → 0/24). fixture 스크립트를 직접 exec하지 않고 이미 평가된 `/bin/sh`에 넘겨 비용을 없앴다. Makefile에 근본 원인을 적고 두 테스트를 `PROCESS_HEAVY_TESTS`에서 제거했다. fixture 경로를 직접 exec해야 하는 나머지 프로브 테스트는 계속 격리한다.

- **SPEC 리뷰 용어를 도메인 모델 시험 결과대로 정리한다** (2026-09-05): review.md의 judge 블록 라벨을 `**Judge Verdict**`로 바꿔 안전 정규화 뒤의 `**Verdict**`(Review Verdict)와 구분하고, CLI의 receipt 안내를 `Review receipt`로 바꾼다(승격이 막힌 실행에도 남는 receipt이므로). `ProviderStatus`는 reviewer 행만 담고 judge는 별도라는 점, `ExecutedBackend`에 `omp`가 있다는 점, 무결성 검사의 단위가 round가 아니라 revision이라는 점을 주석으로 고정했다.

- **새 스킬 5종을 실제 과제로 시험하고 그 피드백으로 다듬는다** (2026-09-05): grilling(3라운드 8질문 인터뷰), domain-modeling(SPEC 리뷰 도메인 CONTEXT.md 초안, 용어 모순 8건), codebase-design(리뷰 서브시스템 모듈 4개 depth 평가·deepening 후보), debugging(process-heavy flaky 테스트의 원인 — macOS 최초 실행 평가의 전역 직렬 대기 — 를 루프·차등 실험으로 특정), review 두 축(SPEC-OMP-006 diff)을 돌려 얻은 피드백을 반영했다: grilling에 materiality 규칙·한 메시지 라운드·`confirm/adjust/stop` 게이트·`Grilling Summary` handoff·`--auto` skip 표기, domain-modeling에 적용 보류 모드·역할 정의·모순 기록 템플릿·소유 위치 규칙, codebase-design에 암묵 입력 점검표·삭제 테스트 기록·adapter 계수 규칙·위임 불가 fallback, debugging에 flaky 테스트 스트레스 루프·재현율 단위·무수정 관찰·환경 축 최소화·가설별 찬반 증거, review에 `task` 부재 시 부모 위임·범위 우선순위·강제 분류·요구사항별 판정 근거. 두 축 리뷰가 잡은 SPEC-OMP-006 결함 2건(provider 오류 미리보기가 240자를 넘고 다중바이트를 자름; 모델 드리프트 뒤 빈 출력이 드리프트를 가림)도 고쳤다.

- **mattpocock/skills를 참고해 스킬 5개를 추가하고 4개를 보강한다** (2026-09-05): `grilling`(design tree·frontier 라운드 인터뷰), `domain-modeling`(루트 `CONTEXT.md` glossary, ADR 대신 Lore), `codebase-design`(deep module 어휘·삭제 테스트·deepening·design-it-twice), `wait-what`, `resolving-merge-conflicts`를 새로 넣고, `debugging`에 피드백 루프 우선(red-capable·deterministic·fast)·가설 3~5개·`[DEBUG-xxxx]` 계측·정리 규율을, `tdd`에 사전 합의 seam·안티패턴(구현 결합·동어반복·수평 슬라이스)·루프 규칙을, `review`에 Standards/Spec 두 축 병렬 리뷰와 Fowler smell baseline을, `writing-skills`에 에이전트용 문서 작성 원칙(context pointer, 정보 계층, leading word, 가지치기)을 더한다. planner/spec-writer/architect 에이전트가 새 스킬을 참조한다. 출처와 MIT 고지는 `THIRD_PARTY_NOTICES.md`.

- **doctor가 SPEC-OMP-005 role reference(`model: '@autopus_<agent>'`)를 malformed로 오판하지 않는다** (2026-09-05): `.omp/agents/*.md`의 selector 검증이 `@`로 시작하는 role 참조를 `model_malformed`로 실패시켜 워크스페이스 doctor에 16건의 거짓 오류가 났다. role 참조는 `^@[A-Za-z0-9_][A-Za-z0-9_.-]{0,63}$`로 받아들이고 실제 투영은 model-routing doctor가 검증한다.

- **SPEC 리뷰 provider를 OMP RPC read-only 세션으로 실행하고 judge 판정 단계를 추가한다 (SPEC-OMP-006)** (2026-09-05): `orchestra.providers.<name>.backend: omp`는 외부 CLI 대신 private OMP RPC 프로세스(`--no-session --no-extensions --no-skills --no-lsp --tools <allowlist> --config <hardening overlay>`)에서 리뷰를 실행하고, `set_model`/`set_thinking_level`로 모델·thinking을 고정한다. argv `--tools`는 built-in만 제한하므로 세션 `get_state.dumpTools`를 읽어 allowlist 밖 도구(MCP/custom/extension)가 보이면 프롬프트 전송 전에 fail-closed한다. `agent_end.messages`의 assistant `stopReason=error`는 `provider_error`(status·message)로 분류하고 transient status(429/529/5xx)는 같은 pin의 새 프로세스로 최대 3회 재시도하며, 응답 `provider/model`이 pin과 다르면 `provider_model_error`로 거부한다. 리뷰어 3자 합의 뒤 judge(`spec.review_gate.judge`)가 finding을 accept/reject/merge하고 receipt에 `judge` 블록과 provider별 `executed_backend`를 남긴다. `RunOrchestra` 전략 6종도 `ProviderBackends`로 같은 라우팅을 탄다. dogfood: 3 provider `executed_backend=omp`, judge PASS 69/69.

- **project-managed 모드를 떠날 때 `.omp/config.yml`을 ledger preimage로 되돌린다** (2026-09-05): SPEC-OMP-005 실검증에서 hand-written overlay 프로필로 전환하자 관리 키는 `.omp/config.yml`에 남고 ownership ledger만 prune돼, project-managed로 되돌릴 때 `managed_key_conflict: prior fingerprint mismatch`로 막혔다. `migrateOMPLegacyBridgeConfigAt`가 config 파일을 prune 집합에서 항상 빼고 legacy 마커 문서만 처리했기 때문이다. `Update`가 `Clean`과 같은 preimage 복원(`releaseOMPProjectManagedConfigAt`)을 수행해 파일이 원래 없었으면 지우고 있었으면 원본 바이트로 되돌린다. 드리프트한 config는 `Clean`처럼 fail closed한다. 워크스페이스에서 project-managed→overlay→project-managed 왕복이 byte-identical로 돌아오는 것을 확인했다.

- **OMP 모델 라우팅을 에이전트별 커스텀 역할 16개로 바꾼다 (SPEC-OMP-005)** (2026-09-05): SPEC-OMP-003의 native role 접기는 OMP 제약이 아니라 설계 선택이었다 - 실측으로 `--model @autopus_executor`가 `modelRoles` 키만 있으면 해석됨을 확인했다. 접기는 두 부작용을 냈다: debugger/deep-worker(fable)가 `task`를 공유해 executor가 OMP에서만 fable로 승격됐고, `default`/`tiny`/`smol`/`commit`이 프로젝트 관리 대상이 되어 사용자 전역값(백그라운드용 luna 등)을 덮었다. 이제 에이전트마다 `autopus_<agent>` 역할을 투영하고 native 키는 기록하지 않는다. capability 레이어는 family diversity와 hand-written 프로필 기본값으로 남고, 프로필 `agents.<name>.candidates`가 에이전트 전용 후보를 받으며 built-in 파생은 max-wins 없이 에이전트 티어를 그대로 쓴다. native role 상수와 `OMPNativeRoleCapability`는 제거했다. 실측(autopus-workspace): `.omp/config.yml` native 키 0/`autopus_*` 16, 전역 `tiny=luna:max` 복귀, RPC 해석 정확, doctor `supported/fresh`. 역할 16개 직렬 readback이 doctor 20s 데드라인을 넘겨 `projection_mismatch`가 났던 것은 readback을 4-way 병렬로 바꿔 해결했다(`auto update` 22s→12s). 어제 4단 티어 커밋의 "go test ./... 전체 통과" 주장은 출력 절단으로 internal/cli 실패 12건을 놓친 것이었다 - 그 Codex 기대값(orchestra Astra, fable 에이전트 Astra/max)도 이 변경에서 바로잡았다.

- **모델 티어를 4단(`fable > opus > sonnet > haiku`)으로 재정렬하고 OMP built-in 프로필을 혼합 프로바이더로 확장한다** (2026-09-05): Claude Fable 5.1(`claude-fable-5-1`, $10/$50)과 GPT-6 Astra(`gpt-6-astra`)가 나오면서 3단 티어로는 최상위 모델을 표현할 수 없었다. `fable` 티어를 quality 프리셋·Claude effort(`fable`→`max`)·Codex 프로필(`fable`→Astra/`max`, `opus`→Sol/`xhigh`, supervisor/orchestra→Astra)·Gemini 표(`gemini-3.1-pro`/`gemini-3.8-flash`, 2.5 세대 제거)·worker routing 기본값·cost 가격표·workflow 안전목록·`LegacyTierRoute`에 관통시켰다. 기본 프리셋은 ultra=추론 코어 7개 fable + 나머지 opus, balanced=planner/architect/security-auditor fable + 구현·리뷰 opus + 나머지 sonnet이며, Codex ultra의 "전부 Sol, 3개만 max" 특례는 티어가 결정하도록 제거하고 opus 하한만 남겼다. OMP built-in `role_model_policy.profile`은 `family`(anchor: `anthropic`|`openai`)와 `config_mode`(`overlay`|`project-managed`)를 받아 anchor 사다리에서 파생하고 `independent_dissent`(advisor)만 반대 family로 보내며, omp/18.1.10 카탈로그에 family/capabilities 필드가 없어 strict trust로는 apply가 실패하므로 operator-attested·required 라우트로 바뀌었다. 같은 이유로 발견한 버그 하나 - omp/18.1.x `config get`은 `--config` 오버레이를 무시하고 `PI_CONFIG_FILES`만 존중해 activation readback이 `retry.fallbackChains` 불일치로 죽었다. 프로브 프로세스가 `--config` 경로를 환경으로도 전달하도록 고쳤다. 실측: autopus-workspace에서 `auto update`가 `.omp/config.yml`에 modelRoles 10개와 fallback chain 3개를 project-managed로 기록했고 doctor가 `profile=ultra receipt=valid`로 16 역할 전부 supported를 냈다.

- **18.1.5는 측정 가능한 context 감소를 내지 않는다 - 비교 실측으로 확정** (2026-09-03): 같은 플랜 생성기, 같은 20 task pair, 같은 게이트웨이·모델로 OMP 바이너리만 바꿔 연속 실행했다. **17.2.7: 압축 8회, 중위 감소 2000bp 통과. 18.1.5: 압축 2회, 중위 감소 0bp.** 게이트 진단이 정확히 말한다 - `compactions=2/2`로 최소치는 만족했고 pair·AB/BA·integrity·security·quality·fallback·rollback 전부 통과했으며 실패는 `median_reduction_bp=0/2000` 하나다. 이로써 두 선택지가 모두 닫혔다. 워크로드를 무겁게 만드는 길은 죽었다 - 18.1.5가 17.2.7에 8회 압축과 통과 중위값을 주는 **동일 워크로드**를 돌렸다. oracle 완화는 더 나쁘다 - 조금 덜 줄이는 버전을 받는 게 아니라 attested 지표에서 **0을 줄이는** 버전을 받는 것이다. `MinReductionBasisPoints != 2000`이 검증기 하드 상수인 이유가 그것이다. 따라서 하네스 변경이 아니라 업스트림 질문이며, 핀은 omp/17.2.7에 머문다. 게이트 진단을 영구화하고 `omp_context_promotion_report_cohort.go`를 205+102줄로 분할했다.

- **omp/18.1.x의 compaction 거부 클래스를 인식하고, 남은 벽이 oracle임을 밝힌다** (2026-09-03): 고친 진단이 독립 cohort 한 번(provider 호출 6건)으로 답을 냈다 - `error="snapcompact would not reduce context locally."`, `pre_acked=true`, `post_acked=false`. `strings` 비교로 출처도 확정했다: 17.2.7엔 **없고** 18.1.2/18.1.5엔 6건씩 있다. 즉 18.1.x가 거부 클래스를 추가했고, 그것은 프로토콜 위반이 아니라 "이미 최소"라는 뜻이다. 게다가 18.1.x는 pre-compaction 훅을 쏜 **뒤** 이득을 판단하므로 거부가 pre-ACK와 함께 도착해 no-op 분기가 두 번 막혔다. 세 거부 문구를 모두 인식하고 pre-ACK를 허용하도록 고쳤다(no-op 증명은 그대로 - transcript와 idle state를 다시 읽어 불변을 요구한다). 실측 결과 독립 cohort가 6에서 멈추는 대신 **42/42 완주**한다. 그런데 한 단계 뒤 `cohort gates failed`에서 막힌다: 게이트가 `compactionCount >= 2`와 median reduction ≥ floor를 요구하는데, 18.1.x가 모든 압축을 이득 없음으로 거부하면 증명할 감소가 없다. 이것은 고칠 버그가 아니라 제품 경계이므로 SPEC이 필요한 정책 결정으로 남긴다. 적응 자체는 유지한다 - 정당한 거부가 프로토콜 위반으로 읽히면 안 되고, 17.2.7은 새 문구를 내지 않아 동작이 바뀌지 않으며, 실패가 이제 정직한 게이트에 떨어진다.

- **v0.50.114 발행 완료, 릴리즈 워크플로가 처음으로 전부 초록** (2026-09-03): A25 = v0.50.114가 발행됐다 - release `382345734`, 자산 15개, immutable, 태그 객체 `7667c522`, 소스 `a6d199fb`, tap `7ea4a82e`. cohort가 42/42로 통과해 omp/17.2.7에서 reduction floor가 유지됨을 확인했다. v0.50.113은 발행은 정확했지만 Homebrew 읽기 경합으로 run이 빨갰고, 그 재시도 수정이 이번에 효과를 냈다. omp/18.1.5 시도는 좌표가 아니라 25분과 provider 호출 6건을 썼다 - canary가 tagging 전에 돌기 때문이다. 발행된 자산으로 설치 바이너리도 갱신했고 sigstore 검증(`Verified OK`)과 digest 일치를 확인했다.

- **omp/18.1.5를 실측으로 비호환 판정하고 핀을 되돌린다** (2026-09-03): v0.50.114 시도가 로컬 게이트·CI·preflight를 전부 통과한 뒤 cohort 안에서 fail-closed됐다 - `observe-session call 6 failed closed: managed active OMP manual compaction response is invalid`, `transcript_records=7/42`. **태그는 생성되지 않았고 좌표는 살아남았다** - canary가 tagging 전에 돌기 때문에 잘못된 핀의 비용은 좌표가 아니라 25분과 provider 호출 6건이다. 이 실패가 증명한 것: active policy identity의 `snapcompact-image-schema=omp-v17.2.7`은 장식이 아니고 manual compaction 응답 형태가 버전에 결합돼 있다. 18.1.5는 그것을 바꿨는데 `negotiate_protocol`은 여전히 v2를 광고하므로 핸드셰이크 probe로는 볼 수 없다 - manual compaction은 이력이 있는 실제 provider 세션이 필요하다. 핀을 17.2.7로 되돌리고, `advance-omp-pin.sh`가 18.1.5를 관측 근거와 함께 이름으로 거부하게 했다. 경계를 명시했다: **핸드셰이크 probe는 실행 계약을, cohort는 compaction 계약을 증명한다.**

- **좌표를 A25 = v0.50.114로 올리고 이력 파일 분류를 고친다** (2026-09-03): 발행된 A24를 상대로 좌표를 처음 올리면서 `advance-release-coordinate.sh`의 근본 결함이 드러났다 - 이력 파일을 치환 대상에 넣어 **발행된 A24 행을 덮었다**. `produce-public-key-receipt.sh`의 `v0.50.113 0.50.113 A24` 행이 `v0.50.114 0.50.114 A24`가 되고, `validate-source.sh`의 phase arm과 `A24_TAG`도 같은 방식으로 사라졌다. 치환은 **소각 좌표에만** 맞다. 이제 스크립트가 `gh api releases/tags/<FROM>`으로 발행 여부를 실측해 갈라진다: 발행됐으면 이력을 보존하고 새 phase 선언을 손으로 요구하며(전임자 핀은 불변 릴리즈에서 측정해야 하므로), 미발행이면 제자리 이동한다. A24 신뢰 핀 9개(checksums, 아카이브 4, manifest 2, tag object, tree)를 release 381657693에서 실측했고, tap 전임자도 실측한 `166e3efc`/`120056555d09`로 옮겨 렌더가 발행 바이트와 일치함을 확인했다. `pipelineOMPActivePolicyIdentity`가 바뀌어 파생 digest도 재측정했다(`f9eecd3d`).

- **OMP 핀을 omp/18.1.5로 올린다** (2026-09-03): `advance-omp-pin.sh`로 5개 파일을 함께 옮겼다. 후보가 먼저 계약을 증명했다 - digest 실측 `7e6c52be`, 자체 보고 `omp/18.1.5`, RPC 핸드셰이크가 managed protocol v2 광고. 첫 실사용에서 가드 밖에 있던 두 곳이 드러났다: `release-exec-smoke-hardening-test.sh`가 shipped materializer의 URL과 digest를 assert하는데 대상 목록에 없었다(이제 둘 다 추가), 그리고 `execsmoke/verified_exec_test.go`의 픽스처 7곳이 `omp/17.2.7`을 리터럴로 담아 `pinnedOMPVersion`과 어긋났다 - 리터럴 대신 상수에서 만들도록 바꿔 다시 표류하지 않는다. 릴리즈 하드닝 11/11 통과. reduction floor는 아직 미측정이며 `--apply`의 cohort가 판정한다.

- **파싱 계약을 타이밍 계약으로 바꿔놓던 프로브 예산을 고친다** (2026-09-03): `TestLatestCLIContract_MixedInstall/OMP_native_provider-free_surface`가 CI에서만 실패하며 모든 capability를 `response_missing`으로 보고했다. `ProbeOMPReadiness`의 기본 예산 5초는 `auto doctor`의 UX 경계로 타당하지만, 그 테스트는 실제 `/bin/sh` 픽스처를 스폰하고 공유 러너가 포화되면 5초를 넘긴다. 검사 대상은 프레임 파싱과 capability 유도이지 지연 시간이 아니므로 테스트에만 90초를 고정했다. 프로덕션 경계는 그대로다. 형제 테스트는 in-process `Runner`를 주입해 subprocess가 없어 영향이 없다.

- **v0.50.113 발행 완료, Homebrew 발행 후 검증의 경합을 고친다** (2026-09-03): A24 = v0.50.113이 발행됐다 - 자산 15개, immutable, draft 아님, release `381657693`, 태그 객체 `158269c057e3e45f0a2d5353a1fb2992878bc9f3`, 소스 `bc2147a8`. Homebrew tap도 올바르게 발행됐고(`166e3efc`, Cask digest가 발행 자산과 일치) 그런데 릴리즈 job이 빨갛다. 발행 직후 contents API가 이전 blob을 돌려주는 최종 일관성 경합이다 - tap head는 02:01:47, 검증 실패는 02:01:49였다. 발행된 바이트를 검증하는 것이 그 블록의 목적이므로 검사를 약화시키지 않고 읽기를 최대 10회 3초 간격으로 재시도한다. ref head 검사는 이미 우리가 쓴 커밋임을 증명한다.
- **OMP 핀 표류를 가드로 막고 이동을 한 명령으로 만든다** (2026-09-03): oh-my-pi가 며칠 사이 v17.2.7 -> v18.0.11 -> v18.1.2 -> v18.1.4 -> v18.1.5로 올라가는데, 핀은 파생 불가능한 7곳(셸 리터럴 2, 다운로드 URL, staging 경로, execsmoke의 Go 상수, active policy identity의 라벨)에 손으로 박혀 있었고 **합치 가드가 없었다** - 릴리즈 좌표를 태운 것과 같은 구조다. `TestOMPPinAgreesEverywhere`가 7곳이 어긋나는 순간 실패하고, `TestOMPPinDigestIsDeclaredExactlyOnce`가 digest 사본을 금지하며, `advance-omp-pin.sh`가 함께 옮긴다. 스크립트는 후보 바이너리가 계약을 증명하기 전에는 아무것도 옮기지 않는다: 자산 digest 실측, 자체 보고 버전 일치, 그리고 RPC 핸드셰이크가 managed protocol v2를 광고하는지 확인한다(`initializeManaged`가 런타임에 요구하는 것과 같은 조건). v18.1.5 대상 dry-run으로 실측 검증했다. 핀이 실제로 사는 것은 좁다 - 프로토콜 계약은 이미 런타임 협상이 집행하고, 핀은 evidence가 이름 붙일 실행 파일을 릴리즈 전에 선언한다.

- **ETXTBSY 재시도를 호출자 timeout으로 묶는다** (2026-09-02): `TestProbeModelCatalogRetriesTextFileBusy`가 CI에서만 실패했다. 재시도가 5회 × 10ms = 40ms 고정 예산이고 테스트는 실행 파일을 25ms 잡는데, 부하 걸린 러너에서 25ms 해제가 40ms 이후로 밀리면 진다. ETXTBSY는 다른 프로세스가 실행 파일을 쓰기로 여는 동안만 지속되므로 고정 횟수로 예측할 수 없다. 시도 상한을 없애고 이미 존재하는 호출자 timeout이 경계가 되게 했다. 프로덕션에서도 더 정확하다.

- **tip의 CI 실패 2건을 닫는다** (2026-09-02): 새 CI 게이트가 `set -- $listed`로 개수를 셌고 shellcheck가 SC2086으로 3곳을 잡았다. 억제 대신 `read -ra`로 배열을 만들어 인용 없는 확장을 없앴다. SKIP 스캔의 `for name in $(...)`도 `while IFS= read -r`로 바꿨다(SC2013). sticky rule 쌍은 rule 본문 교정으로 4492 -> 4628 바이트로 자랐다. aggregate 상한 6000은 그대로이고 여유 1372 바이트로 온전히 주입되므로, 상한이 아니라 tripwire 핀을 실측값으로 옮기고 SPEC REQ-STICKYRULE-FIRE-05과 research에 재측정을 기록했다.

- **실수로 발행한 진행 중 작업을 커밋에서 되돌린다** (2026-09-02): 앞 커밋에서 `git add -A`를 써서 다른 작업의 미커밋 WIP 65개 파일(agents/templates 콘텐츠, `pkg/config` strict 로더)이 함께 발행됐다. strict 로더는 `LoadPreview`가 미지 키를 거부하게 만드는데, `TestSaveQualityProviderPreservesRawConfig` 등 CLI 계약 3건은 같은 `LoadPreview`로 미지 키(`future_extension`)가 라운드트립에서 보존되기를 요구한다. 두 계약은 같은 함수에서 양립할 수 없고, 이는 오타 안전성과 상위 호환성 사이의 제품 결정이라 소유자에게 남긴다. 해당 65개 파일을 커밋에서 되돌리고 내용은 워킹트리 미커밋 상태로 복원해, 실수 이전 상태로 되돌렸다. 내 변경(`pkg/qa/report/render.go` errcheck, runbook 이어가기)은 그대로 유지한다.

- **runbook을 v0.50.113으로 이어가고 남은 errcheck를 정리한다** (2026-09-02): runbook은 이름을 옮기고 소각 이력을 안에 남기는 기존 규약을 따랐다 - v0.50.111 runbook이 v0.50.110 소각을 담고 있는 것과 같다. `docs/runbooks/release-v0.50.112.md`를 `release-v0.50.113.md`로 옮기고, 소각 배너는 112 객체를 계속 가리키게 두고 나머지 좌표만 113으로 옮겼다. CI lint가 `pkg/qa/report/render.go`의 `os.Remove` 3건을 잡았고 `sanitize.go`와 같은 방식으로 명시적으로 버렸다. 저장소 전체의 `os.Remove` 호출을 전수 확인해 남은 곳이 없음을 확인했다.

- **A24 좌표를 v0.50.113으로 확정하고 좌표 이동의 빈틈을 메운다** (2026-09-02): 소각된 v0.50.112 대신 A24 = v0.50.113을 phase 표에 넣었다. 소각 좌표는 표에서 생략하는 것이 기존 규약이다 - v0.50.110, v0.50.75~76이 그렇게 부재한다. 이동 과정에서 좌표를 담고도 이동 대상 목록에 없던 파일 4개를 찾았다: `validate-source.sh`의 phase case arm, `verify-public-key-lineage-coordinates.sh`의 `A24_TAG` 쌍, `produce-public-key-receipt.sh`의 표 행, `verify-current-release-signatures.sh`의 `COSIGN_IDENTITY`. 넷 다 이제 `advance-release-coordinate.sh`의 대상이다. `validate-source.sh`의 annotated-tag 게이트는 A2~A23만 열거해 A24를 검사하지 않았고, 그 상태에서 unsigned 태그와 cross-tag replay가 통과했다. lineage 검증기는 A24를 일반 경로로 보내 A23이 발행하지 않은 manifest 자산 핀을 요구했는데, manifest는 두 darwin 아카이브 안에 담겨 있으므로 그 digest를 실측해 핀으로 넣었다(`7c60a437...`, `b2a6c5c9...`). upgrade-canary는 후보만 113으로 옮기고 전임자를 A22/v0.50.109에 남겨, 검증하면 A23을 건너뛰는 업그레이드를 승인할 상태였다 - 전임자를 A23/v0.50.111로 이동했다. `release_lineage_test.go`가 302줄이 되어 lineage 검증기 phase 매트릭스를 `release_lineage_phase_matrix_test.go`로 분리했다(238 + 78줄). 소각 좌표는 이제 거부 대상으로 테스트한다(`burned_A24_tag_112`).

- **v0.50.112 소각과 preflight의 CI 게이트 추가** (2026-09-02): `--apply`가 완주해 태그와 evidence를 만들고 푸시했지만, `release` job이 skipped됐다 - `needs: [ci, security, omp-production-evidence]`에서 CI가 실패했기 때문이다. 릴리즈 `381031316`은 자산 0개 draft로 남았고 아무 소비자도 부분 릴리즈를 보지 않았다. 잃은 것은 좌표다. 이유는 단순하다: CI가 그날 main에서 **모든 실행 실패**했는데 preflight가 CI를 보지 않았다. Makefile 레인, 릴리즈 스크립트 테스트 11개, 하드닝 스위트가 모두 초록이었고 그게 격차를 안 보이게 했다 - 그것들은 게이트가 아니다. 이제 `preflight-release.sh`가 릴리즈 커밋에서 CI가 초록이 아니면 거부한다. 함께 CI 실패 일부를 고쳤다: `pkg/qa/capture/sanitize.go`의 errcheck 3건(정리 실패가 더 중요한 오류를 대체하지 않도록 명시적으로 버림), `pkg/qa/scenario`의 미사용 상수 1건(같은 파일 헬퍼와 중복), 그리고 운영 도구 5개를 `scripts/release-tools/`로 이동 - `scripts/companion-release/*.sh` 전체를 production evidence 코드로 보고 `testdata/` 언급을 금지하는 보안 테스트가 있어서, 같은 디렉터리에 둔 유지보수 도구가 그 가드를 건드렸다. 도구를 옮기는 것이 가드를 약화시키는 것보다 옳다.

- **OMP oracle 핀을 17.2.7로 되돌리고 managed RPC 실패를 진단 가능하게 만든다** (2026-09-02): `--apply`가 라이브 canary에서 Darwin verified-exec 게이트에서 멈췄다 - `observe managed active OMP exec stop / context deadline exceeded`, 42개 중 1번째 레코드. 버전 상향을 의심했고 **틀렸다**: 실제 embedded bridge로 18.1.2를 격리 밖에서 띄우니 `{"type":"ready","protocolVersion":1,...}`을 내고 rc=0이었다. 하지만 버전 상향이 깨뜨리는 것은 더 미묘하고 나쁘다. `pipelineOMPActivePolicyIdentity`에 `snapcompact-image-schema=omp-v17.2.7`이 박혀 있고 그 문자열이 promotion evidence에 기록된다. ADK는 compaction 프레임 형태(`auto_compaction_end`, `action=snapcompact`)도 단정한다. 즉 **oracle 버전은 올릴 digest가 아니라 evidence가 주장하는 프로토콜 계약**이다. 18.1.2 바이너리를 돌리면서 identity가 v17.2.7이라고 말하면, 실행이 통과하든 말든 사실이 아닌 주장을 발행한다. 그래서 계약이 명시한 버전으로 되돌렸다. 18.x 상향은 별도 작업이다 - compaction image schema 검증, policy identity 갱신, 그리고 ptrace exec stop 두 번에 5초를 주는 게이트 재측정. 함께 관측성 결함도 고쳤다: managed RPC 자식의 stderr가 `io.Discard`로 버려져 시작 실패에 원인 텍스트가 없었다. 이제 4KiB 상한으로 담아 두 실패 경로의 메시지에 마스킹해 싣는다 - 자식 환경에 provider 토큰이 있으므로 JSON 출력과 같은 방식으로 마스킹한다. preflight는 어떤 종류의 상류 진술로 핀을 대조했는지 함께 말한다 - v17.2.7은 `SHA256SUMS.txt`가 없는 시절이라 자산을 직접 측정한다.

- **OMP auth broker와 gateway를 상주 launchd agent로 설치한다** (2026-09-02): 릴리즈마다 gateway를 손으로 띄우는 것은 잊히는 단계이고, 실패가 절차 시작이 아니라 prep 깊은 곳에서 드러났다. `install-auth-services.sh`가 둘을 launchd user agent로 등록한다 - `KeepAlive`로 재부팅과 크래시를 견디며, gateway를 강제 종료해 검증했다(새 pid로 재기동, `ready: true` 복귀). 두 가지를 의도적으로 정했다. 서비스는 **설치된 omp**로 돌린다 - gateway는 credential 프록시이고 evidence oracle은 prep이 자기 핀으로 하는 별도 호출이므로, 상주 서비스를 릴리즈 핀에 묶으면 핀이 움직일 때마다 재설치해야 한다. 그리고 둘 다 loopback 전용에 bearer 인증을 유지한다 - gateway는 토큰 없이 401을 낸다. broker가 구독 OAuth를 담으므로 이 머신에서만 닿고 다른 곳에서는 닿지 않는다. preflight가 상주 여부와 credential 출처를 함께 보고하며, gateway가 ready면 export할 것이 없다고 알린다.

- **릴리즈 prep이 OMP auth-gateway에서 구독 자격을 스스로 얻는다** (2026-09-02): 릴리즈마다 provider credential을 손으로 export해야 했다. 절차가 요구하는 "승인된 loopback gateway"가 무엇인지 추적했더니 OMP 자신의 것이었다 - `omp auth-gateway`는 credential vault인 `omp auth-broker`를 뒤에 두는 loopback forward proxy이고, 구독 계정은 이미 그 vault에 있다. 두 서비스를 한 번 띄우면 `release-prep.sh --preflight`가 인자도 env 파일도 없이 전부 해소한다 - 실측으로 broker ready(로컬 vault credential 69개 중 4개 사용 가능), gateway `ready: true`·`brokerAuthenticated: true`, 래퍼가 `via omp auth-gateway with provider=openai-codex model=gpt-5.6-sol omp=omp/18.1.2`를 보고하고 prep의 자체 검사까지 도달. ADK가 나르는 것은 gateway의 inbound bearer token이고 상류 provider 비밀이 아니다 - 상류는 broker에 남고 릴리즈 프로세스에 들어오지 않으므로 provider key를 export하는 것보다 노출이 적다. credential을 플래그로 옮기지 않은 이유는 `observe_session_evidence.go`가 credential·endpoint·경로가 발행 evidence에 없는지 검사하고, argv는 프로세스 목록과 셸 히스토리에 남기 때문이다. sentinel 실행으로 토큰이 출력에 없음을 확인했다. gateway 없는 환경을 위해 0600 env 파일 경로도 유지하며, 두 경로가 같은 형태 검사로 수렴한다.

- **R2 태그 서명 폐지를 철회한다 - 키는 파기되지 않았다** (2026-09-02): 릴리즈 태그 서명키가 파기됐다는 보고를 받고 서명 전제조건을 제거하고 태그를 서명 없는 annotated로 바꿨다. 그 뒤 로컬을 찾아보니 `~/.config/autopus/release-keys/`에 **전부 있었다** - R2(지문 `SHA256:7FISPX…` 정확 일치, 암호 없이 읽힘), K3 promotion(공개 반쪽 일치), K1·K2 ECDSA(public.pem SPKI가 컴파일된 핀과 일치), 그리고 채널 키 A0의 암호화 백업(ceremony 노트에 기록된 sha256과 일치, passphrase는 macOS 키체인 service `autopus-adk-rotation-2026-q3-r2-k3`). ceremony 노트가 전체 집합을 문서화하고 있었고 키들 옆에 있었다. 변경을 되돌렸다: `verify_tag_signing_authority`, `--tag-signing-key`, publisher의 R2 유도, `git tag -s`, `verify_remote_release`의 서명 검증이 모두 복원됐고 A24도 전임자들과 함께 서명 목록에 들어갔다. 테스트는 이제 커밋된 태그가 서명을 **담고 있음**을 검증한다 - 키가 없다고 믿던 동안의 assertion과 정반대다. 교훈은 "사용자가 틀렸다"가 아니다. 키 보관 상태는 사람이 기억으로 들고 있을 종류가 아니고, "키가 없어졌나"의 정직한 답은 처음부터 파일시스템 검색이었다. 먼저 찾았으면 명령 하나로 끝났고 거짓 전제 위에서 릴리즈 절차를 바꾸지 않았을 것이다. `preflight-release.sh`가 이제 기본 키 저장소를 먼저 들여다본다.

- **릴리즈 evidence oracle을 omp/17.2.7에서 18.1.2로 올린다** (2026-09-02): 하네스는 2026-08-26에 자기 표면을 OMP 18.x로 옮겼는데 릴리즈 oracle은 17.2.7에 남아 있었다. `prepare-release.sh`가 정확한 digest로 고정하므로 prep은 시작조차 못 했다. digest는 로컬 설치본이 아니라 **상류 발행 자산**에서 취했다 - 이게 여기서 중요한 구분이다. prep은 `github.com/can1357/oh-my-pi`가 발행한 `omp-darwin-arm64`를 소비하고, 같은 버전의 Homebrew 빌드는 다른 해시를 갖는다. 로컬에서 측정한 digest를 박으면 CI가 절대 내려받지 않는 것을 고정하게 된다. 세 방법으로 검증했다: 상류 `SHA256SUMS.txt` 항목, 내려받은 자산의 측정값, 바이너리가 보고하는 `omp/18.1.2`. 해결되지 않은 것은 oracle 동등성이다 - 40-call cohort의 판정은 oracle 동작에 의존하므로 18.1.2로 서명된 report는 17.2.7의 것과 교환 가능하지 않다. 첫 A24 evidence는 A23과 비슷할 것이라 가정하지 말고 읽어야 하며 reduction basis point 여유가 움직일 것을 예상해야 한다. `preflight-release.sh`가 핀을 상류 자산과 대조하고 상류가 앞서 나가면 경고한다 - 자동 상향이 아니라 경고인 이유는 oracle을 올리는 것이 매번 결정이기 때문이다.

- **릴리즈 좌표 이동을 스크립트로 만들고 preflight를 추가한다** (2026-09-02): v0.50.112로 올리려니 좌표가 한 상수가 아니라 workflow trigger, ref guard, 자산 이름 리터럴, ruleset gate, 현재 릴리즈 검증기, Homebrew bridge, phase resolver, lineage 좌표, 그리고 테스트 픽스처까지 **19개 파일**에 흩어져 있었다. `release-hardening-test.sh`가 그것들을 함께 지키므로 하나를 놓치면 빨간 테스트가 되지만, 누군가 알아챈 뒤의 일이다. `advance-release-coordinate.sh`가 tag/version 치환과 receipt 테이블 append를 수행하고, phase 라벨은 **의도적으로 치환하지 않고 검토용으로 보고**한다 - 무딘 A23→A24 일괄 치환을 시도했더니 전임자 참조와 "이전 리터럴은 A23이었다"는 역사 주석, 그리고 A23 행이 살아남았는지 확인하는 accumulate assertion까지 바꿔놨다. 세 종류를 사람이 구분해야 한다. `preflight-release.sh`는 비밀이나 라이브 provider 없이 검증 가능한 모든 것을 한 명령으로 확인한다 - 도구 유무, 릴리즈 actor 신원, 트리 청결, 태그 부재, content gate, phase 등록, ancestor 상수, ruleset이 open 상태인지, companion 유효 창, 전임자 발행 상태와 자산 형태, sigstore keyless 검증, K1 봉투 존재, 릴리즈 스크립트 문법과 테스트 11개. 비싼 단계가 마지막에 오는 절차에서 잘못된 상수는 태그를 태운 뒤에야 드러났다. 이동 중 실측으로 두 가지를 고쳤다: Homebrew `PRIOR_TAP_COMMIT`/`PRIOR_CASK_BLOB`이 v0.50.111 발행으로 낡아 있었고(실제 tap head를 읽어 갱신, 렌더한 Cask가 실제 blob과 바이트 일치함을 확인), lineage 좌표에 A24 분기가 없어 `prior_release_identity_mismatch`로 거부됐을 것이다(A23 발행 핀 9개를 release 379595447에서 읽어 고정).

- **R2 태그 서명을 기록과 함께 폐지하고 v0.50.112를 A24로 등록한다** (2026-09-02): 릴리즈 태그 서명키 R2와 그것을 교체할 수 있었던 채널 키가 모두 파기됐다. 회전 권한이 태그 키가 아니라 채널 키인 이유가 잃어버린 태그 키를 교체하기 위함인데, 채널 키까지 없으면 새 키를 승인할 것이 남지 않는다. 먼저 각 키의 실제 보관처를 확인했더니 릴리즈에 필요한 나머지는 전부 GitHub Actions secret이었고 지금도 존재한다 - `ADK_COMPANION_ED25519_PRIVATE_KEY`(lineage receipt), `OMP_CONTEXT_EVIDENCE_SIGNING_KEY`(promotion 증거), `ADK_RELEASE_ECDSA_PRIVATE_KEY`(K1, self-update가 검증하는 봉투). 발행된 v0.50.111 봉투의 유일한 지문이 K1과 일치함으로 확인했고, sigstore keyless도 `cosign verify-blob`으로 Verified OK를 받았다. 소비자는 태그 서명을 검증하지 않는다 - `pkg/selfupdate`는 컴파일된 ECDSA 앵커를, Homebrew는 아카이브 digest를 본다. 그래서 잃은 것은 감사 링크 하나다. 존재할 수 없는 키를 요구하는 검사를 제거했다: `verify_tag_signing_authority` 삭제, `--tag-signing-key` 제거, publisher 인자 13→12, `git tag -s`→`git tag -a`, `git verify-tag`는 annotated 태그 객체와 대상 커밋 확인으로 대체. R2 핀 파일은 트리에 남긴다 - 발행된 v0.50.109 회전 sidecar 검증에 여전히 필요하고 그건 불변 이력이다. 채널 공개키 변수와 권한 문서를 admin으로 고쳐 새 루트를 심는 지름길은 명시적으로 기각했다: 저장소 접근만으로 릴리즈를 발행할 수 없다는 속성이 이 장치가 사는 이유이고, 기능 릴리즈를 풀려고 그것을 쓰면 쓴 흔적조차 남지 않는다. 함께 발견한 숨은 차단 하나도 닫았다 - `validate-source.sh`의 태그→phase 맵이 fail-closed라 v0.50.112가 "frozen policy 밖"으로 거부됐을 것이다. A24로 등록하고 ancestor를 v0.50.111 source commit으로 고정했으며, A23 fallback 분기를 나눠 각 phase가 자기 ancestor를 검사하게 했다. 태그 메시지 리터럴이 `A23 companion release`로 하드코딩돼 있어 이후 모든 릴리즈가 잘못된 phase를 달 것이었던 결함도 함께 고쳤다 - phase를 추측하지 않고 생략한다.

- **팩이 정규식 인자를 쓸 수 있고 재현 명령이 붙여넣기 가능하다** (2026-09-01): `auto qa release`를 이 저장소 자신에게 돌리니 fast lane이 10분에 타임아웃했다. 팩은 `go test ./...`를 skip 없이 돌렸는데, Makefile이 그 suite를 25m/10m 두 레인과 `PROCESS_HEAVY_TESTS`로 나눈 이유를 무시한 것이다. 측정값은 skip 레인만 856s와 1092s였으므로 600s 예산으로는 타임아웃 외의 결과를 낼 수 없었다. 그런데 팩에 skip 정규식을 쓰려 하니 `command.argv contains shell metacharacters`로 거부됐다 — `|`와 `$`가 금지 목록에 있었다. argv는 셸 없이 exec되므로(`pkg/qa/run/command.go:93`) 이 바이트는 해석되지 않지만, 가드가 과잉이 아니었다: `Command: strings.Join(args, " ")`가 그대로 manifest의 `reproduction_command`가 되고, 그 문자열은 사람이 셸에 붙여넣는 용도다. 정규식을 그냥 허용하면 재현 명령이 조용히 파이프라인이 되어 하네스가 실행한 적 없는 것을 실행한다. 그래서 순서를 바꿨다: 먼저 POSIX 인용 렌더러를 넣고(`renderReproductionCommand`), 그다음 argv 가드를 실제로 위험한 것으로 좁혔다 — 셸 호출 금지는 유지, 셸 메타문자 대신 제어문자(`\x00-\x1f\x7f`) 거부. 로그와 manifest 필드에 줄을 위조하거나 값을 절단하는 바이트는 여전히 막힌다. `command.run`은 진짜 셸 문자열이므로 전체 금지를 유지한다. 테스트는 텍스트가 아니라 동작을 본다: 렌더된 줄을 `/bin/sh`로 파싱해 원본 argv가 복원되는지 확인하고, naive join은 복원되지 않음을 대조로 증명한다.

- **부하에서 무작위로 릴리즈 게이트를 막던 pane 테스트** (2026-09-01): `TestExecute_ExportsSessionEnvWhenHookMode`가 `sent commands: []`로 실패했다. 격리 실행은 3/3 통과. 원인은 `Timeout: 1 * time.Second`였다 — FileIPC 대기를 끊으려고 둔 값인데 pane 설정과도 경쟁해서, 병렬 부하가 걸리면 Execute가 export 지점에 닿기 전에 만료됐다. 벽시계로 Execute를 가두면 어서션이 스케줄러와 경쟁한다. 이제 Execute를 goroutine에서 돌리고 export가 관측되면 context를 취소한다. 대기는 취소로 끊기므로 timeout은 넉넉해도 되고, 판정은 머신 속도와 무관해진다. 3회 실행이 3.58s에서 0.93s로 줄었다 — 매 실행 1초를 기다리지 않기 때문이다. mock의 `commands`를 락 없이 읽던 것도 `commandsSnapshot()`으로 바꿨다: Execute가 도는 중에 읽으려면 필요하다.

- **`desktop-native` lane이 3자 앱을 실제로 관찰하고 통과한다** (2026-09-01): SPEC-QAMESH-013. 두 provider adapter 모두 fixture 적합성 검사였다 — Orca 경로는 정확히 14줄인 fixture 트리를 검증해 boolean 하나만 뽑고 고정 3노드 projection을 반환했고, 그 UI를 만드는 앱은 저장소 테스트 헬퍼 밖에 없었다. pin이 네 겹이었고(journey 검증, publication provenance, provider addressing, receipt scope) 한 겹을 벗길 때마다 그 아래가 처음 도달 가능해져 다음 겹이 드러났다. 이제 팩이 `provider_app_id`로 관찰 대상을 지정하고, 하네스가 provider의 실제 트리를 파싱해 projection을 만든다. 핵심 설계는 "완전히 관찰하고 선택적으로 공개"다: 실제 트리에는 사용자 데이터가 있으므로(Slack 채널·메시지, Finder 폴더명) publish되는 projection은 팩이 `required_landmarks`로 선언한 이름만 담고 나머지는 개수만 센다. 기존 name allowlist와 8KiB 한도, publication scan이 1급 pin이 아니라 프라이버시 경계였다. 앱/창 신원은 로케일에 안정적인 헤더 두 줄에서만 얻는다 — role 구문은 OS 로케일로 나오므로(Finder `표준 윈도우` vs 웹뷰 `standard window`) 문자열 매칭으로 AXRole을 추론할 수 없다. reason 분류에 `declared_landmark_not_found`와 `observed_tree_bound_exceeded`를 추가해 이전에 `provider_unavailable`로 뭉뚱그려졌던 두 조건이 이름으로 보고된다. 실측 검증: Slack(378노드, 22KB, 우리 것 아님)에서 lane 통과 — projection에 선언한 landmark 2개만, 관찰된 미선언 토큰 311개 중 누출 0건, `provider_app_id`도 증거에 없음. 라이브 검증에서 스키마가 REQ-4가 선택하려는 landmark를 금지하고 있었음도 드러났다 — 정확히 2개만 허용해서 window 안쪽을 선언할 수 없었고, deeper-landmark 경로가 어떤 유효한 팩에서도 도달 불가였다. canonical 쌍은 필수·위치 고정으로 두고 추가 landmark를 허용하되 관찰 가능한 상태(`enabled`/`focused`/`selected`/`expanded`)만 받고 총 24개로 제한했다. 3계층 landmark가 Slack에서 라이브 통과한다. 실측이 설계를 여섯 번 교정했다: "한 행 = 한 노드"(Finder의 accessibility action 값에 개행), 문자 수를 바이트로 오기, `unicode.IsPrint`가 실제 창 제목의 U+00A0 거부, node bound 256이 실제 앱에 과소(Slack 378), provider가 포커스 없는 앱에 다른 문장(`No UI element is currently focused.`)을 냄, landmark 정확히 2개 제한이 REQ-4를 도달 불가로 만듦.

- **아무것도 실행하지 않은 run은 `passed`가 아니다** (2026-09-01): 미검증 lane을 완주시키려고 완전한 `.autopus/qa/mobile/readiness.yaml`을 작성했더니 `mobile-readiness` lane이 `status: passed`, exit 0을 반환했다 — adapter 0개, check 0개, manifest 0개로. `Assess`는 선언형이라 readiness gap이 모두 해소됐고, 그 lane을 선언하는 Journey Pack이 없어 실행할 것이 없었는데, `aggregateStatus`는 실패도 gap도 없으면 `passed`를 돌려줬다. 증거 없는 green lane은 선언된 setup gap보다 나쁘다 — 하류에서 둘을 구별할 방법이 없다. 이제 실행이 비면 `missing_journey_pack: no Journey Pack declares lane "<lane>"; nothing was executed` gap을 기록하고, 상태는 기존 규칙에 따라 `warning`이 된다. 실제 pack이 있는 lane은 무영향(`fast`: adapter 1개, gap 0개, `passed`). release-readiness의 동일 부류(0개 row에 `passed`)는 같은 세션에서 fail-closed로 고쳤고, 이건 run 레이어에 남아 있던 쪽이다.

- **desktop-native lane을 3자 프로젝트가 실제로 publish할 수 있게 한다** (2026-09-01): app-identity pin을 제거해 팩이 validate되기 시작하자, 이전에는 도달조차 못 했던 publication 경로가 드러났고 거기서 막혔다. 두 가지였다. (1) `WriteFinalManifest` 오류가 버려지고 `desktop observation publication failed`만 남아 원인을 알 수 없었다 — canary의 빈 `H1 failed:`와 같은 부류라 사유를 실어 보낸다. (2) 드러난 진짜 원인: publication 계약이 프로젝트가 선언한 `source_refs`를 Autopus 자신의 상수(`SPEC-QAMESH-012`, `AC-QAMESH12-NNN`)와 일치하도록 요구하고 `owned_paths`가 비어 있어야 했다. 3자 프로젝트가 남의 SPEC id를 자기 팩에 타이핑해야 통과하는 구조로, GUI artifact kind에서 이미 한 번 제거한 magic-constant 패턴이다. 이 evidence의 producer는 프로젝트가 아니라 하네스이므로 provenance도 하네스가 설정한다(`DesktopObservationProvenance()`가 producer와 validator의 단일 출처). 실측: 자체 `SPEC-MYAPP-014`/`AC-MYAPP-007`/`owned_paths`를 선언한 팩이 이제 manifest를 발행하고 실제 런타임 사유를 보고한다.

- **선언한 user scenario를 runner spec으로 컴파일하는 `auto qa scenario`** (2026-09-01): 하네스는 실행·증거·게이트는 완주했지만 시나리오 테스트케이스를 만드는 경로가 없었다 — GUI 케이스는 프로젝트가 Playwright spec을 직접 써야 했고, `scenarios.md`와 `qamesh-check`는 "명령 + exit code/문자열 기대"까지만 표현할 수 있어 사용자 인터랙션 단계를 담지 못했다. `.autopus/qa/scenarios/*.yaml`(`qamesh.scenario.v1`)이 그 격차를 메운다: 프로젝트가 화면과 기대를 선언하고, `auto qa scenario compile`이 프로젝트 Playwright `testDir` 아래 `autopus-generated/<id>.spec.ts`로 렌더링한다. 하네스는 assertion을 발명하지 않고 선언을 runner 방언으로 번역만 한다. step 어휘는 읽기 전용으로 닫혀 있다(`expect_title`/`expect_url`/`expect_text`/`expect_role`/`expect_count`) — `click`/`fill`/`press`가 스키마 필드로 존재하지 않으므로 컴파일된 spec은 `gui.forbidden_actions` guard를 트립시킬 수 없다. 누락이 아니라 안전 속성이다. element는 pack의 `selector_strategy: role-first`와 맞추려 ARIA role로만 지정하고, 알 수 없는 key나 role은 파일 전체를 거부한다. `origin`을 생략하면 named Journey Pack의 `allowed_origins[0]`을 상속하므로 컴파일된 spec은 guard가 이미 허용한 origin으로만 이동한다. 실측 검증: 시나리오 1개(화면 2개, step 7개) → 생성된 spec → 실제 Playwright 실행 → `gui-explore` 4개 check 전부 통과 → 리포트 필름스트립에 `catalog`/`checkout` 스크린샷 렌더. fail-closed 7종(unknown key, `click` 시도, 비-ARIA role, 중복 assertion, path traversal, unknown journey, wrong schema_version) 전부 actionable 메시지로 거부.

- **`gui.screen_matrix`를 만족 가능하게 만든다** (2026-09-01): 생성된 capture producer는 step의 `screen_ref`를 전혀 기록하지 않았고 `evaluateCaptureScreenMatrix`는 그 필드로만 커버리지를 센다. 따라서 `screen_matrix`를 선언한 pack은 어떤 실행에서도 모든 화면을 missing으로 보고받았다 — 선언은 검증되지만 영구히 만족 불가능한 상태였다. 픽스처가 이제 `screen_ref`를 기록한다: `autopus-screen` annotation이 있으면 그 값을, 없으면 첫 `goto`의 origin-relative 경로를 쓴다. 컴파일된 시나리오는 화면당 정확히 하나의 test로 annotation을 push하고, 직접 작성한 spec은 경로로 선언한 matrix row를 수정 없이 만족한다. 음성 대조로 집행을 증명했다: 아무도 방문하지 않는 화면을 선언하면 `gui-policy-runtime`이 "gui screen matrix coverage was incomplete"로 journey를 차단한다.

- **polyglot 저장소에서 fast lane 팩이 사라지지 않는다** (2026-09-01): `detectSignals`는 단일 `Stack`만 들고 있었고 `package.json`이 `go.mod` 값을 덮어썼다(last write wins). `go.mod` + `test` script 없는 `package.json` 조합이면 `nodeFastStarter`가 아무것도 내지 않아 Go fast lane까지 함께 사라졌고, `fast`는 기본 `prelaunch` profile의 `must` lane이라 `setup_gap: missing-journey-pack`으로 release gate가 blocked가 됐다. 같은 프로젝트에서 `auto qa plan --lane fast`는 `detected-go-test`를 candidate로 찾고 있었다 — plan은 compiled candidate를, release는 declared pack을 요구하는 비대칭이었다. 이제 감지된 모든 스택마다 fast starter를 낸다(go, node, python, rust 순서 고정). 실측: 결함 조합에서 `go-fast` 생성 + `fast` lane `passed`, go+node 조합에서 두 팩 모두 생성·실행·통과, 단일 스택 저장소는 무변화.

- **릴리스 증거 체인을 파괴하던 redaction 버그** (2026-09-01): `redactReleaseString`이 `privatePathRe.ReplaceAllString(text, "$PROJECT_ROOT")`를 쓰는데, Go 정규식은 `$PROJECT_ROOT`를 존재하지 않는 캡처 그룹 참조로 확장하므로 매치 전체가 **빈 문자열**이 됐다. 의도한 placeholder는 어떤 release index에도 단 한 번도 나타난 적이 없었다. 결과: `/Users/...` 절대 경로로 `auto qa release`를 돌리면 — macOS에서 실제 프로젝트의 정상 경로다 — `run_index_path: ""`, `manifest_paths: [""]`, `output_paths`의 세 root 전부 `""`가 기록되고 gate는 `warn` + exit 0을 보고했다. 감사 추적이 조용히 사라지는 이 세션 최악의 결함이다. 기존 테스트는 `NotContains("/Users/alice")`만 확인해서 빈 문자열에도 통과했다. 두 계층으로 고쳤다: literal 치환으로 redaction이 파괴적이지 않게 하고, producer가 refs를 project-relative로 기록해 지울 private path 자체가 없게 했다. placeholder도 포함을 거짓 주장하지 않는 이름으로 바꿨다. 실측: relative `.`, 절대 `/Users`, `/tmp`, 심볼릭 `$TMPDIR` 네 가지 표기 전부 증거 체인 보존, `redaction_status: clean`.

- **모든 credential URL scheme을 redaction한다** (2026-09-01): `credentialURLRe`가 `https?://`만 매칭해서 `postgres`, `mysql`, `mongodb`, `redis`, `amqp`, `ssh`, `ftp` 자격증명 URL 7종이 publishable evidence에 그대로 들어갔고 `redaction_status`는 `passed`를 보고했다. 이제 임의 scheme을 매칭하며 `RedactText`와 `FindUnsafeText`가 같은 패턴을 공유하므로 살아남은 credential URL은 publish를 차단한다. userinfo 없는 URL은 건드리지 않는다. 함께: `missing_credentials: opaque` 같은 하네스 자체 진단문을 secret으로 오인해 훼손하던 assignment redaction을 shape 기반으로 좁혔다.

- **pre-redaction bytes와 sandbox HOME 격리** (2026-09-01): `_raw/`에는 secret이 그대로 있고 `auto qa init`은 gitignore를 만들지 않아 커밋 가능한 상태였다. `_raw`는 `auto qa report --embed-media`와 local-only media가 해소하는 경로라 삭제할 수 없으므로(publish 경계인 `auto qa release`가 이미 지운다) `.autopus/qa/.gitignore`를 생성해 `_raw/`, `cache/`, `gui/`를 이유와 함께 제외한다. 또한 sandboxed HOME이 프로젝트 루트를 가리켜 `go test`가 `Library/Application Support/go/telemetry/**`를 저장소에 쓰던 문제를 하네스 소유 디렉터리로 옮겼다 — Playwright는 실제 브라우저 캐시를 계속 쓴다.

- **failed evidence만으로 수리 가능한 feedback bundle** (2026-09-01): bundle의 `evidence_artifacts[].path`는 manifest-relative인데 bundle이 manifest 위치를 기록하지 않아 참조가 전부 dangling이었고, prompt에는 실제 실패 출력이 없어 수리 에이전트가 재현을 다시 돌려야만 무엇을 고칠지 알 수 있었다. 이제 publishable·redacted artifact를 bundle에 복사해(local-only와 raw media는 제외하고 `withheld`로 표시) 참조가 해소되며, prompt가 기록된 실패 출력을 담는다. `--to` 대상별 렌더링도 platform adapter의 실제 규약(CLI 이름, 프로젝트 지시 파일)을 담아 H1 문자열만 다르던 상태를 벗었다.

- **알 수 없는 `--lane`이 조용히 다른 것을 실행하지 않는다** (2026-09-01): `auto qa run --lane totally-bogus-lane`은 프로젝트의 auto-detected journey를 대신 실행하고 `passed` check와 exit 0을 보고했다 — 오타 하나가 npm test 스위트를 돌리고 통과로 읽혔다. lane은 canonical release lane이거나 어댑터/Journey Pack이 선언한 것이어야 하며, 그 외에는 `qa_run_lane_unknown`으로 유효 목록과 함께 거부한다. `--lane` 없음(전체)과 프로젝트 선언 custom lane은 그대로 동작한다.

- **desktop-native와 canary-explicit lane을 3자 프로젝트가 쓸 수 있게 한다** (2026-09-01): `desktop-native`는 `app_ref == "autopus-desktop"`, `window_ref == "main-window"`와 canonical landmark를 요구해 Autopus 자신 외에는 어떤 프로젝트도 유효한 팩을 쓸 수 없었다 — 그런데 `prelaunch`와 `release-candidate`의 `must` lane이다. 신뢰 anchor는 앱 이름이 아니라 서명 검증된 release artifact를 요구하는 local runtime provider이므로, identity pin을 shape 검증으로 바꿨다(비어있지 않음, 안전한 문자, platform allowlist, 읽기 전용 operation 시퀀스와 landmark 구조 유지). `auto canary`도 Autopus 모노레포 레이아웃(`Autopus/backend` 등)을 하드코딩하고 Autopus staging URL을 기본값으로 써서, `qa init`이 모든 프로젝트의 release gate에 만족 불가능한 lane을 심고 있었다. 이제 프로젝트에서 빌드 단위를 감지하고, 대상이 없으면 사유를 밝힌 SKIPPED이며(FAIL이 아니다), 시작조차 못한 check는 exec 오류를 detail로 남긴다 — 이전에는 `H1 failed: `로 사유가 비어 있었다. 출력 순서도 고정해 evidence가 byte-stable해졌다. 1급 경로는 그대로 동작한다(모노레포에서 빌드 단위 4개 감지).

- **집행자 없는 lane과 보이지 않던 setup gap** (2026-09-01): `evidence-dashboard`는 `implementation_state: ready`로 광고되지만 어떤 어댑터도 실행하지 않는 순수 metadata였고, `mobile-readiness`는 세 profile 모두에서 deferred라 자신의 setup gap을 보고할 수 없어 항상 `verdict: pass`/`setup_gap_class: none`으로 표시됐다 — `auto qa run --lane mobile-readiness`가 4개 사유로 정확히 fail-close하는데도 그랬다. dashboard 상태는 `planned`로 정정했고, deferred lane도 실제 감지된 setup gap을 보고한다(gate를 차단하지 않는 것과 보이지 않는 것은 다르다). 실측: 세 profile 전부에서 `mobile-readiness` gap row 등장.

- **release-readiness verdict를 실제 결과에서 도출한다** (2026-09-01): verdict는 payload 생성 시점에 `passed` + `deterministic_authority: true`로 상수 설정되어 그대로 반환됐다 — declined 결과도 `passed`였고, lane 0개를 실행한 preview도 deterministic authority를 주장했다. 이제 dispatch된 lane이 하나라도 있을 때만 verdict를 집계하고, 그 외에는 `not_evaluated` + authority false다. CLI 프로젝트에서 `analyzed_surfaces`가 비면 방금 만든 팩 전부를 제거 대상으로 제안하던 문제는 "빈 분석은 아무것도 제안하지 않는다"로 고쳤다. diff의 `removed`는 승인이 수행하지 않는 삭제를 광고했으므로 `unmatched`로 이름을 바꾸고 `approval_deletes_packs: false`를 명시적 사실로 분리했다. `--approve --decline` 동시 지정은 조용히 declined로 해석되던 것을 거부한다.

- **readiness 투영이 하네스 자신이 쓴 경로를 거부하지 않는다** (2026-09-01): 절대 `--project-dir`로 실행하면(문서화된 방식이고 CI에서 정상) producer가 절대 경로를 쓰고 consumer가 `unsafe_ref:absolute_local_user_path`로 영구 차단했다 — `/tmp`에는 사용자 디렉터리가 없으므로 클래스 이름도 부정확했다. workspace 내부로 해소되는 경로는 정규화하고(심볼릭 루트 양방향), 진짜 `/Users/<name>` 유출과 workspace 외부 경로는 계속 차단하며 클래스 이름을 조건에 맞게 정정했다. `check_counts`가 최신 run index 하나만 세어 자신의 `lane_statuses`와 모순되던 문제는 release가 참조하는 모든 run index를 집계하도록 고쳤고, 읽을 수 없는 index는 조용히 건너뛰지 않고 fail-close한다. evidence가 없을 때의 `open : no such file or directory`는 다음 명령을 알려주는 메시지로, 두 토큰짜리 text 출력은 실제 요약으로 바꿨다.

- **CLI 표면 정직성** (2026-09-01): `auto qa full --run` text 모드가 진단 전체를 버리고 `Error: qa release blocked`만 출력했다 — JSON에는 root blocker lane, journey, failure summary가 있었다. 이제 `qa run`과 같은 공유 writer로 실패 경로에도 진단과 `next:` 힌트를 출력한다. blocked plan을 envelope `warn` + exit 0으로 접던 것은 `qa release`와 같은 규약(blocked → error + exit 1)으로 통일했다. run 모드에서 lane 수를 보고하던 `journey_pack_count`는 실제 pack 수를 센다. `qa evidence --surface` 도움말은 자신의 runner가 만드는 `cli` 값을 빠뜨리고 있었다. `domain-readiness plan`이 존재하지 않는 journey pack ref에 `valid=true`를 보고하던 문제와, 갓 init한 프로젝트가 태어날 때부터 invalid이던 starter catalog를 고쳤고, `domain-readiness init`은 형제 명령과 같이 idempotent가 됐다. `auto qa bootstrap`은 제거했다 — `auto qa init`의 숨겨진 byte-identical alias였고 저장소 전체에 참조가 없었다. `auto qa init` 또는 `auto qa full --bootstrap`을 쓴다.

- **`auto qa report` 시각 증거 리포트와 typed GUI capture contract** (2026-08-31): QAMESH 증거는 JSON manifest만 있었고 사람이 볼 표면이 없었다. `auto qa report`가 run/release index와 evidence manifest를 fail-closed로 수집해 외부 자산·JavaScript가 없는 self-contained `report.html`(`qamesh.qa_report.v1`)을 만든다 — verdict, journey timeline, release lane gate matrix, check drill-down, artifact preview, setup gap. index schema나 redaction gate를 통과하지 못하면 ingestion을 `degraded`/`blocked`로 표시하고 거부 사유를 리포트에 남긴다. GUI 증거는 producer가 선언하던 magic artifact kind 5개(`journey_graph`/`aria_snapshot`/`console_summary`/`network_summary`/`screenshot_quarantine_ref`)를 Journey Pack의 `gui.capture` 정책과 `qamesh.gui_capture_index.v1` 계약으로 교체했다. 정의되지 않은 capture key는 pack 전체를 거부해 오타가 조용히 evidence를 끄지 못한다. 하네스는 여전히 Playwright를 직접 구동하지 않고 capture 디렉터리·정책만 할당하며(`AUTOPUS_QAMESH_GUI_CAPTURE_{DIR,POLICY_PATH,INDEX_PATH}`), `auto qa init`이 프로젝트에 Playwright fixture/reporter를 생성한다. 새 `gui-capture-contract` check가 schema, dense step order, totals 일치, policy conformance, local media digest/size를 검증한다. 정책 집행 증거는 producer 자체 인증이 아니라 하네스가 소유한 guard receipt에서 오고, raw screenshot·trace·video는 절대 publish되지 않으며(`retention_class: local-redacted-local-media`) network evidence는 URL이 아닌 `origin:<index>/path` reference다. `replay`는 합성 코드가 아니라 고정 command와 spec digest다. `auto qa report --embed-media`는 local screenshot을 data URI로 인라인하되 리포트를 `local-only`로 표시한다. 실제 Playwright 실행으로 scaffold→run→report 전 구간을 검증했다.

- **`auto qa init`은 `gui-explore` Journey Pack을 생성하지 않는다** (2026-08-31): 실제 Playwright 실행 두 번으로 생성된 팩이 어떤 설정에서도 통과할 수 없음을 확인했다. 전체 E2E 스위트를 돌리면 `forbidden_actions: mutation`이 click/fill/press 계열을 전부 차단해 `gui-policy-runtime`이 blocked가 되고(guard receipt에 `{"t":"action","method":"click","blocked":true}` 기록), read-only subset을 지정하면 매칭되는 spec이 없어 `gui-capture-contract`가 "capture mode always requires at least one step"으로 blocked가 된다. `gui-explore`는 기본 `prelaunch` profile의 `must` lane이라 실행 불가능한 팩은 release gate를 빨갛게 만든다. 반면 팩이 없으면 `auto qa explore`는 status `warning`, exit 0, 그리고 "project-local gui-explore Journey Pack required" setup gap을 반환한다 — 설정하라는 신호로 읽히는 상태다. 따라서 팩 생성을 제거하고, 복사해 쓸 수 있는 예시 팩 2개(browser/desktop)를 `.autopus/qa/capture/README.md`로 옮겼다. 예시는 read-only `@explore` subset을 타깃하고 `capture.mode: on-failure`, `replay_script: optional`을 사용하며, README에 박힌 YAML이 실제로 `journey.Validate`를 통과하는지 테스트가 검증한다. `--grep` 전달 방식은 패키지 매니저별로 실측했다 — npm은 `--` 구분자를 소비하므로 필요하고, pnpm은 그대로 전달해 grep이 적용되지 않아(실제 `playwright test --list`로 확인) 빼야 하며, yarn 1.x는 향후 그대로 전달한다고 경고한다. 무조건 `--`를 넣으면 pnpm에서 전체 스위트가 실행돼 이번에 제거한 바로 그 실패로 되돌아간다.

- **QA 리포트를 실행 흐름에서 발견 가능하게** (2026-08-31): `auto qa run`과 `auto qa explore`가 run index를 쓴 뒤 `next: auto qa report`를 출력한다. 실패한 run에서도 출력한다 — `Execute`는 index를 먼저 쓰고 나서 error를 반환하므로 증거는 이미 존재하고, 실패야말로 리포트가 가장 필요한 순간이다. dry-run은 index가 없으므로 출력하지 않는다. `auto qa coverage`는 ingest된 manifest가 있을 때만 같은 힌트를 추가한다. `auto-qa` command description은 부분 열거("init, plan, run, release, evidence")를 버리고 "plan, run, report, and publish deterministic QA evidence"로 바꿨다 — 열거형 설명은 서브커맨드가 늘 때마다 조용히 낡았고 이번에 `report`가 빠져 있었다. codex/omp/opencode 어댑터가 이 문자열을 각자 복제하므로 세 패키지에 각각 회귀 테스트를 뒀다.

- **집행할 수 없는 `gui.forbidden_actions` label을 생성하지 않고 증거로 보고한다** (2026-08-31): guard preload는 Playwright method 이름만 관찰하므로(`shouldBlockAction`은 정확한 method 이름 일치 또는 `mutation` 확장만 처리) `payment`, `email_send` 같은 business label은 런타임에서 아무것도 차단하지 않는다. 그런데 생성되는 GUI 예시 팩이 `[mutation, payment, email_send]`를 선언해, 제공하지 않는 보장을 광고하고 있었다 — producer 없이 광고되던 artifact capability와 같은 부류다. 예시 팩은 이제 집행 가능한 `mutation`만 선언하고, 어떤 pack이든 집행 불가능한 label을 선언하면 `gui-policy-runtime` check의 `unenforceable_forbidden_actions`에 보고된다. journey를 차단하지는 않으므로 기존 pack은 계속 동작하고, 격차는 check 증거와 렌더된 리포트에서 보인다. Go의 method 목록과 임베드된 guard script는 드리프트 방지 테스트로 고정했다.

- **`gui.network_policy.mode`를 실제 런타임 집행으로 만든다** (2026-08-31): `summary-only`/`blocked`/`local-only`는 enum으로 검증되기만 하고 세 값의 런타임 동작이 동일했다 — guard는 fetch/XHR을 가로채지 않았으므로 `local-only`를 선언한 pack이 어떤 origin에도 도달할 수 있었다. guard preload가 이제 Playwright `page.route`를 설치한다. `summary-only`는 가로채지 않고(기존 동작 유지), `local-only`는 `allowed_origins` 밖 origin 요청을 abort하며, `blocked`는 거기에 더해 allowed origin의 xhr/fetch까지 abort해 UI가 static asset만으로 렌더되게 한다. `data:`/`blob:`/`file:`은 머신을 떠나지 않으므로 정책 대상이 아니다. route 등록은 비동기이고 `patchPage`는 동기라 `goto`가 등록 완료를 await한다 — 그 barrier가 없으면 첫 navigation이 자신이 선언한 interception을 앞지른다. abort된 요청은 guard receipt에 기록되어 `gui-policy-runtime` check의 `network_stopped`로 보고되며 journey를 실패시키지 않는다: 선언한 정책이 작동한 것은 위반이 아니다. 같은 이유로 "capture network stream이 mode blocked와 모순"이라던 직전 검증 규칙을 제거했다 — 이제 network evidence는 정책이 멈춘 요청 목록이고, 그것이 UI의 empty/error 상태를 증명하는 근거다. 실제 Playwright와 로컬 서버 2개(4173 allowed, 4174 off-origin)로 세 모드를 실측했다: summary-only는 둘 다 통과, local-only는 4174만 차단, blocked는 둘 다 차단.

- **OMP 멀티프로바이더 기획과 역할별 모델 라우팅 연결** (2026-08-30): 명시적 `/auto plan --multi`는 PRD 이후, SPEC 작성 전에 `auto orchestra plan`을 한 번 실행합니다. Claude는 `plan` 권한 모드, Codex는 `read-only` sandbox, Antigravity는 `plan` mode와 sandbox를 강제하며 위험한 인자나 지원하지 않는 프로바이더는 실행 전에 차단합니다. typed receipt와 quorum을 통과한 결과만 2,400-token 이하의 신뢰하지 않는 참고 자료로 단일 `spec-writer`에 전달하고, 최종 멀티프로바이더 SPEC 리뷰는 별도로 유지합니다. OMP 카탈로그에 semantic metadata가 없을 때는 명시적으로 저장한 `catalog_trust: operator-attested` 프로필만 exact native selector 교집합을 사용할 수 있습니다. auth/keyless를 관측했다고 주장하지 않으며 receipt와 doctor가 trust, evidence class, effective family를 결속합니다. `auto init`이 생성하는 16개 에이전트 정의는 explain/status에서 항상 agent→role→capability로 표시하고 manifest/checksum이 누락되거나 변조되면 readiness를 차단합니다.

- **OMP `--team` 실행 토폴로지와 `--multi` 프로바이더 축 분리** (2026-08-30): Codex 전용 `Multi-Agent V2`, 존재하지 않는 `.omp/skills/agent-teams`, `send_message`·`followup_task`·`wait_agent` 계열 도구와 goal primitive가 OMP `auto-go`에 남던 생성 오염을 제거했습니다. `--team`은 owner `omp`의 네이티브 `task` batch에서 Lead/Builder/Guardian 책임을 부여하고 `task`·`hub`·`todo`로만 조율합니다. `--multi`는 실행 토폴로지를 바꾸지 않고 멀티프로바이더 기획·리뷰를 추가하며, `--team --multi`는 두 축을 합성합니다. 기본 task-batch, `--solo`, owner `orca` 충돌 규칙과 생성 표면을 회귀 테스트로 고정했습니다.

- **병렬 통합 테스트에서 Context7 오류 분류가 흔들리던 공유 transport 제거** (2026-08-30): `NewContext7Client`가 `http.DefaultTransport`의 전역 connection pool을 공유해, 다른 병렬 테스트의 idle-connection 정리가 진행 중인 404 요청을 `CloseIdleConnections called` 네트워크 오류로 바꾸던 문제를 수정했습니다. 각 client가 immutable default template에서 clone한 독립 transport를 소유하도록 바꾸고 50회 반복 테스트로 `ErrLibraryNotFound` 분류를 고정했습니다.

- **v0.50.110 소각 후 v0.50.111 정상 보호 릴리즈 lane 복구** (2026-08-31): v0.50.110 릴리즈 태그와 증거 태그는 이미 생겼지만, 보호 작업이 자격 증명을 구체화하거나 자산을 업로드하기 전에 실패했다. 저장소 `GITHUB_TOKEN`에서 필수 검토자 정보가 가려져 엄격한 환경 검증을 통과할 수 없었으며, GitHub 릴리즈 ID `379549016`은 자산이 없는 미게시 실패 초안으로 남았다. 이 좌표는 성공한 릴리즈 단계가 아니므로 이동·삭제·재사용하지 않는다. A23은 v0.50.111에만 매핑하고 A22/v0.50.109를 직접 선행 릴리즈로 유지한다. 보호 작업의 `--sealed-runtime`은 정확히 봉인된 태그 규칙 집합과 배포 태그 정책만 검증하고, GitHub가 `adk-companion-release` 승인 자체를 강제한다. 로컬 사전 점검과 게시자는 필수 검토자·관리자 우회까지 확인하는 엄격한 `--armed`·`--sealed` 검증을 유지한다. v0.50.111은 새 one-shot 증거 객체와 K3 보고서·증명을 만들고, 정확한 15개 자산, Homebrew Cask CAS, 공개 v0.50.109→v0.50.111 canary 순서를 지킨다.

- **Claude Code 2.1.246·Codex CLI 0.149.1·OMP 18.0.5 네이티브 표면 전환** (2026-08-26): 세 CLI의 공식 릴리즈와 실제 설치 바이너리를 다시 확인하고 생성·검증·정리 수명주기를 현재 계약에 맞췄다. Claude는 디렉터리형 `.claude/skills/<name>/SKILL.md`, 이름 있는 `Agent` teammate, 자동 team cleanup, 동적 `Workflow({scriptPath,args})`를 사용하며 제거된 `TeamCreate`·`TeamDelete`와 `team_default`를 더 이상 생성하지 않는다. Codex는 고유한 `.codex/skills/codex-<name>/SKILL.md`, `[features.multi_agent_v2]`, 현재 협업 도구 6개, 공유 cwd/filesystem 계약으로 전환했다. 저장소 범위의 `.codex/prompts/`, Markdown `.codex/rules/`, Codex 소유 `.agents/skills/`와 이를 만들던 dead renderer를 제거하고, OpenCode가 혼합 설치의 `.agents/skills/`와 `AGENTS.md`를 단독 소유하도록 정리했다. OMP는 `.omp/skills/`·`.omp/commands/`를 네이티브 루트로 사용하고 기본 `.omp/config.yml`과 `.agents/skills` 재등록을 없앴다. provider-free doctor RPC는 prompt나 provider 요청 없이 bootstrap 모델을 로컬에서 선택하고, OMP 18의 `compaction.methodOrder`를 사용한 실제 관리형 canary에서 같은 process/session의 pre/post ACK, 정확한 canonical body 재주입, cleanup 0건을 확인했다. 플랫폼 registry와 transaction 순서를 통합해 config 저장 실패 시 생성 파일을 되돌리고, update·remove·doctor가 같은 adapter catalog와 manifest 소유권을 사용한다.

- **플랫폼 수명주기와 정리 소유권 강화** (2026-08-26): `autopus.yaml` symlink와 `${VAR}` 치환값의 secret 고정 저장을 차단하고, 플랫폼 add/remove/update에서 adapter·manifest·config를 하나의 rollback 경계로 묶었다. Codex·Claude Clean은 exact generated-path allowlist와 symlink preflight를 통과한 파일만 정리하며, Claude는 owner-only permission ledger로 실제 추가한 권한만 되돌린다. Codex와 OMP는 사용자가 삭제한 managed path의 manifest claim을 유지해 다음 Update에서 파일이 되살아나지 않게 했고, OMP manifest는 owner-only `0600`만 신뢰한다. Codex↔OpenCode `AGENTS.md` 소유권 handoff, OMP 사용자 native skill/command/agent 보존, provider-free RPC의 zero-message·exact-response gate도 회귀 테스트로 고정했다.

- **v0.50.108→v0.50.109 업그레이드 입장 검증 추가** (2026-08-28): CI는 v0.50.108 프로젝트 fixture에 `0.50.109-canary` source candidate를 적용해 native surface 전환, 사용자 파일 보존, legacy surface 정리, 실제 Git hook 거부·승인, doctor의 config·hook·OMP validation을 검증한다. 게시 후에는 별도 `workflow_dispatch`가 공개 서명 installer로 v0.50.108을 설치하고 프로젝트를 만든 뒤 같은 슬롯을 v0.50.109로 교체해 public asset admission receipt를 남긴다. 두 경로의 증명 범위를 분리해 source fixture 검증을 실제 release asset 검증으로 과장하지 않는다.

- **기존 OMP 프로젝트의 안전한 권한 이전과 릴리즈 gate 복구** (2026-08-28): v0.50.108이 `0644`로 기록한 OMP manifest와 `.omp/config.yml`은 group·other 쓰기 권한이 없을 때만 update가 `0600`으로 좁힌다. 쓰기 가능한 파일과 symlink는 계속 거부하며, Clean은 권한을 자동 교정하지 않고 fail-closed 동작을 유지한다. Claude init 통합 테스트는 native directory skill을 기준으로 갱신했고, Windows process probe는 외부 2초 상한 안에 정리 시간을 확보했다. 남은 lint dead code, 305줄 JSON contract test, lifecycle rollback coverage도 릴리즈 gate 기준에 맞췄다.

- **채널 승인 기반 release signer·promotion key 회전** (2026-08-28): 기존 SSH tag signer와 promotion K2 private key를 사용할 수 없는 상황에서도 같은 release가 자신의 새 키를 신뢰하는 self-pinning을 허용하지 않는다. 이미 배포된 ADK channel A0가 exact v0.50.109 source commit/tree, 이전·신규 tag signer, promotion K3를 묶은 canonical sidecar에 서명하고, 별도 one-shot ref가 tag 생성 전에 이를 배포한다. v0.50.109는 promotion static policy·report·attestation이 없는 canonical-full bridge로 새 공개키만 선배치하며, K3 active evidence는 immutable bridge를 predecessor로 삼는 v0.50.111부터 허용한다. 자세한 절차와 incident boundary는 `docs/runbooks/release-key-rotation.md`에 기록했다.

- **A22 좌표를 v0.50.105로 전환하고 서명 스모크 데드라인을 교정** (2026-08-09): 좌표를 `v0.50.105`로 옮기고 금지 좌표 창을 `v0.50.104`·`v0.50.103`·`v0.50.102`로 민다. v0.50.104는 canary 40콜과 서명·증거 태그까지 통과한 뒤 GoReleaser post hook의 실행 스모크가 15초 데드라인에 걸려 자산 업로드 전에 중단됐다. 스모크가 시간을 재는 명령은 `version --short` 하나뿐이고 가변 비용은 갓 서명된 바이너리를 `nobody` UID 격리에서 처음 실행할 때의 macOS 평가 지연이므로, 증거 임계가 아니라 liveness 한계다. 도구 상한(60초) 안에서 45초로 올린다. A21 predecessor 핀과 lineage 세대 구조는 그대로다.

- **A22 좌표를 v0.50.106으로 전환** (2026-08-10): 좌표를 `v0.50.106`으로 옮기고 금지 좌표 창을 `v0.50.105`·`v0.50.104`·`v0.50.103`으로 민다. v0.50.105는 canary 40콜과 GoReleaser 자산 업로드까지 처음으로 통과했지만, Homebrew cask 게시가 predecessor 핀 불일치로 멈췄다. 릴리즈 자체는 immutable로 게시된 상태이므로 되돌릴 수 없고, 태그 트리에는 핀 수정이 없어 태그 기준 복구 워크플로로도 되살릴 수 없다. 따라서 핀이 교정된 main에서 새 좌표로 재발행한다. cask는 v0.50.92에서 v0.50.106으로 건너뛴다 — cask는 최신만 유지하면 되므로 사용자 영향은 없다.

- **v0.50.106 게시 후 Homebrew predecessor를 실제 tap 상태로 동기화** (2026-08-10): v0.50.106이 게시되며 tap이 `9484e41a`로 전진했고 Cask blob은 `c1f90dc1`이 됐다. 게시가 성공하면 핀은 반드시 한 세대 낡으므로, 다음 릴리즈가 그 사실을 발견하게 두지 않고 지금 맞춘다. v0.50.105가 막힌 원인이 정확히 이 지연이었다 — 핀이 v0.50.92에 머문 채 11개 릴리즈를 통과했다. 렌더러가 게시된 Cask를 바이트 단위로 재현함을 확인해 핀 값을 검증했고(`git hash-object` = `c1f90dc1`), `prepare-release.sh`의 tap 핀 preflight가 실제 tap과 일치함을 실행해 확인했다.

- **Homebrew tap 핀 검사를 GoReleaser 앞으로 옮겨 버전 소각을 막는다** (2026-08-10): 지금까지 tap predecessor 핀은 job의 **맨 끝** Cask 게시에서만 검증됐다. 그 지점에서 실패하면 immutable 릴리즈가 이미 존재하므로 되돌릴 수 없고, 복구 워크플로는 같은 태그의 같은 낡은 스크립트를 다시 돌리므로 그 버전은 영구히 Cask를 못 받는다 — v0.50.105가 그렇게 소각됐다. 같은 비교를 `verify-homebrew-tap-pins.sh`로 뽑아 GoReleaser **앞**에서 실행한다. tap은 public이라 쓰기 자격증명이 필요 없고, GoReleaser 이전에 자격증명을 두지 않는 기존 불변식을 건드리지 않는다. prep의 preflight도 같은 스크립트에 위임하므로 구현은 하나다. 세 실패 경로(헤드 드리프트·blob 드리프트·핀 부재)를 실제 tap 대상으로 구동해 fail-closed를 확인했다. 복구 워크플로에는 적용 범위(일시적 tap 쓰기 실패)와 비적용 범위(태그 스크립트 자체가 틀린 경우 → 새 좌표), 그리고 다른 ref의 스크립트를 쓰면 안 되는 이유를 명시했다.

- **A22 좌표를 v0.50.107로 전환** (2026-08-18): 좌표를 `v0.50.107`로 옮기고 금지 좌표 창을 `v0.50.106`·`v0.50.105`·`v0.50.104`로 민다. v0.50.106은 정상 게시되어 immutable이므로 다음 릴리즈는 정의상 새 좌표다. 좌표는 세 가지 리터럴 형태로 박혀 있어 각각 옮겼다 — v 접두 태그(`v0.50.106`), bare 버전(`0.50.106`: archive 이름·`COMPANION_VERSION`·`RELEASE_VERSION`), bare 패치 번호(`"106"`: `exactLineageTagVersionGuard`). bare 버전을 빼면 `A22_TAG`와 `A22_VERSION`이 어긋나고 lineage dispatch가 둘의 논리곱이므로 `prior_release_identity_mismatch`로 떨어지며, 패치 번호를 빼면 guard가 이전 좌표를 계속 주장한다. predecessor·역사 사실인 세 곳은 그대로 둔다 — `homebrewBridgeCask()`가 만드는 게시된 predecessor Cask와 hardening 테스트의 prior-cask 렌더(둘 다 `PRIOR_CASK_BLOB` 대조 대상), 그리고 `produce.sh`의 15초 측정(v0.50.104)·`verify-homebrew-tap-pins.sh`의 소각 선례(v0.50.105). lineage 세대는 A22로, `A22_A21_ANCESTOR_SHA`도 불변이다. 좌표를 옮긴 뒤 tap 핀 preflight를 실제 tap에 실행해 일치를 확인했다(head `9484e41a`, cask `c1f90dc1`).

- **v0.50.107 게시 후 Homebrew predecessor를 실제 tap 상태로 동기화** (2026-08-18): v0.50.107이 게시되며 tap head가 `957834c5`로 전진하고 Cask blob은 `02e77503`이 됐다. 게시가 성공하면 핀은 반드시 한 세대 낡으므로, 다음 릴리즈가 그 사실을 GoReleaser 뒤에서 발견하게 두지 않고 지금 맞춘다. 핀 값은 렌더러가 게시된 Cask를 바이트 단위로 재현함을 확인해 검증했다 — `render_homebrew_cask 0.50.107`의 출력이 tap의 `Casks/auto.rb`와 `cmp` 일치하고 `git hash-object`가 `02e77503`, sha256이 `7bbb0763`이다. `verify-homebrew-tap-pins.sh`를 실제 tap에 실행해 일치를 확인했고 homebrew·release hardening 스위트와 `internal/companionmanifest` Homebrew 테스트를 통과했다. predecessor fixture(`homebrewBridgeCask()`, hardening 테스트의 prior-cask 렌더)도 같은 좌표로 옮겼다.

- **A22 좌표를 v0.50.108로 전환** (2026-08-19): 좌표를 `v0.50.108`로 옮기고 금지 좌표 창을 `v0.50.107`·`v0.50.106`·`v0.50.105`로 민다(복구 워크플로는 한 세대 더 깊은 `v0.50.104`까지 유지한다 — 창의 불변식은 "release는 현재 좌표 바로 아래 연속 3세대, recovery는 그 집합 + 정확히 한 세대"다). v0.50.107은 정상 게시되어 immutable이므로 다음 릴리즈는 정의상 새 좌표다. 세 리터럴 형태를 각각 옮겼다 — v 접두 태그(`v0.50.108`), bare 버전(`0.50.108`: archive 이름·`COMPANION_VERSION`·`RELEASE_VERSION`·goreleaser의 `eq .Version`), bare 패치 번호(`"108"`: `exactLineageTagVersionGuard`). 이번 이동에서는 직전 이동(`e5b63173`)이 건너뛴 스테일 리터럴 7곳도 함께 정렬했다 — `.autopus/specs/SPEC-OMP-004/{plan,spec}.md`와 `internal/cli` fixture 5곳이 `0.50.106`에 머물러 있었다. 그 7곳은 좌표를 교차 검증하지 않으므로(106인 채로 v0.50.107 릴리즈 CI가 전부 녹색이었다) 게이트에 영향은 없었고, "이 리터럴은 현재 좌표와 같다"는 불변식을 복원한다. predecessor·역사 사실 네 곳은 그대로 둔다 — `homebrewBridgeCask()`가 만드는 게시된 predecessor Cask와 hardening 테스트의 prior-cask 렌더(둘 다 `PRIOR_CASK_BLOB` 대조 대상이며, `3cd9bf40`이 v0.50.107로 동기화해 둔 값이 다음 릴리즈의 predecessor로 정확하다), `produce.sh`의 15초 측정(v0.50.104), `verify-homebrew-tap-pins.sh`의 버전 소각 선례(v0.50.105). 반면 `ompcontextverify/historical_fixture_test.go`의 `AutoVersion`은 파일명과 달리 승격 대상 바이너리의 현재 좌표이므로 함께 옮겼다(`OMPVersion: omp/17.2.7` 오라클 핀은 불변). lineage 세대는 A22로, `A22_A21_ANCESTOR_SHA`도 불변이다. 검증: bash hardening 계약, `internal/companionmanifest`·`pkg/companionmanifest`·`pkg/promptlayer`·루트 패키지 테스트, `goreleaser check`, `actionlint`, 전체 `.sh`의 `bash -n`, golangci-lint 0건.

- **v0.50.108 게시 후 Homebrew predecessor를 실제 tap 상태로 동기화** (2026-08-21): v0.50.108이 게시되며 tap head가 `66d6d4fe`로 전진하고 Cask blob은 `1533b7f4`가 됐다. 게시가 성공하면 핀은 반드시 한 세대 낡으므로, 다음 릴리즈가 그 사실을 GoReleaser 뒤에서 발견하게 두지 않고 지금 맞춘다. 핀 값은 렌더러가 게시된 Cask를 바이트 단위로 재현함을 확인해 검증했다 — 릴리즈 자산의 `checksums.txt`에서 뽑은 네 아카이브 digest로 `render_homebrew_cask 0.50.108`을 돌린 출력이 tap의 `Casks/auto.rb`와 `cmp` 일치하고, `git hash-object`가 `1533b7f4`, sha256이 `db6f57cc`다. `verify-homebrew-tap-pins.sh`를 실제 tap에 실행해 일치를 확인했고(`match (head 66d6d4fe…, cask 1533b7f4…)`) homebrew Go 테스트와 bash hardening 계약을 통과했다. predecessor fixture(`homebrewBridgeCask()`, hardening 테스트의 prior-cask 렌더)도 같은 좌표로 옮겼다.

- **A22 좌표를 v0.50.109로 전환** (2026-08-22): 좌표를 `v0.50.109`로 옮기고 금지 좌표 창을 release `{108,107,106}` / recovery `{108,107,106,105}`로 한 세대 민다 — 창의 불변식은 "release는 현재 좌표 바로 아래 연속 3세대, recovery는 그 집합 + 정확히 한 세대"다. v0.50.108은 정상 게시되어 immutable이므로 다음 릴리즈는 정의상 새 좌표다. 세 리터럴 형태를 각각 옮겼다 — v 접두 태그(`v0.50.109`), bare 버전(`0.50.109`: archive 이름·`COMPANION_VERSION`·`RELEASE_VERSION`·goreleaser `eq .Version`), bare 패치 번호(`"109"`: `exactLineageTagVersionGuard`). 이번 세대는 좌표와 Homebrew predecessor 핀이 둘 다 `0.50.108`이라 문자열만으로는 구분되지 않으므로, 직전 전환(`4af41fbe`)이 건드리지 않은 자리를 기준으로 5줄을 동결했다 — `homebrewBridgeCask()`의 `version "0.50.108"`과 `TestHomebrewFormulaBridge_PublishedV050108TapPins`의 이름·digest·blob 단정, 그리고 `release-homebrew-hardening-test.sh`의 prior-cask 렌더 인자. `PRIOR_TAP_COMMIT`·`PRIOR_CASK_BLOB`은 해시라 스윕 대상이 아니다. `produce.sh`의 15초 측정(v0.50.104)과 `verify-homebrew-tap-pins.sh`의 소각 선례(v0.50.105)도 역사적 사실이므로 불변이다. lineage 세대는 A22로, `A22_A21_ANCESTOR_SHA`도 그대로다. 동결이 옳은지는 실제 tap으로 확인했다 — `verify-homebrew-tap-pins.sh`가 `match (head 66d6d4fe…, cask 1533b7f4…)`로 게시된 v0.50.108 상태와 일치하므로 predecessor는 한 세대 낡은 채 정확하다.

### Removed

- **프로덕션에서 쓰이지 않던 `PaneBackend`** (2026-08-09): 이름은 pane인데 실제로는 `runProvider`로 자식 프로세스를 띄우는 타입이었고, 프로덕션 참조는 없이 자기 자신을 검증하는 테스트와 fresh-judge 허용목록 항목만 남아 있었다. 이 오해를 부르는 이름 때문에 전송 선택 seam을 잘못 고를 뻔했으므로 제거한다. `pipelineBackendHasFreshExecutionSemantics`의 허용목록에서도 빠지며, 남은 두 내장 백엔드(`subprocessBackend`, `InteractivePaneBackend`)의 판정은 그대로다. `SelectBackend`의 주석에 자유 텍스트 호출자는 이 백엔드를 쓰면 안 된다는 계약을 명시했다.

### Fixed

- **연결 손실 1건에 재접속을 여러 번 수행해 등록을 중복시키던 a2a 전송** (2026-08-22): `ReconnectTransport`를 호출하는 드라이버는 셋이다 — receive 루프의 `handleReceiveError`, heartbeat 타임아웃 콜백, 그리고 외부 호출자(`pkg/worker/loop_lifecycle`, 토큰 로테이션의 `pkg/worker/auth`). `reconnectMu`는 이들을 **직렬화만** 했으므로 같은 연결 손실에 대해 각자가 차례로 완전한 재접속을 수행했다. 그 결과 뒤에 온 드라이버가 앞선 드라이버가 막 세운 연결을 다시 헐고 카드를 또 등록했고, 한 번에 하나의 연결만 받는 백엔드에서는 두 번째가 `ws write: websocket: close sent` 또는 `dial: connection refused`로 떨어졌다. `TestServer_ReconnectTransport_ReRegistersAgentCard`가 CI에서 정확히 이 서명으로 죽어 `main`의 `ci / test`를 red로 만들었고(run 32455040386), red CI는 `release` 잡을 skip시키므로 릴리즈 게이트가 막혔다. 전송에 세대 카운터를 도입해 드라이버가 "지금 재접속"이 아니라 "내가 본 이 연결을 교체"를 요청하게 했다 — receive 루프는 `Receive` 직전 세대를 포착해 실패 시 그 세대로 요청하고, 세대가 이미 지나갔으면 재접속은 no-op이다. 직렬화가 아니라 통합(coalescing)이므로 손실 1건에 재접속 1회·등록 1건이 보장된다. 같은 창에서 다시 끊어진 연결의 복구가 루프 1회 지연될 수 있는데 이는 루프의 backoff가 이미 처리하는 경우이고, 반대로 중복 재접속은 안전하게 만들 방법이 없다. 공개 `ReconnectTransport`는 세대를 갖지 않은 외부 호출자용으로 무조건 재접속을 유지한다 — `auth`가 `SetAuthToken` 직후 새 연결을 강제하는 경로가 통합에 삼켜져서는 안 된다.

- **포화된 러너에서 pid 기록을 기다리지 않던 POSIX 설치기 테스트 2건** (2026-08-22): `TestPOSIXInstallerTimeoutKillsReparentedTermResistantChild`와 `TestPOSIXInstallerBoundedCommandTimeoutCleansProcessTree`는 helper가 백그라운드 자식의 pid를 파일에 쓸 것이라 가정하고 readiness 핸드셰이크 없이 그 파일을 읽는다. 그런데 `run_bounded_command 3`이 helper를 3초에 죽이므로, 전체 모듈 병렬 실행(`-race`, 12-way)에서는 helper의 `sh` 기동과 프로세스 생성 두 번이 그 예산 안에 pid 공개까지 도달하지 못해 `open .../child.pid: no such file or directory`로 죽었다(2/2 실패, 격리 실행은 통과). helper는 영원히 hang하므로 bound는 항상 발동하고 exit 124 단정과 reparented child 종료 단정은 예산과 무관하다 — 바꾼 것은 helper 기동에 줄 여유뿐이다. 예산을 6초, elapsed 상한을 12초로 올린다(`bf211386`이 pane 예산을 프로덕션 sleep 위로 올린 것과 같은 계열). 같은 파일의 `TestPOSIXInstallerVersionSmokeBoundsCapturedOutput`은 예산이 있지만 단정이 **exit 124가 아님**을 요구하므로(출력 상한이 타임아웃보다 먼저 발동해야 한다) 예산을 올리면 단정이 약해져 의도적으로 그대로 뒀다.

- **릴리즈가 추가한 무시 패턴이 기존 프로젝트에 영원히 도달하지 않던 배치** (2026-08-22): `updateGitignore`의 프로덕션 호출부는 `init.go` 한 곳뿐이었다. 그래서 `.omp/rules/`·`.omp/agents/`·`.omp/config.yml`·`.omp/extensions/`처럼 직전 릴리즈들이 추가한 패턴은 **`auto init`을 새로 돌린 프로젝트에만** 들어가고, 이미 초기화된 프로젝트는 `auto update`를 몇 번 돌려도 받지 못했다. omp의 규칙·확장 발견이 gitignore 인식 글롭이라는 비대칭 때문에 이 패턴들은 단순 위생 항목이 아니다 — 빠지면 생성 표면이 git에 노출되고 doctor의 hygiene 점검이 걸린다. 실제로 이 워크스페이스 루트는 omp 플랫폼을 새로 설치한 뒤에도 `.gitignore`에 `.omp` 항목이 0건이었다. `update`가 이 쓰기를 소유하게 해 두 진입점의 무시 표면을 일치시킨다. 쓰기는 플랫폼 방출보다 **앞**에 둔다 — 뒤에 두면 생성 표면이 무시 항목보다 먼저 생겨 그 사이에 git에 노출된다. `updateGitignore`를 `planGitignoreUpdate`(읽기 전용 판정)와 쓰기로 쪼개 `--plan`이 `.gitignore would be updated (N pattern(s))`로 예고하고 실제 쓰기가 같은 판정을 쓰게 했다 — preview가 누락한 쓰기가 나올 수 없다. 이미 만족된 파일에서는 아무것도 쓰지 않고 아무것도 보고하지 않으며, 사용자 항목은 그대로 살아남는다.

- **`--self --check`가 CLI가 설치할 수 없는 릴리즈를 그냥 권하던 문제** (2026-08-22): 소유권 게이트는 `installSelfUpdateReleaseWithOperation`에 있는데 `--check`는 그 앞에서 반환한다. 그래서 Homebrew나 Autopus Desktop이 소유한 설치에서도 `업데이트 가능: v0.50.91 → v0.50.108` 한 줄만 나오고, 그 안내를 따라 `--self`를 실행하면 `manager-required`로 거부된다. 게이트가 없던 v0.50.91 이하에서는 같은 안내가 관리형 슬롯 바이너리를 실제로 덮어써 서명 매니페스트의 `artifact_digest`와 불일치를 만들고, Autopus Desktop broker가 `managed_adk_broker_current_slot_rejected`(exit 126)로 슬롯을 거부하게 만든다 — 즉 이 한 줄이 CLI를 실행 불가 상태로 이끄는 안내였다. check 표면을 `reportSelfUpdateAvailability` 하나로 모아 가용성 한 줄이 단독으로 나올 수 없게 묶고, 관리형 설치에서는 소유 관리자와 경로를 뒤에 붙인다. 경로 해석 실패는 침묵한다 — 프로브가 읽기 불가한 실행 경로를 check 오류로 바꿔서는 안 된다. 자기 소유 설치의 출력은 한 줄 그대로다.

- **v0.50.108 릴리즈 게이트를 막은 `TestTaskPoller_OnTaskCallback` 패닉** (2026-08-21): 콜백이 `called.Store(true)`를 먼저 실행하고 `receivedData.Store(...)`를 나중에 실행했는데, 테스트는 `require.Eventually`로 `called`만 기다린 뒤 곧바로 `receivedData.Load().(string)`을 단정했다. 두 저장 사이에 goroutine이 선점되면 `interface conversion: interface {} is nil, not string`으로 패닉하며 패키지 전체가 FAIL한다 — v0.50.108 태그의 `ci / test`가 정확히 이렇게 죽어 `release` 잡이 skip됐고(재실행에서 통과), 릴리즈 게이트가 제품 결함이 아닌 테스트 결함으로 막혔다. 콜백의 저장 순서를 뒤집어(페이로드를 먼저 공개하고 플래그를 나중에 세운다) 창을 없애고, 단정도 `require.True(t, ok, ...)`로 바꿔 다음에 같은 계열 문제가 나면 패닉이 아니라 원인을 지목하는 실패가 되게 했다. `-race -count=300 -p 12 -parallel 12`로 확인했다.

- **omp 규칙 14개가 세션에 하나도 도달하지 않던 배치** (2026-08-18): 어댑터는 규칙을 `.agents/rules/autopus/<name>.md`에 썼지만 omp는 각 규칙 루트를 **비재귀**로만 훑는다. 그래서 14개가 디스크에는 있는데 세션에는 0개가 도달했다 — 라이브 RPC 세션의 `/dump` 시스템 프롬프트에 `<domain-rules>` 블록 자체가 없고, `omp ttsr list --json`에도 트리거를 가진 3개(`shell-portability`·`lore-commit`·`worktree-safety`)가 등록되지 않았다. 같은 파일을 비재귀 루트로 옮기면 세 경로로 나뉘어 전부 도달한다 — `<domain-rules>` 9개, TTSR 3개, `alwaysApply` 본문 주입 2개. omp에는 `rules.customDirectories` 계열 설정 키가 없어(`ttsr.builtinRules`·`ttsr.disabledRules`만 존재) 하위 디렉터리를 등록해 우회할 방법이 없으므로, 네임스페이스를 디렉터리에서 파일명 접두사로 바꿔 `.omp/rules/autopus-<name>.md`로 방출한다. `.agents/rules/`를 더 이상 건드리지 않으므로 그 공유 디렉터리의 충돌 표면도 같이 사라졌다. 소유 경계는 접두사와 매니페스트 기록이며, 같은 디렉터리의 사용자 파일(`.omp/rules/mine.md`, `.omp/RULES.md`)은 Clean 후에도 바이트 동일하게 남는다. 이전 매니페스트가 지목하는 레거시 경로는 prune 루트에 남겨 한 번의 `auto update`로 비워지고 빈 디렉터리까지 접힌다. 셸에 설치된 17.3.5와 CI가 핀한 17.2.7 양쪽에서 14/14를 관측했다.

- **생성 규칙 표면을 gitignore 파일명 글롭으로 덮으면 발견이 죽는 문제** (2026-08-18): `auto init`이 심는 무시 패턴을 `.omp/rules/`(디렉터리 형태)로 고정한다. 같은 워크스페이스에서 한 줄만 바꿔 측정한 결과 파일명 글롭 `.omp/rules/autopus-*.md`는 규칙 14개를 omp 발견에서 전부 떨어뜨리고(`<domain-rules>` 9→0, TTSR 3→0, `alwaysApply` 2→0) 와일드카드 `.omp/rules/*`는 사용자 파일까지 떨어뜨리는데, 디렉터리 패턴 `.omp/rules/`와 루트 고정형 `/.omp/rules/`는 아무것도 억제하지 않는다. 패턴을 아예 빼는 선택은 doctor의 "runtime/generated unignored files" 위생 점검을 깨므로 답이 아니다. 이 비대칭은 git 저장소 안에서만 발현한다 — `git init` 없는 디렉터리에서는 글롭을 넣어도 14개가 그대로 발견되므로, 회귀 테스트는 워크스페이스를 `git init`한 뒤 프로브한다. 무시된 디렉터리 안의 파일은 negation으로 되살릴 수 없으므로(git 계약), 그 안의 자기 규칙을 추적하려면 `git add -f`를 쓰거나 규칙을 `.omp/rules/` 밖에 두라고 문서에 적었다.

- **생성 확장(`.omp/extensions/`)이 무시 목록에서 빠져 git에 노출되던 문제** (2026-08-19): 컨텍스트 브리지를 opt-in하면 `.omp/extensions/autopus-context.ts`와 `autopus-pipeline.ts`가 생성되는데 무시 패턴이 없어 `git status`에 그대로 남았다. 규칙과 같은 비대칭이 확장 탐색에도 적용되는지 몰라 무측정 변경을 피했다가, `autopus-pipeline.ts`의 `pi.registerCommand("autopus-pipeline")`을 관측 지점으로 삼아 실측했다 — 무시 없음: 커맨드 등록됨, 디렉터리 패턴 `.omp/extensions/`: **등록됨**, 파일명 글롭 `.omp/extensions/autopus-*.ts`: **등록 안 됨**. 규칙 표면과 동일한 규칙이 성립하므로 `auto init`의 무시 목록에 디렉터리 형태로 추가하고, 파일명 글롭 형태를 금지 패턴 단정에 넣었다. `ompDiscoveryRoots`에도 `.omp/extensions/`를 등록해 앞으로 이 경로에 파일명형 패턴이 들어오면 테스트가 막는다.

- **omp 단독 프로젝트에서 해소 불가능한 claude 경고로 doctor가 영구 FAIL이던 판정** (2026-08-18): `platforms: [omp]`만 구성한 워크스페이스에서 omp 점검이 전부 OK인데도 `.claude/settings.json not found (run 'auto init' to generate)`와 `Rule Conflicts`가 떴다. omp 어댑터는 `.claude/**`를 절대 만들지 않는 것이 명시적 계약이므로 그 지시는 영원히 이행될 수 없고, 사용자에게 남는 것은 지워지지 않는 빨간 항목뿐이었다. claude 전용 점검(`Hooks & Permissions` 섹션과 그 본문, `Rule Conflicts`, JSON의 `doctor.hooks.*`·`doctor.permissions.allow`·`doctor.rule_conflict.*`)을 `claude-code`가 구성 플랫폼에 있을 때만 수행한다. `claude-code`를 구성한 프로젝트의 동작은 그대로다.

- **autopus.yaml이 없는 디렉터리에서 합성 기본 설정으로 진행하던 fail-open** (2026-08-18): 설정이 없으면 로더가 `DefaultFullConfig`(플랫폼 `claude-code`)를 합성해 돌려주는데 호출자들이 그 사실을 무시했다. 그래서 프로젝트가 아닌 디렉터리에서 `auto doctor`가 `[OK] autopus.yaml (mode: full)`을 출력한 뒤 `.claude/**` 부재로 ERROR 5건을 쏟고, `auto update --preview`는 그 자리에 `create autopus.yaml`과 하네스 전체 스캐폴딩을 계획했다. Orca 사용자의 에이전트 터미널 cwd는 워크트리 상위(워크스페이스 루트)인 경우가 정상이므로, 이 경로는 git 저장소 밖에 유령 프로젝트를 만든다. 이제 `doctor`·`update`(preview 포함)·`platform list/add/remove`는 `autopus.yaml` 부재 시 `auto init`과 `--dir`를 안내하며 비정상 종료하고, `doctor --json`은 `project_missing` 코드로 실패한다. `auto init`은 종전대로 기본값으로 동작하고, 메타 워크스페이스 루트의 `runWorkspaceUpdate` 분기는 가드보다 앞에 두어 영향이 없다. 같은 변경에서 어느 디렉터리에서 실행해도 배너가 `autopus-adk`로 표시되던 원인도 제거했다 — `doctor`와 `check`가 프로젝트 이름 대신 하드코딩 리터럴을 넘기고 있었다.

- **릴리즈 게이트를 막던 pane 테스트 예산 부족** (2026-08-18): `TestInteractivePaneBackend_CompletionCollectionAndCleanupFailuresStayPane`이 요청 예산을 `300ms`로 잡았는데, 그 예산 안에서 프로덕션이 반드시 소모하는 실시간 sleep이 `200ms`다 — 붙여넣기와 Enter 사이의 `promptRegisterDelay` 타이머와 launch 직후의 등록 sleep. 여유가 `100ms`뿐이라 부하가 걸린 러너에서 Go 타이머가 그 창 안에 스케줄되지 못하면 `launch enter error: context deadline exceeded`로 죽었고, 정작 검증 대상인 `TimedOut`·bounded cleanup 단정에는 도달하지 못했다. 예산을 `2s`로 올려 `TestExecute_S12_TimeoutDeterministic`과 같은 자리에 둔다 — stub detector는 절대 완료하지 않으므로 단정은 한 줄도 바뀌지 않는다. `promptRegisterDelay`는 배포 동작이므로 건드리지 않았다: 300ms 예산으로 실패하는 것 자체는 옳은 동작이고, 문제는 타이밍을 검증하지 않는 테스트가 그 예산을 쓴 것이었다. 같은 파일의 다른 Execute 테스트는 500ms~3s를 쓰고 있어 이 테스트만 어긋나 있었다. 오버서브스크립션(`-p 12 -parallel 12`) 아래 5회 반복 통과를 확인했다.

- **doctor OMP readiness 플레이크를 진단 불가로 만들던 증거 부재** (2026-08-18): `TestOMP002_S10_DoctorCLIProjectsStableOMPChecksInTextAndJSON`이 CI에서 rpc 세 capability를 `reason=event_missing`으로 실패했는데, 이 값은 코드상 "RPC가 exit 0으로 끝나고 loopback provider 2회 요청과 receipt 검증까지 통과했지만 인식 가능한 프레임이 하나도 없었다"만을 뜻한다 — 데드라인이 실제로 만료됐다면 `classifyOMPProbeError`가 `timeout`을 붙이므로 값이 달라진다. 즉 데드라인을 올려도 고쳐지지 않는 신호이고, `ompDoctorTotalTimeout`·`ompReadinessBehaviorTimeout`은 릴리즈에 고정된 배포 동작이므로 추측으로 손대지 않는다. 대신 hermetic fixture가 프레임을 출력한 **뒤** `emitted-frames=N`을 invocation 로그에 남기고, healthy 단정이 실패하면 그 로그 전체를 실패 메시지에 첨부한다. 다음 발생에서 "자식이 프레임을 쓰지 못함"과 "썼지만 부모 capture에 도달하지 못함"이 한 번에 갈린다. fixture의 프레임 출력을 제거한 채 실행해 CI와 같은 서명(`event_missing`×3, `loopback-provider-requests=2`, receipt 정상)을 재현하고 새 진단이 `emitted-frames=0`으로 드러나는 것을 확인했다.

- **repo가 아닌 루트에 `.git/`을 조작 생성하던 root-local git hook 설치** (2026-08-17): `adapter.SupportsRootGitHooks`가 `.git` **부재**를 `true`로, `.git` 디렉터리는 내용을 보지 않고 `true`로 판정했다. 그래서 codex·opencode 어댑터가 `.git/hooks/`를 `MkdirAll`하며 git이 절대 실행하지 않는 `pre-commit`·`commit-msg`를 써넣었고, repo가 아닌 체크아웃(형제 repo를 담는 meta workspace)에는 `HEAD`·`config`·`objects`·`refs`가 없는 가짜 `.git/` 디렉터리가 남았다. 훅이 죽는 것보다 나쁜 부작용은 그 디렉터리가 다른 도구에게 "여기가 repo다"라고 거짓 신호를 준다는 점이다. 이제 판정은 읽을 수 있는 `.git/HEAD`를 요구한다 — linked worktree의 gitdir 파일은 종전대로 제외되고, `HEAD` 없는 `.git` 디렉터리는 repo가 아니라 이 버그의 잔여물로 취급한다. 나중에 `git init`하는 루트는 다음 `auto update`에서 훅을 받으므로 기능 손실은 없다. 호출자는 `codex_hooks.go`·`codex_transaction.go`·`opencode_plugin.go`·`opencode_transaction.go` 4곳이며, 루트 manifest 중 codex·opencode만 `.git/hooks/*`를 추적하던 이유가 정확히 이것이다. 어댑터가 필터한 결과로 manifest를 만들므로 stale 엔트리는 다음 update에서 자동 정리된다. 회귀 테스트로 4개 `.git` 상태(부재·worktree 파일·`HEAD` 있는 디렉터리·`HEAD` 없는 디렉터리)와 non-repo에서 `.git`이 생기지 않음을 어댑터 레벨에서 고정했다. 기존 테스트 4개가 bare temp dir을 repo처럼 쓰며 훅이 써지길 기대하고 있었는데 — 특히 헬퍼 `makeWorkspaceRepo`가 `HEAD` 없는 `.git`을 만들고 있었다 — 그 fixture들이 실제 gitdir을 만들도록 고쳐 훅 설치 경로를 진짜로 통과하게 했다. 빌드한 바이너리로 repo·non-repo 두 루트에 `init`을 실행해 확인했다: non-repo는 `.git` 미생성 + 서피스 정상 설치, real repo는 훅 2개 정상 설치.

- **`recheck`가 pane 터미널에서 자식 프로세스로 떨어지던 전송 선택** (2026-08-09): `runRecheck`가 `runProvider`를 직접 호출해 전송 계층을 통째로 우회했다. 그래서 cmux·tmux와 그 위에 올라간 Orca 터미널에서도 두 라운드가 pane이 아닌 자식 프로세스로 실행됐고, receipt의 `backend`는 실제 실행과 무관하게 항상 `subprocess`로 보고됐다. 이제 라운드마다 터미널에 맞는 전송을 고른다 — pane 지원 터미널은 `InteractivePaneBackend`, plain 터미널·강제 subprocess 모드·OMP 같은 에이전트 런타임은 직접 실행 — 그리고 evidence의 backend 이름은 실제로 실행한 전송을 따른다. `SelectBackend`는 쓰지 않는다: 그것이 돌려주는 subprocess 백엔드는 JSON 스키마를 강제하는데 `recheck`는 consensus·pipeline·relay와 같은 자유 텍스트 전략이다. pane 팬아웃 래퍼를 건너뛰는 가드는 유지되지만, 이제 전송이 아니라 래퍼만 건너뛴다.

- **pane 터미널에서 `recheck`가 재유도 없이 끝나던 라우팅** (2026-08-09): `RunOrchestra`는 전략 분기보다 먼저 pane 러너로 위임하므로, cmux·tmux 같은 pane 지원 터미널에서 `--strategy recheck`가 relay·debate·interactive 분기를 모두 비껴가 일반 pane 팬아웃으로 떨어졌다. 그 경로는 프로바이더당 1라운드만 실행하므로 재유도가 일어나지 않은 1라운드 답변이 `recheck` 결과로 반환됐다. 이제 `recheck`는 터미널 종류와 무관하게 자신의 2라운드 루프를 소유하며, 각 라운드의 전송은 위 항목대로 터미널에 맞춰 고른다. 두 진입점(`RunOrchestra`, `RunPaneOrchestra`) 모두에 가드를 두되 서로 위임하지 않아 순환하지 않는다. `auto orchestra`의 전략 목록 오류 메시지에도 빠져 있던 `recheck`를 추가했다.

- **Codex 최상위 모델 부재 시 세대를 건너뛰던 폴백** (2026-08-09): 요청한 Codex 모델이 런타임 카탈로그에 없으면 해결기가 카탈로그의 나머지를 무시하고 하드코딩된 `gpt-5.5`로 직행했다. 프론티어 모델 권한이 없는 ChatGPT 계정도 같은 세대의 균형 모델(`gpt-5.6-terra`)은 광고하므로, 최상위 티어 요청이 오히려 한 세대 낮은 모델로 떨어졌다 — 티어를 올리지 않았을 때보다 나쁜 결과다. 이제 프론티어 요청은 같은 세대 균형 모델을 먼저 시도한 뒤에만 legacy로 내려간다. 균형·소형 티어의 폴백은 바뀌지 않는다: 소형 티어는 명시적으로 싼 모델이므로 더 큰 모델의 대체재로 승격하지 않는다. 카탈로그에 아는 모델이 하나도 없으면 종전대로 런타임 기본값에 위임한다. 폴백은 이미 `Codex model fallback: requested=... selected=... reason=model_unavailable`로 stderr에 보고되고 있었고 그 형식도 그대로다.

### Added

- **규칙이 실제로 omp 세션에 도달하는지 관측하는 라이브 오라클** (2026-08-18): `TestOMPRules_LiveSessionReceivesEverySourceRule`은 격리 프로필로 실제 omp를 RPC 모드로 띄워 `/dump` 시스템 프롬프트와 `omp ttsr list --json`을 읽고, 방출된 frontmatter에서 도출한 기대 경로(트리거 → TTSR, `alwaysApply` → 본문 주입, 그 외 → `<domain-rules>`)와 관측을 대조해 세 경로의 합집합이 `content/rules/` 전체와 같은지 확인한다. 9/3/2 분해는 하드코딩하지 않고 합집합만 14로 고정하므로 분류 정책이 바뀌어도 오라클이 살아 있다. 기존 인수 테스트가 디스크상의 frontmatter 형태만 검사해 "생성은 됐지만 세션에 도달하지 않는" 상태를 녹색으로 통과시켰기 때문에 추가했다. 기본은 skip이고 `AUTOPUS_OMP_RULES_LIVE=1`에서 동작하며, CI의 `omp-native-smoke` 잡이 이미 검증해 둔 핀 바이너리(omp 17.2.7)를 PATH 심으로 재사용해 같은 잡에서 실행한다. 두 가지 변이(규칙을 옛 `.agents/rules/autopus/`로 되돌리기, `.omp/rules/autopus/` 하위 디렉터리로 옮기기)로 실패를 확인해 비공허성을 검증했다.

- **Gemini per-route 명령 표면 19개 완성** (2026-08-18): 생성기는 허용목록 없이 `templates/gemini/commands/auto`를 순회해 `.tmpl` 하나당 `.toml` 하나를 내보내므로, 8개만 나오던 원인은 템플릿 11개의 부재뿐이었다. `SPEC-ADK-ULTRA-EFFICIENCY-001`은 19-route 인벤토리를 고정하면서 Gemini 쪽은 skills 표면만 메웠고 command 표면은 건드리지 않았는데, Gemini 자신의 router 템플릿은 이미 19개를 광고하고 있었다. 빠져 있던 11개(`dev`·`doctor`·`goal`·`map`·`secure`·`setup`·`status`·`test`·`update`·`verify`·`why`)를 추가하고 전체 집합을 회귀 테스트로 고정했다.

- **`balanced` 프리셋에서 executor를 최상위 티어로 승격** (2026-08-09): 기본 품질 프리셋이 `planner`·`architect`·`spec-writer`·`security-auditor`에만 주던 최상위 티어를 `executor`에도 준다. 근거: 여유가 남아 있던 유일한 Go 시맨틱 과제군에서 프론티어 티어가 42/45, 중간 티어가 36/45였고 어떤 멀티에이전트 구성도 그 격차를 메우지 못했다(최선의 논증 장치는 5콜에 40/45). 그 문항들은 "코드가 정확히 무엇을 하는지 예측"하는 executor형 과제이며, executor의 산출물은 이후 모든 역할이 검토하는 대상이다. 측정된 적 없는 열린 판단 역할(planner·architect·spec-writer)의 티어는 근거 부재를 강등 사유로 삼지 않고 그대로 둔다. Codex 매핑에서 최상위 티어는 모델과 reasoning effort를 함께 올리므로 executor는 `gpt-5.6-terra`/`medium`에서 `gpt-5.6-sol`/`xhigh`로 바뀌며, 카탈로그가 해당 모델을 광고하지 않는 환경에서는 기존 opus 역할과 동일하게 `gpt-5.5`로 폴백한다. `tester`·`reviewer`·`debugger`·`devops`·`validator`·`explorer`는 중간 티어로 남고 `ultra` 프리셋은 바뀌지 않는다.

- **합의 미달 차단 게이트 `--require-agreement`** (2026-08-09): `auto orchestra run --strategy consensus --require-agreement <0-1>`은 `ConsensusMetrics.AgreementRatio`가 지정한 하한 미만이면 실행을 `blocked`로 끝낸다 — `terminal_state`와 `gate_status`가 `blocked`, `degraded_reasons`에 `agreement_below_floor`가 붙고, receipt를 먼저 방출한 뒤 0이 아닌 코드로 종료한다. 근거: 45문항 2중 함정 과제를 두 독립 계보로 풀렸을 때 두 provider가 합의한 답만 채택하면 정확도가 61%에서 90%로 올랐고 불일치가 결함 문항의 93%를 잡았다. 반대로 그 불일치를 모델 라운드에 되먹이는 것은 90 model-question 통합 측정에서 아무 효과가 없었으므로(`p=1.000`), 이 신호는 생성기가 아니라 사람·게이트로 향하는 트립와이어로만 노출한다. 하한은 포함 비교이며 `0`(기본)은 게이트를 끄므로 기존 호출자의 동작은 바뀌지 않는다. 범위 밖 값과 consensus 아닌 전략에서의 사용은 실행 전에 거부한다.

- **단일 프로바이더 재검토 전략 `recheck`** (2026-08-09): `auto orchestra run --strategy recheck`는 프로바이더 하나를 두 라운드 실행한다. 1라운드는 독립 응답이고, 2라운드는 원 과제와 **자기 자신의 1라운드 답변만** 보여주며 재유도를 요구한다. 피어 출력은 포함하지 않는다. 근거: 프론티어 쌍(`gpt-5.6-sol` + `claude-sonnet-5`)으로 4중 함정 45문항을 측정했을 때 강한 참가자에게 피어 내용은 아무 값이 없었지만(peers-only 대 own-only 불일치 1대1, `p=1.000`) 재유도 자체는 39/45에서 44/45로 올렸고, 이는 두 모델 중 하나라도 맞힌 oracle 상한과 같다. 같은 45문항에서 논증 장치 전체(2모델×2라운드+판사)는 40/45에 5콜을 썼고 `recheck`는 44/45에 2콜을 쓴다. 최종 답은 항상 재유도된 2라운드 답변이며, 1라운드 답변은 `Responses`·`RoundHistory`·`orchestration_run_receipt.v1`의 `recheck_r1`/`recheck_r2` provider receipt에 증거로 남는다. 프로바이더를 2개 이상 넘기면 fan-out을 조용히 버리지 않고 실패한다. 기존 다섯 전략의 동작은 바뀌지 않는다.

- **Orchestra run의 기계 판독 가능한 합의 증거 노출** (2026-08-08): `auto orchestra run`은 provider 합의·이견·quorum·veto를 이미 `orchestration_run_receipt.v1`로 계산하고 있었지만 CLI 경계에서 rendered markdown과 사람용 요약만 남기고 버렸으므로, 스크립트가 provider 불일치로 게이트할 방법이 없었다. `--json`/`--format json`이 공용 CLI 봉투로 그 receipt를 그대로 내보내고, gate가 막힌 실행도 실패를 반환하기 전에 receipt를 먼저 방출해 차단 사유를 증거로 남긴다. 봉투 status는 degraded 실행을 `warn`으로 투영해 호출자가 markdown을 파싱하지 않고 분기할 수 있다. `ConsensusMetrics`에는 `agreement_ratio`를 추가해 claim이 하나도 없는 경우를 `1`로 정의하고 항상 직렬화하므로, 각 호출자가 0으로 나누는 계산을 재구현하지 않는다. 근거: 두 독립 계보로 45문항 2중 함정 과제를 측정했을 때 두 provider가 합의한 답만 채택하면 정확도가 61%에서 90%로 올랐고, 불일치가 결함 문항의 93%를 잡았다. 기본 text 출력과 기존 종료 코드는 바뀌지 않는다.

- **OMP 네이티브 멀티에이전트 실행·모델 라우팅·컨텍스트 승격 완결 (SPEC-OMP-002/003/004)** (2026-08-04): `/auto go`의 생성 표면을 OMP 자체 `task`·`hub`·`todo`가 소유하는 단일 DAG로 전환하고, planner→executor→validator/reviewer/security 순서와 격리 재시도, 정확한 다섯 필드 receipt, fail-closed completion gate를 20개 workflow skill에 고정했다. child task는 기본적으로 부모 모델을 상속하며 opt-in 역할 프로필만 `omp models --json --no-extensions`의 실제 capability catalog에서 검증된 selector·thinking으로 투영하고, required route의 미해결·비호환 후보는 실행 전에 차단한다. active OMP RPC는 session·binding·model scope·config hash를 다시 검증한 뒤 body-free native route와 bounded context plan을 정확히 한 번 승격하고, stale/partial 응답과 다른 프로세스·세션의 결과를 거부한다. 생성·갱신은 rooted transaction, 0600 journal/backup/receipt, credential-free reverse delta, doctor freshness·ownership 검증으로 원자화하고, 설정과 산출물이 같으면 byte-stable no-op을 보장한다. Orca orchestration은 사용자 요청이 명시된 durable cross-worktree run에만 선택하며 OMP-local 기본 경로와 중첩 DAG를 만들지 않는다.

- **v0.50.98 OMP production evidence의 로컬 단일 실행 경계** (2026-08-04): GitHub Actions가 provider secret으로 별도 canary와 승격을 반복하던 두 workflow를 제거하고, release operator가 한 번 시작하는 `prepare-release.sh --apply` 안에서 deterministic 40-call AB/BA plan, 실제 candidate, provider credential을 root-owned sandbox로 격리해 `nobody`로 순차 실행한다. `gpt-5.6-sol`의 272k model window에 맞춘 9/9/2 split은 마지막 2-call segment가 compaction 전 reset pair만 추가해 cohort median reduction을 왜곡했으므로, production authority를 `320000` model window와 동일한 결정적 10/10 paired segment로 함께 올린다. 각 segment는 fresh full/optimized OMP RPC process·session을 사용하고 session sequence reset, 네 receipt의 전역 유일성, segment 내부 process/session reuse, provider authority 불변을 검증해 baseline만 유리하게 초기화하거나 segment 간 history를 섞는 증거를 거부한다. 성공 report는 두 process/session start를 각 variant에 고정하고 회전된 K2 키로 서명해 active freshness, exact source/candidate/static policy, credential-free public HTTPS orphan-tag readback을 검증한 뒤 release coordinate를 원자적으로 게시한다. Release Actions는 같은 immutable evidence를 역사 모드로 재검증만 하며 provider credential을 보유하거나 canary를 재실행하지 않는다.

- **긴 세션의 sticky 규칙 재부착 (SPEC-STICKYRULE-001)** (2026-08-02): `alwaysApply: true`를 선언한 규칙을 고정된 프롬프트 주기마다 다시 주입해, 모든 응답을 지배하는 지시가 세션 시작에만 있지 않고 긴 대화 후반에도 살아 있게 한다. `alwaysApply`는 기존 세 값 분류(always/paths-scoped/hook-fired) 위에 얹히는 직교 boolean 플래그이며 네 번째 클래스가 아니고, `hook-fired`와의 조합은 검증 단계에서 거부한다. sticky 규칙이 하나 이상이고 플랫폼이 claude-code일 때만 컴파일러가 기존 매니페스트 `.claude/hooks/autopus/conditional-rules.json`에 sticky 집합을 기록하고 `auto rules sticky --event UserPromptSubmit`를 실행하는 timeout 5의 `UserPromptSubmit` 훅 항목 하나를 등록한다(두 번째 매니페스트는 만들지 않는다). codex·gemini·opencode·omp에는 항목도 sticky 매니페스트도 만들지 않으며, 규칙 frontmatter를 보존하는 플랫폼에서는 `alwaysApply`를 원문 그대로 남긴다. 런타임은 `.claude`를 가진 최근접 상위 디렉터리를 프로젝트 루트로 잡고, gitignore된 `.autopus/runtime/sticky-rules/` 아래에 raw session_id의 소문자 hex SHA-256을 파일명으로 쓰는 세션별 프롬프트 카운터를 유지해 경로 순회를 표현 불가능하게 만든다. 주입은 첫 프롬프트와 (index-1) mod N == 0인 프롬프트에서 일어나고 N은 `autopus.yaml`의 `hooks.sticky_cadence`(없거나 0 이하이면 기본 8)에서 온다. 출력은 `hookSpecificOutput.hookEventName`이 UserPromptSubmit인 구조화 형식으로 규칙 이름과 frontmatter를 제거한 본문을 `additionalContext`에 담으며, 본문당 4000바이트와 합계 6000바이트 상한에서 규칙 이름 역순으로 떨어뜨린 뒤 한 줄 절단 고지를 붙이고, 상태는 7일 보존·200개 상한으로 정리한다. UserPromptSubmit의 종료 코드 2는 사용자 프롬프트를 지우므로 모든 경로에서 0으로 종료하고(최상위 deferred recover가 부분 stdout을 폐기), 프롬프트 본문·raw session_id·transcript 경로·작업 디렉터리는 기록하지 않는다. 상태 디렉터리 구성요소와 카운터 파일 leaf를 모두 `os.Root` 프레임에서 일반 파일·하드링크(link count) 검사를 거쳐 열므로 저장소가 심어둔 심볼릭 링크나 하드링크로 체크아웃 밖을 생성·덮어쓰기·삭제할 수 없고, 봉쇄 거부는 stdout·stderr 없이 종료 코드 0인 무해한 whole-run 결과다. `UserPromptSubmit`은 이벤트 키가 아니라 항목 단위로 소유하므로 사용자가 직접 작성한 같은 이벤트 훅은 재생성에서 살아남고 낡은 autopus 항목만 회수된다. 배송된 sticky 규칙은 `language-policy`와 `objective-reasoning`이고, `auto rules sticky`가 새로 생겼으며 `auto rules list`에 sticky 열과 유효 주기 표시가 추가됐다. 커버리지는 `pkg/rulecond` 88.7%, `pkg/config` 89.1%, `pkg/content` 93.8%, `pkg/adapter/claude` 86.9%, CLI sticky 파일 87–92%이고, 코드 리뷰는 blocker 0으로 APPROVE, 적대적 보안 감사는 3라운드에서 PASS로 수렴했다(2라운드가 git clone PoC로 leaf 파일 봉쇄 갭을 재개했고 3라운드가 6000회 경합·FIFO·상대 경로 심링크·하드링크에 대해 폐쇄를 확인). 4000바이트 상한을 주입 본문이 아니라 소스 파일 전체에서 측정하는 fail-closed 선택과 UserPromptSubmit의 항목 단위 소유권은 수용된 편차로 SPEC의 `research.md`에 기록했다.

- **omp(oh-my-pi) 플랫폼 최소 표면 지원 (SPEC-OMP-001)** (2026-08-01): `pkg/adapter/omp/` 어댑터가 규칙 14종을 `.agents/rules/autopus/`로, 에이전트 16종을 `.omp/agents/`로 방출하고 사용자 소유 `.omp/config.yml`에는 마커 관리 섹션만 기록한다. frontmatter 인식 키가 없는 규칙은 결정론적 description 합성으로 omp 세션 발견을 보장한다(라이브 RPC 실측 14/14, trigger 규칙 3종은 TTSR 라우팅 유지). 스킬은 opencode>codex>omp 전순서 양보, 커맨드는 opencode에만 양보하고 antigravity와 md/toml 확장자 분리로 공존한다. Clean/Update는 manifest 기록·체크섬 미변경 경로만 3층 심링크 봉쇄(`SafeWorkspacePath`/`safeTransactionPath`/`SafePruneFilePath`) 하에 개별 제거하고, 설정 부재 시 `config.Exists` 게이트로 fail-closed하며, 마커 재작성은 멀티-도큐먼트 스칼라·중첩 주석 검사로 사용자 값 무음 손실을 거부한다. `.omp/config.yml`은 0600 유지, PATH 상 동명 바이너리는 `omp/` 버전 접두 신원 게이트로 차단(REQ-019), orchestra provider 자동 등록 금지(REQ-018). 커버리지 87.7%, 뮤테이션 11/11 kill, 보안 감사 3라운드 Critical/High 0 + 적대적 폐쇄 검증 PASS.

- **컨텍스트 엔지니어링 진화와 안전한 worker receipt (SPEC-CONTEXT-ENGINEERING-EVOLUTION-001)** (2026-07-27): 검증된 `autopus.context_delivery.v1` 전문을 계속 authoritative full delivery로 유지하면서 body-free `autopus.context_plan.v2` shadow sidecar와 `auto workflow context-plan`을 추가했다. 새 plan은 memindex의 fresh projection에서 pinned/selected ref·hash, full/JIT token estimate, delta, reduction, omitted count, 선택 hit metric을 계산하되 raw query·본문·provider payload는 직렬화하지 않고 projection 장애를 non-gating `unavailable`로 축약한다. provider output의 명시적 terminal marker에만 exact five-field `autopus.worker_receipt.v1`을 적용하며 duplicate/trailing/oversized JSON, unsafe ref, secret·raw prompt·절대 경로 자유 텍스트를 evidence gate에서 차단한다. Claude와 Gemini의 opt-in full/split/full skill compiler를 native/mirror root에 정렬하고 stale prune의 symlink escape를 거부하며, Gemini는 source tool을 native allowlist로 투영하되 unsupported capability와 managed Claude-team permission만 제거하고 사용자 settings 문자열은 보존한다.

- **실행 scenario warn/enforce admission migration (SPEC-SCENARIO-PARSER-MIGRATION-001)** (2026-07-27): `S1`, `S15A`, `S-CANARY-1` ref를 exact round-trip하고 malformed header·unknown/duplicate/non-canonical field를 stable reason code로 진단한다. `auto test run`은 기본 `warn`에서 invalid entry를 build/shell 전에 quarantine하고 `--scenario-validation=enforce`에서는 같은 diagnostics로 실행 전에 실패한다. explicit valid `active`만 runnable이며 invalid-only 입력은 fabricated PASS 없이 `invalid` count를 반환한다. `exit_code`, stdout/stderr, relative file verification primitive는 실제 결과를 평가하고 unknown/malformed primitive와 Unix·Windows absolute artifact path는 fail closed한다. strict-default 승격은 기존 sparse inventory 정리와 별도 승인까지 Completion Debt로 남긴다.

- **네 플랫폼 컨텍스트 엔지니어링 계약 정렬 (SPEC-CONTEXT-ENGINEERING-001)** (2026-07-27): Claude Code, Codex, OpenCode, Gemini/Antigravity가 명령별 required·worker-optional·excluded 문서 집합을 같은 의미로 생성하도록 정렬했다. `go`는 검증된 supervisor 전문 전달과 bounded worker recall을 분리하고, worker handoff의 정확한 다섯 필드를 유지한다. 선택적 세부 정보는 프로젝트 상대 경로의 일반 파일만 참조하며 절대 경로·상위 순회·심볼릭 링크·비정규 파일을 거부하고, sanitize/redact와 injection evidence 보존, 선택 ref/hash 및 생략 수 기록, raw provider/tool 결과와 반복 본문 재전송 금지를 요구한다. 각 플랫폼의 scratch 생성 경로와 Antigravity mirror, 실제 pipeline 참조 도달성, 부정형 보안 fixture, doctor 공개 심볼·JSON ID, 기존 full-delivery hash 무결성, 실행 가능한 scenario wire를 회귀 테스트로 고정했다.

- **Claude/Codex provider별 Quality Mode** (2026-07-25): 기존 `quality.default`를 하위 호환 fallback으로 유지하면서 `quality.providers.claude|codex` override와 `auto quality provider <claude|claude-code|codex> <ultra|balanced|inherit> --apply`를 추가했다. Claude workflow dispatcher와 Codex root·agents·orchestra가 각 provider의 effective mode를 독립적으로 사용하며, provider별 apply는 해당 configured platform만 갱신한다. 명시적 per-run `--quality`는 저장된 두 override보다 우선하되 YAML을 바꾸지 않고, raw config comment·env placeholder·future field·file mode는 atomic update에서 보존한다. Custom preset 이름은 1–64자의 ASCII 영숫자로 시작하고 이후 영숫자·하이픈·밑줄만 허용하며, YAML scalar encoding과 저장 전 semantic validation으로 config injection을 차단한다.

- **Claude Opus 5 기본 모델 경로 및 Claude Code 2.1.219 경계** (2026-07-25): Anthropic의 고정 모델 ID `claude-opus-5`를 workflow fail-closed whitelist와 $5/$25 per MTok 가격표, Ultra `max` effort에 추가하고 Claude Ultra·Balanced 전략 역할·high-complexity 라우팅·기본 premium 설정을 Opus 5로 승격했다. Claude Code의 `opus` alias는 지원 provider에서 `2.1.219` 이상부터 Opus 5를 선택하므로 `route_team` doctor는 `2.1.219`로 fail-closed하되 model-free `route_a`는 기존 `2.1.154` 호환성을 유지하고, provider별 alias 차이를 source/generated 가이드에 명시했다. `claude-opus-4-8`은 계속 선택 가능한 모델이자 Opus 5 사이버보안 거부의 권장 fallback이므로 whitelist·가격·effort 호환 경로에 보존하고, Fable 5는 기존처럼 명시적 opt-in으로 유지한다.

- **Claude Fable 5 opt-in 및 최신 effort 전달** (2026-07-23): 기본 Opus/Sonnet 매핑은 유지하면서 `claude-fable-5`와 `fable`/`best`를 명시적 선택으로 지원하고, resolved full ID 가격($10/$50 per MTok), worker·orchestra·route-team `--effort` 전달을 연결했다. model/API effort는 `low`~`max` 다섯 값으로 유지하며 Claude Code session-only `ultracode`는 binding에서 실제 `xhigh`로 정규화한다. Fable 권한·ZDR와 Claude Code `2.1.170`/`2.1.203`/`2.1.210` 버전 경계도 source/generated 가이드에 동기화했다.

### Fixed

- **v0.50.103 게시 후 재검증 digest 형태 정정과 A22 좌표 전환** (2026-08-08): v0.50.102는 서명·공증된 15개 자산을 immutable release로 게시하는 데 성공했지만, 게시 직후 `verify-current-release.sh`가 published promotion report의 `.candidate.artifact_sha256`을 맨 hex로만 받아들여 `released OMP candidate artifact digest is malformed`로 중단했다. Canary는 언제나 `sha256:` 접두 형태를 기록하고 `validate_canary`도 그 형태를 요구하므로 이 검사는 실제 release에서 성립할 수 없었고, 이전 좌표들이 더 앞 단계에서 실패해 드러나지 않았다. 검증기는 report가 실제로 쓰는 접두 형태를 받아 접두만 제거해 lineage verifier에는 `sha256:` 접두를, `ompcontextverify`에는 맨 hex를 그대로 전달하며, current-release fixture도 같은 잘못된 형태를 담고 있었으므로 실제 release가 만드는 접두 형태로 맞춘다. 이 실패로 Homebrew Cask 단계가 skip됐고 tag가 트리거한 workflow는 그 tag 커밋의 스크립트를 사용하므로, 이미 게시된 v0.50.102 release와 tag는 그대로 보존하고 active A22 workflow·lineage·Homebrew·current-release 좌표를 v0.50.103으로 전환한다.

- **v0.50.102 release asset glob 경로 정정과 A22 좌표 전환** (2026-08-08): v0.50.101은 40-call cohort 42/42, 14/14 gate, `omp-production-evidence` CI 검증까지 통과했지만 GoReleaser가 `extra_files` glob을 `io/fs`로 해석하면서 절대 경로를 `invalid argument`로 거부해 자산을 하나도 게시하지 못했다. 같은 설정의 lineage glob은 `dist/...` 상대 경로라 정상이었으므로, 검증된 promotion report·attestation을 gitignore된 `.autopus/runtime/release-evidence/`에 staging하고 두 glob을 저장소 상대 경로로 맞춘다. Staging 단계는 복사본 digest를 pinned `OMP_CONTEXT_EVIDENCE_*_SHA256`와 재대조하고 worktree가 더러워지지 않았음을 확인해 fail-closed를 유지한다. Tag가 트리거한 workflow는 그 tag 커밋의 파일을 사용하므로 이미 공개된 v0.50.101 source·evidence tag는 이동하지 않고 release가 없는 실패 좌표로 보존하며, active A22 workflow·lineage·Homebrew·current-release 좌표를 v0.50.102로 전환한다.

- **v0.50.101 production execution smoke의 signed artifact 격리 staging, RPC lifecycle 경합, evaluator session 예산과 release authority 정정** (2026-08-08): v0.50.100 release는 signed·notarized arm64 companion을 만든 뒤 GoReleaser workspace의 상위 경로를 `nobody`가 순회할 수 없어 UID-isolated execution smoke에서 올바르게 중단됐다. arm64 execution gate는 OMP canary root의 canonical `auto` 경로에 exact signed bytes를 root-owned `0555`로 materialize하되, release-prep이 이미 같은 canonical 경로에 배치한 artifact는 identity·mode·stable digest 검증 후 그대로 재사용한다. source와 실행 artifact의 digest를 실행 전후 검증하고, gate가 만든 copy만 성공·실패 모두 privilege-pinned cleanup으로 제거하며 caller가 소유한 pre-staged artifact는 상위 canary cleanup에 남긴다. OMP v17.2.7 RPC dispatcher는 `session.prompt`를 시작한 뒤 success response를 쓰므로 `agent_start`·`turn_start`가 response보다 먼저 도착할 수 있고, 프롬프트 하나가 `agentLoopContinue` 재시도로 여러 agent 사이클을 흘리는 동안 wire-level `agent_end`는 in-flight prompt가 풀릴 때까지 보류된다. managed·canonical RPC lifecycle은 반복되는 start·turn 사이클을 하나의 primary provider run으로 수용하고 `isTerminal != false`인 terminal `agent_end`에서만 완료로 판정하며, terminal 이후 도착한 late response, 열린 turn 안의 start, 완료된 turn이 없는 terminal end, 중복·비상관 frame은 계속 fail-closed한다. Evaluator child의 `--max-time`은 OMP 세션 전체를 중단시키는데 2분 고정값이 segment workload(pair당 primary turn + optimized 재사용 call마다 compaction turn = 최대 19 turn)보다 짧아 provider 지연에 따라 cohort 중간에서 turn이 `aborted`로 끊기고 usage delta가 0으로 관측됐으므로, 예산을 segment workload에서 유도해 pair당 45초 기준 15분 세션·60분 전체로 올린다. Release authority는 제품 `standard` tier 기본값인 `anthropic/claude-sonnet-5`의 `1000000` window로 전환한다. full variant가 segment 안에서 history를 누적해 마지막 pair가 약 `296000` token 단일 요청에 도달하므로 200k hard limit 모델은 이 cohort를 실행할 수 없고, 구독 자격으로 `311828` token 요청이 통과함을 실측했다. 원본 artifact의 manifest binding을 유지하고, 이미 공개된 v0.50.100 source·evidence tag는 이동하지 않고 release가 없는 실패 좌표로 보존한다.

- **v0.50.100 remote-only evidence tag 검증과 immutable coordinate 수렴** (2026-08-04): v0.50.99 final cohort와 K2 evidence 검증은 42/42 records로 성공했지만, 격리된 publish repository가 evidence tag를 원격에 게시한 뒤 root release repository에는 그 ref가 없어서 coordinate publisher가 이미 검증된 remote tag를 `local evidence tag is not annotated`로 거부했다. Publisher는 이제 advertised remote ref가 exact tag-object SHA와 일치함을 먼저 확인하고 그 ref를 `--no-tags`로 fetch한 뒤 전달된 object SHA에서 annotated-tag·orphan commit·tree 좌표를 독립 검증한다. 회귀 fixture는 local tag와 unreachable object를 제거한 remote-only 상태에서 transaction 전체를 실행한다. 공개된 v0.50.98·v0.50.99 evidence tag는 이동·삭제하지 않고 release가 없는 실패 좌표로 보존하며, current A22 release 좌표는 v0.50.100으로 전환한다.

- **v0.50.99 evidence publication scope와 immutable coordinate 수렴** (2026-08-04): `publish_local_evidence`의 local `evidence_tree`가 Bash dynamic scope로 `load_evidence`가 복원해야 할 authoritative evidence tree를 가려, immutable `omp-context-evidence-v0.50.98`를 게시·검증한 직후 coordinate publication이 `set -u`로 중단되던 경계를 수정했다. Local orphan commit 작성에는 별도 `published_tree`를 사용해 검증된 tree coordinate를 보존한다. 이미 공개된 v0.50.98 evidence tag는 이동·삭제하지 않고 release가 생성되지 않은 실패 좌표로 남기며, workflow·preflight·promotion·lineage·Homebrew·current-release verifier와 회귀 fixture를 모두 v0.50.99로 원자적으로 전환한다.

- **v0.50.98 local release smoke의 sticky shared-root 판정 수정** (2026-08-07): 40-call final production cohort가 42/42 records를 완료한 뒤, execution smoke가 root-owned `0755` canary 아래의 candidate artifact까지 `/private/tmp`의 일반 write bit만 보고 거부하던 경계를 수정했다. Artifact와 모든 내부 ancestor의 기존 read/execute·non-write·non-target-owner 검사는 유지하고, target이 쓸 수 있는 ancestor는 trusted privilege-runner owner가 소유하며 sticky bit가 있는 shared directory일 때만 허용한다. Untrusted sticky owner, target-owned ancestor와 sticky 없는 writable parent는 계속 fail-closed한다.

- **v0.50.98 native snapcompact 모델 capability 보존** (2026-08-04): production OMP catalog가 모든 provider 모델을 `input: [text]`로 덮어써 image-capable `openai-codex/gpt-5.6-sol`까지 LLM summary fallback으로 밀어내던 경계를 수정했다. 격리 catalog는 pinned OMP 17.2.7의 exact provider/model capability를 상속하고 unknown 모델은 OMP의 text-only 기본값을 유지한다. managed active startup은 선택된 모델 identity와 `text,image` input capability를 실제 `get_state`에서 확인하므로 native snapcompact를 수행할 수 없는 모델은 첫 provider call 전에 차단된다. snapcompact가 transcript에 생성하는 PNG frame은 pinned OMP `ImageContent`의 optional `detail` 값(`auto|low|high|original`)과 `CompactionSummaryMessage.blocks/images` 계약만 허용하고 전체 객체 digest를 compaction provenance에 묶는다.

- **v0.50.98 A22 결정적 재빌드·예약 수렴·canary 진단 강화** (2026-08-06): `v0.50.97` release job에서 pinned GoReleaser v2.17.0이 재생성한 unsigned binary와 preflight signed candidate의 서명 전 byte가 달랐으므로 실패 좌표를 재사용하지 않고 active A22 좌표를 `v0.50.98`로 전환했다. canonical candidate builder는 GoReleaser가 주입하는 8자리 short commit과 UTC `YYYY-MM-DDTHH:MM:SSZ` commit timestamp를 정확히 재현하며, operator-owned draft 생성 직후 GitHub Release 목록의 eventual consistency를 bounded retry로 흡수한다. bootstrap/final 40-call production canary는 시작·완료 진행률을 기록하고 실패 시 label, transcript record 수, exit status와 구조화된 `error_code`를 보존해 장시간 무출력과 원인 불명 실패를 구분한다.

- **v0.50.97 A22 릴리스 좌표 재수렴** (2026-08-06): immutable `omp-context-evidence-v0.50.96`이 이미 v0.50.96 source에 바인딩된 뒤 publisher의 지원되지 않는 `gh variable list --limit` 옵션이 coordinate transaction을 차단했으므로, 증거 태그를 이동하거나 exact-source 실행 경계를 우회하지 않고 current A22 좌표를 `v0.50.97`로 전환했다. publisher는 공식 GitHub CLI list 계약으로 repository·protected environment snapshot을 읽고, workflow·preflight·promotion·lineage·Homebrew·current-release verifier와 회귀 fixture를 모두 새 tag/version에 원자적으로 맞춘다.

- **v0.50.96 A22 원자적 release-prep와 macOS 사전 검증** (2026-08-05): `scripts/companion-release/prepare-release.sh`가 clean exact `origin/main`의 고정 blob에서 runtime helper를 private FD로 materialize한 뒤 provisional/final 40-call production canary, canonical static policy 재생성, policy-bound candidate 재빌드, `macos-15` preflight, orphan evidence 승격·검증을 수행한다. repository와 protected environment의 8개 좌표 및 exact tag policy는 snapshot/CAS rollback으로 수렴하며, publisher는 paginated GitHub Release 전체 목록에서 tag와 GoReleaser draft name이 각각 singleton이고 같은 operator-owned ID인지 검증해 exact coordinate marker draft를 먼저 예약한다. create 응답 유실은 remote state로 reconcile하고, fresh/retained prep lock·삭제 실패·소유권 충돌은 fail-forward status와 lock retention으로 복구한다. 마지막 commit point는 signed annotated `v0.50.96` tag 생성과 prep lock 삭제의 atomic push이며, protected release job은 같은 draft ID만 채워 게시하고 exact source commit, immutable 상태, 15개 unique asset 이름과 GitHub SHA-256 digest를 재검증한 뒤에만 Homebrew 경로를 연다. release와 preflight는 동일한 root-owned OMP canary materialize/remove helper를 사용하므로 runner 실행 경로도 tag 게시 전에 실제 검증된다.

- **v0.50.96 release-prep 환경 상속·cleanup 경계** (2026-08-05): protected environment에 명시적 override가 없으면 GitHub Actions와 같은 repository variable 상속 의미를 사용하고, override가 있으면 repository 값과 exact-match하지 않을 때 차단한다. 빈 root-owned canary 목록의 cleanup은 macOS 기본 Bash 3.2에서 `set -u` 오류 없이 종료하며, `nobody`의 unsigned ID 표현 `4294967294`를 `chown` operand로 재해석하지 않고 canonical account/group name으로 소유권을 설정한다.

- **v0.50.96 live canary 결정성 복구** (2026-08-06): 20-pair release-prep cohort가 모호한 평가 문구로 모델의 tool 호출을 허용해 단일-turn lifecycle gate를 깨뜨리던 문제를 수정했다. 각 pair는 이제 tool·command 실행과 부가 텍스트를 명시적으로 금지하고 task index에 바인딩된 정확한 한 줄 출력을 요구해, AB/BA variant가 동일한 observable oracle을 결정적으로 비교한다.

- **v0.50.96 canary 검증·cleanup의 Bash 3.2 상태 보존 수정** (2026-08-06): production canary 완료 후 `validate_canary`가 하나의 `local` 명령에서 `project`를 바인딩하면서 동시에 `$project` 기반 report 경로를 확장해 `set -u`에서 중단되던 문제와, `EXIT` trap 함수 진입 후 `$?`를 읽어 그 실패 상태를 `0`으로 덮던 문제를 수정했다. positional 인자를 먼저 바인딩한 뒤 report 경로를 별도 선언에서 파생하고, trap이 원래 status를 명시적 인자로 전달한다. 누락 report가 의도한 fail-closed 오류에 도달하고 cleanup이 실패 상태와 임시 디렉터리 삭제를 함께 보존하는 회귀 테스트를 추가했다.

- **v0.50.96 macOS preflight의 reserved tag·canonical artifact 경계 수정** (2026-08-06): `workflow_dispatch` preflight가 GitHub reserved `GITHUB_REF_NAME`을 덮으려 해 candidate build가 `main`을 보고 중단되던 문제, 비정규 basename을 사용하던 문제, private runner 경로의 artifact를 `nobody` smoke에 넘기던 문제를 수정했다. builder는 전용 `COMPANION_RELEASE_TAG`만 신뢰한다. preflight는 root-owned·non-writable `/Library` 아래에 run-scoped 0755 staging root와 canonical 0555 `auto`를 materialize해 전체 ancestor traversal 조건을 만족시키고, exact path·ownership·mode를 재검증한 뒤 success/failure 모두에서 staging과 OMP canary를 제거한다.

- **v0.50.96 verified-exec OMP 기동 경계 수정** (2026-08-06): authority-free smoke가 생성하던 synthetic model catalog의 `contextWindow: 0` 때문에 OMP 17.2.7이 RPC ready 전에 설정을 거부하던 문제를 수정했다. catalog에는 managed active 기본값 `262144`를 명시하고, root-owned binary의 cold start가 기존 8초를 넘는 macOS runner에서도 바깥 30초 gate 안에서 완료되도록 내부 readiness 한도를 20초로 조정했다. 테스트 OMP fixture도 0 이하 context window를 거부해 같은 회귀를 차단한다.

- **v0.50.96 macOS execsmoke process-group 종료 순서 수정** (2026-08-06): timeout 시 `exec.Cmd.Cancel`이 격리 process group을 이미 종료한 뒤 reaped leader PID로 동일한 group에 두 번째 `SIGKILL`을 보내, 빠르게 재사용된 PGID에서 `EPERM`을 반환하며 원래 timeout 결과를 덮던 경쟁을 제거했다. deadline은 cancel 완료 직후 `errExecutionTimeout`으로 확정하고, 정상 종료·inherited-pipe 경로만 후속 group cleanup을 수행한다. subprocess 기반 smoke 테스트는 직렬화하고 UID-isolated CLI fixture 한도를 10초로 넓혀 race build의 cold start 부하에도 결정적으로 검증한다.

- **v0.50.96 evidence-only 실패·미게시 시도 보존** (2026-08-06): source `8877f03fc3b43fe1d4c19688e116ce50b0eef16e`의 두 40-call production canary, `macos-15` preflight와 signed orphan evidence 승격은 통과했지만, publisher가 `gh variable list`에 지원되지 않는 `--limit` 옵션을 전달해 release coordinate transaction 진입 전에 중단되었다. annotated `omp-context-evidence-v0.50.96` tag는 해당 source의 immutable 증거로 보존하며 삭제·이동·재사용하지 않는다. `v0.50.96` source tag와 GitHub Release는 생성되지 않았고 repository/environment 좌표와 Homebrew Formula/Cask도 변경되지 않았다.

- **v0.50.95 실패·미게시 릴리스 시도 보존** (2026-08-05): production evidence, 전체 CI·보안·`83.2%` coverage는 통과했고 protected environment의 exact tag allowlist와 environment-level source pin도 수렴했지만, `macos-15` release job이 존재하지 않는 `/usr/bin/test`를 호출해 root-owned OMP canary materialization 단계에서 중단되었다. `v0.50.95` source tag와 zero-parent annotated `omp-context-evidence-v0.50.95` tag는 immutable 실패 기록으로 보존하며 삭제·이동·재사용하지 않는다. GitHub Release는 생성되지 않았고 Homebrew Formula/Cask도 변경되지 않았다.

- **v0.50.94 실패·미게시 릴리스 시도 보존** (2026-08-04): A22 재시도는 변경하지 않은 전체 커버리지 기준 `83.0%`에 대해 `82.9%`가 두 차례 관측되어 게시 전에 중단되었다. `v0.50.94` source tag와 `omp-context-evidence-v0.50.94` tag/ref는 이 실패 시도의 immutable 기록이며 current release 좌표가 아니고 삭제·이동·재사용하지 않는다. 이 시도에서는 GitHub Release가 생성·게시되지 않았고 Homebrew Formula/Cask도 변경되지 않았다.

- **v0.50.93 실패·미게시 릴리스 시도 보존** (2026-08-04): A22 승격은 promotion attestation에 서명하고 zero-parent(orphan) `omp-context-evidence-v0.50.93` annotated tag를 성공적으로 게시했지만, 승격 산출물 이름이 `promotion-report-v1.json`인 반면 release extractor는 `omp-context-promotion-report.v1.json`을 요구했고 evidence tag object·commit 및 report·attestation SHA-256 repository variable도 처음에는 설정되어 있지 않아 release가 artifact 생성 전에 중단되었다. `v0.50.93` source tag와 orphan evidence tag는 실패 시도의 immutable 기록으로 그대로 보존하며 삭제·이동·재사용하지 않는다. 이 시도에서는 GitHub Release가 생성·게시되지 않았고 Homebrew Formula/Cask도 변경되지 않았다.

- **v0.50.92 companion A21 릴리스 준비** (2026-08-02): 릴리스 진입점을 annotated tag `v0.50.92`로만 제한하고, 아직 게시 전인 A21이 immutable `v0.50.91`을 직접 선행 릴리스로 검증하도록 준비했다. A20 release ID `360327495`, source commit `7f44e4f143b2348c02553bab2209088c966f81ae`, tree `8fdf3615e40f5e81512517619d4225ee067f8c23`, annotated tag object `60158446cbf0599317b2850609d5e0d957a965db`, checksums SHA-256 `ff546eed918bc35dc420451341da563f2d6487c3a6565f9058dbc99362943f5a`을 고정한다. Darwin amd64·arm64 archive SHA-256은 `97a9fc7c6da9348709e3dbc48cb8587551e9980d61a33566103c844abdd3f05c`, `baff6e099c8debc97bdf61bf0ea98813e2bcdef58f5171cad67edb84bd64b548`이고 Linux amd64·arm64 archive SHA-256은 `1011d88f66658f2b9ebe2caefb536038266005f229f846de2f1e597b17e25ea6`, `2905bc851e4637c60fd198f42331a7a6bf13180ee917ca67d9f20affce436c5d`이며, 두 Darwin archive의 embedded manifest SHA-256은 `d34a841d2dbca009e7ae7af2780ed8792e2501b04f30b0711d197093ea61a24c`, `2c08f77641786a8afe9e94de8734cb7a318868d3189e336ef01b704cca0e40d0`로 검증한다. Homebrew는 A20 tap commit `6f4b8d585f028ea746c183f0666f9129e75b1937`, Cask raw SHA-256 `e53574c52330d137f686143e3058ac142c1f5fd47fe4577c292d1fbc0446e7fb`, blob `82ff9d98e40d82ff88c04e22a2ce99c5534a449a`을 선행 스냅샷으로 사용해 Cask만 non-force CAS로 갱신하고, Formula raw SHA-256 `6bc6a0fbf790ee144c74d802a2031ab61f57a2ebd0611b6f15e856c8ed3e8a7c`와 blob `4ebc6c38925002dec00759823d4dd847a499818a`는 동결한다. 보호 환경 승인, K1 서명·공증, immutable release 검증은 실제 태그 게시 단계에서 완료한다.

- **v0.50.91 companion A20 릴리스 준비** (2026-07-27): 릴리스 진입점을 annotated tag `v0.50.91`로만 제한하고, 아직 게시 전인 A20이 immutable `v0.50.90`을 직접 선행 릴리스로 검증하도록 준비했다. A19 release ID `359989008`, source commit `5bc41dccc72f8244943fd9e862cba07a36bf09d3`, tree `d7d26e5f66ac5d01556fb64eadd07407e00b5035`, annotated tag object `d4e2a495d194da97594e3eb6504f9ad3533b0dd8`, checksums SHA-256 `3ac2b6563fd9800beed1cf682ef8d0af4a246041e31bed9eec4c62431ab269e4`을 고정한다. Darwin amd64·arm64 archive SHA-256은 `8fe1e6b04ee107ebb3af2de1e0b3a272f42ab352f15e62ef1a2ab9e0b067c630`, `60c4fe79c83a4b58482808a6ea9937389b9fa5c27015521df76705db98fd42b0`이고 Linux amd64·arm64 archive SHA-256은 `34582ef39c7b54fb9327e295e6cbb50107e3689120cf74f75b1f21a01eecc1dc`, `11a00378365069b97804d6af40435645f26271550d1ab58e7776346ec7d4a879`이며, 두 Darwin archive의 embedded manifest SHA-256은 `c5383760f8d914870c9a5db4d00116beeda68f03be0a0967b35e2b6ec2823c55`, `41996a1a4b5c550cac02ac10f1e426a38f24555a0968fbc85807fad500f8dd9e`로 검증한다. Homebrew는 A19 tap commit `b3d42d3dc86630e2e020cadbd58e1e15c1c70572`, Cask raw SHA-256 `fb61c3d6596dc0dc7dcd9373b749204599992539fd871b7d5d56971a6e5af6ac`, blob `0cd0afda423911ece3fbd099884851e2b4b371f2`을 선행 스냅샷으로 사용해 Cask만 non-force CAS로 갱신하고, Formula raw SHA-256 `6bc6a0fbf790ee144c74d802a2031ab61f57a2ebd0611b6f15e856c8ed3e8a7c`와 blob `4ebc6c38925002dec00759823d4dd847a499818a`는 동결한다. 보호 환경 승인, K1 서명·공증, immutable release 검증은 실제 태그 게시 단계에서 완료한다.

- **v0.50.90 companion A19 릴리스 준비** (2026-07-26): 릴리스 진입점을 annotated tag `v0.50.90`으로만 제한하고, 아직 게시 전인 A19가 immutable `v0.50.89`을 직접 선행 릴리스로 검증하도록 준비했다. A18 release ID `359747783`, source commit `76f35d990e76511d169e239547d33bfedcea7948`, tree `5e90442a2b6dec38ec0072117d24b1d5ecb1f1e1`, annotated tag object `ba18d42068693111ed78b4bc1ccc4891d0663334`, checksums SHA-256 `027f4b6a1b7412f19aafa386c18b9b551096e99a3dfdd44e2ad7b550952eab30`을 고정한다. Darwin amd64·arm64 archive SHA-256은 `4101bc35739f6dd428bdafe9e450624430690687dc0635e4e7dc97b62dd344a6`, `d89d77a8618136643f26c3920f0c5d8aa4f87d5c48f2e5d555f3caf072fc375c`이고 Linux amd64·arm64 archive SHA-256은 `79f54ed935a204c6f5a89a93ba8945e8ffe15557080a1dc25eac437a16398782`, `aa4834873453266c853213d940cde4b672f6c00de9672718fcb8004582076b5f`이며, 두 Darwin archive의 embedded manifest SHA-256은 `71ef53e32e6e9fbfd1c03dd130412742f854bebb7958978ca8e2345e918b0a1e`, `a554b5a75cfd5c5f04ed9c06a6a2e1ba49220c2025e0149b030ed89a68757c56`로 검증한다. Homebrew는 A18 tap commit `abe9d317503d4bd813328b438ee038b3aaf8767e`, Cask raw SHA-256 `71f5e46ba0208fccc2c61ac00b61e9153e01ff3f25f8b0f93c7dda82cb7056fe`, blob `565e37d88837df90557a36d7b5f840b699731bbc`을 선행 스냅샷으로 사용해 Cask만 non-force CAS로 갱신하고, Formula raw SHA-256 `6bc6a0fbf790ee144c74d802a2031ab61f57a2ebd0611b6f15e856c8ed3e8a7c`와 blob `4ebc6c38925002dec00759823d4dd847a499818a`는 동결한다. 보호 환경 승인, K1 서명·공증, immutable release 검증은 실제 태그 게시 단계에서 완료한다.

- **v0.50.89 companion A18 릴리스 준비** (2026-07-25): 릴리스 진입점을 annotated tag `v0.50.89`로만 제한하고, 아직 게시 전인 A18이 immutable `v0.50.88`을 직접 선행 릴리스로 검증하도록 준비했다. A17 release ID `359675749`, source commit `2b062a5e348fbecc414abe9ba5c74c7dc79fe243`, tree `1e376104c94748a546607abce6667813163d3d61`, annotated tag object `8721b6be61a058aa5987727d3bae6d87c5fb38f4`, checksums SHA-256 `a4b437c11a5aaad986a4e0b93b0db0e19c3a49a1a801ba25e8b096c462bf3c47`을 고정한다. Darwin amd64·arm64 archive SHA-256은 `3bae06adc5c31281efd8d9fd016af0ab9158da54262e7f17fafd2ed9be6ff1b1`, `bf7a96a4ce34b58a940ab9813c2b797ec8f4a489c794b54d6b54097c5ecc4cce`이고 Linux amd64·arm64 archive SHA-256은 `e09b66b7a1683ef8764e59ca620242ee7eebbee573c0fec5232f9adb649f6909`, `31cc659ae347346204db2dae13368e6277b1e21f7e3a7c16dab160dd2b98176d`이며, 두 Darwin archive의 embedded manifest SHA-256은 `56b4b53840ee7c859077245ff6ddc95ef0ea530f581b3831aa6ee8645b9a9749`, `457b6fef8ebcda7b3977a0786f2397a30591225ac5112858aab5ba920176df8c`로 검증한다. Homebrew는 A17 tap commit `a2c3e07311b216927225f27d8e7c6f46899cd85c`, Cask raw SHA-256 `c5e8f1de84f02bfd6255a96233e7dacc3e63778deadf35c16f89661689159f8a`, blob `d694773024a3a0a6719a925b36e809fc9c28f402`을 선행 스냅샷으로 사용해 Cask만 non-force CAS로 갱신하고, Formula raw SHA-256 `6bc6a0fbf790ee144c74d802a2031ab61f57a2ebd0611b6f15e856c8ed3e8a7c`와 blob `4ebc6c38925002dec00759823d4dd847a499818a`는 동결한다. 보호 환경 승인, K1 서명·공증, immutable release 검증은 실제 태그 게시 단계에서 완료한다.

- **v0.50.88 companion A17 릴리스 준비** (2026-07-25): 릴리스 진입점을 annotated tag `v0.50.88`로만 옮기고 immutable `v0.50.87`을 직접 선행 릴리스로 검증한다. A17은 A16 source commit `3e02c622af97f74873325ec65940c580e23c580a`, tree `f041a63af99e2ba82bd6fb573272489fbd101e86`, annotated tag object `9fad744950e268fa500812cad0194fa5da47e369`, checksums SHA-256 `7c19613a7f441264f90fb6b0c6e975c78cae3db250f2ed279fa44418a022dc7f`을 고정한다. Darwin amd64·arm64 archive SHA-256은 `cc411eb9cc04476d280d272f0e52b7c1a40fad923439180b526a46c38be1b63e`, `b0880ef40f3089168be234a540cfaf1795e0e54168a4ead3f92275a13eb63012`이고 Linux amd64·arm64 archive SHA-256은 `8da3ec03967fa1b5911708716239bcaa9d0843069e65836f280f986b4cdd1aaa`, `9aeca632be6de54d3540e03ad99ba5dc520f3665f7f592bd12b689b844ea8bf3`이다. 두 Darwin archive의 embedded manifest SHA-256은 `93943fafac83eb3090f10f8c48489a96b80d003c5c9f00dfaa2cd59eabf7be42`, `4cc59ac1a3194df80a050e4159a6919a01e2bcc69efc575907aedf16aacc4057`로 검증한다. Homebrew는 검증된 A16 tap commit `f62ca4883c8770104291185bc30184aa5fb1af50`과 Cask raw SHA-256 `5bdfa91344516d98fe42212b83b4de284fb9a7850fc4219ea144f70870a2319e`, blob `557205adaae7c5e6b12ac1f9bc8346e00ed42a75`을 선행 스냅샷으로 사용하고 Cask만 non-force CAS로 갱신한다. Formula는 raw SHA-256 `6bc6a0fbf790ee144c74d802a2031ab61f57a2ebd0611b6f15e856c8ed3e8a7c`, blob `4ebc6c38925002dec00759823d4dd847a499818a`로 동결한다. 보호 환경의 승인된 source 변수와 K1 서명·공증·immutable release 검증은 태그 게시 단계에서 유지한다.

- **v0.50.87 companion A16 릴리스 준비와 doctor 오탐 제거** (2026-07-23): 릴리스 진입점을 annotated tag `v0.50.87`로만 옮기고 immutable `v0.50.86`을 직접 선행 릴리스로 검증한다. A16은 A15 source commit `0fc4f60dac8ff8afe69b680c8bf723bfbced4769`, tree `3daa4aef3528338439acb34f50d3b4a19ababea5`, annotated tag object `bb24ad6a554beee871063070b219b409245c0e93`, checksums SHA-256 `237f985675f866c234a41066735a2bff3ae0b554a2fe1b1b6b57aed125bac8f7`을 고정한다. Darwin amd64·arm64 archive SHA-256은 `41e2a371c89567ff862d5f47179c838cb3aefd83abeb0ff769e58b12579676e3`, `84ea326a10c860af82663db1c87a8a15bdee492143d77a02ad86a0b3ba930f8f`이고 Linux amd64·arm64 archive SHA-256은 `cae69dd8828cb2c12ba0d312c3f4dbc034104c1b4b9cee6cddf18eebe6430cb6`, `9e943908dabf910e9f3072f838a99dec3c9d4952d9058bfbc1b71cd78e3f29eb`이다. 두 Darwin archive의 embedded manifest SHA-256은 `c2398cd51093cb19804ef2d07e1848cc77d16610a4669e78e0e1577a466df300`, `83da7620c878841c06980ad12315023dc2054b71f27cb7dfb53931a4224d0099`로 검증한다. Homebrew는 검증된 A15 tap commit `bb84d874af4c9187603f36c3ca06460c90b7caea`와 Cask raw SHA-256 `ecf3a06b09fb7c900089abc8af7efbe472131469d0b280e949306782ec7d71da`, blob `f9baefd8723dad6afb3d60999bde44d3913ecb10`을 선행 스냅샷으로 사용하고 Cask만 non-force CAS로 갱신한다. Formula는 raw SHA-256 `6bc6a0fbf790ee144c74d802a2031ab61f57a2ebd0611b6f15e856c8ed3e8a7c`, blob `4ebc6c38925002dec00759823d4dd847a499818a`로 동결한다. Codex와 Gemini의 platform-native `agent-teams` 템플릿을 generator 소유로 잘못 분류해 발생하던 `auto doctor` stale-template 오탐도 함께 제거한다. 보호 환경의 승인된 source 변수와 K1 서명·공증·immutable release 검증은 태그 게시 단계에서 유지한다.

- **v0.50.86 companion A15 릴리스 준비** (2026-07-23): 릴리스 진입점을 annotated tag `v0.50.86`으로만 옮기고 immutable `v0.50.85`를 직접 선행 릴리스로 검증한다. A15는 A14 source commit `4b8eb62200d253b46e022670c482e2f716a992a3`, tree `fbdc83287982899c3d6bfe5fdf7b88494e76bcb0`, annotated tag object `f005dd935dbbcec8c60052adcfda6632fe8831e1`, checksums SHA-256 `5bd11e327eab31c555f89298761e2d27bca2fadebfc3b7961cafb6a140539236`을 고정한다. Darwin amd64·arm64 archive SHA-256은 각각 `66834d509309cb09b84f78bb81a97e68a8d03434c9a37f239a2ae04677dbc10b`, `7fe10bc7b03b3df44f803622e3830e5e91f3ea12b47b706cf14f716b076b012e`이며, Linux amd64·arm64 archive SHA-256은 `187620011ce035f6bdb09f3f6d5b005f878463c3ba0fd805142cbd3e4f587698`, `654e42612a3f1ee670157cd461b3dff1270f2102b085984951975c0284356172`이다. 두 Darwin archive의 embedded manifest SHA-256은 `4265d3f18c7aaab779a720216c2f1dfc9a486c01be898290d4f56be31102008e`, `918c91d4bdee0c58e74e0068314d35463e094fef214986a550579bca08b2ef38`로 검증한다. Homebrew는 검증된 A14 tap commit `d0292d235fd351b1e5f2bcbf5aef213610587e3f`을 정확한 CAS 부모로 사용하고, Cask raw SHA-256 `90843baeadc6d8ae232016aefbbded547ccdc05d69979ead9c46900e2eb4f396`과 blob `adce0d92445ca7e39c5afa50c6636d06149cf5ea`을 선행 스냅샷으로 고정한다. Formula는 raw SHA-256 `6bc6a0fbf790ee144c74d802a2031ab61f57a2ebd0611b6f15e856c8ed3e8a7c`과 blob `4ebc6c38925002dec00759823d4dd847a499818a`를 그대로 동결하며, Cask만 non-force CAS로 갱신한다. 보호 환경의 승인된 source 변수와 K1 서명·공증·immutable release 검증은 태그 게시 단계에서 유지한다.

- **v0.50.85 companion A14 릴리스 준비** (2026-07-22): 릴리스 진입점을 annotated tag `v0.50.85`로만 옮기고 immutable `v0.50.84`를 직접 선행 릴리스로 검증한다. A14는 A13 source commit `2b7aa046bdb7861113dfa57b30489c11715582e9`, tree `95d1b00bcc1cb1bfcca3dd58e1e5e1b94575c367`, annotated tag object `de34e9c1a2a06b27f57235c81a59d1da180eab6d`, checksums SHA-256 `8f00d3b42d71c9e71346bf62cd72f8e1428600cb0795f703d90de64b3b9ba14e`, Darwin amd64·arm64 archive SHA-256 `fa60e03ecd39a5fa203be3cca3e8a7010e3af7854195f0e866ef80e7a0e82f0f`·`f4ed0ef8d6f0274389ada5cebdeb87a2899bf34b7a11bd99318b5914775d84f1`, embedded manifest SHA-256 `ba6f3e92d4a1c0a1a52b7b17e484961cb8640944eae24856652ebe6192210931`·`22660fc029bbcb9ffe312964d9f674ba2587440dba48790e28fb4f35b19dcc69`를 직접 고정한다. Homebrew는 검증된 A13 tap commit `d8dc4c78f42a7c5e30176334b607b036be3bd677`과 Cask blob `524ade82d6466da8ad6d5c173e0b4a214fdbc21f`에서 A14 Cask로만 CAS 갱신하며, Formula blob `4ebc6c38925002dec00759823d4dd847a499818a`는 동결한다. 최종 A14 source pin, 보호 환경 승인, K1 서명·공증, immutable GitHub Release와 Homebrew 반영은 태그 게시 단계에서 완결한다.

- **실행 심사 고착 감지와 배포 경계 강화** (2026-07-22): 설치 직후와 자가 업데이트 교체 전에 `auto version --short`를 제한 시간 안에 실행하고, 요청한 버전과 출력이 정확히 일치할 때만 다음 단계로 진행한다. macOS 릴리스는 Developer ID 서명·공증을 검증한 최종 바이너리를 arm64와 amd64별로 직접 실행한 뒤 manifest를 생성하며, 설치기와 CI는 timeout 시 남은 프로세스 트리를 정리하고 복구 안내를 제공한다. POSIX K1 fingerprint 문법은 호출자의 `locale`에 의존하지 않고 명시적인 ASCII 소문자 16진수 집합으로 검사하며, cmux 오류 테스트가 실제 사용자 pane을 여는 격리 누락도 함께 막았다. ([PR #101](https://github.com/Insajin/autopus-adk/pull/101), [#100](https://github.com/Insajin/autopus-adk/issues/100))

- **v0.50.84 companion A13 릴리스 준비** (2026-07-22): 릴리스 진입점을 annotated tag `v0.50.84`로만 옮기고 immutable `v0.50.83`을 직접 선행 릴리스로 검증한다. A13은 A12 source commit `e6367b5375cd4cdf09cb1515877bc57323521364`, tree `6c9a22e85d5a8c5f23c0d9e1bb41de270cab85a4`, annotated tag object `080507fceb3b4bf31f0e0887e49013fd65645ac2`, checksums SHA-256 `7d871b077766f3a7dd6859427fa9b1333422312764820243d3bf7af5e935dee0`, Darwin archive와 embedded manifest digest를 직접 고정한다. Homebrew는 검증된 A12 tap commit `192cacd10d0c85d5cc0533356400e697152a551c`과 Cask blob `2ba9ab9caa381c68a276588a7d6ad77de46f1dd5`에서 A13 Cask로만 CAS 갱신하며, Formula blob `4ebc6c38925002dec00759823d4dd847a499818a`는 동결한다. 최종 A13 source pin, 보호 환경 승인, K1 서명·공증, immutable GitHub Release와 Homebrew 반영은 태그 게시 단계에서 완결한다.

- **QAMESH Go 캐시 수명 격리와 정규화 취약점 해소** (2026-07-22): 각 QAMESH 명령에 전용 Go 빌드 캐시를 할당하고 명령이 끝나면 해당 캐시만 정리한다. 프로젝트별 모듈 캐시는 계속 공유하므로 재사용성은 유지하면서 동시 실행 간 캐시 오염과 누적을 막는다. 호출 가능한 정규화 경로에서 무한 루프를 일으킬 수 있는 GO-2026-5970을 해소하도록 `golang.org/x/text`도 수정 버전으로 올렸다.

- **멀티프로바이더 pane 인계와 세션 경계 강화** (2026-07-22): yield session 저장이 성공한 뒤에만 생성한 pane의 소유권을 이전하고, 저장에 실패하면 아직 소유 중인 pane을 정리한다. session ID의 경로 순회를 허용하지 않으며 symlink와 기존 session 덮어쓰기를 차단한다. provider의 원문 이름이나 대소문자를 정규화한 이름이 충돌하면 실행 전에 거부한다. debate 응답은 설정된 provider 순서로 결정화하되, 공용 fastest collector에는 실제 도착 순서를 그대로 보존한다.

- **v0.50.83 companion A12 릴리스 준비** (2026-07-21): 릴리스 진입점을 새 annotated tag `v0.50.83`으로만 옮기고 immutable `v0.50.82`를 직접 선행 릴리스로 검증한다. A12는 A11 source commit `a8558ccc36e04125de6b8d84c7ffc9e8ddb5a2c9`, tree `9545ed7437e6dfd7573952586a31964061e30e2d`, annotated tag object `c636f42a6e8dc65ef6500eb95dac4ef7d1faff9a`, checksums SHA-256 `a7973f9fa27d1e0ca1d1943adcfe5be0fa6807ba0517ff9066b2659fa6f4f01c`, Darwin archive와 embedded manifest digest를 직접 고정한다. Homebrew는 live A11 tap commit `624e317d0433fe0efc91efb5dc5e8708ed50a22d`과 Cask blob `7f760f85434457fa4cdc7da3718099e43980ce06`에서 검증된 A12 Cask로만 CAS 갱신하며, Formula blob `4ebc6c38925002dec00759823d4dd847a499818a`는 동결한다. Homebrew 토큰 발급 전 K1과 Sigstore/Rekor를 암호학적으로 검증하고, 취약한 Cosign 2.2.4 기본값 대신 installer v4.1.2와 Cosign v3.1.2를 exact pin으로 사용한다. 최종 A12 source pin, 보호 환경 승인, K1 서명·공증, immutable GitHub Release와 Homebrew 반영은 태그 게시 단계에서 완결한다.

- **v0.50.82 companion A11 릴리스 준비** (2026-07-21): 릴리스 진입점을 새 annotated tag `v0.50.82`로만 옮기고 immutable `v0.50.81`을 직접 선행 릴리스로 검증한다. A11은 A10 source commit `54536edc09c37a634532c2c9b51e62869d393db4`, tree `e9a30f4530e06c9b62933e7bf97e0056faed259c`, annotated tag object `8b37fccb57255fc24003dc3af2700334f4a8d3c4`, checksums SHA-256 `2e97c1f3c8d0cba0f93dd83c724c71eaa4966c79d4812a6a9cf034144c7b178d`, Darwin archive와 embedded manifest digest를 직접 고정한다. Homebrew는 live A10 tap commit `ab9a0e489ee34f8a075019c4acebb2a8ae61c290`과 Cask blob `c6edb108d821d88914e12d2c1bf943540c63351e`에서 검증된 A11 Cask로만 CAS 갱신하며, Formula blob `4ebc6c38925002dec00759823d4dd847a499818a`는 동결한다. 최종 A11 source pin, 보호 환경 승인, K1 서명·공증, immutable GitHub Release와 Homebrew 반영은 태그 게시 단계에서 완결한다.

- **인증으로 보호된 metrics endpoint의 canary 판정 수정** (2026-07-21): `auto canary`가 `/metrics`의 HTTP 401 응답을 서비스 장애로 오판하던 문제를 해결했다. `/metrics`의 401은 보호된 endpoint에 도달한 정상 결과로 처리하되, `/health`의 401과 그 밖의 오류 응답은 계속 실패로 판정한다. 또한 각 endpoint의 상태와 HTTP 결과를 독립적으로 기록해 한 endpoint의 실패가 뒤따르는 정상 결과까지 오염시키지 않도록 했다.

- **Codex update preview와 적용 파일 집합 수렴** (2026-07-21): mixed Codex·OpenCode 하네스에서 실제 update가 유지하는 workflow compatibility skill 10개를 다음 preview가 반복해서 prune 대상으로 표시하던 불일치를 해결했다. Codex `Generate`와 `Update`가 동일한 skill mapping을 사용하므로 `auto update --plan`의 desired file set이 실제 transaction과 일치하며, 적용 직후 preview도 해당 파일을 모두 retain한다.

- **v0.50.81 companion A10 릴리스 준비** (2026-07-21): 릴리스 진입점을 새 annotated tag `v0.50.81`로만 옮기고 immutable `v0.50.80`을 직접 선행 릴리스로 검증한다. A10은 A9 source commit `c9c4f49d48022eb0c8d72ee7b520136a4f21f176`, tree `3a71fa56bd917f447a6b1705772b6ab99bbcfbc8`, annotated tag object `b7d05fa76eed41b1dfb4eddbd9873525e0aac15f`, checksums SHA-256 `9ed1f99d22a761abb7953c70aab3c7de5ab0b7ec3524cf3798fcd3815c53bde7`, Darwin archive와 embedded manifest digest를 직접 고정한다. Homebrew는 live A9 Cask blob `3f3a38e9a2ae556acc0f7d0974895d6189f266dd`에서 검증된 A10 Cask로만 CAS 갱신하고 Formula blob `4ebc6c38925002dec00759823d4dd847a499818a`는 동결한다. 최종 A10 source pin, 보호 환경 승인, 서명·공증, immutable GitHub Release와 Homebrew 반영은 태그 게시 단계에서 완결한다.

- **cmux 멀티프로바이더 pane 입력 트랜잭션 직렬화** (2026-07-20): 여러 provider pane을 동시에 열 때 `SendLongText`와 `Enter`가 cmux의 비동기 입력 큐에서 서로 끼어들어 명령이나 prompt가 손상되던 경합을 process-global cmux transaction gate로 봉인했다. tmux와 plain backend의 병렬성은 유지하고, 대기 중 context 취소·전송/지연/Enter 실패 뒤 gate 해제·Enter 재시도를 명시적으로 처리한다. 구조화 session과 debate, retry, recovery, completion auto-approval, Round 2 환경 전환까지 같은 경계를 적용했으며, `AUTOPUS_ROUND` export가 Enter 없이 다음 입력과 이어지던 복구 결함도 함께 수정했다.

- **v0.50.80 companion A9 릴리스 준비** (2026-07-20): 릴리스 진입점을 새 annotated tag `v0.50.80`으로만 옮기고 immutable `v0.50.79`를 직접 선행 릴리스로 검증한다. A9은 A8 source commit `dd0c2759ed5435d4634011e349caad62ea3df414`, tree `4325913ba332c583dd573ccf9248b38497d76926`, annotated tag object `8c6dcef91407e3321704014559cfd929d14768d0`, checksums SHA-256 `1d0bdbfe50f85c381fde11c334c97a1b783dcfa4e12e0c4023152f38119a0bcd`, Darwin archive와 embedded manifest digest를 직접 고정한다. Homebrew는 live A8 Cask blob `979e62e34124b9f3c68bb2b8e1d0163047ea3ee3`에서 검증된 A9 Cask로만 CAS 갱신하고 Formula blob `4ebc6c38925002dec00759823d4dd847a499818a`는 동결한다. 최종 A9 source pin, 보호 환경 승인, 서명·공증, immutable GitHub Release와 Homebrew 반영은 태그 게시 단계에서 완결한다.

- **멀티프로바이더 사전 검사 대기 문제 해결** (2026-07-20): `auto update`와 `auto init`이 설치된 프로바이더를 찾을 때 더 이상 `--version`을 실행하지 않는다. 버전 정보가 필요한 Claude Code, Codex, OpenCode, Antigravity 검사에는 제한 시간을 적용하고, 실패하면 기존처럼 기능을 축소해 계속 진행한다. POSIX에서는 별도 프로세스 그룹을 정리하고 Windows에서는 Job Object로 자식 프로세스 트리를 종료해, 래퍼가 표준 출력·오류 파이프를 상속한 손자 프로세스를 남겨도 업데이트나 pane 실행 준비가 멈추지 않는다.

- **v0.50.79 companion A8 핫픽스 릴리스 게시** (2026-07-20): source commit `dd0c2759ed5435d4634011e349caad62ea3df414`와 tree `4325913ba332c583dd573ccf9248b38497d76926`을 가리키는 annotated tag object `8c6dcef91407e3321704014559cfd929d14768d0`에서 변경 불가능한 최종 릴리스를 게시했다. `checksums.txt` SHA-256은 `1d0bdbfe50f85c381fde11c334c97a1b783dcfa4e12e0c4023152f38119a0bcd`이며, Darwin amd64·arm64 archive SHA-256은 각각 `19e317cdabc9dde976ca772d9ddbbf693b444dd44eefa70c8d0313a32de89a9b`, `41e29ae1c3c48dd6e3e5f4dfe8076472704d00a7d479b5cc8a90f53c0af6ef31`이다. 두 archive의 embedded manifest SHA-256은 `c5ac37874bac5de87152e781bd82a17c7705894f24be81657ccc907f15ba1f65`, `ebcf563c11f0836be2b2bd4423ea315283eeec12cfa200d479e1a56f5909f5f1`이다. Release 실행 `29743139223`은 11개 자산의 K1 publisher envelope, cosign OIDC bundle, Developer ID 서명·공증을 통과했고, Homebrew commit `2838951580d16348e12be39c09553cf6765504cb`은 Cask blob만 `979e62e34124b9f3c68bb2b8e1d0163047ea3ee3`으로 갱신하며 Formula blob `4ebc6c38925002dec00759823d4dd847a499818a`를 유지했다.

- **v0.50.78 companion A7 릴리스 게시** (2026-07-20): source commit `51de6030a69a8e36fcf7e5790ef157eff6fedf00`와 tree `3cd00b17bd8bd6aa8def213de1c5765c3611765d`를 가리키는 annotated tag object `417a318fb6a11a720e2c4102e92e39ea9ed676e9`에서 변경 불가능한 최종 릴리스를 게시했다. `checksums.txt` SHA-256은 `322d2ef21dff55f02ca36944aba88ee5da92fdae6bcd16a89319f1697efb9733`, Darwin amd64·arm64 archive SHA-256은 `43018046ab37027b7fba3888d288961cb5abc136e478deaa9f878586bcce6629`, `e72653fd3094537caa60398e2017d409796d7ceef88a7662ca93b6299e9d00ec`입니다. 두 archive에서 독립 산출한 embedded manifest SHA-256은 `3f7c879c93dea0d119805987bef434b65c1a53684e80f78b5d9a0c9c2cd011d5`, `87ef2a30d6ee8c9abe9e679d597d0a4fbe9bb5cdee1266572476ad6a66aef975`입니다. Homebrew commit `700e0e544c3c774eb07a61dcaa68465c73a21cd3`은 Cask blob만 `a46b37d61bfd62a31fd5f4c6731d4f83fa1c868a`로 갱신했고 Formula blob `4ebc6c38925002dec00759823d4dd847a499818a`는 유지했다.

- **멀티프로바이더 pane 실행 안정화** (2026-07-20): Codex와 Claude Code용 생성 명령이 사용하는 `--format`·`--no-detach` 옵션을 실제 CLI와 맞추고, 두 프로바이더의 Stop hook 결과를 모두 탐색하도록 응답 수집 경로를 정리한다. pane 라우팅은 설치 여부가 아니라 현재 활성화된 mux만 선택하며, cmux health 응답은 workspace 전체 출력에서 요청한 surface와 pane을 정확히 찾아 판정한다. 같은 surface의 split 요청을 직렬화하고, 일부 split만 성공한 경우 원인과 생성된 pane 증거를 보존한다. 복구 경로는 손상되거나 사라진 pane을 먼저 교체한 뒤 제한된 fallback을 적용하며, 선택한 backend와 전환 사유를 진단 정보에 남긴다.

- **v0.50.77 companion A6 릴리스 재수렴** (2026-07-19): 미게시 `v0.50.75`·`v0.50.76` 태그는 감사 이력으로 보존하고 릴리스 좌표를 `v0.50.77`로 옮겼다. `connect.NewClient`는 더 이상 전역 `http.DefaultTransport`를 공유하지 않고 기본 transport의 clone을 소유하므로, Go 1.26에서 병렬 `httptest.Server.Close()`가 다른 클라이언트의 진행 중 요청을 깨뜨리지 않는다. A6의 직접 선행 릴리스는 마지막으로 게시된 immutable `v0.50.74`이며, 기존 A5 계보 pin과 prior Cask blob `ceed648bfece4555e8310b6e894fedc847520960`을 그대로 검증한다. source commit `902f1acfa91f1d0a2ac9471d5cd79117031a2599`와 tree `a5a2f72495b0f25f4dedb80952d2a881377c5b64`에 고정한 Release 실행 `29682893087`은 annotated tag object `41feed7decafac33d8f7f43e06804e3c9bf37ef3`에서 변경 불가능한 최종 릴리스를 게시했다. 11개 자산의 GitHub 서버 digest와 checksum, K1 publisher envelope, cosign OIDC bundle, Darwin Developer ID `GP2PFA2PUV` 서명·공증, A0→A6 receipt/manifest 계보를 독립 검증했다. 검증된 Homebrew commit `a6adcd0ff7e0eff72f30dab9e9f7f0f73b8c9328`은 Cask blob만 `39b9b77eb51149ff87df7ad4f8fb3c5300b1302c`로 갱신했고, Formula blob `4ebc6c38925002dec00759823d4dd847a499818a`는 그대로 유지했다.

- **v0.50.76 companion A6 릴리스 시도 실패(미게시)** (2026-07-19): annotated tag object `88b8b47dac32686ed7d343e815f6d5d36a42ff34`는 source commit `c4556c8b294c616b745d67c9fe418963deaed07f`에 고정해 보존한다. Release 실행 `29681010220`은 lint·보안·플랫폼 런타임을 통과했지만, 전체 race/integration coverage에서 `pkg/connect`의 전역 HTTP transport 공유가 `TestListWorkspaces_Unauthorized`를 깨뜨렸다. release job은 skipped됐고 GitHub Release 자산과 Homebrew 변경은 생성되지 않았으며, 같은 태그를 이동하거나 재사용하지 않고 `v0.50.77`로 재수렴한다.

- **v0.50.75 companion A6 릴리스 시도 실패(미게시)** (2026-07-19): annotated tag는 source commit `70b9feff75fa75da330fa4bc93b59507924c8447`에 고정해 보존한다. Release 실행 `29680368022`의 CI lint가 Codex 훅 정적 분석 부채 3건을 탐지해 release job이 실행되지 않았으며, GitHub Release 자산과 Homebrew 변경은 생성되지 않았다. 같은 태그를 이동하거나 재사용하지 않고 `v0.50.76`으로 재수렴한다.

- **Codex 구조화 리뷰 pane-first 부채 해소** (2026-07-19): `codex exec` provider만 사전에 subprocess로 바꾸던 예외를 제거해, pane-capable 터미널에서는 Codex와 Claude Code 어느 호스트에서도 선택된 primary pane backend를 최초 요청과 JSON 재요청에 그대로 사용한다. Codex adapter는 공식 `event → matcher group → hooks[]` 스키마로 hook을 생성하고, git root 기준 명령과 실행 가능한 Stop/SessionStart 자산을 직접 설치해 codex-only 하네스도 완결되게 했다. 기존 사용자 hook의 `description`, Windows command, `async`, 미래 필드와 혼합 matcher group의 사용자 handler를 보존하며, 잘못된 기존 JSON은 덮어쓰지 않고 중단한다. pane 수집은 marked response file 다음으로 round-scoped hook result를 사용하고 done을 먼저 기다리되, 아직 신뢰되지 않은 project hook은 완성된 response marker의 짧은 grace fallback으로 전체 제한 시간 대기를 피한다. 같은 provider/round 재요청 전에는 충돌 불가능한 session ID와 안전한 provider 이름을 사용해 해당 attempt의 done/result/ready/input/abort만 지우고 sibling provider와 다른 round를 보존한다. hook 설치와 result/done/ready 기록은 symlink를 거부하는 confined temporary-file + atomic replace로 바꿨으며, Stop hook은 빈·손상 payload에서도 result와 done을 순서대로 남기고 크기가 제한된 일반 `transcript_path`에서 마지막 assistant output을 best-effort로 회수한다.

- **v0.50.74 companion A5 릴리스 계보와 설치 권한 하드닝** (2026-07-18): 릴리스 워크플로를 정확한 `v0.50.74` 태그와 보호 환경에만 열고, A5가 변경 불가능한 `v0.50.73`의 commit, annotated tag object, checksums, Darwin archive와 manifest pin을 직접 검증하도록 확장했다. A0부터 이어진 공개키 receipt/signature/record 바이트는 그대로 유지하며, POSIX 설치기는 `umask 077`과 sudo 경로에서도 설치 디렉터리와 바이너리의 최종 권한을 `0755`로 보장한다. 정확한 source commit `b27252cb1148192a8ae1a95195c50e5f221453a4`와 tree `6c3790ee668b2d1c9f2f44a272144dd1106507d9`에 고정한 Release 실행 `29640813340`은 annotated tag object `c79f133f0108bf3f07cee0162c1abeecf9d379d1`에서 변경 불가능한 최종 릴리스를 게시했다. 11개 자산의 GitHub 서버 digest와 checksum, K1 publisher envelope, cosign OIDC bundle, Darwin Developer ID `GP2PFA2PUV` 서명·공증, A0→A5 receipt/manifest 계보를 독립 검증했다. Homebrew commit `9e3b9b4076b47b85218b14632c79a3d796e6769c`는 Cask blob만 `ceed648bfece4555e8310b6e894fedc847520960`으로 갱신했고, `v0.50.71` Formula blob `4ebc6c38925002dec00759823d4dd847a499818a`는 그대로 유지했다.

- **v0.50.73 companion A4 릴리스 계보 준비** (2026-07-17): 릴리스 워크플로를 정확한 `v0.50.73` 태그와 보호 환경에만 열고, A4가 immutable `v0.50.72`의 commit, annotated tag object, checksums, Darwin archive와 manifest pin을 직접 검증하도록 확장했다. A0부터 이어진 공개키 receipt/signature/record 바이트는 그대로 재게시하며, Homebrew는 고정된 `v0.50.72` Cask blob에서 canonical `v0.50.73` Cask로만 CAS 갱신하고 `v0.50.71` Formula는 호출하거나 변경하지 않는다. 이 항목은 릴리스 준비 코드이며 태그 생성, 환경 source pin 갱신, 실제 서명·공증·게시 완료를 뜻하지 않는다.

- **Codex SPEC 리뷰 응답 수집 안정화** (2026-07-17): 당시 pane 응답 수집 공백을 막기 위해 `codex exec` 프로바이더를 실행 전 subprocess 수집 경로로 선택하는 임시 호환 경계를 추가했다. 명시적 `--timeout`은 provider 실행 예산에도 동일하게 적용되고, timeout 상태는 원문 stdout·stderr·경로·비밀값 없이 source, budget, elapsed, collection mode, partial-output 여부만 구조화해 기록한다. 이 provider-level subprocess 예외는 2026-07-19의 pane hook-result 수집과 attempt 격리 변경으로 제거됐다.

- **Self-update 교체 트랜잭션 하드닝** (2026-07-17): 새 바이너리를 대상 파일시스템의 전용 stage에 준비하고 파일·디렉터리를 동기화한 뒤에만 교체하도록 변경했다. Darwin/Linux는 원자 교환 후 inode를 다시 확인하며, 동시 변경이나 동기화 실패 시 원자적으로 되돌리고 되돌리기마저 실패하면 복구 stage를 보존한다. 교환을 지원하지 않는 커널·파일시스템에서는 기존 바이너리를 유지한 채 안전하게 중단한다. Windows는 실행 중인 바이너리를 `target.old`에 보존하고 no-replace·write-through 이동으로 설치한다. 성공한 설치의 복구본에는 완료 marker를 남겨 다음 실행에서만 정리하고, marker가 없거나 유효하지 않은 복구본은 자동 삭제하지 않는다. Windows 런타임 CI와 모든 OS에서 실행되는 상태 머신 회귀 테스트를 추가했다. 임시 디렉터리 생성 실패, symlink·hardlink alias, target 변경, 부분 복사, chmod·xattr·sync 실패도 교체 전에 차단한다.

- **v0.50.72 macOS self-update 서명 보존** (2026-07-17): v0.50.72에 포함된 새 updater는 SHA-256 검증을 마친 macOS 릴리스 바이너리를 설치한 뒤 ad hoc 서명으로 다시 쓰던 동작을 제거했다. 이 updater가 수행하는 교체는 다운로드한 Mach-O 바이트와 Developer ID 서명, Team ID를 그대로 보존하며, Darwin 전용 회귀 테스트가 바이트 해시와 코드 서명 식별자의 일치를 검증한다. A3 릴리스 게이트는 immutable `v0.50.71`의 commit, annotated tag object, checksums, Darwin archive와 manifest를 고정 검증하고 A0 공개키 record의 연속성을 유지한다. Homebrew는 `v0.50.71` Formula 마이그레이션 브리지를 동결하고 canonical Cask만 `v0.50.72`로 갱신한다.

- **v0.50.72 macOS 기존 self-update의 릴리스 후 이행 안내** (2026-07-17): immutable v0.50.72 릴리스와 새 replacer는 변경하지 않았다. 격리 환경의 smoke test에서 v0.50.71 이하의 macOS 기존 updater가 수행하는 첫 `auto update --self`는 설치한 v0.50.72를 ad hoc 서명으로 다시 쓸 수 있음을 확인했다. 이 경로에서는 `auto update --self`, `auto update --self --force`, `auto update`를 순서대로 한 번 실행해야 한다. 두 번째 명령은 이미 설치된 v0.50.72의 수정된 updater가 릴리스와 정확히 같은 바이트를 다시 설치하여 Developer ID 서명과 `TeamIdentifier=GP2PFA2PUV`를 복원한다. v0.50.72 이상에서 시작하는 이후 self-update는 바이너리 단계에서 `auto update --self` 한 번만 필요하며 릴리스 바이트와 서명을 보존한다. Cask 신규 설치와 기존 Formula에서 Cask로의 이행은 서명된 릴리스 아티팩트를 직접 설치하므로 이 1회 이행 절차의 대상이 아니다.

### Added

- **멀티프로바이더 오케스트레이션 실행 계약 수렴 (SPEC-ORCH-024)** (2026-07-20): pipeline과 orchestra가 실제 provider dispatch, exact typed gate, requested/effective strategy, fallback·judge·quorum·dissent를 하나의 `orchestration_run_receipt.v1` 증거 계약으로 판정하도록 fail-closed 경계를 추가했다. Pipeline은 검증된 frozen SPEC 문서와 정제된 이전 phase 결과만 prompt에 전달하고, checkpoint identity/dependency closure·원자 저장·dashboard parity를 강제한다. Orchestra는 모든 participant/retry/fallback/judge attempt의 role·backend·artifact를 보존하고 post-dispatch partial failure, provider recovery, unresolved Critical veto, typed judge 및 idea 경로의 model-family separation을 감사 가능하게 기록한다. Codex와 Claude Code 생성 workflow는 동일한 `orchestration-contract.v1` 의미를 각 native worker 도구에 바인딩하고, current typed receipt에서 degraded promotion·review·idea·teardown을 판정하며 explicit provider forwarding과 five-field worker handoff를 유지한다. Gemini/OpenCode에는 지원하지 않는 Claude team primitive와 dangling team reference를 생성하지 않으며, concrete generated argv/receipt-consumption oracle로 semantic-only 패리티의 허점을 막았다.

- **릴리스 아티팩트 게시자 서명 완성 (SPEC-ADK-RELEASE-SIGNING-001)** (2026-07-18): v0.50.73부터 ECDSA P-256 다중 서명 envelope `checksums.txt.signatures`를 실제 게시하고, Go self-updater와 POSIX·Windows 설치기 세 소비자가 `checksums.txt`를 신뢰하기 전에 게시자 서명을 fail-closed로 검증하도록 완성했다. V1 envelope는 4 KiB·16레코드 상한, exact LF/header, full lowercase SPKI SHA-256 fingerprint, canonical base64/DER, duplicate 거부와 full-parse-before-crypto를 적용한다. known active K1/K2 중 하나도 통과하지 않거나 서명 asset·검증 도구가 없으면 checksum-only로 돌아가지 않으며, 설치기는 v0.50.73 미만 unsigned 릴리스를 거부한다. POSIX 검증기는 설치기에 SHA256으로 고정하고 OpenSSL을 사용하며, Windows PowerShell 5.1/7 경로는 CNG로 동일 계약을 검증한다. 보호 환경 K1 secret의 exact key pair와 source pin을 확인한 릴리스 run `29588526312`가 v0.50.73 assets를 게시했고, live `checksums.txt`·cosign bundle·publisher envelope와 K1 서명을 독립 검증했다. Stage 2 CI run `29618589360`은 정적 계약·전체 테스트/coverage·lint·Windows 5.1/7·macOS installer oracle을, Security Scan `29618589373`은 secret/dependency scan을 통과했으며 독립 리뷰 finding은 0건이다. offline-next K2는 public pin만 선배포하고 signing input에는 넣지 않았다. 별도 off-device 독립 매체 보관은 확인되지 않은 운영 잔여로 명시한다. 또한 신뢰된 설치기·업데이터 바이트가 유지될 때의 release-asset-only 공격만 방어하며, 저장소 `main`이나 raw-main 전달 경로 침해까지 방어한다고 주장하지 않는다.

- **멀티 리포 sync verify fail-closed 계획기 (SPEC-ADK-SYNC-VERIFY-001)** (2026-07-17): `auto sync verify [--spec SPEC-ID] [--strict]`가 optional Git lock 없이 NUL-delimited dirty 상태와 tracked-but-ignored 파일을 수집하고, canonical root workspace 정책에 따라 모든 경로를 Phase A/B 후보·blocked generated/runtime·미분류로 완전 분할한다. 출력 계획은 4개 셸에서 안전한 상대 경로만 `git -C <repo> add -- <paths>`로 제시한다. `--spec`은 워크스페이스 전체에서 일반 디렉터리이며 심볼릭 링크가 아닌 SPEC 호스트가 정확히 하나인지 확인한다. SPEC 문서 읽기 오류, 중복 호스트, 심볼릭 링크 이탈은 fail-closed로 거부하고 전체 리포에서 해당 SPEC이 소유한 파일만 계획에 남긴다. Git 오류의 stderr와 절대 경로는 진단에 노출하지 않으며, 회귀 테스트는 rename·특수 파일명·경로 구간 경계·generated/unclassified 혼입·index bytes/hash/mtime 불변성을 검증한다.

- **v0.50.71 RC 릴리스 계보 및 업데이트 경로 완성** (2026-07-16): 최신 `main`의 GPT Ultra 구현과 `v0.50.70`의 companion 릴리스 계보를 실제 merge ancestry로 통합했다. 릴리스 워크플로는 정확한 `v0.50.71`만 허용하고, A2가 immutable A1의 commit·annotated tag object·checksums·Darwin archive·manifest pin과 A0부터 이어진 공개키 receipt/signature/record를 모두 검증한 뒤 같은 key record를 재게시하도록 fail-closed로 확장했다. Homebrew Cask를 canonical 배포 경로로 유지하면서 기존 Formula 사용자는 한 번의 호환 릴리스로 갱신한 뒤 Cask로 옮기도록 CAS 기반 bridge와 설치 안내를 추가했으며, 바이너리 갱신 뒤 `auto update`를 실행해야 프로젝트의 생성 파일이 반영된다는 경계도 문서화했다. 이 항목은 RC 코드 준비 상태를 기록하며 태그 생성·실제 릴리스·정책 promotion·Ultra compact 활성화·Desktop B0/R1 완료를 뜻하지 않는다.

- **GPT/Codex 검증형 필수-context 전달** (2026-07-15): `auto workflow context`가 core/SPEC와 존재하는 architecture 문서를 전체 로드하고 secret redaction 및 injection neutralization을 적용해, 변환 전 raw source hash와 실제 delivered prompt hash를 complete body-free manifest에 함께 기록한다. `auto workflow binding`은 rollout receipt와 root/command/SPEC/hash 및 supervisor 추가 필수-ref 집합을 정확히 대조해 omission/replay를 거부하고 실패 시 canonical `full_ultra`를 유지한다. retained worker는 최종 worktree 배정 뒤 snapshot을 한 번 만들어 GPT/Codex direct 호출과 모든 pipeline phase에서 동일하게 재사용하고, planner 이후 phase에는 원 태스크를 한 번씩 보존한다. `context_ack`는 진단 신호이고 enforcement는 expected refs와 hash 검증이다. configured reviewer가 모두 codex/openai/gpt alias일 때만 `auto spec review`가 각 리비전에서 반복 가능한 `--required-document`와 `--conditional-profile`을 포함한 supervisor-held 옵션으로 `BuildContextDelivery`를 실행하고 같은 옵션으로 엄격하게 검증한다. 검증된 core, 존재하는 architecture, 추가 문서 전문과 spec/plan/research/acceptance 전문은 중복 없이 provider prompt에 들어가며, 누락·변조·ref 집합 불일치·stale·wrong-SPEC·128K 초과는 orchestra/provider 호출 전에 차단된다. mixed/Claude/Gemini review와 non-GPT runtime은 기존 동작을 유지한다. 토큰 budget은 optional recall에만 적용하며, 이 변경은 release·promotion·generated workspace 반영을 뜻하지 않는다.

- **Desktop 장치 설정 릴리스 handoff (SPEC-DESKTOP-DEVICE-SETUP-001)** (2026-07-14): 결정론적 companion manifest producer, detached Ed25519 서명, Darwin 릴리스 배선, 검증·롤백 계약은 코드로 구현됐으며 관련 집중 테스트가 통과했다. 다만 `T11`과 `AC-019`는 충족되지 않았으며 릴리스도 완료되지 않았다. Production Desktop 릴리스 정책의 `pinnedPublicKeys`와 `priorReleaseKeyIds`는 비어 있고, 실제 이전 릴리스 키 ID와의 중첩이 필요하다. 실제 key ID와 public key는 릴리스 custodian이 제공해야 하며 fixture 값을 사용할 수 없다. 서명·공증된 현재 ADK 아티팩트와 실제 릴리스 증거는 외부 blocker로 남아 있다.

- **Codex Ultra 역할별 worker effort 적용** (2026-07-11): Codex Ultra의 quality-managed supervisor와 orchestra는 Sol+`ultra`를 유지하고, 전략·보안 역할인 `planner`·`architect`·`security-auditor`만 Sol+`max`, 그 밖의 모든 관리형 에이전트와 unknown role은 Sol+`xhigh`를 사용하도록 중앙 profile resolver를 조정했다. `auto quality ultra --apply`와 fresh init이 같은 3개/나머지 역할 분리를 생성하며, 어떤 managed worker도 자동 task delegation이 있는 `ultra` effort를 받지 않는다.

- **Codex 사용자 기본 모델 상속 및 품질 모드 즉시 적용** (2026-07-11): 새 프로젝트는 `quality.supervisor_model_policy: inherit`를 사용해 `.codex/config.toml`에 주 세션 model/effort를 쓰지 않고 사용자의 Codex 기본값을 상속한다. 정책 필드가 없는 기존 프로젝트의 markerless root 설정은 보존 우선으로 이행하며, `auto quality supervisor inherit|quality --apply`로 소유권을 명시할 수 있다. Codex 설정 업데이트는 사용자 소유 키 목록을 기록해 해당 assignment만 반복 업데이트 후에도 보존하고, 비모델 checksum drift가 생성 model/effort를 고정하지 않도록 한다. `auto quality ultra|balanced --apply`는 설정을 원문 보존 방식으로 저장한 뒤 현재 프로젝트의 플랫폼 하네스를 갱신하고, 일부 플랫폼 실패 시 적용 수와 정확한 재시도 명령을 출력한다.

- **디자인 시스템 문서 provider preflight** (2026-07-10): `auto design docs`가 프로젝트의 Astryx, shadcn/Radix/Tailwind, 로컬 디자인 소스를 metadata-only로 감지하고 component/template/token 문서 preflight를 Markdown 또는 JSON으로 출력한다. `auto design pack`에도 같은 provider 보고서와 setup gap을 포함하며, `design.docs_providers`로 탐지 범위를 제한할 수 있다. Frontend executor, reviewer, verifier와 Claude/Codex/Gemini 템플릿은 감지된 provider의 실제 props/import/token 문서를 먼저 확인하고, Astryx가 없는 프로젝트에는 의존성을 추가하지 않도록 동기화했다.

- **GPT-5.6 기반 Codex 품질 프로필 통합 (SPEC-CODEXQUAL-001)** (2026-07-10): Codex supervisor, managed subagent/native multi-agent, orchestra의 모델·reasoning effort 결정을 하나의 quality resolver로 통합했다. Balanced는 supervisor/orchestra와 Opus-tier worker에 `gpt-5.6-sol+xhigh`, Sonnet-tier worker에 `gpt-5.6-terra+role effort`, Haiku-tier worker에 `gpt-5.6-luna+role effort`를 사용한다. Ultra는 depth-0 supervisor/orchestra에 자동 task delegation을 포함한 Sol+`ultra`, 이미 명시적으로 배치된 managed worker에 Sol+`max`를 사용한다. `codex debug models` structured catalog로 같은 모델의 effort downgrade를 먼저 적용하고, 모델 미지원 시 `gpt-5.5`, 호환 모델 부재 시 runtime default로 관측 가능하게 fallback한다. Orchestra provider에는 `model_policy: quality|pinned` 소유권을 도입해 exact historical canonical 설정만 이행하고 custom argv와 기존 사용자 root model/effort는 보존한다. Runtime `--quality`/`--effort`는 일반 orchestra, `orchestra run`, structured SPEC review에 동일하게 적용된다.

- **Worker redline 수정 지침 전달 배선 (SPEC-REDLINE-EDIT-WIRE-001)** (2026-07-09): ADK worker가 A2A payload의 `redline_instructions`를 block ID와 정제된 수정 지침만 포함하는 untrusted JSON section으로 만들어 prompt 끝에 추가한다. 빈 항목과 digest·approval binding 필드는 제외하며, carrier가 없으면 기존 prompt를 그대로 유지한다. 회귀 테스트는 허용 필드만 전달되고 신뢰 경계 문구가 유지되는지 검증한다.

- **라이브 실행 경로 방어 배선 (SPEC-ADK-LIVEPATH-DEFENSE-001, completed 2026-07-08)**: 설계 감사에서 발견된 "구현돼 있지만 라이브 경로가 우회하던" 3개 방어를 실제 실행 경로에 연결했다. 인터랙티브 debate/rebuttal/judge prompt builder가 subprocess 템플릿과 같은 `AUTOPUS_PART_<hex>-BEGIN/END` fence와 SECURITY NOTE를 적용해 forged header/ignore-instruction payload를 데이터 영역 안에 가둔다. wired `pkg/worker/parallel.WorktreeManager.Create`는 `refs.lock`/`packed-refs.lock`류 shared-lock 실패에 base 3s, factor 2, 최대 3회 retry를 수행하고, dead-in-production `pkg/pipeline.WorktreeManager` private retry duplicate는 제거하되 `NewWorktreeManager`/`Create`/`Remove`/`ActiveCount` public API는 보존했다. 신규 `pkg/experiment.Loop`와 `auto experiment run`은 `MaxIterations`, `CircuitBreakerN`, context cancellation, `ExperimentTimeout`을 in-process hard stop으로 강제하며 `stop_reason`/`total_iterations`를 출력한다. 추가 hardening으로 `pkg/worker/taskid`가 worktree task ID를 branch/path 생성 전에 검증하고, metric command validator가 `&` background operator를 거부한다. 검증: `go test ./pkg/orchestra/... ./pkg/worker/... ./pkg/pipeline/... ./pkg/experiment/... ./internal/cli/...` PASS, `auto spec validate .autopus/specs/SPEC-ADK-LIVEPATH-DEFENSE-001 --strict` PASS, review PASS. Completion Verdict: Outcome Lock satisfied, mandatory 12/12, Must acceptance 7/7, Completion Debt none.

- **route_team 안정성을 manual pipeline 수준으로 격상 (SPEC-HARNESS-WORKFLOW-STABILITY-001)** (2026-06-29): claude-code에서 `auto workflow doctor` 통과 시 `/auto go --team`이 서빙하는 결정적 `route_team` substrate의 네 안정성 격차를 닫음 — ①`gate_build_test` 실패가 즉시 abort가 아니라 `MaxRetry=3`로 bounded RALF remediation + no-progress circuit-break(`pkg/workflow/remediation.go::RunGateRemediation`, `AbortReason="circuit_break_no_progress"`), ②결정적 85% coverage gate(`[NEW] pkg/workflow/coverage_gate.go::EvaluateCoverageGate` — exit-code-only `CommandRunner`로는 stdout을 못 얻으므로 `CoverageRunner.RunOutput` stdout seam 신설, `GateResult`/`VerdictSourceExitCode` 재사용), ③security>code-quality review barrier(`RunReviewBarrier`/`ConsolidateReviewVerdict`, security FAIL → `Barrier=true`/`Reason="security_fail"`), ④phantom config key 제거 — `[NEW] pkg/config/schema_workflow.go::WorkflowConf{TeamDefault,CoverageThreshold}`를 `HarnessConfig`에 추가하고 `DefaultFullConfig` 기본 `TeamDefault=true`(현행 동작 보존) + `applyMissingDefaults` Load-path backfill(섹션 부재/부분 시 zero-value false 금지) + `Validate`/`ParseSchema` 0..100 범위 검증(named error). 안정성은 prose가 아니라 **실 dispatcher 재진입점**으로 보장: `deriveTeamWorkflowJS`를 multi-segment(A planning~gate / B annotation~testing / C review / D release_hygiene)로 확장해 coverage gate가 testing↔review 사이, review barrier가 review↔release_hygiene 사이에 결정적으로 끼어듦(`SEGMENT==='C'`/`'D'` guard). 결정 로직은 LLM-free 순수 Go 함수가 source of truth이고 JS는 verdict 없는 boundary marker 유지. dispatcher contract(`content/skills/harness-workflow.md`·`agent-teams.md`·`auto-router.md.tmpl`·gemini mirror) A→B→C→D 정합화, parity gate(`pkg/content/workflow_parity.go`)를 새 schema field로 fail-closed 확장. regression-0: route_a 2-segment·doctor version pin(2.1.154)·non-claude 플랫폼 불변. 검증: build/vet/gofmt clean, `go test -race -cover ./pkg/workflow/... ./pkg/config/... ./pkg/content/...` 전부 ok(89.2%/87.8%/94.4%, 전부 ≥85%), review.md PASS·6 findings(F-001~006) 전부 resolved, 모든 새 `.go` ≤300줄. **잔여(operational residual, Completion Debt 아님)**: 실 claude-code 세션 `/auto go --team` 라이브 클릭스루는 hermetic 증명 불가(로직은 S1–S12 hermetic oracle로 검증됨).

- **Autopus 기본 minimality discipline (SPEC-ADK-MINIMALITY-DISCIPLINE-001)** (2026-06-27): `@auto plan`, `@auto go`, `@auto fix`, `@auto review` guidance에 "필요한 만큼만 구현" 결정을 기본 discipline으로 배선. plan/spec-writer는 `Minimality Decision Matrix`와 신규 dependency/abstraction justification을 기록하고, go/agent-pipeline은 existing code/helper/pattern 우선 탐색과 minimum sufficient verification receipt를 요구하며, fix/debugger는 caller/shared root-cause 확인 없이는 symptom-only patch를 `revise-target`으로 남긴다. review/reviewer/shared orchestra reviewer는 `Correctness/Security Findings`와 `Complexity Findings`를 분리하고 complexity tag(`delete`, `stdlib`, `native`, `yagni`, `shrink`, `existing-helper`, `existing-dependency`)를 고정했다. `qualityloop`는 minimality reason code 반복 신호를 inactive skill/playbook candidate로 라우팅하고, `skillevolve` path policy는 generated/runtime/plugin-cache/root artifact 변형을 fail-closed로 막는다. 검증: `go test ./templates ./pkg/adapter/codex ./pkg/adapter/opencode ./pkg/adapter/gemini ./pkg/qualityloop ./pkg/skillevolve`, `go run ./cmd/auto spec validate .autopus/specs/SPEC-ADK-MINIMALITY-DISCIPLINE-001 --strict`.

- **Homebrew tap 자동 배포 활성화 (goreleaser `brews`)** (2026-06-22): 릴리즈 시 `auto` CLI formula를 `Insajin/homebrew-autopus` tap repo로 자동 publish하도록 배선. tap repo 신규 생성(public), `.goreleaser.yaml`의 `brews` 블록 활성화(`name: auto`·`repository: Insajin/homebrew-autopus@main`·`directory: Formula`·`install bin.install "auto"`·`test auto version`), `release.yaml`가 cross-repo 푸시 토큰 `HOMEBREW_TAP_TOKEN`을 goreleaser에 전달. `goreleaser check` 통과(brews는 향후 goreleaser v3에서 `homebrew_casks`로 마이그 필요 — deprecation 경고만, v2에서 동작). ⚠️**운영 선행조건**: tap repo에 contents:write 권한을 가진 PAT/fine-grained 토큰을 autopus-adk 저장소 시크릿 `HOMEBREW_TAP_TOKEN`으로 등록해야 다음 릴리즈에서 formula가 publish됨(기본 `GITHUB_TOKEN`은 동일 repo만 접근). 등록 후 `brew install Insajin/autopus/auto` 사용 가능.

- **route_team executor file-ownership 하드 강제 — `auto workflow merge --ownership` (SPEC-HARNESS-WORKFLOW-FIDELITY-001 improvement)** (2026-06-22): executor coordination을 확률적 prompt 유도에서 merge-time 하드 보장으로 격상. 생성된 segment A가 planner 산출(`plan`)을 **return**하고(생성기: top-level `let plan`+`return { plan }`), 디스패처가 이를 임시 JSON으로 persist해 `auto workflow merge --run <id> --ownership <plan.json>`로 전달한다. merge는 각 worktree를 수행한 task에 **1:1 global best-overlap 배정**(`assignWorktreesToTasks`: 최고 overlap pair부터 greedy 점유 → 두 worktree가 한 task를 공유하지 않음, 어긋난 executor도 실제 수행 task로 강제됨)하고 **그 task 소유 파일만** 머지한다. 소유 밖 파일(executor가 다른 task 파일로 overreach)은 `skipped_out_of_scope`로 보고하고 복사하지 않아 executor 오버랩 충돌을 원천 제거. planner 경로 정규화(absolute/prefixed LLM 경로 ↔ repo-relative suffix 매칭, `ownsFile`). `--ownership` 미제공 시 기존 conflict-skip 폴백 유지. `pkg/workflow/merge_ownership.go`(`TaskOwnership`·`ParsePlanOwnership`{tasks}/{plan.tasks}·`ownsFile`·`assignWorktreesToTasks`) + merge.go split(discovery → `merge_discover.go`로 300줄 한계 유지). 회귀 테스트 `merge_ownership_test.go`(parse·suffix-match·강제 overlap 시 stray 드롭+owner 콘텐츠 우선). ⭐**실 런타임 검증**: greeting(impl+test) SPEC로 segment A → planner가 **1 task로 그룹화**(coordination fix 실증, 설명에 "must never be split across worktrees") → segment A가 `{plan}` return(seam 실증) → 디스패처가 plan.json 기록 → `merge --ownership`이 owned 파일 둘 다 머지(exit 0). 게이트 green: build/vet/gofmt·`-race`(pkg/workflow)·golangci 0 issues·file-size≤300.

- **`auto workflow merge` — route_team executor worktree consolidation (SPEC-HARNESS-WORKFLOW-FIDELITY-001 live e2e 후속)** (2026-06-22): FIDELITY-001 executor 종단 e2e가 substrate-level blocker를 드러냄 — route_team 병렬 executor는 Workflow 런타임 `isolation:'worktree'`로 각자 격리 worktree(`.claude/worktrees/wf_<runid>-<N>`)에서 작업하고 **uncommitted 변경을 남기는데**(commit 아님, 실 런타임 실측), 이를 merge하는 단계가 없어 executor 산출이 orphan → `auto workflow gate`가 무변경 main tree를 vacuous pass → route_team 종단 비기능이었다. **신규 결정적 Go 단계** `auto workflow merge --run <runid> [--working-dir]`: runID에 속한 worktree를 `git worktree list --porcelain`으로 열거(`<workingDir>/.claude/worktrees/` 봉쇄), 각 worktree의 `git status --porcelain` 변경 파일을 workingDir로 복사(file-ownership 충돌은 감지·skip·report), `git add` 스테이징 후 worktree+브랜치 정리. `pkg/workflow/merge.go`(+`merge_copy.go`·`merge_links_{unix,other}.go`, GitOutputRunner stdout seam, `pkg/workflow`→`internal/cli` 미import 경계 유지) + `internal/cli/workflow_merge.go`. 디스패처 계약(`content/skills/harness-workflow.md ### Segmented Dispatch Contract`)을 **5-step**으로 갱신: launch A → **merge** → gate → launch B → hygiene. ⭐**실 런타임 live 통합 검증**: 2 executor가 격리 worktree에 disjoint 파일 생성 → merge가 둘 다 consolidate+stage+worktree 제거(exit 0). ⭐⭐**security-auditor 3-라운드 적대 감사로 실 취약점 4건 발견·수정·재실측 PASS**: H-2(과다선택 데이터손실 — `.claude/worktrees/` EvalSymlinks 봉쇄로 main/외부 worktree 제외), M-1(심링크 dst 탈출/비원자 쓰기 — `ensureWithin` EvalSymlinks 조상 해석 + temp+rename), H-1(심링크 src exfil — Lstat 거부 + `O_NOFOLLOW`), H-1-RESIDUAL(**하드링크 src exfil**, 실측 비밀 유출 확인 → `Nlink>1` 수집스킵 + fd `in.Stat()` 백스톱 이중방어; build-tagged unix/non-unix). 회귀 테스트 `pkg/workflow/merge_security_test.go`(symlink/hardlink/containment/deleted) + `merge_test.go`(disjoint/conflict/traversal/unrelated/empty). 최종 감사 Critical/High 0. 게이트 green: build/vet/gofmt·`-race`(pkg/workflow·internal/cli)·file-size≤300. 잔여 L-2(특수문자 파일명 무성 누락, 비차단). 이로써 FIDELITY-001 worktree-merge 종단 blocker 해소(남은 것은 단일 실 SPEC chained run).

### Fixed

- **공유 ADK의 visual snapshot 검증 호환성 복구** (2026-07-16): Desktop 채택 요구를 모든 프로젝트의 기본 hard failure로 확장하던 후보를 호환성 우선으로 재구성했다. 기존 `.autopus/design/verify/latest.json`과 Go report 타입은 v1 그대로 유지하고 상세 assertion/project/proof는 `latest.v2.json` 및 별도 V2 타입으로 분리한다. snapshot proof 누락·비활성·Playwright 1.58 공개 API 한계는 기본 실행에서 finding으로만 남고 `--strict-visual-gate`에서만 차단되며, 실제 Playwright 실행 오류는 계속 항상 실패한다. Reporter는 private `_fullProject` 대신 1.59+ 공개 API를 사용하고 `--update-snapshots=none`을 강제한다. custom project별 최종 PASS, retry/result pairing, 이미지 자원 한도, `os.Root` 경로 고정, `legacy_sha256` 세대 결합, 정제된 stderr 진단을 회귀 계약으로 고정했다. QAMESH ingestion은 실제 consumer가 없어 v2에서는 handoff 후보를 `WARN`으로 기록한다. v1의 기존 `qamesh_handoff` 상태와 check 순서는 공유 소비자 호환성을 위해 그대로 유지하며, ingestion 성공의 근거로 사용하지 않는다. 이 변경은 최신 `v0.50.70` 보호 릴리스 계보 위의 로컬 통합 후보이며 태그·릴리스는 포함하지 않는다.

- **기존 Codex 프로젝트의 모델 상속 이행** (2026-07-11): `auto update`가 Autopus에서 생성한 뒤 수정되지 않은 과거 `.codex/config.toml`의 `gpt-5.5+xhigh` 설정을 감지하면 `quality.supervisor_model_policy: inherit`를 명시하고 프로젝트의 model/effort override를 제거한다. 생성 헤더, manifest merge 정책, 전체 파일 checksum, 과거 관리 tuple이 모두 일치할 때만 자동 이행하며, 사용자 marker, checksum drift, custom tuple은 그대로 보존한다. 같은 릴리스에서 잘못 `pinned`로 기록한 정확한 v0.50.66 Codex orchestra provider는 `quality`로 복구하되 near-match와 명시적 최신 정책은 변경하지 않는다. `auto update --plan`은 쓰기 없이 예정된 이행을 표시한다. Codex 갱신이 실패하면 supervisor policy를 원복하고, workspace 후속 target이 실패하면 앞선 target과 현재 target의 generated transaction 및 원래 `autopus.yaml`을 함께 복원한다. `auto doctor`는 실제 merge와 같은 키 단위 ownership 규칙으로 legacy shadowing, 소유권이 모호한 설정, 적용되지 않은 explicit `inherit`를 읽기 전용 경고로 보고한다.

- **플랫폼별 스킬 제외 출력 명확화** (2026-07-11): Codex와 Gemini 하네스 업데이트에서 Claude 전용 스킬을 오류처럼 보이는 `incompatible`로 표시하던 문구를 `platform-skipped`로 바꾸고, 제외된 스킬 이름을 함께 표시한다. 플랫폼별 스킬 생성 동작은 그대로 유지한다.

- **Go 표준 라이브러리 TLS 취약점 대응** (2026-07-10): toolchain과 Security workflow를 Go `1.26.5`로 올려 `crypto/tls`의 Encrypted Client Hello privacy leak인 `GO-2026-5856`을 해소했다. Security Scan이 patch version을 명시적으로 설치하므로 runner의 `1.26` 해석이나 캐시 상태와 관계없이 수정된 표준 라이브러리로 `govulncheck`와 릴리즈 gate를 실행한다.

- **route_team executor coordination — planner가 상호의존 파일을 분리해 발생하던 merge conflict 예방 (SPEC-HARNESS-WORKFLOW-FIDELITY-001 chained run 발견)** (2026-06-22): chained run 관측 — planner가 impl(`greeting.go`)과 그 test(`greeting_test.go`)를 별도 task로 분리하면, test task의 executor가 격리 worktree에서 컴파일을 위해 impl을 재생성→두 executor가 같은 파일 소유→merge가 conflict로 skip→build 불가(fail-fast, 안전하나 run 재시도 필요). 근본 원인은 isolated 병렬 실행에 맞지 않는 task 분해. **수정**(`pkg/content/workflow_generate_team.go`): planner 프롬프트를 "병렬 isolated-worktree 실행용 disjoint task 분해 + 컴파일 상호의존 파일(impl+test, type+소비자)은 한 task로 그룹화·절대 분리 금지" 제약으로 enrich + executor 프롬프트에 "배정된 files만 소유/생성, 그 외 파일은 병렬 executor 소유라 손대지 말 것" 가드. **planner-only probe 실증**: 동일 impl+test SPEC→taskCount=1, greeting.go+greeting_test.go가 같은 task(grouped), planner가 "상호의존→단일 executor 소유 필수" 명시 추론. merge 무변경(conflict 정책 그대로 안전망).

- **`auto workflow merge`가 새 디렉터리 내 untracked 파일을 누락 (SPEC-HARNESS-WORKFLOW-FIDELITY-001 chained e2e 발견)** (2026-06-22): 단일 실 SPEC chained run(segment A→merge→gate→segment B) 중 발견. `changedFiles`가 `git status --porcelain`(기본)을 써서 **새 untracked 디렉터리를 `?? dir/` 한 줄로 collapse** → 코드가 이를 디렉터리(non-regular)로 보고 skip → executor가 **새 패키지 디렉터리**(흔한 경우: `pkg/greeting/`)에 만든 파일이 전부 merge에서 누락되고 worktree는 그대로 제거되어 산출 유실. **수정**: `git status --porcelain --untracked-files=all`로 중첩 파일을 개별 열거. 실-git 회귀 테스트 `pkg/workflow/merge_realgit_test.go::TestMerge_RealGit_NewDirectoryEnumerated`(temp repo에 새 디렉터리+중첩 파일 생성→개별 열거 단언; fake-runner 테스트는 CLI 플래그 누락을 못 잡으므로 실 git 필요)로 잠금. flat-file 단위/통합 테스트가 가렸던 버그 — 실 SPEC chained run(새 패키지 생성)이 적발.

- **생성 워크플로 JS의 `args` 전파 버그 — 실 런타임은 args를 JSON 문자열로 전달 (SPEC-HARNESS-WORKFLOW-FIDELITY-001 live e2e / SPEC-HARNESS-WORKFLOW-RUNTIME-001 후속)** (2026-06-22): FIDELITY-001 live e2e(실 Claude Code Workflow 런타임 v2.1.174 디스패치) 중 발견. **실 런타임은 `args` 글로벌을 parsed object가 아니라 JSON STRING으로 전달한다**(`typeof args === 'string'` 실측). 따라서 생성 JS의 `const SEGMENT = (args && args.segment) || 'A'`는 항상 'A'로 폴백(→ **segment B 영영 미실행 = segmented dispatch 무력화**)하고 `const ctx = args`는 `.spec` 부재로 빈 컨텍스트가 됐다. RUNTIME-001의 "launch PROVEN"은 args 없이 돌린 0-agent 실행이라 이 버그가 잠복(launch는 됐으나 args 실전 전달은 0회). **수정**: 두 생성기(`pkg/content/workflow_generate.go::deriveWorkflowJS`(route_a)·`workflow_generate_team.go::deriveTeamWorkflowJS`(route_team)) 공유 preamble에 `const ARGV = (typeof args === 'string') ? (args ? JSON.parse(args) : {}) : (args || {})` 정규화(공유 상수 `workflowArgvNormalizeJS`)를 추가하고 `ctx`/`RT`/`SEGMENT`를 ARGV에서 읽도록 변경(string·object·undefined 모두 수용). **재검증**: 수정 전 sentinel `segment:'Z'`는 planner를 잘못 spawn(SEGMENT 버그 'A')했으나 수정 후 동일 sentinel은 0 agents(SEGMENT 정상 'Z') → fix 실증. 추가로 실 planner agent(agentType:'planner')에 `schema: PLAN_SCHEMA` 디스패치 → 검증된 `{tasks:[{id,description,files}]}` 2-task 반환(file ownership 비충돌) 실증. `launch-contract` 오라클에 ARGV 정규화 단언 추가, route_a/route_team `.tmpl` 재생성(route_a golden 동반 갱신). 게이트 green: build/vet/gofmt·`-race`(content·workflow). 잔여: executor parallel/worktree 실 코드-write fan-out + 디스패처 segment-gate 종단 루프(파괴적·operational Completion Debt).

### Added

- **route_team 생성 JS를 충실한 전문 에이전트 팀 디스패치로 격상 (SPEC-HARNESS-WORKFLOW-FIDELITY-001, status: implemented)** (2026-06-22): `SPEC-HARNESS-WORKFLOW-RUNTIME-001`(launch 정합)의 후속. 생성 `route_team.workflow.js`는 launch는 되지만 디스패치가 thin skeleton(`agent(\`Execute <role> agent for spec ...\`, {model,effort})` — agentType 부재·얇은 프롬프트·index-only fan-out)이라 Route A 서브에이전트 파이프라인보다 충실도가 낮아 default-on 기본값으로 부적합했다. **격상**(`pkg/content/workflow_generate_team.go::deriveTeamWorkflowJS`): (1) phase별 등록된 `agentType` emit(planning→`planner`, test_scaffold→`tester`, implementation→`executor`, annotation→`annotator`, testing→`tester`, review→`reviewer`+`security-auditor`) → generic Workflow 에이전트 대신 전문 subagent 시스템 프롬프트(TRUST-5/TDD/OWASP/@AX) 적용; (2) planning이 inline `PLAN_SCHEMA`(id/description/files)로 structured-output 캡처(`const plan = await agent(..., {agentType:'planner', schema: PLAN_SCHEMA, ...})`); (3) implementation fan-out이 `plan.tasks`를 task별로 thread(`min(plan.tasks.length, fan_out_cap≤5)`, task id/description/file-ownership 프롬프트)하여 `parallel(executors)` + `isolation:'worktree'`로 실행; (4) task-focused 프롬프트 enrichment(bare "Execute <role> agent" skeleton 제거). ⭐**실 Workflow 런타임 API 정합(correctness)**: baseline의 `parallel(...executors)`(이미 호출된 promise를 spread)는 실 계약 `parallel(thunks: Array<() => Promise>)`과 어긋나 런타임 crash했을 형태 → `executors.push(() => agent(...))` thunk + `parallel(executors)` 배열로 수정. **0-서브에이전트 probe를 실 Workflow 런타임에 디스패치해 경험적 확인**(`parallel([()=>...])`→결과 배열·`parallel([])`→clean no-op·0 agents·25ms·에러 0). ⭐**F-001 fail-fast(feasibility)**: 생성 JS가 `parallel`/`isolation`을 hard-require하므로 `pkg/workflow/doctor.go`에서 둘을 AdvisoryPrimitives→**RequiredPrimitives 승격** — 미지원 런타임은 doctor fail → 안전한 Route A 폴백(launch 후 mid-crash 방지). liveProber는 primitive별 동일 `present` 반환이라 실 경로 불변. ⭐**F-002 degenerate floor(completeness)**: 빈/실패 `plan.tasks`의 zero-executor silent no-op → 전체-SPEC 단일 fallback executor floor. ⭐**F-004**: review synthesis/vote 호출 모두 `agentType:'reviewer'`(count==2)·audit는 `security-auditor`(count==1) 일관성 잠금. 신규 hermetic 오라클 `pkg/content/workflow_fidelity_contract_test.go`(`agentType`/`schema: PLAN_SCHEMA`/`parallel(`/`push(() => agent(`/`isolation:'worktree'`/`Math.min`+`plan.tasks`/floor 단언 + skeleton-prompt 음성 단언) + `workflow_generate_team_test.go`/`workflow_launch_contract_test.go` faithful 단언 갱신. **trust boundary**: planner task description(untrusted LLM 산출)은 런타임 `plan.tasks[i]` 데이터로만 소비(생성 텍스트 미보간), JS-injection whitelist(phase-id/model/effort/result_type) 불변, agentType는 고정 Go 리터럴. **회귀 0**: route_a 생성 표면 byte-unchanged(`TestS19_RouteARegressionGolden`)·비-claude `--team` 불변. 게이트 green: build/vet/gofmt/`-race`(content·workflow·cli)·file-size≤300·`.tmpl` 재생성 byte-stable·idempotent. SPEC review judge(claude) PASS(critical 0/security 0/major 1=F-001) + 후속 reviewer APPROVE(0 blocker) + security-auditor PASS(0 Critical/High/Medium). **Completion Debt(잔여)**: route_team 실 multi-agent real-LLM 종단 실행(specialized agentType spawn + task-threaded parallel/worktree 실 honor)은 hermetic 불가한 operational 잔여 → `implemented` 유지(운영 검증 후 completed 승격).

- **생성 워크플로 JS를 실제 Workflow 런타임 API에 정합 + segmented dispatch (SPEC-HARNESS-WORKFLOW-RUNTIME-001, status: implemented)** (2026-06-22): `SPEC-HARNESS-WORKFLOW-001`(route_a)·`SPEC-HARNESS-WORKFLOW-TEAM-001`(route_team)이 생성한 `.claude/workflows/route_{a,team}.workflow.js`는 실제 Claude Code Workflow 런타임에서 **`SyntaxError: Unexpected keyword 'export'`로 launch 불가**였다(실 디스패치로 실증). 근본 원인: 생성 JS가 런타임 미지원 API(`export default async function run()` 엔트리 + `env()`·`agent.exec()`·role-only `agent('executor')`·3-인자 `phase()`)를 가정. **수정**: 두 생성기(`pkg/content/workflow_generate.go::deriveWorkflowJS`·`workflow_generate_team.go::deriveTeamWorkflowJS`)가 실 API(단일 `export const meta` + top-level 본문 + `agent(prompt, opts)`(prompt=task 문자열·`${ctx.spec}` 보간) + `args` 글로벌, `export default`/`env(`/`agent.exec(` 0)를 방출하도록 재작성. **Segmented dispatch**(결정적 게이트를 실 barrier로): 단일 `workflow({scriptPath}, args)` launch가 전 phase를 무조건 실행하므로 외부 Go 게이트가 mid-launch를 못 막는다 → 생성 JS에 `const SEGMENT = (args && args.segment) || 'A'` 가드(A = …→`gate_build_test` 마커, B = `annotation`→…→`release_hygiene` 마커)를 두고, 디스패처가 segment A launch → `auto workflow gate`(exit-code) hard barrier → verdict=pass일 때만 segment B launch → `auto check --hygiene --arch --quiet --staged`. 게이트 phase는 `phase(id)+log()` 경계 마커이며 실행은 JS 밖 Go(`verdict_source: exit_code` 보존, "JS는 sequencing만" 경계 유지). 품질 binding 전달 채널 `env`→`args.quality`. 신규 `pkg/content/workflow_launch_contract_test.go`(S1/S2/S3·S11) anti-theater 오라클: 첫 토큰 `export const meta`·`export` 1개·`export default`/`env(`/`agent.exec(` 0·SEGMENT preamble·segment A 마지막 phase=`gate_build_test`·segment B 첫 phase=`annotation`. ⭐**보안(security-auditor V-1, High proven JS-injection)**: `verdict_source`(`PhaseDef.ResultType`)가 model/effort/phase-id와 달리 parse 경계 미검증 → newline이 생성 JS `//` 주석을 종료시켜 실행문 emit·parity 통과 → `pkg/workflow/schema_validate.go::isSafeResultType`(whitelist `""|"exit_code"`)를 `ParseSchema`에서 강제 + 회귀 `TestParseSchema_RejectsUnsafeResultType`. ⭐⭐**operational launch PROVEN**: 재생성 route_a를 실 Workflow 런타임 디스패치 → 완료(0 agents·SyntaxError 0); route_team은 구조동치 + hermetic contract test. 디스패처 계약 docs(`content/skills/harness-workflow.md`·`agent-teams.md`·`templates/claude/commands/auto-router.md.tmpl`)에 2-segment + args `{spec, workingDir, quality, segment}` 명세. 게이트 green: build/vet/gofmt/`-race`(content·workflow·cli)·file-size≤300. reviewer APPROVE(blocking 봉합)·security PASS(V-1 수정 후). 잔여: route_team 실 멀티에이전트 end-to-end 1회(operational, 본 commit 범위 밖).

- **`--team`을 claude-code 결정적 Workflow 기반층으로 대체 (SPEC-HARNESS-WORKFLOW-TEAM-001, status: implemented)** (2026-06-21): 부모 `SPEC-HARNESS-WORKFLOW-001`(route_a, 비-team `--workflow`)에 HARD 의존하며, claude-code 플랫폼에 한해 `/auto go --team`의 "parallel multi-agent" 의도를 **결정적 team Workflow 실행 기반층**(`route_team`)으로 해소한다. 정본(SoT) = `content/workflows/route_team.{md,schema.json}` 2파일(route_a를 재정의하지 않는 별도 8-phase 집합: planning → test_scaffold → implementation(병렬/worktree) → gate_build_test(Gate 2, exit-code) → annotation → testing → review(reviewer+security_auditor) → release_hygiene), JS는 `templates/claude/workflows/route_team.workflow.js.tmpl` → 설치 시 `.claude/workflows/route_team.workflow.js`(편집금지 generated-surface, manifest 파생). 품질 모드는 **모델 tier(기존 `pkg/cost/pricing.go::ModelForAgent`)·effort(기존 `internal/cli/effort_resolve.go::ResolveEffort`)·오케스트레이션 깊이(`pkg/workflow/depth.go::ResolveDepth`, bounded: MaxVerifyVotes=3/MaxFanOut=5/MaxRetry=3)** 세 축을 동시 구동한다. 아키텍처 경계 보존(`pkg/workflow`는 `internal/cli` 미import): 품질→(model,effort) 해석은 CLI 디스패치(`internal/cli/workflow_quality_binding.go`)가 수행하고 결과를 `pkg/workflow/binding.go::QualityBinding` 데이터로 주입하며, 생성 JS는 `RT = JSON.parse(env('AUTOPUS_WORKFLOW_QUALITY'))` override seam을 baseline 리터럴 fallback과 함께 읽는다(런타임 우선). model/effort 문자열은 생성 JS에 보간되므로 `pkg/workflow/schema_validate.go` whitelist/enum으로 parse 경계 fail-closed(JS-injection 방어). parity 게이트(`pkg/content/workflow_parity.go`)는 model/effort/depth 필드까지 확장·fail-closed. `auto workflow render --route team --quality <mode>`가 route 선택+overlay를 노출, claude adapter(`pkg/adapter/claude/claude_workflow.go`)가 route_a와 route_team JS를 함께 설치. routing/fallback taxonomy 1:1 보존(disable=`--no-workflow`/config `workflow.team_default=false` → 기존 Agent Teams; doctor fail → fail-fast Route A; 비-claude → `--team` 불변 = **회귀 0**). `--multi`는 직교(substrate 비결합). SPEC review PASS 69/69(claude·codex·gemini, F-001/F-002 suggestion resolved). sync 시점 게이트 재실행 green: `go build`/`vet`/`gofmt`/`-race`(pkg/workflow·pkg/content·internal/cli·pkg/adapter/claude·pkg/cost), file-size 전 신규 `.go` ≤144, `auto workflow doctor` overall=pass(v2.1.174), team render가 8-phase·ultra overlay(implementation model=claude-opus-4-8/effort=max·review votes=3/synthesis=true)·fan-out·env seam 노출(S16/S18/S19/S20). **Completion Debt NOT none**: live end-to-end `/auto go --team --quality ultra` 실행(claude-code Workflow 런타임이 설치된 route_team.workflow.js를 실제 실행, 실 LLM 트래픽으로 executor×N+reviewer+security_auditor 구동)은 결정적 hermetic 오라클로 불가한 operational 잔여이며 SPEC 설계상 **sync completion을 차단**한다 → status `completed` 미승격, `implemented` 유지(운영 검증 후 재-sync 필요). 부모 `SPEC-HARNESS-WORKFLOW-001`(route_a, 비-team `--workflow`)에 HARD 의존하며, claude-code 플랫폼에 한해 `/auto go --team`의 "parallel multi-agent" 의도를 **결정적 team Workflow 실행 기반층**(`route_team`)으로 해소한다. 정본(SoT) = `content/workflows/route_team.{md,schema.json}` 2파일(route_a를 재정의하지 않는 별도 8-phase 집합: planning → test_scaffold → implementation(병렬/worktree) → gate_build_test(Gate 2, exit-code) → annotation → testing → review(reviewer+security_auditor) → release_hygiene), JS는 `templates/claude/workflows/route_team.workflow.js.tmpl` → 설치 시 `.claude/workflows/route_team.workflow.js`(편집금지 generated-surface, manifest 파생). 품질 모드는 **모델 tier(기존 `pkg/cost/pricing.go::ModelForAgent`)·effort(기존 `internal/cli/effort_resolve.go::ResolveEffort`)·오케스트레이션 깊이(`pkg/workflow/depth.go::ResolveDepth`, bounded: MaxVerifyVotes=3/MaxFanOut=5/MaxRetry=3)** 세 축을 동시 구동한다. 아키텍처 경계 보존(`pkg/workflow`는 `internal/cli` 미import): 품질→(model,effort) 해석은 CLI 디스패치(`internal/cli/workflow_quality_binding.go`)가 수행하고 결과를 `pkg/workflow/binding.go::QualityBinding` 데이터로 주입하며, 생성 JS는 `RT = JSON.parse(env('AUTOPUS_WORKFLOW_QUALITY'))` override seam을 baseline 리터럴 fallback과 함께 읽는다(런타임 우선). model/effort 문자열은 생성 JS에 보간되므로 `pkg/workflow/schema_validate.go` whitelist/enum으로 parse 경계 fail-closed(JS-injection 방어). parity 게이트(`pkg/content/workflow_parity.go`)는 model/effort/depth 필드까지 확장·fail-closed. `auto workflow render --route team --quality <mode>`가 route 선택+overlay를 노출, claude adapter(`pkg/adapter/claude/claude_workflow.go`)가 route_a와 route_team JS를 함께 설치. routing/fallback taxonomy 1:1 보존(disable=`--no-workflow`/config `workflow.team_default=false` → 기존 Agent Teams; doctor fail → fail-fast Route A; 비-claude → `--team` 불변 = **회귀 0**). `--multi`는 직교(substrate 비결합). SPEC review PASS 69/69(claude·codex·gemini, F-001/F-002 suggestion resolved). sync 시점 게이트 재실행 green: `go build`/`vet`/`gofmt`/`-race`(pkg/workflow·pkg/content·internal/cli·pkg/adapter/claude·pkg/cost), file-size 전 신규 `.go` ≤144, `auto workflow doctor` overall=pass(v2.1.174), team render가 8-phase·ultra overlay(implementation model=claude-opus-4-8/effort=max·review votes=3/synthesis=true)·fan-out·env seam 노출(S16/S18/S19/S20). **Completion Debt NOT none**: live end-to-end `/auto go --team --quality ultra` 실행(claude-code Workflow 런타임이 설치된 route_team.workflow.js를 실제 실행, 실 LLM 트래픽으로 executor×N+reviewer+security_auditor 구동)은 결정적 hermetic 오라클로 불가한 operational 잔여이며 SPEC 설계상 **sync completion을 차단**한다 → status `completed` 미승격, `implemented` 유지(운영 검증 후 재-sync 필요).

- **결정적 `--workflow` opt-in 라우트 기반층 (SPEC-HARNESS-WORKFLOW-001)** (2026-06-19): `/auto go` Route A를 Claude Code의 Dynamic Workflows 위에서 결정적으로 실행하는 claude-code 전용 opt-in 라우트의 안전 기반층을 추가한다. **정본(SoT) = manifest 2파일**(`content/workflows/route_a.md` 사람 계약 + `route_a.schema.json` phase-id/retry/budget/result-type 권위)이며, JS(`templates/claude/workflows/route_a.workflow.js.tmpl` → 설치 시 `.claude/workflows/route_a.workflow.js`)는 manifest에서 파생되는 **편집금지 generated-surface**다(Workflow 저작 API가 무계약 내부 프리미티브라 고정 JS 정본 핀을 기각). `pkg/content/workflow_generate.go`가 정본에서 JS를 파생하고 `workflow_parity.go` parity 게이트가 md↔schema↔generated-js의 phase-id/retry/budget/result-type 집합 불일치를 **fail-closed**(exit≠0 + diverging 원소 보고, JS 미기록)로 차단한다. 신규 `pkg/workflow` 패키지: `doctor.go`(capability gate — Primary 프리미티브 agent/schema/phase만 hard-gate, parallel/isolation/budget/model-override는 advisory 비게이팅) + `doctor_version.go`(claude-code >= 2.1.154 핀) + `gate.go`(deterministic Gate — injectable `CommandRunner` seam이 build/test exit-code로 `{verdict, verdict_source:"exit_code", build_exit, test_exit}` 판정, LLM·PhaseBackend 비의존) + `render.go`(dry-run 렌더 + `pkg/promptlayer` 재사용 prompt-manifest 해시) + `fallback.go`(fallback taxonomy 전수 분류 — fail-fast/fail-closed/resumable/explicit, silent 금지) + `drift_gate.go`(release hygiene 종단 — generated-surface drift + `auto check --lore --message` + `auto check --arch --staged` 300줄 차단). `internal/cli/workflow.go`(+`workflow_gate.go`/`workflow_render.go`)가 `auto workflow doctor`/`render`/`gate` 커맨드를 제공하며 `gate`는 workflow JS→Go exit-code bridge다. 4-phase 결정적 실행: Planning → Implementation(worktree 변형은 기존 `pkg/pipeline.WorktreeManager`/`WorktreeSlotCap`=Go 소유, JS는 시퀀싱만) → deterministic Gate(exit-code) → release hygiene. 비-claude(codex/gemini/opencode)는 workflow JS·`--workflow` 라우트·harness-workflow 스킬 **0건으로 회귀**(skill_catalog_policy `claudeOnlySkillSet`으로 claude-scoped 설치) 후 Route A fail-fast 폴백. Outcome Lock satisfied · mandatory 12/12(REQ-001~012) · Must acceptance 14/14(S1-S11·S13·S16 hermetic green, S15 operational evidence) · Completion Debt none. sibling `SPEC-HARNESS-WORKFLOW-GATE-002`(결정적 게이트 엔진)는 Primary 비의존 approved. `go build`/`vet`/`gofmt`/`-race`(pkg/workflow·pkg/content·pkg/adapter·internal/cli) green, golangci-lint 0 issues, file-size 전 .go ≤300.

- **QAMESH-first QA policy and Codex QuestionGate transport** (2026-06-18): `auto qa full` now emits an explicit `qa_policy` payload and text summary that frames QAMESH as the project QA orchestration layer while treating Playwright/browser runners as Journey Pack adapters, not competing QA modes. Generated QA starters and Codex/Claude/Gemini testing guidance now ask for project, execution, environment/origin, credentials, mobile/cloud, or canary authority instead of asking users to choose between QAMESH and Playwright. Codex router, idea/plan/PRD clarification guidance now treats `request_user_input` as the preferred interactive transport whenever it is present in the active tool list, maps the same contract to Codex App Server `tool/requestUserInput`, and falls back to concise plain text only when no Codex question tool is exposed.

- **하네스 업데이트 트랜잭션 롤백 보강** (2026-06-16): `auto update --workspace`가 대상 저장소를 쓰기 전에 설정/플랫폼 preview를 선검증하고, Codex·Claude Code·Antigravity CLI·OpenCode 어댑터의 Update 경로를 공통 트랜잭션 journal 기반으로 묶어 중간 쓰기 실패 시 생성·수정·삭제된 managed surface와 manifest를 원상 복구한다. Claude/Gemini settings 후처리와 OpenCode stale surface prune도 트랜잭션 범위에 포함해 부분 업데이트와 workspace 다중 타깃 실패 전파를 막는다.

- **릴리스타임 크로스서피스 Journey 재생성 + diff 승인 게이트 (SPEC-QAMESH-011)** (2026-06-15): 사용자 호출 단일 명령 `auto qa release-readiness`를 추가해 현재 멀티서피스 코드베이스(web/desktop/mobile)를 분석→starter 템플릿 기반으로 `.autopus/qa/journeys/**` Journey Pack을 재합성→기존 팩과 구조적 필드 단위(added/changed/removed) 비교→정제된 결정적 diff(정확한 카운트·안정 정렬) 출력→**단일 승인 게이트에서 정지**(승인 전 영속/실행 0건, decline은 no-op)→승인 후에만 기존 `runCommand` exec 경로(`qarun.Execute`)로 크로스서피스 실행(present mobile 서피스에 `mobile-scripted` 레인 포함)→exit-code 파생 결정적 verdict + 정제된 `qamesh.evidence.v2` 매니페스트를 발행한다. CI/push/PR 자동트리거가 아닌 릴리스타임 user-invoked 전용(init·scheduler·hook·cron 등록 없음). 신규 패키지 `pkg/qa/regen`(analyze/synthesize/diff/redact/apply/ai-authority)·`pkg/qa/releasereadiness`(orchestrate/dispatch/execute) + CLI `internal/cli/qa_release_readiness.go`. ⭐핵심 설계: 공유 `release.ReleaseLanes()` 카탈로그를 **불변 유지**하고 release-readiness가 자체 레인셋(mobile-scripted 포함)을 합성(REQ-REG-01)·`qarun.Execute` 재사용으로 중복 실행엔진 없음. ⭐`journey.Validate`는 모바일 어댑터 정책에서만 `pass_fail_authority=="ai"`를 거부하므로 web/desktop을 커버하는 **surface-agnostic AI-authority guard**(`qa_regen_ai_authority_forbidden`)를 신설해 어떤 서피스도 AI 판정 권한을 못 갖게 한다(authoring·execution 양쪽 AI 권한 0). ⭐fail-closed: 미존재 도구는 `surface_tool_unavailable`, 부재 서피스는 `surface_absent`로 거짓 통과 금지(`exec.LookPath` 프로빙, GNU `timeout` 래퍼 금지) — reason code는 `pkg/qa/adapter` 와 dispatch 간 published 계약 상수. ⭐보안: diff·재합성 팩·증거 전부 `RedactDiff`+`AssertSafeText`로 정제 후 표시/영속, `apply.go`의 `pack.ID` 경로 traversal 하드닝(reviewer+security 수렴, reject 가드). hermetic 픽스처로 전 invariant 검증(실 디바이스/브라우저 불요). Must AC 15/15(AC-QAMESH11-001..010·012·013·014·015·016) + Should AC-011, 커버리지 89.7/93.4%, 라이브 `--approve` smoke=phase executed·verdict blocked(fail-closed 실증). GUI/mobile 행위 추출(code→flow)은 구조적 필드 diff 범위 밖 Evolution Idea, Appium 탐색(SPEC-QAMESH-009)·클라우드 디바이스랩(SPEC-QAMESH-010)은 reserved sibling. Completion Debt none.

- **모바일 로컬 실행엔진 — Maestro 스크립트 회귀 (SPEC-QAMESH-008)** (2026-06-14): planning-only였던 `mobile-readiness`를 실행 가능한 `mobile-scripted` 레인으로 종결한다(SPEC-QAMESH-006이 "future SPEC"으로 연기했던 실행 절반). `mobile.Assess`가 `ready`이면 `maestro-scripted` 팩을 `selected_adapters`/`selected_journeys`에 유지하고, 주입 가능한 `MobileDeviceRunner` seam(`mobile_lane`/`mobile_exec`/`mobile_runner`/`mobile_device`/`mobile_oracle`/`mobile_artifacts`)을 통해 기존 `runCommand` 단일 엔진으로 프로젝트-로컬 Maestro 플로우를 실행한다(중복 실행엔진 없음). 불투명 `device_ref`→런타임 핸들 해소는 프로세스 env로만 전달하고 published `device_ref`는 불투명 유지, 해소 실패는 `device_ref_unresolved`로 fail-closed; opt-in 관리형 모드는 `exec.LookPath`로 `maestro`/`adb`/`xcrun` 부재 감지(GNU `timeout` 래퍼 금지)·context 타임아웃 경계 부팅/설치를 수행하되 컴퓨티드 sha256이 `app_artifact_digest`와 일치할 때만 설치(불일치=`app_artifact_digest_mismatch`, 설치 0회). 오라클은 exit+선언 assertion으로 결정적 판정하고 `pass_fail_authority=="ai"`를 `validateMaestroPolicy`에서 거부한다(AI 권한 0). 증거는 `qamesh.evidence.v2`(surface `mobile`, 디바이스 메타, app digest, sanitized 로그, screenshot/video quarantine refs)로 발행하되 raw 미디어/서명 URL/raw 디바이스 id/비정제 경로는 `unsafe_mobile_artifact`로 최종 매니페스트 쓰기 전 차단한다. ⭐보안 HIGH 적출·봉합: 디바이스 핸들이 published `sanitized_log` stdout 본문으로 누출(maestro/adb가 `MAESTRO_DEVICE`를 echo, 게이트 regex가 `emulator-5554`·대시 UUID 미포착)→known-value `redactMobileHandle`(패턴 아닌 정확 치환, `WriteFinalManifest` 전)로 4포맷 독립 재검증. `HasAndroidSignals`/`HasIOSSignals` 감지와 리뷰 가능한 `maestro-scripted` 스타터 스캐폴드 추가. 전부 hermetic `fakeMobileDeviceRunner`로 검증(실 디바이스 불요), 실 device-exec seam은 의도적 미커버. AC-QAMESH8-001..010 + edge 012/013(Must 12/12), AC-011(Should). Appium 탐색(SPEC-QAMESH-009)·클라우드 디바이스랩(SPEC-QAMESH-010)은 sequenced sibling 로드맵.

- **SPEC 리뷰 파이프라인 정합성 보강 (SPEC-SPECREV-002)** (2026-06-12): `pkg/spec` provider 체크리스트 파서가 `N/A` 상태를 1급으로 파싱하고(`reChecklist` PASS|FAIL|N/A), self-verify가 빈 reason N/A를 fail-closed로 거부하며, review.md 렌더가 빈 reason N/A를 `reason missing` 마커로 구분 표기한다. inert였던 글로벌 `--loop` 플래그가 spec review 반복 한도 floor(5)로 실효 배선되고(`resolveSpecReviewMaxRevisions`), EARS 미인식 SHALL 라인이 `auto spec validate` warning으로 표면화되며, SPEC Load 실패가 "본문이 비어있습니다" 오진단 대신 원인 보존 메시지로 보고된다. S1~S13 oracle 회귀 잠금.

- **오케스트라·learn·worker 런타임 견고성 하드닝 (SPEC-ORCH-023)** (2026-06-12): cc21 완료 감지의 detector 에러 3중 묵살을 관측 가능(provider 로그 + ctx취소/I/O 구분 + completed=false 강제)으로 전환, learn 스토어 `UpdateReuseCount`/`Prune`의 무잠금 truncate-rewrite를 뮤텍스로 직렬화해 동시 Append 유실 race 봉인(-race oracle), 프로바이더 fast-fail/hook/prompt 패턴을 `ProviderConfig` 오버라이드로 선언화(기본값=기존 하드코딩, default-equivalence oracle), 디베이트/judge 프롬프트 참가자 출력을 라운드별 랜덤 sentinel(`AUTOPUS_PART_<hex>`) 펜스로 감싸 위조 구조 헤더 주입 무력화, reliability 영수증 영속화 실패 store당 1회 경고, unsigned 제어평면 진입 프로세스당 1회 경고(fail-open 정책 불변), surface tracker를 `~/.autopus/surfaces`(uid/0700 검증 + ref 형식 검증 + legacy read-only reap)로 하드닝.

- **플랫폼 어댑터 패리티 보강 (SPEC-PARITY-002)** (2026-06-12): Gemini/Antigravity 생성 규칙에 누락됐던 `deferred-tools`/`project-identity`/`spec-quality` 3종을 추가해 content/rules 14종 패리티를 달성하고, Gemini extended skill의 `.claude/skills/autopus/` 정규 참조를 네이티브 경로로 해소(generate/update 양 경로). `platform:` frontmatter 값을 어댑터 식별자로 정규화(`shell-portability` gemini→antigravity-cli)하고, source−exclusion 양방향 패리티 커버리지 게이트 테스트(`runCoverageGate` + synthetic probe)로 플랫폼 누락의 구조적 재발을 차단.

- **Hook-IPC headless multiprovider completion + pane orphan reaping (SPEC-ORCH-022)** (2026-06-11): Inside Claude Code (CLAUDECODE), `auto spec review` / `orchestra` now collect provider completion through the Stop-hook done-file IPC with no 0/N screen-scrape timeout, finishing SPEC-ORCH-022. The completion detector no longer gates `FileIPCDetector` behind the CC21 monitor feature flag (`resolveCompletionDetector`, `pkg/orchestra/cc21_monitor.go`): when a hook session is active it is selected first as a full-budget completion floor, so `Execute` blocks until the done file appears instead of letting a screen-poll fallback return early and race the deferred session-dir cleanup against the provider's Stop hook (the root cause of the done file landing in an already-removed directory and never being collected). `content/hooks/hook-claude-stop.sh` now writes the done signal unconditionally and guards `chmod`, so an empty assistant message can never suppress completion. Verified end-to-end in a trusted-ancestor cmux e2e (done file collected, session dir alive at Stop time, no screen-scrape fallback). Also adds killed-process pane orphan reaping (`pkg/orchestra/surface_tracker.go`): created cmux/tmux surfaces are tracked per owning orchestrator PID and reaped on the next run only when their owner process is no longer alive, so a SIGKILL/crash leak is cleaned up without ever closing a live concurrent run's panes (PID-liveness gated, with a conservative reap-later degrade on PID reuse).

- **QAMESH one-command full QA entrypoint and coverage/profile diagnostics** (2026-06-01): `auto qa full` now acts as the simple default for full project QA planning, with `--bootstrap` for safe starter generation and `--run` for explicit full gate execution. Meta workspace roots now return scored project candidates instead of writing root QA artifacts. New `auto qa coverage` summarizes latest run/release lane, journey, manifest, setup-gap, and domain-readiness coverage, and `auto qa profile check` compares Journey Pack capability requirements against `standalone/local/ci/prod` test profiles before execution. Domain-readiness starter catalogs now expand from project signals such as browser, auth, desktop, and build surfaces.

- **Interactive-pane orchestration as the default execution path, subscription-first (SPEC-ORCH-021)** (2026-05-30): The three structured orchestra entry points — `orchestra brainstorm`, structured `spec review`, and structured `orchestra run` — now default to the interactive cmux/tmux pane backend, with `-p` headless subprocess demoted to a best-effort fallback (most users run subscription sessions that only authenticate through a logged-in interactive CLI, not the `-p` API path). A real per-provider pane `ExecutionBackend` (`pkg/orchestra/pane_backend.go`, `pane_backend_collect.go`, `pane_fallback.go`) reuses the existing session-ready / `waitForCompletion` (monitor→poll, bounded) / screen-sanitize mechanisms without inventing a new detector, and degrades gracefully when completion hooks are absent. Backend selection is unified behind one shared `paneCapable(term, subprocessMode)` predicate (`pane_capable.go`) consumed by both the legacy `RunOrchestra` guard and `SelectBackend`, fixing F-001 where a non-nil `"plain"` terminal returned a fake pane backend. On pane failure the system attempts `-p` best-effort, records the executed backend in `ProviderResponse.ExecutedBackend`, and surfaces an actionable both-failed error (naming the subscription-vs-API reality and a recovery step) instead of a raw API error. Provider argv correctness: gemini subprocess delivers the prompt as the `--print` value (eliminating the `flag needs an argument: -print` crash) and drops `--print` in interactive pane mode; codex standardizes on `exec --sandbox workspace-write … --output-schema` (no deprecated `--full-auto`); and `review_gate.providers` now includes codex so it is no longer silently dropped from structured review. 16 requirements (REQ-001~016) / 22 Must acceptance scenarios (S1~S20). Verified with `go build ./...`, `go vet`, and `go test ./pkg/orchestra/... ./internal/cli/... ./pkg/config/...`.

- **Claude Opus 4.8 model adoption + ultra effort mapping fix** (2026-05-29): Default premium/ultra tier, cost pricing table, worker routing, and skill docs/templates now use `claude-opus-4-8` (pricing verified at $5/$25 input/output per MTok against official docs). Opus 4.7 pricing is retained as a still-available legacy model. Fixed `resolveUltraMode` hardcoding `opus-4-7` as the only `max`-effort branch, which silently demoted Opus 4.8 to `high` in ultra mode; 4.8 is now in the max branch with a regression test (TC3b). The harness remains `opus`/`sonnet` alias-based, so only model-ID literals changed; `probeOpus47`/`HasOpus47` are preserved as legacy 4.7-availability probes.

- **CodeOps ADK supervised delivery local contract (SPEC-CODEOPS-ADK-001)** (2026-05-25): `auto delivery` now exposes source-owned dry-run planning and strict phase-result envelope validation for PM-owned CodeOps delivery.
  - `pkg/delivery/**` — canonical workflow mode, provider modes, phase order, generated/runtime deny-list checks, dry-run plan schema, and `codeops.phase_result.v1` validation.
  - `internal/cli/delivery.go` + `internal/cli/root.go` — public `auto delivery plan|validate` namespace with JSON output and fail-closed validation errors.
  - Tests passed with `go test ./pkg/delivery ./internal/cli -run 'TestDelivery'`.

- **Codex `/goal` wrapper and native team profile support** (2026-05-25): Codex harness generation now enables `goals` alongside `multi_agent`, exposes `@auto goal` / `$auto goal` as a thin wrapper over Codex `get_goal` / `create_goal` / `update_goal` thread state, and treats `--team` as a native Codex multi-agent Lead/Builder/Guardian profile rather than a Claude Team API compatibility shim. Generated Codex/OpenCode router surfaces, AGENTS guidance, workflow skills, and regression tests now preserve goal handoff semantics without creating separate ADK-persisted goal state.

- **Workspace folder profile policy in memindex/setup guidance (SPEC-WORKSPACE-FOLDER-PROFILE-001)** (2026-05-21): `pkg/memindex` now mirrors workspace folder policy includes/excludes so human-managed project/spec docs remain indexable, `.autopus/inbox/**` remains candidate-only, sanitized learning rows remain projection-only, and generated/runtime/harness paths such as `.autopus/runtime/**`, `.autopus/qa/**`, `.autopus/context/signatures.md`, `.autopus/*-manifest.json`, `.autopus/plugins/**`, `.codex/**`, `.claude/**`, `.gemini/**`, `.opencode/**`, `.agents/plugins/**`, and `config.toml` are skipped with reason codes. Source-owned setup guidance/templates now describe ADK as an execution harness inside a Platform/Desktop-owned workspace folder profile, not the folder identity owner.

- **Visual Brief guidance for `auto idea` and `auto plan`** (2026-05-17): idea and planning guidance now ask agents to explain outcomes with an appropriate visual aid: Mermaid flowcharts for workflows/state, low-fi wireframes for UI/UX, or sequence/data-flow/command-flow sketches for CLI/API/backend work. Planner/spec-writer guidance preserves these visuals as explanation and planning context without promoting visual-only elements into requirements unless they map to the Outcome Lock or Must acceptance.

- **QAMESH readiness projection and repair handoff (SPEC-QAMESH-007)** (2026-05-16): `auto qa readiness` now validates QAMESH run/release indexes and redacted evidence manifests, rejects unsafe raw refs before rendering or prompt handoff, emits a generic `qamesh.readiness_projection.v1` model with lane status taxonomy, setup gaps, audit/evidence/feedback refs, and safe repair actions, and includes a non-Autopus fixture proving the projection is not product-bound.

- **Deep Interview clarification gate for `auto idea` (SPEC-ADK-IDEA-CLARIFY-001)** (2026-05-15): `auto idea` source guidance and Claude/Codex/Gemini/OpenCode templates now require a five-row `Clarification Ledger` before orchestra fan-out, select the single highest-gain unresolved question in interactive mode, keep `--auto` non-blocking with `assumed`/`deferred` rows, record external `deep-interview` provenance as untrusted evidence, and teach `auto plan --from-idea` / spec-writer guidance to map ledger rows into requirements, non-goals, risks, acceptance seeds, open questions, and reviewer focus.

- **QAMESH Journey Pack init flow** (2026-05-15): `auto qa init` now creates a project-local `.autopus/qa/journeys/desktop-gui-explore.yaml` starter when desktop GUI signals are detected, preserves existing packs, validates the generated pack before returning, and documents that generated Journey Packs require human review before execution.

### Changed

- **릴리스 게이트에 보안 워크플로 게이팅** (2026-06-12): `security.yml`(gitleaks+govulncheck)이 release 경로에 게이팅되지 않아 태그 푸시가 보안 검사를 우회할 수 있던 구멍을 `workflow_call` + `needs: [ci, security]`로 봉인.
- **300줄 경계 파일 선제 분할** (2026-06-12): `pkg/worker/compress/tool_pairs.go`(300)→types/prune 2파일, `pkg/qa/evidence/manifest.go`(300)→types 분리, `pkg/adapter/codex/codex_workflow_custom.go`(300)→bodies 분리. 전부 동작 불변(body byte-identical 검증).
- **테스트 커버리지 보강 80.7%→83.9% + CI 임계값 80→83 ratchet** (2026-06-12): 7개 레인 헤르메틱 테스트 보강(+약 1,250 covered 문장, 신규 테스트 70+파일·~7,900줄)으로 domainreadiness 51.7→96.0%, internal/cli 69.2→73.0%, content 84.8→94.8%, setup 85.6→89.7%, orchestra 86.4→88.2% 등 달성. CI 게이트는 실측(83.9%) 아래 0.9%p 여유의 83으로 ratchet. 85 목표 잔여 갭은 hermetic 한계(실 프로세스/PTY/임베디드 FS 에러 경로) — 인터페이스 주입 리팩토링이 선행돼야 하며 별도 작업으로 기록.

- **Project-scoped SQL migration numbering guidance** (2026-05-26): Source-owned database, executor, validator, pipeline, router, and worktree guidance now treats each owning repo's migration directory as a serialized numbering lane. New paired SQL migrations must use 6-digit zero-padded numbers, compute `max(existing)+1` inside the target directory only, keep same-stem up/down pairs, avoid parallel number reservation, and validate affected directories before deploy.

- **QA 대상 리포 자동 해석 및 workspace 문서화** (2026-05-23): `auto qa init`이 기본 실행에서 meta workspace를 감지하면 nested git repo의 Go/Node/Python/Rust/Playwright/desktop 신호를 점수화해 Journey Pack을 제품 리포에 생성합니다. `auto setup`/`auto sync` source guidance와 multi-repo 렌더링은 QA/Journey Pack 대상 리포, `auto qa init --project-dir <repo>` 명령, root `.autopus/qa/**` runtime/generated 경계를 명시하도록 갱신되었습니다.

- **`auto go` QAMESH scope budget** (2026-05-15): go-stage guidance now limits QAMESH execution to affected/fast/smoke lanes and defers full GUI/native/release matrices to explicit `auto qa ...` or `auto canary` runs.
- **QAMESH harness/project-local Journey Pack contract** (2026-05-15): `auto qa plan` and `auto qa explore --dry-run` now expose a `harness_contract` declaring ADK as the harness and project-local `.autopus/qa/journeys/**` as the owner of concrete Journey Packs. Desktop GUI signals now produce non-blocking `project_hints` on fast plans and explicit `setup_gaps` on `gui-explore` requests when the target project has not declared a GUI Journey Pack, while `gui-explore` no longer falls back to generic detected Node/Vitest/Playwright adapters.

### Fixed

- **Windows self-update GitHub API 403 진단 보강** (2026-06-18): `auto update --self`의 릴리스 확인 요청이 명시적인 GitHub REST API 헤더(`User-Agent`, `Accept`, API version)를 보내도록 수정하고, `AUTOPUS_GITHUB_TOKEN`/`GITHUB_TOKEN`/`GH_TOKEN` 기반 인증 요청을 지원한다. GitHub API가 403을 반환하면 응답 본문과 rate-limit reset 정보를 포함해 토큰 설정 후 재시도할 수 있도록 안내한다.

- **worktree gitfile 대상 update 실패** (2026-06-16): 연결 worktree처럼 `.git`이 디렉터리가 아니라 `gitdir:` 파일인 저장소에서 Codex/OpenCode fallback git hook 경로 `.git/hooks/*`를 transaction write/prune 대상으로 잡아 `lstat ... .git/hooks/pre-commit: not a directory`로 `auto update --workspace`가 실패하던 문제를 수정했다. native hook surface(`.codex/hooks.json`, OpenCode plugin)는 계속 갱신하되 root-local fallback git hook은 해당 형태에서 건너뛴다.

- **Claude Code Stop/SessionStart 훅 상대경로 실패** (2026-06-12): 설치된 settings.json의 훅 명령이 상대경로(`.claude/hooks/autopus/hook-claude-stop.sh`)라 훅 spawn cwd가 settings 루트와 다른 서브에이전트/하위 디렉토리 세션에서 매 Stop마다 "No such file or directory"로 실패했다(하루 27회 실측). 생성기(`pkg/content/hooks_completion.go`)가 `"${CLAUDE_PROJECT_DIR:-.}"/` 프리픽스로 앵커하도록 수정하고 회귀 테스트로 고정. 설치된 워크스페이스 복사본 7개는 동일 값으로 로컬 패치됨(다음 `auto update` 재생성과 일치).

- **Gemini SPEC review subprocess timeout backfill** (2026-06-04): Default `agy`/Gemini orchestra provider config now declares a 480s subprocess execution timeout, matching the structured SPEC review budget used by Claude, and in-memory orchestra config migration backfills existing `autopus.yaml` entries where `orchestra.providers.gemini.subprocess.timeout` was missing. This prevents Gemini review runs from falling back to the 240s global `orchestra.timeout_seconds` and failing at exactly 4 minutes.

- **OpenCode shared workflow skill metadata regression** (2026-05-23): `auto update` no longer lets extended `content/skills` entries overwrite `.agents/skills/auto-*` workflow skills, preventing empty `description` frontmatter such as `.agents/skills/auto-setup/SKILL.md` from being emitted and skipped by Codex/OpenCode skill loaders.

- **SPEC review issue #55 migration gap** (2026-05-20): `auto spec review` now applies orchestra provider migrations in-memory before building review providers, so existing configs with legacy Claude `--effort max` adopt `--effort high` and the 480s per-provider timeout during review. Legacy generated `context_max_lines: 500` is treated as unset for review execution so the adaptive 500/1500/3000-line context budget is not accidentally capped back to 500.

- **QAMESH profile capability resolution** (2026-05-16): `auto qa run` / `auto qa explore` now resolve required Journey Pack capabilities from the effective test profile, including project-local `autopus.yaml` profile additions. The local profile also advertises `auth-state`, and QAMESH runtime/cache/gui/feedback artifacts are ignored as generated local evidence.


## [v0.50.10] — 2026-05-20

### Added

- **Official Antigravity CLI harness surface** (2026-05-20): `antigravity-cli` now generates an Antigravity plugin-compatible `.agents/plugins/autopus/` surface with `plugin.json`, skills, rules, agents, and command mappings, while preserving the legacy `.gemini/**` compatibility surface. Generated `GEMINI.md` imports rules from the Antigravity plugin surface, and `.agents/hooks.json` uses official `PreToolUse`/`PostToolUse` hook structure with `run_command` matchers and JSON stdout wrappers.

### Fixed

- **Antigravity AGY worker invocation** (2026-05-20): worker and orchestra defaults now invoke local `agy` through supported non-interactive `--print` mode instead of obsolete Gemini CLI flags such as `--output-format`, `--resume`, and `--model`. Plain-text `agy --print` output is parsed as task results and multiline output is preserved.

## [v0.47.5] — 2026-05-12

### Added

- **Provider transport smoke diagnostics** (2026-05-12): `auto doctor --provider-smoke` now runs a bounded text transport smoke against configured spec-review providers and reports per-provider pass/warn/fail status in both terminal and JSON doctor output. The default `auto doctor` path keeps this probe skipped so routine diagnostics remain non-blocking.

- **Orphaned orchestra provider process detection** (2026-05-12): `auto doctor` now identifies orphaned headless provider commands from orchestra/spec-review runs and includes them in stale runtime process warnings and `--fix` cleanup.

- **Codex bundled browser plugin enablement** (2026-05-12): generated `.codex/config.toml` now enables `browser-use@openai-bundled` by default so frontend verification sessions can load the in-app Browser plugin without manual project setup. Codex validation now warns when that bundled browser plugin toggle is missing.

- **Executable canary CLI baseline (SPEC-CANARY-001)** (2026-05-10): `auto canary` is now a real Cobra subcommand in addition to generated workflow guidance.
  - `internal/cli/{canary,canary_helpers,canary_browser}.go` — dry-run JSON planning, root workspace build targets, `auto test run --scenario version`, `auto doctor`, URL endpoint/page checks, local frontend Playwright smoke, latest-result persistence, and PASS/WARN/FAIL summary output
  - `internal/cli/root.go` — public command registration
  - `internal/cli/canary_test.go` — dry-run JSON contract and fail-closed persistence error regression
  - `--watch` and `--compare` are accepted and reported in result metadata; active loop and commit snapshot diff remain follow-up hardening

- **Delegation Safety Rails (SPEC-ADK-SAFE-RAILS-001)** (2026-05-06): ADK-managed delegation, worktree, provider-timeout, reclaim, and hard-interrupt paths now emit bounded safety evidence instead of silently continuing.
  - `pkg/pipeline/{safety,runtime_safety,worktree_scheduler,reclaim,interrupt}.go` — shared `DegradedEvidence`, `DelegationContext`, depth-cap checks, workflow authenticity preflight, FIFO worktree slot scheduling, reclaim terminal states, and hard-interrupt evidence contracts
  - `pkg/pipeline/{engine,runner}.go` and `internal/cli/pipeline_run.go` — default subagent pipeline authenticity, delegation depth metadata, worktree slot cap decisions, and safety event collection are wired into pipeline execution
  - `pkg/orchestra/{failure_result,pipeline_execute,runner,types}.go` — failed-provider diagnostics now include timeout source, configured/elapsed duration, role, continuation status, failure class, remediation, and redacted previews
  - `pkg/worker/{worktree_safety,loop_audit,loop_exec,loop_runtime,loop_subprocess,pipeline,pipeline_phase}.go`, `pkg/worker/host/resolve.go`, `pkg/worker/parallel/semaphore.go`, and `pkg/worker/security/emergency.go` — required worktree isolation fails closed unless an explicit fallback override reason is present, worktree reclaim emits terminal audit states, and emergency stop records SIGTERM/SIGKILL evidence
  - `content/skills/{agent-pipeline,worktree-isolation}.md` and `templates/**` — source-owned Claude/Codex/Gemini/OpenCode guidance now requires `subagent_dispatch_count`, `degraded_mode`, delegation-depth metadata, worktree slot caps, and reclaim evidence
  - Acceptance coverage exercises depth-cap blocking, workflow authenticity blockers, FIFO slot scheduling, provider timeout evidence, worktree fallback refusal, reclaim sanitization, emergency stop evidence, and source-template safety wording

- **Structured Context Compression (SPEC-CONTEXT-COMPRESS-001)** (2026-05-06): phase handoff compression now preserves long-running agent context as a replayable compaction contract instead of a lossy short summary.
  - `pkg/worker/compress/{summarizer,compressor,events,pruner,tool_pairs,tool_payload}.go` — seven-section summaries (`Goal`, `Constraints`, `Progress`, `Decisions`, `Relevant Files`, `Next Steps`, `Critical Context`), summary continuity metadata, redacted-derived index eligibility, pair-aware tool call/result pruning, safe provider-payload omission, source-ref extraction, and fail-closed context-budget blockers
  - `pkg/pipeline/{engine,events}.go` and `pkg/worker/pipeline.go` — compaction events are recorded before the next phase/model handoff, and context-budget blockers abort instead of silently dropping constraints or decisions
  - `pkg/orchestra/context_compaction.go` — orchestra-side context summarization now reuses the structured compressor contract
  - Acceptance coverage now exercises schema preservation, tool-pair integrity across XML/fenced/JSON-style traces, repeated compaction continuity, redaction of secrets/local paths/provider payloads, event source-ref safety, and pipeline/worker blocker handling

- **FTS5 Decision/Quality Index (SPEC-AUTO-MEM-001)** (2026-05-06): `auto mem` now provides a local, rebuildable quality recall projection over human-managed project docs, SPEC docs, learning JSONL entries, and redacted QAMESH summaries.
  - `pkg/memindex/**` — SQLite FTS5 projection schema, source scanner, deterministic source hashes, redaction/source-root admission guards, QAMESH and learning importers, top-k search, stale/corrupt fail-closed handling, status output, and bounded prompt context rendering
  - `pkg/memindex/driver` — `modernc.org/sqlite` backed FTS5 startup probe before projection writes
  - `internal/cli/mem.go` and `internal/cli/root.go` — public `auto mem rebuild|search|context|status` command namespace with JSON envelopes
  - `internal/cli/init.go` — generated gitignore patterns now include `.autopus/runtime/` so projection files stay runtime-only
  - `pkg/worker/a2a/{heartbeat.go,heartbeat_test.go}` — resolved the stale `@AX:TODO` heartbeat branch-test note by adding non-ok response coverage and removing the annotation during sync lifecycle management

### Fixed

- **Spec review and orchestra provider failure convergence** (2026-05-12): provider-only review failures now stop the spec-review loop without burning all revision retries, empty provider output is classified separately from generic execution errors, pane stdin commands use valid pipeline grouping before `tee`, and provider timeout evidence now reports execution timeout provenance instead of pane startup timeout.

## [v0.44.0] — 2026-05-05

### Added

- **Adaptive SPEC review context limit + Provider Health labeling** (2026-05-04, [SPEC-SPECREV-001](.autopus/specs/SPEC-SPECREV-001/spec.md), issue [#55](https://github.com/Insajin/autopus-adk/issues/55)): multi-provider spec review now scales the citation context budget per SPEC and surfaces provider infrastructure failures as a structured verdict label so operators can distinguish content concerns from timeouts.
  - `pkg/spec/context_limit.go` — new `AdaptiveContextLimit(citedFileCount, ceiling)` mapping (`0~2 → 500`, `3~5 → 1500`, `6+ → 3000`); honors optional `autopus.yaml` ceiling (REQ-CTX-1, REQ-CTX-4)
  - `pkg/spec/metadata.go` — new `ParseReviewContextOverride` reads optional `review_context_lines` SPEC frontmatter override; rejects values ≤0 or >10000 with explicit error (REQ-CTX-2, REQ-CTX-3)
  - `internal/cli/spec_review_context.go` — new `resolveSpecReviewContextLimit` orchestrates cited count → adaptive map → frontmatter override → ceiling cap, emitting `SPEC review context: cited=N applied=M [override=frontmatter] [ceiling=K]` to stderr
  - `pkg/spec/provider_health.go` — new `BuildProviderStatuses`, `RenderProviderHealthSection`, `DegradedLabel`, `ShouldLabelDegraded`; classifies orchestra responses into success/timeout/error and renders `## Provider Health` table (REQ-VERD-1, REQ-VERD-2). Provider Note column is sanitized (control chars stripped, length capped at 200) so committed review.md never embeds raw provider stderr
  - `pkg/spec/merge.go` — new `MergeVerdictsWithDenomMode` adds optional `excludeFailed` denom mode plus AC-VERD-1 fix (dropped providers without supermajority → REVISE not silent PASS); existing `MergeVerdicts` delegates with `excludeFailed=false`
  - `pkg/spec/review_persist.go` — `formatReviewMd` now renders `## Provider Health` after the verdict line and appends `(degraded — N/M providers responded)` when failure ratio ≥ 50% (REQ-VERD-2/4)
  - `pkg/config/schema_spec.go` — new `ExcludeFailedFromDenom bool` yaml field (default false, backward-compatible) (REQ-VERD-3)
  - `internal/cli/spec_review_loop.go` — wires orchestra responses into `BuildProviderStatuses` and switches to denom-mode merge
  - **Behavior change**: `MergeVerdicts` now treats any single REVISE vote as REVISE even when the supermajority math would otherwise pass (AC-VERD-BACKCOMPAT). Existing `TestMergeVerdictsSupermajorityPass` was renamed to `TestMergeVerdicts_AnyReviseWins` to reflect this. External tooling that grepped `**Verdict**: PASS` should be updated to handle the new optional `(degraded — N/M …)` suffix.
  - **Follow-up hardening (2026-05-04)**:
    - `pkg/spec/provider_health.go::sanitizeNote` now uses rune-aware truncation (200 runes + ellipsis) instead of byte slicing, so multi-byte UTF-8 in provider stderr never lands as malformed runes in committed `review.md`.
    - `pkg/spec/metadata.go` split into 3 files (`metadata.go`, `metadata_status.go`, `metadata_frontmatter.go`) — each ≤100 lines, fully out of the 200-line warning band.
    - `internal/cli/spec_review_loop.go` now skips ParseVerdict for failed providers (`TimedOut || ExitCode != 0 || Error != ""`). A failed provider's partial stdout containing `VERDICT: REJECT` no longer triggers the REJECT short-circuit (S-005 hardening).
    - `pkg/orchestra/output_parser.go::ParseReviewer` accepts `PASS | FAIL | N/A` checklist statuses (was `PASS | FAIL`).
- **Checklist Summary section in review.md** (2026-05-04, SPEC-SPECREV-001 follow-up): `formatReviewMd` now renders a `## Checklist Summary` section between `## Provider Health` and `## Findings` whenever `ReviewResult.ProviderStatuses` carries checklist outcomes. The section follows the same column-aligned table pattern as Provider Health.
  - Section structure: heading `## Checklist Summary`, columns `| ID | Status | Provider | Reason |`, terminal totals line `Total: N (PASS: P, FAIL: F, N/A: A)`.
  - `pkg/spec/types.go` — new `ChecklistStatusNA ChecklistStatus = "N/A"` constant; `ChecklistOutcome.Reason` is now required for FAIL **and** N/A (see `content/rules/spec-quality.md` § "N/A Status Guidance" for usage).
  - `pkg/spec/checklist_render.go` [NEW] — `CountChecklistStatuses` (per-status totals) and `RenderChecklistSection` (markdown table, reason sanitization via shared `sanitizeNote`).
  - `internal/cli/spec_review_output.go::printChecklistSummary` now prints `체크리스트 결과: N건 (PASS: P, FAIL: F, N/A: A)` — the N/A count is a new field. Tooling that grepped the previous 2-tuple format must be updated.
  - `internal/cli/spec_self_verify.go` `auto spec self-verify --status` flag now accepts `PASS | FAIL | N/A` (was `PASS | FAIL`); error string is `expected PASS, FAIL, or N/A`.
  - `pkg/spec/selfverify.go::AppendSelfVerifyEntry` accepts `N/A` and writes it verbatim to `.self-verify.log` JSONL entries.
  - **External grep contract**: tools that consume `review.md` should expect either `## Provider Health` immediately followed by `## Findings`, or with `## Checklist Summary` interposed when checklist data is present. Section order is: verdict → Provider Health → Checklist Summary → Findings → Provider Responses.

### Changed

- **Spec review claude provider defaults relaxed for stability** (2026-05-04, issue [#55](https://github.com/Insajin/autopus-adk/issues/55)): default claude orchestra entry now uses `--effort high` (was `max`) and a per-provider subprocess timeout of 480s, exceeding the 240s global timeout to prevent the 4-minute cutoff observed on opus reasoning during multi-provider spec review.
  - `pkg/config/defaults.go` — new `ClaudeOrchestraTimeoutSeconds = 480` constant; claude provider entry sets `Subprocess.Timeout` and switches `--effort` to `high`
  - `pkg/config/defaults_test.go` — regression coverage for claude provider timeout and effort defaults
  - Existing installs are migrated in-memory when `auto spec review` resolves providers; `auto update` can still rewrite `autopus.yaml` to persist the new defaults.

## [v0.43.0] — 2026-05-01

### Changed

- **UX skills now include platform-neutral design-system reasoning** (2026-05-01): `frontend-skill` now performs a compact UX Intelligence pass before UI implementation, and `frontend-verify` / UX agents use the same matrix for visual verification across Claude, Codex, Gemini, and OpenCode surfaces.
  - `content/skills/{frontend-skill,frontend-verify}.md`, `content/agents/{frontend-specialist,ux-validator}.md` — design discovery matrix, UX Intelligence synthesis, viewport matrix, state/accessibility checks, and pattern/style mismatch detection
  - `templates/{codex,gemini}/**/{frontend-skill,frontend-verify,frontend-specialist,ux-validator}*` — regenerated Codex/Gemini surfaces from canonical content
  - `pkg/content/ux_skill_parity_test.go` — regression coverage that the UX Intelligence sections transform for Claude, Codex, Gemini, and OpenCode

- **DESIGN.md starter now participates in init/update** (2026-04-30): `auto init` creates a non-destructive starter `DESIGN.md`, and `auto update` backfills missing `design:` config plus the starter file for older harness installs.
  - `internal/cli/{init.go,update.go,design.go,update_preview.go}` — starter creation/preservation, update backfill, and `--plan` preview visibility
  - `pkg/config/loader.go` — top-level config key detection for safe migration decisions
  - `internal/cli/{init_test.go,update_test.go,update_preview_test.go}`, `pkg/config/defaults_design_test.go` — regression coverage for init, update, disabled design, and dry-run behavior

## [v0.42.1] — 2026-04-30

### Fixed

- **Orchestra degraded run diagnostics are now persisted** (2026-04-30): `auto orchestra brainstorm` and related successful-but-degraded runs now preserve structured failed-provider diagnostics in Markdown artifacts, terminal summaries, and sidecar JSON reports.
  - `internal/cli/{orchestra.go,orchestra_output.go,orchestra_failure_output.go}` — degraded success artifacts now include provider failure class, stderr/stdout previews, timeout provenance, remediation hints, and `degraded-*.json` sidecar reports
  - `pkg/orchestra/{runner.go,pipeline.go,pipeline_execute.go}` — partial provider failures now mark results as degraded and pass through shared failed-provider classification
  - `internal/cli/orchestra_timeout_test.go`, `pkg/orchestra/pipeline_execute_test.go` — regression coverage for degraded Markdown/JSON diagnostics and subprocess pipeline failure preservation

## [v0.42.0] — 2026-04-29

### Added

- **Semantic invariant acceptance gate hardening (SPEC-ACCGATE-002)** (2026-04-29): SPEC generation and implementation guidance now preserve original task semantic invariants through research inventory, oracle acceptance, behavioral tests, validator coverage, and observable subagent pipeline evidence.
  - `content/rules/spec-quality.md`, `content/agents/{spec-writer,tester,validator}.md` — `Q-COMP-05`, `Semantic Invariant Inventory`, oracle acceptance, and structural-only test rejection guidance
  - `content/skills/agent-pipeline.md`, `templates/{claude,codex,gemini}/**`, `pkg/adapter/opencode/opencode_test.go` — `subagent_dispatch_count`, dispatched-role evidence, degraded-mode blocker language, and cross-platform regression coverage
  - `templates/template_test.go` — source-of-truth template assertions for semantic-invariant and workflow-authenticity contracts

- **Project-local DESIGN.md context support (SPEC-DESIGN-001)** (2026-04-29): UI-sensitive ADK workflows can now discover safe local design context, inject compact `## Design Context` evidence into verify/review surfaces, and import external design references only through explicit sanitized generated artifacts.
  - `pkg/design/**`, `internal/cli/design.go` — safe path policy, source-of-truth frontmatter selection, deterministic summary trimming, UI file detection, public-HTTPS URL fetch guard, sanitizer, import artifact writer, and `auto design init/context/import`
  - `internal/cli/{verify.go,orchestra_helpers.go}`, `pkg/adapter/opencode/opencode_workflow_custom.go` — shared UI detector and design-context reporting/injection for `auto verify`, `auto orchestra review`, and OpenCode verify surfaces
  - `content/**`, `templates/**`, `README.md`, `docs/README.ko.md` — platform prompt parity and user docs for optional DESIGN.md, non-blocking skip semantics, read-only review checks, and generated-surface ownership

### Docs

- **Desktop runtime ownership boundary synced to desktop repo (SPEC-DESKTOP-014)** (2026-04-23): packaged `autopus-desktop-runtime` 의 source/build/release provenance 가 `autopus-desktop/runtime-helper/` 로 이동했음을 문서에 반영하고, ADK의 `connect` / `desktop` / `worker` 표면을 harness 또는 compatibility 범위로 재정의
  - `README.md`, `docs/README.ko.md` — desktop runtime source-of-truth 와 ADK compatibility boundary 안내 추가

## [v0.40.51] — 2026-04-25

### Changed

- **Plan workflow now requires complete feature coverage or sibling SPEC decomposition** (2026-04-25): `auto plan` 이 단일 스캐폴드 SPEC으로 멈추지 않도록 completion outcome, Feature Coverage Map, sibling SPEC 세트 분해 계약을 Codex/Claude/Gemini plan surface와 spec-writer/planner agent 지침에 반영
  - `content/agents/{planner.md,spec-writer.md}` — 사용자 요청의 최종 기능 결과를 먼저 정의하고 단일 SPEC 충분성 또는 sibling SPEC 세트를 판단하도록 기획/작성 절차 보강
  - `content/rules/spec-quality.md` — `Q-COMP-04` / `Q-COH-03` 품질 게이트를 추가해 스캐폴드-only SPEC과 vague future work를 self-verify/review 실패로 분류
  - `templates/{codex,gemini,claude}/...` — plan workflow prompt/router/skill surface에 primary/sibling SPEC 추출, Feature Coverage Map, 필수 follow-on SPEC 교차 참조 계약 추가

## [v0.40.45] — 2026-04-23

### Fixed

- **Orchestra multi-provider timeout semantics and config-backed provider resolution hardened** (2026-04-23): pane startup timeout과 실제 실행 timeout을 분리하고, `spec review --multi` 및 subprocess `orchestra run` 경로가 config/CLI timeout 우선순위를 일관되게 사용하도록 정리
  - `internal/cli/{orchestra.go,orchestra_brainstorm.go,orchestra_config.go,orchestra_file_cmds.go,orchestra_helpers.go,spec_review.go,spec_review_runtime.go,orchestra_run.go,orchestra_run_runtime.go}` — command timeout precedence, config-backed provider resolution, subprocess run timeout wiring 추가
  - `pkg/orchestra/{types.go,runner.go,pipeline.go,runner_timeout_config_test.go,pipeline_subprocess_test.go}` — `ExecutionTimeout` 분리, subprocess debater/judge request timeout 전달, 회귀 테스트 보강
  - `internal/cli/{orchestra_provider_timeout_test.go,spec_review_test.go,spec_review_result_ready_test.go,orchestra_run_test.go}` — CLI/config timeout precedence와 review/run wiring regression 추가

- **Debate prompt growth and pane round-2 readiness failures no longer silently drop providers** (2026-04-23): Round 2 rebuttal과 judge prompt에 공통 budget cap을 적용하고, prompt-ready가 되지 않은 pane은 명시적으로 skip/timed-out 처리해 긴 3-provider debate에서 Gemini 등 일부 provider가 조용히 탈락하는 경로를 줄임
  - `pkg/orchestra/{prompt_budget.go,debate.go,crosspolinate.go,interactive_debate_round.go}` — rebuttal/judge prompt budget cap, anonymized subprocess prompt cap, Round 2 prompt-ready guard 추가
  - `pkg/orchestra/{debate_test.go,crosspolinate_test.go,interactive_debate_test.go}` — long-output truncation, judge cap, prompt-ready skip 회귀 테스트 추가

## [v0.40.44] — 2026-04-23

### Added

- **Worker execution lane advertisement surfaced in runtime metadata** (2026-04-23): worker 런타임이 제공 가능한 execution lane 정보를 status/setup 경로에서 기계적으로 노출해 desktop / orchestration consumer가 lane-safe routing 가능 여부를 사전 판정할 수 있도록 확장
  - `pkg/worker/{loop.go,setup/status.go}`, `pkg/worker/a2a/{types.go,server_runtime.go}` — worker config/runtime payload에 `execution_lanes` metadata를 연결하고 server runtime surface에 반영
  - `pkg/worker/{setup/status_test.go,a2a/server_runtime_test.go}` — lane advertisement 회귀 테스트 추가

### Fixed

- **Provider capability fixtures and orchestra timeout expectations aligned with current runtime contracts** (2026-04-23): 최근 orchestration/runtime contract 변경 이후 흔들리던 테스트 기대값을 실제 provider capability / startup timeout 규칙에 맞춰 재정렬
  - `internal/cli/{doctor_json_platforms_test.go,orchestra_provider_timeout_test.go}` — installed CLI capability surface와 provider timeout 회귀 기대값 보정

- **Codex hooks empty categories now serialize as arrays instead of null** (2026-04-23): `.codex/hooks.json` 의 `SessionStart` / `Stop` 빈 카테고리가 `null`로 직렬화되어 Codex CLI가 `invalid type: null, expected a sequence`로 실패하던 문제를 복구
  - `pkg/adapter/codex/{codex_hooks.go,codex_internal_test.go}` — empty hook slice를 `[]`로 내보내는 marshal contract와 회귀 테스트 추가

## [v0.40.43] — 2026-04-23

### Added

- **Claude statusLine 선택 UX** (2026-04-23): 설치/업데이트 시 statusLine 동작을 명시적으로 선택할 수 있도록 CLI surface와 adapter wiring을 확장
  - `internal/cli/{init.go,statusline_mode.go,update.go,update_preview.go,update_preview_test.go,update_statusline_test.go}` — statusLine mode 선택, preview, 회귀 테스트 추가
  - `pkg/adapter/claude/{claude.go,claude_generate.go,claude_settings.go,claude_statusline.go,claude_hooks_test.go}` — 선택된 mode를 실제 Claude settings/statusline surface에 반영
  - `pkg/config/{runtime.go,schema.go}` — runtime 설정 스키마와 adapter 전달 경로 보강

### Fixed

- **기존 사용자 관리 Claude `statusLine` 설정 보존** (2026-04-23): workspace가 이미 사용자 정의 `statusLine`을 가지고 있을 때 하네스 업데이트가 이를 덮어쓰지 않고, Autopus statusline을 쓰는 경우에만 안전하게 갱신하도록 정리
  - `pkg/adapter/claude/{claude.go,claude_files.go,claude_prepare_files.go,claude_settings.go,claude_statusline.go}` — 기존 `statusLine` 감지/보존과 Autopus-managed 갱신 경계 추가
  - `pkg/adapter/claude/claude_hooks_test.go`, `internal/cli/update_statusline_test.go` — preserve/update 분기 회귀 테스트 추가

### Changed

- **Self-hosted generated/runtime artifact ignore 정리** (2026-04-23): self-hosting 과정에서 생기는 backup/context/docs/telemetry, split-mode `.opencode/skills`, demo/internal CLI 하위 `.autopus` 산출물이 작업트리를 오염시키지 않도록 ignore 규칙을 보강
  - `.gitignore` — self-host generated/runtime 경로를 release 이전 기본 ignore set에 포함

## [v0.40.42] — 2026-04-22

### Fixed

- **Spec review non-interactive verdict completion no longer waits for lingering provider processes** (2026-04-22): provider가 `VERDICT:`를 출력한 뒤 tail output 때문에 subprocess가 더 살아 있어도, review flow가 의미 있는 결과를 idle grace 이후 성공으로 수집하고 정리하도록 수정
  - `pkg/orchestra/{types.go,provider_runner.go,provider_result_ready.go,runner_timeout_test.go}` — semantic result-ready pattern/grace contract, non-interactive terminate monitor, regression test 추가
  - `internal/cli/{spec_review.go,spec_review_test.go}` — spec review provider에 `VERDICT:` completion hint를 주입하고 orchestration config 회귀 테스트를 보강

## [v0.40.41] — 2026-04-22

### Added

- **Skill registry + split surface compiler contract (SPEC-SKILLSURFACE-001)** (2026-04-22): 100+ skill / mixed Codex+OpenCode workspace 를 giant shared surface 없이 수용할 수 있도록 canonical catalog, split compiler mode, manifest diff/prune contract 를 도입
  - `pkg/content/{skill_catalog.go,skill_catalog_distribution.go,skill_catalog_policy.go,skill_catalog_test.go,skill_transformer_refs.go}` — canonical skill metadata, bundle/visibility/compile target, dependency extraction, `registered / compiled / visible` state 분리, registry-driven reference rewrite 추가
  - `pkg/config/{schema.go,schema_skill_compiler.go}` — `skills.compiler.mode`, explicit skill, OpenCode/Codex long-tail target validation 추가
  - `pkg/adapter/{manifest_diff.go,manifest_prune.go}`, `internal/cli/update_preview.go`, `internal/cli/update_preview_test.go` — emit/retain/prune preview, checksum diff, stale artifact prune contract 추가
  - `pkg/adapter/codex/*`, `pkg/adapter/opencode/*`, `README.md`, `docs/README.ko.md` — shared/core vs platform-local long-tail ownership split 과 사용자 문서를 split compiler model 에 맞게 정렬

## [v0.40.40] — 2026-04-21

### Added

- **Desktop sidecar contract metadata surfaced for supervision preflight (SPEC-DESKTOP-005)** (2026-04-21): desktop가 retained ADK source of truth를 strict parsing으로 소비할 수 있도록 runtime contract / sidecar protocol metadata를 worker status/session과 shared contract package에 고정
  - `pkg/worker/{setup/status.go,setup/desktop_session.go,sidecarcontract/contract.go}` — `runtime_contract_*`, `sidecar_protocol_*` metadata를 machine-readable bootstrap/session surface에 추가
  - `pkg/worker/host/sidecar.go` — same contract metadata를 sidecar runtime stream에 맞춰 정렬

### Changed

- **Desktop supervision approval correlation and launch parity (SPEC-DESKTOP-005)** (2026-04-21): `auto worker sidecar` 가 desktop launch nonce 플래그를 수용하고, approval request/response 경로가 `approval_id` / `trace_id` correlation metadata를 A2A → worker loop → sidecar NDJSON까지 유지하도록 정리
  - `internal/cli/worker_sidecar.go` — `--desktop-launch-nonce` 플래그를 sidecar entrypoint에 추가해 desktop supervision launch command parity를 맞춤
  - `pkg/worker/a2a/{types.go,server_approval.go,server_approval_test.go}` — approval payload/request-response에 correlation metadata를 추가하고 A2A round-trip 회귀 테스트를 보강
  - `pkg/worker/{loop.go,loop_runtime.go,loop_task.go,loop_approval_state.go,loop_approval_test.go,host_observer.go}` — pending approval state를 task별로 보존하고 response/resolution/task cleanup 시 correlation metadata를 유지
  - `pkg/worker/host/{sidecar.go,resolve_test.go}` — sidecar NDJSON approval payload에 `approval_id` / `trace_id`를 노출하고 unknown host event를 explicit degraded signal로 처리

### Fixed

- **Codex auto skill duplicate surface cleanup** (2026-04-21): generated plugin/local skill surface가 동시에 남을 때 중복 라우팅 흔적과 README drift가 발생하던 문제를 정리
  - `pkg/adapter/codex/{codex.go,codex_standard_skills.go,codex_surface_cleanup.go,codex_surface_test.go,codex_update_test.go}` — duplicate skill cleanup 경로와 회귀 테스트를 추가
  - `pkg/adapter/integration_test.go`, `README.md`, `docs/README.ko.md` — surface cleanup 동작과 사용자 문서를 현재 Codex contract에 맞춤

### Docs

- **SPEC-SETUP-003 planning/status sync** (2026-04-21): preview-first setup/connect truth-sync 이후 SPEC 문서를 구현 상태 기준으로 갱신
  - `.autopus/specs/SPEC-SETUP-003/{spec,plan,acceptance}.md` — 구현/검증 상태와 follow-up 범위를 실제 완료 기준에 맞춰 정리

## [v0.40.39] — 2026-04-21

### Added

- **Preview-first bootstrap planning and connect truth-sync (SPEC-SETUP-003)** (2026-04-21): `auto update` 와 `auto setup generate/update` 가 no-write preview를 먼저 계산하고, `auto connect` 는 deterministic verify surface와 실제 구현 기준 안내 문구를 제공하도록 정리
  - `internal/cli/{setup.go,preview_output.go,setup_preview.go,setup_preview_test.go,update.go,update_preview.go,update_config_preview.go,update_preview_test.go}` — `--plan`/`--preview`/`--dry-run` preview 출력, tracked/generated/runtime/config 분류, no-write regression test 추가
  - `pkg/config/loader.go`, `pkg/setup/{engine.go,engine_docs.go,meta.go,scenarios.go,sigmap_integration.go,types.go,change_plan.go,change_apply.go,change_plan_test.go,workspace_hints.go,sigmap_helpers_test.go}` — reusable change-plan 모델, stale preview revalidation, repo-aware workspace hint, preview/apply shared helpers 추가
  - `internal/cli/{connect.go,connect_status.go,connect_truth_sync_test.go}`, `README.md`, `docs/README.ko.md` — `auto connect status` surface와 onboarding wording truth-sync, README/help drift regression test 추가

- **Stable machine-readable CLI JSON envelopes (SPEC-CLIJSON-001)** (2026-04-21): phase-1 상태/진단 명령과 기존 JSON surface를 공통 envelope로 정렬해 CI, desktop, agent chaining이 text scraping 없이 재사용할 수 있도록 정리
  - `internal/cli/{output_json.go,doctor_json.go,doctor_json_platforms.go,doctor_json_checks.go,status_json.go,setup_json.go,telemetry_json.go,test_json.go,worker_status_json.go}` — shared envelope writer, redaction/home-path masking, command별 payload/check helper 추가
  - `internal/cli/{doctor.go,status.go,setup.go,telemetry.go,permission.go,test.go,worker_commands.go,root.go}` — `--json`/`--format json` rollout, warn/error payload contract, fatal JSON path cleanup 반영
  - `pkg/connect/headless_event.go`, `internal/cli/json_contract_test.go` — `connect --headless` NDJSON compatibility metadata와 contract/redaction/fatal-path regression test 추가

- **Multi-repo workspace detection and cross-repo setup rendering (SPEC-SETUP-002)** (2026-04-21): `auto setup` / `auto arch` 가 root+nested repo topology를 1급 모델로 인식하고 repo boundary/workflow/scenario 문서를 생성하도록 확장
  - `pkg/setup/{multirepo.go,multirepo_deps.go,multirepo_types.go,multirepo_render.go,scanner.go,types.go}` — `MultiRepoInfo` 모델, immediate-child repo discovery, Go/NPM cross-repo dependency mapping, aggregate scan wiring 추가
  - `pkg/setup/{renderer_arch.go,renderer_docs.go,scenarios.go}` — Workspace / Development Workflow / Repository Boundaries 섹션과 path-aware language-specific cross-repo scenario 생성 추가
  - `pkg/setup/{multirepo_test.go,multirepo_render_test.go,multirepo_scenarios_test.go}` — topology, rendering, scenario synthesis acceptance 회귀 테스트 추가

- **Desktop bootstrap session surface for the approval-only shell (SPEC-DESKTOP-004)** (2026-04-21): desktop handoff/session restore가 ADK source of truth를 재사용하도록 `auto worker session` 과 status readiness contract를 추가
  - `internal/cli/{worker_commands.go,worker_session.go}` — `worker session` command 등록, desktop-oriented machine-readable help/command boundary 정리
  - `pkg/worker/setup/{status.go,desktop_session.go}` — `credential_backend`, `secure_storage_ready`, `desktop_session_ready` 를 `worker status --json` 에 노출하고 fail-closed desktop session payload 구현
  - `pkg/worker/setup/desktop_session_test.go` — desktop bootstrap readiness/reason contract 회귀 테스트 추가

- **Orchestra reliability receipts, failure bundles, and run correlation (SPEC-ORCH-020)** (2026-04-21): pane/hook/detach orchestration에 provider preflight, prompt transport, collection receipt와 compact failure bundle contract를 추가
  - `pkg/orchestra/reliability_{receipt,preflight,bundle}.go`, `pkg/orchestra/{types.go,detach.go,job.go}` — schema v1, `run_id`, fallback mode, sanitized artifact, runtime artifact root/retention wiring 추가
  - `pkg/orchestra/{interactive_debate.go,interactive_debate_helpers.go,interactive_debate_round.go,interactive_collect.go}` — hook timeout structured event, partial collection receipt, degraded summary, remediation hint 연결
  - `internal/cli/{orchestra.go,orchestra_output.go}` — degraded 상태, run id, artifact dir를 CLI 결과물에 표면화
  - `pkg/orchestra/reliability_{core,collection}_test.go` — secret redaction, preflight receipt, retention, timeout bundle 회귀 테스트 추가

### Fixed

- **Worker status/session credential source mismatch** (2026-04-21): secure storage backend와 auth validity 판정이 command마다 달라질 수 있던 문제를 단일 credential snapshot 경로로 정리
  - `pkg/worker/setup/{credential_snapshot.go,credentials_store.go}` — keychain/encrypted/plaintext credential payload를 하나의 snapshot loader로 통합
  - `pkg/worker/setup/{auth_test.go,status_coverage_test.go,desktop_session_test.go}` — status/session이 같은 credential backend와 readiness를 반환하는지 회귀 검증 추가

- **pkg/orchestra full-suite timeout regression** (2026-04-21): reliability work 이후에도 `go test -timeout 120s ./pkg/orchestra`가 다시 통과하도록 interactive polling/backoff와 fixture sequencing을 결정적으로 정리
  - `pkg/orchestra/{completion_poll.go,interactive.go,interactive_collect.go,interactive_surface.go,surface_manager.go,interactive_debate_round.go}` — polling interval, retry/backoff, submit/empty-output wait를 짧고 결정적으로 조정
  - `pkg/orchestra/{pane_mock_test.go,interactive_pane_debate_test.go,interactive_surface_test.go,interactive_surface_round_test.go,interactive_edge_test.go,surface_manager_test.go,warm_pool_test.go,cc21_monitor_test.go}` — pane-aware mock sequencing과 stale/idle recovery fixture를 정리하고 runtime expectation을 현재 detector contract에 맞춤

## [v0.40.38] — 2026-04-21

### Added

- **Worker shared host assembly and machine-readable sidecar entrypoint (SPEC-DESKTOP-003)** (2026-04-20): desktop supervision이 launch logic를 fork하지 않도록 shared host runtime과 NDJSON sidecar surface를 추가
  - `internal/cli/worker_sidecar.go`, `internal/cli/worker_commands.go` — `auto worker sidecar` command 등록 및 machine-oriented help surface 추가
  - `pkg/worker/host/{errors.go,resolve.go,runtime.go,sidecar.go,resolve_test.go}` — typed host input, resolved runtime config, structured host errors, sidecar protocol/event contract 구현
  - `pkg/worker/host_observer.go`, `pkg/worker/{loop.go,loop_runtime.go,loop_task.go,loop_subprocess.go,loop_lifecycle.go,loop_approval_test.go}` — runtime/task/approval observer bridge와 degraded/progress/completion signal wiring 추가

### Changed

- **Legacy worker start path now reuses the shared host runtime** (2026-04-20): `auto worker start`가 duplicated assembly를 버리고 compatibility shim으로 축소되고, explicit credentials path override가 desktop sidecar용 실제 auth source로 동작
  - `internal/cli/worker_start.go`, `internal/cli/worker_start_test.go` — start command를 shared runtime shim으로 정리하고 기존 local resolver 테스트를 host package로 이동
  - `pkg/worker/setup/{apikey.go,status.go,credentials_override.go,apikey_coverage_test.go}` — `LoadAPIKeyFromPath`, `LoadAuthTokenFromPath`, path-backed CredentialStore, custom credentials path coverage 추가

### Fixed

- **Worker setup device auth now honors deadline boundaries** (2026-04-21): Windows에서 `auto worker setup` 승인 직후 polling deadline 경계에 걸리면 stale token 요청이 한 번 더 나가 backend의 `expired_token`을 그대로 surfacing하던 문제를 수정
  - `pkg/worker/setup/auth.go` — poll interval 대기를 context-aware `select`로 바꾸고 token exchange HTTP request에 context를 전달해 deadline 이후 추가 poll과 hanging request를 차단
  - `pkg/worker/setup/auth_device_test.go`, `pkg/worker/setup/auth_deadline_test.go` — 새 context-aware exchange signature 반영 및 deadline 경계 회귀 테스트 2건 추가

## [v0.40.37] — 2026-04-19

### Changed

- **Residual golangci-lint cleanup sweep across ADK** (2026-04-19): 남아 있던 `staticcheck`/`ineffassign`/test-style 경고를 일괄 정리해 현재 `golangci-lint run --max-issues-per-linter=0 --max-same-issues=0` 기준 0 issue 상태로 수렴
  - `.golangci.yml`, `internal/cli/**`, `pkg/orchestra/**`, `pkg/setup/**`, `pkg/worker/**` — 빈 에러 브랜치, 비효율 할당, 루프/append 패턴, 테스트 fixture/헬퍼 표현을 정리
  - `pkg/adapter/opencode/opencode_router_contract.go`, `pkg/content/agent_transformer_condense.go`, `internal/cli/issue_auto.go` — 더 이상 쓰이지 않는 보조 경로와 dead code를 제거
  - 광범위한 테스트/헬퍼 파일에서 lint 친화적 표현으로 정렬해 release gate를 통과하도록 회귀 범위를 동기화

## [v0.40.36] — 2026-04-19

### Fixed

- **Install bootstrap now separates install from init** (2026-04-19): installer가 `auto init`/`auto update`를 자동 실행하지 않고, 필수 도구만 점검한 뒤 `auto init`, `auto update --self`, `auto update`의 역할을 명시적으로 안내하도록 정리
  - `install.sh`, `install.ps1` — post-install 단계에서 required dependency만 자동 설치하고, 자동 project init/update 분기 제거
  - `internal/cli/doctor.go`, `internal/cli/doctor_fix.go` — `--required-only` 플래그와 required dependency filter 추가
  - `pkg/detect/detect.go` — `gh`를 필수 도구로 승격하고 Gemini CLI npm 패키지를 `@google/gemini-cli`로 정정
  - `README.md`, `docs/README.ko.md`, `internal/cli/doctor_fix_runtime_test.go`, `internal/cli/doctor_fix_test.go`, `pkg/detect/fullmode_deps_test.go` — 설치 가이드/회귀 테스트 동기화 및 테스트 파일 분할로 300-line limit 유지

- **E2E scenario runner backend submodule path correction** (2026-04-19): Backend build 시나리오가 `Autopus/`를 cwd로 잡아 존재하지 않는 `cmd/server` 경로를 참조하던 문제를 실제 backend 소스 경로인 `Autopus/backend/`로 정렬
  - `pkg/e2e/build.go`, `pkg/e2e/build_test.go` — default submodule map을 canary H2/H3 build cwd와 일치시키고 회귀 테스트 추가

- **Permission detection tests now use injected process-tree stubs** (2026-04-19): `--dangerously-skip-permissions`가 걸린 세션에서 `pkg/detect` 테스트가 실제 부모 프로세스 트리에 오염되던 문제를 제거
  - `pkg/detect/permission.go`, `pkg/detect/permission_test.go` — `checkProcessTreeFn` 주입 지점과 결정적 stub helper 추가

- **CC21 monitor runtime flake removed via Claude version injection hook** (2026-04-19): `claude --version` subprocess timeout으로 인해 `TestResolveCC21MonitorRuntime_Enabled`가 간헐적으로 실패하던 문제를 테스트 전용 version injector로 제거
  - `pkg/platform/claude.go`, `internal/cli/orchestra_cc21_test.go` — `claudeVersionFn`/`SetClaudeVersionForTest` 추가 및 monitor runtime 회귀 테스트 보강

## [v0.40.35] — 2026-04-19

### Fixed

- **Release workflow bootstrap ordering** (2026-04-19): `goreleaser-action@v7`가 `cosign`이 PATH에 있을 때 GoReleaser 다운로드 자체의 sigstore bundle을 추가 검증하는데, upstream bundle 검증 실패로 `v0.40.34` release workflow가 즉시 중단되던 문제를 우회
  - `.github/workflows/release.yaml` — action을 `install-only`로 먼저 실행해 checksum 검증만 수행하고, 이후 `cosign` 설치와 `goreleaser release --clean` 직접 실행으로 실제 checksum signing 단계만 유지하도록 순서 조정

## [v0.40.34] — 2026-04-19

### Added

- **Test Profile 기반 시나리오 요구조건 스킵** (2026-04-19): `auto test run`에 `--profile` capability 집합을 도입해 시나리오의 `Requires` 조건이 충족되지 않으면 FAIL 대신 SKIP으로 처리
  - `internal/cli/test.go`, `internal/cli/test_profile_test.go` — `--profile` 플래그, SKIP 집계, JSON 출력 회귀 테스트 추가
  - `pkg/config/test_profiles.go`, `pkg/config/test_profiles_test.go`, `pkg/config/schema.go` — profile별 capability 기본값 및 `autopus.yaml` 확장
  - `pkg/e2e/requires.go`, `pkg/e2e/scenario.go`, `pkg/e2e/scenario_requires_test.go` — `Requires` 파싱 및 capability mismatch 계산 로직 추가
  - `templates/shared/scenarios-*.md.tmpl` — 시나리오 템플릿에 `Requires` 필드 추가

### Fixed

- **SPEC review finding status breakdown summary** (2026-04-19): `auto spec review` 최종 요약이 단순 unique count 대신 `open/resolved/out_of_scope` 상태별 집계를 함께 출력하도록 개선해 운영자가 `review-findings.json`을 별도로 집계하지 않아도 열린 finding 수를 바로 확인 가능
  - `pkg/spec/findings_summary.go`, `pkg/spec/findings_test.go` — `ReviewFinding` slice를 상태별로 집계하는 `SummarizeFindings` / `FindingsSummary.Format` 로직과 회귀 테스트 추가
  - `internal/cli/spec_review.go` — 최종 CLI 요약을 status breakdown 표면으로 교체

- **Pipeline worktree remove canonical path fallback** (2026-04-19): macOS의 `/tmp` → `/private/tmp`, `/var` → `/private/var` symlink 환경에서 `git worktree remove`가 symlink path를 실제 worktree로 인식하지 못해 release gate의 `pkg/pipeline` 테스트가 실패하던 문제를 수정
  - `pkg/pipeline/worktree.go` — remove 시 원본 path와 canonical path를 순차 재시도하고, 실제 git worktree가 아닌 fallback 디렉터리는 안전하게 `os.RemoveAll`로 정리하도록 보강
  - `pkg/pipeline/worktree_internal_test.go` — symlink alias로 생성한 실제 worktree를 remove 하는 회귀 테스트 추가

- **SPEC 리뷰 체크리스트 런타임 주입 및 self-verify 기록 경로 복구 (SPEC-SPECWR-002)** (2026-04-19): `auto spec review`가 `content/rules/spec-quality.md`를 실제 런타임 프롬프트에 주입하고, `CHECKLIST:` 응답을 구조화 파싱하며, `auto spec self-verify`로 결정적 JSONL 기록을 남길 수 있도록 동기화.
  - `pkg/spec/checklist.go`, `pkg/spec/prompt.go` — embed 우선 + 디스크 fallback 체크리스트 로더, `## Quality Checklist` 주입, checklist response examples 추가
  - `pkg/spec/types.go`, `pkg/spec/reviewer.go`, `internal/cli/spec_review_loop.go`, `internal/cli/spec_review.go` — `ChecklistOutcome` 타입, `CHECKLIST:` 파싱, provider outcome 집계, 최종 요약 출력 연결
  - `pkg/spec/selfverify.go`, `internal/cli/spec.go`, `internal/cli/spec_self_verify.go`, `.gitignore` — `auto spec self-verify` 서브커맨드, 100라인 retention, `.self-verify.log` ignore 규칙 추가
  - `pkg/spec/checklist_test.go`, `pkg/spec/reviewer_checklist_test.go`, `pkg/spec/selfverify_test.go`, `internal/cli/spec_review_checklist_test.go`, `internal/cli/spec_self_verify_test.go` — checklist injection/parser/CLI/self-verify 회귀 테스트 추가

- **SPEC 리뷰 수렴성 재구축 (SPEC-REVFIX-001)** (2026-04-19): `auto spec review --multi`가 대부분의 SPEC에서 PASS에 도달하지 못하고 REVISE 루프를 소진한 뒤 circuit breaker로 종료되던 7개 복합 결함 제거.
  - **REQ-01 Supermajority verdict**: `MergeVerdicts`가 `spec.review_gate.verdict_threshold`(기본 0.67) 기준 supermajority를 적용. 1 REJECT 단독 override는 유지(security gate). `pkg/spec/reviewer.go`
  - **REQ-02 Revision 루프 내 재로드**: `runSpecReview`가 iteration마다 `spec.Load(specDir)` 재호출. 외부 수정이 다음 round에 반영됨. `internal/cli/spec_review_loop.go`
  - **REQ-03 다중 문서 주입**: `BuildReviewPrompt`가 plan.md / research.md / acceptance.md 본문을 별도 섹션으로 주입. `doc_context_max_lines`(기본 200)로 trim. `pkg/spec/prompt.go`
  - **REQ-04 Verdict 판정 기준 명문화**: 프롬프트에 `critical==0 && security==0 && major<=2 → PASS` 규칙 포함. `pass_criteria` override 지원.
  - **REQ-05 FINDING 포맷 강제 + empty RawContent guard**: structured FINDING few-shot(positive 2 + negative 1), `doc.RawContent == ""` 시 early error.
  - **REQ-06 DeduplicateFindings / MergeSupermajority 프로덕션 통합**: REVCONV-001이 구현했으나 호출되지 않던 dead code를 `runSpecReview` 경로에 연결. critical/security는 supermajority 우회.
  - **REQ-07 Finding ID 전역 유니크**: `parseDiscoverFindings`가 ID 비어있게 두고 `DeduplicateFindings`가 global `F-001..` 재발급. `ApplyScopeLock` 오동작 해결.
  - 신규: `pkg/spec/merge.go`, `pkg/config/schema_spec.go`, `internal/cli/spec_review_loop.go`, `pkg/spec/prompt_test.go`, `pkg/spec/reviewer_supermajority_test.go`, `internal/cli/spec_review_scaffold_test.go`
  - `autopus.yaml` 샘플에 `verdict_threshold`, `pass_criteria`, `doc_context_max_lines` 주석 예시 추가

### Changed

- **Claude Code 2.1 CC21 경로 연결 및 precedence 정렬 (SPEC-CC21-001)** (2026-04-19): effort frontmatter, TaskCreated hook, initial prompt 검사, monitor 기반 완료 감지를 source-of-truth와 CLI/runtime 경로에 연결
  - `internal/cli/effort*.go`, `internal/cli/check_initial_prompt*.go`, `internal/cli/orchestra_cc21.go`, `internal/cli/check_cc21.go`, `internal/cli/cc21_runtime.go` — CC21 전역 플래그, runtime precedence, check 명령, orchestra wiring 추가
  - `pkg/orchestra/cc21_monitor.go`, `pkg/platform/claude.go`, `pkg/platform/claude_test*.go` — Claude Code 2.1 capability 감지와 monitor contract 연결
  - `content/hooks/task-created-validate.sh`, `content/hooks/README.md`, `pkg/content/hooks.go`, `pkg/adapter/claude/claude_task_created_test.go` — TaskCreated generated default와 runtime override precedence 정렬
  - `content/skills/monitor-patterns.md`, `content/embed.go`, `content/skills/adaptive-quality.md`, `content/skills/idea.md`, `content/skills/agent-pipeline.md` — CC21 monitor/effort 규칙과 문서 표면 동기화
  - `pkg/adapter/claude/claude_generate.go`, `pkg/adapter/claude/claude_prepare_files.go`, `pkg/adapter/claude/claude_update.go` — Claude adapter 파일 생성/업데이트 경로를 300줄 제한에 맞게 분리 정리

- **Claude deferred-tools 선로딩 규칙 추가** (2026-04-18): Claude Code의 지연 로드 도구(`AskUserQuestion`, `TaskCreate`, `TeamCreate` 등)가 스키마 미로드 상태로 호출될 때 생기던 평문 downgrade / validation error를 줄이기 위해 전역 규칙을 추가
  - `content/rules/deferred-tools.md` — `/auto triage`, Gate 1 승인, `--team` 진입 시 `ToolSearch`로 스키마를 먼저 로드하도록 trigger point 규칙 추가

- **Claude Code Agent Teams + mode 파라미터 동기화** (2026-04-18): Agent Teams 공식 스펙(https://code.claude.com/docs/en/agent-teams)을 반영하고, Agent() 호출 파라미터 이름을 `permissionMode` → `mode` 로 통일. 플랫폼별 `--team` 플래그 동작 명시.
  - `content/skills/agent-pipeline.md`, `content/skills/worktree-isolation.md` — 본문 `Agent(... permissionMode=)` 10건 → `mode=`
  - `templates/codex/skills/agent-pipeline.md.tmpl`, `templates/codex/skills/worktree-isolation.md.tmpl`, `templates/gemini/skills/agent-pipeline/SKILL.md.tmpl`, `templates/gemini/skills/worktree-isolation/SKILL.md.tmpl` — 동일 변경 (각 4-6건)
  - `content/skills/agent-teams.md` — Prerequisites 섹션(v2.1.32+ 버전 요구) + Team Constraints 섹션(nested 금지, leader-only cleanup, 3-5명 권장, 영속 경로) 신설. Team Creation Pattern의 `Teammate()` → `Agent(team_name=..., name=...)` 공식 문법으로 교정
  - `templates/claude/commands/auto-router.md.tmpl` — Route B preflight 2단계(버전 + 환경변수) 추가, 에러 메시지 개선
  - `templates/codex/skills/agent-teams.md.tmpl` — 상단 ⚠️ Platform Note: Claude Code 전용 명시, Codex는 `spawn_agent` fallback
  - `templates/gemini/commands/auto-router.md.tmpl`, `templates/gemini/skills/agent-teams/SKILL.md.tmpl` — Platform Note 배너 + Route B 비활성화 + `--team` 경고 후 Route A fallback, 스테일 "Gemini CLI Agent Teams" 참조 제거
  - **Subagent frontmatter `permissionMode:` 필드는 공식 스펙이므로 그대로 유지** (Agent() 호출 파라미터와 별개 레이어)

### Docs

- **spec-writer 자체 품질 체크리스트 도입 문서 동기화 (SPEC-SPECWR-001)** (2026-04-19): `content/rules/spec-quality.md` 신규 체크리스트, `content/skills/spec-review.md`의 pre-review self-check, `content/agents/spec-writer.md`의 자체 검증 루프를 실제 산출물 기준으로 정렬하고 SPEC 문서를 completed 상태로 동기화
  - `content/rules/spec-quality.md`, `content/skills/spec-review.md`, `content/agents/spec-writer.md` — 체크리스트, pre-review self-check, 자체 검증 루프 source-of-truth 반영
  - `.autopus/specs/SPEC-SPECWR-001/{spec,plan,acceptance,research}.md` — completed 상태 동기화, validator/review 기준 정렬
  - 후속 보강: `research.md`의 `Self-Verify Summary` 관측 지점과 구조화된 `Open Issues` 스키마를 문서 규약으로 추가해 reviewer가 retry 경로를 문서 안에서 추적 가능하도록 보강

- **`/auto go --team` Route B 실행 절차 공백 수정** (2026-04-18): `--team` 플래그로 실행해도 core 4명 중 lead 1명만 spawn되어 멀티에이전트 협업이 작동하지 않던 문제를 수정. 실측 증거: `~/.claude/teams/spec-waitux-001/config.json` 의 members 배열에 team-lead 1명만 등록. 근본 원인: Route B 문서가 TeamCreate 호출 주체·시점, ToolSearch 선행 의존성, 4명 병렬 spawn 규칙, members 검증 게이트, phase별 SendMessage 디스패치를 명시하지 않음
  - `templates/claude/commands/auto-router.md.tmpl` — Route B에 **Team Orchestration Procedure (B1~B5)** 신설: ToolSearch → TeamCreate → 4명 병렬 Agent() spawn → `.members | length == 4` HARD GATE → SendMessage 오케스트레이션
  - `content/skills/agent-teams.md` — Lead 책임에서 "Creates the team" 문구 제거(teammates MUST NOT call TeamCreate), Team Creation Pattern을 top-level session 주체 + ToolSearch 선행 + verification gate 구조로 재작성
  - `templates/codex/skills/agent-teams.md.tmpl`, `templates/gemini/skills/agent-teams/SKILL.md.tmpl` — 플랫폼 비지원 명시를 유지한 채 Lead 문구와 코드 주석 정정

- **Route B 실측 smoke-test 기반 절차 정정** (2026-04-18): 1차 패치의 Route B 절차를 실제 `TeamCreate` + 3명 `Agent()` 호출로 smoke-test 한 결과, 공식 Claude Code Agent Teams API와 어긋난 4가지 세부 사항을 확인하고 정정. 실측 증거: `~/.claude/teams/team-probe-001/config.json` members=4 (team-lead + builder-1 + tester + guardian) 정상 생성 후 `SendMessage({type:"shutdown_request"})` ×3 + `TeamDelete()` 사이클 E2E 통과
  - **TeamCreate 파라미터명 정정**: `TeamCreate(name=...)` → `TeamCreate(team_name=..., agent_type="planner")` — 공식 스키마 파라미터는 `team_name` (기존 `name`은 오타)
  - **Lead 자동 등록 명시**: `TeamCreate`는 호출 시점에 메인 세션을 자동으로 `name: "team-lead"`, `agentType: <agent_type>`로 등록한다. Step B3은 **lead 제외 3명만 spawn**(builder-1 / tester / guardian)으로 축소 — lead Agent() 중복 spawn 방지
  - **SendMessage 주소 교정**: phase 오케스트레이션 매핑 표의 `to="lead"` → `to="team-lead"`. Phase 1 Planning은 메인 세션이 직접 담당하므로 SendMessage 불필요
  - **Step B6: Teardown 신설**: 구조화된 `{type:"shutdown_request"}`는 **per-teammate** 발송 필수 (broadcast `to:"*"`는 plain text 전용, structured payload rejected). `TeamDelete()`는 active members 남아 있으면 실패하므로 shutdown_request 후 `sleep 8` 대기 필수
  - 수정 파일: `templates/claude/commands/auto-router.md.tmpl`, `content/skills/agent-teams.md`, `templates/codex/skills/agent-teams.md.tmpl`, `templates/gemini/skills/agent-teams/SKILL.md.tmpl`

### Chore

- **SPEC review 산출물 ignore 정리** (2026-04-19): review 실행이 생성하는 `review.md`, `review-findings.json`을 runtime artifact로 간주하고 git 추적 대상에서 제외
  - `.gitignore` — `**/.autopus/specs/**/review.md`, `**/.autopus/specs/**/review-findings.json` 패턴 추가

## [v0.40.32] — 2026-04-17

### Changed

- **Claude Opus 4.7 Alignment**: 2026-04-16 Anthropic Opus 4.7 공식 출시에 맞춰 하네스 모델 ID/가격을 전면 동기화. 기존 cost estimator가 Opus 가격을 $15/$75로 과대 산정하던 오류도 함께 보정
  - `pkg/cost/pricing.go` — 모델 ID를 `claude-opus-4-7` / `claude-sonnet-4-6` / `claude-haiku-4-5`로 버전 명시, Opus 입력/출력 가격을 공식가 $5/$25로, Haiku를 $1/$5로 정정 (이전 $15/$75, $0.80/$4)
  - `pkg/cost/pricing_test.go`, `pkg/cost/estimator_test.go`, `pkg/cost/estimator_extra_test.go` — 모델명 assertion과 실제 달러 기대값(ultra/executor 4k 토큰 시 $0.04 등) 재계산
  - `pkg/worker/routing/config.go`, `pkg/worker/routing/{config,router}_test.go`, `pkg/worker/routing_integration_test.go` — Complex tier를 `claude-opus-4-7`로 승격
  - `pkg/config/defaults.go`, `autopus.yaml`, `configs/autopus.yaml` — Full 모드 기본 router tier `premium` / `ultra` 를 Opus 4.7로 갱신
  - `demo/simulate-claude.sh` — welcome banner 모델 표기를 `claude-opus-4-7`로 교체

### Docs

- **using-autopus Router Tier 예시 동기화**: `auto init` 이 생성하는 `configs/autopus.yaml` 기본값이 이미 `claude-opus-4-7` / `claude-sonnet-4-6` 버전 명시형인데, 가이드 문서의 예시 블록은 unversioned alias 로 남아 있어 사용자 혼란을 유발하던 불일치 제거
  - `content/skills/using-autopus.md`, `templates/codex/skills/using-autopus.md.tmpl`, `templates/gemini/skills/using-autopus/SKILL.md.tmpl` — router.tiers 예시 블록 통일

## [v0.40.29] — 2026-04-16

### Fixed

- **Codex Auto-Go Completion Handoff Gate Recovery**: Codex `@auto go ... --auto --loop` 가 구현/검증 요약만 남기고 종료하지 않도록 completion handoff contract를 source-of-truth와 회귀 테스트에 고정
  - `templates/codex/skills/auto-go.md.tmpl`, `templates/codex/prompts/auto-go.md.tmpl` — `Completion Handoff Gates` 와 `Final Output Contract` 를 추가해 `current_gate`, `phase_4_review_verdict`, `next_required_step`, `next_command`, `auto_progression_state` 가 비면 success-style completion summary로 닫지 못하게 보강
  - `pkg/adapter/codex/codex_surface_test.go`, `pkg/adapter/codex/codex_prompts_test.go` — generated Codex skill/prompt surface가 workflow lifecycle 뒤에 next-step handoff contract를 유지하는지 회귀 테스트 추가

## [v0.40.28] — 2026-04-16

### Fixed

- **Legacy SPEC Status Sync Recovery**: `auto spec review` 가 PASS 후 `approved` 상태를 새 scaffold SPEC뿐 아니라 기존 legacy SPEC 형식에도 안전하게 반영하도록 메타데이터 파서와 상태 갱신 경로를 복구
  - `pkg/spec/metadata.go` — `# SPEC: ...` + `**SPEC-ID**:` / `**Status**:` legacy metadata를 읽도록 보강하고, frontmatter 탐지를 문서 상단으로 제한해 본문 `---` 구분선을 잘못된 frontmatter로 오인하지 않도록 수정
  - `pkg/spec/metadata_test.go` — legacy ID/status 파싱, legacy status rewrite, 본문 separator 보호 회귀 테스트 추가

- **Status Dashboard Legacy Title Recovery**: `status` 대시보드가 legacy `# SPEC: ...` 헤더를 쓰는 SPEC에서도 ID, 상태, 제목을 다시 함께 표시하도록 회귀를 보강
  - `internal/cli/status_legacy_test.go` — `# SPEC: ...` + `**SPEC-ID**:` 형식의 legacy SPEC가 대시보드에서 제목과 상태를 유지하는지 검증

## [v0.40.27] — 2026-04-16

### Fixed

- **Auto Sync Completion Gate Recovery**: Codex `auto sync` 가 더 이상 컨텍스트/주석/커밋 게이트를 빠뜨린 채 완료를 선언하지 않도록 completion discipline을 source-of-truth와 테스트에 고정
  - `templates/codex/skills/auto-sync.md.tmpl`, `templates/codex/prompts/auto-sync.md.tmpl` — `Context Load`, `SPEC Path Resolution`, `@AX Lifecycle Management`, `Lore commit hash 또는 blocked reason`, `2-Phase Commit decision` 을 `Completion Gates` 로 승격하고, 암묵적 subagent 제한 시 사용자 opt-in 또는 `--solo` 확인을 먼저 요구하도록 보강
  - `pkg/adapter/codex/codex_prompts_test.go`, `pkg/adapter/codex/codex_surface_test.go` — generated Codex prompt/skill surface가 `@AX: no-op`, `commit hash`, completion gate 문구를 유지하는지 회귀 테스트 추가

- **OpenCode Runtime Wording Parity**: OpenCode generated `auto sync` skill에 Codex 전용 런타임 문구가 새지 않도록 변환기와 회귀 테스트를 보강
  - `pkg/adapter/opencode/opencode_util.go` — `task(...)` 문맥에서 `Codex 런타임 정책` 잔여 문구를 `OpenCode 런타임 정책` 으로 정규화
  - `pkg/adapter/opencode/opencode_test.go`, `pkg/adapter/opencode/opencode_sync_gate_test.go` — shared `.agents/skills/auto-sync/SKILL.md` 에 completion gate와 OpenCode wording parity가 유지되는지 검증

## [v0.40.26] — 2026-04-16

### Fixed

- **Workspace Policy Context Propagation**: `auto setup` 이 루트 저장소 역할과 nested repo 경계, generated/runtime 추적 정책을 별도 `workspace.md` 문서로 기록하고 이후 라우터가 공통 컨텍스트로 다시 읽도록 정렬
  - `templates/codex/skills/auto-setup.md.tmpl`, `templates/codex/prompts/auto-setup.md.tmpl` — `workspace.md` 를 `.autopus/project/` 핵심 산출물로 승격하고 meta workspace / source-of-truth / generated-runtime 경로 기록 규약 추가
  - `templates/codex/skills/auto-go.md.tmpl`, `templates/codex/prompts/auto-go.md.tmpl`, `templates/codex/skills/auto-sync.md.tmpl`, `templates/codex/prompts/auto-sync.md.tmpl` — 구현/동기화 단계가 `.autopus/project/workspace.md` 를 공통 프로젝트 컨텍스트로 로드하도록 보강
  - `pkg/adapter/codex/codex_context_docs.go`, `pkg/adapter/codex/codex_skill_render.go`, `pkg/adapter/opencode/opencode_router_contract.go`, `pkg/adapter/opencode/opencode_util.go`, `templates/claude/commands/auto-router.md.tmpl` — Codex prompt/plugin router, OpenCode shared router/alias command, Claude router가 모두 동일한 workspace policy context load 및 canonical router hand-off 계약을 따르도록 정렬
  - `pkg/adapter/codex/codex_workspace_context_test.go`, `pkg/adapter/opencode/opencode_workspace_context_test.go`, `pkg/adapter/claude/claude_workspace_context_test.go` — `workspace.md` 전파 회귀 테스트를 추가해 플랫폼별 contract drift를 다시 통과하지 못하게 보강

## [v0.40.25] — 2026-04-16

### Fixed

- **Codex Router Prompt Contract Recovery**: Codex `@auto` 메인 prompt surface가 workflow skill 쪽에만 있던 브랜딩/실행 계약을 prompt에도 동일하게 주입하고, 대형 프로젝트 문서가 잘리지 않도록 기본 project doc budget을 상향
  - `pkg/adapter/codex/codex_prompts.go`, `pkg/adapter/codex/codex_skill_render.go` — generated `.codex/prompts/auto*.md` 에 canonical branding block과 `Router Execution Contract` 를 주입
  - `templates/codex/config.toml.tmpl`, `pkg/adapter/codex/codex_lifecycle.go` — `project_doc_max_bytes` 기본값을 `262144` 로 상향하고, router prompt / config drift를 `validate` 에서 탐지하도록 보강
  - `pkg/adapter/codex/codex_*_test.go` — branding, router contract, Context7 rule, doc budget 회귀 테스트 추가

- **Context7 Web Fallback Contract Recovery**: 외부 라이브러리 문서 조회 규칙이 이제 `Context7 MCP 우선 → 실패 시 web search fallback` 계약을 공통 rule, pipeline skill, Codex/OpenCode generated surface 전반에서 일관되게 유지
  - `content/rules/context7-docs.md`, `content/skills/agent-pipeline.md`, `pkg/adapter/codex/codex_extended_skill_rewrites_agents.go` — Context7 실패 시 official docs / release notes / API reference 중심 web fallback 절차를 문서화
  - `pkg/content/skill_transformer_replace.go` — non-Claude platform surface에서 `mcp__context7__*` references를 단순 `WebSearch` 치환이 아니라 Context7-first / web-fallback 의미가 보존되는 안내로 변환
  - `pkg/adapter/opencode/opencode_lifecycle.go`, `pkg/adapter/opencode/opencode_test.go`, `pkg/content/*test.go` — OpenCode/Codex validate와 content transformer 회귀 테스트로 fallback 계약 누락을 다시 통과하지 못하게 보강

## [v0.40.24] — 2026-04-16

### Fixed

- **Acceptance Gate Lifecycle Recovery**: `spec validate` 와 pipeline validate/review 경로가 더 이상 `acceptance.md` 를 무시하지 않고, scaffold 기본 시나리오 형식도 실제 Gherkin 파서와 일치하도록 복구
  - `pkg/spec/template.go`, `pkg/spec/gherkin_parser.go` — `spec.Load()` 가 `acceptance.md` 를 함께 로드해 `AcceptanceCriteria` 를 채우고, `### Scenario 1:` / `### Edge Case 1:` scaffold 헤더를 파싱하도록 정렬
  - `pkg/pipeline/phase_prompt.go`, `pkg/spec/template_test.go`, `pkg/pipeline/phase_prompt_test.go`, `internal/cli/cli_extra_test.go` — `test_scaffold` / `implement` / `validate` / `review` 프롬프트에 acceptance context를 주입하고, scaffolded SPEC validate 회귀를 추가

- **Codex Shared Skill Branding Recovery**: Codex 에서 `@auto` 브랜드 배너가 간헐적으로 사라지던 문제를, 실제 우선 선택되던 shared `.agents/skills/` 경로에도 canonical branding block을 주입하도록 보강
  - `pkg/adapter/opencode/opencode_util.go`, `pkg/adapter/opencode/opencode_skills.go`, `pkg/adapter/opencode/opencode_workflow_custom.go` — OpenCode가 소유하는 shared skill surface에도 `## Autopus Branding` 과 canonical banner injection을 적용
  - `pkg/adapter/opencode/opencode_test.go` — generated `.agents/skills/auto*.md` 가 branding header를 유지하는지 회귀 테스트 추가

## [v0.40.20] — 2026-04-15

### Fixed

- **OpenCode Router SPEC Path Resolution Contract Recovery**: OpenCode `auto` command/skill 생성물이 shared router contract의 `SPEC Path Resolution` 섹션을 다시 포함하고, OpenCode 표면에 Codex 전용 wording이 새지 않도록 정렬
  - `pkg/adapter/opencode/opencode_router_contract.go`, `pkg/adapter/opencode/opencode_commands.go`, `pkg/adapter/opencode/opencode_skills.go` — Claude canonical router에서 SPEC path resolution block을 추출해 OpenCode `auto` surfaces에 재주입하고, `TARGET_MODULE` / `WORKING_DIR` / `Available SPECs` 계약을 복원
  - `pkg/adapter/opencode/opencode_test.go` — 생성된 `.opencode/commands/auto.md` 와 `.agents/skills/auto/SKILL.md` 가 `SPEC Path Resolution` 을 유지하고 Codex wording leak이 없는지 회귀 테스트 추가

- **Workspace-Root Submodule SPEC Resolution Regression Coverage**: workspace root에서 실행되는 OpenCode SPEC 워크플로우가 `Autopus/.autopus/specs/...` 같은 실제 서브모듈 SPEC를 놓치지 않도록 회귀 케이스를 보강
  - `pkg/spec/resolve_test.go` — `SPEC-OPCOCK-001` 이 workspace root 기준으로 `Autopus` 서브모듈에서 정확히 resolve 되는지 검증

## [v0.40.18] — 2026-04-14

### Fixed

- **Codex `@auto` Branding Injection**: Codex local plugin skill surface가 router/prompt에는 있던 문어 배너 지시를 실제 `@auto` plugin workflow skill에도 동일하게 주입하도록 정렬
  - `pkg/adapter/codex/codex_skill_render.go`, `pkg/adapter/codex/codex_workflow_custom.go` — router skill과 workflow/custom workflow skill 생성 경로 모두에 canonical Autopus branding block을 삽입
  - `pkg/adapter/codex/codex_surface_test.go` — `.agents` / `.autopus/plugins` Codex skill surfaces가 branding header를 유지하는지 회귀 테스트 추가

## [v0.40.17] — 2026-04-14

### Added

- **OpenCode Strategic Skill Canonical Sources**: OpenCode가 더 이상 Claude 전용 산출물에 의존하지 않도록 `product-discovery`, `competitive-analysis`, `metrics`를 canonical `content/skills/`에 추가
  - `content/skills/product-discovery.md`, `content/skills/competitive-analysis.md`, `content/skills/metrics.md` — platform-agnostic source로 승격하여 OpenCode `.agents/skills/`에도 동일하게 배포되도록 정렬

### Fixed

- **Codex Workflow and Rule Parity Recovery**: Codex 하네스가 Claude Code 기준 workflow surface와 규칙 패키징을 다시 충족하도록 정렬
  - `pkg/adapter/codex/codex_workflow_specs.go`, `pkg/adapter/codex/codex_workflow_custom.go`, `pkg/adapter/codex/codex_prompts.go`, `templates/codex/prompts/auto.md.tmpl` — `@auto` router와 workflow generation이 `status`, `map`, `why`, `verify`, `secure`, `test`, `dev`, `doctor`를 포함한 전체 helper flow surface를 생성하도록 복구
  - `pkg/adapter/codex/codex_rules.go`, `pkg/adapter/codex/codex_skill_render.go`, `pkg/adapter/codex/codex_skill_template_mappings.go`, `pkg/adapter/codex/codex_standard_skills.go` — Codex rule/skill rendering이 stub `@import` 대신 canonical content와 Codex-native semantics를 사용하고 `branding`, `project-identity` rule parity를 회복
  - `pkg/adapter/codex/codex_*_test.go`, `pkg/adapter/parity_test.go`, `pkg/adapter/integration_test.go` — prompt/rule count와 cross-platform parity 회귀 테스트를 추가해 workflow 누락과 규칙 드리프트를 다시 통과하지 못하게 보강

- **OpenCode Helper Flow Surface Recovery**: OpenCode router와 command surface가 `setup` 외 helper flow도 노출하고, Codex prompt 단일 의존 없이 OpenCode 전용 contract를 사용하도록 정리
  - `pkg/adapter/opencode/opencode_specs.go`, `pkg/adapter/opencode/opencode_router_contract.go`, `pkg/adapter/opencode/opencode_workflow_custom.go` — `status`, `map`, `why`, `verify`, `secure`, `test`, `dev`, `doctor` helper flow inventory와 custom skill/command body 추가
  - `pkg/adapter/opencode/opencode_commands.go`, `pkg/adapter/opencode/opencode_skills.go` — router/command generation이 OpenCode-native helper semantics와 상세 스킬 목록을 사용하도록 갱신

- **OpenCode Plugin Wiring Diagnostics**: hook plugin이 파일만 생성되고 `opencode.json`에는 연결되지 않던 결손을 수정하고, registration 누락을 validation에서 탐지하도록 보강
  - `pkg/adapter/opencode/opencode_config.go`, `pkg/adapter/opencode/opencode.go`, `pkg/adapter/opencode/opencode_lifecycle.go`, `pkg/adapter/opencode/opencode_util.go` — managed plugin 경로를 기본 등록하고 plugin array parsing/validation을 보강
  - `pkg/adapter/opencode/opencode_runtime_test.go`, `pkg/adapter/opencode/opencode_test.go` — helper flow surface, plugin registration, strategic skill generation 회귀 테스트 추가

- **Queued Task Deadline Guard**: 이미 만료된 worker task가 semaphore 슬롯을 선점하거나 subprocess를 시작하지 않도록 acquire 단계의 cancellation 우선순위를 보강
  - `pkg/worker/parallel/semaphore.go`, `pkg/worker/loop_runtime_fix_test.go` — 만료된 context는 즉시 거절하고 queued-task expiry 회귀 테스트 기대를 다시 만족하도록 정렬
  - `pkg/adapter/integration_test.go` — Codex prompt surface 확장에 맞춰 E2E prompt count 기대치를 갱신

- **Worker MCP Startup Compatibility**: Codex가 worker MCP 서버를 startup 단계에서 타입 오류 없이 수용하도록 초기 lifecycle, tool schema, resource 응답 형식을 최신 MCP 계약에 가깝게 정렬
  - `pkg/worker/mcpserver/server.go`, `pkg/worker/mcpserver/server_test.go` — `initialize` protocol negotiation, `tools/list` schema metadata, `tools/call` structured result envelope, `resources/templates/list`, `resources/read` contents wrapper 추가
  - `pkg/worker/mcpserver/resources.go`, `pkg/worker/mcpserver/resources_test.go` — resource title/template metadata를 추가해 execution URI template discovery를 노출
  - `templates/codex/config.toml.tmpl` — Codex generated config가 `autopus` MCP를 다시 기본 등록해도 startup validation을 통과하도록 정렬

## [v0.40.13] — 2026-04-14

### Fixed

- **OpenCode Workflow Surface Alignment**: OpenCode가 `auto` workflow를 얇은 prompt entrypoint가 아니라 실제 skill 템플릿과 맞는 표면으로 생성하도록 정렬
  - `pkg/adapter/opencode/opencode_specs.go`, `pkg/adapter/opencode/opencode_skills.go` — workflow별 prompt와 skill source를 분리하고, `auto`는 thin router / 하위 workflow는 실제 skill 템플릿으로 생성되도록 조정
  - `pkg/adapter/opencode/opencode_util.go` — OpenCode `task(...)` / command entrypoint semantics에 맞는 body normalization과 예제 치환 보강
  - `pkg/adapter/opencode/opencode_test.go` — workflow skill / command surface 회귀 테스트 추가

- **Codex Router Thin-Skill Stabilization**: Codex router skill이 더 이상 Claude router rewrite에 의존하지 않고 Codex thin router semantics로 생성되도록 정리
  - `pkg/adapter/codex/codex_standard_skills.go`, `pkg/adapter/codex/codex_skill_render.go`, `pkg/adapter/codex/codex_plugin_manifest.go` — router rendering과 plugin metadata를 분리하고 300-line limit를 만족하도록 파일 분할
  - `pkg/adapter/codex/codex_test.go` — `.agents/.autopus/.codex` 전 surface 회귀 테스트 추가

- **Gemini Canary Workflow Parity**: Gemini `canary` command가 참조하던 `auto-canary` skill 누락을 보완해 command-skill 정합성을 복구
  - `templates/gemini/skills/auto-canary/SKILL.md.tmpl` — Gemini 전용 `auto-canary` skill 추가
  - `pkg/adapter/gemini/gemini_test.go` — workflow command와 대응 skill 생성 정합성 회귀 테스트 추가

## [v0.40.12] — 2026-04-14

### Fixed

- **`auto update` New Platform Detection**: 바이너리 업데이트 후 새로 설치한 OpenCode 같은 supported CLI가 기존 프로젝트의 `auto update` 경로에서 자동 반영되지 않던 문제 수정
  - `internal/cli/update.go`, `internal/cli/init_helpers.go` — `update`가 현재 설치된 supported platform을 다시 감지해 `autopus.yaml`에 누락된 플랫폼을 추가하고, 같은 실행에서 해당 하네스를 생성하도록 정렬
  - `internal/cli/update_test.go` — 기존 `claude-code` 프로젝트에서 `opencode` 설치 후 `auto update`가 `opencode.json`과 `.opencode/` 하네스를 생성하는 회귀 테스트 추가

## [v0.40.11] — 2026-04-14

### Fixed

- **Worker Queue Timeout Separation**: worker 실행 대기와 provider 세마포어 대기를 분리해, 혼잡 상황에서도 queue starvation과 잘못된 타임아웃 해석이 줄어들도록 정리
  - `pkg/worker/loop.go`, `pkg/worker/loop_exec.go`, `pkg/worker/loop_test.go` — worker loop가 queue wait / execution timeout을 구분해 처리하고 직렬화 경로를 더 명확히 검증하도록 보강
  - `internal/cli/worker_start.go`, `internal/cli/worker_start_test.go` — worker start 경로가 새 timeout semantics와 직렬화 보강을 반영하도록 조정

- **Codex Worker Concurrency Stabilization**: Codex worker 동시 실행 시 output artifact와 setup 경로가 더 안정적으로 유지되도록 보강
  - `internal/cli/worker_setup_wizard.go`, `internal/cli/worker_setup_wizard_test.go` — setup wizard가 최신 worker concurrency 흐름과 일치하도록 조정

## [v0.40.10] — 2026-04-14

### Added

- **OpenCode Native Harness Generation**: `auto init/update`가 이제 OpenCode를 정식 하네스 설치 플랫폼으로 지원하여 `.opencode/` 네이티브 산출물과 `.agents/skills/` 표준 스킬을 함께 생성
  - `pkg/adapter/opencode/*` — OpenCode 어댑터를 stub에서 실제 generate/update/validate/clean 구현으로 확장하고 `AGENTS.md`, `opencode.json`, `.opencode/rules/`, `.opencode/agents/`, `.opencode/commands/`, `.opencode/plugins/`를 생성
  - `internal/cli/init_helpers.go`, `internal/cli/update.go`, `internal/cli/doctor.go`, `internal/cli/platform.go`, `internal/cli/init.go` — OpenCode를 init/update/doctor/platform add-remove 및 gitignore 경로에 연결
  - `pkg/adapter/opencode/opencode_test.go`, `pkg/content/opencode_transform_test.go` — OpenCode 산출물 생성, 설정 병합, CLI 연결, 변환 규칙 회귀 테스트 추가

### Fixed

- **OpenCode Content Mapping**: Claude 중심 helper 문서와 agent source가 OpenCode native surface에 맞게 치환되도록 정렬
  - `pkg/content/skill_transformer.go`, `pkg/content/skill_transformer_replace.go`, `pkg/content/agent_transformer_opencode.go` — `.claude/*` 경로를 `.opencode/*` / `.agents/skills/*`로 치환하고, subagent/tool references를 OpenCode `task`, `question`, `todowrite` 중심 semantics로 재해석

### Fixed

- **JWT-Only Worker / No-Bridge Cleanup**: worker setup, connect wizard, runtime lifecycle가 더 이상 bridge source provisioning이나 bridge-based file sync를 전제로 하지 않도록 정리
  - `internal/cli/worker_setup_wizard.go`, `internal/cli/connect.go`, `internal/cli/worker_start.go` — setup/connect가 JWT-only auth 및 authenticated provider 우선 선택으로 정렬되고 bridge source 자동 생성 제거
  - `pkg/worker/loop.go`, `pkg/worker/loop_lifecycle.go`, `pkg/worker/setup/config.go` — runtime이 legacy bridge sync source를 더 이상 사용하지 않고 local knowledge search만 유지하도록 조정
  - `pkg/e2e/build.go`, `README.md` — user-facing build/docs 표면에서 deprecated bridge target 설명 제거

## [v0.40.5] — 2026-04-13

### Fixed

- **Worker Launch Readiness Alignment**: worker setup이 knowledge source provisioning, worktree isolation, runtime launch 경로를 실제 실행 계약과 맞추도록 정리
  - `internal/cli/worker_setup_wizard.go`, `internal/cli/worker_start.go`, `pkg/worker/loop_lifecycle.go` — setup wizard에서 받은 knowledge/worktree 설정이 런칭 직전 lifecycle과 source provisioning에 실제 연결되도록 보강
  - `pkg/worker/setup/config.go`, `pkg/worker/setup/config_test.go` — worker config가 knowledge source 및 isolation 필드를 안정적으로 유지하도록 회귀 보강

- **Knowledge Sync / MCP Path Contract Repair**: knowledge sync와 MCP 검색 경로가 현재 서버 계약 및 테스트 기대와 다시 일치
  - `pkg/worker/knowledge/syncer.go`, `pkg/worker/knowledge/syncer_test.go` — knowledge sync 입력/출력 경로와 에러 처리 흐름을 서버 계약 기준으로 복구
  - `pkg/worker/mcpserver/tools.go`, `pkg/worker/mcpserver/tools_test.go` — MCP search tooling이 sync된 knowledge location을 기준으로 검색하도록 정렬

- **Claude Worker Session Resume Recovery**: Claude worker 재개 경로가 현재 런타임/테스트 기대와 맞게 복구
  - `pkg/worker/adapter/claude.go` — resumed Claude worker session wiring을 현재 adapter contract에 맞게 조정

## [v0.40.4] — 2026-04-13

### Fixed

- **Codex Team Mode Semantics**: Codex `--team` 문서와 생성 스킬이 이제 Claude Team API가 아니라 하네스가 생성한 `.codex/agents/*` 역할 정의를 사용하는 멀티에이전트 오케스트레이션으로 정렬
  - `pkg/adapter/codex/codex_extended_skill_rewrites.go` — `agent-teams` / `agent-pipeline` Codex rewrite가 harness-defined agents와 `spawn_agent(...)` coordination을 기준으로 설명되도록 갱신
  - `templates/codex/skills/agent-teams.md.tmpl`, `templates/codex/skills/auto-go.md.tmpl`, `templates/codex/prompts/auto-go.md.tmpl` — generated Codex docs now explain `--team` as `.codex/agents/` role orchestration and `--multi` as extra review/orchestra reinforcement

- **`--multi` Runtime Activation**: 루트 전역 플래그 `--multi`가 더 이상 단순 노출에 그치지 않고 SPEC review / pipeline run에서 실제 멀티 프로바이더 리뷰 흐름을 확장
  - `internal/cli/spec_review.go` — `--multi` 시 review provider set을 review gate + orchestra config + default providers로 확장하고, 설치된 provider가 2개 미만이면 명확히 실패
  - `internal/cli/pipeline_run.go` — `auto pipeline run --multi` 완료 후 실제 `runSpecReview(...)`를 호출해 다중 프로바이더 검증을 수행
  - `internal/cli/spec_review_test.go`, `internal/cli/pipeline_run_test.go`, `pkg/adapter/codex/codex_coverage_test.go` — provider expansion 및 Codex multi/team semantics regression coverage 추가

## [v0.40.3] — 2026-04-13

### Fixed

- **Codex Harness Hook Drift**: Codex 훅 생성이 더 이상 깨진 템플릿 명령에 의존하지 않고, 실제 훅 생성 로직과 같은 소스에서 `.codex/hooks.json`을 만들도록 정리
  - `pkg/adapter/codex/codex_hooks.go` — Codex hook rendering now marshals `pkg/content/hooks.go` output directly, so `PreToolUse`/`PostToolUse` stay aligned with real CLI support
  - `pkg/adapter/codex/codex_internal_test.go`, `pkg/adapter/codex/codex_coverage_test.go` — invalid `SessionStart`/`Stop` expectations 제거, unsupported `auto check --status`, `auto session save`, `auto check --lore --quiet` 회귀 방지

- **Lore Guidance Alignment**: Lore 문서와 생성 스킬이 현재 프로토콜과 실제 검사 범위를 기준으로 정리
  - `content/rules/lore-commit.md`, `content/skills/lore-commit.md` — legacy `Why/Decision/Alternatives` 중심 설명을 `Constraint` 계열 프로토콜과 `auto check --lore` / `auto lore validate` 실제 역할 기준으로 갱신
  - `templates/codex/skills/lore-commit.md.tmpl`, `templates/gemini/skills/lore-commit/SKILL.md.tmpl` — 생성되는 Codex/Gemini Lore 스킬도 동일한 프로토콜로 정렬

## [v0.40.2] — 2026-04-13

### Fixed

- **Release Workflow Action Drift**: GitHub Release workflow의 deprecated Node 20 / floating version 경고를 줄이기 위해 action 버전과 GoReleaser 버전 범위를 최신 기준으로 정리
  - `.github/workflows/release.yaml` — `actions/checkout@v6`, `actions/setup-go@v6`, `goreleaser/goreleaser-action@v7` 로 갱신
  - `.github/workflows/release.yaml` — GoReleaser 실행 버전을 `latest` 대신 `~> v2`로 고정해 릴리즈 시 경고를 제거
  - `.github/workflows/release.yaml` — 더 이상 필요 없는 `FORCE_JAVASCRIPT_ACTIONS_TO_NODE24` 환경 변수 제거

## [v0.40.1] — 2026-04-13

### Fixed

- **Codex Harness Flag Parity**: Codex `@auto` router와 하위 스킬이 Claude 전용 가정을 덜어내고 Codex 실행 모델에 맞게 정규화됨
  - `pkg/adapter/codex/codex_standard_skills.go` — `AskUserQuestion`, `TeamCreate`, `SendMessage`, legacy `/auto` 예시를 Codex의 `spawn_agent(...)`, `send_input(...)`, plain-text 확인 흐름으로 재해석
  - `templates/codex/skills/auto-*.md.tmpl`, `templates/codex/prompts/auto-*.md.tmpl` — `--team`, `--loop`, `--auto`, `--quality`, `--continue` 등 핵심 플래그 의미와 `@auto ...` 표기를 보강
  - `templates/codex/skills/auto-canary.md.tmpl` — `auto-canary`를 prompt fallback이 아닌 전용 skill 템플릿 기반으로 생성

- **Codex Helper Skill Rewrite Layer**: 깊은 helper 문서가 더 이상 Claude Code Team/permission/worktree 전제를 직접 요구하지 않도록 Codex 전용 body rewrite 추가
  - `pkg/adapter/codex/codex_extended_skill_rewrites.go` — `agent-teams`, `agent-pipeline`, `worktree-isolation`, `subagent-dev`, `prd` 문서를 Codex orchestration semantics로 재작성
  - `pkg/adapter/codex/codex_extended_skills.go`, `codex_skills.go`, `codex_prompts.go`, `codex_agents.go` — helper path 및 invocation 정규화를 생성 파이프라인 전반에 적용
  - `pkg/adapter/codex/codex_coverage_test.go` — Codex 전용 rewrite 회귀 테스트 추가

## [v0.40.0] — 2026-04-13

### Added

- **Codex Standard Skills + Local Plugin Bootstrap**: Codex 최신 표준에 맞춰 repo skill 및 local plugin 진입점을 자동 생성
  - `pkg/adapter/codex/codex_standard_skills.go` — `.agents/skills/*` 표준 스킬과 `.autopus/plugins/auto` 로컬 플러그인 번들 생성
  - `pkg/adapter/codex/codex.go` — Codex generate/update 시 `.agents/skills`, `.agents/plugins`, `.autopus/plugins/auto` 출력 경로 생성
  - `pkg/adapter/codex/codex_lifecycle.go` — validate/clean이 `.agents/skills/*`, `.agents/plugins/marketplace.json`, `.autopus/plugins/auto`를 인식하도록 확장
  - `pkg/adapter/codex/codex_skills.go` — AGENTS.md에 Agent Skills / Plugin Marketplace 경로 노출
  - `internal/cli/init.go` — Codex 다음 단계 안내를 `$auto ...` / `@auto ...` 기준으로 갱신하고 `.agents/plugins/`를 gitignore에 추가
  - `pkg/adapter/codex/codex_test.go`, `pkg/adapter/integration_test.go`, `pkg/adapter/parity_test.go`, `internal/cli/*_test.go` — 표준 스킬/플러그인 생성 회귀 테스트 추가

- **Codex Invocation Normalization**: Codex generated skill examples and chaining messages now prefer `@auto plan`, `@auto go`, `@auto idea` syntax while preserving `$auto ...` fallback
  - generated Codex skills normalize legacy `/auto` and `@auto-foo` references into Codex-compatible `@auto foo` forms

- **Codex Brainstorm / Multi-Provider Parity**: `auto idea` workflow is now exposed through Codex standard entrypoints without dropping multi-provider discussion or flag-based chaining
  - generated `auto-idea` Codex skills preserve `--strategy`, `--providers`, `--auto` and `@auto plan --from-idea ...` chaining semantics

### Added

- **Gemini CLI Harness Parity**: Gemini CLI 어댑터에 Claude Code 및 Codex 수준의 기능 패리티 구현
  - `/auto` 라우터 명령어 지원 (`auto-router.md.tmpl`)
  - 상태 업데이트를 위한 `statusline.sh` 복사 로직 추가
  - 테스트 코드에 Gemini 템플릿 포함 및 검증 추가

### Fixed

- **macOS Self-Update Crash (zsh: killed)**: `auto update --self` 실행 시 macOS 커널 보호(SIGKILL) 및 Linux ETXTBSY 에러 우회
  - 실행 중인 바이너리를 덮어쓰지 않고 `.old`로 이동(Rename) 후 새 바이너리로 교체하도록 `replacer.go` 수정
  - Cross-device 링크 시 fallback (io.Copy) 로직 추가


- **Init Platform Auto-Detection**: `auto init` without `--platforms` now scans PATH for supported installed coding CLIs and installs all detected supported platforms
  - `internal/cli/init.go` — default platform selection now delegates to PATH-based detection when `--platforms` is omitted
  - `internal/cli/init_helpers.go` — `detectDefaultPlatforms()` filters detected CLIs to ADK-supported init targets (`claude-code`, `codex`, `gemini-cli`) with Claude fallback
  - `internal/cli/init_test.go` — auto-detect and no-CLI fallback regression tests
  - `pkg/detect/detect.go` — orchestra provider detection now tracks `codex` instead of stale `opencode`
  - `pkg/detect/detect_test.go` — provider detection expectations updated to Codex
  - `README.md`, `docs/README.ko.md` — docs aligned to 3 auto-generated platforms and supported-CLI wording

- **Worker 프로세스 안정화** (SPEC-WKPROC-001):
  - `pkg/worker/pidlock/` — PID lock 패키지 (advisory flock, stale detection, auto-reclaim)
  - `pkg/worker/reaper/` — Zombie 프로세스 reaper (30초 주기, Unix Wait4, build-tag 분리)
  - `pkg/worker/mcpserver/sse.go` — MCP SSE transport (/mcp/sse 엔드포인트)
  - `pkg/worker/mcpserver/config.go` — MCP config 구조체 + JSON 검증
  - `pkg/worker/mcpserver/server.go` — NewMCPServerFromConfig, StartSSE 메서드
  - `pkg/worker/loop.go` — Start/Close에 PID lock 획득/해제 통합
  - `pkg/worker/loop_lifecycle.go` — startServices에 reaper goroutine 추가
  - `pkg/worker/daemon/launchd.go` — ProcessType=Background, ThrottleInterval=10
  - `pkg/worker/daemon/systemd.go` — StandardOutput/StandardError 로그 경로
  - `internal/cli/worker_commands.go` — worker status에 PID 표시

## [v0.37.0] — 2026-04-07

### Added

- **Pipeline-Learn Auto Wiring** (SPEC-LEARNWIRE-002): 파이프라인 gate 실패 시 자동 학습 기록
  - `pkg/learn/store.go` — AppendAtomic 동시성 안전 메서드 (sync.Mutex)
  - `pkg/pipeline/learn_hook.go` — nil-safe hook wrapper 4개 (gate fail, coverage gap, review issue, executor error) + 출력 파싱
  - `pkg/pipeline/runner.go` — SequentialRunner/ParallelRunner에 learn hook 와이어링 (R2-R6, R9)
  - `pkg/pipeline/phase.go` — DefaultPhases()에 GateValidation/GateReview 할당 (R10)
  - `pkg/pipeline/engine.go` — EngineConfig.RunConfig 필드 추가
  - `internal/cli/pipeline_run.go` — .autopus/learnings/ 조건부 Store 초기화 (D4)

- **SPEC Review Convergence** (SPEC-REVCONV-001): 2-Phase Scoped Review로 REVISE 루프 수렴성 보장
  - `pkg/spec/types.go` — FindingStatus, FindingCategory, ReviewMode 타입, ReviewFinding 확장 (ID/Status/Category/ScopeRef/EscapeHatch)
  - `pkg/spec/prompt.go` — Mode-aware BuildReviewPrompt (discover: open-ended, verify: checklist + FINDING_STATUS 스키마)
  - `pkg/spec/reviewer.go` — ParseVerdict 확장 (priorFindings 기반 scope filtering), ShouldTripCircuitBreaker, MergeFindingStatuses (supermajority merge)
  - `pkg/spec/review_persist.go` — PersistReview 분리 (reviewer.go 300줄 리밋 준수)
  - `pkg/spec/findings.go` — review-findings.json 영속화, ScopeRef 정규화, ApplyScopeLock, DeduplicateFindings
  - `pkg/spec/static_analysis.go` — golangci-lint JSON 파싱, RunStaticAnalysis graceful skip, MergeStaticWithLLMFindings dedup
  - `internal/cli/spec_review.go` — REVISE 루프 (discover→verify 전환, max_revisions, circuit breaker, static analysis 통합)
  - 테스트 커버리지 93.7% (convergence_test, findings_test, static_analysis_test, coverage_gap_test, coverage_merge_test)

- **resolvePlatform Unit Tests** (SPEC-AXQUAL-001): PATH 의존 플랫폼 감지 로직 단위 테스트 추가
  - `internal/cli/pipeline_run_test.go` — `TestResolvePlatform` table-driven 테스트 (explicit platform, PATH 탐색 우선순위, 빈 PATH 폴백)
  - `internal/cli/pipeline_run.go` — `@AX:TODO` 태그 제거, `@AX:NOTE` 추가
  - `internal/cli/agent_create.go`, `skill_create.go` — 템플릿 TODO 마커에 `@AX:EXCLUDE` 문서화

- **ADK Worker Approval Flow** (SPEC-ADKWA-001): Backend MCP → A2A WebSocket → Worker TUI 승인 플로우 구현
  - `pkg/worker/a2a/types.go` — `MethodApproval`, `MethodApprovalResponse` 상수, `ApprovalRequestParams`, `ApprovalResponseParams` 타입 정의
  - `pkg/worker/a2a/server.go` — `ApprovalCallback` 콜백 필드, `handleApproval` 핸들러 (input-required 상태 전환)
  - `pkg/worker/a2a/server_approval.go` — `SendApprovalResponse` (tasks/approvalResponse JSON-RPC 전송, working 상태 복원)
  - `pkg/worker/tui/model.go` — `OnApprovalDecision` / `OnViewDiff` 콜백, a/d/s/v 키 바인딩
  - `pkg/worker/loop.go` — WorkerLoop A2A 콜백 → TUI program 브릿지 와이어링

- **Multi-Platform Harness Integration** (SPEC-MULTIPLATFORM-001): Codex/Gemini 어댑터를 Claude Code 수준 하네스 패리티로 확장
  - Codex: 커스텀 프롬프트 (`codex_prompts.go`), 에이전트 정의 (`codex_agents.go`), 훅 설정 (`codex_hooks.go`), MCP/권한 설정 (`codex_settings.go`), 규칙 인라인 (`codex_rules.go`), 전체 스킬 변환 (`codex_skills.go`), 라이프사이클/마커 관리 (`codex_lifecycle.go`, `codex_marker.go`)
  - Gemini: 커스텀 커맨드 (`gemini_commands.go`), 에이전트 정의 (`gemini_agents.go`), 훅/설정 통합 (`gemini_hooks.go`, `gemini_settings.go`), 규칙+@import (`gemini_rules.go`), 전체 스킬 변환 (`gemini_skills.go`), 라이프사이클/마커 관리 (`gemini_lifecycle.go`, `gemini_marker.go`)
  - Shared: 크로스 플랫폼 템플릿 헬퍼 (`pkg/template/helpers.go` — TruncateToBytes, MapPermission, SkillList), 공유 테스트 유틸 (`pkg/adapter/testutil_test.go`)
  - Templates: `templates/codex/` (agents, prompts, skills, hooks.json.tmpl, config.toml.tmpl), `templates/gemini/` (commands, rules, settings, skills)

- **Permission Detect** (SPEC-PERM-001): `auto permission detect` 서브커맨드 및 agent-pipeline 동적 권한 상승
  - `pkg/detect/permission.go` — DetectPermissionMode: 부모 프로세스 트리에서 `--dangerously-skip-permissions` 감지, 환경변수 오버라이드, fail-safe 반환
  - `pkg/detect/permission_test.go` — 환경변수 오버라이드, invalid 값 폴백, 프로세스 검사 실패 시 safe 반환 테스트
  - `internal/cli/permission.go` — `auto permission detect` Cobra 서브커맨드, `--json` 출력 모드 지원
  - `content/skills/agent-pipeline.md` — Permission Mode Detection 섹션 추가, 동적 mode 할당 규칙
  - `templates/claude/commands/auto-router.md.tmpl` — Step 0.5 Permission Detect 및 조건부 mode 파라미터

- **Brainstorm Multi-Turn Debate Protocol** (SPEC-ORCH-009): brainstorm 커맨드에서 멀티턴 debate 활성화 및 ReadScreen 출력 정제 강화
  - `internal/cli/orchestra_brainstorm.go` — `resolveRounds()` 호출 추가로 brainstorm debate 기본 2라운드 적용, `--rounds N` 플래그 추가
  - `pkg/orchestra/screen_sanitizer.go` — SanitizeScreenOutput: ANSI/CSI/OSC/DCS 이스케이프, 상태바, trailing whitespace 제거하는 순수 함수
  - `pkg/orchestra/interactive_detect.go` — cleanScreenOutput()에서 SanitizeScreenOutput() 호출로 rebuttal 프롬프트 품질 개선

- **Interactive Multi-Turn Debate** (SPEC-ORCH-008): interactive pane에서 N라운드 핑퐁 토론 실행
  - `pkg/orchestra/interactive_debate.go` — runInteractiveDebate: 멀티턴 debate 루프 (Round1 독립응답 → Round2..N 교차 반박)
  - `pkg/orchestra/interactive_debate_helpers.go` — collectRoundHookResults, runJudgeRound, consensusReached, buildDebateResult
  - `pkg/orchestra/round_signal.go` — RoundSignalName: 라운드 스코프 시그널 파일명, CleanRoundSignals, SendRoundEnvToPane
  - `pkg/orchestra/hook_signal.go` — WaitForDoneRound/ReadResultRound: 라운드별 hook 결과 수집 (하위 호환)
  - `internal/cli/orchestra.go` — `--rounds N` 플래그 (1-10, debate 전략 전용, 기본값 2)
  - `content/hooks/` — AUTOPUS_ROUND 환경변수 인식 (라운드 스코프 파일명 분기, 정수 검증)
  - 조기 합의 감지 (MergeConsensus 66% 임계값), Judge 라운드 interactive 실행
  - hook-opencode-complete.ts sessId path traversal 검증 추가 (보안 수정)

- **Orchestra Hook-Based Result Collection** (SPEC-ORCH-007): 프로바이더 CLI의 hook/plugin 시스템을 활용하여 구조화된 JSON 파일 시그널로 결과 수집
  - `pkg/orchestra/hook_signal.go` — HookSession: 세션 디렉토리 관리, done 파일 200ms 폴링 감시, result.json 파싱, 0o700/0o600 보안 권한
  - `pkg/orchestra/hook_watcher.go` — Hook 모드 waitForCompletion: 프로바이더별 hook/ReadScreen 혼합 분기, 타임아웃 graceful degradation
  - `content/hooks/hook-claude-stop.sh` — Claude Code Stop hook: `last_assistant_message` 추출 → result.json 저장
  - `content/hooks/hook-gemini-afteragent.sh` — Gemini CLI AfterAgent hook: `prompt_response` 추출 → result.json 저장
  - `content/hooks/hook-opencode-complete.ts` — opencode plugin: `text` 필드 추출 → result.json 저장
  - `pkg/adapter/opencode/opencode.go` — opencode PlatformAdapter: plugin 자동 주입, opencode.json 생성/머지
  - `pkg/adapter/claude/claude_settings.go` — Stop hook 자동 주입 (기존 사용자 hook 보존)
  - `pkg/adapter/gemini/gemini_hooks.go` — AfterAgent hook 자동 주입 (기존 사용자 hook 보존)
  - `pkg/config/migrate.go` — codex → opencode 자동 마이그레이션
  - hook 미설정 프로바이더는 기존 SPEC-ORCH-006 ReadScreen + idle 감지로 자동 fallback (R8)
  - debate/relay/consensus 전략이 hook 결과의 `response` 필드를 직접 활용 (R11-R13)

### Fixed

- **Issue Reporter / React Hook Reliability**:
  - `internal/cli/issue.go` — `auto issue report/list/search` now prefer `autopus.yaml` repo config and default autopus issue target for `auto ...` command failures instead of accidentally following the current workspace remote
  - `internal/cli/react.go` — `auto react check --quiet` now skips cleanly when the repo has no configured remote, avoiding repeated Claude hook noise
  - `pkg/content/hooks.go`, `templates/codex/hooks.json.tmpl`, `content/hooks/react-*.sh` — all generated reaction hooks now use the supported `auto react check --quiet` command and deduplicate duplicate `PostToolUse` entries
  - `pkg/spec/resolve_test.go` — added nested submodule regression coverage for depth-2 SPEC resolution

- **SPEC Review Context + Parent Harness Isolation**:
  - `pkg/spec/prompt.go`, `internal/cli/spec_review.go` — `auto spec review` now collects code context only from files explicitly referenced by SPEC `plan.md` / `research.md`, instead of recursively sweeping the whole repo
  - `pkg/spec/reviewer_test.go` — regression coverage for target-file-only collection and module-relative path resolution
  - `pkg/detect/detect.go`, `internal/cli/prompts.go` — parent Autopus rule directories are now treated as real inherited conflicts, and non-interactive init/update automatically set `isolate_rules: true`
  - `pkg/detect/detect_test.go`, `internal/cli/prompts_test.go`, `pkg/adapter/claude/claude_markers.go` — tests and Claude isolation guidance updated for nested harness scenarios

- **Installer PATH Visibility**: installers now expose the actual CLI location and make post-install shell behavior explicit, so `auto`/`autopus` are discoverable after one-line installs
  - `install.sh` — creates an `autopus` alias alongside `auto`, prints concrete PATH export instructions when the install dir is not visible to the current shell, and defers platform auto-detection to `auto init`
  - `install.ps1` — creates `autopus.exe` alongside `auto.exe`, persists PATH updates without duplicate entries, warns Git Bash users to reopen the shell or export the printed path, and defers platform auto-detection to `auto init`
  - `README.md`, `docs/README.ko.md` — install docs now state the `autopus` alias and the Git Bash PATH refresh caveat

- **E2E Scenario Runner Monorepo Build Path** (SPEC-E2EFIX-001): 모노레포 루트에서 `auto test run`할 때 서브모듈별 빌드 커맨드와 작업 디렉토리를 올바르게 해석하도록 수정
  - `pkg/e2e/build.go` (신규) — `BuildEntry` 구조체, `ParseBuildLine()` 멀티 빌드 파서, `ResolveBuildDir()` 서브모듈 경로 매핑, `MatchBuild()` 시나리오별 빌드 선택
  - `pkg/e2e/scenario.go` — `ScenarioSet.Builds []BuildEntry` 필드 추가, `ParseScenarios()` 멀티 빌드 위임
  - `pkg/e2e/runner.go` — 빌드 엔트리별 `sync.Once` 맵, 시나리오 섹션 기반 빌드 선택 및 서브모듈 WorkDir 적용
  - `internal/cli/test.go` — `set.Builds`를 `RunnerOptions`에 전달, 단일 빌드 폴백 유지

### Added

- **Orchestra Interactive Pane Mode** (SPEC-ORCH-006): cmux/tmux에서 프로바이더 CLI를 인터랙티브 세션으로 직접 실행하고 결과 자동 수집
  - `pkg/terminal/terminal.go` — Terminal 인터페이스에 `ReadScreen`, `PipePaneStart`, `PipePaneStop` 메서드 추가
  - `pkg/terminal/cmux.go` — CmuxAdapter: `cmux read-screen`, `cmux pipe-pane` 명령 래핑
  - `pkg/terminal/tmux.go` — TmuxAdapter: `tmux capture-pane`, `tmux pipe-pane` 명령 래핑
  - `pkg/terminal/plain.go` — PlainAdapter no-op 구현
  - `pkg/orchestra/interactive.go` — 인터랙티브 pane 실행 플로우 (pipe capture, session launch, prompt send, ReadScreen 폴링 완료 감지, 결과 수집)
  - `pkg/orchestra/interactive_detect.go` — 프로바이더별 프롬프트 패턴 매칭, idle 감지, ANSI 이스케이프 제거
  - `pane_runner.go`에 `OrchestraConfig.Interactive` 플래그 기반 인터랙티브 모드 분기
  - plain 터미널 또는 인터랙티브 실패 시 기존 sentinel 모드로 자동 fallback (R8)
  - 부분 타임아웃 시 `ReadScreen`으로 수집된 부분 결과를 `TimedOut: true`와 함께 기록 (R9)
  - ANSI 이스케이프 시퀀스, CLI 프롬프트 장식 자동 제거로 깨끗한 결과 전달 (R10)

- **Browser Automation Terminal Adapter** (SPEC-BROWSE-001): 터미널 환경별 브라우저 백엔드 자동 선택
  - `pkg/browse/backend.go` — BrowserBackend 인터페이스 + NewBackend 팩토리 (cmux → CmuxBrowserBackend, 그 외 → AgentBrowserBackend)
  - `pkg/browse/cmux.go` — CmuxBrowserBackend: `cmux browser` CLI 래핑, surface ref 관리, shell escape
  - `pkg/browse/agent.go` — AgentBrowserBackend: `agent-browser` CLI 래핑
  - cmux 실패 시 AgentBrowserBackend로 자동 fallback (R6)
  - 세션 종료 시 브라우저 surface/프로세스 자동 정리 (R7)

- **Orchestra Relay Pane Mode** (SPEC-ORCH-005): relay 전략에서 cmux/tmux pane 기반 인터랙티브 실행 지원
  - `pkg/orchestra/relay_pane.go` — 순차 pane relay 실행 엔진: SplitPane → 인터랙티브 실행 → sentinel 완료 감지 → 결과 수집 → 맥락 주입
  - `-p` 플래그 없이 프로바이더 CLI를 실행하여 전체 TUI/인터랙티브 기능 활용 가능
  - 이전 프로바이더 결과를 heredoc으로 다음 pane에 프롬프트 주입
  - 프로바이더 실패 시 skip-continue 처리 (SPEC-ORCH-004 REQ-3a 패턴 재사용)
  - `runner.go` relay pane fallback 경고 제거 — relay도 `RunPaneOrchestra`로 통합 라우팅
  - pane 라이프사이클 관리: 완료 후 defer로 모든 pane 및 임시 파일 정리
  - plain 터미널 환경에서는 기존 standard relay 실행으로 자동 fallback

- **Agent Teams Terminal Pane Visualization** (SPEC-TEAMPANE-001): `--team` 모드에서 팀원별 cmux/tmux 패널 분할 및 실시간 로그 스트리밍
  - `pkg/pipeline/team_monitor.go` — TeamMonitorSession: PipelineMonitor 인터페이스 구현, plain 터미널 graceful degradation
  - `pkg/pipeline/team_layout.go` — LayoutPlan: 순차적 Vertical split 전략, 3~5인 팀 지원
  - `pkg/pipeline/team_pane.go` — 팀원별 패널 생성/정리, tail -f 로그 스트리밍, shell-escape 보안
  - `pkg/pipeline/team_dashboard.go` — 폭 인식(width-aware) 대시보드 렌더링, compact 모드(< 38자)
  - `pkg/pipeline/monitor.go` — PipelineMonitor 인터페이스 추가 (MonitorSession + TeamMonitorSession 공통 계약)
  - SplitPane 실패 시 자동 cleanup 및 plain 터미널 폴백
  - tmux 지원 (개별 패널 닫기 미지원 제한사항 문서화)

- **Orchestra Agentic Relay Mode** (SPEC-ORCH-004): 프로바이더를 agentic one-shot 모드로 순차 실행하는 relay 전략
  - `pkg/orchestra/relay.go` — 릴레이 실행 로직, 프롬프트 주입, 결과 포맷팅
  - 프로바이더별 agentic 플래그 자동 매핑 (claude: `--allowedTools`, codex: `--approval-mode full-auto`)
  - 이전 프로바이더 분석 결과를 `## Previous Analysis by {provider}` 섹션으로 다음 프로바이더에 주입
  - 부분 실패 시 skip-continue 처리 (REQ-3a)
  - `--keep-relay-output` 플래그로 결과 파일 보존 옵션
  - `/tmp/autopus-relay-{jobID}/` 임시 디렉토리 관리

- **Orchestra Detach Mode** (SPEC-ORCH-003): pane 터미널(cmux/tmux) 감지 시 auto-detach 비동기 실행
  - `pkg/orchestra/job.go` — Job persistence model, status tracking, stale job GC
  - `pkg/orchestra/detach.go` — ShouldDetach() 판정, RunPaneOrchestraDetached() 진입점
  - `internal/cli/orchestra_job.go` — `auto orchestra status/wait/result` CLI 서브커맨드
  - `--no-detach` 플래그로 blocking 실행 강제 가능
  - REQ-11: 1시간 이상 된 abandoned job 자동 정리 (opportunistic GC)
