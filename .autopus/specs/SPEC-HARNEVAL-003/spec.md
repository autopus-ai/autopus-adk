# SPEC-HARNEVAL-003: 서명된 live 증거와 release 차단

**Status**: approved
**Created**: 2026-10-06
**Domain**: HARNEVAL
**Module**: autopus-adk
**Primary**: SPEC-HARNEVAL-001 rev 4 (golden set, live lane A/B, 격리 grader, calibration, advisory verdict를 소유)
**Sibling 사유**: 보안·컴플라이언스 경계(서명키, protected Environment, release 차단). HARNEVAL의 두 번째이자 마지막 sibling이다.

## 목적

SPEC-HARNEVAL-001의 live lane은 서명하지 않는 advisory report만 낸다. 이 SPEC은 같은 세션을 GitHub Actions의 격리된 job에서 실행하고, 공개 transparency log에 남는 attestation으로 세션 하나하나를 기록한다. 보호 Environment의 키로 `eval_regression_report.v1` + `eval_regression_attestation.v2`에 서명하고, release workflow는 이를 기존 strict 경로(`auto check --eval-regression`)로 다시 검증한다. 회귀나 숨겨진 세션이 있으면 release를 막는다.

## Outcome Boundary

- Outcome Lock 원문은 `research.md`의 `## Outcome Lock`이다. 요약:
  - (b) live lane이 권한 없는 eval job에서 001의 A/B 세션을 실행한다. 서명 lane은 black-box oracle을 가진 agent task만 쓴다. 결과는 main에서 빌드한 trusted oracle harness가 agent가 고친 artifact를 별도 process에서 실행해 출력으로만 판정한다. 보호 Environment의 signer job이 그 결과를 데이터로만 다시 도출해 서명하고, 기존 strict verifier가 수정 없이 검증한다.
  - (c) release는 release source의 binding digest에 해당하는 세션 중 두 출처 어느 한쪽에라도 기록된 세션을 모두 센다. 두 출처는 attestation 기록(Sigstore 공개 log에 남음)과 Actions run·attempt 목록이고, 기준은 한 번 읽은 `T_now`의 window다. 하나라도 검증된 `ok`/`incomplete`가 아니거나, 두 출처가 어긋나거나, 검증된 `ok`가 없으면 release를 멈춘다. 게시 직전에 `release` job 안에서 같은 검사를 다시 한다. 한 행위자가 두 출처를 모두 지우는 경우는 잔여 위험이며 CD-HR-1과 probe A2가 그 범위를 정한다.
- Mandatory requirements: REQ-HR-01 ~ REQ-HR-10 (Must).
- Explicit non-goals: Autopus/backend producer와 Autopus gate workflow 변경, unsigned-accept 경로나 verifier 의미 완화, PR마다 live LLM 실행, 운영자 로컬 서명, 001 결정적 lane의 의미 변경, branch protection·Environment·secret 설정의 자동 수행, live 결과로 통계적 유의성 주장, corpus oracle의 black-box 재작성.
- Completion evidence: `acceptance.md`의 Must 시나리오 S1-S12 전부, T10 OPS 증거(hosted control run 포함), Completion Debt CD-HR-1~CD-HR-5 해소. 그 전에는 sync 완료가 아니다.

## Trust Model

- 신뢰: protected branch `main`의 코드와 workflow 정의, 커밋된 manifest·corpus·grader profile·공개키 allowlist, repo admin(Environment와 branch protection을 관리하므로 이미 신뢰 경계 안이다), Sigstore 공개 log의 entry와 bundle의 검증된 timestamp, GitHub OIDC가 발급한 workflow 신원. 검증된 timestamp는 Rekor v1 entry이면 서명된 integrated time이고, Rekor v2 entry이면 RFC 3161 TSA timestamp다. v2 log는 더 이상 서명된 timestamp를 돌려주지 않는다.
- 비신뢰 실행: agent, 그리고 agent가 고친 코드로 빌드한 artifact. 같은 process 안의 코드는 그 process의 출력과 종료를 마음대로 할 수 있다고 가정한다. 그래서 서명 lane은 결과를 그 process 안에서 증명하지 않는다. trusted oracle harness는 artifact와 address space를 공유하지 않는다.
- 비신뢰 데이터: eval job artifact(protocol, record, oracle 결과, calibration 결과), artifact가 쓴 출력 디렉터리(파일 내용뿐 아니라 링크·특수 파일 같은 구조 포함), workflow run 목록과 artifact, GitHub attestation API 응답(서명과 log 증명을 검증하기 전까지).
- 비신뢰 행위자: write 권한 사용자. run·attempt 취소, 재실행, run·artifact 삭제를 할 수 있다. attestation 삭제에 어떤 권한이 필요한지는 probe A2로 확인한다.
- 비밀: Codex credential은 `adk-harness-eval-agent` Environment의 golden step 하나에, 그 안에서도 codex 프로세스 환경에만 둔다. 서명 개인키는 `adk-harness-eval-signing` Environment의 export step 하나에만 둔다. OIDC 토큰(`ACTIONS_ID_TOKEN_REQUEST_*`)은 agent와 grader 환경에 들어가지 않는다(001의 허용 목록 환경이 `ACTIONS_*`를 뺀다). 한 job이 Codex credential과 서명키를 함께 받지 않는다.

## Cross-SPEC Changes (SPEC-HARNEVAL-001)

이 SPEC은 001이 소유한 파일과 계약을 바꾼다. 001 T1-T13이 먼저 merge되어야 하고, 아래 변경은 이 SPEC의 task가 맡는다.

| 001 대상 | 변경 | 이 SPEC의 task |
|----------|------|----------------|
| REQ-HE-11 문장과 S12의 마지막 And(".github/workflows/에 live lane workflow 없음", "pkg/evalregression과 allowlist diff 0건") | 001 advisory lane에 한정한다. 003이 들어오면 S12 단언은 "001 golden 명령은 서명하지 않는다"로 좁히고, workflow·allowlist 단언은 003 S3·S9가 맡는다. 001 문서 개정이 필요하다(CD-HR-5) | T12 |
| `internal/cli/eval_harness_workflow_test.go`(001 T9)의 S12 단언 | 위와 같이 바꾼다. Existing Test Changes 4번째 항목이다 | T12 |
| `scripts/benchmarks/harness/golden.py`(001 T12) | protocol 필드 다섯 개를 쓴다(signed lane에서는 필수, maintainer host에서는 없음). black-box task는 artifact를 빌드하고 oracle harness를 별도 process로 실행해 그 결과 JSON을 record에 연결한다 | T13 |
| `harness_golden_set.v1` manifest(001 T1) | strict schema의 `floors`에 `signed_agent_tasks`를 더한다(초기 5) | T15 |
| 001 `expectation_digest` 공식(REQ-HE-03) | `{kind, variants, assertions, corpus_ref, expected_tests, oracle_mode, black_box_oracle}`로 넓힌다. `black_box_oracle`은 명령, 입력 fixture와 기대 출력 파일의 `{path, sha256}` 목록을 담는다. 그래서 기대값을 바꾸면 `expectation_changed`가 되고 binding도 바뀐다 | T15 |
| 001 record `signal` 닫힌 목록과 `oracle` 필드(REQ-HE-08) | black-box signal 여섯 개(`artifact_build_failed`, `oracle_harness_error`, `artifact_timeout`, `output_link_rejected`, `output_too_large`, `expectation_mismatch`)를 더한다. black-box trial에는 리터럴 검사와 white-box 채점 signal을 쓰지 않는다. black-box record의 `oracle{ran, build_failed, expected_passed, expected_failed}` 의미를 REQ-HR-08대로 정한다 | T13, T15 |
| 001 `calibration.json`(REQ-HE-09) | black-box task는 같은 파일의 `before`·`after`에 artifact 기준 결과를 쓴다. 거부 규칙은 001과 같다. 서명 lane에서는 그 bytes의 SHA-256을 session-result attestation에 넣는다 | T13 |
| `pkg/harneval/records.go`와 protocol decoder(001 T10) | protocol 필드 다섯 개(`run_id`, `run_attempt`, `binding_digest`, `baseline_commit`, `runner_tree_digest`)를 optional로 decode한다. signer는 다섯 개 모두 있어야 받는다 | T2 |
| 001 Wire Contracts의 protocol·record·task 행 | protocol에 위 다섯 필드를 더한다. record에는 `oracle_result_sha256`, `stage_reached`, `agent_termination`을 더한다. agent task에는 `oracle_mode: white_box\|black_box`와 `black_box_oracle`을 더한다(001 strict schema 개정, CD-HR-5) | T2, T13, T15 |
| golden.py의 workspace snapshot(001 T12) | `evals/harness/**`를 agent workspace에서 뺀다. agent가 oracle 입력과 기대 출력을 볼 수 없게 하기 위해서다 | T13 |

## Requirements

### REQ-HR-01 Live workflow 신뢰 경계
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL run the signed live lane only through `workflow_dispatch` on main in `[NEW] .github/workflows/harness-eval-live.yml` with a binding job that holds no stored secret, an eval job that holds only the Codex credential, and a separate signer job that holds only the signing key, declare no pull request trigger, accept no dispatch inputs, and never interpolate event, input, or artifact values into run scripts.

- job `bind`: `macos-15`, Environment·stored secret 없음, `permissions: {contents: read, id-token: write, attestations: write}`. binding digest를 계산하고, pinned `actions/attest@<40-hex>`로 `bound` attestation을 만든다(REQ-HR-07).
- job `live-eval`: `needs: bind`, `macos-15`, `environment: adk-harness-eval-agent`, `permissions: {contents: read, id-token: write, attestations: write}`, `persist-credentials: false`. 시작할 때 같은 `run_id`·`run_attempt`의 `bound` attestation이 있는지 확인하고, 없으면 `attempt_unbound`로 끝난다(job 단위 재실행 방지). 이어서 hosted sandbox preflight(REQ-HR-10) 뒤 001 golden 모드를 실행한다. Codex credential은 golden step의 `env:`에만 매핑한다. agent process가 모두 끝난 뒤 `session_result` attestation(protocol·record·oracle 결과·calibration 결과 digest)을 만든다.
- job `sign`: `needs: live-eval`, 새 `macos-15` runner, `environment: adk-harness-eval-signing`, `permissions: {contents: read, actions: read, id-token: write, attestations: write}`. 같은 `github.sha`를 checkout해 `auto`를 빌드하고 `auto eval harness export`만 실행한다. export step의 env 이름은 `HARNESS_EVAL_SIGNING_KEY` 하나다. 명령은 `printf '%s' "$HARNESS_EVAL_SIGNING_KEY" | env -u HARNESS_EVAL_SIGNING_KEY auto eval harness export …`다. shell이 읽는 변수와 `env -u`가 지우는 변수가 같으므로, 키는 stdin으로만 들어가고 `auto`와 그 자식 process의 환경에는 없다. 서명한 증거는 업로드 전에 스스로 검증한다(REQ-HR-05).
- 세 job 모두 `if: github.ref == 'refs/heads/main'`이고, `uses:`는 40-hex SHA로 고정한다. dispatch input은 없다. 동적 값은 `env:`나 파일로만 넘긴다.
- Environment 보호 규칙(OPS, T10): 두 Environment 모두 deployment branch를 `main`으로 제한한다. `adk-harness-eval-signing`은 required reviewer 1명 이상과 self-review 금지를 둔다.

### REQ-HR-02 Signer의 trusted protocol 재구성과 결과 재도출
Priority: Must · EARS: EventDriven

WHEN the signer job receives a live result artifact, THEN THE SYSTEM SHALL rebuild the trusted protocol from the main checkout, compare the artifact protocol with it field by field, check the protocol against the attested run, attempt, and log times, and re-derive every trial outcome from the attested records and oracle results before any verdict is computed.

- trusted 필드: `policy`, `pins`, `model`, `cli_version`, `workspace_revision`, `baseline_ref`, `baseline_commit`, `agent_set_digest`, `corpus_digests`, `runner_tree_digest`, `order`, 두 `surface_digest`, `binding_digest`. 하나라도 다르면 `protocol_mismatch`와 첫 필드 경로다.
- run 귀속:
  - 실행 정보는 `--run-meta <file>`(`gh api`로 받은 `{run_id, run_attempt, run_created_at, attempt_started_at}`)로 받는다. hermetic test는 이 파일을 주입한다.
  - protocol의 `run_id`와 `run_attempt`는 그 파일 값과 같아야 한다. `started_at`은 그 attempt의 `bound` attestation log 시각 이후이고 `session_result` attestation log 시각 이전이어야 한다. log 시각은 bundle의 검증된 timestamp다(Trust Model). log 시각은 runner가 고칠 수 없으므로 freshness 기준점을 비신뢰 데이터가 정하지 못한다.
  - 범위를 벗어나면 `protocol_mismatch`(`run_id`, `run_attempt`, `started_at`)다. 승인 지연은 상한에 걸리지 않는다.
- 결과 재도출:
  - signer는 검사를 고정된 순서로 한다. 먼저 attestation digest 대조다(REQ-HR-09). 받은 protocol·record·oracle 결과·calibration 결과의 digest가 `session_result` attestation과 다르거나 그중 하나가 없으면 `attestation_digest_mismatch`로 끝난다(전송 중 변조). 그다음에 의미 검사를 한다. 정상적으로 attest되었지만 내용이 틀린 경우가 여기서 걸린다.
  - 의미 검사의 입력(출처는 attest된 bytes뿐이다): signer는 세 입력만 쓴다.
    - records 파일 bytes: SHA-256이 predicate의 `records_sha256`과 같아야 한다. trial마다 `stage_reached`·`agent_termination`·`signal`·`oracle_result_sha256`을 읽는다.
    - trial별 `harness_oracle_result.v1` 파일 bytes: SHA-256이 그 trial record의 `oracle_result_sha256`과 같고 predicate의 `oracle_result_sha256[]`에 들어 있어야 한다. `stage_reached`가 `oracle`이 아닌 trial에는 oracle 결과가 없고 `oracle_result_sha256`은 null이다.
    - `calibration.json` bytes: SHA-256이 predicate의 `calibration_sha256`과 같아야 한다. `session_id`와 task 집합이 trusted protocol과 다르면 `records_protocol_mismatch`다. `before`·`after`의 `status`가 task별 결과와 맞지 않으면(`passed`인데 clean 실패나 mutated 통과가 있음) `outcome_derivation_mismatch`다. 001 verdict는 calibration 입력으로 이 bytes만 쓴다.
    - 여기에 REQ-HR-08 판정표를 다시 적용해 outcome, signal, `oracle` 필드를 계산한다. record 값과 다르거나 어느 행에도 맞지 않으면 `outcome_derivation_mismatch`다. runner가 보고한 outcome은 그대로 믿지 않는다.
- record 검증: `session_id`와 `(task_id, arm, trial)` 집합이 trusted `order`와 같아야 한다(`records_protocol_mismatch`).
- 정책: `max_agent_runs × trial_timeout_seconds ≤ 19800`(6시간 job 한도에서 준비 30분을 뺀 값)이어야 한다. 아니면 `policy_out_of_range`다. 초기값 12 task × K=2 × 2 arm = 48 trial이면 trial당 400초 이하가 된다. 실제 소요는 T10 hosted control run에서 측정한다.
- signer는 artifact 안의 어떤 파일도 실행하지 않는다. 실행하는 코드는 같은 main SHA에서 빌드한 `auto`, 그리고 그것이 이전 tag에서 빌드하는 baseline driver뿐이다. 자식 process는 허용 목록 환경으로만 띄운다.

### REQ-HR-03 Report 변환과 v2 서명
Priority: Must · EARS: EventDriven

WHEN the trusted protocol and the attested records, oracle results, and calibration result are consistent, THEN THE SYSTEM SHALL map the SPEC-HARNEVAL-001 verdict into an `eval_regression_report.v1` with every field populated and sign an `eval_regression_attestation.v2` with the Environment key read only from stdin into a new output directory.

- 변환 표:

| 001 verdict | report `blocked` | report `reason` |
|-------------|------------------|-----------------|
| `ok` | false | `within_threshold` |
| `regression` | true | `hard_flip` 또는 `pass_rate_regression` |
| `incomplete` | true | `incomplete` |
| `vacuous` | true | `vacuous` 또는 `oracle_calibration_failed` |

  - `regression_delta`는 001 값이고, `incomplete`·`vacuous`에서는 0이다. `threshold_value = threshold_bp / 10000`이다. 001 REQ-HE-10의 reason literal과 같아야 한다(CD-HR-5에서 001 문서에 표를 명시).
- report 값: `comparison_scope=adk-harness-golden-live`, `threshold_metric=pass_rate_delta`, `attributed_version`=binding digest, `baseline_ref`, `workspace_scope=autopus-adk`, `redaction_status=body_free`, `retention_class=release_evidence`, `raw_payload_present=false`, `produced_at`=protocol `started_at`. report와 attestation에 같은 문자열을 쓴다(probe A1).
- attestation: `key_id`=`[NEW] evalregression.ADKHarnessEvalKeyID`, `trust_lane`=`[NEW] evalregression.ADKHarnessEvalTrustLane`(`adk-harness-eval`), `source_environment=adk-harness-live`, `target_environment=adk-release`, `source_revision`=binding digest, `workspace_scope=autopus-adk`. 서명은 `[NEW] pkg/evalregression/sign_v2.go::SignEvalRegressionAttestationV2`가 unexported `evalRegressionAttestationV2Message`를 써서 만든다.
- 키 입력: stdin이 비면 `readPrivateKey`를 부르기 전에 `private_key_missing`으로 끝난다. 그 밖의 키 오류는 `readPrivateKey`(`internal/cli/companion_manifest.go:167`)의 오류를 `private_key_invalid`로 낸다.
- 출력: `--output <new-dir>`은 존재하지 않아야 하고(`output_exists`), 0700으로 만든다. 두 파일을 다 쓴 뒤에만 성공이다. 중간에 실패하면 그 디렉터리를 지우고 종료 코드 1이다.

### REQ-HR-04 Binding digest
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL compute one binding digest from the candidate default surface digest, the agent set digest, the corpus file digests, the runner tree digest, the live policy, the model, the workspace revision, the baseline ref and its commit, and the pinned generator and CLI versions, and use that digest as both `source_revision` and `attributed_version`.

- 정의: SHA-256 hex(Go `json.Marshal` of `harness_eval_binding.v1{candidate_surface_digest, agent_set_digest, corpus_digests[], runner_tree_digest, policy{…}, model, workspace_revision, baseline_ref, baseline_commit, signing_key_id, pins{…}}`). `signing_key_id`는 `ADKHarnessEvalKeyID` 상수다. 키를 회전하면 binding이 바뀌므로, 회전 전 키의 세션은 새 binding의 window에 들어오지 않는다.
- `runner_tree_digest`는 결과를 정하는 코드 전부의 (경로, SHA-256)을 정렬해 해시한 값이다. 범위는 `scripts/benchmarks/harness/**`(runner, grader 준비, profile, surface driver), `pkg/harneval/**`(verdict, records, derive, protocol_trust, report_v1, releasecheck), `[NEW] cmd/harneval-oracle/**`(oracle harness), sandbox profile 원본이다. 채점·판정·격리 코드 중 하나라도 바뀌면 binding이 바뀐다. `model` 변경도 마찬가지다.
- `baseline_commit`은 `baseline_ref` tag가 가리키는 40-hex 커밋이다. release는 tag가 아직 그 커밋을 가리키는지 확인한다. 그래서 이전 tag를 다른 조상으로 옮기면 binding이 맞지 않는다.
- binding digest를 계산하는 job은 모두 `macos-15`에서 돈다.

### REQ-HR-05 Trust lane 분리와 키 회전
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL add only the public key of the `adk-harness-eval` lane to `evalRegressionPublicKeys`, keep the lane key id and trust lane as exported constants, add no unsigned-accept path, prove by test that harness-lane and Autopus-lane evidence reject each other with `attestation_policy_mismatch`, and verify every signed output against the committed allowlist before upload.

- Autopus lane 값: trust lane `staging-to-main`, source `staging`, target `production`은 `../Autopus/.github/workflows/eval-regression-gate.yml:201-203`에 있다. expected key id는 같은 파일 L200에서 `${EXPECTED_KEY_ID}`로 넘기고, 그 값 `autopus-eval-staging-to-main-2026-07`은 `pkg/evalregression/attestation.go:64`(`evalRegressionPromotionKeyID`)에 있다. 양방향 거부는 key_id 차이로 성립한다(probe A1).
- 자기 검증: sign job은 업로드 전에 `checkEvalRegressionStrict`와 같은 경로로 자기 출력을 검증한다. `ok`나 `regression_blocked`가 아니면 업로드하지 않고 `self_verify_failed`로 끝난다.
- 키 회전: 상수 교체가 binding을 바꾼다(REQ-HR-04의 `signing_key_id`). 그래서 회전 전 세션은 새 binding의 window에 들어오지 않고 release를 막지 않는다. 순서는 다음과 같다. live dispatch를 멈추고, 실행 중이거나 승인 대기 중인 run을 취소한다. 공개키 추가·상수 교체·Environment secret 교체를 한 번에 적용한 뒤 새 binding으로 세션을 다시 만든다. runbook에 이 순서를 적고 `TestEvalRegressionRequiredCheckRunbookExists`의 문자열은 보존한다.
- v1 API `VerifyEvalRegressionArtifact`는 lane policy 없이 allowlist 전체를 신뢰한다. 지금 production 파일 `internal/cli/eval_regression.go:178`의 `evaluateEvalRegression`이 v1을 부르지만, 그 경로(`checkEvalRegression`)는 test에서만 쓰이고 운영 경로는 strict만 쓴다(`check.go:224`). T6은 이 두 v1 helper를 test 전용 파일로 옮긴다. 그 뒤 non-test 호출자 0개를 static test로 고정한다.

### REQ-HR-06 Release 차단 연결과 게시 직전 재검증
Priority: Must · EARS: EventDriven

WHEN the release workflow runs, THEN THE SYSTEM SHALL compute the binding digest from the release source, read the current time once as `T_now`, collect every session for that binding inside the window from both the attestation record and the Actions run and attempt listing, verify each one with `auto check --eval-regression` using all six expected policy flags without `--warn-only`, stop unless at least one session verifies as `ok` and every other session verifies as `ok` or verified `incomplete`, and repeat the same check inside the `release` job immediately before publishing.

- window: `[L, T_now]`, `L = T_now − 72h + 15m`. 포함 여부는 두 출처 모두 하나의 timestamp로 정한다. 그 timestamp는 그 attempt의 bound 기록의 log 시각(검증된 timestamp, REQ-HR-07)이다. B subject와 run key subject는 같은 bound attestation에 들어 있다. 경계는 닫힌 구간이다. log 시각이 정확히 `L`이면 포함, `L − 1s`이면 제외다. bound 기록 없이 session-result만 남은 세션은 그 session-result의 log 시각으로 정한다.
- 검사 시간: `harness-eval-evidence` job의 검사 step과 `release` job의 재검증 step은 둘 다 step 수준 `timeout-minutes: 15`다(`release` job 전체는 60분). 각 step은 명령 직전에 `T_now`를 한 번 읽어 `--now-file`로 넘긴다. 그래서 `T_now`부터 `auto check`가 실제 시계로 freshness를 판정하는 순간까지가 15분 안에 든다. `produced_at`은 bound log 시각보다 늦으므로, window 안의 모든 증거는 `--eval-regression-max-age 72h` 검사에서 fresh다.
- 세션 판정:
  - 아래 귀속 검사로 binding이 B로 정해진 attempt만 판정한다. 완료되지 않은 attempt는 `run_in_progress`, `success`가 아닌 attempt는 `run_not_ok`다. 승인 거부, 취소, 실패가 여기에 해당한다.
  - 증거는 `auto check` 출력으로 판정한다. 출력이 `eval-regression: ok (version=B)`이면 통과다.
  - 출력이 정확히 `eval-regression: regression_blocked (version=B)`일 때만 서명·policy·freshness가 모두 검증된 것이다. 이때에 한해 검증된 report bytes의 `reason`을 읽고, `incomplete`이면 세지 않는다.
  - 그 밖의 출력은 모두 멈춘다.
  - 검증된 `ok`가 없으면 `artifact_missing`이다.
- 열거와 귀속:
  - run 쪽: `gh api --paginate`로 `harness-eval-live.yml`의 runs와 attempts를 모은다. attempt 시작 시각이 `[T_now−96h, T_now]`인 것을 후보로 둔다. bind가 늦게 끝나는 경우를 포함하는 상위 집합이다. 포함 여부는 위의 bound log 시각으로 정한다. bind가 attempt 시작 뒤 24시간 가까이 지나서야 끝난 드문 attempt는 후보에서 빠질 수 있다. 그러면 attestation 쪽에만 남아 `session_log_inconsistent`로 멈춘다(fail-closed). 모은 수가 `total_count`와 다르면 `run_list_truncated`다.
  - run의 binding 귀속: Actions 응답에는 binding이 없다. 그래서 bind job은 같은 bound 사실을 두 subject로 attest한다. 하나는 `sha256:<B>`이고, 다른 하나는 `sha256("harneval-run:" + run_id + ":" + attempt)`인 run key다. release는 run 쪽 attempt마다 run key subject를 조회해 binding을 알아낸다. binding이 B가 아니면 세지 않는다. run key 기록이 없는 후보 attempt는 jobs API의 live-eval step 상태로 나눈다(golden 세션 step 이름은 `golden-session`으로 고정한다). golden 세션 step이 시작되지 않았으면 trial이 없으므로 세지 않는다. 예를 들어 bound 확인 step이 `attempt_unbound`로 실패한 부분 재실행이 여기에 해당한다. golden 세션 step이 시작되었으면 `session_log_inconsistent`다. 이 귀속 검사는 `run_not_ok` 판정보다 먼저 한다.
  - attestation 쪽: `sha256:<B>` subject 목록(REQ-HR-07)에서 window에 드는 세션 중 run 쪽에 없는 것은 `session_log_inconsistent`다. bound 기록 없이 session-result만 남은 세션도 `session_log_inconsistent`다.
- 권한: `harness-eval-evidence` job은 `permissions: {contents: read, actions: read, attestations: read}`다. `release` job은 기존 `contents: write`, `id-token: write`에 `actions: read`, `attestations: read`를 더한다. 기존 test(`release_wiring_test.go`)는 `id-token` 값만 단언하므로 영향이 없다. 다른 workflow run의 artifact를 받으려면 `actions: read`가 필요하다.
- 실행 형태:
  - `harness-eval-evidence` job과 `release` job의 재검증 step은 `auto eval harness release-check --binding B --now-file <f>`를 `env -i PATH="$PATH" HOME="$HOME" GH_TOKEN="$GH_TOKEN"`으로 실행한다.
  - step env는 `GH_TOKEN` 하나다. release.yaml을 읽는 기존 test가 step env를 allowlist(`GH_TOKEN` 포함)로 제한하고, job env를 금지하며, run 안의 `${{ secrets.`를 금지하고, release 명령에 `env -i`를 요구하기 때문이다.
  - step 이름과 경로에는 `fixture/`, `testdata/`, `localhost`, `PREVIOUS…FILE` 형태를 쓰지 않는다.
- 재검증: `release` job은 `adk-companion-release` 승인을 최대 30일 기다릴 수 있다. 그래서 게시 step 직전에 `T_now`를 다시 읽고 같은 검사를 한다. 승인이 늦어 증거가 만료되었거나 그 사이 새 회귀 세션이 생겼으면 게시하지 않는다(기존 OMP 증거 재검증 패턴, `release.yaml` L316).
- 복구: 증거가 만료되면 binding을 만든 원래 run을 `Re-run all jobs`로 다시 실행한다. GitHub의 재실행 가능 기간 안이면 같은 SHA, 같은 binding의 새 attempt가 된다. main이 평가 입력을 바꾼 뒤라면 새 release tag가 필요하다. 72시간 window와 flaky 차단은 사용자가 수락했다(answered).
- `needs`는 `[ci, security, omp-production-evidence, harness-eval-evidence]`이고 기존 세 항목의 의미는 유지한다.

### REQ-HR-07 Append-only 세션 기록
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL record every bound session and every session result as a GitHub artifact attestation whose subject digest is the binding digest, verify each attestation bundle with its transparency log proof and the workflow identity before counting it, and treat any session missing from either the attestation record or the run listing as blocking.

- 매체(coordinator 결정, 2026-10-06): `autopus-ai/autopus-adk`는 public repo다. 그래서 `actions/attest`는 공개 Sigstore instance에 서명하고 transparency log에 entry를 남긴다. 그 entry는 repo 내부자가 지울 수 없다.
  - subject: `sha256:<binding digest>`
  - predicate type `https://autopus.ai/harness-eval/bound/v1`과 `…/session-result/v1`
  - predicate: `{run_id, run_attempt, binding_digest, event}`이고, session-result에는 protocol·record·oracle 결과·calibration 결과 digest가 더 들어간다
- 열거 방법과 한계:
  - release는 `GET /repos/autopus-ai/autopus-adk/attestations/sha256:<B>`(paginate)로 attestation을 모은다.
  - bundle마다 서명, Fulcio 인증서의 workflow 신원(`…/harness-eval-live.yml@refs/heads/main`), log inclusion 증명, timestamp를 검증한다. 도구는 `gh attestation verify --bundle` 또는 sigstore-go를 쓴다. 신원이나 증명이 틀리면 `attestation_identity_invalid`다.
  - 시각은 검증 결과가 돌려준 timestamp만 쓴다. bundle 안의 `integratedTime` 값을 검증 없이 읽지 않는다. 검증된 timestamp가 하나도 없으면 `attestation_timestamp_missing`이다. 실제 bundle의 timestamp 출처(Rekor v1 integrated time 또는 TSA)는 probe A2가 관측한다.
  - Rekor의 hash 검색(`rekor-cli search --sha`, `/api/v1/index/retrieve`)은 열거에 쓰지 않는다. v1에서도 best-effort였고 Rekor v2에서는 제거되었다.
  - GitHub attestation 저장소에는 삭제 API가 있다. 그래서 완전성은 run 목록과의 교차 대조(REQ-HR-06)로 확보한다. 지워진 attestation의 log entry는 감사 증거로 남는다.
- trade-off:
  - 공개되는 정보: binding digest, run id, attempt, repo·workflow 신원이며, 모두 hash나 식별자다. report 본문과 secret은 없다.
  - Sigstore와 GitHub API에 가용성을 의존한다. 기록 생성에 실패하면 bind job이 실패해 trial이 돌지 않는다. release 때 조회에 실패하면 `session_log_unavailable`로 멈춘다(fail-closed).
- 보장하는 것과 보장하지 않는 것:
  - 보장: 기록된 세션의 tamper-evidence다. attest된 bound·session-result는 공개 log entry로 남아 내용을 바꿀 수 없다. 남아 있는 기록과 run 목록이 서로 어긋나면 release가 멈춘다.
  - 보장하지 않음: run과 GitHub attestation을 둘 다 지울 수 있는 행위자에 대한 열거 완전성이다. 그런 행위자는 세션을 숨길 수 있다. 공개 log entry는 남지만 열거 출처가 아니기 때문이다.
  - A2 결과에 따른 분기(CD-HR-1):
    - A2 PASS(attestation 삭제에 admin 권한이 필요함): 잔여 위험은 신뢰 경계 안의 admin으로 한정된다. T7의 release 연결을 merge할 수 있다.
    - A2 FAIL(write 권한으로 지울 수 있음): T7을 merge하지 않고 release gate를 켜지 않는다. 서명 증거는 release를 막지 않는 참고 자료로만 남는다. 차단을 켜려면 세 번째 출처가 필요하다. 예를 들어 bot만 쓸 수 있고 ruleset이 force-push와 삭제를 막는 protected branch의 세션 log다. 이 출처는 이 SPEC을 개정해 정의하고 구현한다. 그때까지 CD-HR-1은 열려 있고 sync 완료가 아니다. 그 사실을 사용자에게 보고한다.

### REQ-HR-08 Black-box oracle
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL admit to the signed release gate only agent tasks whose oracle is black-box, build and run the agent-modified artifact under a sandbox profile that denies reading the expected outputs, and judge it only with a trusted oracle harness built from main that the trusted runner starts as a separate sandboxed process after the artifact process has exited, that receives the expected outputs from the runner instead of reading them from disk, that reads each pinned output only as a regular single-link file inside the output root without following links, and that compares the artifact's exit status, stdout, and output files with the task's pinned expectations.

- 모델: agent 코드는 artifact의 빌드와 실행에만 영향을 준다. oracle harness(`[NEW] cmd/harneval-oracle`)는 main에서 빌드한 별도 binary다. artifact와 address space를 공유하지 않는다. 다루는 것은 artifact가 끝난 뒤의 출력 파일, 그리고 runner가 stdin으로 넘긴 기대 출력·task 정의·stdout·종료 상태뿐이다. 그래서 같은 process에서 결과를 꾸미는 위협 유형은 판정에 영향을 줄 수 없다.
- process 구조(고정): sandbox 밖의 trusted runner(golden.py)가 각 단계를 형제 process로 차례대로 띄운다. sandbox 안에서 다른 profile을 다시 적용하는 중첩 구조는 쓰지 않는다. 2026-10-06 로컬 macOS 26.5.2에서 제한이 있는 profile 안의 process가 다른 profile로 `sandbox-exec`를 다시 실행하면 `sandbox_apply: Operation not permitted`(exit 71)로 실패했다.
  1. 준비: runner가 task의 입력 fixture를 trial별 입력 디렉터리에 복사한다. 복사본과 main checkout의 기대 출력을 `black_box_oracle`의 SHA-256 목록과 대조하고, 다르면 trial 없이 `workspace_setup_failed`다.
  2. 빌드: runner가 grade 사본에서 artifact를 빌드한다. 빌드도 agent가 고친 입력을 처리하므로 `artifact.sb` 빌드 모드에서 한다. 빌드 모드는 읽기 전용 module cache와 trial별 build cache를 더한다.
  3. 실행: runner가 artifact를 `artifact.sb` 실행 모드로 띄운다. stdout(1 MiB까지), 종료 상태, timeout 여부는 runner가 받아 runner 소유 디렉터리에 쓴다. artifact가 끝나거나 timeout이 되면 runner는 그 process group을 끝내고, 001 REQ-HE-08처럼 남은 process가 0개인지 확인한다. 확인하지 못하면 oracle을 띄우지 않고 `observation_failed`다.
  4. 판정: 그 뒤 runner가 oracle harness를 `oracle.sb`로 띄운다. runner는 다음을 stdin으로 넘긴다: SHA-256을 대조한 기대 출력, main의 task 정의에서 가져온 assertion 정의와 고정 출력 경로, 캡처한 stdout, 종료 상태. `oracle.sb`가 읽기를 허용하는 곳은 oracle binary, artifact 출력 root, 실행에 필요한 시스템 경로뿐이고, main checkout과 기대 출력 파일은 읽지 못하게 한다. 쓰기는 자기 결과 디렉터리뿐이며 artifact 파일에는 쓸 수 없다.
- `artifact.sb`(읽기 허용 목록 방식):
  - 읽기는 실행에 필요한 시스템 경로, artifact 자신, trial별 입력 디렉터리, 자기 scratch 디렉터리만 허용하고(빌드 모드는 cache 두 개를 더한다) 그 밖은 모두 거부한다. 그래서 main checkout(`evals/harness/**`와 `.git` 포함), records·protocol·control 디렉터리, runner 소유 디렉터리, `$HOME`, runner 임시 디렉터리를 읽을 수 없다.
  - network는 거부한다. 쓰기는 scratch 출력 디렉터리뿐이다(빌드 모드는 build cache와 출력 경로를 더한다). 환경은 허용 목록뿐이다.
  - 이 제한은 artifact가 만든 자식 process에도 상속된다. 남은 process는 oracle 전에 정리하고 확인한다(3단계). 정리 전에도 그 process는 기대 출력을 읽거나 결과·runner 디렉터리에 쓸 수 없다.
- 출력 열기 규칙(confused deputy 방지): oracle은 기대 출력을 디스크에서 읽을 수 없다. 그래도 artifact가 만든 출력 구조를 통해 다른 파일을 대신 읽게 되는 위협 유형이 있다. 그 유형은 링크, link 수가 2 이상인 파일, 일반 파일이 아닌 항목, root 밖으로의 이탈이고, 아래 규칙으로 막는다.
  - oracle은 artifact 출력 디렉터리를 `os.Root`로 연다. main의 task 정의에 고정된 상대 경로만 열고, artifact가 낸 내용이나 디렉터리 목록에서 경로를 얻지 않는다.
  - 각 경로는 모두 만족해야 한다: 중간 요소가 모두 symlink가 아닌 디렉터리다. 마지막 요소의 Lstat이 일반 파일이다. `O_RDONLY|O_NOFOLLOW|O_NONBLOCK`으로 연 fd의 fstat이 일반 파일이고, link 수가 1이며, Lstat 결과와 같은 파일(`os.SameFile`)이다. 내용은 그 fd에서만 파일당 1 MiB까지 읽는다.
  - 하나라도 어긋나거나 root 밖을 가리키면 `output_check=link_rejected`이고, 상한을 넘으면 `too_large`다. 거부된 출력은 비교하지 않는다.
  - 근거(2026-10-06 로컬, go1.26.6과 macOS 26.5.2, scratch 일회성 probe): `os.Root`는 root 밖을 가리키는 symlink, `..`, 디렉터리 symlink를 `path escapes from parent`로 거부했다. 그러나 root 안의 symlink는 `O_NOFOLLOW`를 줘도 따라갔다(SameFile 거짓). hard link는 바깥 파일 내용을 그대로 읽었고(link 수 2), FIFO는 `O_NONBLOCK`으로 막히지 않고 열렸다(일반 파일 아님). 그래서 Lstat, SameFile, link 수, 파일 종류 검사를 함께 둔다.
- 기대값 비공개: 기대 출력은 main checkout에만 있고 trial 디렉터리로 복사하지 않는다. 디스크의 기대 출력을 읽는 process는 runner 하나이고, oracle harness는 stdin으로만 받는다. 같은 날 로컬 Seatbelt는 읽기가 거부된 파일로 hard link를 만드는 것도 `Operation not permitted`로 거부했다. 그래도 위 link 수 검사는 그와 별개로 둔다. agent workspace에서는 `evals/harness/**`를 빼고(pilot profile이 workspace 밖 읽기를 막는다), 빌드와 실행 중에는 `artifact.sb`가 읽기를 막는다. probe A3와 hosted preflight(REQ-HR-10)가 읽기 거부를 확인한다.
- 판정: oracle harness는 `harness_oracle_result.v1`을 쓴다(Wire Contracts). trusted runner와 signer는 같은 표를 쓴다. 위에서부터 처음 맞는 행 하나가 outcome과 signal을 정한다.
  - 1-4행은 001 REQ-HE-08의 분류를 따른다. 001처럼 agent 단계가 실패해도 채점은 계속하고, `observation_failed`와 `scope_violation`이면 그 뒤 단계를 건너뛴다.
  - black-box trial에는 001의 리터럴 검사와 go test 채점을 하지 않는다. 그래서 `forbidden_construct`, `oracle_failed`, `oracle_timeout`, `oracle_output_invalid`는 생기지 않는다. record `signal`이 이 표에 없는 값이면 signer는 `outcome_derivation_mismatch`로 거부한다.

| 순위 | 판정 입력(records bytes와 oracle 결과 bytes) | outcome, signal |
|------|------------------------------------------------|-----------------|
| 1 | record `stage_reached=setup`이고 `signal`이 `workspace_setup_failed`·`mutation_failed`·`warmup_failed` 중 하나 | error, 그 signal |
| 2 | record `agent_termination`: `launched=false`이면 `agent_launch_failed`, `timed_out=true`이면 `agent_timeout`, 그 밖에 `exit_code≠0`이거나 `os_signal`이 있으면 `agent_exit_nonzero`(이 순서). 채점 결과와 무관하다 | fail, 해당 signal |
| 3 | record `signal=observation_failed`(runner가 agent나 artifact의 남은 process 0개를 확인하지 못함) | fail, `observation_failed` |
| 4 | record `signal=scope_violation`이고 `stage_reached=agent`(빌드와 oracle 생략) | fail, `scope_violation` |
| 5 | record `stage_reached=build`(빌드가 실패해 실행 단계로 가지 못함) | fail, `artifact_build_failed` |
| 6 | `stage_reached=oracle`인데 oracle 결과 bytes가 없거나 schema가 틀림. 또는 `timed_out=false`, `output_check=ok`인데 assertion id 집합이 main의 task 정의와 다름 | fail, `oracle_harness_error` |
| 7 | oracle 결과 `timed_out=true` | fail, `artifact_timeout` |
| 8 | oracle 결과 `output_check`가 `link_rejected` 또는 `too_large` | fail, `output_link_rejected` 또는 `output_too_large` |
| 9 | assertion 하나라도 `passed=false` | fail, `expectation_mismatch` |
| 10 | 모든 assertion이 `passed=true` | pass, `accepted` |

- `oracle{…}` 필드는 001처럼 행과 별개로 채운다.
  - `build_failed`는 빌드 단계가 실패했는지다.
  - `ran`은 실제로 비교했는지다. oracle 결과가 schema에 맞고, `timed_out=false`, `output_check=ok`이며, assertion id 집합이 main의 task 정의와 같을 때만 true다.
  - `expected_passed`와 `expected_failed`는 `ran`이 true일 때 통과·실패한 assertion 수이고, 아니면 0이다.
- 001 verdict의 vacuity 규칙(한 arm이라도 `oracle.ran`이 true인 record가 0개이면 `vacuous`)은 black-box record에도 그대로 적용된다. 그래서 환경 고장으로 두 arm의 모든 trial이 빌드 실패, timeout, 출력 거부가 되어도 `ok`가 서명되지 않는다.

- 대상 task: 서명 lane의 agent task는 `oracle_mode: black_box`이고 `black_box_oracle`(명령, 입력 fixture, 기대 출력)을 가진다. white-box oracle만 가능한 task는 서명 gate에서 빠지고 001 advisory lane에 남는다. corpus 12개의 분류와 coverage trade-off는 `research.md` `## Black-box Oracle Coverage`에 있다. manifest의 `floors.signed_agent_tasks`(초기 5)보다 적으면 `vacuous`다.
- calibration: black-box task도 001 calibration과 같은 두 방향 검사를 위의 process 구조로 한다. 변형 전 reference artifact는 `accepted`여야 하고, mutation을 적용한 artifact는 `expectation_mismatch`여야 한다. 결과는 001 `calibration.json`의 `before`(세션 시작 전)와 `after`(모든 trial 뒤)에 쓴다. `before`가 실패하면 001 REQ-HE-09처럼 agent를 호출하지 않고 record 없이 golden step이 종료 코드 1로 끝나므로, 그 run은 release에서 `run_not_ok`다. `before`나 `after`가 `passed`가 아닌 세션은 001 REQ-HE-10에 따라 `ok`가 될 수 없다. 그래서 mutation까지 통과시키는 약한 oracle이나 세션 중 고장 난 환경은 서명 gate를 열 수 없다. `after`는 모든 trial 뒤에 쓰이므로 protocol digest가 덮지 못한다. 그래서 `calibration.json` bytes의 SHA-256을 session-result attestation에 넣고 signer가 대조한다(REQ-HR-02, REQ-HR-09).
- 001 advisory lane의 in-process 채점과 리터럴 검사는 그대로 둔다. 서명 lane은 그것을 판정 근거로 쓰지 않는다.

### REQ-HR-09 Signer 입력의 attestation 대조
Priority: Must · EARS: Unwanted

IF the protocol, records, oracle results, or calibration result received by the signer differ from the digests in the `session_result` attestation of the same run and attempt or any of them is missing, THEN THE SYSTEM SHALL refuse to sign with `attestation_digest_mismatch` before any semantic check and write nothing.

- 이 대조로 eval job이 올린 뒤 artifact가 바뀌거나 바꿔치기되는 것을 막는다. attestation 신원은 같은 workflow의 `live-eval` job이어야 한다.

### REQ-HR-10 Hosted 실행 검증
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL run a sandbox preflight on the hosted `macos-15` runner before any trial, run the end-to-end chain test on a macOS CI job with a test-count floor and a pass-set comparison, and require one recorded hosted control run before the release gate is enabled.

- preflight: runner에서 다음을 확인한다. 하나라도 기대와 다르면 `sandbox_preflight_failed`로 trial 없이 끝난다.
  - 001 probe A3와 같은 검사: grade 밖 쓰기와 network 거부, 안 쓰기 성공.
  - `artifact.sb`의 읽기 거부: main checkout의 `evals/harness/**` 읽기와, 그 파일로의 hard link 생성.
  - oracle harness의 출력 열기 자체 검사: runner가 scratch 출력 root에 위협 유형별 항목을 하나씩 만든다. root 안 symlink, root 밖 symlink, 디렉터리 symlink, hard link, FIFO는 모두 `link_rejected`, 상한 초과 파일은 `too_large`, 일반 파일만 `ok`여야 한다.
- CI: S5 사슬은 sandbox-exec가 필요하므로 기존 `macos-runtime` job 패턴을 따른다. 그 job에서 `go test -list`로 개수 floor를 확인한 뒤 `-v`로 실행하고, PASS 집합을 비교한다. ubuntu `go test ./...`에서는 build tag로 빠지며, skip을 통과로 세지 않는다.
- control run(OPS, T10): main에서 실제 dispatch 한 번. 증거는 run URL, attestation 두 개, 서명 증거, `release-check` 출력, trial 소요 시간이다.

## Wire Contracts

| 문서 | 필드 |
|------|------|
| protocol 추가 필드 | `run_id`, `run_attempt`, `binding_digest`, `baseline_commit`, `runner_tree_digest` 다섯 개(001 protocol에 optional로 더한다. signed lane에서는 필수. writer는 golden.py T13, decoder는 T2). black-box calibration은 001 `calibration.json`에 쓴다 |
| record 추가 필드 | `oracle_result_sha256`(`stage_reached`가 `oracle`이 아니면 null), `stage_reached: setup\|agent\|build\|run\|oracle`, `agent_termination{launched, exit_code, os_signal, timed_out}`(agent process의 종료). 정규화: 시작하지 못했으면 `launched=false`이고 나머지는 null·false다. signal로 끝났으면 `exit_code`는 null, `os_signal`은 signal 이름(예: `SIGKILL`)이다. Python의 음수 returncode는 runner가 이 형식으로 바꾼다. `timed_out`은 runner가 시간 초과로 끝냈는지이며 종료 표기와 별개다. trusted runner가 쓴다. black-box record의 `signal`과 `oracle{…}`은 REQ-HR-08 판정표의 매핑을 따른다 |
| `harness_oracle_result.v1` | `task_id`, `output_check: ok\|link_rejected\|too_large\|not_checked`, `assertions[]{id, passed}`, `artifact_exit`, `timed_out`. oracle harness가 쓴다. `artifact_exit`과 `timed_out`은 runner가 넘긴 값을 옮긴다. `timed_out`이 true이면 `output_check=not_checked`이고, `output_check`가 `ok`가 아니면 `assertions`는 비어 있다 |
| `calibration.json` | 001 `harness_golden_calibration.v1` 그대로다. 서명 lane에서는 그 bytes의 SHA-256이 session-result predicate의 `calibration_sha256`이다 |
| `harness_eval_binding.v1` | REQ-HR-04 정의 |
| run meta 파일 | `{run_id, run_attempt, run_created_at, attempt_started_at}` |
| attestation predicate | bound(subject 두 개: binding과 run key): `{run_id, run_attempt, binding_digest, event: "bound"}`. session-result: 여기에 `protocol_sha256`, `records_sha256`, `oracle_result_sha256[]`, `calibration_sha256`이 더해진다 |
| `eval_regression_report.v1`, `eval_regression_attestation.v2` | 기존 schema 그대로. 값은 REQ-HR-03 |
| artifact 이름 | `harness-eval-unsigned-<run_id>-<attempt>`, `harness-eval-evidence-<binding16>-<run_id>-<attempt>` |

## Existing Test Changes

근거로 release.yaml을 읽는 test를 helper 호출까지 따라가 grep했다. 9개 파일에 23개가 있고, 그중 5개는 모든 job에 제약을 건다.

- 모든 job 제약 test:
  - `TestReleaseWorkflow_UsesOnlyImmutableActions`
  - `TestReleasePublicKeyReceipt_A0Policy_IsOneExactAuditableBootstrap`(env 이름에 BOOTSTRAP·IS_A0·ALLOW_A0 금지)
  - `TestReleasePublicKeyReceipt_FixturesAreParserOnlyAndCannotCreateFakeA0Pass`(`fixture/`·`testdata/`·`localhost`·`(A0|PRIOR|PREVIOUS)…(PATH|FILE)` 금지)
  - `TestReleasePublicKeyReceipt_Workflow_ExternalActionsAreImmutableSHAPinned`
  - `TestReleasePublicKeyReceipt_Workflow_SecretsAreStepScopedAndCleanupFailuresObserved`(job env 금지, step env allowlist, run 안의 `${{ secrets.` 금지, release 명령의 `env -i`)
- 새 job은 이 다섯 제약을 지키므로 그 test들은 바뀌지 않는다. ci.yaml을 읽는 기존 test는 4개 파일에 8개다(001 확인).

| 파일 | 위치 | 지금 | 바뀐 뒤 |
|------|------|------|---------|
| `pkg/evalregression/verify_e2e_test.go` | L120 `len(keys) != 1` | allowlist 길이 1 | key id 집합이 `{autopus-eval-staging-to-main-2026-07, ADKHarnessEvalKeyID}`와 같음 |
| `pkg/evalregression/verify_e2e_test.go` | L138 `len(again) != 1` | 방어 사본 재조회 길이 1 | 반환 map을 바꾼 뒤 다시 조회한 key id 집합이 위 두 개와 같음 |
| `internal/companionmanifest/release_contract_test.go` | L15 `TestReleaseWorkflow_ExactA34ProtectedNormalLane` | 리터럴 `needs: [ci, security, omp-production-evidence]` | 리터럴 `needs: [ci, security, omp-production-evidence, harness-eval-evidence]` |
| `internal/cli/eval_harness_workflow_test.go`(001 T9, 아직 없음) | 001 S12 단언 | live lane workflow 없음, allowlist diff 0 | 001 golden 명령이 서명하지 않음만 단언(Cross-SPEC Changes) |

## 생성 파일 상세

- `[NEW] pkg/evalregression/sign_v2.go`와 상수 `ADKHarnessEvalKeyID`, `ADKHarnessEvalTrustLane`. `attestation.go`에는 공개키 한 항목과 package doc만 추가한다.
- `[NEW] pkg/harneval/binding.go`, `protocol_trust.go`, `derive.go`(oracle 결과 재도출), `report_v1.go`, `releasecheck.go`, `[NEW] cmd/harneval-oracle/`(trusted oracle harness), `[NEW] scripts/benchmarks/harness/artifact.sb`, `oracle.sb`(두 sandbox profile).
- `[NEW] internal/cli/eval_harness_export.go`, `eval_harness_policy.go`, `eval_harness_releasecheck.go`: `auto eval harness export|policy|release-check`, `digest --binding`. T6은 `internal/cli/eval_regression.go`의 v1 helper 두 개를 test 전용 파일로 옮긴다.
- 001 소유 파일 변경은 Cross-SPEC Changes 표대로다. `scripts/benchmarks/harness/golden.py`, `pkg/harneval/records.go`가 대상이다.
- `[NEW] .github/workflows/harness-eval-live.yml`, `.github/workflows/release.yaml`(`harness-eval-evidence` job, `needs`, `release` job 재검증 step), `.github/workflows/ci.yaml`(macos job의 S5 step), `.github/EVAL_REGRESSION_REQUIRED_CHECK.md`(lane 절과 키 회전).
- `[NEW] internal/cli/eval_harness_lane_test.go`, `eval_harness_live_workflow_test.go`, `eval_harness_e2e_test.go`(`//go:build darwin`), `eval_harness_release_test.go`.

## Related SPECs

- SPEC-HARNEVAL-001 rev 4 (Primary): T10-T13에 의존한다. 001 advisory 의미는 유지하고, Cross-SPEC Changes 표의 개정이 필요하다(CD-HR-5).
- SPEC-HARNEVAL-002: 의존 관계가 없다.
- Sibling 한도: HARNEVAL sibling은 002와 이 SPEC 둘로 끝난다. 이 SPEC은 sibling을 만들지 않는다.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-HR-01 | T5, T10 | S3 | INV-HR-03 |
| REQ-HR-02 | T2 | S4 | INV-HR-04 |
| REQ-HR-03 | T1, T3 | S1, S2 | INV-HR-01, INV-HR-02 |
| REQ-HR-04 | T4, T11 | S5 | INV-HR-05 |
| REQ-HR-05 | T6, T10 | S1, S12 | INV-HR-01 |
| REQ-HR-06 | T7, T10 | S6, S10 | INV-HR-06 |
| REQ-HR-07 | T8 | S7 | INV-HR-07 |
| REQ-HR-08 | T9, T13, T15 | S8 | INV-HR-08 |
| REQ-HR-09 | T2 | S4 | INV-HR-04 |
| REQ-HR-10 | T11, T14, T10 | S11, S5 | - |
| Brownfield와 001 교차 변경 | T6, T7, T12 | S9 | - |

## Review Resolution (rev 2-4)

| Finding | 처리 | 위치 |
|---------|------|------|
| HR-F-002 (critical) 같은 process 위조 | rev 2는 in-process 관측과 정적 검사로 완화하려 했다. rev 3에서 black-box oracle로 대체했다 | REQ-HR-08 |
| HR-F-001, HR-F-005 001 소유 변경 | Cross-SPEC Changes 표, 소유 task(T2, T12, T13), 001 선행 조건 | Cross-SPEC Changes |
| HR-F-003 원장 권한 충돌 | attestation을 택하고 bind·live-eval·sign 권한을 다시 정했다(stored secret은 없고 OIDC만) | REQ-HR-01, REQ-HR-07 |
| HR-F-004 window | `T_now` 한 번, `[T_now−72h+15m, T_now]`, log 시각 기준, timeout 15분 | REQ-HR-06, S6 |
| HR-F-010 부분 재실행 | live-eval이 같은 attempt의 bound 기록을 요구하고(`attempt_unbound`), 복구는 전체 재실행이다 | REQ-HR-01, REQ-HR-06, S6 |
| HR3-F-003 binding 범위 | `model`, `runner_tree_digest`, `baseline_commit` 추가 | REQ-HR-04, S5 |
| HR3-F-008 incomplete 면제 | 변환 표, 검증된 `regression_blocked` 출력 뒤에만 reason을 읽음 | REQ-HR-03, REQ-HR-06, S6 |
| HR3-F-004 게시 직전 | `release` job 안에서 다시 검사 | REQ-HR-06, S10 |
| HR3-F-010 hosted 증거 | preflight, macOS CI job, control run | REQ-HR-10, S11 |
| HR-F-007 수치 | test 23개와 모든 job 제약 5개를 열거하고 key id 위치를 정정했다 | Existing Test Changes, REQ-HR-05 |
| HR-F-009 started_at | log 시각으로 상한과 하한을 둔다 | REQ-HR-02 |
| HR-F-011 S5 위치 | macOS CI job | REQ-HR-10 |
| HR-F-012 시간 한도 | 400초 계산과 control run 측정 | REQ-HR-02, T10 |
| HR-F-013 S3·S8 | 40-hex, `persist-credentials` 단언, reason을 정확히 하나로 | S3, S8 |
| HR-F-014 export 계약 | `--run-meta`, `private_key_missing` 순서, 새 디렉터리 규칙 | REQ-HR-02, REQ-HR-03, S2 |
| HR-F-015 키 회전 | 자기 검증과 회전 절차 | REQ-HR-05, S12 |
| HR3-F-011 복구 | 원래 run 전체 재실행 또는 새 tag | REQ-HR-06 |
| HR-F-016 자식 env | `env -u`와 허용 목록 자식 환경, S2 canary | REQ-HR-01, REQ-HR-02, S2 |
| HR-F-017 tag 이동 | `baseline_commit` binding | REQ-HR-04 |
| HR-F-018 v1 API | non-test 호출자 0 static test | REQ-HR-05 |
| HR-F-002 (rev 3, critical) 같은 process 진위 | in-process 증명을 그만두고 black-box oracle로 바꿨다. trusted harness는 별도 binary·process·sandbox에서 출력만 판정한다. sentinel과 gate는 서명 lane에서 뺐다 | REQ-HR-08, S8 |
| F-001 (rev 3) sentinel과 `-run` 필터 | sentinel을 없애 해당하지 않는다 | REQ-HR-08 |
| HR-F-003 (rev 3) 열거 완전성 | 보장하는 것(기록된 세션의 tamper-evidence)과 보장하지 않는 것(둘 다 지우는 행위자)을 구분했다. CD-HR-1과 A2를 유지한다 | REQ-HR-07 |
| HR-F-004 (rev 3) run 귀속 | run key subject attestation으로 run을 binding에 귀속한다. attempt 시작 시각으로 window를 적용한다 | REQ-HR-06, S6, S7 |
| HR-F-005 (rev 3) wire 필드 | `baseline_commit`, `runner_tree_digest`, stage·termination 관측의 writer와 decoder를 정했다 | Cross-SPEC Changes, Wire Contracts |
| HR3-F-003 (rev 3) 판정 코드 범위 | `runner_tree_digest`에 `pkg/harneval/**`와 oracle harness를 넣었다 | REQ-HR-04 |
| HR-F-013 (rev 3) 복합 실패 | 판정표에 우선순위를 두었고 signer도 같은 순서를 쓴다 | REQ-HR-08, S8 |
| HR-F-015 (rev 3) 키 회전 | `signing_key_id`를 binding에 넣고, 진행 중 run을 취소하는 절차를 두었다 | REQ-HR-04, REQ-HR-05 |
| HR-F-018 (rev 3) v1 호출자 | v1 helper 두 개를 test 전용 파일로 옮기는 작업을 T6에 넣었다 | REQ-HR-05 |
| F-002 (rev 3) S4 검사 순서 | digest 대조를 먼저 하고, 변조 fixture와 정상 attest·의미 오류 fixture를 나눴다 | REQ-HR-02, REQ-HR-09, S4 |
| F-003 (rev 3) release 권한 | evidence job과 release job의 permissions를 정했다 | REQ-HR-06 |
| HR4-F-001, F-004 (rev 4) artifact의 기대 출력 읽기와 process 구조 | sandbox 밖의 runner가 빌드·실행·판정을 형제 process로 차례대로 띄운다. 중첩 sandbox는 로컬에서 `sandbox_apply` EPERM으로 실패해 쓰지 않는다. `artifact.sb`를 읽기 허용 목록 방식으로 바꿔 main checkout(기대 출력 포함), records·control, runner 디렉터리를 읽지 못하게 했다. A3, preflight, S8, S11로 확인한다 | REQ-HR-08, REQ-HR-10, S8, S11 |
| HR-F-005, F-007 (rev 4) 재도출 출처 불일치 | 재도출 출처를 attest된 oracle 결과 bytes와 records bytes 하나로 통일하고, 문서 전체의 raw stream 표현을 정리했다. 판정표에 setup·agent·scope 행과 `oracle` 매핑을 넣었다 | REQ-HR-02, REQ-HR-08, S8 |
| F-005 (rev 4) black-box calibration·vacuity | `calibration.json`의 before·after와 `oracle.ran` 매핑을 정했다 | REQ-HR-08, Cross-SPEC Changes |
| HR-F-003 (rev 4) Outcome Lock 문구와 A2 실패 분기 | (c)를 "어느 한쪽에라도 기록된 세션을 모두 센다"로 고쳤다. A2가 실패하면 세 번째 출처를 두거나 advisory를 유지한다 | Outcome Boundary, REQ-HR-07 |
| HR-F-004 (rev 4) window 기준 둘 | 두 출처 모두 bound log 시각 하나로 window를 정하고 경계 oracle을 넣었다. 재검증 step에도 15분 timeout을 둔다 | REQ-HR-06, S6 |
| HR-F-010 (rev 4) 부분 재실행 귀속 | golden 세션 step 시작 여부로 나누고 귀속 검사를 `run_not_ok`보다 먼저 한다 | REQ-HR-06, S7 |
| HR-F-016 (rev 4) env 이름 불일치 | 읽는 변수와 지우는 변수를 `HARNESS_EVAL_SIGNING_KEY` 하나로 통일하고 S2·S3에서 확인한다 | REQ-HR-01, S2, S3 |
| F-006, HR4-F-003/004 (rev 4) 기대값 digest와 001 소유 변경 | expectation_digest 공식, `floors.signed_agent_tasks`, signal 목록, calibration.json을 Cross-SPEC Changes와 CD-HR-5에 넣었다 | Cross-SPEC Changes |
| F-008 (rev 4) Rekor v2 timestamp | log 시각을 bundle의 검증된 timestamp(v1 integrated time 또는 v2 TSA timestamp)로 정의했다 | Trust Model, REQ-HR-02, REQ-HR-06 |
| F-004 / HR5-F-001 (rev 5) 출력 디렉터리를 통한 confused deputy | 기대 출력을 stdin으로만 넘기고 `oracle.sb`가 기대 출력 파일을 읽지 못하게 했다. 출력은 `os.Root`, 고정 상대 경로, Lstat·SameFile·link 수 1·일반 파일·1 MiB 상한으로만 연다. 남은 process 확인 전에는 oracle을 띄우지 않는다. 로컬 probe로 `os.Root`가 root 안 symlink와 hard link를 막지 못함을 확인해 검사를 함께 두었다 | REQ-HR-08, REQ-HR-10, S8, S11 |
| F-005 (rev 5) calibration bytes 결속 회귀 | `calibration_sha256`을 session-result predicate에 넣고, signer가 digest·session·task 집합·status 일관성을 확인한 bytes만 verdict에 쓴다 | REQ-HR-02, REQ-HR-09, Wire Contracts, S4 |
| HR-F-005 (rev 5) 종료 표기와 표 밖 signal | `agent_termination`에 `launched`를 두고 signal 종료 표기를 정규화했다. `observation_failed` 행을 넣고, black-box trial에는 리터럴 검사와 white-box signal이 생기지 않음을 정했다 | REQ-HR-08, Wire Contracts, S8 |
