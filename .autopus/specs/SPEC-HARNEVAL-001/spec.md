# SPEC-HARNEVAL-001: Harness 변경 Golden-Task Eval 게이트 (결정적 PR lane + advisory live lane)

**Status**: approved
**Created**: 2026-10-06
**Domain**: HARNEVAL
**Module**: autopus-adk
**Source PRD**: `prd.md` (같은 디렉터리)
**Siblings**: SPEC-HARNEVAL-002(사고 → golden 후보), SPEC-HARNEVAL-003(서명된 live 증거와 release 차단)

## 목적

canonical harness source(`content/`, `templates/`, `pkg/content`, `pkg/adapter`와 생성 의존성)가 바뀌면 5개 플랫폼 생성 표면의 행동을 버전 관리되는 golden-task 세트로 평가한다. 회귀가 있으면 병합을 막는다. 실제 agent 행동은 maintainer host에서 baseline과 candidate를 짝지어 실행하고, 서명하지 않는 advisory report로 보여 준다. 사용자는 D1에서 HYBRID를 골랐다. 2026-10-06 결정으로 live lane의 서명과 release 차단은 SPEC-HARNEVAL-003으로 분리했다.

## Outcome Boundary

- Outcome Lock 원문은 `research.md`의 `## Outcome Lock`이다. 요약:
  - (a) harness 입력을 바꾸는 PR에서 required check `harness-eval`이 결정적 golden-task eval을 실행하고, committed baseline 대비 pass rate, regression delta, task별 전이를 보고하며, 회귀·vacuity·stale templates·근거 없는 기대값 변경·tombstone 없는 삭제에서 실패한다.
  - (b) maintainer가 macOS host에서 live lane을 실행하면 baseline arm과 candidate arm이 한 세션에서 격리된 grader로 채점된다. 그 결과는 서명하지 않는 `harness_live_advisory.v1` report로 나온다. report는 merge도 release도 막지 않는다. oracle이 실제로 빌드·실행되지 않은 세션은 `ok`가 될 수 없다.
- Mandatory requirements: REQ-HE-01 ~ REQ-HE-13 (Must). REQ-HE-14는 Should.
- Explicit non-goals: live 증거 서명, release 차단, GitHub Environment·secret, live lane GitHub workflow(모두 SPEC-HARNEVAL-003), Autopus/backend 변경, `pkg/evalregression` 변경, PR마다 live LLM 실행, golden 후보 intake·승격(SPEC-HARNEVAL-002), 기존 82개 contract test 이동·삭제, branch protection의 자동 설정, live 결과로 통계적 유의성 주장.
- Completion evidence: `acceptance.md`의 Must 시나리오 S1-S15 전부와 T14 OPS 증거(`harness-eval` required check 등록).

## Trust Model

- 신뢰: 실행 중인 checkout의 product, runner, parser 코드와 commit된 manifest·corpus·grader profile.
- 비신뢰 실행: agent, 그리고 agent가 고친 코드를 컴파일·실행하는 grader. 둘 다 Seatbelt sandbox 안에서만 돈다. 쓰기는 자기 workspace나 grade 사본에만 허용하고 network는 거부한다. 환경은 허용 목록뿐이다.
- 비밀: maintainer의 Codex credential은 codex 프로세스 환경에만 둔다. grader와 agent 명령은 이를 상속하지 않는다. 이 SPEC은 서명키와 GitHub secret을 다루지 않는다.

## Requirements

### REQ-HE-01 Golden-task 형식과 strict decode
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL strictly decode `harness_golden_task.v1` tasks and the `harness_golden_set.v1` manifest under `evals/harness/` according to the Wire Contracts section and reject unknown fields, trailing data, unknown assertion kinds, invalid assertion fields, duplicate task ids, zero-assertion tasks, out-of-range policy values, unclean or symlinked active paths, and active paths at or under `evals/harness/candidates/` with reason `invalid`.

- detail code: `unknown_field`, `trailing_data`, `unknown_assertion_kind`, `assertion_field_invalid`, `duplicate_task_id`, `no_assertions`, `policy_out_of_range`, `unclean_path`, `symlink_not_allowed`, `reserved_active_path`, `corpus_digest_mismatch`, `expected_tests_missing`.
- 경로 규칙: active path와 assertion path는 slash 구분 상대 경로이고 `path.Clean(p) == p`, `..`·절대 경로·빈 값이 없어야 한다. active path는 `evals/harness/` 아래여야 하며, 정규화 뒤 `evals/harness/candidates`와 같거나 그 아래이면 거부한다. loader는 `os.Lstat`로 걷고 symlink를 만나면 거부한다.
- runner는 manifest `active_paths`만 로드한다. 그 밖의 파일(SPEC-HARNEVAL-002 quarantine 포함)은 읽지 않는다.

### REQ-HE-02 Hermetic 생성과 typed assertion
Priority: Must · EARS: EventDriven

WHEN the deterministic lane runs, THEN THE SYSTEM SHALL generate every declared variant surface for the five platforms in-process into a fresh temp root with the generator version, project name, and host probes pinned to manifest values, and evaluate the typed assertions without network access, LLM calls, or writes outside the temp root.

- 플랫폼: `claude-code`, `codex`, `antigravity-cli`, `opencode`, `omp`. 생성은 `adapter.PlatformAdapter.Generate(ctx, cfg)` 패턴(`pkg/adapter/latest_cli_contract_test.go::generateLatestCLIFixture`)이다. config는 `config.DefaultFullConfig(pins.project_name)`에 variant override를 적용한 값이다.
- version 고정: `pkg/adapter/codex/codex_plugin_manifest.go::codexPluginVersion`은 `version.Version()`(ldflags, BuildInfo, `dev` 순)을 읽는다. in-process 경로는 `[NEW] codex.WithPluginBaseVersion(pins.generator_version)`로, 다른 revision의 driver는 `-ldflags "-X github.com/insajin/autopus-adk/pkg/version.version=<pins.generator_version>"`로 같은 값을 넣는다. probe A2에서 version 문자열만 바꿔도 표면 digest가 달라졌다.
- host probe 고정: `codex.WithModelCatalog`, `codex.WithCLIVersion`, `opencode.WithCLIVersion`. sentinel이 잡는 다른 probe는 `[NEW]` pin option으로 고정한다.
- sentinel: 생성 동안 `PATH`는 `codex`, `opencode`, `claude`, `agy`, `gemini`, `omp`, `git` sentinel만 있는 temp 디렉터리, `HOME`은 빈 temp 디렉터리다. sentinel이 실행되면 reason `host_probe_unpinned`와 binary 이름으로 실패한다.
- task 평가: variant마다 모든 assertion을 각 assertion의 `platform` 표면에서 평가한다. `variants`가 빈 배열이면 기본 config 하나만 쓴다. task는 모든 (variant, assertion) 평가가 통과할 때만 pass다. assertion이 1개 이상이므로 평가 조합은 비지 않는다.

### REQ-HE-03 Baseline 비교와 결과 문서
Priority: Must · EARS: EventDriven

WHEN surface evaluation completes, THEN THE SYSTEM SHALL write a `harness_eval_result.v1` document compared against the committed `harness_eval_baseline.v1` and exit 1 for any pass-to-fail transition, missing task without tombstone, expectation change without baseline update, or set digest mismatch.

- 지표는 active surface task만 센다: `pass_rate = passed_surface / executed_surface`, `baseline_pass_rate = baseline에서 kind surface, state active인 행 중 result pass 수 / 그 행 수`(kind는 baseline 행에 기록된 역사적 값이라 tombstone 없이 지운 task도 분류된다), `regression_delta = pass_rate − baseline_pass_rate`. 허용 오차 1e-9. baseline 파일이 없거나 active surface 행이 0개면 precondition `baseline_missing`으로 실패하고, `auto eval harness baseline --init`으로만 첫 baseline을 만든다.
- 전이 kind: `regression`, `improved`, `new`, `retired`, `task_missing`, `expectation_changed`. 변화 없는 task는 넣지 않고, task id byte 오름차순으로 정렬한다. `failure_reasons`는 중복 없는 오름차순 배열이다. `improved`만 있으면 status `pass`, exit 0이다.
- 출력 채널: `--output <file>`은 결과 문서를 파일로 쓴다. `--format json`이면 stdout에는 결과 JSON만 쓰고, 사람용 안내(예: `auto eval harness baseline --update`로 고정)는 stderr에 쓴다.
- `expectation_digest` = SHA-256 hex(Go `json.Marshal` of `{kind, variants, assertions, corpus_ref, expected_tests}`). agent task의 기대 행동은 `corpus_ref.file_sha256`과 `expected_tests`로 들어간다. surface task에서 두 필드는 빈 값이다. `intent`/`outcome` 문구는 포함하지 않는다.
- `set_digest` = SHA-256 hex(`set_version` + id 오름차순 `{id, state, expectation_digest}` 배열의 `json.Marshal`). `agent_set_digest`는 같은 방식으로 agent task만 계산한다.
- `surface_digest` = 기본 config × 5 platform 생성 결과의 `relpath\x00sha256(content)\n` 행을 정렬해 이은 문자열의 SHA-256 hex. timestamp를 담는 ADK bookkeeping인 `.autopus/txns/**`와 `.autopus/<platform>-manifest.json`은 닫힌 제외 목록으로 뺀다(probe A2에서 실행 간 차이가 난 파일은 이 두 종류뿐이었다).
- `produced_at`만 시간에 의존한다. 같은 tree의 두 결과는 `produced_at`을 빼면 byte-identical이다.

### REQ-HE-04 Vacuity와 stale templates 방어
Priority: Must · EARS: Unwanted

IF the executed surface set differs from the declared active surface set, an active task count is below its floor, any task is invalid, or the template regeneration comparison differs or reports a walk, read, or regeneration error, THEN THE SYSTEM SHALL fail the run with reason `vacuous`, `invalid`, or `templates_stale` and never report pass.

- 검사 순서: load(`invalid`, `baseline_missing`) → stale templates(`templates_stale`) → 생성(`host_probe_unpinned`) → 평가 → vacuity·baseline 비교. precondition은 즉시 중단하고 reason 하나만 낸다. 평가 단계 reason(`expectation_changed`, `regression`, `set_digest_mismatch`, `task_missing`, `vacuous`)은 모두 모아 정렬한다.
- PR lane은 agent task를 실행하지 않는다. agent task는 schema와 `corpus_ref` digest만 검사하고, 유효한 active agent task 수가 `floors.agent_tasks`보다 작으면 `vacuous`다. live lane은 실행한 agent 집합이 active agent 집합과 같아야 한다.
- 재생성 비교는 `[NEW] pkg/content/regen_drift.go::DetectTemplateRegenDrift(dir) ([]string, error)`로 추출한다. 기존 `diffRegeneratedTemplates`가 버리던 WalkDir, Rel, ReadFile 오류를 반환한다. `internal/cli`에는 기존 test가 호출하는 `detectTemplateRegenDrift`와 `diffRegeneratedTemplates` 이름을 thin wrapper로 남겨 doctor의 advisory 동작(오류면 조용히 건너뜀)을 유지한다. eval은 오류를 detail `regen_failed`인 `templates_stale`로 처리한다.

### REQ-HE-05 CI job과 적용 여부 판단
Priority: Must · EARS: EventDriven

WHEN `ci.yaml` runs for a pull request, a push to main, or a workflow call, THEN THE SYSTEM SHALL run a `harness-eval` job that always reports its check, evaluates every non-pull-request event, evaluates a pull request whose changed paths intersect the derived harness input set, and otherwise exits 0 with status `not_applicable`.

- workflow 수준 `paths`/`paths-ignore`와 job 수준 `if:`를 쓰지 않는다. 판단 규칙은 "event가 `pull_request`가 아니면 평가"이므로 release의 `workflow_call`에서 event 값이 `push`이든 `workflow_call`이든 결과가 같다.
- changed paths = `git diff --name-only --no-renames <base_sha>...<head_sha>`(`fetch-depth: 0`). rename은 원래 경로와 새 경로가 모두 나온다.
- 파생 입력 집합: `content/**`, `templates/**`, `evals/harness/**`, `scripts/benchmarks/harness/**`(corpus, runner, driver), `go.mod`, `go.sum`, `.github/workflows/ci.yaml`, `internal/cli/eval_harness*.go`, 그리고 `go list -deps ./pkg/harneval`의 in-module package 디렉터리. 디렉터리 일치는 path segment 경계로 본다. `go list`가 실패하면 건너뛰지 않고 reason `closure_unavailable`로 평가한다.
- merge 차단에는 `harness-eval`을 main의 required status check로 등록하는 OPS 작업(T14)이 필요하다. 등록 증거가 없으면 sync 완료가 아니다.

### REQ-HE-06 명시적 baseline 갱신
Priority: Must · EARS: EventDriven

WHEN `auto eval harness baseline --update` runs, THEN THE SYSTEM SHALL rewrite the baseline only after every pass-to-fail transition is named by `--accept-regression <task-id>` with a non-empty `--reason` and every task removed from the set has a retired tombstone, and otherwise exit 1 with `regression_not_accepted`, `accept_reason_required`, or `tombstone_required` while leaving the baseline byte-identical.

- 두 검사는 독립이다. 기대값이 바뀐 task라도 result가 pass→fail이면 수락이 필요하다.
- retired 행은 baseline에 `state: retired`와 `retired_reason`으로 영구히 남는다. 그 뒤 tombstone 파일을 지워도 `task_missing`이 아니다. 수락된 행은 `result: fail`과 `accepted_regression_reason`을 남긴다.
- baseline은 timestamp 없이 id 오름차순 행으로 쓴다. precondition 실패에서는 갱신하지 않는다. `--init`은 baseline이 없을 때만 동작한다.

### REQ-HE-07 Live lane A/B와 protocol 동결
Priority: Must · EARS: StateDriven

WHERE the live lane runs in golden mode on a maintainer macOS host, THEN THE SYSTEM SHALL run every active agent task in a baseline arm and a candidate arm on one pinned workspace revision within one session, generate each arm surface with a driver built from that arm revision, freeze the session inputs in a protocol created exclusively before the first trial, and record every scheduled attempt without outcome-dependent retries.

- 입력은 모두 commit된 manifest에서 온다: `live.workspace_revision`(corpus mutation을 작성한 40-hex 커밋), `live.baseline_ref`(직전 release tag), `live.model`. 두 arm은 같은 workspace snapshot을 쓰고 표면만 다르다. pin은 두 arm 모두 candidate manifest의 `pins`를 쓴다.
- 표면 생성: arm revision을 `git archive`로 temp 디렉터리에 풀고, `[NEW] scripts/benchmarks/harness/surface_driver/main.go`를 복사해 `go build -trimpath`와 위 version ldflags로 빌드한 뒤 실행한다. driver는 v0.50.122에도 있는 API(`config.DefaultFullConfig`, `config.Save`, `<platform>.NewWithRoot`, `codex.WithModelCatalog`, `codex.WithCLIVersion`, `opencode.WithCLIVersion`, `Generate`)만 쓴다. 빌드가 실패하면 `baseline_ref_unsupported`로 시작을 거부한다.
- 시작 전 거부(agent 호출 0회): macOS가 아니면 `unsupported_os`(pilot permission profile이 Seatbelt 전용이다), `codex --version` ≠ `pins.codex_cli_version`이면 `codex_cli_version_mismatch`, 어떤 task의 mutation이 workspace에서 정확히 1회 일치하지 않으면 `workspace_mutation_mismatch`, `tasks × K × 2 > max_agent_runs`면 `run_cap_exceeded`, protocol 파일이 있으면 `protocol_exists`(O_EXCL).
- 균형 순서: trial `t`(0..K-1)마다 task를 id 오름차순으로 돌고 `(t + task_index)`가 짝수면 baseline arm 먼저 실행한다.
- protocol에는 grader 입력의 digest도 남긴다: `runner_sha256`(실행 checkout의 `golden.py`), `grader_profile_sha256`(실행 checkout의 `[NEW] scripts/benchmarks/harness/grader.sb`). corpus oracle은 `corpus_digests`로 고정된다. advisory report가 이 값을 그대로 옮기고, 서명 lane(SPEC-HARNEVAL-003)이 trusted 값과 대조한다.
- 세션 안에서는 재시도하지 않는다. 세션 간 best-of-N(여러 번 돌려 좋은 결과만 남기기)은 advisory lane에서 막지 않는다. 결과가 merge나 release를 결정하지 않기 때문이다(REQ-HE-11의 수용한 잔여 위험). 서명 증거의 세션 집계는 SPEC-HARNEVAL-003이 소유한다.

### REQ-HE-08 Trial outcome, 격리 채점, trusted record
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL classify a trial as `error` only for failures before the arm surface enters the trial workspace, as `pass` only for an accepted oracle result from an isolated grader with a clean scope audit, and as `fail` for every other result, and write every record only from the trusted runner process.

- 신호 매핑(record `signal`): error = `workspace_setup_failed`, `mutation_failed`, `warmup_failed`. fail = `agent_launch_failed`, `agent_exit_nonzero`, `agent_timeout`, `forbidden_construct`, `oracle_failed`, `oracle_timeout`, `oracle_output_invalid`, `scope_violation`, `observation_failed`. pass = `accepted`. 단계 순서: snapshot → mutation → warmup → arm 표면 복사 → agent → scope audit → 채점. error trial은 agent를 부르지 않고 재시도하지 않는다. 채점은 agent 단계가 실패해도 실행하고, `forbidden_construct`나 `scope_violation`일 때만 건너뛴다. trusted parser는 record의 `oracle{ran, build_failed, expected_passed, expected_failed}`를 채운다. `ran`은 `expected_tests` 중 하나의 test 수준 이벤트(`run`, `pass`, `fail`, `skip`)가 있었는지이고, `build-fail`·`build-output`만 있으면 false다. 따라서 candidate 표면은 error를 만들 수 없다.
- grader 격리: pilot은 oracle을 profile 없이 상속 환경으로 실행한다(`run.py:105`, `execute()`의 `os.environ.copy()`). golden 모드는 oracle을 별도 프로세스로 `sandbox-exec -f grader.sb`에서 실행한다. profile은 network를 모두 거부하고, 그 trial의 새 grade 사본 밖 쓰기를 거부한다. 따라서 records, protocol, 출력 디렉터리, 다른 trial, runner checkout, `GITHUB_ENV` 류 파일은 쓸 수 없다. grade 사본은 workspace snapshot을 새로 복사하고 mutation을 적용한 뒤 `run.py::copy_candidate`로 허용 파일만 옮긴 것이다. 환경은 `env -i` 뒤 허용 목록(`PATH`=toolchain, grade 안의 `HOME`·`TMPDIR`·`GOPATH`, 그 trial 전용 `GOCACHE` clone, 세션의 읽기 전용 `GOMODCACHE`, `GOFLAGS=-mod=mod`, `GOPROXY=off`, `GOSUMDB=off`, `GOTOOLCHAIN=local`)뿐이다(REQ-HE-09). Codex credential과 `GITHUB_*`, `ACTIONS_*`, `RUNNER_*`는 없다(probe A3).
- grader 결과: grader는 결과 파일을 직접 쓰지 않는다. trusted runner가 grader stdout(`go test -json`)을 trial당 파일 하나(`grader.jsonl`, 최대 1 MiB)로 받는다. trusted parser(실행 checkout의 `golden.py`)가 schema와 크기 제한으로 읽는다. pass는 네 조건을 모두 요구한다: 종료 코드 0, 제한 안의 올바른 test2json 이벤트, task `expected_tests` 전부의 top-level pass 이벤트, 그 이름들의 fail 이벤트 0개. 하나라도 어긋나면 `oracle_output_invalid` 또는 `oracle_failed`다. `-run`이 prefix 정규식인 oracle(corpus a04 `^TestEvaluateGate_`, 통과 test 7개)이 있으므로 이름은 정규식에서 뽑지 않고 task에 고정한다(probe A1).
- 위조 완화(advisory 수준): agent diff의 허용 파일에 `func init(`, `os.Exit`, `syscall.`, `unsafe.`, `//go:linkname`이 리터럴로 새로 생기면 채점 전에 `forbidden_construct`로 fail한다. oracle test 파일은 corpus에서 오며 agent가 바꿀 수 없다. 이 리터럴 검사는 alias import(`import s "syscall"`)나 `var _ = f()` 초기화로 우회되고, 같은 process의 코드가 test2json framing을 위조할 수 있다. advisory lane에서는 이를 수용한 잔여 위험으로 둔다(REQ-HE-11). 차단 lane에서 쓰기 전에 고칠 일은 SPEC-HARNEVAL-003의 must-resolve 항목이다.
- agent 격리: codex는 pilot permission profile(Seatbelt, network off)로 돈다. maintainer의 Codex credential은 codex 프로세스 환경에만 넣고, codex가 실행하는 명령에는 shell environment 상속 정책으로 넘기지 않는다(설치된 codex 버전의 정책 key는 T12에서 확인한다). trial마다 agent와 grader의 process group을 종료하고 남은 process가 0개인지 확인한다.
- prompt injection 완화: prompt는 digest로 고정된 corpus에서만 오고, workspace는 고정 revision snapshot이며, 외부 fetch는 없다. 판정은 oracle, parser, scope audit만 따른다.

### REQ-HE-09 Grader 빌드 환경과 oracle calibration
Priority: Must · EARS: Unwanted

IF the trusted preparation cannot build the read-only module cache and warm build cache, or any agent task oracle fails on the clean workspace or passes on the mutated workspace under the grader profile, THEN THE SYSTEM SHALL refuse the session with `oracle_calibration_failed` before any agent call.

- trusted 준비(sandbox 밖, agent 전): workspace snapshot의 `go.mod`·`go.sum`으로 세션 전용 module cache를 `go mod download`로 채운다. 출처는 `GOPROXY`나 maintainer module cache의 file proxy이고, `go.sum`으로 검증한다. 그 뒤 cache를 읽기 전용으로 바꾼다. 다음으로 oracle package를 컴파일해 warm build cache를 만든다. trial마다 이 build cache를 APFS clone(`cp -c -R`)으로 grade 안에 복사한다. agent가 고친 코드가 다른 trial의 build cache를 오염시킬 수 없다.
- 거부할 때 trusted runner는 예정 `order`를 담은 protocol과 `calibration.json`(`status: failed`, task별 결과)을 쓰고 record는 쓰지 않은 채 종료 코드 1로 끝난다. 모든 trial이 끝나면 같은 calibration을 한 번 더 실행해 `calibration.json`의 `after`에 남긴다. 세션 중 grader 환경이 망가진 경우를 잡기 위해서다.
- calibration: 같은 grader profile에서 각 agent task의 oracle을 두 번 실행한다. 변형 전 workspace에서는 `expected_tests`가 모두 pass여야 하고, mutation을 적용한 workspace에서는 accept되지 않아야 한다. 하나라도 어긋나면 세션을 시작하지 않는다.
- 근거(probe A1, 이 macOS host): trusted 준비가 성공했다(warm 10.9초). corpus 12개 oracle은 변형 전 tree에서 모두 exit 0이었다. mutation을 적용하면 12/12가 accept되지 않았다. 빈 module cache에서는 exit 1, pass 이벤트 0개였다. 이 실행에서 repo 파일은 바뀌지 않았다.

### REQ-HE-10 Advisory verdict와 report
Priority: Must · EARS: EventDriven

WHEN a live session completes, THEN THE SYSTEM SHALL compute the verdict from the frozen protocol, the records, and the calibration result and write an unsigned `harness_live_advisory.v1` report with the arm pass rates, regression delta, hard flips, completeness, calibration status, and verdict.

- 입력은 protocol, record, `calibration.json` 세 가지다. `grader.jsonl`은 진단용이며 report 입력이 아니다. 먼저 calibration을 본다. `before`나 `after`가 `passed`가 아니면 record가 비어 있어야 하고(아니면 `records_protocol_mismatch`), verdict는 `vacuous`, reason은 `oracle_calibration_failed`다. calibration이 통과했으면 모든 record의 `session_id`가 protocol과 같고 `(task_id, arm, trial)` 집합이 protocol `order`와 정확히 같아야 한다. 어긋나면 `records_protocol_mismatch`로 report를 쓰지 않는다. SPEC-HARNEVAL-003의 signer에서는 calibration이 실패한 세션의 record 집합이 order와 달라 서명되지 않는다. 그러면 그 run은 release에서 `run_not_ok`로 센다(fail-closed).
- 공식: arm별 `pass_rate = passes / valid`(valid = outcome ≠ error), `regression_delta = pass_rate(candidate) − pass_rate(baseline)`, `completeness = (valid_baseline + valid_candidate) / (2 × tasks × K)`. hard flip은 두 arm 모두 valid trial이 K개이고 baseline pass = K, candidate pass = 0인 task다.
- 회귀 판정은 정수 연산이다: `10000 × (candidate_pass × baseline_valid − baseline_pass × candidate_valid) < threshold_bp × baseline_valid × candidate_valid`. `threshold_bp`는 protocol 값(초기값 −1000)이다. 정확히 경계인 delta는 회귀가 아니다.
- `verdict`/`reason`의 우선순위: `vacuous` > `incomplete` > `regression`(`hard_flip`, 그다음 `pass_rate_regression`) > `ok`(`within_threshold`). `vacuous`는 `calibration.json`이 없거나 `before`·`after` 중 하나가 `passed`가 아닐 때, 또는 한 arm이라도 `oracle.ran`이 true인 record가 0개일 때다. 빌드 실패만 있는 trial은 `ran` false이므로, 모든 trial이 빌드 실패로 함께 fail한 세션은 `ok`가 아니라 `vacuous`다. `incomplete`는 completeness < `completeness_floor`이거나 어느 arm의 valid가 0일 때다. valid가 0인 arm의 `pass_rate`는 JSON `null`(Go `*float64`)이고, `regression_delta`는 0으로 쓴다. 따라서 NaN이 생기지 않는다. 따라서 oracle이 빌드·실행되지 않은 세션은 `ok`가 될 수 없다.
- report 필드: `schema_version`, `advisory: true`, `session_id`, `started_at`, `workspace_revision`, `baseline_ref`, 두 `surface_digest`, `agent_set_digest`, `runner_sha256`, `grader_profile_sha256`, `policy`, arm별 `passes`·`valid`·`pass_rate`, `regression_delta`, `hard_flips[]`, `completeness`, `calibration{status, tasks}`, `verdict`, `reason`. raw prompt, transcript, payload는 없다.
- `auto eval harness report --input <session-dir> --format json`은 report를 stdout에 쓰고 verdict와 관계없이 종료 코드 0이다. 입력이 잘못됐을 때만 1이다.

### REQ-HE-11 Advisory 경계
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL keep the live lane advisory by producing no signature, reading no signing key or GitHub secret, adding no GitHub workflow for the live lane, and leaving merge and release decisions to the deterministic lane alone.

- report schema는 `eval_regression_report.v1`과 다르고 서명이 없다. `auto check --eval-regression`의 strict 경로는 decode 전에 attestation부터 검증한다(`internal/cli/eval_regression.go`: missing precedes verify, verify precedes decode). 그래서 advisory report는 decode까지 가지 않고 `artifact_unsigned`로 거부된다(S12). `release.yaml`, `ci.yaml`, `pkg/evalregression`, allowlist는 live lane 때문에 바뀌지 않는다.
- 수용한 잔여 위험(advisory 전용, 근거: report가 어떤 gate의 입력도 아니다):
  - A-F-017 best-of-N, run·artifact 삭제, 재실행. maintainer가 좋은 결과만 공유할 수 있다. 영향은 사람의 판단에 그친다.
  - A-F-001 grader stdout 위조와 `forbidden_construct` 우회. 위조된 `ok`는 정보를 왜곡할 뿐 병합·release를 열지 못한다.
  - F-004 runner·profile digest의 binding 누락. binding이 없으므로 해당하지 않는다. report에는 두 digest를 그대로 싣는다.
- 셋 다 SPEC-HARNEVAL-003이 서명·차단 lane에 쓰기 전에 고쳐야 하는 요구사항으로 옮겼다.

### REQ-HE-12 Seeded mutation self-test
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL include a self-test that applies each of at least five fixed seeded mutations to the generated surface and proves that every mutated surface fails with exactly the task ids listed in the committed mutation table as `regression` transitions while the unmutated surface passes.

- mutation: (M1) `.claude/settings.json`의 managed PreToolUse hook 항목 제거, (M2) router의 route→detail 매핑 파괴, (M3) 한 플랫폼에서 prompt 계약 구문 제거, (M4) `hooks.pre_commit_arch=false` variant 평가에 기본(flag on) 표면을 넣어 flag 무시를 흉내, (M5) skill 노출 파일 누락. 기본 config는 이미 `PreCommitArch: true`(`pkg/config/defaults.go:84`)이므로 의미 있는 variant는 `false`다.
- mutation은 생성 직후, assertion 직전 표면에 적용한다.

### REQ-HE-13 초기 golden set
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL ship an initial golden set with at least 20 active surface tasks covering all five platforms and at least four categories, at least 60 percent of surface tasks asserting two or more platforms or two or more paths, and at least 12 agent tasks imported from the existing benchmark corpus by file digest reference.

- category: `routing`, `hooks_settings`, `prompt_contract`, `agent_skill_exposure`, `generated_root_hygiene`. 기존 contract test 단언 하나를 다시 적은 task는 review에서 거부한다(기계 검사 불가, reviewer focus).

### REQ-HE-14 CI job summary
Priority: Should · EARS: EventDriven

WHEN the deterministic lane runs in CI, THEN THE SYSTEM SHALL write a body-free category table and the transition list to `$GITHUB_STEP_SUMMARY` and upload the result document as a workflow artifact.

## Wire Contracts

| 문서 | 필수 필드 |
|------|-----------|
| `harness_golden_set.v1` | `schema_version`, `set_version`, `active_paths[]`(초기값 `evals/harness/tasks/surface`, `evals/harness/tasks/agent`), `floors{surface_tasks≥1, agent_tasks≥1}`, `live{k≥1, threshold_bp∈[−10000,0], completeness_floor∈(0,1], max_agent_runs≥1, trial_timeout_seconds≥1, workspace_revision(40-hex), baseline_ref, model}`, `pins{generator_version, project_name, codex_model_catalog(파일 경로 또는 빈 값), codex_cli_version, opencode_cli_version}` |
| `harness_golden_task.v1` | `schema_version`, `id`(`^GT-[A-Z][A-Z0-9-]{2,40}$`), `kind: surface\|agent`, `category`, `intent`, `outcome`(기대 행동 문구, 비면 invalid), `variants[]`(`{name, overrides{key: value}}`, 허용 key는 `hooks.pre_commit_arch` bool), `assertions[]`(≥1), agent만 `corpus_ref{file, task_id, file_sha256}`와 `expected_tests[]`(oracle이 통과해야 하는 top-level test 이름, ≥1, 없으면 detail `expected_tests_missing`), `provenance{kind: manual\|benchmark\|incident, ref, fingerprint?}`, `status{state: active\|retired, reason}` |
| `harness_eval_baseline.v1` | `schema_version`, `set_version`, `set_digest`, `rows[]{id, kind: surface\|agent, state, result: pass\|fail\|not_run, expectation_digest, accepted_regression_reason?, retired_reason?}` id 오름차순. `kind`는 기록 당시 값으로 고정된다. surface 행의 result는 pass나 fail이고, agent 행은 PR lane이 실행하지 않으므로 항상 `not_run`이다. agent 행의 변화는 `expectation_digest`로만 드러난다 |
| `harness_eval_result.v1` | `schema_version`, `status: pass\|fail\|not_applicable`, `failure_reasons[]`, `details[]`, `totals{declared_surface, executed_surface, passed_surface, declared_agent}`, `pass_rate`, `baseline_pass_rate`, `regression_delta`, `transitions[]{task_id, kind}`, `categories[]{category, passed, total}`, `set_digest`, `surface_digest`, `produced_at` |
| `harness_golden_live_protocol.v1` | `session_id`(128-bit hex), `started_at`, `workspace_revision`, `baseline_ref`, `baseline_surface_digest`, `candidate_surface_digest`, `agent_set_digest`, `corpus_digests[]`, `runner_sha256`, `grader_profile_sha256`, `calibration{status, tasks[]}`, `policy{…}`, `pins{…}`, `cli_version`, `model`, `order[]{task_id, arm, trial}`, `prompt_layers[]` |
| `harness_live_advisory.v1` | REQ-HE-10의 report 필드. 서명 필드가 없고 `advisory: true`다. valid가 0인 arm의 `pass_rate`는 `null`이다 |
| `harness_golden_calibration.v1` (`calibration.json`) | `session_id`, `before{status: passed\|failed, tasks[]{task_id, clean_accepted, mutated_accepted}}`, `after{…}`(trial을 하나라도 실행한 뒤에만) |
| `grader.jsonl` | trusted runner가 받은 `go test -json` stdout. 최대 1 MiB, 한 줄에 test2json 이벤트 하나. 고정 test 이름의 pass 이벤트가 판정 근거다 |
| `harness_golden_live_record.v1` | `session_id`, `task_id`, `arm: baseline\|candidate`, `trial`, `outcome: pass\|fail\|error`, `signal`(REQ-HE-08 목록), `oracle{ran, build_failed, expected_passed, expected_failed}`, `duration_s` |

Assertion kind(닫힌 집합). 모든 kind는 `platform`(5개 이름 중 하나)과 생성 root 기준 `path`를 갖는다. 예외는 `section_parity`다.

| kind | 추가 필드 | 판정 |
|------|-----------|------|
| `file_exists` / `file_absent` | - | 파일이 있음 / 없음 |
| `contains` / `not_contains` | `needle` | 파일 bytes에 `needle`이 정확히(대소문자 구분) 있음 / 없음 |
| `json_path_present` / `json_path_absent` | `json_path`, `value_contains?` | `json_path` 문법: `segment('.'segment)*`, segment = `[A-Za-z0-9_-]+` 뒤에 선택적 `[N]` 또는 `[*]`. present는 해석된 값이 1개 이상(`value_contains`가 있으면 그것을 포함하는 문자열 값이 1개 이상), absent는 그 부정. JSON이 아니면 detail `json_invalid`로 fail |
| `route_detail` | `route`, `detail` | `path`의 어떤 줄이 `route`와 `detail`을 함께 담고, `detail` 파일이 같은 플랫폼 표면에 있음 |
| `section_parity` | `files[]{platform, path}`(≥2), `heading` | 각 파일에 `heading`과 똑같은 줄이 있고, 다음 동급 이상 heading까지의 본문(줄 끝 공백 제거)이 모든 파일에서 byte-identical |

## Existing Test Changes

이 SPEC은 기존 test를 바꾸지 않는다. rev 4에서 allowlist와 `release.yaml` 변경이 SPEC-HARNEVAL-003으로 옮겨 갔기 때문이다. `ci.yaml`을 읽는 기존 test는 4개 파일에 8개다. 그중 셋이 새 `harness-eval` job을 제약하며, 이 SPEC은 그 제약을 지킨다.
- `TestReleaseWorkflow_UsesOnlyImmutableActions`: 모든 job step의 `uses`는 40-hex SHA여야 한다.
- `TestSecurityWorkflow_JobsHaveBoundedTimeouts`: `ci.yaml` 어디에도 `version: latest`가 없어야 한다. 새 job도 `timeout-minutes`를 둔다.
- `TestOMPNativeSmokeCIPinsExactBinaryAndTest`: `  omp-native-smoke:`부터 `  macos-runtime:`까지의 구간에 `./...`, `${{ secrets.`가 없어야 하고 `live_smoke ` 호출이 정확히 2개여야 한다. 새 job은 그 구간 밖(`static-contracts` 다음)에 둔다.

나머지는 job별 조회나 고정 문자열 검사라 새 job과 충돌하지 않는다. `doctor_drift_source_test.go`는 wrapper 이름 유지로 그대로 컴파일된다.

## Prompt Layer Manifest Contract

| Layer | 내용 | 식별자 | 무효화 관측 |
|-------|------|--------|-------------|
| stable | arm별 생성 harness 표면, agent set | arm별 `surface_digest`, `agent_set_digest` | digest 변화 |
| snapshot | corpus task prompt, workspace revision, model id, CLI 버전 | `corpus_digests`, `workspace_revision`, `model`, `cli_version` | 값 변화 |
| ephemeral | trial workspace, agent 출력 | 저장하지 않음 | 해당 없음 |

raw prompt, transcript, raw payload는 protocol·record·report·attestation 어디에도 넣지 않는다(`raw_payload_present=false`).

## 생성 파일 상세

- `[NEW] pkg/harneval/`: `schema.go`, `load.go`, `assert.go`, `jsonpath.go`, `generate.go`, `sentinel.go`, `digest.go`, `compare.go`, `result.go`, `summary.go`, `applicable.go`, `verdict.go`, `records.go`, `advisory.go`와 각 `_test.go`, `mutation_test.go`. 파일당 300줄 이하, package coverage 85% 이상.
- `[NEW] internal/cli/eval_harness.go`, `eval_harness_run.go`, `eval_harness_baseline.go`, `eval_harness_report.go`: `auto eval harness run|baseline|applicable|digest|report`.
- `[NEW] pkg/adapter/codex` option `WithPluginBaseVersion`. `[NEW] pkg/content/regen_drift.go`. `internal/cli/doctor_drift_source.go`는 기존 함수 이름을 wrapper로 유지한다.
- `[NEW] evals/harness/`: `manifest.json`, `tasks/surface/*.json`, `tasks/agent/*.json`(`corpus_ref`, `expected_tests`), `baseline.json`, `fixtures/`, `README.md`.
- `[NEW] scripts/benchmarks/harness/golden.py`, `test_golden.py`, `surface_driver/main.go`, `grader.sb`, `prepare_grader.py`(module cache·warm cache 준비와 calibration). `run.py`에는 `--mode golden` 분기만 추가한다.
- `.github/workflows/ci.yaml`(`harness-eval` job), `[NEW] internal/cli/eval_harness_workflow_test.go`.

## Related SPECs

- SPEC-HARNEVAL-002 (sibling): 이 SPEC의 T1(loader, manifest), T2, T3, T4(digest), T5, T6(CLI 골격)에 의존한다. 이 SPEC이 소유하는 인터페이스는 `harness_golden_task.v1`, `provenance`/`status`, `active_paths`, quarantine 경로 거부, `## Existing Test Changes`다. rev 4가 추가한 agent 전용 `expected_tests`는 002가 만드는 surface 초안과 무관하다.
- SPEC-HARNEVAL-003 (sibling): 이 SPEC의 live lane(REQ-HE-07 ~ REQ-HE-10)을 GitHub workflow, 서명, release 차단으로 확장한다. trusted protocol 재구성, binding digest, run 집계, 수용한 잔여 위험 세 가지의 해소를 소유한다.
- SPEC-HARNESS-BENCH-001(pilot 불변), SPEC-HARNESS-EFFICIENCY-001(observational 불변), SPEC-ADK-DRIFT-GATE-001(doctor advisory 불변).

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-HE-01 | T1, T7 | S1, S14 | INV-HE-05 |
| REQ-HE-02 | T2, T3 | S2 | INV-HE-04 |
| REQ-HE-03 | T4, T6 | S3, S4 | INV-HE-01, INV-HE-02, INV-HE-03 |
| REQ-HE-04 | T5 | S5 | INV-HE-05 |
| REQ-HE-05 | T6, T9, T14 | S6 | INV-HE-06 |
| REQ-HE-06 | T6 | S7 | INV-HE-07 |
| REQ-HE-07 | T12, T13 | S8 | INV-HE-08 |
| REQ-HE-08 | T11, T12 | S11 | INV-HE-12 |
| REQ-HE-09 | T11 | S10 | INV-HE-10 |
| REQ-HE-10 | T10 | S9, S12 | INV-HE-09, INV-HE-10 |
| REQ-HE-11 | T10, T12 | S12 | - |
| REQ-HE-12 | T8 | S13 | INV-HE-02 |
| REQ-HE-13 | T7 | S14 | INV-HE-11 |
| REQ-HE-14 | T9 | S16 | INV-HE-02 |
| Brownfield (CD-6) | T15 | S15 | - |

## Review Resolution (rev 2 ~ rev 4)

| Finding | 최종 처리 | 위치 |
|---------|-----------|------|
| F-002 (rev 4, critical) grader가 corpus oracle을 빌드할 수 없음 | trusted 준비가 읽기 전용 `GOMODCACHE`와 trial별 build cache clone을 만든다. calibration을 넣고 `expected_tests`를 고정했다. probe A1에서 12/12 빌드, 12/12 mutation 탐지, 빈 cache는 pass 0 | REQ-HE-08, REQ-HE-09, S10 |
| vacuous 세션의 `ok` | `vacuous` verdict가 최우선이다 | REQ-HE-10, S10, S12 |
| F-003 (rev 4) 승인 지연과 `started_at` 하한 | 서명이 없어 해당하지 않는다. SPEC-HARNEVAL-003으로 옮겨 하한을 없애는 설계로 고쳤다 | SPEC-HARNEVAL-003 |
| A-F-017, A-F-001, F-004 | advisory lane에서는 수용한 잔여 위험(근거는 REQ-HE-11), SPEC-HARNEVAL-003에서는 must-resolve | REQ-HE-11 |
| rev 2~3의 서명·release·workflow·trust lane 항목(A-F-003, A-F-005, B-F-002, B-F-003, B-F-011, A-F-014, A-F-019, A-F-021) | 설계를 그대로 SPEC-HARNEVAL-003으로 옮겼다 | SPEC-HARNEVAL-003 |
| 결정적 lane 항목(A-F-002, A-F-004, A-F-006, A-F-007, A-F-011, A-F-012, A-F-016, A-F-018, A-F-020, B-F-004~B-F-006, B-F-009, B-F-012, B-F-014, B-F-015, B-F-017, F-001) | rev 3 처리를 유지했다(version pin, workspace revision, tombstone, 입력 집합, 출력 채널, wrapper 이름, 경로 정규화, macOS 전용, baseline `kind`, 오류 반환 추출, required check, per-revision driver, EARS 문법, S8 호출 수) | Requirements, Wire Contracts, S1-S8 |
