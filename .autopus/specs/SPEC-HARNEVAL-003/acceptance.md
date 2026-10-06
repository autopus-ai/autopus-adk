# SPEC-HARNEVAL-003 수락 기준

## Test Scenarios

binding digest D는 test 안에서 만든 64자리 lowercase hex다. 합성 token과 test key는 test 실행 중에만 만들고 문서와 fixture 파일에는 넣지 않는다.

### S1: test key round trip과 양방향 cross-lane 거부
Priority: Must
Given test key로 서명한 harness lane control pair(good, blocked)와 binding digest D가 있다
When `checkEvalRegressionStrict`를 harness lane policy와 주입한 trusted key로 실행한다
Then good은 `eval-regression: ok (version=D)`와 true, blocked는 `eval-regression: regression_blocked (version=D)`와 false다
And report 1바이트 변조는 `eval-regression: signature_invalid`, 없는 key는 `eval-regression: signature_key_unknown`, 73시간 뒤는 `eval-regression: artifact_stale (version=D)`이다
And harness 증거를 Autopus policy(`staging-to-main`, `staging`, `production`)로, Autopus lane test 증거를 harness policy로 검증하면 둘 다 `eval-regression: attestation_policy_mismatch`이고 `produced_at` 1초 차이도 같다

### S2: 개인키는 stdin으로만 받고 어떤 표현으로도 새지 않는다
Priority: Must
Given 64-byte test 개인키의 base64를 stdin으로 넘기고 검증을 통과한 live result가 있다
When `auto eval harness export --input <results> --output <dir>`을 실행한다
Then report와 attestation의 mode는 0600이고 둘 다 strict decode에 성공한다
And stdout, stderr, report, attestation, 출력 디렉터리 전체에 개인키 64 bytes와 seed 32 bytes의 raw bytes, std/raw/url base64, 소문자·대문자 hex, seed base64의 padding 없는 앞 40자가 한 번도 나오지 않는다
And 출력 report가 이미 있으면 `output_exists`로 기존 bytes를 유지하고, stdin이 비면 `private_key_missing`, key flag는 unknown flag 오류다

### S3: live workflow는 bind·eval·서명을 분리하고 주입 경로가 없다
Priority: Must
Given `[NEW] .github/workflows/harness-eval-live.yml`이 있다
When workflow static contract test를 실행한다
Then `on:` key 집합은 정확히 `{workflow_dispatch}`이고 `inputs`가 없으며 모든 `run:` 본문에 `${{`가 0개다
And `bind`는 environment와 `secrets.` 참조가 없고, `live-eval`은 `needs: bind`와 `environment: adk-harness-eval-agent`, `sign`은 `needs: live-eval`과 `environment: adk-harness-eval-signing`, `actions: read`를 갖는다. 세 job 모두 `if: github.ref == 'refs/heads/main'`, `runs-on: macos-15`이다
And 서명키 secret은 `sign`에서만, Codex credential은 `live-eval` golden step `env:`에서만 참조되고, `sign` step에는 `golden.py`, `python`, `codex`, `go test`가 0개이며 `TestEvalRegressionADKWorkflowIsRetired`는 수정 없이 통과한다

### S4: signer는 protocol을 trusted 값과, 자기 run·attempt와, record를 order와 대조한다
Priority: Must
Given baseline [pass,pass] candidate [pass,fail]인 task 하나(K=2)의 정상 결과와, 그 protocol의 `policy.threshold_bp`만 −10000으로 바꾼 결과, `order`와 record에서 같은 task를 함께 뺀 결과, `run_attempt`를 1에서 2로 바꾼 결과, `started_at`을 run `created_at`보다 이르게 바꾼 결과, record 하나를 복제한 결과가 있다
When `auto eval harness export`를 각각 실행한다
Then 정상 결과는 delta -0.5, hard flip 0으로 `blocked` true, `reason` `pass_rate_regression`인 report를 낸다
And 변형 결과는 아무 파일도 쓰지 않고 순서대로 `protocol_mismatch`(detail `policy.threshold_bp`, `order`, `run_attempt`, `started_at`)와 `records_protocol_mismatch`(detail `duplicate GT-AG-001/candidate/0`)를 낸다
And `started_at`이 run 생성 10분 뒤이고 signer 시각이 그보다 30시간 늦은 정상 결과(승인 지연)도 서명되며, 그 report의 `produced_at`은 `started_at`과 같은 문자열이다

### S5: digest 사슬이 live에서 release까지 이어지고 평가 입력 변경은 증거를 무효로 만든다
Priority: Must
Given fixture repo, agent stub, 64-byte test key, 다른 디렉터리에서 다른 version ldflags로 빌드한 `auto`가 있다
When golden 모드 → export → 별도 binary의 `auto eval harness digest --binding`과 `auto eval harness policy` → `checkEvalRegressionStrict`(test key 주입) 순서로 실행한다
Then protocol `binding_digest`와 별도 binary 출력이 같은 64자리 hex B이고 검증 출력은 `eval-regression: ok (version=B)`이다
And corpus 1바이트(+`file_sha256`), `threshold_bp`, `workspace_revision`, `baseline_ref`, template 한 줄, `golden.py` 한 줄, `grader.sb` 한 줄, agent task `expected_tests` 하나를 각각 바꾸면 binding이 B와 달라지고 기존 증거는 `eval-regression: attestation_policy_mismatch`가 된다

### S6: release는 binding의 72시간 attempt를 결론과 무관하게 모두 본다
Priority: Must
Given 수정된 `release.yaml`과, attempt마다 status·conclusion·bind 기록·서명 증거를 정한 run 목록 fixture가 있다
When workflow static contract test와 선택 단계 hermetic test를 실행한다
Then `jobs.release.needs` 집합은 `ci`, `security`, `omp-production-evidence`, `harness-eval-evidence`이고, 그 job은 `macos-15`, `actions: read`, `gh api --paginate`의 runs URL(`branch=main&event=workflow_dispatch&created=>=…&per_page=100`), attempts URL, 여섯 `--eval-regression-expected-*` flag, `--eval-regression-max-age 72h`를 쓰며 `--warn-only`가 0개다
And binding B의 판정은 (ok, ok) 진행, (ok, incomplete) 진행, (ok, 승인 거부된 `failure`) `run_not_ok`, (ok, `cancelled`) `run_not_ok`, (ok, `in_progress`) `run_in_progress`, (ok, regression_blocked) 중단, (incomplete만) `artifact_missing`, 빈 목록 `eval-regression: artifact_missing`이다
And attempt 1이 bind 뒤 취소되고 attempt 2가 ok인 run은 `run_not_ok`(attempt 1)이고, `total_count` 150인데 100개만 모인 목록은 `run_list_truncated`, 조상이 아닌 `baseline_ref`는 `baseline_ref_invalid`다

### S7: 삭제된 run과 artifact도 세션 집계에서 빠지지 않는다
Priority: Must
Given binding B의 세션 두 개가 append-only 세션 기록에 있고, 그중 하나는 run과 artifact가 모두 삭제된 fixture가 있다
When release 선택 단계를 실행한다
Then 삭제된 세션은 기록에서 세어져 `run_not_ok`로 release가 멈추고, 기록을 읽을 수 없는 fixture는 `session_log_unavailable`로 멈춘다
And 기록에 없는 run(bind 전에 취소된 run)은 세지 않는다

### S8: agent diff의 위조 경로는 alias와 무관하게 막히고 framing 모순은 fail이다
Priority: Must
Given 허용 파일에 `import s "syscall"`과 `s.Exit(0)`, `import o "os"`와 `o.Exit(0)`, dot import `. "os"`와 `Exit(0)`, `var _ = f()` 초기화, `//go:linkname`, 새 `import "reflect"`를 각각 넣은 diff와, corpus 12개 task의 정상 수정(mutation 되돌리기) diff가 있다
When grader 전 AST gate와 parser를 실행한다
Then 위조 diff 여섯 개는 각각 `forbidden_construct`와 파일·줄 위치로 fail이고, 정상 수정 diff 12개에서는 `forbidden_construct`가 0건이다
And 한 expected test에 pass와 fail 이벤트가 함께 있는 출력과, pass 이벤트가 모두 있지만 종료 코드가 1인 출력은 둘 다 `oracle_output_invalid` 또는 `oracle_failed`로 `fail`이다

### S9: 기존 계약은 열거한 세 단언 외에는 수정 없이 유지된다
Priority: Must
Given 기존 test 파일이 있다
When `go test ./pkg/evalregression/... ./internal/companionmanifest/... ./pkg/companionmanifest/...`와 `go test ./internal/cli -run 'EvalRegression|EvalHarness'`를 실행한다
Then 모든 test가 PASS이고 `TestEvalRegressionADKWorkflowIsRetired`, `TestEvalRegressionRequiredCheckRunbookExists`, `TestReleaseWorkflow_ExactA34ProtectedNormalLane`이 포함된다
And 기존 test diff는 `spec.md` `## Existing Test Changes`의 세 단언(`verify_e2e_test.go` L120·L138, `release_contract_test.go` L15)뿐이다

## Oracle Acceptance Notes

- 모든 Must 시나리오는 concrete expected output(정확한 `eval-regression:` stdout 줄, reason과 detail literal, 파일 mode, SHA-256 동일성, 거부 위치)이나 explicit tolerance를 가진다. 파일 존재, heading, 종료 코드만으로 닫는 Must 시나리오는 없다.
- S1의 출력 문자열은 2026-10-06 probe A1에서 실제로 확인했다.
- S7과 S8은 must-resolve 설계(CD-HR-1~CD-HR-3)가 정해진 뒤에야 통과할 수 있다. 통과 전에는 sync 완료가 아니다.
