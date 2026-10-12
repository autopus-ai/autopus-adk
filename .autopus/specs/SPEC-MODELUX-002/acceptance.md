# SPEC-MODELUX-002 수락 기준

## Test Scenarios

fixture는 모두 temp project 안에 만든다. 폼 test는 `composeFormsProbe`를 true로, `composeRunForm`을 huh accessible runner(`WithAccessible(true).WithInput(cmd.InOrStdin()).WithOutput(cmd.OutOrStdout())`)로 바꾸고, 입력은 `testing/iotest.OneByteReader`로 준다. accessible Select는 `N. label` 목록을 쓰고 숫자를 읽는다. 아래에서 "X를 고른다"는 그 라벨의 번호를 입력한다는 뜻이고 Enter는 기본값이다. 대신하는 seam은 TTY·CI probe, 폼 판정과 실행, Codex catalog probe, OMP runner·activate, init wizard 결과뿐이다. 나머지는 실제 명령(`NewRootCmd`)과 실제 adapter `Generate`로 돈다. 실제 터미널은 S20에서만 쓴다.

- D, R, P, O1, O2, X, K, H, W, CAT-A, CAT-B, CAT-D: SPEC-MODELUX-001 `acceptance.md`와 같다. D는 `config.DefaultFullConfig("p")`를 `config.Save`한 파일(platforms `claude-code, codex`, balanced current, `supervisor_model_policy: inherit`)이다. P는 D에 `quality.providers.codex: codex-custom`(16개 모두 `haiku`)을 더한 파일이다. 따로 적지 않으면 catalog는 CAT-A다.
- D1: D에서 platforms를 `claude-code`로 줄인 파일. N: platforms가 `opencode`, `antigravity-cli`뿐인 파일.
- O3: D에 platform `omp`를 더해 `config.Save`한 파일(`role_model_policy` 없음). OMP runner는 기존 `ompCLIFakeRunner{catalog: ompCLIBalancedCatalogJSON()}`, activate는 호출 횟수와 받은 cfg를 기록하는 stub이다.
- DP: D에 감지 플랫폼 `opencode`, `omp`가 있는 실행. DC: D1에 감지 플랫폼 `codex`가 있는 실행. 감지는 PATH의 가짜 실행 파일로 만든다(`omp`는 `--version`에 답한다). DC의 가짜 `codex`는 호출을 기록하고 `debug models`에 CAT-D를 낸다. 기존 test는 `t.Setenv("PATH", …)`와 `writeFakeOMPBinary`(`omp_identity_gate_test.go:15-24`)로 감지를 고정한다.
- IO: 빈 temp project에서 `auto init --dir <dir> --platforms claude-code,codex,omp`, wizard seam 결과 `role-groups`. OMP catalog runner는 `initComposeOMPDeps`, OMP 생성 runner는 test 전용 `platformOMPModelRunner`로 준다. 생성용 가짜는 설정 되읽기에 답해야 하고, internal/cli에서 만들 수 있는지는 미확인이다(`research.md` `## OMP test seam 조사`).

### S1: 입력 방식 선택과 줄 driver 불변
Priority: Must
Given HEAD 44773355에서 기록한 줄 입력 출력 golden 세 개(R의 `auto update` Enter, D의 `auto quality` `global\n1\n`, D의 `auto quality` `g\n\n3\n\n4\nn\n`)와 실행 횟수를 세는 `composeRunForm` stub이 있다
When 같은 입력을 `cmd.SetIn`으로 이 branch에서 다시 돌리고, `composeFormsProbe`의 기본 판정을 Input Mode Contract의 조건마다 부른다
Then 세 출력은 golden과 bytes가 같고 폼 실행은 0회다
And 기본 판정은 명령 입력이 `os.Stdin`이고 stdin과 stdout이 모두 터미널일 때만 true다. SetIn reader, stdout pipe, Windows에서 `WT_SESSION`과 `TERM_PROGRAM`이 모두 빈 경우는 false다
And TTY probe false, `--yes`, `--check`, `CI=true`인 `auto update`는 stdin Read 0회, 폼 실행 0회다

### S2: 메뉴 Select, 반복, snapshot 1회
Priority: Must
Given R과 D, catalog probe 호출을 세는 CAT-A probe가 있다
When R에서 `auto update`를 실행해 메뉴에 Enter를 준다. D에서 `auto quality`를 실행해 구성 편집 → Codex → 역할 그룹 → 설계·판단 `6.1-sol · xhigh  [opus]` → 나머지 Enter → 적용 → 메뉴 유지를 고른다. 또 D에서 `auto quality`로 기본 프리셋 고르기 → `ultra`를 고른다
Then R 메뉴의 출력은 `1. 유지`, `2. 최신 기본값으로 갱신`, `3. 구성 편집`이고, Enter 뒤 autopus.yaml SHA-256이 같으며 update는 플랫폼 생성까지 끝난다
And D의 `auto quality` 메뉴는 `1. 유지`, `2. 구성 편집`, `3. 기본 프리셋 고르기`다. 적용 뒤 표가 다시 그려지고 그 표의 Codex 머리말은 `Codex (개별 설정: codex-custom)`이며, 메뉴가 한 번 더 나온 뒤 끝난다. catalog probe는 run 전체에서 1회다
And 기본 프리셋 고르기 뒤 출력에 `quality.default = ultra`가 있고, 파일 diff는 `default:` 한 줄뿐이다

### S3: 도구 탭의 선택지
Priority: Must
Given D, D1, N, O3, DP가 있다
When D, D1, N, O3은 `auto quality`에서, DP는 `auto update`에서 구성 편집을 고른다. O3과 같은 platforms(`claude-code, codex, omp`)로 init의 역할별 직접 구성도 실행한다
Then D의 탭은 `1. 모든 도구 같게`, `2. Claude Code`, `3. Codex`이고, O3은 여기에 `4. OMP`가 더해진다. D1의 탭은 `1. 모든 도구 같게`, `2. Claude Code`다
And DP의 탭은 D와 같고(omp는 감지만 됐다) 출력에 `OMP는 이번 update에서 새로 설치됩니다. 모델은 다음 auto update나 auto quality에서 지정할 수 있습니다.`가 한 번 있다. 어느 탭에도 OpenCode와 Antigravity가 없다
And N은 탭 없이 바로 방식 Select가 나온다. init의 탭은 O3과 같고(`4. OMP` 포함) init OMP 안내 줄은 없다

### S4: 모든 도구 같게의 실제 모델 라벨과 binding
Priority: Must
Given D가 있다
When `auto quality --apply`에서 구성 편집 → 모든 도구 같게 → 역할 그룹을 고르고, 설계·판단에서 opus를 고른 뒤 나머지 그룹은 Enter, 확인에서 적용, 메뉴에서 유지를 고른다
Then 설계·판단 Select의 선택지는 차례로 `그대로 — fable`, `fable — Claude Code: fable-5-1 · max | Codex: 6-astra · max`, `opus — Claude Code: opus-5-5 · high | Codex: 6.1-sol · xhigh`, `sonnet — Claude Code: sonnet-5-5 · medium | Codex: 6-luna · max`, `haiku — Claude Code: haiku-4-5 · - | Codex: 6-luna · max`다
And 미리보기는 Claude 16칸 전부와 Codex 4칸(planner, architect, spec-writer, security-auditor의 `6-astra · max`→`6.1-sol · xhigh`)에 `*`를 붙인다. 바뀌는 설정은 `quality.presets.custom 새로 씀 (16개 에이전트)`, `quality.default balanced → custom`, `custom은 balanced 표준 배치를 받지 않음`, `quality 블록을 다시 씁니다: 블록 안 주석 0줄과 서식은 유지되지 않습니다(값은 그대로)` 네 줄이다
And 적용 뒤 `quality.default: custom`이다. custom의 planner, architect, spec-writer, security-auditor는 `opus`, debugger, deep-worker, reviewer는 `fable`, 나머지 9개는 `sonnet`이고 `quality.providers` key는 없다. 출력에 `quality.applied_platforms = 2`가 있고, 생성된 `.claude/agents/autopus/planner.md`는 `model: opus`, `effort: high`, `.codex/agents/planner.toml`은 `model = "gpt-6.1-sol"`, `model_reasoning_effort = "xhigh"`다

### S5: Codex 탭 그룹 편집과 저장
Priority: Must
Given D가 있다
When `auto quality --apply`에서 구성 편집 → Codex → 역할 그룹을 고르고, 설계·판단에서 `6.1-sol · xhigh  [opus]`, 나머지 그룹은 Enter, 확인에서 적용, 메뉴에서 유지를 고른다
Then 설계·판단 Select는 `그대로 — 6-astra · max`, `6-astra · max  [fable]`, `6.1-sol · xhigh  [opus]`, `6-luna · max  [sonnet]`, `6-luna · max  [haiku]`다. 구현 Select의 haiku 선택지는 `6-luna · medium / 6-luna · max  [haiku]`다
And 미리보기의 Codex 머리말은 `Codex (개별 설정: codex-custom)`이고 `*`는 Codex의 planner, architect, spec-writer, security-auditor 4칸에만 있다(Claude 열과 등급 열에는 없다). 바뀌는 설정은 `quality.presets.codex-custom 새로 씀 (16개 에이전트)`, `quality.providers.codex 없음 → codex-custom`, `codex-custom은 balanced 표준 배치를 받지 않음`, 블록 재작성 안내, `quality.providers는 v0.50.88보다 오래된 auto가 읽지 않습니다` 다섯 줄이다
And 적용 뒤 `quality.default`는 `balanced` 그대로이고 `quality.providers.codex`는 `codex-custom`이다. `codex-custom`의 description은 `Codex custom agent tiers`, agents는 architect, planner, security-auditor, spec-writer `opus`, debugger, deep-worker, reviewer `fable`, 나머지 9개 `sonnet`이다. quality 블록 밖 bytes는 같고, 새 quality 블록은 현재 `QualityConf`로 `KnownFields(true)` decode에 성공한다
And 같은 run의 `planner.toml`은 `model = "gpt-6.1-sol"`, `model_reasoning_effort = "xhigh"`, `debugger.toml`은 `gpt-6-astra`·`max`, `executor.toml`은 `gpt-6-luna`·`max`다. `.claude/agents/autopus/planner.md`는 `model: claude-fable-5-1`, `effort: max` 그대로다

### S6: Claude 탭의 에이전트별 편집
Priority: Must
Given D가 있다
When `auto quality --apply`에서 구성 편집 → Claude Code → 에이전트별을 고르고, 목록에서 executor → `opus-5-5 · high  [opus]` → 완료, 확인에서 적용, 메뉴에서 유지를 고른다
Then 처음 목록은 `완료`로 시작하고 `planner — fable-5-1 · max`, `executor — sonnet-5-5 · max`, `validator — sonnet-5-5 · high`를 포함한다. executor 값 Select는 `그대로 — sonnet-5-5 · max`, `fable-5-1 · max  [fable]`, `opus-5-5 · high  [opus]`, `sonnet-5-5 · max  [sonnet]`, `haiku-4-5 · -  [haiku]`다
And 고른 뒤 목록은 `executor — opus-5-5 · high`, `validator — sonnet-5-5 · medium`, `planner — fable-5-1 · max`를 보인다
And 미리보기의 Claude 머리말은 `Claude Code (개별 설정: claude-custom)`이고 Claude 16칸 모두에 `*`가 붙는다(executor `opus · high*`, validator `sonnet · medium*`, planner `fable · max*`). Codex 칸에는 `*`가 없다
And 적용 뒤 `quality.providers.claude`는 `claude-custom`이다. `claude-custom`의 description은 `Claude Code custom agent tiers`, executor `opus`, architect, debugger, deep-worker, planner, reviewer, security-auditor, spec-writer `fable`, 나머지 8개 `sonnet`이다. 생성된 `executor.md`는 `model: opus`, `effort: high`, `validator.md`는 `model: sonnet`, `effort: medium`이고, `.codex/agents/executor.toml`은 `gpt-6-luna`·`max` 그대로다

### S7: 확인 Select, 계속 편집, 변경 없음
Priority: Must
Given D가 있다
When S5의 입력으로 확인에서 Enter를 준다. 다시 S5의 입력으로 확인에서 계속 편집을 고르고 방식 Select에서 에이전트별을 고른다. 또 구성 편집 → Codex → 역할 그룹 → Enter 4번을 준다
Then 확인 Select는 `1. 취소`, `2. 적용`, `3. 계속 편집`이고, Enter 뒤 `취소했습니다. 설정을 바꾸지 않았습니다.`가 나온 뒤 메뉴로 돌아가며 SHA-256이 같다
And 계속 편집 뒤 방식 Select가 다시 나오고 에이전트 목록에 `planner — 6.1-sol · xhigh`가 있다(작업 구성이 남는다)
And Enter 4번은 `변경 없음`을 출력하고 확인 Select 없이 메뉴로 돌아가며 SHA-256이 같다

### S8: OMP 탭 적용과 같은 run의 동기화
Priority: Must
Given O3이 있다
When `auto quality`에서 구성 편집 → OMP → 에이전트별 지정 (balanced 위에) → GPT를 고르고, 목록에서 task → `openai-codex/gpt-6-astra` → thinking `max` → 완료, 확인에서 적용, 메뉴에서 유지를 고른다. 같은 입력으로 O3 사본에 `runUpdateComposeStep`(`updateComposeOMPDeps`는 같은 stub)도 부른다
Then 미리보기는 기존 요약 `OMP balanced — model preview (no changes yet)`이고 `task*` 행에 `openai-codex/gpt-6-astra`와 `max`가 있다. 확인 위에 `OMP 설정은 autopus.yaml 전체를 다시 씁니다: 주석과 서식이 바뀔 수 있습니다(값은 그대로)`가 있다
And 적용 뒤 `role_model_policy.profile`은 `balanced`, `family`는 `openai`이고, `agents.task.candidates`는 selector `openai-codex/gpt-6-astra`, thinking `max`, family `openai` 하나다. scout, reviewer, security-reviewer, sonic은 `agents`에 없다. activate는 1회이고 출력에 `OMP models applied. Start a new OMP session to use them.`가 있다. env 확장 없이 decode한 quality 값은 실행 전과 deep-equal이다
And 다시 그린 표의 OMP 머리말은 `OMP (요청 route · catalog 확인 전)`이고 executor 칸은 `task · 6-astra:max`다
And `runUpdateComposeStep`이 끝난 뒤 cfg.RoleModelPolicy는 파일에서 다시 읽은 `role_model_policy`와 deep-equal이다

### S9: OMP 차단, 거부, 취소, pin 없음
Priority: Must
Given O3, `ompCLIBalancedCatalogWithout(t, "openai-codex/gpt-6-astra")` catalog, 기존 `platform_omp_config_placeholder_test.go`가 쓰는 placeholder role policy를 넣은 O3 사본(내용은 미확인)이 있다
When (1) 빠진 catalog로 OMP → balanced → GPT를 고른다. (2) placeholder 사본에서 OMP → balanced → GPT를 고른다. (3) 정상 catalog로 S8의 선택 뒤 확인에서 Enter를 준다. (4) 에이전트별 지정 → GPT 뒤 목록에서 완료를 바로 고른다
Then (1)은 `omp_profile_candidate_unavailable`과 `task`를 출력하고 확인 Select 없이 메뉴로 돌아간다. (2)는 `role_model_policy_placeholder_unsupported`를 출력하고 메뉴로 돌아간다
And (3)은 `취소했습니다. 설정을 바꾸지 않았습니다.`, (4)는 `No agent was pinned; nothing changed.`를 출력한다
And 네 경우 모두 activate는 0회이고 autopus.yaml SHA-256이 같다

### S10: 감지 플랫폼 열과 snapshot 공유
Priority: Must
Given DC와 DP, TTY probe true, CAT-A catalog probe가 있다
When DC와 DP에서 `auto update`를 줄 입력 Enter로 실행한다
Then DC 표의 머리말 칸은 차례로 `그룹`, `에이전트`, `등급`, `Claude Code`, `Codex`다. planner Codex 칸은 `6-astra · max`, footer는 `Codex catalog: 읽음 · 대체 0칸`이다. catalog probe는 1회, 가짜 `codex`의 `debug models`는 0회다
And DC의 같은 run에서 codex 생성이 성공하고 `.codex/agents/planner.toml`은 `model = "gpt-6-astra"`, `model_reasoning_effort = "max"`다(adapter가 스스로 CAT-D를 읽었다면 표준 배치 거부로 실패한다)
And DP 표의 플랫폼 열은 차례로 `Claude Code`, `Codex`, `OMP`, `OpenCode`이고 OMP와 OpenCode 칸은 모두 `플랫폼 기본값`이다

### S11: 유지 경로는 quality와 OMP 설정 bytes를 바꾸지 않는다
Priority: Must
Given DP, R 사본, O3 사본, TTY probe true가 있다
When 줄 입력 Enter, 폼 메뉴 유지, 구성 편집 → Codex → 역할 그룹 → Enter 4번 → 유지, 구성 편집 → Codex → 설계·판단 opus → 확인 Enter → 유지, OMP → 에이전트별 지정 → 완료 → 유지를 각 fixture에 따로 실행한다
Then 모든 run에서 quality 블록 bytes와 `role_model_policy` bytes(없으면 없음 그대로)가 실행 전과 같다
And DP의 파일 diff는 `platforms`에 감지 순서로 `- opencode`, `- omp`가 더해진 것뿐이고, R 사본과 O3 사본의 SHA-256은 같다

### S12: Ctrl+C
Priority: Must
Given R, 빈 temp project(init `--platforms claude-code,codex`, wizard seam 결과 `role-groups`), D, 정해진 순번의 form에서 `huh.ErrUserAborted`를 돌려주는 runner가 있다
When update의 메뉴, init의 도구 탭, quality의 확인 Select에서 각각 중단을 돌려준다
Then update는 `중단했습니다. 구성을 더 바꾸지 않습니다.`를 쓰고 `→ Updating claude-code...`부터 생성을 이어 가 종료 코드 0으로 끝나며, R의 SHA-256이 같다
And init은 `init cancelled.`를 쓰고 autopus.yaml, DESIGN.md, 플랫폼 파일을 만들지 않는다
And quality는 중단 줄을 쓰고 오류 없이 끝나며 D의 SHA-256이 같다

### S13: 기존 test는 고치지 않고 통과한다
Priority: Must
Given HEAD 44773355의 test 파일이 있다
When `go test ./pkg/config/... ./pkg/adapter/... ./internal/cli/...`를 필터 없이 돌리고 `git diff --stat 44773355 -- '*_test.go'`를 본다
Then 모든 test가 통과한다
And diff에는 새 test 파일만 있고 기존 `_test.go`의 변경 줄은 0이다

### S14: 라벨, 미리보기, 생성은 같은 snapshot의 같은 값이다
Priority: Must
Given D와 R, CAT-A와 CAT-B가 있다
When D와 CAT-B로 `auto quality --apply`에서 Codex 탭의 설계·판단에 opus를 고르고 적용한다. 또 D와 R의 Claude 탭·Codex 탭에서 그룹 4개 × 등급 4개의 선택지마다 후보 config를 만들고, 같은 snapshot으로 실제 Claude Code·Codex `Generate`를 temp dir에서 돌린다
Then CAT-B의 opus 선택지 라벨은 `6-sol · xhigh (대체)  [opus]`이고, 미리보기의 planner Codex 칸은 `6-sol · xhigh (대체)*`, footer는 `Codex catalog: 읽음 · 대체 4칸`이다. 생성된 `planner.toml`은 `model = "gpt-6-sol"`, `model_reasoning_effort = "xhigh"`다
And 모든 후보에서 Codex 라벨의 각 칸은 생성된 `.codex/agents/<agent>.toml`의 `model`(`gpt-` 제거)·`model_reasoning_effort`와 같다. Claude 라벨의 effort는 생성된 frontmatter의 `effort:`(없으면 `-`)와 같고, 라벨의 모델은 frontmatter 값이 alias면 `ClaudeModelForTier(alias)`, 정확한 ID면 그 ID에서 `claude-`를 뗀 것과 같다

### S15: 폼 표시의 터미널 안전
Priority: Must
Given `quality.default`가 `fast‮​`이고 같은 이름의 프리셋이 있는 D 사본, 이 branch의 소스 트리가 있다
When 폼 메뉴와 기본 프리셋 Select의 출력을 읽고, `internal/cli` 아래 test가 아닌 Go 파일에서 `HasDarkBackground`, `BackgroundColor(`, `termenv.NewOutput`을 찾는다
Then 출력에 U+202E, U+200B, ESC, BEL 원문이 없고 그 이름은 `terminalSafe` 표기로 나온다
And 검색 결과는 0건이다

### S20: 실제 터미널 smoke (SPEC-MODELUX-001 S21 개정판)
Priority: Must
Given macOS 실제 터미널, 이 branch로 만든 `auto`, `git init`한 빈 temp dir, PATH에 claude·codex·opencode·omp가 있고 `CODEX_HOME`을 scratch로 둔 셸이 있고, `script -q`로 기록한다
When `auto init --platforms claude-code,codex`에서 wizard quality 단계의 `역할별 직접 구성`을 고르고, 화살표와 Enter로 구성 편집 → Codex → 역할 그룹 → 설계·판단 `6.1-sol · xhigh  [opus]` → 나머지 Enter → 적용 → 유지를 고른다. 이어 `auto update`를 실행해 메뉴에서 Enter를 누른다. 마지막으로 `auto quality`에서 구성 편집 → OMP → balanced → GPT → 적용 → 유지를 고른다
Then init의 구성 단계가 wizard와 같은 모양의 huh 폼으로 나오고, transcript의 입력 echo에 `^[[C`, `^[[D` 같은 화살표 글자가 없다. 생성된 `.codex/agents/planner.toml`은 `model = "gpt-6.1-sol"`, `model_reasoning_effort = "xhigh"`, `.claude/agents/autopus/planner.md`는 `model: claude-fable-5-1`, `effort: max`이고, autopus.yaml에 `quality.providers.codex: codex-custom`이 있다
And `auto update`의 표에는 `+ 새 플랫폼 감지`에 나온 플랫폼의 열이 모두 있고 Codex 머리말은 `Codex (개별 설정: codex-custom)`이다. Enter 뒤 quality 블록 bytes가 같고 파일 diff는 `platforms` 추가뿐이다
And `auto quality`의 OMP 요약에 native agent 5행의 실제 route가 있고, 적용 뒤 파일에 `role_model_policy.profile: balanced`, `family: openai`가 있다. 설치된 `omp` catalog가 blocker를 내면 그 출력과 SHA-256 불변을 대신 기록한다
And 원본 transcript와, OSC·CSI를 지운 비교용 사본을 함께 `evidence/manual-smoke-002.txt`에 남긴다

### S21: init의 OMP 탭은 메모리 config로 같은 검사를 거쳐 저장된다
Priority: Must
Given IO 조건의 빈 temp project 세 개, 폼 seam, `initComposeOMPDeps`의 runner `ompCLIFakeRunner{catalog: ompCLIBalancedCatalogJSON()}`, `platformOMPModelRunner`의 가짜 runner, 호출을 세는 `deps.activate` stub이 있다
When 첫 project에서 구성 편집 → OMP → 에이전트별 지정 (balanced 위에) → GPT를 고르고, 목록에서 task → `openai-codex/gpt-6-astra` → thinking `max` → 완료, 확인에서 적용, 메뉴에서 유지를 고른다. 둘째 project는 `ompCLIBalancedCatalogWithout(t, "openai-codex/gpt-6-astra")` catalog로 OMP → balanced → GPT를 고른다. 셋째로 selector에 `${HOME}`이 든 후보 `role_model_policy`를 init 분기의 검사에 직접 넣는다
Then 첫 project의 미리보기는 `OMP balanced — model preview (no changes yet)`이고 `task*` 행에 `openai-codex/gpt-6-astra`와 `max`가 있다. 적용 뒤 `OMP 모델 선택을 init 설정에 넣었습니다. 파일 생성 때 함께 씁니다.`가 나온다
And 첫 project의 init이 끝난 autopus.yaml에서 `role_model_policy.profile`은 `balanced`, `family`는 `openai`, `agents.task.candidates`는 selector `openai-codex/gpt-6-astra`, thinking `max`, family `openai` 하나이고 나머지 native agent는 `agents`에 없다. `deps.activate`는 0회이고, OMP 생성은 `platformOMPModelRunner` 가짜 runner로 돌아 `models --json --no-extensions` 호출을 기록한다. `.claude/agents/autopus/planner.md`와 `.codex/agents/planner.toml`도 생성된다
And 둘째 project는 `omp_profile_candidate_unavailable`과 `task`를 출력하고 확인 없이 메뉴로 돌아가며, 끝난 autopus.yaml에 `role_model_policy` key가 없다. 셋째 입력은 `role_model_policy_placeholder_unsupported`로 거부되고 init cfg의 `RoleModelPolicy`는 그대로다
And 같은 cfg로 부른 `planOMPProfileForConfig`의 결과는, 같은 내용을 autopus.yaml로 쓴 project에서 부른 `planOMPProfile`의 결과와 deep-equal이다

### S22: init에서 OMP catalog를 읽지 못하면 안내하고 계속한다
Priority: Must
Given IO 조건의 빈 temp project, 폼 seam, `initComposeOMPDeps`와 `platformOMPModelRunner`에 같은 `ompCLIFakeRunner{fail: true}`가 있다
When 구성 편집 → OMP → balanced → GPT를 고르고, 다시 구성 편집 → OMP → 에이전트별 지정 (balanced 위에) → GPT를 고른 뒤 메뉴에서 유지를 고른다
Then 두 번 모두 `OMP catalog를 읽지 못해 지금은 OMP 모델을 지정할 수 없습니다. init 뒤 auto quality에서 지정할 수 있습니다.`와 원인 한 줄이 나오고 메뉴로 돌아간다
And init은 종료 코드 0으로 끝나고 autopus.yaml에 `role_model_policy` key가 없으며, `.claude/agents/autopus/planner.md`와 `.codex/agents/planner.toml`이 생성된다. profile이 빈 OMP 생성은 model integration을 건너뛰므로(`omp_model_integration.go:38-40`) `platformOMPModelRunner` 호출은 0회다

### S16: override 질문 폼
Priority: Should
Given P가 있다
When 구성 편집 → 모든 도구 같게 → 역할 그룹, 설계·판단 opus, 나머지 Enter 뒤 나오는 질문에 한 번은 Enter, 한 번은 `y`로 답한다
Then 미리보기 전에 `Codex도 새 구성을 따를까요? (quality.providers.codex = codex-custom 제거)`와 `[y/N]`이 나온다
And Enter면 Codex 머리말이 `Codex (개별 설정: codex-custom)` 그대로이고 바뀌는 설정에 providers 줄이 없다. `y`면 `quality.providers.codex 제거` 줄이 있고 Codex 열이 custom 값으로 바뀌어 `*`가 붙는다

### S17: locale
Priority: Should
Given D가 있다
When `composeLanguage`를 `en`과 `ko`로 두고 폼 메뉴, 도구 탭, 확인 Select를 연다
Then en은 `keep`, `edit the composition`, `same for all tools`, `Apply?`를, ko는 `유지`, `구성 편집`, `모든 도구 같게`, `적용할까요?`를 출력한다
And 두 경우 모두 저장되는 `codex-custom` description은 `Codex custom agent tiers`다

### S18: OSC 11 기록
Priority: Should
Given `research.md` `## OSC 11 조사`, CHANGELOG v0.50.125, S20 기록이 있다
When 기록과 smoke 처리 절차를 읽는다
Then research.md와 CHANGELOG는 원인 위치(bubbletea v1.3.6 `tea_init.go`의 package `init()`)와 프로그램 안에서 고치지 않는 이유를 적는다
And S20 원본에는 `ESC ] 11 ; ?`가 있고 비교용 사본에는 OSC와 CSI가 없으며, S20의 판정은 비교용 사본으로 한다

### S19: 문서
Priority: Should
Given CLI help, README, CHANGELOG가 있다
When `auto init --help`, `auto update --help`, `auto quality --help`를 실행하고 문서를 읽는다
Then 세 help에 화살표·Enter 폼, 도구 탭, `<provider>-custom`과 `quality.providers` 저장, OMP 탭(파일에 설정된 omp만), 라벨과 표의 Claude 표기 차이가 있다
And README와 v0.50.125 CHANGELOG에 같은 내용이 있고, `quality_compose_docs_test.go`가 고정한 기존 문장이 그대로 있으며, `make generate-templates` 뒤 생성 루트에 반영되지 않은 diff가 없다

## Oracle Acceptance Notes

- 모든 Must 시나리오는 concrete expected output을 갖는다. 라벨 문자열, 선택지 번호 목록, 칸 값, 머리말, 바뀌는 설정 줄, 안내·중단·거부 문구, SHA-256과 블록 bytes 동일성, 파일 key 값, 생성 파일의 `model`·`effort`, probe·activate·폼 실행 횟수다. 파일 존재나 종료 상태만으로 닫는 Must 시나리오는 없다.
- 라벨의 oracle은 같은 snapshot으로 돈 실제 adapter `Generate`다(S14). Claude 라벨의 모델은 slug라서 frontmatter와 직접 같지 않다. S14가 그 대응 규칙(`ClaudeModelForTier`)을 oracle로 쓴다.
- 기대 값은 2026-10-12에 코드를 읽고 계산했다(`codex_profile.go:105-124`, `quality_effective.go:28-65`, `quality_tier.go:94-114`, `role_model_policy_balanced.go:21-69`, `content/agents/*.md`의 effort, `quality_compose_table_render.go:107-171`). 실행으로 확인한 것은 probe A1뿐이다. 코드 사실과 다르면 test가 아니라 이 SPEC을 고친다.
- S21의 OMP 생성 확인(가짜 runner 기록)은 설정 되읽기에 답하는 가짜 runner가 있어야 한다(R4). 못 만들면 그 문장은 "저장 직전 init cfg의 `role_model_policy`가 위 값과 같다"로 바꾸고 review에 알린다. S22에서 profile이 빈 OMP 생성이 runner 밖에서 `omp`를 실행하는지는 미확인이다.
- S11과 S20의 블록 경계는 SPEC-MODELUX-001 REQ-MUX-08 규칙이다. probe A1은 같은 경계를 `quality:` 줄부터 다음 1열 key 직전까지로 잘라 비교했다.
