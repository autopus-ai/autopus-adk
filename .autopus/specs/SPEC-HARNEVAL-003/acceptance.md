# SPEC-HARNEVAL-003 수락 기준

## Test Scenarios

binding digest B는 test 안에서 만든 64자리 lowercase hex다. 합성 token과 test key는 test 실행 중에만 만들고 문서와 fixture 파일에는 넣지 않는다. GitHub API와 Sigstore 응답은 hermetic fixture로 주입한다.

### S1: test key round trip과 양방향 cross-lane 거부
Priority: Must
Given test key로 서명한 harness lane control pair(good, blocked)와 binding digest B가 있다
When `checkEvalRegressionStrict`를 harness lane policy와 주입한 trusted key로 실행한다
Then good은 `eval-regression: ok (version=B)`와 true, blocked는 `eval-regression: regression_blocked (version=B)`와 false다
And report 1바이트 변조는 `eval-regression: signature_invalid`, 없는 key는 `eval-regression: signature_key_unknown`, 73시간 뒤는 `eval-regression: artifact_stale (version=B)`이다
And harness 증거를 Autopus policy(`staging-to-main`, `staging`, `production`, key `autopus-eval-staging-to-main-2026-07`)로, Autopus lane test 증거를 harness policy로 검증하면 둘 다 `eval-regression: attestation_policy_mismatch`이고 `produced_at` 1초 차이도 같다

### S2: 개인키는 stdin으로만 받고 출력·자식 환경 어디로도 새지 않는다
Priority: Must
Given export step과 같은 부모 환경이 있다. `HARNESS_EVAL_SIGNING_KEY`에는 64-byte test 개인키의 base64가 들어 있다. 검증을 통과한 live result와 run meta 파일도 있다
When 그 환경의 shell에서 `printf '%s' "$HARNESS_EVAL_SIGNING_KEY" | env -u HARNESS_EVAL_SIGNING_KEY auto eval harness export --input <results> --run-meta <meta> --output <new-dir>`을 실행한다
Then 새 디렉터리의 mode는 0700, report와 attestation의 mode는 0600이고 둘 다 strict decode에 성공한다
And stdout, stderr, 출력 디렉터리 전체에 개인키 64 bytes와 seed 32 bytes의 raw, std/raw/url base64, 소문자·대문자 hex, seed base64의 padding 없는 앞 40자가 없다. 서명은 stdin의 키로 성공한다. `auto` process 자신의 환경 기록(test seam)과, baseline driver 빌드를 포함한 자식 process 환경 기록에는 `HARNESS_EVAL_SIGNING_KEY`와 키 bytes가 없다
And `--output`이 이미 있으면 `output_exists`로 그 디렉터리가 바뀌지 않는다. report를 쓴 뒤 실패하도록 주입하면 디렉터리가 사라지고 종료 코드는 1이다. 빈 stdin은 `private_key_missing`, 잘못된 키는 `private_key_invalid`, key flag는 unknown flag 오류다

### S3: live workflow는 bind·eval·서명을 분리하고 주입 경로가 없다
Priority: Must
Given `[NEW] .github/workflows/harness-eval-live.yml`이 있다
When workflow static contract test를 실행한다
Then `on:` key 집합은 정확히 `{workflow_dispatch}`이고 `inputs`가 없으며, 모든 `run:` 본문에 `${{`가 0개이고, 모든 `uses:`는 40자리 hex SHA다(`actions/attest` 포함)
And `bind` permissions는 정확히 `{contents: read, id-token: write, attestations: write}`이고 environment와 `secrets.` 참조가 없다. `live-eval`은 `needs: bind`, `environment: adk-harness-eval-agent`, permissions `{contents: read, id-token: write, attestations: write}`, checkout `persist-credentials: false`를 갖는다. 이름이 `golden-session`인 step이 정확히 하나 있고, Codex credential은 그 step의 `env:`에만 있다. `sign`은 `needs: live-eval`, `environment: adk-harness-eval-signing`, permissions `{contents: read, actions: read, id-token: write, attestations: write}`를 갖는다. export step의 `env:` key 집합은 정확히 `{HARNESS_EVAL_SIGNING_KEY}`이고, run 본문은 `printf '%s' "$HARNESS_EVAL_SIGNING_KEY" | env -u HARNESS_EVAL_SIGNING_KEY auto eval harness export`로 시작한다. 읽는 변수와 지우는 변수가 다른 변형 fixture는 이 test가 실패로 잡는다
And 세 job 모두 `if: github.ref == 'refs/heads/main'`, `runs-on: macos-15`이고, `sign` step에는 `golden.py`, `python`, `codex`, `go test`가 0개이며, `TestEvalRegressionADKWorkflowIsRetired`는 수정 없이 통과한다

### S4: signer는 digest 대조를 먼저 하고, 그다음 protocol·run·log 시각·oracle 결과를 의미 검사한다
Priority: Must
Given baseline [pass,pass] candidate [pass,fail]인 task 하나(K=2)의 정상 결과, run meta, bound·session-result attestation fixture가 있다. 변형은 두 계열이다. 계열 A는 attest 뒤에 protocol, record, oracle 결과, `calibration.json` 중 하나를 1바이트 바꾸거나 `calibration.json`을 뺀다. `calibration.json` 변조는 `after.status`를 `failed`에서 `passed`로 바꾼 것이다. 계열 B는 처음부터 틀린 내용을 그대로 attest한다. B의 경우는 여섯 가지다. (B1) `policy.threshold_bp`가 −10000 (B2) `run_attempt`가 meta와 다름 (B3) `started_at`이 bound log 시각보다 이름 (B4) `agent_termination.timed_out`이 true이고 채점은 모두 통과인 trial을 record가 `pass`로 적음 (B5) record 중복 (B6) `after.status`는 `passed`인데 task 결과에 mutated 통과가 있는 `calibration.json`
When `auto eval harness export`를 각각 실행한다
Then 정상 결과는 delta -0.5, hard flip 0으로 `blocked` true, `reason` `pass_rate_regression`인 report를 내고, 그 `produced_at`은 `started_at`과 같은 문자열이다
And 계열 A는 다섯 경우 모두 의미 검사 전에 `attestation_digest_mismatch`로 끝나고 아무 파일도 쓰지 않는다
And 계열 B는 아무 파일도 쓰지 않고 순서대로 다음을 낸다. `protocol_mismatch`(detail `policy.threshold_bp`, `run_attempt`, `started_at`), `outcome_derivation_mismatch`, `records_protocol_mismatch`(detail `duplicate GT-AG-001/candidate/0`), `outcome_derivation_mismatch`(calibration)
And 정상 결과를 30시간 뒤에 서명해도 성공한다(승인 지연은 상한에 걸리지 않는다)

### S5: digest 사슬이 live에서 release까지 이어지고 평가 입력 변경은 증거를 무효로 만든다
Priority: Must
Given macOS CI job, fixture repo, agent stub, 64-byte test key, 다른 디렉터리에서 다른 version ldflags로 빌드한 `auto`가 있다
When golden 모드 → export → 별도 binary의 `auto eval harness digest --binding`과 `policy` → `checkEvalRegressionStrict`(test key 주입) 순서로 실행한다
Then protocol `binding_digest`와 별도 binary 출력이 같은 64자리 hex B이고, 검증 출력은 `eval-regression: ok (version=B)`이다
And 다음을 각각 바꾸면 binding이 B와 달라지고 기존 증거는 `eval-regression: attestation_policy_mismatch`가 된다. corpus 1바이트(+`file_sha256`), `threshold_bp`, `workspace_revision`, `baseline_ref`, baseline tag가 가리키는 커밋, template 한 줄, `golden.py`·`run.py`·`prepare_grader.py`·`grader.sb`·`pkg/harneval` 판정 코드·oracle harness 원본·sandbox profile 각 한 줄, agent task `expected_tests` 하나, `live.model`, black-box task 기대 출력 파일 1바이트(+`black_box_oracle`의 sha256), task `oracle_mode`
And 001 PR lane에서 같은 기대 출력 변경은 `expectation_changed`다
And 그 CI step은 `go test -list` 개수가 floor 이상인지 확인하고 PASS 집합이 목록과 같을 때만 성공한다

### S6: release는 같은 T_now의 window 안 세션을 두 출처에서 모두 세어 판정한다
Priority: Must
Given 고정한 `T_now`와 세션 fixture가 있다. `L = T_now−72h+15m`이다. bound log 시각은 각각 `T_now−73h`, `L−1s`, `L`, `T_now−71h`, `T_now−10m`이다. bound log 시각이 `L`인 세션의 attempt 시작은 `L−20m`이다. 각 세션의 attempt 결론과 서명 증거는 fixture가 정한다
When `auto eval harness release-check --binding B --now-file <f>`를 실행한다
Then `T_now−73h`와 `L−1s` 세션은 두 출처 모두에서 세지 않는다. `L` 세션은 두 출처 모두에서 센다. 그 세션은 attempt 시작이 `L`보다 이르지만 `session_log_inconsistent`가 아니다
And 판정은 다음과 같다. (ok, ok)와 (ok, 검증된 incomplete)는 진행이다. (incomplete만)은 `artifact_missing`이다. (ok, 승인 거부된 `failure`)와 (ok, `cancelled`)는 `run_not_ok`, (ok, `in_progress`)는 `run_in_progress`, (ok, `hard_flip` 증거)는 중단이다. report `reason`이 `incomplete`라고 적혔지만 서명이 깨진 증거는 출력이 `signature_invalid`라서 세지 않는 대상이 아니고 중단이다
And attempt 1이 bound 뒤 취소되고 attempt 2가 ok인 run은 `run_not_ok`(attempt 1)이다. bind 없이 live-eval만 다시 실행한 attempt(`attempt_unbound`, golden 세션 step 미시작)는 run key 기록이 없어 세지 않는다. 그 attempt는 B의 release도 다른 binding의 release도 막지 않는다
And run key 기록의 binding이 B가 아닌 run은 세지 않는다. run key 기록이 없는데 golden 세션 step이 시작된 attempt는 `session_log_inconsistent`다. 다른 binding의 run 때문에 거짓 차단이 생기지 않는다
And `total_count` 150인데 100개만 모이면 `run_list_truncated`, 조상이 아닌 `baseline_ref`는 `baseline_ref_invalid`, baseline tag를 다른 커밋으로 옮기면 binding이 달라져 `artifact_missing`이다
And release.yaml static test에서 새 step의 env는 `GH_TOKEN` 하나이고 `env -i`로 실행한다. 두 검사 step의 `timeout-minutes`는 15이고, `T_now` 파일은 같은 step 안에서 쓴다. `fixture/`·`testdata/`·`localhost`·`PREVIOUS…FILE` 형태가 없다

### S7: 세션은 run이나 attestation 한쪽이 지워져도 숨겨지지 않는다
Priority: Must
Given binding B의 attestation 목록 fixture와 run·attempt·job 목록 fixture가 있다
When release-check를 실행한다
Then 다음 세 경우는 모두 `session_log_inconsistent`로 멈춘다. run은 지워졌지만 B subject의 bound·session-result attestation이 남은 세션, bound attestation은 지워졌지만 golden 세션 step이 시작된 attempt, bound 기록 없이 session-result만 남은 세션이다
And run과 두 attestation을 모두 지운 fixture는 세지 못하고 그대로 통과한다. 이것이 REQ-HR-07이 보장하지 않는 범위다. A2가 FAIL이면 T7을 merge하지 않고 release gate를 켜지 않는다(REQ-HR-07 분기, CD-HR-1)
And workflow 신원이 `…/harness-eval-live.yml@refs/heads/main`이 아니거나 log 증명이 맞지 않는 bundle은 `attestation_identity_invalid`로 멈춘다. 검증된 timestamp가 없는 bundle은 `attestation_timestamp_missing`으로 멈춘다. attestation API나 Sigstore 조회 실패(HTTP 5xx fixture)는 `session_log_unavailable`로 멈춘다
And bind가 끝나기 전에 취소되어 bound attestation도 live-eval 시작도 없는 run은 세지 않는다

### S8: 서명 lane의 결과는 black-box oracle harness의 출력 판정으로만 정해진다
Priority: Must
Given black-box task fixture가 있다. artifact 쪽 변형은 정상 수정, mutation을 되돌리지 않은 수정, 빌드되지 않는 수정, 기대 출력 일부를 낸 뒤 멈추지 않는 수정, 가짜 성공 문구를 출력하거나 일찍 끝내는 수정, main checkout의 기대 출력 경로를 읽으려는 수정, 출력 항목을 링크나 일반 파일이 아닌 항목으로 만든 수정이다. trial 쪽 변형은 workspace setup 실패, agent timeout 뒤 채점이 모두 통과한 trial(agent는 signal로 끝나 `exit_code` null, `os_signal` `SIGKILL`), agent 비정상 종료 뒤 빌드 실패, artifact 뒤 남은 process를 확인하지 못한 trial, scope 위반이다. 그 밖에 결과 JSON을 쓰지 않도록 망가뜨린 oracle harness seam, white-box 전용 task, mutation을 적용한 reference artifact도 통과시키는 약한 oracle, 표에 없는 signal(`forbidden_construct`)을 적은 record가 있다
When trusted runner가 준비·빌드·실행·판정을 형제 process로 차례대로 실행하고, signer가 같은 records·oracle 결과 bytes로 다시 계산한다
Then 각 trial의 (outcome, signal, `oracle.ran`, `oracle.build_failed`)은 정확히 하나다. artifact 쪽 변형은 다음과 같다. 정상 수정은 (pass, `accepted`, true, false), 되돌리지 않은 수정은 (fail, `expectation_mismatch`, true, false), 빌드 실패는 (fail, `artifact_build_failed`, false, true), 일부를 낸 뒤 멈추지 않는 수정은 (fail, `artifact_timeout`, false, false), harness seam은 (fail, `oracle_harness_error`, false, false), 출력 항목을 링크나 일반 파일이 아닌 항목으로 만든 수정은 (fail, `output_link_rejected`, false, false)다
And trial 쪽 변형은 다음과 같다. setup 실패는 (error, `workspace_setup_failed`, false, false)이고 agent 호출이 0회다. agent timeout 뒤 채점 통과는 signal 종료 표기여도 (fail, `agent_timeout`, true, false)이고 `agent_launch_failed`가 아니다. agent 비정상 종료 뒤 빌드 실패는 (fail, `agent_exit_nonzero`, false, true)다. 남은 process를 확인하지 못한 trial은 (fail, `observation_failed`, false, false)이고 oracle 실행이 0회다. scope 위반은 (fail, `scope_violation`, false, false)이고 빌드와 oracle 실행이 0회다
And 가짜 성공 문구, 조기 종료, 빈 출력은 출력 비교를 바꾸지 못해 `expectation_mismatch`다. 기대 출력을 읽으려는 수정은 그 읽기가 `Operation not permitted`이고 `accepted`가 되지 않는다
And artifact process의 쓰기는 scratch 밖에서 거부된다. 그래서 oracle 결과 디렉터리, runner 디렉터리, fixture SHA-256이 그대로다. oracle harness는 artifact 파일을 쓰지 못한다
And 두 arm의 모든 trial이 `artifact_build_failed`인 세션, `artifact_timeout`인 세션, `output_link_rejected`인 세션은 모두 verdict가 `vacuous`다. 서명된 report는 `blocked` true, `reason` `vacuous`다
And 약한 oracle에서는 `calibration.json`의 `before.status`가 `failed`이고, agent 호출과 record가 0개이며, golden step 종료 코드는 1이다. 그 run은 release-check에서 `run_not_ok`다
And white-box 전용 task는 서명 lane 세션에 들어가지 않는다. black-box task 수가 `floors.signed_agent_tasks`보다 적으면 `vacuous`다. signer가 다시 계산한 값은 모든 trial에서 record와 같다. 표에 없는 signal을 적은 record는 signer가 `outcome_derivation_mismatch`로 거부한다

### S9: 기존 계약은 열거한 네 변경 외에는 수정 없이 유지된다
Priority: Must
Given 기존 test 파일이 있다
When `go test ./pkg/evalregression/... ./internal/companionmanifest/... ./pkg/companionmanifest/...`와 `go test ./internal/cli -run 'EvalRegression|EvalHarness'`를 실행한다
Then release.yaml을 읽는 test 23개(모든 job 제약 5개 포함), `TestEvalRegressionADKWorkflowIsRetired`, `TestEvalRegressionRequiredCheckRunbookExists`, `TestReleaseWorkflow_ExactA34ProtectedNormalLane`이 모두 PASS다
And 기존 test diff는 `spec.md` `## Existing Test Changes`의 네 항목뿐이다. T6이 `checkEvalRegression`과 `evaluateEvalRegression`을 test 전용 파일로 옮긴 뒤 `VerifyEvalRegressionArtifact`(v1)의 non-test 호출자는 0개다

### S10: release job은 게시 직전에 다시 검사한다
Priority: Must
Given `harness-eval-evidence` job이 `T_now1`에 통과한 뒤, `release` job이 승인 대기로 73시간 뒤에 시작하는 fixture와, 그 사이 같은 binding의 `hard_flip` 세션이 새로 생긴 fixture가 있다
When `release` job의 재검증 step이 `T_now2`를 다시 읽어 release-check를 실행한다
Then 첫 fixture는 window 안 `ok` 증거가 없어 `artifact_missing`이고, 두 번째는 중단이다. 두 경우 모두 게시 step이 실행되지 않는다
And release.yaml static test는 재검증 step이 게시 step보다 앞에 있음을 단언한다

### S11: hosted runner에서 sandbox와 실제 실행이 확인된다
Priority: Must
Given 정상 runner가 있다. preflight seam은 네 개다. 세 개는 sandbox가 쓰기, network, 기대 출력 읽기 중 하나를 거부하지 못하게 만든다. 나머지 하나는 oracle harness의 출력 열기 검사가 위협 유형 하나를 받아들이게 만든다
When live-eval이 trial 전 preflight를 실행한다
Then 네 seam 모두 `sandbox_preflight_failed`로 trial 0개다. 정상 runner에서는 grade 밖 쓰기, network, main checkout의 `evals/harness/**` 읽기, 그 파일로의 hard link 생성이 모두 `Operation not permitted`(EPERM)이고 trial이 시작된다
And 정상 runner의 출력 열기 자체 검사 결과는 정확히 다음과 같다. 일반 파일은 `ok`다. root 안 symlink, root 밖 symlink, 디렉터리 symlink, hard link(link 수 2), FIFO는 각각 `link_rejected`이고, 1 MiB를 넘는 파일은 `too_large`다. FIFO 검사는 막히지 않고 끝난다. `oracle.sb` 안의 harness가 main checkout의 기대 출력 파일을 직접 열면 `Operation not permitted`다
And T10 OPS 증거로 main의 hosted control run 하나가 남는다. 증거는 run URL, bound·session-result attestation, 서명 증거, release-check 출력, trial 소요 시간이며, 소요 시간은 `trial_timeout_seconds` 400 이하다

### S12: 서명 결과는 업로드 전에 자기 검증되고 키 회전 절차가 고정된다
Priority: Must
Given 상수 `ADKHarnessEvalKeyID`와 다른 key로 서명하도록 만든 sign 환경과 정상 환경이 있다
When sign job의 export와 자기 검증을 실행한다
Then 다른 key 환경은 `self_verify_failed`로 업로드하지 않고, 정상 환경은 `ok`나 `regression_blocked` 자기 검증 뒤 업로드한다
And runbook에는 키 회전 순서가 있다. 순서는 dispatch 중지, 실행 중이거나 승인 대기 중인 run 취소, 공개키 추가·상수 교체·secret 교체 동시 적용, dispatch 재개, 새 세션이다

## Oracle Acceptance Notes

- 모든 Must 시나리오는 concrete expected output(정확한 `eval-regression:` stdout 줄, reason과 detail literal, 파일 mode, 권한 집합, 시각 경계)을 가진다. 파일 존재, heading, 종료 코드만으로 닫는 Must 시나리오는 없다.
- S1의 출력 문자열은 2026-10-06 probe A1에서 실제로 확인했다.
- S7과 S8은 probe A2·A3(CD-HR-1, CD-HR-2)가 끝나야 통과할 수 있고, S11의 control run은 OPS(CD-HR-3)다. 통과 전에는 sync 완료가 아니다.
