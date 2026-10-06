# SPEC-HARNEVAL-003: 서명된 live 증거와 release 차단

**Status**: draft (2026-10-06 사용자 결정으로 SPEC-HARNEVAL-001에서 분리. must-resolve 항목이 Completion Debt로 남아 있다)
**Created**: 2026-10-06
**Domain**: HARNEVAL
**Module**: autopus-adk
**Primary**: SPEC-HARNEVAL-001 rev 4 (golden set, live lane A/B, 격리 grader, calibration, advisory verdict를 소유)
**Sibling 사유**: 보안·컴플라이언스 경계(서명키, protected Environment, release 차단). HARNEVAL의 두 번째이자 마지막 sibling이다.

## 목적

SPEC-HARNEVAL-001의 live lane은 서명하지 않는 advisory report만 낸다. 이 SPEC은 그 세션을 GitHub Actions의 격리된 job에서 실행한다. 보호 Environment의 키로 서명한 `eval_regression_report.v1` + `eval_regression_attestation.v2`를 만들고, release workflow가 이를 기존 strict 경로(`auto check --eval-regression`)로 검증해 회귀가 있으면 release를 막는다. 설계는 001 rev 3 review(3개 provider + judge, 3회)를 거친 것을 옮겼다. 남은 must-resolve 항목 세 가지는 이 SPEC 안에서 끝낸다.

## Outcome Boundary

- Outcome Lock 원문은 `research.md`의 `## Outcome Lock`이다. 요약:
  - (b) live lane이 권한 없는 eval job에서 001의 A/B 세션을 실행한다. 보호 Environment의 signer job이 결과를 데이터로만 검증해 서명하고, 기존 strict verifier가 수정 없이 검증한다.
  - (c) release workflow는 release source에서 계산한 binding digest의 72시간 안 세션을 모두 센다. status, conclusion, attempt와 무관하고, 삭제된 run과 artifact도 포함한다. 검증된 `ok` 증거가 없거나, 하나라도 완료된 `ok`/`incomplete` 증거가 아니면 release를 중단한다.
- Mandatory requirements: REQ-HR-01 ~ REQ-HR-08 (Must).
- Explicit non-goals: Autopus/backend producer와 Autopus gate workflow 변경, unsigned-accept 경로나 verifier 의미 완화, PR마다 live LLM 실행, 운영자 로컬 서명, 001의 결정적 lane·advisory 의미 변경, branch protection·Environment·secret 설정의 자동 수행, live 결과로 통계적 유의성 주장.
- Completion evidence: `acceptance.md`의 Must 시나리오 S1-S9 전부, T10 OPS 증거, Completion Debt CD-HR-1~CD-HR-4 해소. 그 전에는 sync 완료가 아니다.

## Trust Model

- 신뢰: protected branch `main`의 코드(필수 review를 거친 product, runner, parser, signer 코드)와 workflow 정의, 커밋된 manifest·corpus·grader profile·공개키 allowlist, 이전 release tag.
- 비신뢰 실행: agent, 그리고 agent가 고친 코드를 실행하는 grader(001 REQ-HE-08의 sandbox).
- 비신뢰 데이터: eval job artifact(protocol, record), grader 출력, workflow run 목록에서 가져온 증거. 검증 대상일 뿐 실행 대상이 아니다.
- 비신뢰 행위자: write 권한 사용자. run·attempt 취소, 재실행, run·artifact 삭제를 할 수 있다(REQ-HR-06, REQ-HR-07).
- 비밀: Codex credential은 `adk-harness-eval-agent` Environment의 golden step 하나에, 그 안에서도 codex 프로세스 환경에만 둔다. 서명 개인키는 `adk-harness-eval-signing` Environment에만 둔다. 저장소, 로컬 디스크, argv, log, artifact에는 두지 않는다. 한 job이 두 비밀을 함께 받지 않는다.

## Requirements

### REQ-HR-01 Live workflow 신뢰 경계
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL run the signed live lane only through `workflow_dispatch` on main in `[NEW] .github/workflows/harness-eval-live.yml` with a secret-free binding job, an eval job that holds only the Codex credential, and a separate signer job that holds only the signing key, declare no pull request trigger, accept no dispatch inputs, and never interpolate event, input, or artifact values into run scripts.

- job `bind`: `macos-15`, Environment·secret 없음, `permissions: contents: read`. main checkout에서 `auto eval harness digest --binding`을 계산한다. 결과는 `harness-eval-binding-<binding16>` artifact, job summary, REQ-HR-07의 append-only 기록으로 남긴다.
- job `live-eval`: `needs: bind`, `macos-15`, `environment: adk-harness-eval-agent`, `permissions: contents: read`, `persist-credentials: false`. 001 golden 모드를 실행한다. Codex credential은 golden step의 `env:`에만 매핑한다. protocol에는 `run_id`(`GITHUB_RUN_ID`)와 `run_attempt`(`GITHUB_RUN_ATTEMPT`)를 넣는다. protocol, record, `calibration.json`만 unsigned artifact로 올린다.
- job `sign`: `needs: live-eval`, 새 `macos-15` runner, `environment: adk-harness-eval-signing`, `permissions: {contents: read, actions: read}`. 같은 `github.sha`를 checkout해 `auto`를 빌드하고 `auto eval harness export`만 실행한다. 개인키는 secret에서 stdin pipe로만 넘긴다. 차단 여부와 관계없이 완료된 모든 세션을 서명해 올린다.
- 세 job 모두 `if: github.ref == 'refs/heads/main'`이다. dispatch input은 없다. 동적 값은 `env:`로만 넘긴다. `uses:`는 40-hex SHA로 고정한다.
- Environment 보호 규칙(OPS, T10): 두 Environment 모두 deployment branch를 `main`으로 제한한다. `adk-harness-eval-signing`은 required reviewer 1명 이상과 self-review 금지를 둔다. 개인키는 운영자가 일회용 셸에서 만들어 `gh secret set --env`에 stdin으로 넣고 로컬 사본을 지운다.

### REQ-HR-02 Signer의 trusted protocol 재구성
Priority: Must · EARS: EventDriven

WHEN the signer job receives a live result artifact, THEN THE SYSTEM SHALL rebuild the trusted protocol from the main checkout, compare the artifact protocol with it field by field, check that the protocol belongs to the signer's own run and attempt, and validate the records against the trusted order before any verdict is computed.

- trusted 필드: `policy`, `pins`, `model`, `cli_version`(= `pins.codex_cli_version`), `workspace_revision`, `baseline_ref`, `agent_set_digest`, `corpus_digests`, `runner_sha256`, `grader_profile_sha256`, `order`, `candidate_surface_digest`(in-process 생성), `baseline_surface_digest`(baseline ref driver 재빌드), `binding_digest`. 하나라도 다르면 `protocol_mismatch`와 첫 필드 경로(예: `policy.threshold_bp`)로 거부하고 아무것도 쓰지 않는다.
- run 귀속: protocol `run_id`와 `run_attempt`가 signer의 `GITHUB_RUN_ID`, `GITHUB_RUN_ATTEMPT`와 같아야 한다. `started_at`은 그 run의 `created_at`(runs API) 이후이고 signer 시각 +5분 이하여야 한다. 하한을 시각 차이로 두지 않으므로 reviewer 승인 지연(GitHub는 최대 30일 대기)이 서명을 실패시키지 않는다(001 rev 4 review F-003). 증거 나이는 release의 `--eval-regression-max-age 72h`가 `produced_at`(= `started_at`)으로 제한한다. 범위를 벗어나면 `protocol_mismatch`(`run_id`, `run_attempt`, `started_at`)다.
- 정책 검증: `max_agent_runs × trial_timeout_seconds`가 GitHub-hosted job 한도(6시간)에서 준비 시간 30분을 뺀 값을 넘으면 manifest 검증에서 `policy_out_of_range`다.
- record 검증: 모든 record의 `session_id`가 protocol과 같아야 하고, `(task_id, arm, trial)` 집합이 trusted `order`와 정확히 같아야 한다. 어긋나면 `records_protocol_mismatch`다.
- signer는 artifact 안의 어떤 파일도 실행하지 않는다. `golden.py`, Python, `codex`, `go test`, oracle을 실행하지 않는다. 실행하는 코드는 같은 main SHA에서 빌드한 `auto`, 그리고 그것이 trusted 이전 tag에서 빌드하는 baseline driver뿐이다.

### REQ-HR-03 Report 변환과 v2 서명
Priority: Must · EARS: EventDriven

WHEN the trusted protocol and the records are valid, THEN THE SYSTEM SHALL map the SPEC-HARNEVAL-001 verdict into an `eval_regression_report.v1` with every field populated and sign an `eval_regression_attestation.v2` with the Environment key read only from stdin into new mode 0600 files.

- 변환: 001 `verdict`가 `ok`이면 `blocked` false, 그 밖(`regression`, `incomplete`, `vacuous`)이면 true다. report `reason`은 001 `reason`을 그대로 쓴다. `regression_delta`는 001 값이고, `incomplete`와 `vacuous`에서는 0이다. `threshold_value = threshold_bp / 10000`이다.
- report 값: `comparison_scope=adk-harness-golden-live`, `threshold_metric=pass_rate_delta`, `attributed_version`=binding digest, `baseline_ref`=trusted 값, `workspace_scope=autopus-adk`, `redaction_status=body_free`, `retention_class=release_evidence`, `raw_payload_present=false`, `produced_at`=protocol `started_at`(UTC 초, RFC3339 `Z`). report와 attestation에 같은 문자열을 쓴다(probe A1: 1초 차이는 `attestation_policy_mismatch`).
- attestation: `key_id`=`[NEW] evalregression.ADKHarnessEvalKeyID`, `trust_lane`=`[NEW] evalregression.ADKHarnessEvalTrustLane`(`adk-harness-eval`), `source_environment=adk-harness-live`, `target_environment=adk-release`, `source_revision`=binding digest, `workspace_scope=autopus-adk`. 서명은 `[NEW] pkg/evalregression/sign_v2.go::SignEvalRegressionAttestationV2`가 unexported `evalRegressionAttestationV2Message`를 그대로 써서 만든다.
- 키 입력은 `readPrivateKey`(`internal/cli/companion_manifest.go:167`) 방식의 stdin 전용 64-byte ed25519 개인키다. 출력은 O_EXCL로 만들고, 이미 있으면 `output_exists`다.

### REQ-HR-04 Binding digest
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL compute one binding digest from the candidate default surface digest, the agent set digest, the corpus file digests, the runner and grader profile digests, the live policy, the workspace revision, the baseline ref, and the pinned generator and CLI versions, and use that digest as both `source_revision` and `attributed_version`.

- 정의: SHA-256 hex(Go `json.Marshal` of `harness_eval_binding.v1{candidate_surface_digest, agent_set_digest, corpus_digests[{file, sha256}], runner_sha256, grader_profile_sha256, policy{k, threshold_bp, completeness_floor, max_agent_runs, trial_timeout_seconds}, workspace_revision, baseline_ref, pins{generator_version, project_name, codex_cli_version, opencode_cli_version, codex_model_catalog_sha256}}`). `agent_set_digest`는 001 expectation digest를 통해 `expected_tests`를 담는다. runner와 grader profile을 넣어 채점·격리 코드가 바뀌면 이전 증거가 무효가 된다(001 rev 4 review F-004).
- 64-hex는 `attributed_version`의 `gitSHAShape`를 통과한다(probe A1). 비기본 variant는 release의 `ci` workflow_call에서 도는 결정적 lane이 막으므로 binding에 넣지 않는다.
- binding digest를 계산하는 job(bind, signer, release evidence)은 모두 `macos-15`에서 돈다.

### REQ-HR-05 Trust lane 분리
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL add only the public key of the `adk-harness-eval` lane to `evalRegressionPublicKeys`, keep the lane key id and trust lane as exported constants, add no unsigned-accept path, and prove by test that harness-lane evidence fails the Autopus lane policy and Autopus-lane evidence fails the harness lane policy with `attestation_policy_mismatch`.

- Autopus lane 값은 runbook이 아니라 `../Autopus/.github/workflows/eval-regression-gate.yml:201-203`에 있다: `staging-to-main`, `staging`, `production`, key `autopus-eval-staging-to-main-2026-07`. 양방향 거부는 key_id가 달라서 생기므로 Autopus 값이 바뀌어도 성립한다(probe A1).
- runbook에 adk-harness lane 절(키 회전: 새 공개키 추가 → 상수 교체 → Environment secret 교체 → 다음 release 뒤 이전 키 제거)을 추가하고 `TestEvalRegressionRequiredCheckRunbookExists`의 문자열은 보존한다. `TestEvalRegressionADKWorkflowIsRetired`는 수정 없이 통과한다.

### REQ-HR-06 Release 차단 연결
Priority: Must · EARS: EventDriven

WHEN the release workflow runs, THEN THE SYSTEM SHALL run a `harness-eval-evidence` job listed in the `release` job `needs` that recomputes the binding digest from the release source, enumerates every attempt of every `harness-eval-live.yml` run on main created within 72 hours regardless of status or conclusion through a complete paginated listing, and stops the release unless at least one attempt for that binding has verified `ok` evidence and every attempt for that binding is completed with verified `ok` or `incomplete` evidence.

- run 목록: `gh api --paginate "repos/{owner}/{repo}/actions/workflows/harness-eval-live.yml/runs?branch=main&event=workflow_dispatch&created=>=<window_start>&per_page=100"`. 모은 수가 `total_count`와 다르거나 `total_count`가 1000을 넘으면 `run_list_truncated`다. 각 run은 1부터 `run_attempt`까지 `GET …/actions/runs/{id}/attempts/{n}`으로 모든 attempt를 따로 판정한다. 재실행으로 attempt 1이 가려지지 않는다.
- attempt의 binding은 그 attempt의 bind 기록(REQ-HR-07)으로 정한다. binding이 release binding과 같은 attempt가 완료되지 않았으면 `run_in_progress`다. conclusion이 `success`가 아니면(`cancelled`, `failure`, `timed_out`, `action_required`, `skipped`, `stale`, `startup_failure`, `neutral`, 승인 거부 포함) `run_not_ok`다.
- success attempt의 서명 증거는 `auto check --eval-regression`으로 검증한다. 여섯 expected flag와 `--eval-regression-max-age 72h`를 쓰고 `--warn-only`는 쓰지 않는다. `ok`이면 통과다. 검증된 report `reason`이 `incomplete`이면 세지 않는다. 그 밖은 멈춘다. 셀 `ok`가 없으면 `artifact_missing`이다.
- 72시간 window와 그에 따른 flaky 차단은 사용자가 2026-10-06에 수락했다(answered). job은 `permissions: {contents: read, actions: read}`, `macos-15`다.
- expected 값: key id와 trust lane은 REQ-HR-05 상수, 환경과 workspace scope는 REQ-HR-03 상수, source revision은 `auto eval harness digest --binding`이다. manifest `live.baseline_ref`는 release commit의 엄격한 조상인 직전 release tag여야 한다(아니면 `baseline_ref_invalid`).
- `needs`는 `[ci, security, omp-production-evidence, harness-eval-evidence]`가 되고 기존 세 항목의 의미는 유지한다. 증거가 72시간을 넘기면 live workflow를 다시 dispatch하고 release workflow를 다시 실행한다.

### REQ-HR-07 Append-only 세션 기록 (must-resolve)
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL record every bound session in an append-only session log that repository writers cannot rewrite or delete, and count every logged session for the release binding even after its run, attempt, or artifacts are deleted.

- 이유: write 권한으로 workflow artifact와 run을 지울 수 있다. runs 목록만 세면 불리한 세션을 지워 모집단에서 빼는 best-of-N이 남는다(001 rev 3 review A-F-017).
- 설계 선택지(probe A2로 결정, CD-HR-1):
  - (1) GitHub artifact attestation(`actions/attest-build-provenance`). subject는 binding digest 파일, predicate는 run id·attempt·event다. release는 `GET /repos/{o}/{r}/attestations/{subject_digest}`로 그 binding의 세션을 모두 센다. 확인할 점은 private repo에서 write 권한으로 attestation을 지울 수 없는지다.
  - (2) Sigstore 공개 transparency log(Rekor) 기록. 지울 수 없지만, 공개 log에 binding digest와 run id가 노출된다.
  - (3) bot만 push할 수 있는 protected branch의 append-only binding log. ruleset으로 force-push와 삭제를 금지하고 entry마다 서명한다. 서명키를 하나 더 관리해야 한다.
- release 판정: log에 있는데 `ok`/`incomplete`로 검증된 증거가 없는 세션은 `run_not_ok`다. log를 읽을 수 없으면 release를 멈춘다(`session_log_unavailable`).

### REQ-HR-08 Oracle 결과 진위 (must-resolve)
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL reject before grading every agent diff that adds a package-level initializer, a reference to `os.Exit`, `syscall.Exit`, `runtime.Goexit`, or `unsafe`, a `//go:linkname` directive, or a new import of `syscall`, `unsafe`, `runtime`, `reflect`, `plugin`, or `os/exec` resolved through the type-checked Go source under any import alias, and fail every trial whose grader output carries both pass and fail events for an expected test.

- 이유: 001의 리터럴 `forbidden_construct`는 alias import(`import s "syscall"`)와 `var _ = f()` 초기화로 우회된다. 또 같은 process의 코드가 test2json framing을 찍고 exit 0으로 끝낼 수 있다(001 rev 3 review A-F-001). 차단 lane에서는 위조된 `ok`가 release를 연다.
- 설계: `go/parser`와 `go/types`로 agent 전·후 허용 파일을 비교한다. 새 package-level `var` 초기화식의 함수 호출, 새 `init`, 금지 selector 참조(alias·dot import 해석 포함), 금지 import 추가를 찾는다. 찾으면 `forbidden_construct`와 위치로 fail한다. parser 규칙은 둘이다. 한 expected test에 pass와 fail이 함께 있으면 fail이다. 종료 코드가 0이 아니면 pass 이벤트와 관계없이 fail이다.
- 남는 한계: process 종료 없이 framing만 위조한 뒤 실제 실패로 끝나는 경우는 fail 이벤트로 잡힌다. 종료 없이 `ok`를 만들려면 실제 test가 통과해야 한다. 완전한 해소인지는 probe A3와 CD-HR-2·CD-HR-3에서 판정한다.

## Wire Contracts

| 문서 | 필드 |
|------|------|
| protocol 추가 필드 | `run_id`, `run_attempt`, `binding_digest` (001 `harness_golden_live_protocol.v1`에 이 SPEC이 더한다) |
| `harness_eval_binding.v1` | REQ-HR-04 정의 |
| `eval_regression_report.v1`, `eval_regression_attestation.v2` | 기존 `pkg/evalregression` schema 그대로. 값은 REQ-HR-03 |
| session log entry | `binding_digest`, `run_id`, `run_attempt`, `event: bound\|signed`, `created_at`. 저장 매체는 REQ-HR-07 선택지 |
| artifact 이름 | `harness-eval-binding-<binding16>`, `harness-eval-unsigned-<run_id>-<attempt>`, `harness-eval-evidence-<binding16>-<run_id>-<attempt>` |

## Existing Test Changes

바꾸는 기존 test는 아래 세 단언뿐이다. 근거로 release.yaml을 읽는 test 9개, ci.yaml을 읽는 test 4개, allowlist를 읽는 test를 grep했다.

| 파일 | 위치 | 지금 | 바뀐 뒤 |
|------|------|------|---------|
| `pkg/evalregression/verify_e2e_test.go` | L120 `len(keys) != 1` | allowlist 길이 1 | key id 집합이 `{autopus-eval-staging-to-main-2026-07, ADKHarnessEvalKeyID}`와 같음 |
| `pkg/evalregression/verify_e2e_test.go` | L138 `len(again) != 1` | 방어 사본 재조회 길이 1 | 반환 map을 바꾼 뒤 다시 조회한 key id 집합이 위 두 개와 같음 |
| `internal/companionmanifest/release_contract_test.go` | L15 `TestReleaseWorkflow_ExactA34ProtectedNormalLane` | 리터럴 `needs: [ci, security, omp-production-evidence]` | 리터럴 `needs: [ci, security, omp-production-evidence, harness-eval-evidence]` |

## 생성 파일 상세

- `[NEW] pkg/evalregression/sign_v2.go`와 상수 `ADKHarnessEvalKeyID`, `ADKHarnessEvalTrustLane`. `pkg/evalregression/attestation.go`에는 공개키 한 항목과 package doc만 추가한다.
- `[NEW] pkg/harneval/binding.go`, `protocol_trust.go`, `report_v1.go`, `[NEW] pkg/harneval/astgate/`(REQ-HR-08). `[NEW] internal/cli/eval_harness_export.go`, `eval_harness_policy.go`(`auto eval harness export|policy`, `digest --binding`).
- `[NEW] .github/workflows/harness-eval-live.yml`, `.github/workflows/release.yaml`(`harness-eval-evidence` job과 `needs`), `.github/EVAL_REGRESSION_REQUIRED_CHECK.md`(lane 절).
- `[NEW] internal/cli/eval_harness_lane_test.go`, `eval_harness_live_workflow_test.go`, `eval_harness_e2e_test.go`. 기존 test 변경은 위 세 단언뿐이다.

## Related SPECs

- SPEC-HARNEVAL-001 rev 4 (Primary): T10(verdict·advisory), T11(grader 준비·calibration), T12(golden runner), T13(surface driver)에 의존한다. 001의 advisory 의미(서명하지 않음)는 바꾸지 않는다. 이 SPEC은 같은 세션 데이터에 서명 경로를 더한다.
- SPEC-HARNEVAL-002: 의존 관계가 없다.
- Sibling 한도: HARNEVAL sibling은 002와 이 SPEC 둘로 끝난다. 이 SPEC은 sibling을 만들지 않는다.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-HR-01 | T5, T10 | S3 | INV-HR-03 |
| REQ-HR-02 | T2 | S4 | INV-HR-04 |
| REQ-HR-03 | T1, T3 | S1, S2 | INV-HR-01, INV-HR-02 |
| REQ-HR-04 | T4, T11 | S5 | INV-HR-05 |
| REQ-HR-05 | T6, T10 | S1, S9 | INV-HR-01 |
| REQ-HR-06 | T7, T10 | S6 | INV-HR-06 |
| REQ-HR-07 | T8 | S7 | INV-HR-07 |
| REQ-HR-08 | T9 | S8 | INV-HR-08 |
| Brownfield | T6, T7 | S9 | - |
