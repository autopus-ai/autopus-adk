# SPEC-HARNESS-BENCH-002: Claude Code 전체 harness 경로 vs native 단일 세션 — 외부 OSS 중간 규모 기능 pilot benchmark

**Status**: draft
**Created**: 2026-11-04
**Revised**: 2026-11-06 rev 7. T0 결과와 사용자 결정 D1(review panel claude+codex), D2(review codex는 자체 sandbox)를 반영하고 rev 6 fresh review finding을 해소했다. 공개에 그친 편향은 통제로 바꾸거나 Bias Ledger에서 `잔여`로 낮췄다. 결정 원문은 `research.md` `## User Decisions (rev 2)`, `## T0 결과와 결정 (rev 7)`에 있다.

**Domain**: HARNESS-BENCH
**Change Class**: feature (새 runner mode와 사전등록·보고서 JSON 계약. 최종 class와 gate 적용은 `auto spec gates`가 쓰는 `gate-applicability.json`이 정한다)
**Source PRD**: `prd.md`(같은 디렉터리, rev 7 결정 반영)
**Target module**: `autopus-adk/` (이 repository)
**Module ownership**: `scripts/benchmarks/harness/**`, `docs/benchmarks/harness-feature-*`, repo root `README.md`의 benchmark claim 한 줄(조건부), `.autopus/specs/SPEC-HARNESS-BENCH-002/**`(review-*.json 제외)
**Predecessors**: SPEC-HARNESS-BENCH-001(pilot mode), SPEC-HARNEVAL-001(격리 grader)

## 목적

2026-09-20 pilot(`docs/benchmarks/harness-2026-09-20.md`)은 작은 회귀 수리에서 harness의 품질 이득을 보이지 못했다. 이 SPEC은 harness가 겨냥하는 중간 규모 기능 작업에서 두 경로를 비교하는 runner mode를 만든다.

- harness 경로: Claude Code에서 `/auto plan --auto` → `auto spec review`(claude, codex. D1로 gemini/agy 제외) → `/auto go --auto`
- native 경로: Autopus 없는 Claude Code 단일 세션

목적은 harness의 실제 효과 확인이다. 알려진 편향은 공개로 끝내지 않고 설계로 제거하고, 제거할 수 없는 것만 Bias Ledger에 `잔여`로 적는다.

규모는 6 task × 2 arm × 1 run = 12 run이다. 사전등록은 첫 trial 전에 commit·push로 동결하고, 보고서는 결과와 무관하게 공개한다. README 문구는 R1 판정이 `claim_allowed: true`일 때만 들어간다.

## Outcome Boundary

- Outcome Lock 원문은 `research.md` `## Outcome Lock`이다.
- Mandatory requirements: REQ-HB2-01 ~ REQ-HB2-25 (모두 Must).
- Explicit non-goals:
  - 통계적 유의성 주장, 반복 측정(K>1), 6개를 넘는 corpus
  - Claude Code 외 플랫폼 arm, 경쟁 harness arm, 구성 요소별 ablation
  - harness 제품 코드(`pkg/**`, `internal/**`, `content/**`, `templates/**`) 변경
  - 외부 repository 코드를 이 repository에 commit하는 것
  - USD 비용 추정, `/auto sync` 품질 평가, 사람 리뷰 부담 측정, CI에서 benchmark 실행
  - Python task 채점 adapter(Go 적격 6개 미만이면 동결 전 개정, REQ-HB2-03)
  - 새 secret, 새 계정, 승인 절차 도입(open-source 1인 운영). 필요해지면 Completion Debt blocker다
  - gemini/agy review(D1: macOS에서 agy 인증이 Keychain 전용이라 격리 home으로 돌릴 수 없다)
- Blocker 조건: `research.md`의 열린 CD-7(D1·D2와 PR #267 포함 pinned `auto`로 RFP-2 재실행)이 닫히지 않으면 benchmark는 차단된다. 사전등록과 실행을 하지 않고, lower bound나 통제 완화로 진행하지 않는다(REQ-HB2-19).
- Completion evidence:
  1. `plan.md` `## Verification`의 Python·Go test가 model 호출 없이 통과한다(acceptance S1–S16, S18, S19).
  2. T0 기록(RFP-1 조건부 PASS, RFP-2 FAIL과 D1·D2 결정), CD-7 재실행 PASS, 사전등록용 usage probe receipt(claude·codex parse 성공), RFP-3 PASS가 사전등록에 들어 있다.
  3. 6개 외부 task의 calibration 기록(reference 구성 2회 pass, parent 구성 fail).
  4. 사전등록 commit이 첫 trial 전에 origin/main에서 도달 가능하다.
  5. 12 run과 blind 채점이 재시도 없이 기록된다(OPS).
  6. `docs/benchmarks/harness-feature-<date>.{md,json}`이 결과와 무관하게 commit된다.
  7. README diff는 `claim_allowed`가 false면 0줄, true면 생성된 claim line 한 줄이다(S17).

## Trust Model

- **agent**(두 arm, sandbox 안 모든 process)는 permission 우회 모드라 신뢰하지 않는다. default-deny 읽기, pid 귀속 egress, 거부 tool로 가둔다(REQ-HB2-08, 21, 22).
- **receipt**는 sandbox 밖 runner만 만들어 agent가 읽지도 쓰지도 못하는 `<session>/receipts/`에 둔다. 빠지거나 깨지거나 순서가 어긋나면 claim을 막는다(REQ-HB2-25). agent가 쓸 수 있는 `auto` shim 메시지는 R1 밖 부차 지표에만 쓴다(RB-3).
- **review codex(D2)**: codex 자체 Seatbelt는 바깥 Seatbelt 안에서 다시 걸 수 없다(`forbidden-sandbox-reinit`). 그래서 broker는 review codex를 `feature_provider.sb` 없이 돌리고, 그 shell 도구의 경계는 codex 자체 `--sandbox read-only`다. 바깥 통제가 여전히 보장하는 것은 env allowlist, broker home, cwd 검사, process group kill, pid 귀속 egress, agent 밖 receipt, trial 동안 다른 저장소의 DAC 잠금, codex 명령 event 관측이다(REQ-HB2-25, RB-8).
- **채점**: grader는 opaque id와 후보 change set만 받고, `/` 기준 읽기 거부 profile `[NEW] feature_grader.sb`에서 돈다. arm mapping은 모든 grade 기록 뒤에 읽는다. runner는 agent가 쓴 workspace에서 `git`을 실행하지 않는다.
- **외부 입력**(PR 메타데이터, 외부 코드, provider 출력)은 untrusted다. prompt는 파일로 전달한다. 외부 코드는 sandbox 밖에서 build·실행하지 않는다. sandbox 밖 단계는 trusted 저장소의 `git`, `go mod download`, `go mod verify`, Go 소스 parse뿐이다.
- **credential**: master는 trusted 저장소에만 있고 runner가 sandbox 밖에서 master에 CLI를 실행할 때만 갱신된다. trial 사본은 master로 되쓰지 않고 redaction을 위해서만 읽은 뒤 지운다(REQ-HB2-17).

## Requirements

### 사전등록

**REQ-HB2-01** 사전등록 문서
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL keep exactly one committed pre-registration document `[NEW] scripts/benchmarks/harness/feature_prereg.json` holding hypotheses H1–H4, metric definitions, the R1 rule and thresholds, the selection rule (repository tiers with expected licenses, window start with its per-model sources, window end, filters, prompt template and strip-rule version), the corpus manifest without any third-party text or code, `corpus_sha256`, the excluded candidates with reason codes, the arm configuration, the frozen config overlay, the frozen config home files, the agent environment allowlist, the denied tool set, the usage schema, the egress allowlist, one usage probe receipt digest per provider, the seeded arm order, the remaining-bias list, and `runner_sha256`.

- corpus manifest의 task별 항목: repository URL, license SPDX id, PR 번호, `created_at`, `merged_at`, 최초 commit 시각, merge SHA, parent SHA, 두 tree id, oracle 경로별 상태(`A`/`M`/`D`)와 SHA-256, `expected_tests`(`[package import path, test name]` 쌍), grade packages, 제거한 harness surface 경로, prompt SHA-256, 메타데이터 snapshot SHA-256.
- `corpus_sha256`은 불변 field만 해시한다. `[repo, pr_number, parent_sha, merge_sha]` 배열의 목록을 `(repo, pr_number)` 오름차순으로 정렬하고(`repo`는 소문자 `owner/name`), `json.dumps(rows, separators=(',', ':'), ensure_ascii=True)`의 UTF-8 바이트(끝 줄바꿈 없음)의 SHA-256 hex다. GitHub field를 다시 받아도 값은 바뀌지 않는다.
- arm 설정: Claude Code·codex·go 버전 문자열과 CLI 설치 snapshot digest, 동결 model 집합, pinned `auto`의 `auto_source_commit`·build 명령·binary SHA-256·version 문자열, model `claude-opus-5-5`, effort, deadline 10800초, permission mode, 거부 tool 목록, MCP config, `auto init` argv.
- usage probe는 provider별(`claude`, `codex`) `{receipt_sha256, fields}`다.
- 가설: H1 harness `delivered_pass` 수 > native(R1이 판정), H2 harness token > native(`token_ratio` 보고), H3 harness wall time > native(`time_ratio` 보고), H4 harness review finding을 severity·status별로 센 기술 통계(finding과 hidden test를 잇는 기계 규칙이 없어 대응 비율은 재지 않는다, REQ-HB2-12).
- `runner_sha256`은 `[NEW] FEATURE_RUNNER_FILES`의 tree digest다. 형식은 `golden_protocol.runner_digest`와 같다. `feature_prereg.json` 자신은 목록에서 뺀다.

**REQ-HB2-02** 사전등록 강제
Priority: Must
Type: Unwanted
IF the output directory already holds `protocol.json`, the pre-registration document is missing, its bytes differ from the blob at the last commit that touched it, that commit is not an ancestor of the commit `git ls-remote origin refs/heads/main` reports, any frozen digest or the frozen arm order differs from the runner's recomputation, or any pinned version string differs from the installed one, THEN THE SYSTEM SHALL refuse the session before any model call by printing one JSON line `{"status":"refused","reason":<code>,"detail":<text>}` to stdout and exiting 1.

- 검사 순서: `unsupported_os`, `protocol_exists`, `prereg_missing`, `prereg_dirty`, `prereg_unpublished`, `usage_probe_missing`(REQ-HB2-19), `prereg_digest_mismatch`(detail은 field 이름: `runner_sha256`, `corpus_sha256`, `auto_sha256`, `order` 등), `cli_version_mismatch`, `oracle_calibration_failed`(REQ-HB2-04), `sandbox_policy_leak`(detail은 경로), `host_solution_copy`(REQ-HB2-25).
- 원격 SHA object가 local에 없으면 `prereg_unpublished`다. 어느 거부든 `protocol.json`을 쓰기 전에 끝난다.
- 한 사전등록 commit으로는 session을 한 번만 시작한다. 멈춘 session은 재개하거나 다시 시작하지 않는다. 새 시도는 앞 session 보고서를 먼저 공개하고, 그 보고서의 JSON SHA-256을 `prior_sessions`에 담은 새 사전등록 commit으로만 한다(RB-6).

### Corpus

**REQ-HB2-03** 외부 OSS corpus 선택 규칙
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL select the 6 tasks mechanically as the first 6 eligible pull requests in ascending `merged_at` order, ties broken by repository name and then PR number, across the frozen Tier 1 Go repository list, where a pull request is eligible only when its `created_at`, the author and committer dates of every PR commit, and the `created_at` of every same-repository issue the PR body references are all after the frozen window start, it was merged at or before the frozen window end into the default branch by a non-bot author, its repository license is in the allowlist, it changes 3 to 12 non-generated production `.go` files and adds or modifies at least one `_test.go`, it leaves `go.mod` and `go.sum` unchanged and touches no harness surface path, the file list of `git diff --no-renames <merge>^1 <merge>` equals the PR file list, its oracle closure references no new or changed unexported identifier, its stripped description is non-empty, its `expected_tests` is non-empty, its module cache holds no module of its own repository, and its calibration passes.

- **window 시작**: 동결 model 집합의 vendor 공개 training data cutoff 중 가장 늦은 값이다. 동결 model 집합은 두 arm에서 쓰일 수 있는 모든 model이다: top-level `claude-opus-5-5`, ultra preset agent alias(`fable`, `opus`; `pkg/config/defaults.go:136-144`)와 Claude Code 내장 subagent가 실제로 쓰는 model id(CD-7이 `result.modelUsage` key로 관측), review panel `claude-opus-5-5`·`gpt-6-astra`. 실행 중 이 집합 밖 model id가 보이면 `model_substituted`다(REQ-HB2-10). 공개 cutoff가 없는 model은 공개 출시일을 쓴다. T11이 값, 출처 URL, 확인 시각을 동결한다. 이 규칙 하나만 쓴다(rev 2의 2026-06-30은 하한일 뿐이다).
- Tier 1(Go, ADK `go.mod` 직접 의존성 제외): `urfave/cli`, `go-chi/chi`, `labstack/echo`, `gin-gonic/gin`, `BurntSushi/toml`, `pelletier/go-toml`, `samber/lo`, `spf13/viper`, `go-playground/validator`, `uber-go/zap`, `cli/cli`, `junegunn/fzf`, `jesseduffield/lazygit`, `hashicorp/go-retryablehttp`, `caddyserver/caddy`, `prometheus/client_golang`.
- Tier 2(Python: `pallets/click`, `pallets/flask`, `encode/httpx`, `fastapi/typer`, `pydantic/pydantic`, `python-attrs/attrs`, `Textualize/rich`, `psf/requests`)는 Tier 1 적격이 6개 미만일 때만 쓰며, 그때는 동결 전에 Python adapter를 위해 이 SPEC을 개정한다(유일한 범위 변경 trigger).
- license allowlist: `MIT`, `Apache-2.0`, `BSD-2-Clause`, `BSD-3-Clause`, `ISC`, `MPL-2.0`(GitHub API `license.spdx_id`).
- issue 참조는 REQ-HB2-16의 형식 전부와 닫는 keyword 없는 `#<n>`이다. 하나라도 window 시작 이전이면 `before_window:issue`다.
- generated 파일은 package 절 앞 주석에 `^// Code generated .* DO NOT EDIT\.$` 줄이 있는 `.go`다. production 파일 수는 `_test.go`, `testdata/`, `vendor/`, generated를 뺀 `.go` 수다.
- 제외 reason code. 이 순서로 검사하고 처음 실패한 것 하나를 쓴다: `before_window:<created_at|commit|issue>`, `outside_window`, `bot_author`, `not_default_branch`, `license_not_allowed:<spdx>`, `prod_files_out_of_range:<n>`, `no_test_change`, `go_mod_changed`, `harness_surface_changed`, `ambiguous_parent`, `unexported_api:<ident>`, `no_description`, `no_expected_tests`, `go_toolchain_unsupported`, `modcache_self:<module>`, `calibration_failed:<merge|parent|flaky>`. 적격이지만 앞 6개 밖이면 `beyond_first_6`.
- unexported 검사는 `[NEW] feature_goapi/main.go`(stdlib `go/parser`·`go/ast`, 실행 없음)가 oracle closure에 대해 한다.
  - closure 시작점: 상태 `A`·`M`인 oracle `_test.go`의 모든 package-level 선언, 같은 디렉터리 모든 `_test.go`의 `TestMain`, `init`, package-level `var` 초기화식. 시작점이 이름으로 참조하는 같은 디렉터리 `_test.go`의 선언(함수, type과 그 method 전부, var, const)을 고정점까지 더한다.
  - closure의 식별자와 `.name` selector 중, 대상 package non-test 파일의 unexported 선언 이름이 parent에 없거나 선언이 바뀌었으면 `unexported_api:<ident>`다(이름 비교라 보수적). `export_test.go`를 추가·수정하면 `unexported_api:export_test.go`다.
  - REQ-HB2-16 API 절은 같은 closure에서 새로 생기거나 선언이 바뀐 exported 식별자로 만든다.

**REQ-HB2-24** oracle 구성
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL classify every path of `git diff --no-renames --name-status <parent> <merge>` as oracle when it ends in `_test.go` or has a path component `testdata` and as production otherwise, build every graded tree by the single composition parent tree plus solution plus oracle overlay, where the solution is a candidate change set (REQ-HB2-09) or the reference change of the merge's production paths and the overlay writes the merge blob of every added or modified oracle path and deletes every deleted oracle path, derive `expected_tests` as the `(package import path, test name)` pairs of top-level `func TestXxx(*testing.T)` declarations in oracle files whose own declaration or REQ-HB2-03 closure holds a declaration that is new or changed between parent and merge, and derive the grade packages as the directories holding an added or modified oracle `_test.go`.

- rename은 삭제와 추가가 되어 옛 test 파일을 overlay가 지운다. reference 구성 tree id가 merge tree id와 다르면 `ambiguous_parent`다.
- import path는 `go.mod` `module`과 디렉터리로 만든다(`p_test`도 같다, test2json `Package` 값).
- grade packages는 module root 기준 상대 디렉터리 목록으로 저장하고 root는 `.`로 쓴다(사전등록 manifest 형식). go 인자로는 `./<dir>`로 바꾸고 root는 `./`가 된다(`prepare_grader.py:244`와 같은 `./` 형식). 동결 채점 명령은 `go test -count=1 <변환한 인자>`이고 `-json`은 `grader.grader_argv`가 붙인다(`grader.py:120-128`). table case만 바꾼 PR도 closure로 잡히고, 그래도 비면 `no_expected_tests`다.

**REQ-HB2-04** calibration
Priority: Must
Type: Unwanted
IF a selected task's oracle is not accepted on its reference composition in two consecutive runs, or is accepted on its parent composition (parent tree plus empty solution plus oracle overlay), as judged by the `(package, test)` judge of REQ-HB2-09 under `feature_grader.sb` offline with the task's download-only module cache and an empty build cache, THEN THE SYSTEM SHALL exclude it during selection with `calibration_failed:<merge|parent|flaky>`, and the runner SHALL refuse a session whose frozen tasks fail the same check with reason `oracle_calibration_failed` and the task ids before any model call.

- module cache는 download 전용이다. `[NEW] feature_corpus.prefetch`가 sandbox 밖에서 parent tree `go.mod`·`go.sum`으로 `go mod download -json`(`GOPROXY=https://proxy.golang.org`, `GOTOOLCHAIN=local`)과 `go mod verify`만 하고, go.sum에 없는 hash가 있으면 실패한 뒤 cache를 읽기 전용으로 만든다. `prepare_grader.prepare`의 warm build(`go test -run '^$'`, `prepare_grader.py:133`)는 쓰지 않는다.
- calibration은 후보 채점과 같은 구성과 같은 `feature_grade.judge_pairs`로 한다. 실행은 `grader.grader_argv`·`run_sandboxed`만 쓰고, `grader_argv`가 읽는 `prepared['warm_cache']`에는 빈 디렉터리를 준다. `grader.grade`와 `grader.judge`는 bare 이름 판정이라 부르지 않는다(`grader.py:254`, `:278-285`). compile은 모두 grade root 안 `[NEW] feature_grader.sb` 아래 cold로 한다. 이 profile은 `grader.sb`(`(allow default)`, `:22`)를 고치지 않고, `/` 기준 읽기 거부에 grade root, module cache, GOROOT, REQ-HB2-08 시스템 목록만 열며 network와 mach-lookup은 `grader.sb`처럼 막는다. `grader_argv`의 `profile` 인자로 넘긴다(`grader.py:115`). 채점 timeout은 900초다. 검사한 모든 후보에 같다.
- `feature_grader.sb`는 loopback도 거부하므로 기존 test가 listener를 쓰는 grade package는 calibration에서 빠진다(RB-4).

### Arm 구동

**REQ-HB2-05** harness arm
Priority: Must
Type: Event-driven
WHEN a harness-arm trial runs, THEN THE SYSTEM SHALL run the pinned `auto` through the `auto` shim with the frozen `auto init` argv on the trial workspace inside the agent sandbox with the REQ-HB2-22 environment, apply the frozen config overlay to the generated `autopus.yaml`, put the harness-only `auto` shim directory first on PATH, run `claude -p "/auto plan <F> --auto"` where `<F>` is the frozen task instruction of REQ-HB2-16, then run `claude -p "/auto go <SPEC-ID> --auto"` with the remaining deadline only when exactly one new SPEC directory appeared under `.autopus/specs/`, and record the harness path the product chose with zero human input.

- 동결 `auto init` argv: `auto init --yes --dir <ws> --platforms claude-code --quality ultra --no-rea --statusline-mode keep`. 환경은 `env -i`와 REQ-HB2-22 allowlist(`LANG=en_US.UTF-8`)라 host locale과 무관하다.
- **`auto` shim**: `<session>/auto-shim/auto`는 `<session>/pinned/auto`를 감싸고 호출마다 argv와 시작·종료를 broker에 알린다. broker가 받은 시각으로 `<session>/receipts/<trial>/auto.jsonl`을 쓴다. 두 디렉터리는 trial 디렉터리 밖이고 agent에게 읽기 전용이며 harness trial에서만 열린다(REQ-HB2-08). harness PATH의 `auto`는 이 shim 하나뿐이고 native에는 `auto`가 없다. `review_invocations`와 `phase_seconds.review`는 이 기록으로 센다(R1 밖 부차 지표, RB-3).
- 동결 overlay는 `auto_source_commit` C의 `autopus.yaml` `orchestra.providers.claude`·`codex`를 그대로 복사한다. C는 PR #267(head `0dfcce9c`, `autopus.yaml:79-90`)을 포함해야 한다. 근거는 origin/main `4f21f309`와 #267 diff다. digest는 사전등록에 있다.
  - 기대 값(#267 기준): `claude` args `[--print, --model, claude-opus-5-5, --effort, max, --output-format, json]`, `codex` args `[exec, --json, --sandbox, workspace-write, -m, gpt-6-astra, -c, 'model_reasoning_effort="max"']`, `model_policy: quality`, `schema_flag: --output-schema`, timeout 900, `orchestra.timeout_seconds: 240`, `judge: claude`. C에서 값이 다르면 C의 값을 쓰고 사전등록에 적는다.
  - D1(선언된 제품 기본값 이탈): `gemini`를 `orchestra.providers`, 모든 `orchestra.commands.*.providers`, `spec.review_gate.providers`에서 뺀다. `spec.review_gate: {enabled: true, strategy: debate, providers: [claude, codex], judge: claude, min_providers: 0}`. quorum은 제품 규칙 `DefaultMinProviders(2) = 2/2+1 = 2`(`pkg/spec/provider_health.go:236-241`, `:247-266`)라 두 reviewer가 모두 성공해야 한다(기본 3명일 때는 2/3). 미달이면 제품이 review를 degraded로 표시한다(`spec_review_integrity.go:38-39`). `init.go:150-153`의 gate 끄기와 무관하다.
  - MCP(선언된 이탈): project MCP(`templates/claude/mcp.json.tmpl`의 context7, sequential-thinking)는 빈 MCP config와 `--strict-mcp-config`로 두 arm 모두 꺼진다. 둘 다 `npx` 내려받기가 필요해 egress 아래 뜰 수 없다(RB-10).
  - `language: {comments: en, commits: en, ai_responses: en}`. 한국어 출력은 token을 더 써 token 비교를 기울이므로 모든 surface를 영어로 둔다. 고정 env에서 `init.go:139-142`도 `en`을 골라 `CLAUDE.md` Language Policy(`pkg/adapter/claude/claude_markers.go:148-157`)가 overlay와 같다. native에는 언어 지시가 없다(RB-5).
- pinned `auto`는 사전등록 PR 전에 고른 origin/main commit `C`(`auto_source_commit`, 사전등록 commit P의 조상)의 `git archive`에서 `CGO_ENABLED=0 GOENV=off GOTOOLCHAIN=local go build -trimpath -buildvcs=false -ldflags "-s -w -X …/pkg/version.version=bench-<c7> -X …/pkg/version.commit=<c7> -X …/pkg/version.date=<C commit date>" -o <session>/pinned/auto ./cmd/auto`로 만든다(`<c7>`은 C의 앞 7자, `.goreleaser.yaml:8-27` flag). 입력이 모두 P보다 먼저 정해져 순환이 없다. preflight는 C가 P의 조상인지 보고 다시 build해 `auto_sha256`과 `auto version` 출력(`pkg/version/version.go:82-84`)을 비교한다.
- `auto init` 중 provider 호출도 broker로 재고 phase `setup`으로 harness 합계에 넣는다.
- 기록하는 harness 경로: `spec_id`, `change_class`, `profile`, `review_invocations`, `review_rounds`, `review_verdict`, `go_entered`. `review_rounds`는 최종 `**Revision**` + 1이다(Revision은 0부터, `pkg/spec/types.go:193`). 새 SPEC이 0개면 `plan_no_spec`, 2개 이상이면 `plan_ambiguous_spec`이고 go는 실행하지 않는다.

**REQ-HB2-06** native arm
Priority: Must
Type: Event-driven
WHEN a native-arm trial runs, THEN THE SYSTEM SHALL run one `claude -p "<F>"` session with no `auto` on PATH and no read access to the `auto` shim and pinned `auto` directories, and SHALL refuse that trial with `native_surface_present` and the offending path whenever the pre-trial audit finds any of `workspace.HARNESS_DIRS` or `workspace.HARNESS_FILES` at any depth of the workspace or in any ancestor directory up to `/`.

- 외부 repo가 원래 담은 `AGENTS.md`, `CLAUDE.md`, `.claude/` 등은 `feature_corpus`가 모든 깊이에서 두 arm에 똑같이 지우고 manifest에 남긴다. 그래서 거부는 runner 결함일 때만 생긴다(REQ-HB2-23). native도 내장 subagent를 쓸 수 있다(Q5).

**REQ-HB2-07** 동등성, seeded 순서, 같은 machine
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL build both arms' top-level `claude` argv and environment from one arm configuration so that, after every occurrence of the trial root path is replaced by `<TRIAL>`, they differ only in the prompt argument, in the leading `auto` shim directory entry on PATH, and in the sandbox parameter that opens the `auto` directories, SHALL run the 12 runs serially on one machine with the two runs of each task adjacent and their order taken from the frozen seeded order, SHALL record the machine fingerprint and every pinned version string before each run, and SHALL stop the session with `cli_version_changed` or `machine_changed` before the next run when either differs.

- 순서: task key는 `sha256(corpus_sha256 + ":" + task_id)` hex다. key 오름차순 앞 3개 task가 harness 먼저, 나머지가 native 먼저다. task는 사전등록 순서(`merged_at` 오름차순)로 돈다.
- machine fingerprint는 `{kern.osproductversion, kern.osversion, hw.model, hw.ncpu, hw.memsize}`의 SHA-256이다. hostname과 serial은 넣지 않는다.
- 설치 디렉터리는 session 시작 때 복사한다(CLI snapshot, digest 기록). top-level claude 사본은 `<session>/cli-agent/`, broker가 쓰는 `claude`·`codex` 사본은 agent가 읽을 수 없는 `<session>/cli-broker/`에 둔다. 매 run 전 version 검사는 host 설치본이 아니라 이 snapshot 파일과 pinned `auto`의 digest를 다시 계산해 비교한다. host 자동 갱신은 session을 멈추지 않는다.
- 결과에 따른 재시도는 없다. harness 내부 선택(subagent, review model)은 처치다(REQ-HB2-20).

**REQ-HB2-17** 격리된 config home과 credential 수명
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL give every trial of both arms a fresh config home `<trial>/home` with `HOME` set to it and `CLAUDE_CONFIG_DIR` to `<trial>/home/.claude`, and a broker config home `<session>/broker-home/<trial>` that the agent cannot read for brokered `claude` and `codex` calls, seed both only with the frozen config home files and copies of the master provider authentication files, pass an empty MCP config with `--strict-mcp-config`, refuse the trial with `config_home_contaminated` and the path when the pre-trial audit finds any other file or a frozen file with different bytes, and after the trial's process tree has ended redact credentials from every retained artifact and only then delete the config home.

- 동결 config home 파일(두 arm 동일): `.claude/settings.json`(거부 tool deny), `.gitconfig`(`bench <bench@invalid>`, gpgsign 끔), Go telemetry mode 파일.
- 인증(T0 확인): claude 로그인은 Keychain 항목 `Claude Code-credentials`에만 있다. trusted runner가 sandbox 밖에서 그 항목을 읽어 `claudeAiOauth`만 남긴 `.claude/.credentials.json`을 만든다(같은 항목의 다른 `mcpOAuth` token은 버린다). agent home에는 이것만, broker home에는 claude 사본과 codex `auth.json`을 둔다. sandbox는 Keychain을 닫는다. `claude setup-token`, API key 같은 새 secret은 쓰지 않는다.
- refresh: trial 전 trusted runner가 master 만료가 남은 deadline + 30분보다 늦은지 보고, 아니면 sandbox 밖에서 master home을 대상으로 CLI를 실행해 갱신한다(외부 코드 없음, `maintenance_calls`에 기록). 갱신 뒤에도 조건이 맞지 않으면 그 trial을 model 호출 없이 `trial_refused:auth_expiry`로 기록하고 session을 멈춘다. trial 사본은 master에 되쓰지 않는다. trial 안 회전으로 master가 무효가 되면 다음 trial은 `provider_auth_failed`로 claim을 막는다. 갱신 뒤 수명과 강제 회전 뒤 master 재사용 가능 여부는 CD-7이 잰다.
- 보존 artifact는 change set, receipt, transcript, record뿐이다. redact 대상은 seed한 인증 파일과 trial 뒤 인증 파일(trial 중 회전한 token 포함)의 16자 이상 문자열 값 전부, 그리고 credential 모양 pattern(`sk-[A-Za-z0-9_-]{20,}`, `eyJ[\w-]{10,}\.[\w-]{10,}\.[\w-]{10,}`, `ya29\.[\w-]{20,}`, `1//[\w-]{20,}`)이다. 모두 `[REDACTED]`로 바꾸고 수를 `credential_redactions`로 남긴다.

**REQ-HB2-21** 거부 tool과 transcript 검사
Priority: Must
Type: Unwanted
IF any runner-side transcript or broker receipt of a trial of either arm, including Claude Code subagent messages and every codex model call (the harness review calls included), contains a web search or URL fetch tool call, THEN THE SYSTEM SHALL record the trial with the control violation `tool_policy:<tool>` and `delivered_pass: false`, where Claude Code `WebFetch` and `WebSearch` are additionally denied in both arms by `--disallowedTools WebFetch WebSearch` on the top-level session and the frozen `settings.json` deny rule for every claude process.

- codex는 설정이 아니라 broker가 본 event로 통제한다(T0: 투영 argv에서 `type:"web_search"` item이 started·completed로 나온다. 검색은 서버 쪽이라 proxy에는 보이지 않는다). review codex 투영은 `--ignore-user-config`를 더하고(`internal/cli/orchestra_readonly_policy.go:253-258`) `-c`는 `model_reasoning_effort=`만 받으며(`:224-226`), 투영 변경은 처치 변경이라 하지 않는다. 검사 대상은 top-level `tool_use.name`(subagent 포함), review claude 출력(`--tools=Read,Grep,Glob` 투영 `:236-251`), codex `web_search`·URL item이고 호출 성공 여부와 무관하다. agy는 D1로 쓰지 않는다.

### 격리

**REQ-HB2-08** sandbox, 환경, egress
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL run every agent process tree under `[NEW] feature_agent.sb`, which denies every file read rooted at `/` and reopens only the frozen read list below, identical in both arms except one parameter that opens the `auto` directories in harness trials, allows writes only under the trial directory and `/dev/null`, closes the `com.apple.SecurityServer` Mach service, allows `process-info*` only for itself and `signal` only within the same sandbox (`(allow signal (target same-sandbox))`, as `/System/Library/Sandbox/Profiles/application.sb` uses, so the product's process-group SIGKILL of a shim client works), and allows outbound network only to the runner's loopback CONNECT proxy `[NEW] feature_egress.py` and the provider broker port of REQ-HB2-25, SHALL start that tree with exactly the frozen environment allowlist of REQ-HB2-22, SHALL refuse the session with `sandbox_policy_leak` before any model call when a preflight probe under the agent profile can read a canary placed in the session, and SHALL leave no process of the trial session after the trial ends.

- 동결 읽기 목록(`file-read*`): 자기 trial 디렉터리, task module cache(읽기 전용), GOROOT, `<session>/cli-agent/`, `<session>/shims/`, harness만 `<session>/auto-shim/`·`<session>/pinned/`, 그리고 시스템 경로 `/System`, `/usr/lib`, `/usr/libexec`, `/usr/share`, `/bin`, `/sbin`, `/usr/bin`, `/usr/sbin`, `/private/etc`, `/private/var/db/dyld`, `/private/var/db/timezone`, `/Library/Developer/CommandLineTools`, `/private/var/select`, `/dev`, `/` literal(`file-read-data`). `file-read-metadata`는 열린 경로들의 조상 literal과 root symlink `/etc`, `/var`, `/tmp`, `/private/var/db/xcode_select_link`, `/Applications/Xcode.app/Contents/Developer`에만 연다(T0 최소 추가). `/dev/dtracehelper` 쓰기 거부는 무해한 noise라 `sandbox_denied_other`다. 목록 밖의 OS·framework·CLI 읽기가 필요하면 RFP-2가 찾아 이 목록에 넣고 사전등록에 동결한다. 암묵적 허용은 없다.
- 그래서 `--output`이 어디에 있든 자기 trial 밖 session(receipts, broker-home, cli-broker, 다른 trial, native의 `auto` 디렉터리)과 oracle·selection·repo 저장소는 읽을 수 없다.
- preflight canary: 첫 trial 전 runner가 `<session>/receipts/`, `<session>/broker-home/`, `<session>/cli-broker/`, 이웃 trial 디렉터리, oracle·selection·repo 저장소, `--output`의 부모에 canary를 두고 두 arm의 parameter로 읽기를 시도한다. `<session>/pinned/`·`auto-shim/` canary는 native parameter로만 시도한다(harness에는 설계상 열려 있다). 하나라도 읽히면 `sandbox_policy_leak`으로 거부한다(REQ-HB2-02).
- 위반 검출: runner가 sandbox 밖에서 `log stream --style ndjson --predicate 'sender == "Sandbox"'`로 trial process tree pid의 거부를 모은다. 해답이 들어 있을 수 있는 보호 목록(자기 trial 밖 session, oracle·selection·repo 저장소, `--repo` checkout) 아래 거부는 통제 위반 `isolation:<path>`다. credential 경로(계정 home의 `.ssh`, `.config`, `.aws`, `.netrc`, `.git-credentials`, `.docker`, `.claude`, `.codex`, Keychain, `security` 실행) 거부는 내용이 새지 않고 결과를 기울이지 않으므로 `sandbox_denied_credential` 수만 남긴다. 그 밖의 거부는 `sandbox_denied_other` 수다. CD-7은 두 arm 정상 trial이 `control_violations: []`로 끝나는 기준선을 확인한다.
- 매 trial agent 시작 전 같은 profile로 보호 canary 읽기를 1회 해서 거부 로그가 잡히는지 본다. 안 잡히면 `denial_log_unavailable`로 session을 멈춘다. 거부된 egress host는 `egress_denied` 수로 남긴다.

**REQ-HB2-22** agent runtime 환경
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL start the agent of both arms with only these environment keys: `PATH` (in this order: the harness-only `auto` shim directory, the provider shim directory, Go toolchain `bin`, `/usr/bin:/bin:/usr/sbin:/sbin`), `HOME`, `CLAUDE_CONFIG_DIR`, `USER`, `LOGNAME`, `SHELL=/bin/zsh`, `TERM=dumb`, `LANG=en_US.UTF-8`, `LC_ALL=en_US.UTF-8`, `TMPDIR` and `GOTMPDIR` set to `<trial>/tmp`, `GOCACHE` set to an empty `<trial>/gocache`, `GOPATH` set to `<trial>/gopath`, `GOMODCACHE` set to the task's read-only download-only module cache, `GOPROXY=off`, `GOSUMDB=off`, `GOFLAGS=-mod=mod`, `GOTOOLCHAIN=local`, `GOENV=off`, `HTTPS_PROXY` and `HTTP_PROXY` and their lowercase forms set to the egress proxy, `NO_PROXY` empty, `DISABLE_AUTOUPDATER=1`, `CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1`, `CLAUDE_CODE_TMPDIR` set to `<trial>/tmp`, and `SSL_CERT_FILE=/etc/ssl/cert.pem`.

- 상속 key는 없다(`env -i`). `GH_TOKEN`, `AWS_*`, `GIT_*` 등은 들어가지 않고 git identity는 `.gitconfig`가 준다. merge tree로 compile한 build cache는 어느 arm에도 없다. agent test의 loopback listener는 두 arm 모두 sandbox가 막는다. 마지막 두 key는 T0가 찾은 최소 추가다(CD-6, CD-5).

**REQ-HB2-25** provider broker와 trusted receipt
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL capture the top-level `claude` stream-json from the runner's own stdout pipe, SHALL run every other `claude` and `codex` invocation only through `[NEW] feature_broker.py` outside the agent sandbox, which receives argv, stdin, and cwd from the in-sandbox shim client over a loopback port, accepts only the frozen model-call and auxiliary argv patterns and a cwd inside the trial directory, starts the CLI from `<session>/cli-broker/` in that cwd with the broker config home and environment, under `[NEW] feature_provider.sb` for `claude` and under codex's own `--sandbox read-only` for `codex` (D2), relays stdout, stderr, and exit code, kills the CLI's process group when the client goes away or the trial ends, and records argv, sequence number, start and end times, raw events, usage, model arguments, and tool events, SHALL let `feature_egress.py` open a tunnel to a provider host only when `lsof` attributes the connecting socket to the top-level `claude` pid or to a broker-started CLI pid or its descendant, and SHALL write all of these receipts only to `<session>/receipts/<trial>/`, which the agent sandbox can neither read nor write.

- `feature_provider.sb`(claude 전용)도 `/` 기준 읽기 거부에 REQ-HB2-08 시스템 목록, 요청 cwd의 trial 디렉터리, `cli-broker`, broker home, `<session>/broker-tmp/<trial>`, toolchain만 연다. 쓰기는 trial 디렉터리, broker home, broker tmp, `/dev/null`, network는 egress proxy뿐이다.
- D2 review codex: codex는 이 profile 없이 돈다. 대신 trial 동안 runner가 자기 trial 밖 session, oracle·selection·repo 저장소, arm map, grades를 mode `000`으로 잠그고(DAC, runner는 잠그기 전에 연 fd로 receipt를 쓴다) trial 뒤 푼다. 두 arm에 같다. broker는 codex `--json`의 `command_execution` item에서 trial 디렉터리와 시스템 목록 밖 경로를 realpath로 가리키는 인자, 또는 그런 곳으로 가는 `cd`를 보면 통제 위반 `review_read_outside_trial`이다. preflight는 host `GOMODCACHE`와 `--repo` 밖 알려진 clone 경로에서 task module의 merge 이후 version을 찾으면 `host_solution_copy`로 거부한다. 나머지는 RB-8이다.
- 환경: `env -i`에 REQ-HB2-22 allowlist를 쓰되 `HOME`·`CLAUDE_CONFIG_DIR`·`CODEX_HOME`은 broker home, `CLAUDE_CODE_TMPDIR`· `TMPDIR`·`GOTMPDIR`는 broker tmp, `PATH`는 `cli-broker` 사본 디렉터리, Go `bin`, 시스템 경로다. runner 환경은 상속하지 않는다.
- cwd: shim이 자기 cwd를 보낸다. broker는 realpath가 trial 디렉터리 안일 때만 그곳에서 CLI를 시작한다. 제품이 auto process의 cwd를 provider에 물려주는 것(`spec_review_loop.go:108-109`)과 같다. 밖이면 CLI를 시작하지 않고 shim에 exit 126을 주며 `{"seq":n,"refused":"cwd_outside_trial","cwd":<realpath>}`과 통제 위반 `broker_cwd_refused`를 남긴다. codex argv는 바꾸지 않는다(`--skip-git-repo-check` 없음, `orchestra_readonly_policy.go:260-262`). trial workspace는 `workspace.initialize`가 만든 git repo다.
- argv: 허용 pattern은 REQ-HB2-10의 model call과 aux 목록(`codex debug models`, `codex login status`, `claude auth status --json`, `claude --version`, `codex --version`)뿐이다. 그 밖의 argv는 CLI를 시작하지 않고 shim에 exit 126을 주며 `{"seq":n,"refused":"argv_not_allowed","argv0":<binary>}`과 통제 위반 `broker_argv_refused`를 남긴다. `agy`도 여기 걸린다.
- 종료: CLI는 새 process group으로 시작한다. shim 연결이 끊기면(orchestra timeout·결과 뒤 reap의 process group SIGKILL, `pkg/orchestra/command_unix.go:18-25`, #267 `provider_runner.go:160-166`) broker는 2초 안에 CLI process group 전체를 SIGKILL하고 `{"seq":n,"terminated_by":"client_gone","exit":-9}` 종료 기록을 쓴다. trial이 끝날 때 남은 CLI도 같이 죽이고 `terminated_by: "trial_end"`로 닫는다. 이렇게 닫힌 call은 `observation_incomplete`가 아니다. usage 완결 여부는 종료 원인이 아니라 receipt로 정한다(REQ-HB2-10).
- fail closed: broker 순번이 1..N으로 이어지지 않거나, 어느 순번에든 종료 기록(`exit`, `client_gone`, `trial_end`, `refused`)이 없거나, receipt가 strict JSON이 아니면 `observation_incomplete`, 귀속되지 않거나 pid를 못 찾은 provider host 연결은 거부하고 `unbrokered_egress:<host>`다. 둘 다 통제 위반이다(`delivered_pass: false`, R1 `control_violation`). 그래서 shim 우회, receipt 삭제·변조, sandbox 안 claude binary 재실행은 관측을 피하지 못한다.

### 채점

**REQ-HB2-09** blind 채점
Priority: Must
Type: Event-driven
WHEN every scheduled trial has ended, THEN THE SYSTEM SHALL grade every stored candidate change set under an opaque random id in an order shuffled by that id, passing the grader only the id, the change set, and the task's oracle, judge each run by `(package import path, test name)` pairs, write all grades to `grades.json` without any arm field, and only then read the arm map and join it to the grades.

- change set: runner는 agent workspace에서 `git`을 실행하지 않는다(`.git/config`의 `core.fsmonitor`나 `.gitattributes` diff driver가 host에서 실행될 수 있고, `workspace.initialize`의 보호는 처음 세 명령뿐이다, `workspace.py:121-128`). trial 전 seeded 파일의 `path → sha256` manifest를 두고, trial 뒤 `.git`을 뺀 workspace를 `lstat`으로 걸어(symlink 비추적) 바뀐·추가·삭제된 경로와 바이트를 모은다. oracle 분류 경로와 harness surface(모든 깊이)는 뺀다. 채점 사본은 REQ-HB2-24 구성이다.
- `[NEW] feature_grade.judge_pairs`는 `grader.parse_events` event를 `(Package, Test)`로 묶고 `/` 없는 top-level event만 센다. 기대 쌍 전부 top-level pass, 기대 쌍의 fail·skip 0, build 실패 0, exit 0일 때만 `accepted`다. 기대 쌍이 아닌 test의 skip은 허용하고 fail은 exit ≠ 0으로 걸린다. 실행은 `grader.grader_argv`·`run_sandboxed`를 쓰고 `grader.judge`(이름만 비교, `grader.py:254-274`)는 쓰지 않는다.
- `delivered_pass`는 기대 쌍 전부 pass, grade package 기존 test 통과, scope·통제 위반 없음, 모든 agent phase가 deadline 안에 exit 0일 때만 참이다. scope 위반은 symlink·비일반 파일, workspace 밖 경로, `go.mod`·`go.sum` 변경이다.
- `hidden_passed`, `hidden_total`(기대 쌍 수), `escaped_defects`를 기록한다. 중단된 session은 `feature.py --grade <session>`으로 같은 방식으로 채점할 수 있다.

**REQ-HB2-16** 기계적 prompt 도출과 파일 전달
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL derive each task prompt only by the frozen template and strip rules from the PR title and body, append the linked issue by the frozen issue rule, append the public API section printed by `feature_goapi` from the oracle closure, write the result as the read-only file `<trial>/task/TASK.md` in both arms, and pass the agent only the frozen flag-free instruction `<F>` = `Implement the change described in ../task/TASK.md in this repository. That file is the complete task description.`, never PR text in argv.

- template(LF 줄바꿈, UTF-8, 끝에 `\n` 하나):

```text
Implement the following change in this repository.

Title: {title}

{description}

The acceptance tests call this public API:
{api_lines}
```

- `{api_lines}`의 각 줄은 `- <package import path>: <선언 서명>`이고 import path, 그다음 식별자 순으로 정렬한다. API 항목이 없으면 마지막 두 줄을 뺀다. 입력 CRLF는 LF로 바꾼다.
- strip 규칙(순서대로, 버전 `strip-v1`, 이름 비교는 대소문자 구분 부분 문자열):
  1. HTML comment를 지운다.
  2. info string이 `diff`·`patch`인 fenced block, 길이 8 이상 PR production 추가 줄과 trim 후 같은 줄을 포함한 block, `func Test`를 포함한 block을 지운다.
  3. oracle 파일 basename, `expected_tests` 이름, `/pull/<n>/files`, `/pull/<n>/commits`, `/commit/<sha>` 링크가 든 줄을 지운다.
  4. task-list 줄과 image markdown을 지운다.
  5. code block 밖에서 길이 8 이상 production 추가 줄과 trim 후 같은 줄을 지운다.
  6. 빈 줄 3개 이상은 2개로, 앞뒤 공백 제거, 8000자 초과는 줄 경계에서 자른다.
- title에는 규칙 3과 5를 적용한다.
- issue: 규칙 1–5를 거친 PR 본문이 200자 미만이고 같은 repository issue 참조가 정확히 하나일 때만 붙인다. 참조는 대소문자 무시 `close|closes|closed|fix|fixes|fixed|resolve|resolves|resolved`, 선택적 `:`, 공백, 그리고 `#<n>`·`<owner>/<repo>#<n>`·`https://github.com/<owner>/<repo>/issues/<n>`이다(owner/repo도 대소문자 무시, 같은 번호는 하나). issue 제목·본문에도 규칙 1–5를 적용하고, description은 `<PR 본문>\n\nIssue: <issue 제목>\n\n<issue 본문>`이며 규칙 6은 붙인 뒤 한 번 적용한다.
- `<F>`에는 `--`가 없어서 router·plan flag 해석(`auto-workflows.md.tmpl:93`, `:739-747`)이 아무것도 꺼내지 않는다. trial 뒤 `TASK.md` SHA-256이 manifest prompt SHA-256과 다르면 통제 위반 `task_file_modified`다. 사람이 쓰거나 고친 prompt는 없다.

### 측정

**REQ-HB2-10** provider별 token, null 금지
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL record tokens for every model call of both arms (`claude` top-level and review, `codex`) as the four fields `uncached_input`, `cache_read_input`, `cache_write_input`, and `output` read through the frozen per-provider usage schema from machine-readable receipts that the runner captures outside the sandbox (REQ-HB2-25), SHALL write a field as `not_applicable` only where the frozen schema declares that the provider has no such field, SHALL never write `null`, zero, or an estimate for an unreadable value, SHALL treat a call as usage-complete only when its receipt holds the provider's terminal usage record whatever ended the process, and SHALL set `usage_status: "unparsed"` with the failing call ids `<provider>#<n>` for a run whose any call is not usage-complete.

- **model call**은 동결 argv pattern에 맞는 호출뿐이다: `claude`는 `-p`·`--print`, `codex`는 첫 non-flag 인자 `exec`. REQ-HB2-25 aux 목록(`codex_catalog_runtime.go:43-58`, `provider_readiness.go:102-105`)은 `aux_calls`에 argv 앞 두 개, exit code, 시간만 남기고 usage와 번호에 넣지 않는다. 둘 다 아니면 broker가 거부한다(`broker_argv_refused`).
- terminal usage record: top-level claude는 stream-json `type:"result"`의 `modelUsage`(model별 합, subagent 포함. `usage`는 top-level만이라 쓰지 않는다, T0 CD-4), review claude는 json `type:"result"`의 `modelUsage`, codex는 `turn.completed.usage`다. 결과 envelope 뒤 제품이 process를 reap해 `client_gone`으로 닫혀도(#267 `provider_completion.go:11-17`) record가 있으면 완결이다.
- reasoning·thinking token은 output의 부분집합으로 기록만 한다(claude thinking `usage_parse.go:84-86`, codex `reasoning_output_tokens` `:103-106`, #267). provider가 output과 관계를 알 수 없는 component를 내면 그 call은 완결이 아니다.
- run 합계: `usage_status: "complete"`일 때만 `tokens_total`(네 field)을 쓴다. `unparsed`면 `tokens_total` 없이 완결된 call만 더한 `tokens_observed`를 쓰고, 이 값은 비율에 쓰지 않는다.
- 출력 모드: top-level `--output-format stream-json --verbose`, review claude는 투영 뒤 형식(`json`·`stream-json`), codex `exec --json`.
- model 대체 금지: model call의 model 인자(codex는 `-m gpt-6-astra`와 `model_reasoning_effort="max"`)가 동결 값과 다르거나, `modelUsage` key가 동결 model 집합(REQ-HB2-03) 밖이면 통제 위반 `model_substituted:<provider>`다. catalog probe 실패 때 `CodexFallbackModel`로 바뀌는 경로(`pkg/config/codex_profile_resolve.go:52-61`)가 걸린다. `model_policy`를 빼 probe를 없애면 처치가 제품 설정과 달라져 관측을 택했고, T0에서 `SSL_CERT_FILE`로 probe가 성공함을 확인했다(CD-6).
- codex `input_tokens`는 cached를 포함한다. uncached는 `input_tokens - cached_input_tokens`이고 `cache_write_input`은 `not_applicable`이다. 
- `<n>`은 run 안 provider별 call 순번(1부터)이다. terminal usage record 없이 끝난 call(deadline kill 포함)은 `unparsed`다.

**REQ-HB2-19** usage 관측 gate
Priority: Must
Type: Unwanted
IF the pre-registration lacks, for each of `claude` and `codex`, a usage probe receipt in which one tiny call run through the broker with the frozen benchmark arguments yielded every applicable field of the frozen usage schema, THEN THE SYSTEM SHALL refuse the session with `usage_probe_missing` and the provider, and IF the top-level Claude Code receipt path cannot account for subagent tokens, THEN THE SYSTEM SHALL record the benchmark as blocked with `subagent_usage_unavailable` and SHALL NOT produce a pre-registration.

**REQ-HB2-11** wall time
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL record per-run `elapsed_seconds` as the monotonic interval from the first agent process start to the last agent process exit, timeouts and spec review included and `auto init`, setup, calibration, and grading excluded, plus `phase_seconds` for `plan` and `go` from process boundaries and for `review` as the summed durations of the `auto spec review` calls the `auto` shim observed.

**REQ-HB2-12** review defect
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL record for the harness arm the review findings counted by severity and by final status from the SPEC's `review-findings.json` labelled `reported` with `verified: "unknown"`, because this pilot has no mechanical rule that links a finding to a failed hidden test, the string `n/a` for every review metric of the native arm, and for both arms `escaped_defects` as the number of expected pairs that failed on the final graded tree.

### 결과 분류

**REQ-HB2-23** outcome taxonomy
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL map every terminal state of a scheduled run to exactly one `operational_error` value and to the R1 effects of the table below, record `control_violations`, `provider_errors`, and `usage_status` as separate fields, and stop the session at the states marked as stopping.

| terminal state | `operational_error` | `delivered_pass` | R1 효과 | session |
|---|---|---|---|---|
| 모든 phase가 deadline 안에 exit 0 | `none` | 채점 결과 | 없음 | 계속 |
| agent 또는 harness `auto init` exit ≠ 0, provider 오류 아님 | `agent_failed` | false | fail | 계속 |
| deadline kill | `agent_timeout` | false | fail. killed call의 receipt가 없으면 `usage_unparsed` | 계속 |
| `plan_no_spec`, `plan_ambiguous_spec` | 같은 이름 | false | fail | 계속 |
| 어느 call이든 rate limit, 인증 실패, provider 불가 | `provider_rate_limited`·`provider_auth_failed`·`provider_unavailable` | false | `primary_undetermined` | 계속 |
| 통제 위반(`tool_policy`, `isolation`, `task_file_modified`, `model_substituted`, `unbrokered_egress`, `observation_incomplete`, `broker_cwd_refused`, `broker_argv_refused`, `review_read_outside_trial`) | `control_violation` | false | `control_violation` | 계속 |
| trial 도중 proxy·broker 종료 | `infra_stopped:<proxy\|broker>`, 행과 change set을 남기고 채점 | false | `primary_undetermined` | 멈춤 |
| trial 거부(`native_surface_present`, `config_home_contaminated`, `auth_expiry`) | `trial_refused:<code>` | false | `primary_undetermined` | 멈춤 |
| run 시작 전 `machine_changed`, `cli_version_changed`, `denial_log_unavailable`, proxy·broker 없음 | 행 없음, session `stop_reason` | – | 남은 run마다 `runs_missing` | 멈춤 |

- 상태가 겹치면 `operational_error`는 표의 아래 행이 우선한다. R1 효과는 세 field에서 모두 계산한다. provider 오류는 broker가 동결 classifier(claude `is_error`와 status, codex `error` event)로 판정한다. 마지막 행은 실행이 시작되지 않은 run만 덮는다. 멈춘 session은 재개하지 않는다(REQ-HB2-02).

### 보고서와 주장

**REQ-HB2-13** 보고서와 기계 판정
Priority: Must
Type: Event-driven
WHEN the blind grades are joined, THEN THE SYSTEM SHALL generate `docs/benchmarks/harness-feature-<date>.json` and `.md` by code only from `protocol.json`, `calibration.json`, `records.json`, and the joined grades, containing every scheduled run, the per-task table, the per-provider token table, the control conditions, the treatment and remaining-bias section, and the decision object `{rule_id, inputs, claim_allowed, reasons, claim_line}`, with no prompt, PR text, code, diff, transcript, home path, hostname, or credential, and with the SHA-256 of the JSON bytes printed in the Markdown.

- R1. 아래를 모두 만족할 때만 `claim_allowed: true`다. 실패 조건마다 reason을 이 순서로, 같은 규칙 안에서는 task id, 그다음 arm(`harness`, `native`) 순으로 남긴다.
  1. 12 run 모두 기록. 아니면 `runs_missing:<task>-<arm>`
  2. 통제 위반·infra failure 0. `control_violation:<task>-<arm>`, `primary_undetermined:<task>-<arm>`(provider 오류, trial 거부), `usage_unparsed:<task>-<arm>` 순
  3. harness `delivered_pass` ≥ native + 2. 아니면 `margin_below_2:<h>-<n>`
  4. native pass·harness fail task 0개. 그런 task마다 `reversal:<task>`
- `token_ratio`는 harness 6 run `tokens_total` 합(`not_applicable` 제외, setup 포함) ÷ native 6 run 합, `time_ratio`는 `elapsed_seconds` 합의 비다. 소수 둘째 자리 반올림. 한 run이라도 없거나 `unparsed`이거나 분모가 0이면 값은 문자열 `"undetermined"`다. 완결된 native run의 token은 0일 수 없으므로 0은 `unparsed`로 본다. provider 표는 `tokens_observed`를 `usage_status` 열과 함께 싣는다.
- `claim_line`은 `claim_allowed`가 true일 때만 아래 template으로 만들고 false면 `null`이다. `%`, `cheaper`, `faster`, `save`, `reduce`는 쓰지 않는다.

```text
Pilot (n=6 tasks from external open-source Go repositories merged after the model knowledge cutoff, 1 run per arm, no statistical significance): the full Autopus path (review panel claude+codex; the product default also includes gemini; project MCP servers off) passed {h}/6 hidden-test suites vs {n}/6 for native Claude Code, using {token_ratio}x tokens and {time_ratio}x wall time. Details: [report](docs/benchmarks/harness-feature-{date}.md).
```

**REQ-HB2-20** 처치와 남은 편향 명시
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL render in both report files a fixed section stating that the harness arm's additional provider (codex), subagents, and review rounds are the treatment being measured and not a bias, that the review panel claude+codex and the disabled project MCP servers are declared deviations from the product default, and listing every remaining bias recorded in the pre-registration with its reason.

**REQ-HB2-14** README 가드
Priority: Must
Type: Unwanted
IF the repo root `README.md` contains a line that links `docs/benchmarks/harness-feature-` while the linked report's JSON has `claim_allowed` other than `true`, or the line differs from that JSON's `claim_line`, or the Markdown's recorded JSON SHA-256 differs from the JSON bytes, THEN THE SYSTEM SHALL fail the offline guard test `[NEW] test_feature_readme_guard.py`.

- `claim_allowed`가 false인 동안 README 변경은 0줄이다(Q6).

**REQ-HB2-18** 외부 코드와 license
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL keep external repository clones, snapshots, PR metadata snapshots, and derived prompts only under the session or selection cache outside the git working tree, record for every selected task its repository URL, SPDX license id, and the allowlist decision, and commit no external source file, test file, PR text, or prompt to this repository.

**REQ-HB2-15** 기존 모드와 제품 코드 보존
Priority: Must
Type: Ubiquitous
THE SYSTEM SHALL keep the arguments, outputs, and tests of `run.py --mode pilot` (the default) and `run.py --mode golden` unchanged, keep `golden_protocol.RUNNER_FILES`, `grader.py`, `grader.sb`, `prepare_grader.py`, and `report.py` unchanged, import `feature` in `run.py` only inside the feature branch so that the module set golden loads stays equal to `RUNNER_FILES`, and change no file under `pkg/`, `internal/`, `content/`, `templates/`, or `evals/harness/`.

## 생성 파일 상세

모두 `scripts/benchmarks/harness/` 아래이고 범위는 `plan.md` T1–T9가 정한다. 변경은 `run.py`(분기 안 import)와 `README.md`(재현 절)다. `[NEW]`는 `feature*.py` 16개, `feature_agent.sb`, `feature_provider.sb`, `feature_goapi/`, `feature_prereg.json`, `test_feature_*.py`(S1–S16, S18, S19), 보고서 `docs/benchmarks/harness-feature-<date>.{md,json}`이다. Python 소스는 모두 300줄 이하다(`architecture.max_file_lines: 300`).

## Related SPECs

- SPEC-HARNESS-BENCH-001, SPEC-HARNEVAL-001: 재사용과 보존(REQ-HB2-15). sibling SPEC은 없다.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-HB2-01 | T1, T11 | S1, S7, S17 | INV-007, INV-004 |
| REQ-HB2-02 | T1 | S1 | INV-007 |
| REQ-HB2-03 | T2 | S2 | INV-014 |
| REQ-HB2-04 | T3 | S3 | INV-006 |
| REQ-HB2-05 | T4, T8 | S5 | INV-011 |
| REQ-HB2-06 | T4 | S6 | INV-012 |
| REQ-HB2-07 | T4 | S7 | INV-004 |
| REQ-HB2-08 | T5 | S8 | INV-012 |
| REQ-HB2-09 | T6 | S9 | INV-005, INV-016 |
| REQ-HB2-10 | T7 | S10, S14 | INV-003 |
| REQ-HB2-11 | T7 | S11 | INV-010 |
| REQ-HB2-12 | T8 | S12 | INV-011 |
| REQ-HB2-13 | T9, T13 | S13, S14, S17 | INV-001, INV-002, INV-008 |
| REQ-HB2-14 | T9, T13 | S15, S17 | INV-009 |
| REQ-HB2-15 | T1, T14 | S16 | INV-013 |
| REQ-HB2-16 | T2, T4 | S4 | INV-015 |
| REQ-HB2-17 | T0, T4 | S6 | INV-012 |
| REQ-HB2-18 | T2, T9 | S2, S14, S16 | INV-008 |
| REQ-HB2-19 | T0, T1, T10 | S1, S10 | INV-003 |
| REQ-HB2-20 | T9 | S14 | INV-008 |
| REQ-HB2-21 | T0, T5 | S18 | INV-017 |
| REQ-HB2-22 | T4, T5 | S8 | INV-012 |
| REQ-HB2-23 | T6, T7 | S9, S13, S18 | INV-018 |
| REQ-HB2-24 | T3 | S19 | INV-005, INV-006 |
| REQ-HB2-25 | T0, T5 | S8, S18 | INV-012, INV-017 |
