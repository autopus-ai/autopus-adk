# SPEC-MODELUX-003 수락 기준
## Test Scenarios

fixture는 모두 temp project 안에 만든다. 폼 test는 SPEC-MODELUX-002처럼 `composeFormsProbe`를 true로, `composeRunForm`을 huh accessible runner로 두고 입력을 `testing/iotest.OneByteReader`로 준다. "X를 고른다"는 그 라벨의 번호를 입력한다는 뜻이고 Enter는 기본값이다. OMP는 `ompCLIFakeRunner`(catalog·plan), `platformOMPModelRunner`에 `ompModelDoctorE2ERunner`(생성·되읽기), 호출을 세는 activate stub을 쓴다. 나머지는 실제 명령(`NewRootCmd`)과 실제 adapter `Generate`·`Update`다. 실제 터미널은 S17에서만 쓴다.

- D, O3, CAT-A, IO: SPEC-MODELUX-002 `acceptance.md`와 같다. 따로 적지 않으면 Codex catalog는 CAT-E다.
- CAT-E: `gpt-6-astra`와 `gpt-6-luna`는 `low`·`medium`·`high`·`xhigh`·`max`, `gpt-6.1-sol`은 `low`·`medium`·`high`·`xhigh`를 보고하는 `codex debug models` JSON(`default_reasoning_level`은 모두 `medium`).
- OP: O3에 `role_model_policy {version: v1, profile: balanced, family: openai}`를 더한 파일. OP2: OP에 옛 native pin `agents.task.candidates: [{selector: openai-codex/gpt-6-astra, thinking: max, family: openai}]`. OP3: OP에 `agents.sonic` pin.
- NM: HEAD 93dbb564 바이너리로 `auto init --yes --platforms claude-code,codex,omp`를 한 project(OMP manifest에 `.omp/agents` 없음).
- UF: O3에 manifest에 없는 `.omp/agents/reviewer.md`(내용 `---\nname: reviewer\ndescription: mine\n---\nmine\n`). UC: O3에 `.omp/config.yml` = `task:\n  disabledAgents:\n    - browser\n`.
### S1: effort key 읽기와 거부
Priority: Must
Given D, D에 `quality.providers.claude: claude-custom`과 `quality.presets.claude-custom`(16개 `sonnet`, `efforts: {planner: xhigh}`)을 더한 파일, 같은 파일에서 값만 `hihg`로 바꾼 것, key만 `not-an-agent`로 바꾼 것이 있다
When 네 파일을 `config.Load`로 읽고, D로 이 branch와 HEAD 93dbb564에서 Claude·Codex `Generate`를 같은 snapshot으로 돌린다
Then 둘째 파일은 읽히고 `QualityPreset.Efforts`가 `{planner: xhigh}`다
And 셋째는 `quality.presets.claude-custom.efforts.planner: effort_invalid "hihg"`를, 넷째는 `quality.presets.claude-custom.efforts.not-an-agent`를 담은 오류로 실패한다
And D의 `.claude/agents/autopus/*.md` 16개와 `.codex/agents/*.toml` 16개는 HEAD 93dbb564 결과와 bytes가 같다
### S2: 명시 effort의 해석과 생성
Priority: Must
Given D에 손으로 `quality.presets.balanced.efforts.planner: high`를 넣은 파일(B1)과, D에 `quality.providers.claude: claude-custom`, claude-custom = 등급 executor `opus`·annotator `haiku`·나머지 D와 같음, `efforts: {executor: xhigh, annotator: high}`인 파일(B2)이 있다
When 두 파일로 `auto quality --apply`를 비대화형으로 실행한다
Then B1의 `.claude/agents/autopus/planner.md`는 `model: fable`, `effort: high`이고 `.codex/agents/planner.toml`은 `model = "gpt-6-astra"`, `model_reasoning_effort = "high"`다. B1의 architect.md는 `model: claude-fable-5-1`, `effort: max` 그대로다(표준 배치 유지)
And B2의 executor.md는 `model: opus`, `effort: xhigh`이고 annotator.md는 `model: haiku`이며 `effort:` 줄이 없다
### S3: Claude Code 탭 effort 선택과 저장
Priority: Must
Given D가 있다
When `auto quality --apply`에서 구성 편집 → Claude Code → 역할 그룹 → 설계·판단 `opus-5-5 · high  [opus]` → effort `opus-5-5 · xhigh` → 나머지 그룹의 등급과 effort는 Enter → 적용 → 유지를 고른다
Then 설계·판단 effort Select는 차례로 `그대로 — opus-5-5 · high`, `opus-5-5 · low`, `opus-5-5 · medium`, `opus-5-5 · high (자동)`, `opus-5-5 · xhigh`, `opus-5-5 · max`다
And 미리보기의 planner, architect, spec-writer, security-auditor Claude 칸은 `opus · xhigh*`이고, 바뀌는 설정은 `quality.presets.claude-custom 새로 씀 (16개 에이전트)`, `quality.presets.claude-custom.efforts 새로 씀 (4개 에이전트)`, `quality.providers.claude 없음 → claude-custom`, `claude-custom은 balanced 표준 배치를 받지 않음`, 블록 재작성 안내, `quality.providers는 v0.50.88보다 오래된 auto가 읽지 않습니다`, `quality.presets.*.efforts는 v0.50.125보다 오래된 auto가 읽지 못합니다`다
And 적용 뒤 `quality.presets.claude-custom.efforts`는 architect, planner, security-auditor, spec-writer `xhigh` 넷뿐이다. 생성된 planner.md는 `model: opus`, `effort: xhigh`이고 `.codex/agents/planner.toml`은 실행 전과 bytes가 같다
### S4: Codex 탭 effort 선택은 catalog가 허용한 단계뿐이다
Priority: Must
Given D와 CAT-E, catalog probe가 오류를 내는 D 사본이 있다
When `auto quality --apply`에서 구성 편집 → Codex → 역할 그룹 → 설계·판단 `6.1-sol · xhigh  [opus]` → effort `6.1-sol · high` → 나머지 Enter → 적용 → 유지를 고른다. 사본에서도 같은 등급까지 고른다
Then effort Select는 `그대로 — 6.1-sol · xhigh`, `6.1-sol · low`, `6.1-sol · medium`, `6.1-sol · high`, `6.1-sol · xhigh (자동)`이고 `max`가 없다
And 적용 뒤 `quality.presets.codex-custom.efforts`는 architect, planner, security-auditor, spec-writer `high`이고, `planner.toml`은 `model = "gpt-6.1-sol"`, `model_reasoning_effort = "high"`, `.claude/agents/autopus/planner.md`는 실행 전과 bytes가 같다
And 사본은 `Codex catalog를 읽지 못해 effort는 자동입니다`를 쓰고 effort Select를 한 번도 열지 않는다
### S5: 그대로, 자동으로 되돌리기, 다른 편집에서 effort 보존
Priority: Must
Given S4가 끝난 파일이 있다
When (1) Codex → 역할 그룹 → 모든 Select에 Enter. (2) Codex → 에이전트별 → executor → `6.1-sol · xhigh  [opus]` → effort Enter → 완료 → 적용. (3) Codex → 에이전트별 → planner → 등급 Enter → effort `6.1-sol · xhigh (자동)` → 완료 → 적용
Then (1)은 `변경 없음`을 쓰고 SHA-256이 같다
And (2) 뒤 efforts는 넷 그대로이고, 바뀌는 설정에 efforts 줄과 옛 바이너리 줄이 없다
And (3) 뒤 efforts는 architect, security-auditor, spec-writer 셋이고 `quality.presets.codex-custom.efforts 고쳐 씀 (3개 에이전트)` 줄이 있으며 `planner.toml`은 `model_reasoning_effort = "xhigh"`다
### S6: agent_source 값
Priority: Must
Given O3에서 `agent_source`가 없는 것, `autopus`, `native`, `nativ`인 것, 그리고 `role_model_policy: {agent_source: native}`만 있는 O3 사본이 있다
When 각각 `config.Load` 뒤 `auto update --yes`를 실행한다
Then `nativ`는 `role_model_policy.agent_source_invalid: "nativ"`로 실패한다
And 없음과 `autopus`는 `.omp/agents`에 16개 파일을, `native`는 0개 파일과 `.omp/config.yml` 없음을 남긴다
And 마지막 사본은 읽히고 OMP model integration이 돌지 않는다(runner의 `models` 호출 0회)
### S7: OMP 기본 생성
Priority: Must
Given O3가 있다
When `auto update --yes`를 두 번 실행한다
Then `.omp/agents`의 파일 이름은 정규 16개와 정확히 같다. planner.md frontmatter는 `name: planner`, content의 description, `thinkingLevel: max`, 정렬된 `tools`이고 `model` key가 없다. executor는 `thinkingLevel: medium`이고 tools에 `lsp`가 있다
And reviewer.md는 `thinkingLevel: high`이고 `output.properties.overall_correctness.enum`이 `[correct, incorrect]`, `explanation`·`confidence`가 있고 `optionalProperties.findings`가 있다
And OMP manifest에 `.omp/agents/` 항목이 16개다. `.omp/config.yml`은 mode 0600이고 decode하면 `task.disabledAgents`가 `[scout, security-reviewer, sonic]`이며 ownership ledger가 그 key를 담는다
And 두 번째 실행 뒤 OMP 생성 파일과 `.omp/config.yml`의 SHA-256이 첫 실행 뒤와 같다
### S8: 사용자 파일과 사용자 소유 설정
Priority: Must
Given UF, UC, 그리고 S7이 끝난 뒤 사용자가 `.omp/agents/executor.md` 끝에 한 줄을 더한 project가 있다
When 각각 `auto update --yes`를 실행하고 UC에서 `auto doctor`를 실행한다
Then UF의 reviewer.md는 SHA-256이 같고, 생성 파일은 15개이며, 출력에 `.omp/agents/reviewer.md는 사용자 파일이라 그대로 둡니다`가 있고 manifest에 reviewer 항목이 없다
And UC의 `.omp/config.yml`은 SHA-256이 같고 출력에 `task.disabledAgents를 사용자가 정해 두어` 줄이 있으며, update는 종료 코드 0, doctor의 OMP 상태는 `degraded`이고 blocker에 `agents:disabled_agents_user_owned`가 있다
And 셋째 project의 `.autopus/backup/<ts>/.omp/agents/executor.md`는 사용자가 고친 bytes이고 `.omp/agents/executor.md`는 생성 내용이다
### S9: Autopus 이름 route와 옛 native pin
Priority: Must
Given OP, OP2, OP3과 IO의 생성 runner가 있다
When 각각 `auto update --yes`를 실행하고 같은 run의 `planOMPProfile` 미리보기를 기록한다
Then OP가 활성화한 `task.agentModelOverrides`의 key는 정규 16개와 `task`(17개)이고 scout, security-reviewer, sonic은 없다. 각 값은 미리보기에서 그 에이전트의 effective `<selector>:<thinking>`과 같다. `--mode rpc` 되읽기 실행은 17회다. `.omp/agents/*.md`에는 여전히 `model`이 없다
And OP2에서 annotator, architect, debugger, deep-worker, devops, executor, frontend-specialist, perf-engineer, planner, spec-writer, tester, ux-validator, validator와 `task`의 값은 `openai-codex/gpt-6-astra:max`이고 explorer, reviewer, security-auditor는 OP와 같다. 출력에 `omp_native_agent_conflict`가 없다
And OP3의 미리보기에 `role_model_policy.agents.sonic`을 담은 경고 1줄이 있고 override에 `sonic` key가 없다
### S10: OMP 탭 역할 그룹 지정
Priority: Must
Given OP와 OMP runner `ompCLIFakeRunner{catalog: ompCLIBalancedCatalogJSON()}`, activate stub이 있다
When `auto quality`에서 구성 편집 → OMP → 에이전트별 지정 (balanced 위에) → GPT → 역할 그룹 → 설계·판단 model `openai-codex/gpt-6-astra` → thinking `max` → 나머지 그룹 Enter → 적용 → 유지를 고른다
Then 설계·판단 model Select는 `프로필 기본값`으로 시작하고, 나머지 선택지는 planner, architect, spec-writer, security-auditor의 capability를 모두 선언한 설치 model뿐이다
And 적용 뒤 `role_model_policy.agents`의 key는 planner, architect, spec-writer, security-auditor이고 각 candidates는 selector `openai-codex/gpt-6-astra`, thinking `max`, family `openai` 하나다. native 이름 key는 쓰지 않는다. activate는 1회다
### S11: OMP thinking만 바꾸기
Priority: Must
Given S10과 같은 조건이 있다
When OMP → 에이전트별 지정 (balanced 위에) → GPT → 에이전트별 → planner → model `프로필 기본값` → thinking에서 기본값이 아닌 첫 단계 L → 완료 → 적용을 고른다
Then thinking Select의 첫 선택지는 `프로필 기본값 (<t>)`이고 `<t>`는 같은 run의 plan에서 planner의 thinking이다
And 적용 뒤 `agents.planner.candidates[0]`의 selector는 편집 전 plan의 planner effective selector와 같고 thinking은 L이다. `agents`에 다른 key는 없다
### S12: update migration
Priority: Must
Given NM과 그 autopus.yaml의 SHA-256이 있다
When 이 branch로 `auto update --yes`를 두 번 실행한다
Then 첫 출력의 미리보기에 `+ .omp/agents/planner.md`를 포함한 `.omp/agents` 16줄, `task.disabledAgents: scout, security-reviewer, sonic`, `OMP는 이제 Autopus 에이전트 16개를 씁니다.`로 시작하는 줄이 있다
And 끝난 뒤 16개 파일과 claim이 있고 autopus.yaml SHA-256은 실행 전과 같다. 두 번째 실행은 OMP 파일을 바꾸지 않는다
### S13: native로 되돌리고 다시 기본으로
Priority: Must
Given S12가 끝난 project와 그 OMP 파일 SHA-256 목록, 같은 config로 HEAD 93dbb564가 만든 OMP 파일 목록이 있다
When autopus.yaml에 `role_model_policy:\n  agent_source: native`를 손으로 더하고 `auto update --yes`, 그 줄을 지우고 다시 `auto update --yes`를 실행한다
Then native 뒤 `.omp/agents`에 생성 파일이 없고 `.omp/config.yml`이 없으며(ADK가 만든 파일이었다), OMP 생성 파일 목록과 bytes는 HEAD 93dbb564 결과와 같다
And 되돌린 뒤 OMP 파일 SHA-256 목록은 S12 끝과 같다
### S14: skill dispatch 이름
Priority: Must
Given `Agent(subagent_type="executor", prompt="a")`, `Agent(subagent_type="explorer", prompt="b")`, `Agent(subagent_type="reviewer", prompt="c")`를 담은 skill 본문이 있다
When `NormalizeOMPSemanticReferencesFor(body, autopus)`, `NormalizeOMPSemanticReferencesFor(body, native)`, `NormalizeOMPSemanticReferences(body)`를 부른다
Then autopus 결과에는 `"agent": "executor"`, `"agent": "explorer"`, `"agent": "reviewer"`가 있고 `"agent": "scout"`이 없다
And native 결과에는 executor 항목의 agent 줄이 없고 `"agent": "scout"`, `"agent": "reviewer"`가 있으며, 옛 함수의 결과와 bytes가 같다
### S15: doctor catalog와 receipt
Priority: Must
Given S7 project, 거기서 `.omp/agents/validator.md`를 지운 사본, native O3에 `.omp/agents/reviewer.md`를 둔 project, OP의 receipt가 있다
When OMP doctor projection과 pipeline backend의 OMP model 해석을 실행한다
Then S7은 agent catalog `Expected` 17, `Shadowed` 0, 상태 ready다. 사본은 degraded이고 blocker는 `agents:autopus_agent_missing`이다. native project는 지금처럼 `agents:native_agent_shadowed`다
And OP의 phase model은 plan ← `planner`, test scaffold ← `tester`, implement ← `executor`, validate ← `validator`, review ← `reviewer` 행이다. receipt에서 `executor` 행을 지우면 `executor`를 이름에 담은 오류로 실패한다
### S16: 회귀와 안전
Priority: Must
Given 이 branch와 HEAD 93dbb564의 test, SPEC-MODELUX-002 S1의 줄 입력 golden이 있다
When `go test ./pkg/... ./internal/cli/...`를 필터 없이 돌리고 `git diff --stat 93dbb564 -- '*_test.go'`를 보고, `--yes`·`CI=true` update의 stdin Read 횟수를 센다
Then 모든 test가 통과하고, 바뀐 기존 test 파일은 T1 목록에 있으며 규칙(fixture 한 줄 또는 기본값 전환 단언)을 따른다
And 줄 입력 golden은 bytes가 같고, effort를 고르지 않은 SPEC-MODELUX-002 S5·S6 입력의 출력과 파일은 HEAD 93dbb564와 같으며, 비대화형 stdin Read는 0회다
### S17: 실제 터미널 smoke
Priority: Must
Given macOS 실제 터미널, 이 branch의 `auto`, omp 18.6.1, `git init`한 빈 temp dir, scratch `CODEX_HOME`, `script -q` 기록이 있다
When `auto init --platforms claude-code,codex,omp`에서 역할별 직접 구성 → Claude Code → 역할 그룹 → 설계·판단 opus → effort xhigh → 나머지 Enter → 적용 → 유지를 고른다. `omp`를 열어 메시지를 보내지 않고 `/agents`만 본 뒤 닫는다. `auto quality`에서 OMP → balanced → GPT → 적용 → 유지를 고르고 `/agents`를 다시 본다
Then `.claude/agents/autopus/planner.md`는 `model: opus`, `effort: xhigh`이고 `.omp/agents`에 16개 파일, `.omp/config.yml`에 `disabledAgents`가 있다
And `/agents`에 Autopus 16개와 task가 있고 scout, security-reviewer, sonic은 disabled이며 reviewer의 설명이 `content/agents/reviewer.md`의 description이다. OMP 적용 뒤 planner의 model 칸이 override 값이다(blocker면 그 출력과 SHA-256 불변을 대신 적는다)
And transcript를 `$HOME`, scratch 경로, 사용자 이름을 치환해 `evidence/manual-smoke-003.txt`에 남긴다. `/review` verdict 확인은 모델 호출이 필요해 사용자 동의가 있을 때만 하고, 하지 않으면 미확인으로 적는다
### S18: locale
Priority: Should
Given D가 있다
When `composeLanguage`를 `en`과 `ko`로 두고 S4의 effort Select를 연다
Then en은 `keep — 6.1-sol · xhigh`와 `(automatic)`, ko는 `그대로 — 6.1-sol · xhigh`와 `(자동)`을 쓰고 저장 값은 같다
### S19: 문서
Priority: Should
Given help, README, CHANGELOG가 있다
When `auto init --help`, `auto update --help`, `auto quality --help`를 실행하고 문서를 읽는다
Then effort 편집, `efforts`의 옛 바이너리 한계, OMP Autopus 기본값, `agent_source: native`와 다음 릴리스 제거, migration이 있고 `quality_compose_docs_test.go`가 고정한 문장이 그대로이며 `make generate-templates` 뒤 반영되지 않은 diff가 없다
## Oracle Acceptance Notes

- Must 시나리오는 모두 concrete 값을 판정한다: 선택지 목록과 순서, 라벨, 바뀌는 설정 줄, frontmatter·TOML 값, override key 집합과 값, 실행 횟수, SHA-256, 오류 문구. 파일 존재나 종료 코드만으로 닫는 Must는 없다.
- S9·S11의 selector 값은 catalog fixture에 따라 정해지므로 같은 run의 `planOMPProfile` 미리보기와의 일치로 판정한다. 둘 다 결정적 출력이다.
- 기대 값은 2026-10-12 코드와 문서에서 계산했다(`codex_profile.go:105-124`, `quality_effective.go:48-65`, 선언 effort, Claude Code effort 표, P0 reviewer 모양). 실행으로 확인한 것은 probe P0·P3뿐이다. 코드 사실과 다르면 test가 아니라 이 SPEC을 고친다.
