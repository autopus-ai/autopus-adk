# SPEC-HARNESS-BENCH-002 수락 기준

## Test Scenarios

S1–S16, S18, S19는 model 호출과 GitHub network 없이 `scripts/benchmarks/harness/test_feature_*.py`와 `feature_goapi/main_test.go`에서 돈다. 공통 fixture는 다음과 같다.

- **work repo**: `<tmp>/work`. `<tmp>/origin.git`(bare)에 push된 git repo이고 `GIT_CONFIG_GLOBAL=/dev/null`, `GIT_CONFIG_NOSYSTEM=1`로 다룬다.
- **가짜 `claude`**: `--version`에 `2.1.314 (Claude Code)`를 출력한다. 그 밖의 호출은 argv를 `<tmp>/calls.log`에 남긴다. "model 호출 0"은 `calls.log`가 없다는 뜻이다.
- **runner**: `feature.main([...], steps)`를 부른다.

### S1: AC-HB2-001 사전등록 거부 코드
Priority: Must
Given 사전등록이 commit·push되어 있고 모든 digest, 순서, version, usage probe receipt가 runner 재계산값과 같다
When 아래 각 변형으로 runner를 실행한다
Then 각 실행의 stdout은 JSON 한 줄이고 종료 코드는 1이며 `calls.log`가 없다. 예상 출력은 아래와 같다
And `protocol.json`이 바이트 `X`로 있으면 `protocol_exists`이고 파일은 여전히 `X`다. prereg가 함께 없어도 `protocol_exists`다. prereg가 없으면 `prereg_missing`, working tree에서 공백 하나를 더하면 `prereg_dirty`, commit만 하고 push하지 않으면 `prereg_unpublished`다. local `origin/main`을 `git update-ref`로 옮겨도 `prereg_unpublished`다
And `usage_probe`의 agy receipt가 빠지면 `usage_probe_missing`이고 detail은 `agy`다
And `order`의 f02 항목을 뒤집어 push하면 `prereg_digest_mismatch`이고 detail은 `order`다. `runner_sha256`을 `"0"*64`로 바꾸면 detail은 `runner_sha256`이다. `auto_source_commit`을 사전등록 commit 자신이나 그 뒤 commit으로 적으면 detail은 `auto_source_commit`이고, C archive를 Steps build로 다시 만든 SHA-256이 `auto_sha256`과 같으면 통과한다. `claude --version`이 `2.1.999 (Claude Code)`이면 `cli_version_mismatch`이고 detail은 `claude: installed '2.1.999 (Claude Code)', pin '2.1.314 (Claude Code)'`다
And 통과한 control 실행의 `protocol.json`은 `prereg_commit`이 prereg를 마지막으로 바꾼 commit이고, `remote_main_sha`가 `git ls-remote` 값이다.

### S2: AC-HB2-002 기계적 외부 corpus 선택
Priority: Must
Given 기록된 메타데이터 fixture에 후보 c1–c18이 있다. 각 후보는 repo, PR, `created_at`, commit 시각, `merged_at`, author type, files, license, 본문 canary `BODY-CANARY-91`을 갖는다
When `feature_select`를 window 시작 `2026-07-01T00:00:00Z`, window end `2026-11-01T00:00:00Z`로 실행한다
Then 제외 reason은 다음과 같다. c1(`merged_at` 2026-06-30T12:00:00Z)은 `before_window:created_at`이다. c17(`created_at` 2026-06-20, `merged_at` 2026-07-10)도 `before_window:created_at`이고, c18(`created_at` 2026-07-05, commit author date 2026-06-28)은 `before_window:commit`이다
And c2(`dependabot[bot]`, type `Bot`)는 `bot_author`, c3(`GPL-3.0`)은 `license_not_allowed:GPL-3.0`, c4(production 2개)는 `prod_files_out_of_range:2`, c5(test 변경 없음)는 `no_test_change`, c6은 `go_mod_changed`다
And c7(internal `TestX` 본문은 helper `check()`만 부르고 `check`가 새 unexported `parseUnit`을 부름)은 `unexported_api:parseUnit`이다. c16(`git diff --no-renames merge^1 merge` 목록이 PR 파일 목록과 다름)은 `ambiguous_parent`다
And 적격 c8–c15 가운데 `merged_at` 오름차순(같으면 repo 이름, 그다음 PR 번호) 앞의 6개 c8, c9, c10, c11, c12, c13이 f01–f06이 된다. c14와 c15는 `beyond_first_6`이다
And 출력 manifest의 각 task는 SPDX id와 allowlist 결정 `allowed`를 갖는다. 출력 JSON 어디에도 `BODY-CANARY-91`과 fixture 소스의 줄이 없다.

### S3: AC-HB2-003 두 구성 calibration, sandbox 밖 실행 0
Priority: Must
Given 외부형 fixture repo가 있다. parent P에는 `dur.go`가 있고, merge M은 `ParseDuration`과 `parse_test.go`(`TestParseDuration`, package `dur_test`)를 더한다
When calibration을 실행한다
Then 행은 `{"task_id":"f01","merge_accepted":[true,true],"parent_accepted":false}`이고 manifest grade packages는 `["."]`, 채점 argv는 `go test -json -count=1 ./`다. 판정은 `feature_grade.judge_pairs`가 하고 `grader_argv`에 넘긴 `prepared['warm_cache']` 디렉터리는 비어 있다
And Steps가 기록한 sandbox 밖 process argv는 `git` 명령, `go mod download -json`, `go mod verify`뿐이다. `go test`, `go build`, `go vet`, `go list`는 하나도 없다
And Steps의 grade가 reference 구성에서 `accepted` 다음 `oracle_failed`를 돌려주면 `calibration_failed:flaky`, parent 구성에서 `accepted`를 돌려주면 `calibration_failed:parent`다. 동결 task가 runner preflight에서 같은 검사에 실패하면 stdout은 `{"status":"refused","reason":"oracle_calibration_failed","detail":"f01"}`이고 `calls.log`가 없다
And f01 workspace에는 `dur.go`가 있다. `parse_test.go`, `.git` 과거 history, `CLAUDE.md`, `.autopus/`는 없다.

### S4: AC-HB2-004 기계적 prompt 도출
Priority: Must
Given PR 제목 `Add ParseDuration helper`와 본문이 있다. 본문에는 HTML comment, 문장 `This adds ParseDuration so callers can parse "1h30m" style strings.`, 줄 `Covered by TestParseDuration in parse_test.go.`, production 추가 줄 `return parseUnits(s)`를 담은 go code block, `- [x] tests added`, image markdown, `/pull/12/files` 링크 줄이 있다
And `feature_goapi`의 API 출력은 `example.com/dur: func ParseDuration(s string) (time.Duration, error)`다
When prompt를 도출한다
Then 예상 출력은 정확히 아래 여덟 줄이다(빈 줄 3개 포함)

```text
Implement the following change in this repository.

Title: Add ParseDuration helper

This adds ParseDuration so callers can parse "1h30m" style strings.

The acceptance tests call this public API:
- example.com/dur: func ParseDuration(s string) (time.Duration, error)
```

And 템플릿 결과 바이트는 `\n` 하나로 끝나고, 그 SHA-256이 manifest prompt SHA-256이다
And 본문이 `fixes: #7`뿐이고 issue #7이 제목 `Support durations`, 본문 `Need a parser for 1h30m.\nSee TestParseDuration.`이면 description은 `fixes: #7\n\nIssue: Support durations\n\nNeed a parser for 1h30m.`이다(issue 본문에도 규칙 3 적용). 같은 본문에 `Closes acme/dur#7`이 더 있어도 참조는 하나다. `Fixes #7, fixes #8`이면 issue를 붙이지 않는다
And 두 arm 모두 `<trial>/task/TASK.md` 바이트가 같고 SHA-256이 manifest 값과 같다. harness prompt 인자는 정확히 `/auto plan <F> --auto`, native는 `<F>`이며 `<F>`에 `--`가 없다
And 본문에 `fzf --multi --target x`가 있는 변형에서 그 문자열은 `TASK.md`에 그대로 있고 어느 argv에도 없다.

### S5: AC-HB2-005 harness arm (`auto init`, overlay, 경로)
Priority: Must
Given 가짜 `auto`와 가짜 `claude`가 있다. 가짜 `claude`는 `/auto plan`에서 `SPEC-FAKE-001`(`profile: full`, `change_class: feature`)을 만들고 `auto spec review SPEC-FAKE-001`을 1회 실행한다. 이때 `review.md`에는 `**Verdict**: PASS`, `**Revision**: 2`가 쓰인다. 부모 환경은 `LANG=ko_KR.UTF-8`이고 PATH에는 `codex`와 `agy`가 없다
When f01 harness trial을 실행한다
Then `<session>/receipts/<trial>/auto.jsonl` 첫 행의 argv는 `init --yes --dir <ws> --platforms claude-code --quality ultra --no-rea --statusline-mode keep`이고, 그 process 환경은 REQ-HB2-22 allowlist(`LANG=en_US.UTF-8`, `HOME`은 trial config home)다
And overlay 뒤 `autopus.yaml`의 `orchestra.providers.codex.args`는 `[exec, --json, --sandbox, workspace-write, -m, gpt-6-astra, -c, 'model_reasoning_effort="max"']`, gemini args 끝은 `--output-format, json`, claude args 끝은 `--output-format, json`이다. `spec.review_gate.enabled`는 `true`, `language`는 `en/en/en`이다. 생성된 `CLAUDE.md` Language Policy의 세 줄은 모두 `English (en)`로 끝나 overlay와 같다
And `calls.log` 두 번째 줄의 prompt 인자는 `/auto go SPEC-FAKE-001 --auto`다
And `harness_path`는 `{"spec_id":"SPEC-FAKE-001","change_class":"feature","profile":"full","review_invocations":1,"review_rounds":3,"review_verdict":"PASS","go_entered":true}`다
And SPEC을 만들지 않는 변형은 `operational_error: plan_no_spec`, `go_entered: false`이고, SPEC이 2개인 변형은 `plan_ambiguous_spec`이다.

### S6: AC-HB2-006 native audit, config home 격리와 credential 수명
Priority: Must
Given 두 arm의 trial이 있고 seed한 가짜 인증 파일에 문자열 `oat-CANARY-55aa66bb77cc`가 있다
When 오염 변형마다 trial을 시작한다: (a) 출력 경로 조상에 `CLAUDE.md`, (b) config home에 `.claude/skills/x/SKILL.md`, (c) config home에 `.claude/projects/p/memory/MEMORY.md`, (d) 동결 바이트와 다른 `.claude/settings.json`
Then (a)의 native trial은 `operational_error: "trial_refused:native_surface_present"`이고 session `stop_reason`이 같다. (b)–(d)는 두 arm 모두 `config_home_contaminated`이고 detail은 해당 경로다. 모두 `calls.log`가 없다
And 외부 repo에 `sub/AGENTS.md`가 있는 변형은 두 arm workspace 모두에서 그 파일이 없고 manifest 제거 경로에 `sub/AGENTS.md`가 있으며 trial은 거부되지 않는다
And 깨끗한 trial의 `HOME`은 `<trial>/home`, `CLAUDE_CONFIG_DIR`은 `<trial>/home/.claude`이고 agent 환경에 `CODEX_HOME`이 없다. codex·agy 인증은 `<session>/broker-home/<trial>`에만 있고 agent shell의 그 경로 읽기는 실패한다. `--mcp-config` 파일 내용은 `{"mcpServers":{}}`이며 `--strict-mcp-config`가 있다. native에서 `command -v auto`는 종료 코드 1이고 `<session>/pinned/auto` 읽기는 실패한다. harness에서는 `<session>/auto-shim/auto`를 출력하고 그 파일과 `<session>/pinned/auto` 쓰기는 실패한다
And 가짜 agent가 인증 파일을 transcript에 출력한 trial이 끝나면 `<trial>/home`이 없고, 보존 파일 어디에도 `oat-CANARY-55aa66bb77cc`가 없으며 transcript에는 `[REDACTED]`가 있고 `credential_redactions`는 1이다. 가짜 CLI가 trial 중 인증 파일 값을 `oat-ROTATED-99dd88ee77ff`로 바꾸고 agent가 그 값과 파일에 없는 `sk-ant-oat01-ABCDEFGHIJKLMNOPQRSTUVWXYZ012345`를 출력한 변형은 세 문자열이 모두 보존 파일에 없고 `credential_redactions`는 3이며, master 인증 파일 바이트는 trial 전과 같다(되쓰기 없음)

### S7: AC-HB2-007 동등 argv와 seeded 순서
Priority: Must
Given task f01–f06의 corpus 행이 `["acme/dur", 100+i, "<i를 40번>", "<abcdef의 i번째 글자를 40번>"]`(i = 1..6)이다
When `corpus_sha256`, 순서, argv를 만든다
Then `corpus_sha256`은 `779c55c10d9c81a9e7a99e0ca696a0b51cd6bb16dd327b72c7fdd88d53b93b14`이고, 행 순서를 뒤섞거나 `updated_at` 같은 field를 더한 메타데이터로 다시 만들어도 같다
And 예상 순서는 `f01 native, f01 harness, f02 native, f02 harness, f03 harness, f03 native, f04 harness, f04 native, f05 harness, f05 native, f06 native, f06 harness`이다(key가 가장 작은 f04, f03, f05가 harness 먼저)
And 각 task의 두 arm argv와 환경에서 trial root를 `<TRIAL>`로 바꾸면, argv 차이는 prompt 인자 하나이고 환경 차이는 harness `PATH` 맨 앞의 `<session>/auto-shim` 항목 하나이고, sandbox 차이는 `auto` 디렉터리를 여는 profile parameter 하나다. `HOME`, `CLAUDE_CONFIG_DIR`, `TMPDIR`, `GOCACHE`, `--mcp-config` 값은 정규화 뒤 같다
And 가짜 machine fingerprint가 run 5 직전에 바뀌면 records는 4행이고 session 종료 사유는 `machine_changed`다. `codex --version`이 바뀌면 `cli_version_changed`다.

### S8: AC-HB2-008 sandbox, 환경, egress (macOS)
Priority: Must
Given `feature_agent.sb`, 실행 중인 `feature_egress.py`(allowlist `127.0.0.1:<test port>`), oracle canary, trial B, `grades.json`, arm map, 부모 환경의 `GH_TOKEN=canary-gh`와 `AWS_SECRET_ACCESS_KEY=canary-aws`가 있다
When trial A의 shell이 읽기, 쓰기, `env`, `curl`을 실행한다
Then oracle canary, trial B, `grades.json`, arm map, `--repo` `go.mod`, `~/.ssh/config`, 실제 `~/.claude/settings.json`, `~/go/pkg/mod` 아래 파일 읽기는 모두 실패한다. task module cache 읽기는 성공한다. trial A workspace 쓰기는 성공하고 `<out>/records.json`과 module cache 쓰기는 실패한다. top-level 역할 pid의 proxy 경유 `127.0.0.1:<test port>` 요청은 200이고, 같은 요청을 trial A shell(자식 pid)이 하면 거부되어 `control_violations`에 `unbrokered_egress:127.0.0.1:<test port>`가 남는다. `<session>/cli-broker/codex` 실행과 `<session>/receipts/` 읽기·쓰기는 실패한다. `https://github.com` 요청은 실패하고 proxy 기록은 `{"egress_denied":{"github.com:443":1}}`이다. proxy 없이 `curl --noproxy '*' http://203.0.113.1`을 하면 sandbox가 거부한다
And `env`의 key 집합은 REQ-HB2-22 allowlist와 정확히 같고 `GH_TOKEN`, `AWS_SECRET_ACCESS_KEY`, `GIT_` 변수가 없다. `GOCACHE`는 비어 있던 `<trial>/gocache`다
And trial record의 `control_violations`에는 oracle canary 경로의 `isolation:` 항목이 있고, `/private/var/folders` 아래 읽기 거부는 `sandbox_denied_other` 수만 늘린다. trial 뒤 남은 process는 0개다
And `--output`을 옛 거부 root 밖인 `/opt/homebrew/var/tmp/fb-<random>`(없으면 `FEATURE_TEST_OUTSIDE_ROOT`가 가리키는 쓰기 가능한 디렉터리, 둘 다 없으면 skip이 아니라 fail)에 둔 변형에서도 위 읽기는 모두 실패하고 `<session>/receipts/`, `<session>/broker-home/`, 이웃 trial의 patch 읽기도 실패한다. native parameter로는 `<session>/pinned/auto` 읽기가 실패한다. 같은 변형에서 profile을 rev 5식 부분 거부(account home, `/Users`, `/Volumes`, temp root만)로 바꾸면 preflight가 `{"status":"refused","reason":"sandbox_policy_leak","detail":"<session>/receipts/canary"}`를 출력하고 `calls.log`가 없다.

### S9: AC-HB2-009 blind 채점
Priority: Must
Given 12 trial이 끝났고 f01 harness patch 내용은 다음과 같다: 올바른 `ParseDuration`, 추가 `mine_test.go`, `.autopus/specs/`, oracle 경로 `parse_test.go`의 `t.Skip()` 버전
When 채점 단계를 실행한다
Then Steps 접근 기록은 grade 12회, `grades.json` 쓰기, arm map 읽기 순서이고, 채점 순서는 opaque id 오름차순이다
And grade 호출 인자와 `grades.json`의 각 행에 `arm` key와 `harness`/`native` 문자열이 없다
And f01 harness 채점 사본에는 후보 `dur.go`가 있다. `mine_test.go`와 `.autopus/`는 없다. `parse_test.go`는 oracle 바이트다. join된 record는 `delivered_pass: true`, `hidden_passed: 1`, `hidden_total: 1`이다
And symlink 변형은 `scope_violation: ["dur_link.go"]`, `go.mod` 변형은 `scope_violation: ["go.mod"]`, deadline 변형은 `delivered_pass: false`, `operational_error: "agent_timeout"`, `usage_status: "unparsed"`, `usage_unparsed_calls: ["claude#1"]`이다
And 기대 쌍이 `[["example.com/dur/a","TestParse"],["example.com/dur/b","TestParse"]]`이고 a의 `TestParse`는 pass, b의 `TestParse`는 skip인 변형은 signal이 `accepted`가 아니며 `hidden_passed: 1`, `hidden_total: 2`, `delivered_pass: false`다.

### S10: AC-HB2-010 usage 정규화, null 금지
Priority: Must
Given receipt가 넷 있다. top-level Claude stream-json `result` usage는 `{"input_tokens":1200,"cache_read_input_tokens":30000,"cache_creation_input_tokens":4000,"output_tokens":900}`이다
And review claude JSON usage는 `{"input_tokens":300,"cache_read_input_tokens":2000,"cache_creation_input_tokens":0,"output_tokens":150}`이다. codex `turn.completed`는 `{"input_tokens":50000,"cached_input_tokens":42000,"output_tokens":3000}`와 `{"input_tokens":10000,"cached_input_tokens":9000,"output_tokens":500}`이다
And agy JSON은 `{"stats":{"input_tokens":700,"cached_tokens":100,"output_tokens":120}}`이다. fixture schema는 input이 cached를 포함하고 cache write는 `not_applicable`이라고 선언한다
When 정규화한다
Then 예상 값은 claude top `{1200,30000,4000,900}`, review claude `{300,2000,0,150}`, codex `{9000,51000,"not_applicable",3500}`, agy `{600,100,"not_applicable",120}`이다(순서는 uncached, cache read, cache write, output)
And run 합계는 `102870`이고 JSON 어디에도 `null`이 없다
And agy stdout이 text `LGTM`인 변형은 `usage_status: "unparsed"`, `usage_unparsed_calls: ["agy#1"]`이다. 첫 codex call의 `turn.completed` 줄에 중복 key가 있는 변형은 `usage_unparsed_calls: ["codex#1"]`이다. 두 변형 모두 `operational_error`는 `none`이고 합계 field와 `null`이 없다. usage 없는 `codex debug models`, `codex login status`, `claude auth status --json` 호출이 섞여도 값과 번호는 같고 `aux_calls`만 3이 된다.

### S11: AC-HB2-011 wall time 구간
Priority: Must
Given 가짜 monotonic clock 값이 있다: `auto init` 0–20, plan 30–130, go 130–280, grading 400–500. `auto` shim의 spec review 기록은 50–70과 150–160이다
When record를 만든다
Then 예상 값은 `elapsed_seconds: 250.0`, `phase_seconds: {"plan":100.0,"go":150.0,"review":30.0}`이다
And deadline 10800초로 끝난 native run은 `elapsed_seconds`가 10800.0 이상 10810.0 이하다(explicit tolerance 10초).

### S12: AC-HB2-012 review defect
Priority: Must
Given `review-findings.json`에 `critical/open`, `major/resolved`, `minor/resolved`가 있다(object 형식과 legacy 배열 형식 각각)
When record를 만든다
Then harness `review`는 `{"label":"reported","by_severity":{"critical":1,"major":1,"minor":1},"by_status":{"open":1,"resolved":2},"verified":"unknown"}`이고 native는 `"n/a"`다. hidden 5개 중 2개가 실패하면 `escaped_defects`는 2다.

### S13: AC-HB2-013 R1 평가기 (paired task 비교)
Priority: Must
Given 12-run 결과표가 있다. H는 harness pass 집합, N은 native pass 집합이다. 따로 적지 않으면 run은 모두 기록됐고 infra failure가 없다
When `feature_decision.evaluate`를 실행한다
Then 예상 출력은 아래와 같다
And A(H=f01–f05, N=f01–f03)는 `claim_allowed: true`, `reasons: []`다. B(H=f01–f05, N=f01,f02,f06)는 `["reversal:f06"]`이다
And C(H=f01–f04, N=f01–f03)는 `["margin_below_2:4-3"]`이다. D(A에서 f04-native가 `provider_rate_limited`)는 `["primary_undetermined:f04-native"]`이다
And E(A에서 f06-harness가 없음)는 `["runs_missing:f06-harness"]`이다. F(H=f01, N=f03,f04, f02-harness가 `provider_auth_failed`)는 `["primary_undetermined:f02-harness","margin_below_2:1-2","reversal:f03","reversal:f04"]`이다
And G(A에서 f02-native가 `usage_status: "unparsed"`)는 `["usage_unparsed:f02-native"]`이고 `claim_line: null`이다
And H(A에서 f03-harness에 `tool_policy:WebSearch`, f05-native가 `trial_refused:config_home_contaminated`이고 f06이 없음)는 `["runs_missing:f06-harness","runs_missing:f06-native","control_violation:f03-harness","primary_undetermined:f05-native","margin_below_2:4-3","reversal:f03"]`이다.

### S14: AC-HB2-014 보고서 생성
Priority: Must
Given 표 A가 있다. harness run은 각각 S10 usage(합계 102870)와 elapsed 6120.0이고, native run은 각각 claude usage 41000과 elapsed 2280.0이다. 날짜는 2026-11-20이다. records에는 canary `PROMPT-CANARY-7f3a`, `BODY-CANARY-91`, `/Users/alice`, `sk-test-CANARY`, fixture 소스 한 줄이 있다
When 보고서를 생성한다
Then JSON 예상 값은 `inputs.token_ratio: 2.51`, `inputs.time_ratio: 2.68`, `claim_allowed: true`이고 JSON에 `null`이 없다
And `claim_line`은 `Pilot (n=6 tasks from external open-source Go repositories merged after the model knowledge cutoff, 1 run per arm, no statistical significance): the full Autopus path passed 5/6 hidden-test suites vs 3/6 for native Claude Code, using 2.51x tokens and 2.68x wall time. Details: [report](docs/benchmarks/harness-feature-2026-11-20.md).`과 같다
And Markdown 예상 출력에 행 `| f01 | pass | pass | 102,870 | 41,000 | 6,120.0 | 2,280.0 |`, `| agy | harness | 6 | 3,600 | 600 | not_applicable | 720 |`, `| agy | native | 0 | 0 | 0 | not_applicable | 0 |`가 있다
And `report_json_sha256:` 값은 JSON 바이트의 SHA-256이다. 처치·남은 편향 절에 `codex`, `agy`, `treatment`와 사전등록의 남은 편향 항목이 모두 있다
And 다섯 canary는 JSON에도 Markdown에도 없다.

### S15: AC-HB2-015 README 가드
Priority: Must
Given S14 보고서가 있다
When 가드 test를 실행한다
Then 링크가 없는 README는 통과한다. `claim_line`과 같은 한 줄이 있는 README도 통과한다
And `2.51x`를 `2.50x`로 바꾼 줄, `claim_allowed`를 false로 바꾼 JSON(digest 불일치), 표 C 보고서를 링크하는 줄은 실패한다.

### S16: AC-HB2-016 보존과 외부 코드 비commit
Priority: Must
Given 이 SPEC의 branch가 있다
When `plan.md` `## Verification`을 실행한다
Then Python과 Go test가 통과하고 기존 test 파일은 수정되지 않았다
And `git diff origin/main -- scripts/benchmarks/harness/report.py scripts/benchmarks/harness/golden_protocol.py`와 `git diff --name-only origin/main -- pkg internal content templates evals/harness`는 비어 있다
And `git diff --name-only origin/main`의 모든 경로는 `scripts/benchmarks/harness/`, `docs/benchmarks/harness-feature-`, `README.md`, `.autopus/specs/SPEC-HARNESS-BENCH-002/` 아래다. 그리고 `feature_prereg.json`에는 PR 본문과 외부 소스 줄이 없다(T11 산출물 검사).

### S17: AC-HB2-017 OPS pilot과 공개
Priority: Must
Given T11 사전등록이 origin/main에 있다
When maintainer가 T12와 T13을 실행한다
Then `prereg_commit`은 `remote_main_sha`의 ancestor이고 첫 run 시작보다 먼저다. records는 12행이고 순서는 사전등록 `order`와 같다. 모든 행의 machine fingerprint와 version이 같다
And `grades.json` 쓰기 시각은 arm map 첫 읽기 시각보다 이르다. 보고서 두 파일이 commit되었고 Markdown digest는 JSON 바이트와 같다
And `claim_allowed`가 false면 `git diff <prereg_commit> -- README.md`는 비어 있다. true면 그 diff는 `claim_line`과 같은 한 줄 추가뿐이다.

### S18: AC-HB2-018 거부 tool과 outcome 분류
Priority: Must
Given 끝난 trial의 transcript·receipt fixture와 분류기가 있다
When `feature_outcome`이 각 변형을 분류한다
Then top-level stream-json에 `parent_tool_use_id`가 있는 subagent message의 `{"type":"tool_use","name":"WebSearch"}`가 있으면 `control_violations: ["tool_policy:WebSearch"]`, `operational_error: "control_violation"`, `delivered_pass: false`다. 그 tool 결과가 오류여도 같다. codex receipt에 `{"type":"item.completed","item":{"type":"web_search"}}`가 있으면 `control_violations: ["tool_policy:codex.web_search"]`다
And `TASK.md` 바이트가 바뀐 trial은 `["task_file_modified"]`다. catalog fallback으로 codex model call argv의 `-m`이 `gpt-6-astra`가 아니면 `["model_substituted:codex"]`이고, 그 run의 `aux_calls`의 `codex debug models` 항목은 exit code와 시간을 갖는다
And review 전에 `codex debug models`, `claude auth status --json`, `codex login status`가 돌고 첫 codex `exec` call이 rate limit error event를 냈고 run은 deadline으로 끝난 변형은 `operational_error: "provider_rate_limited"`, `aux_calls` 3개, `provider_errors: ["codex#1:rate_limited"]`이고 R1 입력에는 `primary_undetermined`와 deadline fail이 함께 남는다. tool 이름이 `Read`, `Bash`, `Task`뿐인 깨끗한 transcript는 `control_violations: []`, `operational_error: "none"`이다. broker 순번이 1, 2, 4이거나, 2번에 종료 기록이 없거나, 3번 receipt가 parse되지 않는 변형은 각각 `["observation_incomplete"]`, `delivered_pass: false`이고 R1 reason은 `control_violation:<task>-<arm>`이다
And broker 변형(가짜 codex는 `turn.completed` usage `{"input_tokens":1000,"cached_input_tokens":400,"output_tokens":50}` 한 줄 뒤 60초 잔다): shim client의 process group을 SIGKILL하면 2.0초 안에 CLI process group이 0개가 되고 종료 기록은 `{"seq":1,"terminated_by":"client_gone","exit":-9}`, 그 call usage는 `{600,400,"not_applicable",50}`로 합계에 들어가며 run은 `usage_status: "unparsed"`, `usage_unparsed_calls: ["codex#1"]`이다. 같은 CLI가 trial 끝까지 돌면 종료 기록은 `{"seq":1,"terminated_by":"trial_end","exit":-9}`이고 `control_violations`에 `observation_incomplete`가 없다. shim cwd가 `/private/tmp`이거나 trial 밖을 가리키는 symlink이면 CLI는 시작되지 않고 shim exit은 126, 기록은 `{"seq":1,"refused":"cwd_outside_trial","cwd":"/private/tmp"}`, `control_violations: ["broker_cwd_refused"]`다. trial workspace cwd이면 가짜 codex가 받은 argv는 동결 argv와 같고 `--skip-git-repo-check`가 없다.

### S19: AC-HB2-019 oracle 구성 (이질 PR)
Priority: Must
Given 세 fixture PR이 있다. P1은 production `fmt.go`와 `fmt_test.go`, 새 `testdata/golden.txt`를 바꾸고 test가 `testdata/golden.txt`를 읽는다. P2는 `old_test.go`를 `new_test.go`로 rename하며 `TestOld`를 `TestNew`로 바꾼다. P3는 `a/x_test.go`와 `b/x_test.go`에 같은 이름 `TestParse`를 더한다
When 각 PR의 oracle을 분류하고 reference·parent 구성으로 calibration한다
Then P1의 oracle 경로는 `fmt_test.go`(`M`), `testdata/golden.txt`(`A`)이고 production 경로는 `fmt.go`다. reference 구성 tree id는 merge tree id와 같고 calibration 행은 `{"merge_accepted":[true,true],"parent_accepted":false}`다
And testdata를 뺀 후보 patch(올바른 `fmt.go`)의 채점 사본에는 overlay가 쓴 `testdata/golden.txt`가 있고 `delivered_pass: true`다
And P2의 overlay는 `old_test.go`를 지우고 `new_test.go`를 쓴다. 채점 사본에 `old_test.go`가 없고 중복 선언 build 실패가 없으며 `expected_tests`는 `[["example.com/m","TestNew"]]`다. P3의 `expected_tests`는 `[["example.com/m/a","TestParse"],["example.com/m/b","TestParse"]]`, grade packages는 `["a","b"]`, 채점 argv의 package 인자는 `./a ./b`, `hidden_total`은 2다. P1의 grade packages는 `["."]`이고 인자는 `./`다.

## Oracle Acceptance Notes

모든 Must scenario는 concrete fixture와 예상 값을 짝짓고, 종료 코드나 파일 존재만으로 닫히는 것은 없다. 2.51 = 617220/246000, 2.68 = 36720/13680이다. INV 연결은 `research.md` `## Semantic Invariant Inventory`에 있다.

- S8은 `test_golden_sandbox_isolation.py`처럼 darwin `skipUnless`로 감싸고, 완료 증거는 macOS에서 `-v`로 본 `ok`다(`skipped`는 아님). explicit tolerance는 S11의 10초 하나뿐이다.
