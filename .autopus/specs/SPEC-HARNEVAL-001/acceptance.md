# SPEC-HARNEVAL-001 수락 기준

## Test Scenarios

fixture task id는 `GT-FIX-A` ~ `GT-FIX-F`, `GT-AG-001` ~ `GT-AG-005`처럼 test 안에서 정한다. 수치 oracle의 허용 오차는 1e-9다.

### S1: strict decode가 잘못된 task와 manifest를 거부한다
Priority: Must
Given unknown field, trailing data, assertion 0개, 미지의 kind, `platform` 없는 assertion, 중복 id, `threshold_bp` 5, active path `evals/harness/tasks/../candidates`, symlink task 파일, `expected_tests` 없는 agent task fixture가 각각 있다
When `auto eval harness run --format json`을 fixture마다 실행한다
Then 각 `failure_reasons`는 정확히 `["invalid"]`이고 detail은 순서대로 `unknown_field`, `trailing_data`, `no_assertions`, `unknown_assertion_kind`, `assertion_field_invalid`, `duplicate_task_id`, `policy_out_of_range`, `unclean_path`, `symlink_not_allowed`, `expected_tests_missing`이다
And baseline 파일이 없는 fixture는 `["baseline_missing"]`이고, 정상 fixture는 `totals.declared_surface` 3이다

### S2: 결정적 생성은 build·host와 반복에 무관하다
Priority: Must
Given 서로 다른 디렉터리에서 `-X …/pkg/version.version=v0.50.123`과 `=dev`로 따로 빌드한 `auto` 두 개와, 서로 다른 `HOME`, `TMPDIR`, `TZ`가 있다
When 두 binary로 같은 tree에서 `auto eval harness run --format json`을 실행한다
Then 두 결과 JSON은 `produced_at`을 지우면 byte-identical이고 `surface_digest`는 같은 64자리 lowercase hex이며 sentinel 로그는 비어 있다
And `.autopus/txns/**`와 `.autopus/<platform>-manifest.json`은 digest 입력에 없다
And codex pin을 제거한 fixture는 sentinel 로그에 `codex debug models`를 남기고 `["host_probe_unpinned"]`와 detail `codex`를 낸다

### S3: baseline 비교가 전이와 pass rate를 계산한다
Priority: Must
Given baseline이 A pass, B pass, C fail, E pass, F pass이고 현재 set은 A pass, B fail, C pass, D(신규) pass, E retired tombstone, F 파일 삭제(tombstone 없음)이다
When `auto eval harness run --format json`을 실행한다
Then `pass_rate` 0.75, `baseline_pass_rate` 0.8(삭제된 F는 baseline 행의 `kind` `surface`로 센다), `regression_delta` -0.05이다
And `transitions`는 순서대로 `GT-FIX-B regression`, `GT-FIX-C improved`, `GT-FIX-D new`, `GT-FIX-E retired`, `GT-FIX-F task_missing`이고 `failure_reasons`는 `["regression","set_digest_mismatch","task_missing"]`, 종료 코드 1이다
And baseline A pass, C fail, set 불변, 현재 A pass, C pass인 fixture는 `status` `pass`, 종료 코드 0, `regression_delta` 0.5이고 stdout은 JSON 하나로 decode되며 `baseline --update` 안내는 stderr에만 있다

### S4: 기대값 digest는 assertion과 agent 기대 행동 변경만 드러낸다
Priority: Must
Given baseline과 일치하는 set에서 `GT-FIX-A` needle만 바꾼 tree, `outcome` 문구만 바꾼 tree, corpus 파일 1바이트와 `file_sha256`을 함께 바꾼 tree, agent task의 `expected_tests`에서 이름 하나를 뺀 tree가 있다
When 각 tree에서 `auto eval harness run --format json`을 실행한다
Then needle tree는 `GT-FIX-A expectation_changed`와 `["expectation_changed","set_digest_mismatch"]`를 내고, 문구 tree는 `status` `pass`이며 `set_digest`가 변경 전과 같은 64자리 hex다
And corpus tree와 `expected_tests` tree는 해당 agent task의 `expectation_changed`를 내고, 그 baseline 행은 `kind` `agent`, `result` `not_run`이다

### S5: vacuity와 stale templates는 통과로 보고되지 않는다
Priority: Must
Given floor 20에 active surface 19개인 set, task 하나를 건너뛰게 한 evaluator seam, `content/`만 바꾼 tree, 읽을 수 없는 committed `templates/` 하위 디렉터리가 있는 tree가 있다
When 각각 `auto eval harness run --format json`을 실행한다
Then 앞의 둘은 `["vacuous"]`이고 detail은 `active_surface_tasks=19 floor=20`, `executed=19 declared=20 missing=[<id>]`이다
And `content/` tree는 정렬된 stale 경로 목록과 빈 `transitions`의 `["templates_stale"]`, 읽기 실패 tree는 detail `regen_failed`인 `["templates_stale"]`이다
And 기존 `doctor_drift_source_test.go`는 수정 없이 통과한다

### S6: 적용 여부는 파생 입력 집합과 segment 경계로 정한다
Priority: Must
Given 주입한 closure `pkg/adapter/codex`, `pkg/content`, `pkg/harneval`과 고정 glob 집합이 있다
When `auto eval harness applicable --event pull_request --changed-files <file>`을 실행한다
Then `README.md`만 바뀌면 `not_applicable`과 종료 코드 0, `pkg/adapter/codex/codex.go`가 있으면 `applicable`과 matched `["pkg/adapter/codex/codex.go"]`, `pkg/adapter/codexfoo/x.go`만 바뀌면 `not_applicable`이다
And `scripts/benchmarks/harness/corpus_a.json`만 바뀐 목록과, `--no-renames` diff가 낸 `content/a.md`·`docs/a.md` 목록은 `applicable`이다
And closure 계산이 실패하면 `applicable`과 reason `closure_unavailable`, `--event push`는 reason `non_pull_request_event`이다
And `ci.yaml` static test에서 `on:` 아래 `paths`/`paths-ignore`가 0개이고 `harness-eval` job에 `if:`가 없다

### S7: baseline 갱신은 수락된 회귀와 tombstone만 기록한다
Priority: Must
Given baseline에서 pass였던 `GT-FIX-B`가 현재 fail이고 별도 fixture에서 `GT-FIX-F` 파일이 tombstone 없이 삭제되었다
When `auto eval harness baseline --update`를 수락 flag 없이, `--accept-regression GT-FIX-B`만으로, `--reason "hook intentionally removed"`를 더해서, 그리고 F fixture에서 실행한다
Then 앞의 둘은 종료 코드 1과 `regression_not_accepted: GT-FIX-B`, `accept_reason_required: GT-FIX-B`를 내고 baseline SHA-256이 같다
And 세 번째 뒤 `GT-FIX-B` 행은 `{"id":"GT-FIX-B","kind":"surface","state":"active","result":"fail","accepted_regression_reason":"hook intentionally removed"}` 필드를 갖고 행은 id 오름차순이다
And F fixture는 `tombstone_required: GT-FIX-F`로 실패하고, retired tombstone을 둔 뒤 갱신하면 F 행이 `state` `retired`로 남아 tombstone 파일을 지운 다음 run에도 `task_missing`이 없다

### S8: live protocol은 입력과 순서를 동결하고 재시도하지 않는다
Priority: Must
Given agent task `GT-AG-001`, `GT-AG-002`, K=2, 세 번째 예정 시도에서 `warmup_failed`를 주입하는 harness seam, agent stub, macOS host가 있다
When golden 모드 live runner를 실행한다
Then protocol `order`는 정확히 `(001,baseline,0) (001,candidate,0) (002,candidate,0) (002,baseline,0) (001,candidate,1) (001,baseline,1) (002,baseline,1) (002,candidate,1)`이고 record는 8개다
And 세 번째 record는 `outcome` `error`, `signal` `warmup_failed`이고 agent stub 호출은 7회(그 trial은 agent를 부르지 않고 재시도도 없음)다
And protocol의 `workspace_revision`, `baseline_ref`, `model`, `runner_sha256`, `grader_profile_sha256`은 manifest와 checkout 파일 값과 같고 두 arm workspace snapshot의 tree hash가 같다
And 시작 전 거부는 agent 호출 0회로 각각 `protocol_exists`, `run_cap_exceeded`(`max_agent_runs` 6), `workspace_mutation_mismatch`, `codex_cli_version_mismatch`, `unsupported_os`(Linux), `baseline_ref_unsupported`(driver API 없는 ref)를 낸다

### S9: advisory verdict 공식과 우선순위
Priority: Must
Given K=2인 세 task record: T1 baseline [pass,pass] candidate [fail,fail], T2 baseline [pass,fail] candidate [pass,pass], T3 baseline [pass,pass] candidate [pass,error]가 있다
When verdict를 계산한다
Then baseline 0.8333333333, candidate 0.6, `regression_delta` -0.2333333333, completeness 0.9166666667, hard flip 1, `verdict` `regression`, `reason` `hard_flip`이다
And T1 candidate가 [pass,fail]이면 delta -0.0333333333, `verdict` `ok`(`within_threshold`), 거기서 T2 candidate가 [error,error]이면 completeness 0.75, `verdict` `incomplete`다
And baseline 6/6, candidate T1 [pass,fail] T2 [fail,pass] T3 [fail,pass]이면 delta -0.5, hard flip 0, `reason` `pass_rate_regression`이고, 5 task baseline 10/10 candidate 9/10이면 delta -0.1로 `verdict` `ok`다
And 두 arm이 모두 error뿐이면 `verdict` `vacuous`, `reason` `oracle_not_run`, 두 arm의 `valid` 0과 `pass_rate` `null`, `regression_delta` 0이고 report가 strict decode에 성공한다

### S10: grader는 corpus oracle을 실제로 빌드하고, calibration 없는 세션은 ok가 될 수 없다
Priority: Must
Given trusted 준비가 만든 읽기 전용 module cache와 warm build cache, corpus 12개 agent task, 빈 module cache fixture, 변형 전에 실패하도록 깨뜨린 oracle fixture, oracle에 걸리지 않는 mutation fixture가 있다
When golden 모드 세션을 시작한다
Then 정상 준비에서는 12개 task 모두 변형 전 accept, 변형 후 비accept로 `calibration.status` `passed`이고 trial이 시작된다
And 세 fixture는 agent 호출 0회로 `oracle_calibration_failed`(detail에 task id)와 종료 코드 1이고, 세션 디렉터리에는 예정 order의 protocol과 `before.status` `failed`인 `calibration.json`만 있으며 record는 없다. 그 디렉터리의 `auto eval harness report`는 `verdict` `vacuous`, `reason` `oracle_calibration_failed`다
And 모든 trial이 빌드 실패(`oracle.ran` false, `build_failed` true)로 끝난 세션(`reason` `oracle_not_run`)과 세션 끝 재calibration(`after`)이 실패한 세션(`reason` `oracle_calibration_failed`)도 `verdict` `vacuous`다
And `calibration.json`이 없는 세션과 `before`는 `passed`인데 `after`가 없는 세션의 report는 `calibration.status` `missing`, `verdict` `vacuous`, `reason` `oracle_calibration_failed`다
And oracle이 모든 trial에서 돈 세션에서 두 arm의 valid trial이 모두 agent 단계 신호(`agent_launch_failed`, `agent_exit_nonzero`, `agent_timeout`, `observation_failed`)로 끝나면 completeness가 floor보다 낮아도 `verdict` `vacuous`, `reason` `agent_all_failed`다. 나머지 fail 신호 5종으로만 채운 세션은 `ok`(`within_threshold`)이고, candidate만 `agent_launch_failed`이고 baseline이 4/4 pass인 세션은 `regression`(`hard_flip`)이다
And grader의 `GOMODCACHE` 쓰기는 거부되어 cache 파일 목록 hash가 그대로이고, 한 trial이 build cache에 쓴 파일은 다음 trial의 grade에 없다

### S11: trial outcome은 신호 표대로 정해지고 agent와 grader는 격리된다
Priority: Must
Given 신호 13종을 하나씩 내는 stub, codex 시작을 막는 candidate 표면, oracle 실행 중 records·protocol·다른 trial·runner checkout 쓰기, network 연결, 환경 기록, background process를 시도하는 agent 작성 코드 fixture가 있다
When golden 모드 trial을 실행한다
Then `workspace_setup_failed`, `mutation_failed`, `warmup_failed`만 `error`, `accepted`만 `pass`, 나머지 9종은 `fail`이고 codex 시작 실패 표면은 `agent_launch_failed`로 `fail`이다
And grader의 쓰기 시도는 모두 거부되어 대상 파일 SHA-256이 그대로이고, network 연결은 `operation not permitted`이며, 기록된 grader 환경에는 credential canary, `GITHUB_*`, `ACTIONS_*`, `RUNNER_*`가 없다
And 허용 파일에 리터럴 `func init(`이나 `os.Exit`을 넣은 diff는 `forbidden_construct`, 1 MiB를 넘거나 `expected_tests`의 pass 이벤트가 없는 grader 출력은 `oracle_output_invalid`로 `fail`이고, trial 뒤 process group에 남은 process는 0개다

### S12: live report는 advisory이고 어떤 gate에도 들어가지 않는다
Priority: Must
Given S9 첫 record 집합으로 끝난 세션 디렉터리가 있다
When `auto eval harness report --input <session> --format json`을 실행하고, 그 출력을 여섯 expected flag를 채운 `auto check --eval-regression --eval-regression-artifact <file>`에 넣는다
Then report는 `advisory` true, `verdict` `regression`, `reason` `hard_flip`, `calibration.status` `passed`, 두 `surface_digest`, `runner_sha256`, `grader_profile_sha256`을 갖고 서명 필드가 없으며 종료 코드는 0이다
And `auto check`는 `eval-regression: artifact_unsigned`로 거부한다
And `ci.yaml`과 `release.yaml`에 `golden`과 `harness_live_advisory` 문자열이 0개이고 `.github/workflows/`에 live lane workflow가 없으며 `pkg/evalregression`과 allowlist의 diff는 0건이다

### S13: seeded mutation은 모두 탐지된다
Priority: Must
Given mutation M1~M5와 각 mutation이 regress시켜야 할 task id 집합을 담은 committed mutation table이 있다
When `pkg/harneval` mutation self-test를 실행한다
Then 각 mutation의 `regression` task 집합은 table 값과 정확히 같고 비어 있지 않으며, 변형하지 않은 표면은 `status` `pass`, 집합 공집합이다

### S14: 초기 golden set이 범위 floor를 만족한다
Priority: Must
Given committed `evals/harness/` set이 있다
When coverage test를 실행한다
Then active surface ≥ 20, assertion platform 집합 = 5종, category ≥ 4, multi 비율 ≥ 0.60, active agent ≥ 12이고, 각 agent task의 `corpus_ref.file_sha256`이 corpus 파일 raw bytes의 SHA-256과 같으며 `expected_tests`가 비어 있지 않다
And surface 5개 중 multi 3개 fixture는 0.6으로 통과, 2개 fixture는 `multi_ratio=0.40 floor=0.60`으로 실패하고, corpus 1바이트 변조는 detail `corpus_digest_mismatch`인 `invalid`다

### S15: 기존 계약은 수정 없이 유지된다
Priority: Must
Given 기존 test 파일과 benchmark corpus·report·export 파일이 있다
When `scripts/benchmarks/harness` Python unit test, `go test ./pkg/experiment/... ./pkg/evalregression/... ./internal/companionmanifest/... ./pkg/companionmanifest/...`, `go test ./internal/cli -run 'EvalRegression|TelemetryHarness|Doctor'`를 실행한다
Then 모든 test가 PASS이고 `TestEvalRegressionADKWorkflowIsRetired`, `TestReleaseWorkflow_ExactA34ProtectedNormalLane`, `TestCommittedAllowlistContainsPromotionKeyAndIsDefensiveForE2E`가 포함된다
And 기존 test 파일과 corpus·report·export 파일의 `git diff --exit-code`는 0건이고 CI total coverage는 85% 이상이다

### S16: CI summary는 category 표를 body 없이 쓴다
Priority: Should
Given routing 2/2, hooks_settings 1/2인 fixture result가 있다
When summary를 렌더링한다
Then 행은 `hooks_settings | 1 | 2 | 0.50`, `routing | 2 | 2 | 1.00` 순서이고 task `intent`·`outcome` 문구는 없다

## Oracle Acceptance Notes

- 모든 Must 시나리오는 concrete expected output(정확한 reason literal, 정렬된 배열, stdout 한 줄, SHA-256 동일성, 종료 코드와 그 의미) 또는 explicit tolerance(수치 1e-9)를 가진다. 파일 존재, heading, 종료 코드만으로 닫는 Must 시나리오는 없다.
- S3/S9의 수치는 heterogeneous 입력(pass/fail/error/신규/retired/삭제가 섞인 task)으로 계산한 expected value다.
- S10의 정상 경로는 probe A1에서 이 macOS host의 실제 corpus 12개로 확인했다. 빌드는 12/12 exit 0, mutation 탐지는 12/12, 빈 module cache는 pass 이벤트 0이었다.
