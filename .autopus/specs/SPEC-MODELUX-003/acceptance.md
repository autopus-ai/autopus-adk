# SPEC-MODELUX-003 수락 기준
## Test Scenarios

fixture는 모두 temp project 안에 만든다. 폼 test는 SPEC-MODELUX-002처럼 `composeFormsProbe`를 true로, `composeRunForm`을 huh accessible runner로 두고 입력을 `testing/iotest.OneByteReader`로 준다. "X를 고른다"는 그 라벨의 번호 입력이고 Enter는 기본값이다. OMP는 `ompCLIFakeRunner`(catalog·plan), `platformOMPModelRunner`에 `ompModelDoctorE2ERunner`(생성·`config get` 되읽기), `omp_model_activation_rpc_test.go`의 `RunWithInput` fake(RPC 되읽기), 호출을 세는 activate stub을 쓴다. transaction 쓰기 실패는 test 전용 `ompRootedWriteFailureHook`(nil 기본)으로 준다. 나머지는 실제 명령(`NewRootCmd`)과 실제 adapter `Generate`·`Update`다. 기준 바이너리는 `v0.50.125`(dd6d6ea2)로 만든 `auto`다. 실제 터미널은 S17·S23에서만 쓴다.

- D, O3, CAT-A, H, IO: SPEC-MODELUX-002 `acceptance.md`와 같다. 따로 적지 않으면 Codex catalog는 CAT-E다.
- CAT-E: `gpt-6-astra`·`gpt-6-luna`는 `low`–`max` 다섯 단계, `gpt-6.1-sol`은 `low`·`medium`·`high`·`xhigh`를 보고하는 `codex debug models` JSON.
- OP: O3 + `role_model_policy {version: v1, profile: balanced, family: openai}`. OPM: OP + `config_mode: project-managed`. OP2: OP + `agents.task` pin(`openai-codex/gpt-6-astra`, `max`). OP3: OP + `agents.sonic` pin. OPX: OP + `agents.executor` pin `openai-codex/gpt-6-astra:max`과 `agents.tester` pin `openai-codex/gpt-6.1-sol:xhigh`, planner pin 없음.
- ML: metadata 없는 OMP catalog `ompCLIProfileNativeCatalogJSON()`. MR: metadata가 있는 `ompCLIBalancedCatalogJSON()`. NO: `openai-codex` model을 뺀 OMP catalog.
- NM: v0.50.125 바이너리로 `auto init --yes --platforms claude-code,codex,omp`를 한 project. UF: 빈 dir에 manifest에 없는 `.omp/agents/reviewer.md`(`---\nname: reviewer\ndescription: mine\n---\nmine\n`). UC: O3 + `.omp/config.yml` = `task:\n  disabledAgents:\n    - browser\n`.
### S1: effort key 읽기와 거부
Priority: Must
Given D, D + `quality.providers.claude: claude-custom`과 claude-custom(16개 `sonnet`, `efforts: {planner: xhigh}`), 값만 `hihg`인 사본, key만 `not-an-agent`인 사본이 있다
When 네 파일을 `config.Load`로 읽고 D로 이 branch와 v0.50.125에서 Claude·Codex `Generate`를 같은 snapshot으로 돌린다
Then 둘째는 읽히고 `Efforts`가 `{planner: xhigh}`다. 셋째는 `quality.presets.claude-custom.efforts.planner: effort_invalid "hihg"`, 넷째는 `quality.presets.claude-custom.efforts.not-an-agent`를 담은 오류로 실패한다
And D의 `.claude/agents/autopus/*.md`와 `.codex/agents/*.toml` 32개는 v0.50.125 결과와 bytes가 같다
### S2: 명시 effort의 해석과 생성
Priority: Must
Given D + `quality.presets.balanced.efforts.planner: high`(B1)와, D + `quality.providers.claude: claude-custom`, claude-custom = executor `opus`·annotator `haiku`·나머지 D와 같음, `efforts: {executor: xhigh, annotator: high}`(B2)가 있다
When 두 파일로 `auto quality --apply`를 비대화형으로 실행한다
Then B1의 planner.md는 `model: fable`, `effort: high`, planner.toml은 `model = "gpt-6-astra"`, `model_reasoning_effort = "high"`이고 architect.md는 `model: claude-fable-5-1`, `effort: max` 그대로다
And B2의 executor.md는 `model: opus`, `effort: xhigh`, annotator.md는 `model: haiku`이고 `effort:` 줄이 없다
### S3: Claude Code 탭 effort
Priority: Must
Given D가 있다
When `auto quality --apply`에서 구성 편집 → Claude Code → 역할 그룹 → 설계·판단 `opus-5-5 · high  [opus]` → effort `opus-5-5 · xhigh` → 나머지 그룹의 등급·effort Enter → 적용 → 유지를 고른다
Then 설계·판단 effort Select는 `그대로 — opus-5-5 · high`, `opus-5-5 · low`, `opus-5-5 · medium`, `opus-5-5 · high (자동)`, `opus-5-5 · xhigh`, `opus-5-5 · max` 순서다
And 미리보기의 네 에이전트 Claude 칸은 `opus · xhigh*`이고 바뀌는 설정은 `quality.presets.claude-custom 새로 씀 (16개 에이전트)`, `quality.presets.claude-custom.efforts 새로 씀 (4개 에이전트)`, `quality.providers.claude 없음 → claude-custom`, `claude-custom은 balanced 표준 배치를 받지 않음`, 블록 재작성 안내, `quality.providers는 v0.50.88보다 오래된 auto가 읽지 않습니다`, `quality.presets.*.efforts는 v0.50.126보다 오래된 auto가 읽지 못합니다`다
And 적용 뒤 claude-custom efforts는 architect, planner, security-auditor, spec-writer `xhigh` 넷뿐이고 planner.md는 `model: opus`, `effort: xhigh`, `.codex/agents/planner.toml`은 bytes가 같다
### S4: Codex 탭 effort는 catalog 단계뿐이다
Priority: Must
Given D와 CAT-E, catalog probe가 오류를 내는 D 사본이 있다
When Codex → 역할 그룹 → 설계·판단 `6.1-sol · xhigh  [opus]` → effort `6.1-sol · high` → 나머지 Enter → 적용 → 유지를 고르고 사본에서도 같은 등급까지 고른다
Then effort Select는 `그대로 — 6.1-sol · xhigh`, `6.1-sol · low`, `6.1-sol · medium`, `6.1-sol · high`, `6.1-sol · xhigh (자동)`이고 `max`가 없다
And 적용 뒤 codex-custom efforts는 네 에이전트 `high`, planner.toml은 `model = "gpt-6.1-sol"`, `model_reasoning_effort = "high"`, Claude planner.md는 bytes가 같다. 사본은 `Codex catalog를 읽지 못해 effort는 자동입니다`를 쓰고 effort Select를 열지 않는다
### S5: 그대로, 자동으로 되돌리기
Priority: Must
Given S4가 끝난 파일이 있다
When (1) Codex → 역할 그룹 → 모든 Select Enter. (2) Codex → 에이전트별 → executor → `6.1-sol · xhigh  [opus]` → effort Enter → 완료 → 적용. (3) Codex → 에이전트별 → planner → 등급 Enter → `6.1-sol · xhigh (자동)` → 완료 → 적용
Then (1)은 `변경 없음`과 같은 SHA-256, (2) 뒤 efforts는 넷 그대로이고 efforts 줄·옛 바이너리 줄이 없다
And (3) 뒤 efforts는 architect, security-auditor, spec-writer 셋이고 `quality.presets.codex-custom.efforts 고쳐 씀 (3개 에이전트)` 줄이 있으며 planner.toml은 `model_reasoning_effort = "xhigh"`다
### S6: agent_source 값
Priority: Must
Given O3에서 `agent_source`가 없음·`autopus`·`native`·`nativ`인 파일과 `role_model_policy: {agent_source: native}`만 있는 사본이 있다
When 각각 `config.Load` 뒤 `auto update --yes`를 실행한다
Then `nativ`는 `role_model_policy.agent_source_invalid: "nativ"`로 실패하고, 없음·`autopus`는 `.omp/agents` 16개, `native`는 0개와 `.omp/config.yml` 없음을 남긴다
And 마지막 사본은 읽히고 OMP model integration이 돌지 않는다(runner `models` 호출 0회)
### S7: OMP 기본 생성
Priority: Must
Given O3와 OP(IO 생성 runner)가 있다
When 각각 `auto update --yes`를 두 번 실행한다
Then O3의 `.omp/agents` 이름은 정규 16개와 같고 planner.md는 `name: planner`, content description, `thinkingLevel: max`, 정렬된 `tools`이며 `model` key가 없다. executor는 `thinkingLevel: medium`이고 tools에 `lsp`가 있다
And reviewer.md의 `output`은 `testdata/omp-18.6.1-reviewer-output.yaml`과 deep-equal이고, OP에서 override가 이름을 담은 에이전트의 파일에는 `thinkingLevel` 줄이 없다
And OMP manifest에 `.omp/agents/` 16개, `.omp/config.yml`은 0600이고 `task.disabledAgents: [scout, security-reviewer, sonic]`, ledger가 그 key를 `agent_surface`로 담는다. 두 번째 실행 뒤 OMP 파일 SHA-256이 같다
### S8: 사용자 파일과 사용자 소유 설정
Priority: Must
Given UF, UC, S7이 끝난 뒤 사용자가 `.omp/agents/executor.md` 끝에 한 줄을 더한 project가 있다
When UF에서 `auto init --yes --platforms claude-code,codex,omp`(Generate) 뒤 `auto update --yes`(Update), UC와 셋째에서 `auto update --yes`, UC에서 `auto doctor`를 실행한다
Then UF의 reviewer.md는 두 실행 뒤 모두 SHA-256이 같고 생성 파일은 15개, 출력에 `.omp/agents/reviewer.md는 사용자 파일이라 그대로 둡니다`가 있으며 manifest에 reviewer 항목이 없다
And UC의 `.omp/config.yml`은 SHA-256이 같고 출력에 `task.disabledAgents를 사용자가 정해 두어` 줄, update 종료 코드 0, doctor 상태 `degraded`와 blocker `agents:disabled_agents_user_owned`가 있다
And 셋째의 `.autopus/backup/<id>/transaction/omp/.omp/agents/executor.md`는 사용자가 고친 bytes이고 `.omp/agents/executor.md`는 생성 내용이다
### S9: Autopus 이름 route와 되읽기
Priority: Must
Given OP, OP2, OP3과 IO 생성 runner, `RunWithInput` fake가 있다
When 각각 `auto update --yes`를 실행해 같은 run의 plan 미리보기를 기록하고, adapter test에서 OP를 `RunWithInput` fake로 활성화하고 한 번은 fake가 `planner` selector를 다르게 답하게 한다
Then OP의 되읽기는 `config get task.agentModelOverrides --json` 한 번이고 그 record key는 정규 16개와 `task`(17개)이며 scout, security-reviewer, sonic이 없다. 각 값은 미리보기의 effective `<selector>:<thinking>`과 같다
And `RunWithInput` fake로는 RPC 세션이 17번 열리고 각 selector가 override 값과 같다. 다르게 답한 run은 활성화 전체가 실패하고 `.omp`, overlay, ledger bytes가 실행 전과 같다
And OP2의 annotator, architect, debugger, deep-worker, devops, executor, frontend-specialist, perf-engineer, planner, spec-writer, tester, ux-validator, validator와 `task`는 `openai-codex/gpt-6-astra:max`, explorer·reviewer·security-auditor는 OP와 같다. OP3 미리보기에는 `role_model_policy.agents.sonic`을 담은 경고 1줄이 있고 `sonic` key가 없다
### S10: OMP 탭 역할 그룹
Priority: Must
Given OP, activate stub, ML과 MR catalog가 있다
When `auto quality`에서 구성 편집 → OMP → 에이전트별 지정 (balanced 위에) → GPT → 역할 그룹 → 설계·판단 model `openai-codex/gpt-6-astra` → thinking `max` → 나머지 Enter → 적용 → 유지를 ML과 MR로 각각 고른다
Then ML의 model Select는 기존 표시용 catalog 안내 뒤 `프로필 기본값`과 설치 model 전부를, MR은 `프로필 기본값`과 네 에이전트의 capability를 모두 선언한 model만 보인다
And 두 경우 모두 적용 뒤 `role_model_policy.agents` key는 planner, architect, spec-writer, security-auditor이고 각 candidates는 selector `openai-codex/gpt-6-astra`, thinking `max`, family `openai` 하나이며 activate는 1회다
### S11: OMP pre-plan과 thinking만 바꾸기
Priority: Must
Given OP·MR과 NO catalog의 OP 사본이 있다
When OP에서 OMP → 에이전트별 지정 → GPT → 에이전트별 → planner → model `프로필 기본값` → thinking에서 기본값이 아닌 첫 단계 L → 완료 → 적용을 고르고, 사본에서 OMP → 에이전트별 지정 → GPT를 고른다
Then 방식 Select 전에 plan이 한 번 돌고 thinking Select 첫 선택지는 `프로필 기본값 (<t>)`다(`<t>`는 그 pre-plan의 planner thinking). 적용 뒤 `agents.planner.candidates[0]`은 pre-plan의 planner effective selector와 L이고 다른 key가 없다
And 사본은 pre-plan blocker의 오류 줄을 쓰고 방식 Select 없이 메뉴로 돌아가며 activate 0회, SHA-256이 같다
### S12: update migration
Priority: Must
Given NM과 그 autopus.yaml의 SHA-256이 있다
When 이 branch로 `auto update --yes`를 두 번 실행한다
Then 첫 미리보기에 `+ .omp/agents/planner.md`를 포함한 16줄, `task.disabledAgents: scout, security-reviewer, sonic`, `OMP는 이제 Autopus 에이전트 16개를 씁니다.`로 시작하는 줄이 있다
And 끝난 뒤 16개 파일과 claim이 있고 autopus.yaml SHA-256이 같으며 두 번째 실행은 OMP 파일을 바꾸지 않는다
### S13: native로 되돌리고 다시 기본으로
Priority: Must
Given S12가 끝난 project, 그 OMP 파일 SHA-256 목록, v0.50.125 바이너리가 같은 project의 S12 시작 전 파일(`agent_source` 줄 없음)로 만든 OMP 파일 목록과 bytes가 있다
When autopus.yaml에 `role_model_policy:\n  agent_source: native`를 손으로 더해 `auto update --yes`, 그 줄을 지우고 다시 `auto update --yes`를 실행한다
Then native 뒤 `.omp/agents`에 생성 파일이 없고 `.omp/config.yml`과 ledger가 없으며 OMP 파일 목록과 bytes는 v0.50.125 기준과 같다
And 되돌린 뒤 OMP 파일 SHA-256 목록은 S12 끝과 같다
### S14: 렌더 mode는 실제 생성 산출물에서 맞다
Priority: Must
Given O3, O3 + `agent_source: native`, `Agent(subagent_type="executor", prompt="a")`·`explorer`·`reviewer`를 담은 본문이 있다
When 두 project에서 OMP `Adapter.Generate`를 돌리고, 본문을 `ReplacePlatformReferencesWith(body, "omp", autopus)`와 native로 렌더한다
Then autopus 산출물의 `.omp/` 아래 어느 파일에도 `"agent": "scout"`, `"agent": "security-reviewer"`, `"agent": "sonic"`, `There is no Autopus-specific agent`가 없고, explorer dispatch는 `"agent": "explorer"`, executor는 `"agent": "executor"`다
And native 산출물은 v0.50.125 바이너리 결과와 bytes가 같고, 본문 렌더는 autopus에서 세 이름을 그대로, native에서 executor는 agent 줄 없이·explorer는 `"agent": "scout"`로 쓴다
### S15: doctor catalog와 receipt
Priority: Must
Given S7 project, 거기서 validator.md를 지운 사본, native O3에 `.omp/agents/reviewer.md`를 둔 project, OP의 receipt가 있다
When OMP doctor projection과 pipeline backend의 OMP model 해석을 실행한다
Then S7은 `Expected` 17, `Shadowed` 0, ready다. 사본은 degraded와 `agents:autopus_agent_missing`, native project는 `agents:native_agent_shadowed`다
And OP의 phase model은 plan ← `planner`, test scaffold ← `tester`, implement ← `executor`, validate ← `validator`, review ← `reviewer` 행이고, `executor` 행을 지우면 `executor`를 담은 오류로 실패한다
### S16: 회귀와 안전
Priority: Must
Given 이 branch, v0.50.125의 test, SPEC-MODELUX-002 S1의 줄 입력 golden이 있다
When `go test ./pkg/... ./internal/cli/...`를 필터 없이 돌리고 `git diff --stat v0.50.125 -- '*_test.go'`를 T1 분류와 맞추고, `--yes`·`CI=true` update의 stdin Read를 센다
Then 모든 test가 통과하고 바뀐 기존 test는 T1 목록과 규칙(기본값 전환 단언, native fixture·option, 폼 입력 Enter)대로다
And 줄 입력 golden은 bytes가 같고, effort를 고르지 않은 생성 결과는 v0.50.125와 같으며, 비대화형 stdin Read는 0회다
### S17: 실제 터미널 smoke
Priority: Must
Given macOS 실제 터미널, 이 branch의 `auto`, omp 18.6.1, `git init`한 빈 temp dir, scratch `CODEX_HOME`, `script -q` 기록이 있다
When `auto init --platforms claude-code,codex,omp`에서 역할별 직접 구성 → Claude Code → 역할 그룹 → 설계·판단 opus → effort xhigh → 나머지 Enter → 적용 → 유지를 고르고, `auto quality`에서 OMP → balanced → GPT → 적용 → 유지를 고른다. plain `omp`를 열어 메시지 없이 `/agents`만 보고, `PI_CONFIG_FILES=$PWD/.autopus/runtime/omp-model-routing-v1.yml omp`로 다시 열어 `/agents`를 본다
Then planner.md는 `model: opus`, `effort: xhigh`이고 `.omp/agents` 16개, `.omp/config.yml`에 `disabledAgents`가 있다
And plain `/agents`에 Autopus 16개와 task가 있고 scout, security-reviewer, sonic은 disabled이며 reviewer 설명이 content의 description이다. overlay를 준 `/agents`에서 planner의 model 칸이 overlay override 값이다(blocker면 그 출력을 적고 CD-1을 남긴다)
And transcript를 `$HOME`, scratch 경로, 사용자 이름을 치환해 `evidence/manual-smoke-003.txt`에 남긴다
### S18: locale
Priority: Should
Given D가 있다
When `composeLanguage`를 `en`과 `ko`로 두고 S4의 effort Select를 연다
Then en은 `keep — 6.1-sol · xhigh`와 `(automatic)`, ko는 `그대로 — 6.1-sol · xhigh`와 `(자동)`을 쓰고 저장 값은 같다
### S19: 문서
Priority: Should
Given help, README, CHANGELOG가 있다
When 세 명령의 `--help`를 실행하고 문서를 읽는다
Then effort 편집, `efforts`의 옛 바이너리 한계, OMP Autopus 기본값, `agent_source: native`와 v0.50.127 제거, migration이 있고 `quality_compose_docs_test.go`가 고정한 문장이 그대로이며 `make generate-templates` 뒤 반영 안 된 diff가 없다
### S20: efforts 보존
Priority: Must
Given S4가 끝난 파일, D + `quality.default: custom`과 custom(16개 등급, `efforts: {executor: xhigh}`)인 파일, H의 balanced에 `efforts: {planner: high}`를 더한 파일, D의 balanced에 `efforts: {planner: high}`를 더하고 `custom`이 없는 파일(넷째), `quality.default: custom`이고 custom = D의 balanced 기본 등급에서 구현 그룹만 haiku이며 `efforts: {executor: xhigh}`인 파일(다섯째)이 있다
When 첫 파일에서 Claude Code 탭 설계·판단 opus를 적용하고, 둘째에서 모든 도구 같게 → 역할 그룹 → 설계·판단 opus → 적용, 셋째에서 최신 기본값으로 갱신 → 적용, 넷째에서 모든 도구 같게 → 역할 그룹 → 구현 haiku → 나머지 Enter → 적용을 고른다. 다섯째에서 모든 도구 같게 → 역할 그룹 → 구현을 balanced 기본 등급으로 → 나머지 Enter → 적용, 셋째 사본에서 모든 도구 같게 → 에이전트별로 H와 최신 balanced가 다른 에이전트만 최신 등급으로 → 완료 → 적용을 고른다. 또 `composeBindTarget`을 efforts가 있는 P(`custom`)와 Current·Historical·Absent로 분류되는 built-in 결과로 직접 부른다
Then 첫 파일의 codex-custom efforts는 네 에이전트 `high` 그대로이고 planner.toml은 `high` 그대로다
And 둘째의 미리보기에서 executor 칸에 `*`가 없고 적용 뒤 custom efforts는 `{executor: xhigh}`, executor.md는 `effort: xhigh`다. 셋째는 갱신 뒤 balanced가 최신 등급이고 efforts는 `{planner: high}`이며 planner.md는 `effort: high`다
And 넷째는 `custom`을 새로 만들고 그 efforts는 `{planner: high}`이며 바뀌는 설정에 `quality.presets.custom.efforts 옮겨 씀 (1개 에이전트)`가 있다. 미리보기의 planner 칸에는 `*`가 없고 planner.md는 `model: fable`, `effort: high`, planner.toml은 `model = "gpt-6-astra"`, `model_reasoning_effort = "high"`로 실행 전과 같다
And 다섯째는 결과 등급이 balanced 기본과 같아도 `quality.default: custom`에 머물고 custom 등급은 balanced 기본, efforts는 `{executor: xhigh}`이며 executor.md는 `effort: xhigh`이고 `quality.presets.balanced` bytes는 실행 전과 같다. 셋째 사본은 balanced를 Historical 갱신으로 최신 등급에 맞추고 efforts `{planner: high}`를 남기며 `custom`을 만들지 않는다. 직접 호출은 세 분류 모두 `custom`과 P의 efforts를 돌려준다
### S21: task route와 degraded 판정
Priority: Must
Given OPX, OPX + `agents.task` pin(`openai-codex/gpt-6-astra:high`)인 사본, OP와 OPX를 NO catalog로 돌리는 사본이 있다
When 각각 `config.Load` 뒤 `auto update --yes`를 실행한다
Then OPX는 `omp_native_agent_conflict` 없이 읽히고 override는 executor `openai-codex/gpt-6-astra:max`, tester `openai-codex/gpt-6.1-sol:xhigh`, task와 planner는 프로필 planner route 값이다
And task pin 사본은 자기 pin이 없는 11개 task 접힘 역할(annotator, architect, debugger, deep-worker, devops, frontend-specialist, perf-engineer, planner, spec-writer, ux-validator, validator)과 task, 모두 12개 key가 `openai-codex/gpt-6-astra:high`이고 executor·tester는 자기 pin이다
And NO의 OP는 성공하고 openai 파생 route 에이전트는 override에 없으며 그 파일에 `thinkingLevel`이 있다. NO의 OPX는 `required_route_unresolved`와 `executor`를 담은 오류로 실패하고 OMP 파일 bytes가 실행 전과 같다
### S22: claim 집합 전환과 복원
Priority: Must
Given `.omp/config.yml`이 없는 O3가 있다
When 차례로 (a) `auto update --yes`, (b) OPM으로 바꿔 update, (c) `config_mode: overlay`로 바꿔 update, (d) `.omp/config.yml` 첫 줄에 `# team note`를 더해 update, (e) autopus.yaml을 OPM으로 바꿔(프로필 key가 더해지고 route 에이전트의 `thinkingLevel`이 빠지는 실제 변경) `ompRootedWriteFailureHook`이 plan의 마지막 write에서 실패하게 한 채 update하고 autopus.yaml을 (d)의 내용으로 되돌린 뒤, (f) seam을 끄고 `agent_source: native`로 update, 별도 (a) 상태 사본에서 `auto clean`을 실행한다
Then (a) 파일은 `task.disabledAgents`뿐이고 ledger preimage는 missing이다. (b) 파일은 `task.disabledAgents`와 프로필 key(`task.agentModelOverrides` 17개 key 포함)이고 ledger는 두 owner를 담는다. (c) 파일은 다시 `task.disabledAgents`뿐이다
And (d) 뒤 파일은 `# team note`와 `task.disabledAgents`를 담고 ledger preimage가 `# team note`를 담는다. (e)는 오류로 끝나고 transaction journal에 실패 전에 끝난 write가 1개 이상 있으며(실제 transaction이 열렸다), `.omp/config.yml`, ledger, manifest, `.omp/agents`, overlay 파일 bytes가 (d) 뒤와 같다
And (f) 뒤 `.omp/config.yml`은 `# team note`만 담고 ledger가 없으며, clean 사본은 `.omp/config.yml`과 ledger가 없다
### S23: 동의 뒤 live dispatch와 `/review`
Priority: Must
Given S17 project와 사용자의 모델 호출 동의, OMP 탭에서 planner thinking을 `low`로 pin한 상태가 있다
When `PI_CONFIG_FILES`를 준 `omp`에서 planner에게 한 줄 작업을 보내고, 작은 diff에 `/review`를 한 번 실행한다
Then planner subagent의 model과 thinking이 override `<selector>:low`와 같고, `/review` 출력에 `Patch is correct` 또는 `Patch is incorrect`와 confidence가 있다
And 결과를 `evidence/manual-smoke-003.txt`에 남긴다. 동의가 없거나 막히면 PASS로 닫지 않고 CD-2를 남긴다
### S24: 프로필과 섞인 상태의 사용자 `task.disabledAgents` 편집
Priority: Must
Given O3를 OPM으로 `auto update --yes`한 project(파일에 `task.disabledAgents`와 프로필 key)와 그 사본 두 개가 있다
When 첫 project에서 `task.disabledAgents`를 `[scout]`로 고쳐 `auto update --yes`와 `auto doctor`를 실행하고, 이어 `config_mode: overlay`로 바꿔 update, 다시 값을 `[scout, security-reviewer, sonic]`로 되돌려 update한다. 첫 사본에서는 그 key를 지우고 update한 뒤 `config_mode: overlay`로 바꿔 update, 다시 update 두 번, `auto clean`을 차례로 실행하고, 둘째 사본에서는 그 key를 고치면서 첫 줄에 `# team note`도 더해 update한다
Then 첫 update는 종료 코드 0이고 `.omp/config.yml` SHA-256이 사용자가 고친 뒤와 같으며 `task.agentModelOverrides`는 17개 key 그대로다. ledger는 `agent_surface_user_owned`를 담고 출력에 `task.disabledAgents를 사용자가 정해 두어` 줄, doctor에 `agents:disabled_agents_user_owned`가 있다
And overlay 전환 뒤 파일은 preimage와 `task.disabledAgents: [scout]`이고 프로필 key가 없다. 값을 되돌린 update 뒤 ledger에 사용자 소유 표시가 없고 그 key의 owner가 `agent_surface`다
And 첫 사본은 첫 update 뒤 파일에 `task.disabledAgents`가 없고 SHA-256이 사용자가 지운 뒤와 같다. overlay 전환 update 뒤 `.omp/config.yml`은 없고(ADK가 만든 파일) ledger는 사용자 소유 표시만 담으며, 그 뒤 두 번의 update 뒤에도 `.omp/config.yml`이 없고 ledger bytes가 같다. `auto clean` 뒤 ledger가 없다. 둘째 사본은 `managed_key_conflict`로 끝나고 `.omp/config.yml`과 ledger bytes가 실행 전과 같다
## Oracle Acceptance Notes

- Must 시나리오는 concrete 값을 판정한다: 선택지 목록과 순서, 라벨, 바뀌는 설정 줄, frontmatter·TOML 값, override key 집합과 값, 실행 횟수, SHA-256, 오류 문구. 파일 존재나 종료 코드만으로 닫는 Must는 없다.
- S9·S11·S21의 selector 값은 catalog fixture에 따라 정해지므로 같은 run의 plan 미리보기와 일치로 판정한다. 둘 다 결정적 출력이다.
- reviewer 정합성은 S7 golden이, 런타임 verdict는 S23이 닫는다. S17·S23이 기록되기 전에는 `research.md`의 CD-1·CD-2가 남는다.
- 기대 값은 2026-10-13에 v0.50.125 코드와 문서에서 계산했다. 실행으로 확인한 것은 probe P0·P3뿐이다. 코드 사실과 다르면 test가 아니라 이 SPEC을 고친다.
