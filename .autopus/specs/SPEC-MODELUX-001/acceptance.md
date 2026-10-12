# SPEC-MODELUX-001 수락 기준

## Test Scenarios

fixture는 모두 temp project 안에 만든다. 입력은 scripted reader로 준다. TTY, CI, Codex catalog probe, huh wizard 결과만 주입한 seam으로 정하고, 나머지는 실제 명령과 실제 adapter `Generate`로 돈다. 실제 TTY는 S21에서만 쓴다.

- D: `config.DefaultFullConfig("p")`를 `config.Save`한 파일. platforms `claude-code, codex`, `supervisor_model_policy: inherit`.
- R: 이 저장소 `autopus.yaml` 사본(balanced HB2, ultra HU1, `default: ultra`, `supervisor_model_policy: quality`, 플랫폼 5개, `role_model_policy` 없음). R2: R에서 platforms를 `claude-code, codex`로 줄인 파일.
- B2와 H: D에서 balanced를 각각 HB2, HB1로 바꾼 파일. P: D에 `quality.providers.codex: codex-custom`(16개 모두 `haiku`)을 더한 파일.
- O1: D에 platform `omp`와 `role_model_policy.profile: balanced`를 더한 파일. O2: R에 `role_model_policy.profile: ultra`를 더한 파일.
- X: R에서 ultra `tester: gpt`. C: R에서 ultra `devops: sonnet`. K: S6을 `y`로 끝낸 결과. W: probe A1의 손으로 쓴 2칸 fixture(같은 bytes).
- catalog: CAT-A는 `gpt-6-astra`(low–ultra), `gpt-6.1-sol`·`gpt-6-luna`·`gpt-5.6-sol`(low–max)이다. CAT-B는 CAT-A에서 `gpt-6.1-sol`을 빼고 `gpt-6-sol`(low–xhigh)을 넣었다. CAT-C는 probe 오류, CAT-D는 CAT-A에서 `gpt-6-astra`를 뺐다. 따로 적지 않으면 CAT-A다.

### S1: 그룹은 정규 에이전트를 한 번씩 덮고 YAML에 남지 않는다
Priority: Must
Given Role Group Contract와 `config.CanonicalAgentNames()`가 있다
When 그룹 정의를 읽고, D에서 S6 입력으로 custom을 저장한다
Then 그룹 순서는 `설계·판단, 구현, 검증·리뷰, 탐색·보조`이고, 멤버는 차례로 `planner, architect, spec-writer, security-auditor`, `executor, debugger, deep-worker, devops, frontend-specialist, perf-engineer`, `reviewer, validator, tester, ux-validator`, `explorer, annotator`다. 합집합은 16개 이름과 같고 중복이 없다
And 저장된 quality 블록의 key 경로는 `default`, `presets.balanced.*`, `presets.ultra.*`, `presets.custom.description`, `presets.custom.agents.<정규 이름 16개>`뿐이고, 그룹 이름 문자열은 파일 어디에도 없다

### S2: 시작 구성은 실제 생성 등급이고 손대지 않으면 쓰지 않는다
Priority: Must
Given D, R, B2가 있다
When 각 파일로 구성 세션을 열어 그룹 표시를 읽고, `g`와 Enter 4번으로 편집 없이 끝낸다
Then D는 `fable`, `혼합 (fable 2 · sonnet 4)`, `혼합 (fable 1 · sonnet 3)`, `sonnet`이고, R은 `opus`, `혼합 (opus 4 · sonnet 2)`, `혼합 (opus 3 · sonnet 1)`, `혼합 (opus 1 · sonnet 1)`이다
And B2에서 debugger의 시작 등급은 YAML 값 `sonnet`이 아니라 `fable`이다
And 세 파일 모두 `변경 없음`을 출력하고, 확인 질문 없이 끝나며, autopus.yaml SHA-256이 실행 전과 같다

### S3: 그룹 선택과 고급 편집
Priority: Must
Given D로 시작한 세션이 있다
When `g`, Enter, `3`, Enter, `4`를 입력한다. 미리보기에서 `e`와 `a`를 고르고 `debugger=opus`, `tester=gpt`, `explorer=fable`, 빈 줄을 입력한다
Then 그룹 단계 뒤 표시는 `fable`, `sonnet`, `혼합 (fable 1 · sonnet 3)`, `haiku`다. `debugger=opus` 뒤 구현은 `혼합 (opus 1 · sonnet 5)`이고, `explorer=fable` 뒤 탐색·보조는 `혼합 (fable 1 · haiku 1)`이다
And `tester=gpt`는 `unknown tier "gpt" (choose one of: fable, opus, sonnet, haiku)`를 출력한다. 앞의 debugger 편집과 tester의 `sonnet`은 그대로다

### S4: 표 칸은 이질적 config에서 정해진 값을 낸다
Priority: Must
Given D, R, P, O1, O2가 있다
When 각 파일의 구성 표를 렌더한다
Then R의 planner는 `opus`, Claude `opus · max`, Codex `6.1-sol · xhigh`, OMP·OpenCode·Antigravity `플랫폼 기본값`이다. deep-worker는 `기본값(opus)`, `opus · max`, `6.1-sol · xhigh`이고, annotator는 `기본값(sonnet)`, `sonnet · high`, `6.1-sol · xhigh`다. footer는 `Codex catalog: 읽음 · 대체 0칸`과 `Codex 기본 세션: 6-astra · ultra`다
And D는 열이 Claude Code와 Codex뿐이다. planner는 `fable`, `fable-5-1 · max`, `6-astra · max`이고, executor는 `sonnet`, `sonnet-5-5 · max`, `6-luna · max`이며, validator는 `sonnet`, `sonnet-5-5 · high`, `6-luna · max`다
And P의 Codex 머리말은 `Codex (개별 설정: codex-custom)`이고, planner `6-luna · max`, executor `6-luna · medium`, reviewer `6-luna · high`다. Claude 칸은 D와 같다
And O1의 OMP 머리말은 `OMP (요청 route · catalog 확인 전)`이고, executor는 `task · fable-5-1:max`, explorer는 `scout · sonnet-5-5:high`다. O2의 executor는 `task · opus-5-5:xhigh`다
And 같은 입력으로 두 번 렌더한 bytes가 같고, 출력에 ANSI escape가 없다

### S5: Claude·Codex 칸은 같은 snapshot으로 돈 실제 생성 결과와 같다
Priority: Must
Given D, R, P, O1, O2, X, K, H와 CAT-A가 있다
When 각 파일로 표를 그리고, 같은 CAT-A snapshot을 `codex.WithModelCatalog`로 준 실제 Claude Code·Codex adapter `Generate`를 temp dir에서 실행한다
Then 8개 파일 × 16개 에이전트 모두에서 `.claude/agents/autopus/<agent>.md`의 `model:`과 `effort:`(없으면 `-`)가 Claude raw 값과 같고, `.codex/agents/<agent>.toml`의 `model`과 `model_reasoning_effort`가 Codex raw 값과 같다
And X의 tester 행은 `잘못된 값→sonnet`, `sonnet · high`, `6.1-sol · xhigh`다. R의 deep-worker는 기존 도구별 표의 `sonnet`(fallback `""`)이 아니라 생성 파일과 같은 `opus · max`다
And O1·O2의 OMP 칸은 같은 config로 부른 `OMPNativeAgentRoute`의 첫 후보와 같다(OMP 칸의 oracle은 요청 route다)

### S6: 미리보기, 확인, custom binding
Priority: Must
Given D가 있다
When 대화형 `auto quality`에 `g`, Enter, `3`, Enter, `4`를 입력해 미리보기를 보고, 한 번은 `n`, 한 번은 `y`로 답한다
Then 미리보기는 Claude 16칸 전부에 `*`를 붙인다. Codex는 4칸에만 붙인다(debugger와 deep-worker는 `6-astra · max`→`6-luna · max`, explorer와 annotator는 `6-luna · max`→`6-luna · medium`)
And Claude 칸의 예는 planner `fable-5-1 · max`→`fable · max`, executor `sonnet-5-5 · max`→`sonnet · medium`, explorer `sonnet-5-5 · high`→`haiku · -`다
And 바뀌는 설정은 `quality.presets.custom 새로 씀 (16개 에이전트)`, `quality.default balanced → custom`, `custom은 balanced 표준 배치를 받지 않음`, `quality 블록을 다시 씁니다: 블록 안 주석 0줄과 서식은 유지되지 않습니다(값은 그대로)` 네 줄이다. supervisor 줄은 없다
And `n`이면 `취소했습니다. 설정을 바꾸지 않았습니다.`를 출력하고 SHA-256이 같다
And `y`면 `quality.default: custom`이 된다. `quality.presets.custom.agents`는 16개다. architect, planner, reviewer, security-auditor, spec-writer는 `fable`, debugger, deep-worker, devops, executor, frontend-specialist, perf-engineer, tester, ux-validator, validator는 `sonnet`, explorer와 annotator는 `haiku`다. `quality.providers` key는 없다

### S7: binding은 결정 표를 따른다
Priority: Must
Given 결과 구성과 기록된 built-in 프리셋 상태의 조합이 있다
When binding 함수와 writer를 부른다
Then 결과가 시작 구성과 같으면 쓰기가 없다. 결과가 최신 balanced이고 balanced가 current이며 default가 `custom`이면 line writer를 쓰고, 파일 diff는 `default:` 한 줄뿐이다
And 결과가 최신 balanced이고 balanced가 HB2면 `presets.balanced`가 최신 16개가 되고 `default: balanced`가 된다. balanced 프리셋이 없으면 프리셋과 default를 같이 쓰고, 결과 파일은 `Validate`를 통과한다
And 결과가 최신 balanced이고 balanced가 customized(최신 기본값에 `tester: fable`)면 `presets.custom`(결과 16개)과 `default: custom`을 쓰고, balanced의 agents와 description 값은 그대로다
And 결과가 최신 ultra이고 ultra가 HU1이며 default가 `ultra`면 `presets.ultra`만 최신으로 바뀌고 `default:` 줄은 그대로다
And 사용자 프리셋 `fast`가 묶인 상태에서 그 밖의 결과는 `presets.custom`과 `default: custom`으로 저장되고 `fast`의 값은 그대로다. custom이 이미 있고 binding이 아니면 미리보기에 `quality.presets.custom 덮어씀`이 있다

### S8: GAP 1 판별은 정확 일치이고 런타임은 그대로다
Priority: Must
Given registry 6행, 현재 기본값 2개, R, X, `devops`를 뺀 HU1, description만 바꾼 HB2, H가 있다
When `ClassifyBuiltinQualityPreset`, `isHistoricalBalancedDefault`, `NativeBalancedAgentCandidate`를 부른다
Then 6행은 모두 `historical`, 현재 기본값 2개는 `current`다. R은 balanced와 ultra 모두 `historical`이고, description만 다른 HB2도 `historical`이다
And `devops`를 뺀 HU1과 X의 ultra는 `customized`이고, 둘 다 `사용자 수정 (12개 다름)`으로 표시된다
And `isHistoricalBalancedDefault`는 HB2와 HB5에서 true, HB1과 HB4에서 false다(이 SPEC 전과 같다). H의 Claude projection은 main과 같다(planner `opus · high`, debugger `sonnet · medium`, validator `haiku · -`)
And registry의 각 행은 정규 이름만 쓰고, 16개 이하이며, 등급이 모두 유효하고, 현재 기본값과 다르다

### S9: historical 갱신과 사용자 수정 보호
Priority: Must
Given R과 C가 있다
When R에서 `auto quality`에 `u`, `y`를 준다. C에서 대화형 `auto update`를 실행하고, `auto quality`에 `claude-code\ndevops=fable\n\nultra\nultra\nultra\n`를 주고, 고급으로 16개를 최신 ultra 값에 맞춰 `y`로 확인한다
Then R의 미리보기는 ultra 차이 `architect, debugger, planner, reviewer, security-auditor, spec-writer opus→fable`과 추가 `annotator→opus, deep-worker→fable, frontend-specialist→opus, perf-engineer→opus, ux-validator→opus`, balanced 줄 `모델 변화 없음, YAML만 정리`를 보인다. Claude 11칸과 Codex 7칸에 `*`가 붙는다
And `y` 뒤 ultra와 balanced의 agents와 description은 코드 기본값과 같고, `quality.default`는 `ultra`이며, quality 블록 밖 bytes는 같다. 재생성한 `planner.md`는 `model: fable`, `effort: max`다
And C의 상태 줄은 `ultra: 사용자 수정 (12개 다름)`이고, `u`는 balanced만 갱신한다. 도구별 편집기는 `ultra` 이름을 받을 때마다 `사용자 수정 프리셋 ultra는 덮어쓰지 않습니다. 다른 이름을 입력하세요.`를 출력하고, 3번째에서 취소하며, SHA-256이 같다
And C의 고급 확인은 `quality.presets.custom`과 `quality.default: custom`으로 저장된다. 모든 흐름 뒤 C의 ultra agents와 description은 원래 값과 같다

### S10: update TTY 계약
Priority: Must
Given R과 D 사본, 주입한 TTY·CI probe, Read가 불리면 test를 실패시키는 reader가 있다
When 아래 조건마다 `auto update --dir <dir>`를 실행한다
Then TTY true, 플래그 없음, CI 없음이면 R은 표, `ultra: 이전 버전 기본값 (갱신 가능)`, `구성을 바꿀까요?`를 출력한다. 입력이 `\n`, 빈 입력(EOF), `x\nx\nx\n` 중 무엇이든 SHA-256은 같다. 마지막 경우에는 `입력이 올바르지 않아 구성을 유지합니다.`를 출력하고, update는 종료 코드 0으로 끝난다. catalog probe는 run당 한 번 불린다
And `CI=false`와 `CI=0`은 대화형이다. TTY false, `--yes`, `-y`, `--auto`, `--check`(`--self` 없음), `CI=true`, `CI=1`은 실패 reader를 읽지 않고, SHA-256이 같으며, `참고: ultra, balanced 프리셋이 이전 버전 기본값입니다. 대화형 auto update 또는 auto quality에서 갱신할 수 있습니다.`를 정확히 한 번 출력한다. `--check`도 플랫폼 파일 생성은 지금처럼 한다
And `--plan`은 reader를 읽지 않고 이 기능의 출력이 없다. `--self --check`는 구성 단계 진입 counter가 0이다. R 사본 2개 workspace의 `update --workspace --yes`는 reader를 읽지 않고, 안내는 리포마다 한 번 이하다
And D에서 TTY true로 `g\n\n3\n\n4\ny\n`을 주면 같은 run의 `.claude/agents/autopus/debugger.md`는 `model: sonnet`, `effort: medium`이고, `.codex/agents/debugger.toml`은 `model = "gpt-6-luna"`, `model_reasoning_effort = "max"`다. 기존 update test는 고치지 않고 통과한다

### S11: 실제 init 명령의 세 번째 선택지
Priority: Must
Given 빈 temp project 두 개, `Quality: role-groups`를 돌려주는 `runInitWizard` seam, TTY probe true, CAT-A가 있다
When `auto init --dir <dir> --platforms claude-code,codex`를 stdin `g\n\n3\n\n\ny\n`과 `g\n\n3\n\n\nn\n`으로 각각 실행한다. 또 `auto init --yes`와 `auto init --quality ultra --yes`를 실행한다
Then 첫 run의 autopus.yaml은 `quality.default: custom`이다. custom에서 debugger와 deep-worker는 `sonnet`이고 나머지는 balanced 기본 등급이다. 출력에는 구성 표와 미리보기가 있다. 생성된 `.claude/agents/autopus/debugger.md`는 `model: sonnet`, `effort: medium`이고, `.codex/agents/debugger.toml`은 `model = "gpt-6-luna"`, `model_reasoning_effort = "max"`다
And 둘째 run의 quality 블록은 `config.DefaultFullConfig`와 같고, 생성된 `planner.md`는 `model: claude-fable-5-1`, `effort: max`다
And `--yes`의 quality 블록은 `DefaultFullConfig`와 같고, `--quality ultra --yes`는 그 블록과 `default: ultra`만 다르다. 두 경우 모두 구성 표를 출력하지 않고 wizard seam을 부르지 않는다

### S12: 실제 auto quality 진입점과 --apply
Priority: Must
Given D, R2, CAT-A가 있고 `qualityPlatformUpdater` stub은 없다
When `auto quality`에 `claude-code\nexecutor=fable\n\n\ny\n`, `global\n1\n`, `t\nglobal\n1\n`을 차례로 준다. 실제 stdin이 non-TTY인 상태에서 인자 없이 실행한다. `auto quality --apply`를 R2에서 `u\ny\n`로, D에서 S6 입력과 `y`로 실행한다
Then 첫 입력은 `quality.providers.claude = claude-custom`을 쓰고, `claude-custom` 16개 중 executor가 `fable`이다. 이 쓰기는 `saveQualityBlock`을 지나 quality 블록 밖 bytes가 같다
And `global\n1\n`과 `t\nglobal\n1\n`은 둘 다 `quality.default = ultra`를 출력한다. non-TTY 실행은 `interactive quality selection requires a TTY; use an explicit quality or profile command` 오류다
And R2의 `--apply`는 `quality.applied_platforms = 2`를 출력한다. 생성된 `planner.md`는 `model: fable`, `effort: max`이고, `.codex/agents/planner.toml`은 `model = "gpt-6-astra"`, `model_reasoning_effort = "max"`다
And D의 `--apply` 뒤 `.codex/agents/explorer.toml`은 `model = "gpt-6-luna"`, `model_reasoning_effort = "medium"`이고, `.claude/agents/autopus/explorer.md`는 `model: haiku`이며 `effort:` 줄이 없다

### S13: writer는 펼쳐진 값을 쓰지 않고 블록 밖 bytes를 지킨다
Priority: Must
Given `PRESET_NOTE=leak-value-123`이 설정되어 있다. fixture는 다음과 같다. R 사본, W, W의 CRLF 사본, quality 블록이 없는 파일, quality가 마지막인 혼합 들여쓰기 파일. `default:`와 `presets:` 사이에 1열 주석 `# preset notes`가 있는 파일. `project_name: &note '${PRESET_NOTE}'`와 `quality.presets.balanced.description: *note`를 가진 파일 F1. `quality.presets.fast.description: '${PRESET_NOTE}'`를 가진 파일 F2. quality 안에 anchor와 alias가 있는 파일 F3
When 각 파일에 S6의 custom 쓰기를 실행하고, F1에는 도구별 편집기 입력 `claude-code\nexecutor=fable\n\n\ny\n`도 준다. F1과 F2는 `PRESET_NOTE`를 지운 상태로도 한 번 더 실행한다
Then 앞의 여섯 파일은 쓰기에 성공한다. quality 블록 밖 줄은 byte 단위로 같고(W의 `project_name: 'demo'   # trailing comment`, 빈 줄 3개, `  decoy: &bin codex`, `      binary: *bin`), CRLF 사본의 새 블록도 `\r\n`이다. 블록이 없는 파일은 원본 bytes 뒤에 새 블록이 붙는다. 1열 주석 파일의 결과에는 `presets` key가 하나뿐이고 `config.Load`로 읽힌다
And F1은 변수 설정 여부와 관계없이, 구성 쓰기와 도구별 편집기 쓰기 모두 `quality 블록에 환경 변수 값이나 YAML 참조(alias·merge)가 섞여 있어 자동으로 쓰지 않습니다. autopus.yaml을 직접 고치세요.`를 출력한다. SHA-256이 같고 파일 어디에도 `leak-value-123`이 없다
And F2는 변수가 설정되면 같은 문구로 거부되고 SHA-256이 같다. 변수가 없으면 쓰기에 성공하고, `fast.description`은 글자 그대로 `${PRESET_NOTE}`로 남는다. F3은 거부된다
And 성공한 모든 쓰기에서 env 확장 없이 decode한 새 quality의 손대지 않은 프리셋은 원본의 raw decode와 deep-equal이다. `quality.providers`가 없는 파일의 quality 블록은 a0355acf 이전 schema(`default`, `supervisor_model_policy`, `presets.<name>.description`, `presets.<name>.agents`)로 `KnownFields(true)` decode가 성공한다. rename seam이 실패하면 원본 bytes가 같고 temp 파일이 남지 않는다

### S14: `*`는 같은 snapshot으로 생성한 두 결과의 차이다
Priority: Must
Given D와 S6의 후보 config, CAT-A가 있다
When 두 config로 실제 Claude Code·Codex `Generate`를 각각 다른 temp dir에서 같은 snapshot으로 실행하고 agent 파일을 비교한다
Then 다른 줄은 Claude의 `model:`·`effort:`와 Codex의 `model`·`model_reasoning_effort`뿐이다(explorer와 annotator는 `effort:` 줄이 빠진다)
And Claude는 16개 파일이 모두 다르고 Codex는 debugger, deep-worker, explorer, annotator 4개만 다르다. 다른 값은 모두 S6 미리보기에서 `*`가 붙은 칸의 raw 값과 같다

### S19: Codex catalog의 대체, 없음, 거부가 표와 생성에 같게 나타난다
Priority: Must
Given R과 D, catalog CAT-B, CAT-C, CAT-D가 있다
When 각 조합으로 표를 그리고, 같은 snapshot으로 실제 Codex `Generate`를 실행하거나 대화형 update에서 Enter를 준다
Then R과 CAT-B: planner Codex 칸은 `6-sol · xhigh (대체)`, footer는 `Codex catalog: 읽음 · 대체 16칸`이다. 생성된 `planner.toml`은 `model = "gpt-6-sol"`, `model_reasoning_effort = "xhigh"`다
And R과 CAT-C: planner는 `5.6-sol · xhigh (catalog 없음)`, footer는 `Codex catalog: 읽지 못함`이고, 생성 값은 `gpt-6.1-sol`이 아니라 `gpt-5.6-sol`·`xhigh`다. D와 CAT-C: planner는 `6-astra · max (미확인)`이고 생성 값은 `gpt-6-astra`·`max`다
And D와 CAT-D: 표준 배치 상위 7개(architect, debugger, deep-worker, planner, reviewer, security-auditor, spec-writer) 칸이 `생성 거부`이고, 화면에 `Codex 생성이 실패합니다`가 있다. Enter 뒤 autopus.yaml SHA-256이 같고, update는 `플랫폼 업데이트 실패: codex`를 보고하며, Claude Code 파일은 갱신된다

### S20: 디스크의 오래된 생성 파일은 표에 나오지 않고 표 값으로 맞춰진다
Priority: Must
Given R에서 생성한 뒤 `.claude/agents/autopus/planner.md`를 `model: sonnet`으로, `.codex/agents/planner.toml`을 `model = "gpt-5.5"`로 고친 project가 있다
When TTY true로 `auto update`를 실행하고 Enter를 준다
Then 표의 planner 행은 `opus · max`, `6.1-sol · xhigh`이고 `*`가 없으며, 출력 어디에도 `gpt-5.5`가 없다
And update 뒤 `planner.md`는 `model: opus`, `effort: max`, `planner.toml`은 `model = "gpt-6.1-sol"`, `model_reasoning_effort = "xhigh"`이고, autopus.yaml SHA-256은 같다

### S21: 실제 터미널 smoke
- 종결(2026-10-12): 2026-10-12 1차 smoke에서 줄 입력 UX와 SHA oracle 문제가 드러났다(사용자 피드백 → SPEC-MODELUX-002). oracle은 SPEC-MODELUX-002 REQ-MUX2-13으로 고쳤고, 실제 TTY 확인은 SPEC-MODELUX-002 S20(`.autopus/specs/SPEC-MODELUX-002/evidence/manual-smoke-002.txt`)이 닫는다.
Priority: Must
Given macOS 터미널(실제 TTY), 이 branch로 만든 `auto`, `git init`한 빈 temp dir이 있다
When `auto init --platforms claude-code,codex`에서 huh quality 단계의 `역할별 직접 구성`을 고르고 줄 프롬프트에 `g`, Enter, `3`, Enter, Enter, `y`를 입력한다. 이어서 `auto update`를 실행하고 Enter를 누른다
Then huh 폼이 닫힌 직후 줄 프롬프트가 나타나고 입력이 빠지지 않는다. `.claude/agents/autopus/debugger.md`는 `model: sonnet`, `effort: medium`이다
And `auto update`는 표와 `구성을 바꿀까요?`를 출력하고, Enter 뒤 autopus.yaml SHA-256이 같다. 전체 기록은 `evidence/manual-smoke.txt`에 남긴다

### S15: override가 새 구성을 따를지 미리보기 전에 묻는다
Priority: Should
Given P가 있다
When S6 입력 뒤 추가 질문에 한 번은 `n`, 한 번은 `y`로 답하고, 미리보기를 본 뒤 `y`로 확인하고 `--apply`로 생성한다
Then 질문은 미리보기 전에 나온다: `Codex도 새 구성을 따를까요? (quality.providers.codex = codex-custom 제거) [y/N]`
And `n`이면 미리보기의 Codex 열이 codex-custom 값 그대로이고 바뀌는 설정에 providers 줄이 없다. `y`면 Codex 열이 custom 값(executor `6-luna · max`)으로 바뀌어 `*`가 붙고 `quality.providers.codex 제거` 줄이 있다. 두 경우 모두 생성된 Codex 값은 승인한 미리보기와 같다

### S16: show --agents는 읽기 전용이다
Priority: Should
Given R이 있고 stdin이 non-TTY다
When `auto quality show --agents`를 실행한다
Then 기존 `show` 출력 줄이 그대로 앞에 오고 그 뒤에 S4의 R 표가 온다. autopus.yaml SHA-256은 같다

### S17: locale
Priority: Should
Given R이 있다
When `LANG=en_US.UTF-8`과 `LANG=ko_KR.UTF-8`로 구성 단계를 연다
Then en은 `Design·Judgment`, `platform default`, `Change the composition?`을, ko는 `설계·판단`, `플랫폼 기본값`, `구성을 바꿀까요?`를 출력한다. 두 경우 모두 저장되는 custom description은 `역할 그룹 구성`이다

### S18: 문서
Priority: Should
Given CLI help, README, CHANGELOG가 있다
When `auto quality --help`와 `auto update --help`를 실행하고 문서를 읽는다
Then help에 역할 그룹, 표, binding, update 질문 규칙이 있다. README와 v0.50.125 CHANGELOG에는 같은 내용과 GAP 1 분류, OMP 요청 route 범위가 있다. `make generate-templates` 뒤 생성 루트에 반영되지 않은 diff가 없다

## Oracle Acceptance Notes

- 모든 Must 시나리오는 concrete expected output을 갖는다. 칸 값, 그룹 표시 문자열, 바뀌는 키 줄, 안내·거부 문구, SHA-256 동일성, 생성 파일의 `model`·`effort` 값, Read 0회, probe 횟수다. 파일 존재나 종료 상태만으로 닫는 Must 시나리오는 없다.
- Claude·Codex 칸의 oracle은 같은 snapshot으로 돈 실제 adapter `Generate`다(S5, S14, S19). OMP 칸의 oracle은 요청 route(`OMPNativeAgentRoute`)이고, OpenCode·Antigravity는 `플랫폼 기본값` 글자다. 디스크에 있던 생성 파일은 oracle이 아니다(S20).
- 칸 값은 2026-10-10에 코드를 읽고 계산했다(`codex_profile.go:105-124`, `codex_profile_resolve.go:38-110`, `role_model_policy_balanced.go:21-69`, `claude_agent_content.go:114-148`, `role_model_policy_builtin.go:42-47`, `content/agents/*.md`). 아직 실행하지 않았다. 코드 사실과 다르면 test가 아니라 이 SPEC을 고친다(probe A3).
- S13의 W는 probe A1의 fixture와 같은 bytes다. F1은 review finding F-001의 입력 그대로다.
