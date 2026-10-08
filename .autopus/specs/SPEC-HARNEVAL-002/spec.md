# SPEC-HARNEVAL-002: Learning·incident를 golden-task 후보로 승격

**Status**: implemented
**Created**: 2026-10-06
**Domain**: HARNEVAL
**Module**: autopus-adk
**Primary**: SPEC-HARNEVAL-001 rev 3 (`harness_golden_task.v1` 형식, strict loader, manifest, baseline, quarantine 경로 거부를 소유)
**Source PRD**: `../SPEC-HARNEVAL-001/prd.md` `## Sibling SPEC Decision`

## 목적

사고(incident)와 교훈(learning)은 지금 `.autopus/learnings/pipeline.jsonl`(tracked)에 자유 문구로만 남는다. `LearningEntry`(`pkg/learn/types.go`)에는 expected/actual, 재현 명령, fingerprint가 없고, `learn.Prune`은 나이만 보고 지운다. 그래서 사고가 영구 eval이 되지 못한다. 이 SPEC은 learning entry를 quarantine된 golden-task 후보로 만든다. 사람이 assertion을 작성해 명시적으로 승격하거나 거절하고, 승격·연결된 근거 entry는 prune에서 지우지 않는다.

## Outcome Boundary

- Outcome Lock 원문은 `research.md`의 `## Outcome Lock`이다. 요약은 다음 순서다. 기록할 때 secret을 가린다. intake가 fingerprint로 묶은 후보를 만든다. 사람이 `category`와 assertion을 쓴다. promote로 active golden set에 넣고 영구 link record를 남긴 뒤 SPEC-HARNEVAL-001 baseline 갱신으로 고정한다. 그룹의 모든 근거 entry는 manifest 유무와 관계없이 prune에서 보존된다. 거절한 후보는 다시 만들어지지 않는다.
- Mandatory requirements: REQ-HC-01 ~ REQ-HC-08, REQ-HC-10, REQ-HC-11 (Must). REQ-HC-09는 Should.
- Explicit non-goals: 자동 승격(`--all`, `--auto` 없음), 자유 문구에서 assertion 자동 생성(LLM 포함), agent kind 사고 task(`--kind agent`는 거부), `repro` 실행, SPEC-HARNEVAL-001 schema·runner·baseline 의미 변경, 이 SPEC 이전 binary의 동작 변경, 기존 learn store가 보장하지 않는 프로세스 간 동시성 보장, Autopus/backend 변경.
- Completion evidence: `acceptance.md`의 Must 시나리오 S1-S8, S10, S11 전부, 그리고 SPEC-HARNEVAL-001 선행 task merge(Completion Debt CD-HC-1).

## Trust Model

- 신뢰: main의 코드, commit된 manifest와 task.
- 비신뢰 입력: 운영자가 넣는 learning 문구와 flag 값, 손으로 고친 store와 intake 파일. 모두 secret이나 제어 문자를 담을 수 있다.
- 첫 쓰기 경계: learning store writer는 free-text field 전부를 `[NEW] pkg/secretscan.Redact`로 가린 뒤에만 bytes를 쓴다. CLI가 출력하는 문구도 같은 함수로 가린다. `/auto fix` template의 store 직접 append 지시(`templates/claude/commands/auto-workflows.md.tmpl:2359`)는 이 SPEC이 없앤다. 그러면 raw 문구는 프로세스 메모리에만 남는다. 운영자 셸 history에 남는 flag 값은 이 SPEC 밖이며 README에 적는다.
- intake 영역: `evals/harness/candidates/` 아래에 열린 후보, `promoted/` link record, `rejected/` 거절 record를 둔다. SPEC-HARNEVAL-001 REQ-HE-01이 이 경로를 active path로 선언하는 것을 막으므로 평가 대상이 되지 않는다. 모든 파일은 가려진 데이터로만 쓰는 tracked 0644 파일이다. 경로 규칙은 REQ-HC-11이다.
- `repro`는 데이터일 뿐이다. 이 SPEC의 어떤 코드도 실행하거나 shell 확장하지 않는다.

## Requirements

### REQ-HC-01 Learning schema 확장과 첫 쓰기 redaction
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL add optional `expected`, `actual`, and `repro` string fields with `omitempty` to `LearningEntry`, accept them through `auto learn record --expected --actual --repro`, reject a value with a control character or over its byte cap with `learning_field_invalid`, and redact every free-text field with `pkg/secretscan.Redact` inside the store writer and in every CLI echo before any byte is persisted or printed.

- cap과 순서: `expected`, `actual` 각 1024 byte, `repro` 512 byte다. 검사 순서는 다섯 단계로 고정한다. (1) raw 값에 제어 문자(C0·개행·DEL·C1)가 있으면 거부한다. (2) raw 길이가 cap의 4배를 넘으면 가리기 전에 거부한다. (3) `Redact`를 적용한다. (4) 가린 값이 cap을 넘으면 거부한다(placeholder가 원문보다 길 수 있으므로 cap은 가린 값에 적용한다). (5) 저장한다. 거부 reason은 모두 `learning_field_invalid`이고 detail은 `control_char`, `raw_over_limit`, `over_cap_after_redaction`이다. 그래서 저장된 값은 항상 cap 안이다. 값이 없는 entry는 key 없이 직렬화된다.
- detector(span 병합 union): `[NEW] pkg/secretscan.Redact(s) (string, bool)`는 원문 하나에 detector를 모두 실행한다. 단계는 다음과 같다.
  - detector 표: `pkg/qa/evidence/redaction.go`의 정규식 14개(secret 5, 할당, flag, JSON key, credential URL, query, private note 2, Unix·Windows 사용자 경로)와 `pkg/worker/security` 기본 패턴 11개(`sk-[a-zA-Z0-9]{20,}`, `AKIA[A-Z0-9]{16}`, `ghp_`/`gho_` 36자, `Bearer [a-zA-Z0-9._\-]+`, 일반 할당 `(?i)(password|secret|api_key|apikey|token)\s*[=:]\s*\S+`, AWS 문맥, `"private_key…"` JSON, azure, PEM 머리줄, `apjwt_`)다.
  - span: 각 match에서 secret 부분을 고른다. qa detector는 `RedactText`가 바꾸는 group(Bearer 뒤 값, 할당·flag·JSON의 값, URL userinfo, query 값, note 값, 사용자명)을 쓰고, worker 패턴은 match 전체를 쓴다. qa의 prose 예외는 적용하지 않는다(더 엄격). placeholder와 겹친다는 이유로 span을 버리지 않는다. 대신 span 경계를 겹친 placeholder token의 경계까지 넓힌 뒤 병합하고, 병합 span 전체를 다시 가린다. 그 안에 있던 placeholder도 함께 덮인다. 넓힌 span의 text가 canonical placeholder 하나와 정확히 같을 때만 그 span을 건너뛴다. 이 예외 덕분에 다시 적용해도 결과가 같다. 그래서 사용자가 넣은 placeholder가 같은 match나 인접 match의 원문 secret을 가려 주지 못한다(S11).
  - 병합: span을 시작 위치로 정렬하고, 겹치거나 맞닿으면 합친다. 합친 span의 종류는 우선순위 secret > private note > user로 정한다.
  - 치환: 합친 span마다 한 번만 `[REDACTED_SECRET]`, `[REDACTED_PRIVATE_NOTE]`, `[REDACTED_USER]`로 바꾼다.
  - neutral copy와 고정점(rev 7): detector는 canonical placeholder를 같은 길이 filler `#`로 바꾼 사본을 훑는다. placeholder는 불투명한 값이라 붙은 원문은 그 값에 합쳐지고, placeholder 글자(`SECRET`)는 keyword가 아니다. 원문 자체의 span은 placeholder와 겹치지 않거나 사본의 span과도 겹칠 때만 더한다(S11의 `[REDACTED_SECRET]; aws <40>`). 한 pass가 아무것도 바꾸지 않을 때까지 반복하므로 `Redact(Redact(x)) == Redact(x)`다.
  - 겹친 match와 replay(rev 7): detector마다 match의 선택 부분 시작(전체 선택이면 시작+1)에서 다시 찾아 값이 삼킨 다음 key의 match도 모은다. 중첩 검색은 detector당 text의 8배+4096 byte까지만 훑고, 넘으면 그 지점부터 끝까지를 secret 하나로 가린다. `RedactText`와 worker `Scan`을 위치를 추적하며 다시 돌려 각 치환이 지운 원문 범위도 가린다. PEM 머리줄 span은 footer 끝까지, footer가 없으면 뒤의 base64 본문까지다.
- 순차 적용(rev 4)은 앞 detector가 바꾼 문맥 때문에 뒤 detector가 원문 secret을 놓쳤다. 예를 들어 `see /Users/aws/` + 40자는 사용자명만 가려지고 40자 값이 남았다(실행 확인). 원문 기준 span 병합은 이를 `see /Users/[REDACTED_SECRET] end`로 완전히 가린다(probe A1).
- 성질과 drift test:
  - `Redact`는 같은 입력에 같은 결과를 낸다.
  - 출력에 detector를 다시 돌리면(pass가 neutral copy 기준으로 모으는 span) placeholder 밖에는 span이 없다.
  - 원문 secret span에서 placeholder token을 뺀 부분의 8자 이상 부분 문자열은 출력에 남지 않는다.
  - test에서만 두 package를 import한다. `SecretDetectorSources()`, `DefaultPatternSources()`가 돌려주는 정규식 출처와 `pkg/secretscan` 표가 정확히 같은지 검사하므로, 어느 detector가 바뀌어도 test가 실패한다.
- redaction 위치: `pkg/learn/store.go`의 쓰기 함수(`Append`와 `AppendAtomic`이 거치는 경로). 대상 free-text field는 `pattern`, `resolution`, `phase`, `spec_id`, `expected`, `actual`, `repro`다. 받은 그대로 저장하는 field는 거부만 한다(rev 7). `severity`는 빈 값, `low`, `medium`, `high`, `critical`만 받고(detail `unknown_value`), fingerprint 입력인 `files`·`packages`는 `Redact`가 바꿀 항목이 있으면 거부한다(detail `needs_redaction`). store 파일이 symlink이거나 regular file이 아니면 append와 rewrite 모두 거부한다. Go writer는 `record.go`의 `AppendAtomic` 하나뿐이므로 CLI와 `pkg/pipeline/learn_hook.go` 모두 이 경계를 지난다. `rewriteStore`(`Prune`, `UpdateReuseCount`가 공유)는 파싱된 entry를 지금처럼 canonical `MarshalJSON`으로 다시 encode한다. 다만 redaction을 다시 적용하지 않는다. 문자열 값과 `null`·`[]`는 round-trip에서 그대로이므로 저장 값과 fingerprint 입력은 prune 뒤에도 변하지 않는다. 시간대 표기 같은 기존 canonical 정규화(SPEC-ADK-EVIDENCE-LOOPS-001 계약)와 `reuse_count` 증가는 지금처럼 일어난다. 이 SPEC 이전 binary가 쓴 legacy raw entry도 다시 가리지 않으므로 원문과 fingerprint가 그대로 남는다(S2). 그 원문은 intake가 후보로 복사할 때만 가려진다. tracked store에 남은 legacy 원문은 수동 정리 대상이며 잔여 위험으로 둔다. 파싱되지 않은 줄(`SkipRecord`)은 경계를 지난 적이 없으므로 원문 줄 전체에 `Redact`를 적용해 다시 쓴다.
- CLI 출력: `auto learn record`의 `Recorded %s entry: %s`(`learn_record.go:68`)는 원문이 아니라 저장된(가려진) pattern을 출력하고, 제어 문자는 `\uXXXX`로 escape한다(rev 7). 기존 test는 접두사 `Recorded <type> entry`만 단언한다.
- template(canonical source는 `templates/claude/commands/auto-workflows.md.tmpl`이고 `content/`에는 이 문구가 없다). 두 곳을 고친다.
  - L2359: "append directly to `.autopus/learnings/pipeline.jsonl`" fallback을 "`auto learn record` 실패를 보고하고 store를 직접 쓰지 않는다"로 바꾼다.
  - Sync Target 4.5 (L2671-2674): 항상 실패하는 `auto learn prune --max-age 90d`(`IntVar`라 `90d`는 parse 오류)를 `auto learn prune --days 90`으로 바꾼다. "read the file directly and remove entries older than 90 days" fallback은 "prune 실패(`eval_links_unreadable` 포함)를 보고하고 store를 고치지 않는다"로 바꾼다.
  - 생성 표면은 직접 고치지 않고 재생성한다. static test는 template·content 전체에 대해 두 가지를 단언한다. store를 직접 append·삭제·수정하라는 지시가 0건이고, 모든 `auto learn prune` 호출이 정수 `--days N`을 쓴다.
- 오탐 영향: union이므로 worker의 넓은 패턴 때문에 `Bearer token header …`가 `[REDACTED_SECRET] header …`로, `session: expired` 같은 key 형식의 일반 문구도 가려진다. coverage를 잃지 않는 쪽을 택해 오탐을 받아들인다. 지금 store의 3건은 qa detector로 바뀌지 않았다(probe A1). worker 패턴 적용분은 T2 test에서 다시 확인한다.

### REQ-HC-02 Learning fingerprint
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL compute a learning fingerprint as the lowercase hex SHA-256 of the Go `json.Marshal` of a fixed-order struct `{v, type, pattern, files, packages}` with `v` set to 1, the stored pattern lowercased by `strings.ToLower` and whitespace-collapsed by `strings.Fields`, files trimmed, empty-dropped, backslash-converted, `path.Clean`ed, `.`-dropped, deduplicated, and byte-sorted, and packages trimmed, empty-dropped, deduplicated, and byte-sorted.

- 순서가 정의다. 빈 항목을 `path.Clean` 전에 버린다(`path.Clean("")`은 `"."`다). files도 trim한다(`--files a.go, b.go`의 앞 공백). struct이므로 key 순서가 고정된다. 정규화 함수는 항상 non-nil slice(`make([]string, 0)`)를 돌려준다. 그래서 `null`, 빈 목록, 항목이 모두 제거된 목록이 모두 `[]`로 encode되고 `null`은 나오지 않는다(S2 vector).
- 제외 필드: `id`, `timestamp`, `phase`, `spec_id`, `severity`, `resolution`, `reuse_count`, `expected`, `actual`, `repro`.
- `Fingerprint`는 detector를 부르지 않는다. 입력은 store에 저장된 값 그대로다. 새 entry는 쓰기 시점에 이미 가려져 있고, prune rewrite는 파싱된 entry를 다시 가리지 않는다. 그래서 detector 패턴이 바뀌거나 prune이 돌아도 기존 entry의 fingerprint는 변하지 않는다(S2). 패턴이 바뀐 뒤 같은 사고를 새로 기록하면 문구가 달라져 다른 fingerprint가 될 수 있다. 이 경우 중복 후보가 생기고 사람이 reject한다(잔여 위험). 이전 entry의 raw 문구는 hash로만 쓰이고 출력에 나오지 않는다. files·packages는 가리지 않는다.
- 알고리즘을 바꾸면 `v`를 올린다. 후보·link·거절 record는 `fingerprint_version`을 저장하고, 중복 판정은 (version, fingerprint) 쌍으로 한다.
- 위치: `[NEW] pkg/harneval/intake/fingerprint.go::Fingerprint`.

### REQ-HC-03 후보 intake, grouping, 대표 entry
Priority: Must · EARS: EventDriven

WHEN `auto eval harness intake` runs with exactly one of `--learning <ids>` or `--all-eligible`, THEN THE SYSTEM SHALL process the selected entries in ascending numeric id order, group entries with equal fingerprints, and create one `harness_golden_candidate.v1` file per new fingerprint whose representative is the lowest-id entry of the group and whose evidence fields come only from that representative.

- eligible(`--all-eligible`): `expected`와 `actual`이 비어 있지 않고, entry의 fingerprint가 열린 후보, promoted link, 거절 record, active incident task provenance 어디에도 없어야 한다. id는 eligible 판정에 쓰지 않는다. `Store.NextID`(`store.go:110-126`)는 prune 뒤 최대 id를 다시 쓰므로, id만 같고 fingerprint가 다른 entry는 다른 사고다(S4). 후보와 link가 참조하는 entry는 prune 보호를 받으므로 그 id는 다시 쓰이지 않는다.
- 대표 규칙: 후보의 `expected`, `actual`, `repro`, `task.intent`(pattern), `task.outcome`(expected)은 대표 entry에서만 가져온다. 다른 member 값과 섞지 않는다. 대표에 `repro`가 없으면 빈 값이다. `learning_refs`는 그룹의 모든 id를 숫자 순서로 담는다. intake는 store 값을 REQ-HC-01과 같은 순서로 다시 검증하고 다시 가린다. 실패한 entry의 행 결과는 `skipped`(`learning_field_invalid`)이고 종료 코드는 2다. `redacted_fields`는 이때 바뀐 대표 field 이름의 byte 순서 목록이고, `redacted`는 그 목록이 비지 않았는지다.
- 명시한 id에 expected/actual이 없으면 행 결과는 `skipped`(`learning_missing_expected_actual`)다. `--expected`와 `--actual`은 함께, 그리고 `--learning` id가 정확히 하나일 때만 쓸 수 있다(`flag_pair_required`, `flag_requires_single_learning`). flag 값은 REQ-HC-01과 같은 순서의 검사와 redaction을 거친다. 실패하면 실행 단위 오류 `learning_field_invalid`(종료 코드 1, 쓰기 없음)다. learning entry는 바꾸지 않는다. 없는 id는 `learning_not_found`다. `--kind`는 `surface`만 받는다(`kind_unsupported`).
- 초안 task는 SPEC-HARNEVAL-001 `harness_golden_task.v1`의 모든 필드를 갖는다: `schema_version`, `id` `GT-INC-<fingerprint 앞 8 hex 대문자>`, `kind` `surface`, `category` `""`, `intent`, `outcome`, `variants` `[]`, `assertions` `[]`, `provenance{kind: incident, ref: 대표 id, fingerprint}`, `status{state: active, reason: ""}`. 사람이 채우는 필드는 `category`(001 enum)와 `assertions`(≥1)이고 `variants`는 선택이다.
- 파일: `evals/harness/candidates/GTC-<fingerprint 앞 12 hex>.json`. encoding은 `json.MarshalIndent`(공백 2칸)와 끝 LF이고, struct 필드 순서를 따른다. timestamp가 없어서 같은 입력은 같은 bytes를 낸다. 쓰기는 REQ-HC-11을 따른다.
- 출력과 종료 코드: `--format json`은 stdout에 `harness_intake_result.v1` 하나만 쓰고 안내는 stderr에 쓴다. 행 결과는 `created`, `grouped`, `duplicate_candidate`, `already_promoted`, `already_rejected`, `skipped`다. 모든 행이 `skipped` 밖이면 0, `skipped` 행이 하나라도 있으면 2(성공 행은 유지), flag 오용이나 읽기 실패 같은 실행 단위 오류는 쓰기 없이 1이다.

### REQ-HC-04 Fingerprint 중복 제거
Priority: Must · EARS: Unwanted

IF a selected entry's fingerprint matches an open candidate, a promoted link record, a rejected record, or the provenance fingerprint of an active golden task, THEN THE SYSTEM SHALL write no file for that entry, report `duplicate_candidate`, `already_promoted`, or `already_rejected` with the matching id, and leave every existing file byte-identical.

- intake는 기존 파일을 수정하지 않는다. 열린 후보는 사람이 편집 중일 수 있다. 따라서 그룹이 만들어진 뒤에 기록된 중복 entry는 그룹에 연결되지 않고 prune 보호도 받지 않는다. `--all-eligible`은 이런 entry를 조용히 건너뛰고, 명시한 `--learning`일 때만 보고한다.
- 같은 `GTC-<12 hex>` 파일이 있는데 fingerprint가 다르면 그 행은 `skipped`(`candidate_id_collision`)이고 아무것도 쓰지 않는다.

### REQ-HC-05 신뢰할 수 없는 문구와 repro
Priority: Must · EARS: Unwanted

IF a pattern contains a control character other than newline or tab or exceeds 4096 bytes, THEN THE SYSTEM SHALL skip that entry with `candidate_text_invalid` and never execute or shell-expand any `repro` value in intake, promote, reject, or prune.

- secret은 거부하지 않고 가린다(REQ-HC-01, REQ-HC-03). key 형식의 일반 문구도 탐지될 수 있으므로, 거부하면 정상 교훈이 막힌다.
- JSON 출력은 id, fingerprint, reason, 후보 id만 담고 learning 문구를 담지 않는다.

### REQ-HC-11 Intake 경로 confinement
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL accept only candidate ids matching `^GTC-[0-9a-f]{12}$` and task ids matching the SPEC-HARNEVAL-001 id grammar, require every path component from the project root to the intake area and to the active task directory to be a real directory and every read or deleted leaf to be a regular file, perform every intake-area operation through an `os.Root` opened at the project root with `O_EXCL` creation, refuse intake, promote, and reject writes on Windows with `platform_unsupported`, and refuse any other case with `candidate_id_invalid` or `path_unsafe`.

- 인자는 형식 검사 뒤에만 경로에 합친다. 후보 파일 안의 `id`는 파일 이름과 같아야 한다(`candidate_invalid`, detail `id_mismatch`).
- 검사 대상은 `evals`, `evals/harness`, `evals/harness/candidates`, 그 아래 `promoted`, `rejected`, 그리고 `evals/harness/tasks/surface`의 각 구성 요소다. `Root.Lstat`로 symlink와 regular file·directory가 아닌 항목(Windows junction 포함)을 거부한다. 검사와 열기 사이의 경쟁은 운영자 로컬 환경의 잔여 위험으로 둔다. `os.Root`는 repo 선례(`pkg/adapter/codex/codex_hooks_io.go:20`, `pkg/companionmanifest/signed_pair_io.go:43`)이며, 그 사이에 바뀐 symlink로 root 밖에 닿는 것을 막는다.
- 게시 API: intake 영역의 생성·link·삭제는 모두 `os.Root` 메서드(`OpenFile`, `Link`, `Remove`, `Mkdir`, `Lstat`)로 한다. 그래서 검사 뒤 상위 디렉터리가 symlink로 바뀌어도 root 밖에 파일이 생기거나 지워지지 않는다(001 rev 3 review HC-003).
- 플랫폼: 쓰기 경로는 `safepath_unix.go`(`//go:build !windows`, 디렉터리 fsync 포함)에 두고, `safepath_windows.go`는 `platform_unsupported`를 반환한다. 읽기 전용 prune 보호 scan은 모든 플랫폼에서 `os.Root`와 `Lstat`로 동작한다. ci.yaml을 바꿔(T13) 두 곳에서 확인한다. ubuntu `test` job에서는 `GOOS=windows GOARCH=amd64 go vet ./pkg/harneval/intake/`가 windows test까지 컴파일한다. 기존 `windows-runtime` job에서는 그 job의 기존 step 규약을 따라 실행한다. 먼저 `go test -list 'PlatformUnsupported' ./pkg/harneval/intake/`의 test 수가 floor 1 이상인지 확인하고, `-v`로 실행한 뒤 PASS한 test 집합이 목록과 같은지 비교한다. 그래서 test 이름이 바뀌거나 빠졌을 때 'no tests to run'으로 통과하지 않는다. 이 규약은 `[NEW] internal/cli/eval_harness_intake_workflow_test.go`가 단언한다. 새 step은 action을 추가하지 않고, `version: latest`를 쓰지 않으며, omp-native-smoke 구간 밖에 있다. `TestWindowsRuntimeRunsSelfUpdateAdmissionContracts`는 `Contains` 단언이라 영향이 없다. golden set 관리는 macOS·Linux maintainer 작업이다.

### REQ-HC-06 사람 승인 승격
Priority: Must · EARS: EventDriven

WHEN `auto eval harness promote <candidate-id>` runs, THEN THE SYSTEM SHALL apply the ordered promote checks, publish the task file, roll back only that task file and keep the candidate untouched when the post-publish SPEC-HARNEVAL-001 load fails, publish a permanent `harness_incident_link.v1` record, remove the candidate only after both files are durable, and report the promoted task's current deterministic outcome without executing any `repro` value.

- 검사 순서(첫 실패에서 멈추고 아무것도 쓰지 않는다):
  1. `candidate_id_invalid`, `platform_unsupported`
  2. `path_unsafe`
  3. `candidate_invalid`(detail `decode`, `id_mismatch`, rev 7부터 `unredacted_text`: `expected`, `actual`, `repro`, task `intent`·`outcome`·`status.reason`에 `Redact`를 다시 적용해 바뀌면 거부)
  4. `candidate_provenance_mismatch`(kind ≠ incident, fingerprint ≠ 후보 fingerprint, ref ≠ `representative`, 또는 ref ∉ `learning_refs`)
  5. `draft_incomplete`(detail 순서 `status`, `category`, `assertions`, `kind`)
  6. `candidate_invalid`(001 strict loader detail 그대로)
  7. `no_active_path_for_kind`
  8. `active_set_invalid`(게시 전 001 loader로 기존 active set을 읽어 이미 invalid이면 거부). 예외: 대상 `<id>.json`이 이 promote의 bytes와 같고 그 id의 link record가 없으면 10과 11 사이 중단으로 보고 11부터 재개한다. link record가 있으면 재개하지 않고 아무것도 바꾸지 않은 채 거부한다(rev 7)
  9. 대상과 중복 검사: 대상 `<id>.json`이 같은 bytes로 있으면 `already_active`로 10을 건너뛰되 그 디렉터리를 다시 fsync한다. 같은 id가 다른 파일에 있거나 bytes가 다르면 `task_id_exists`, 같은 fingerprint의 active task나 promoted link가 있으면 `already_promoted`
- 게시와 rollback(이 순서 하나만 있다):
  10. task 게시: 모든 연산은 project root의 `os.Root`로 한다. 같은 디렉터리의 `.<id>.json.tmp-<16 hex>`(이름이 `.json`으로 끝나지 않아 001 loader가 읽지 않음)를 `Root.OpenFile(O_CREATE|O_EXCL|O_WRONLY)`로 쓰고 fsync한다. `Root.Link`로 `<id>.json`을 만들고(go1.26.6 `(*os.Root).Link`), `Root.Open`한 디렉터리를 fsync한다. `Root.Remove`로 temp를 지우고 디렉터리를 다시 fsync한다. 경로 기반 `os.Link`·`os.Rename`·`os.Remove`는 쓰지 않는다. 같은 id의 남은 temp는 시작할 때 지운다.
  11. 사후 검사: 001 loader로 active set 전체를 다시 읽는다. invalid면 `Root.Remove`로 `<id>.json`을 지우고 디렉터리를 fsync한 뒤 `promote_rolled_back`(001 detail)으로 끝난다. 후보와 link는 이 시점까지 건드리지 않았으므로 그대로 남는다.
  12. link 게시: `evals/harness/candidates/promoted/<id>.json`을 10과 같은 방식으로 만든다. 같은 bytes가 있으면 fsync만 한다.
  13. 후보 삭제와 candidates 디렉터리 fsync.
- 중단 뒤 재실행: 10 중간이면 temp 정리 뒤 다시 게시하고, 10과 11 사이면 9의 `already_active`로 11부터, 12와 13 사이면 12의 같은 bytes 규칙으로 13만 진행한다. 11의 rollback이 끝난 뒤의 재실행은 처음과 같은 결과를 낸다. 성공한 뒤의 재실행은 후보가 없어 `candidate_missing`이고 아무 파일도 바꾸지 않는다.
- `current_outcome`: 001 evaluator로 그 task 하나를 평가한 `pass`나 `fail`이다. 001 precondition(예: `templates_stale`)이 실패하면 `not_evaluated`와 reason을 낸다. 이 경우에도 승격은 유지된다. 평가는 001의 hermetic 생성과 sentinel 아래에서 돌고 `repro`를 쓰지 않는다.
- 출력: `--format json`은 stdout에 `harness_promote_result.v1` 하나, 안내(baseline 갱신)는 stderr에 쓴다. `promoted`와 `already_active`는 종료 코드 0, 거부는 1이다. 후보 id는 정확히 하나만 받고 `--all`, `--auto`는 없다. 승격된 task는 baseline 갱신 전까지 001 run에서 `new` 전이와 `set_digest_mismatch`로 드러난다.

### REQ-HC-10 후보 거절
Priority: Must · EARS: EventDriven

WHEN `auto eval harness reject <candidate-id> --reason <text>` runs, THEN THE SYSTEM SHALL move the candidate into a `harness_candidate_rejection.v1` record under `evals/harness/candidates/rejected/` that keeps its fingerprint, learning refs, and redacted reason, so that later intake reports `already_rejected` instead of recreating the candidate.

- 경로 규칙과 게시 방식은 REQ-HC-11, REQ-HC-06과 같다. 거절 record는 prune 보호 대상이 아니다. entry가 prune된 뒤에도 fingerprint가 남아 재생성을 막는다. 후보 파일을 손으로 지우는 것은 거절이 아니며, 다음 `--all-eligible`이 같은 후보를 다시 만든다.

### REQ-HC-07 Prune 보호
Priority: Must · EARS: EventDriven

WHEN `auto learn prune` runs, THEN THE SYSTEM SHALL keep every entry referenced by an open candidate, a promoted link record, or the provenance of an incident-kind active task regardless of age, compute that set inside the store lock, and refuse with `eval_links_unreadable` while leaving the store byte-identical when any scanned intake file, manifest, or task file is unreadable, malformed JSON, or a symlink.

- 범위: intake 영역이 있으면 manifest가 없어도 열린 후보와 promoted link를 읽는다. 열린 후보는 이름이 GTC 형식인 regular file이고, promoted link는 이름이 001 task id 형식인 `.json`이다. 다른 이름(`.gitkeep`, README)은 무시한다. 거절 record는 보호에 쓰지 않는다. active task는 manifest가 있을 때만 읽는다. provenance만 관대하게 decode하므로 001 의미 검증(`corpus_digest_mismatch` 등)은 prune을 막지 않는다. intake 영역과 manifest가 모두 없으면 기존 `learn.Prune` 동작과 출력이 그대로다.
- 그룹 보존: promote는 후보의 `learning_refs` 전체와 대표의 expected/actual/repro 사본을 link record로 옮긴다. 그래서 대표가 아닌 근거 entry도, flag로 받은 actual·repro도 승격 뒤에 남는다. 001 baseline 갱신 뒤 tombstone 파일을 지워도 link record는 남는다.
- API: `[NEW] learn.PruneExcept(store, days, protect func() (map[string]bool, error)) (removed, keptProtected int, err error)`. `protect`는 store lock 안에서 호출된다. `Prune`은 signature를 바꾸지 않고 `protect` nil로 위임한다. 프로세스 간 잠금은 기존 store처럼 없다(in-process `sync.Mutex`).
- 출력: 기존 `Removed N entries older than D days.`(`internal/cli/learn_prune.go:43`). 보호 때문에 남은 오래된 entry가 K>0이면 `Kept K entries linked to golden-task evals.`를 덧붙인다. K는 cutoff 이전인데 보호로 남은 수다.

### REQ-HC-08 Quarantine 격리
Priority: Must · EARS: Ubiquitous

THE SYSTEM SHALL keep the intake area out of evaluation so that creating, editing, rejecting, or deleting files under `evals/harness/candidates/` leaves the `auto eval harness run` result identical except `produced_at`, and keep `pkg/learn`, `pkg/secretscan`, `pkg/worker/security`, and `pkg/harneval/intake` outside the dependency closure of `pkg/harneval`.

- loader 규칙(quarantine 경로의 active 선언 거부)은 SPEC-HARNEVAL-001 REQ-HE-01이 소유한다. 이 SPEC은 end-to-end와 `go list -deps ./pkg/harneval`로 검증한다.

### REQ-HC-09 흐름 문서화
Priority: Should · EARS: Ubiquitous

THE SYSTEM SHALL document the record, intake, assertion authoring, promote or reject, and baseline update flow, plus the mixed-version prune, duplicate-entry, and shell-history caveats, in `evals/harness/README.md` and in the help text of the three intake commands.

- mixed-version 주의: 이 SPEC 이전 binary로 prune하면 두 가지가 일어난다. 연결된 entry도 나이 기준으로 지워지고, `expected`, `actual`, `repro`가 사라진다(probe A2). link record와 후보가 사본을 가지므로 eval 근거 문구는 남는다.

## Wire Contracts

| 문서 | 필드 |
|------|------|
| `harness_golden_candidate.v1` | `schema_version`, `id`, `fingerprint_version`, `fingerprint`, `representative`, `learning_refs[]`, `expected`, `actual`, `repro`, `redacted`, `redacted_fields[]`, `task`(초안 `harness_golden_task.v1`) |
| `harness_incident_link.v1` | `schema_version`, `task_id`, `candidate_id`, `fingerprint_version`, `fingerprint`, `representative`, `learning_refs[]`, `expected`, `actual`, `repro`, `redacted`, `redacted_fields[]` |
| `harness_candidate_rejection.v1` | `schema_version`, `candidate_id`, `fingerprint_version`, `fingerprint`, `learning_refs[]`, `reason`(redacted, ≤1024 byte) |
| `harness_intake_result.v1` (stdout) | `schema_version`, `rows[]{learning_id, result, candidate_id?, match?, fingerprint?, learning_refs?, reason?}` 처리 순서. `learning_refs`는 `created` 행에만, `reason`은 `skipped` 행에만 있다 |
| `harness_promote_result.v1` (stdout) | `schema_version`, `result: promoted\|already_active`, `candidate_id`, `task_id`, `task_path`, `link_path`, `current_outcome: pass\|fail\|not_evaluated`, `not_evaluated_reason?` |

모든 파일 encoding은 `json.MarshalIndent`(공백 2칸)와 끝 LF이고 struct 필드 순서를 따른다. `learning_refs`는 숫자 순서, `redacted_fields`는 byte 순서다. 세 record 모두 `DisallowUnknownFields`로 strict decode한다.

## Existing Test Changes

바꾸는 기존 test는 없다. 근거는 다섯 가지다. (1) `pkg/learn`, `internal/cli/learn*`, `pkg/pipeline` learn test를 새 detector 패턴(할당, 경로, Bearer, URL 포함)으로 grep해 0건이었다. (2) `Recorded` 출력 test는 접두사만 단언한다(`learn_subcmd_test.go:80,113`). (3) `learn.Prune` signature는 그대로다. (4) `pkg/worker/security`와 `pkg/qa/evidence`는 바꾸지 않는다. (5) template 문구 변경은 해당 줄을 단언하는 test가 없음을 T2에서 grep으로 다시 확인한다. 구현 중 바뀌는 test가 생기면 요구사항으로 올리지 않고 이 절에 열거한 뒤 review를 받는다.

## 생성 파일 상세

- `[NEW] pkg/secretscan/redact.go`와 `redact_test.go`: REQ-HC-01의 span 병합 `Redact`, S11 fixture, 출처 일치 drift test, superset·조각 잔존 test. `pkg/qa/evidence`와 `pkg/worker/security`에는 `[NEW] SecretDetectorSources()`, `[NEW] DefaultPatternSources()` accessor만 더한다. `templates/claude/commands/auto-workflows.md.tmpl`은 L2359 append fallback 제거와 L2671-2674 Sync Target 4.5 교체(`--days 90`, 직접 수정 금지). `.github/workflows/ci.yaml`은 T13의 두 step.
- `pkg/learn/types.go`, `store.go`, `rewrite.go`, `record.go`: 필드 3개, 쓰기 경계 redaction. `pkg/learn/prune.go`: `[NEW] PruneExcept`, `Prune` 위임.
- `internal/cli/learn_record.go`: flag 3개와 검증. `internal/cli/learn_prune.go`: 보호 callback 조합과 추가 출력 줄.
- `[NEW] pkg/harneval/intake/`: `fingerprint.go`, `candidate.go`, `intake.go`, `safepath.go`, `safepath_unix.go`, `safepath_windows.go`, `publish.go`, `promote.go`, `reject.go`, `links.go`와 각 `_test.go`. 파일당 300줄 이하, coverage 85% 이상.
- `[NEW] internal/cli/eval_harness_intake.go`, `eval_harness_promote.go`, `eval_harness_reject.go`. SPEC-HARNEVAL-001 T6이 만든 `internal/cli/eval_harness.go`에는 이 세 명령의 등록 줄만 추가한다.
- `evals/harness/README.md`(001이 만든 파일)에 intake 절을 추가한다. intake 영역 디렉터리는 필요할 때 만든다.

## Related SPECs

- SPEC-HARNEVAL-001 rev 3 (Primary). 선행 task는 T1(loader, manifest), T2(hermetic 생성·sentinel), T3(assertion), T4(digest·비교), T5(stale templates), T6(`auto eval harness` 골격)이다. `current_outcome`과 S6/S8의 `auto eval harness run`이 이 task들에 의존한다. 001이 소유하는 것은 `harness_golden_task.v1`, `provenance`/`status`, `active_paths`, quarantine 경로 거부, `## Existing Test Changes`다. 이 SPEC은 001 baseline의 `kind`/`not_run` 계약을 읽지 않는다.
- SPEC-LEARN-001 계열: learn store 형식(JSONL, tolerant read)을 그대로 쓰고 쓰기 경계에만 redaction을 더한다.
- Sibling 한도: 이 SPEC은 sibling을 만들지 않는다.

## Traceability Matrix

| Requirement | Plan Task | Acceptance Scenario | Semantic Invariant |
|-------------|-----------|---------------------|--------------------|
| REQ-HC-01 | T1, T2, T13 | S1, S11 | INV-HC-04, INV-HC-08 |
| REQ-HC-02 | T3 | S2 | INV-HC-01 |
| REQ-HC-03 | T4, T6 | S3, S4 | INV-HC-02, INV-HC-03 |
| REQ-HC-04 | T4, T6 | S4 | INV-HC-03 |
| REQ-HC-05 | T4, T6, T7 | S5 | INV-HC-04, INV-HC-10 |
| REQ-HC-06 | T7 | S6 | INV-HC-05 |
| REQ-HC-07 | T9 | S7 | INV-HC-06 |
| REQ-HC-08 | T10 | S8 | INV-HC-07 |
| REQ-HC-09 | T11 | S9 | - |
| REQ-HC-10 | T8 | S4 | INV-HC-03 |
| REQ-HC-11 | T5, T13 | S10 | INV-HC-09 |
| Brownfield 회귀 | T12 | S1, S7 | INV-HC-08 |

## Review Resolution (rev 2)

| Finding | 처리 | 위치 |
|---------|------|------|
| F-003 그룹 보호 소실, flag 값 소실 | promote가 `learning_refs` 전체와 증거 사본을 영구 link record로 옮긴다. manifest가 없어도 intake 영역을 보호 범위에 넣는다 | REQ-HC-06, REQ-HC-07, S6, S7 |
| HC-002 tracked store에 secret 원문 | 첫 쓰기 경계(store writer)에서 secret detector로 가린다. flag 값과 store 재읽기도 다시 검증하고 가린다 | Trust Model, REQ-HC-01, REQ-HC-03, S1, S5 |
| HC-003 경로 이탈 | id 형식 검사, `os.Root`와 `Root.Lstat` 구성 요소 검사, regular file, `O_EXCL`(rev 3에서 이식성 때문에 `O_NOFOLLOW` 대신 `os.Root`) | REQ-HC-11, S10 |
| F-001 승격 뒤 set invalid | active set 전체 id·fingerprint 검사, status 검사, 사후 001 loader 검사와 rollback | REQ-HC-06, S6 |
| F-002 초안 필드와 S6 순서 | 초안에 001 필드를 모두 둔다. 이 SPEC이 검사 순서를 정한다. S6 fixture를 001 rev 3에 맞추고, `--kind`는 surface만 받는다 | REQ-HC-03, REQ-HC-06, S6 |
| F-005, F-006, HC-010 fingerprint | 정규화 순서와 함수를 고정했다. `v`를 추가하고 scanner와 분리했다. 여러 경로·package oracle을 넣었다 | REQ-HC-02, S2 |
| HC-009 repro 미실행 근거 | 성공한 promote까지 sentinel로 확인한다 | S5 |
| HC-005 내구성 | temp, fsync, link(rev 4부터 `Root.Link`), 디렉터리 fsync, 재실행 수렴 | REQ-HC-06, S6 |
| F-007 보호 범위·K 정의 | 파일 범위, 관대한 task 읽기, lock 안 계산, K 정의 | REQ-HC-07, S7 |
| F-008 거절 재생성 | `reject` 명령과 거절 record | REQ-HC-10, S4 |
| F-009 출력·종료 코드·encoding | 행 형식, 0/2/1 종료 코드, flag 조합, canonical encoding, S1 줄 번호 | REQ-HC-03, Wire Contracts, S1, S3 |
| F-010 EARS | `WHEN …, THEN THE SYSTEM SHALL`로 고치고 실제 parser로 확인 | Requirements |
| F-011 선행 조건 | 001 T2, T3, T5 추가, `eval_harness.go` 등록 줄 명시 | Related SPECs, 생성 파일 상세 |
| F-012 참조·Self-Verify·Debt | L43으로 정정, 전 항목 Self-Verify, Completion Debt 정직하게 기록 | research.md |
| HC-011 검증 명령 | `pkg/learn`, `pkg/harneval` 전체 test를 필터 없이 실행 | plan.md T12, research.md |
| F-013 (rev 3) detector 범위·phase/spec_id·stdout | `pkg/qa/evidence.RedactText` 재사용 + 3종, 일곱 field, CLI 출력 가림, 형식별 oracle | REQ-HC-01, S1, S11 |
| F-001 (rev 3) rollback 모순·순서 | 게시 전 active set 검사, 단일 게시·rollback 순서, 후보는 13단계에서만 삭제 | REQ-HC-06, S6 |
| HC-005 (rev 3) 내구성·temp | temp 이름 규칙, `already_active`에서도 fsync, 중단 지점별 재실행 규칙 | REQ-HC-06, S6 |
| F-006 (rev 3) prune 재redaction drift | 파싱된 entry는 다시 가리지 않는다. skip 줄만 가린다. prune 전후 fingerprint 동일 oracle | REQ-HC-01, REQ-HC-02, S2 |
| HC-010 (rev 3) active task 읽기 실패 | 깨진 task와 symlink task oracle 추가 | S7 |
| F-014 (rev 3) Windows | `os.Root`, build tag 분리, Windows 쓰기 `platform_unsupported`, windows test 컴파일 | REQ-HC-11, S10 |
| F-015 (rev 3) scanner 이동이 내부 test를 깨뜨림 | 이동하지 않는다. 바뀌는 test 0건 | REQ-HC-01, Existing Test Changes |
| F-017 (rev 3) template 직접 append | template fallback 제거와 static test | Trust Model, REQ-HC-01, S1 |
| F-018 (rev 4) sync prune 경로와 coverage 손실 | template L2671-2674를 정수 `--days 90`과 직접 수정 금지로 바꾼다. detector를 qa+worker union으로 바꾸고 잃었던 형식마다 oracle을 둔다 | REQ-HC-01, S1, S11 |
| HC-003 (rev 4) `os.Link`의 root 이탈 | 게시·rollback·삭제를 모두 `os.Root` 메서드로 한다(`Root.Link`는 go1.26.6에 있음) | REQ-HC-06, REQ-HC-11 |
| F-016 (rev 4) nil과 빈 목록 | 정규화가 항상 `[]`를 낸다. canonical bytes와 SHA-256 vector를 S2에 넣었다 | REQ-HC-02, S2 |
| F-019 (rev 4) worker 전용 형식 누락 | union으로 해소했다. 짧은 할당, prose 할당, AWS 문맥, 짧은 Bearer, azure, private_key JSON oracle | REQ-HC-01, S11 |
| F-020 (rev 4) rewrite 문구 모순 | canonical 재encode는 유지하고 재redaction만 하지 않는다고 고쳤다. 기존 test 4개 영향 없음 | REQ-HC-01, S1 |
| F-019 (rev 5) 순차 합성의 조각 누출 | 원문 기준 span 수집·병합·한 번 치환으로 바꾸고 출처 일치·조각 잔존 test를 두었다. 순차 방식의 누출은 실행으로 확인했다 | REQ-HC-01, S11 |
| F-021 (rev 5) id 재사용 | eligible을 fingerprint로만 판정한다. 재사용 id oracle을 넣었다 | REQ-HC-03, S4 |
| F-006 (rev 5) legacy 구분 | legacy raw 줄과 새 줄을 구분하는 fingerprint oracle을 두 값으로 넣었다 | REQ-HC-01, S2 |
| F-022, F-023 (rev 5) 잔존 문구·S11 추적 | 생성 파일 상세, research, plan의 이전 설계 문구를 고치고 S11을 완료 근거, Traceability, INV-HC-04에 넣었다 | spec.md, research.md, plan.md |
| F-024 (rev 5) Windows test 미실행 | CI task T13을 넣었다(ubuntu vet, windows-runtime 실행) | REQ-HC-11, plan T13 |
| F-025 (rev 5) cap과 redaction 순서 | 5단계 순서와 detail, 재검증 실패의 행 결과를 정했다 | REQ-HC-01, REQ-HC-03, S1, S4 |
| F-026 (rev 6) placeholder가 secret을 가려 줌 | 겹친 span을 버리지 않고 placeholder 경계까지 넓혀 병합 span 전체를 다시 가린다. 정확히 placeholder 하나인 span만 건너뛴다. 같은 match와 인접 match oracle을 넣었다. 이전 규칙의 누출 여섯 건은 실행으로 확인했다 | REQ-HC-01, S11 |
| F-024 (rev 6) Windows test가 비어도 통과 | windows-runtime step에 `go test -list` floor와 PASS 집합 비교를 두고 static test로 고정한다. S10 문구를 CI 실제 단계와 맞췄다 | REQ-HC-11, S10, plan T13 |

## Review Resolution (rev 7, Phase 4 RALF 1)

| Finding | 처리 | 위치 |
|---------|------|------|
| C1 8단계 재개가 durable link 뒤 task를 지움 | 재개 예외는 task bytes가 같고 그 id의 link record가 없을 때만 적용한다. link가 있으면 아무것도 바꾸지 않고 `active_set_invalid`로 멈춘다. 결함을 고친 뒤 재실행은 `already_active`로 13단계만 진행한다 | REQ-HC-06, S6 |
| C2 `Redact` 멱등 위반(placeholder의 `SECRET`이 worker AWS 문맥) | neutral copy를 훑고 placeholder 글자만으로 생긴 원문 span은 버린다. pass를 고정점까지 반복한다. S11 fixture 37개 bytes는 그대로이고 SHA fixture, fixture·무작위 corpus 멱등 성질 test를 더했다 | REQ-HC-01, S11 |
| C3 intake 검사기 사본이 raw C1 byte를 받음 | intake는 `learn.CheckEvidence`와 `learn.HasControlChar`를 쓴다. 깨진 UTF-8은 제어 문자다 | REQ-HC-01, REQ-HC-03, REQ-HC-05 |
| S1 placeholder에 붙은 원문(`session=[REDACTED_SECRET]hunter2xyz`) | neutral copy의 filler가 값 문자이므로 붙은 원문이 값에 합쳐져 가려진다. qa 전용 key와 flag shield fixture 9개 | REQ-HC-01, S11 |
| S2 `--severity`, `--files`, `--packages` 원문 저장 | severity는 enum만 받는다. files·packages는 가리지 않고(fingerprint 입력) `Redact`가 바꿀 항목이 있으면 `learning_field_invalid`로 거부한다. CLI는 쓰기 전에 같은 검사를 한다 | REQ-HC-01, REQ-HC-02 |
| S3 match가 다음 key를 삼킴(`Bearer token: secret: hunter2xyz`) | 선택 부분 시작에서 다시 찾아 겹친 match를 모으고, `RedactText`·worker `Scan`의 순차 치환을 위치 추적 replay로 함께 가린다. placeholder 없는 무작위 입력 6000개에서 upstream이 지운 secret을 남기지 않는 차등 test를 둔다. placeholder가 이미 든 입력은 C2 때문에 superset 대상에서 뺀다(worker는 `SECRET` 글자를 문맥으로 읽는다) | REQ-HC-01, S11 |
| S4 PEM 본문 노출 | 머리줄 span을 footer 끝까지, footer가 없으면 base64 본문까지 넓힌다 | REQ-HC-01, S11 |
| S5 `Recorded` 줄의 제어 문자 | `intakePrintable`로 가리고 escape한다 | REQ-HC-01 |
| S6 손으로 고친 후보의 원문 | 3단계에서 게시할 text에 `Redact`를 다시 적용해 바뀌면 `candidate_invalid`(`unredacted_text`). `PromoteRequest.Redactor`는 필수다 | REQ-HC-06 |
| S7 symlink store(`pipeline.jsonl -> ../../.env`) | append와 rewrite는 Lstat regular file 검사 뒤 unix에서 `O_NOFOLLOW`·`O_NONBLOCK`으로 연다. 읽기와 상위 디렉터리 symlink는 잔여 위험이다 | REQ-HC-01, REQ-HC-07 |
| 실행자 결정 `reason_required` | 빈 `--reason`은 쓰기 없이 `reason_required`, 종료 코드 1 | REQ-HC-10 |
| 실행자 결정 `rejection_exists` | 같은 이름의 거절 record가 다른 bytes면 `rejection_exists`, 같은 bytes면 fsync 뒤 유지해 재실행이 이동을 끝낸다 | REQ-HC-10 |
| 실행자 결정 `learning_not_found` | 없는 `--learning` id는 실행 단위 오류가 아니라 행 `skipped`(`learning_not_found`)이고 종료 코드 2다 | REQ-HC-03 |
| 실행자 결정 Windows floor | windows-runtime step의 floor는 PlatformUnsupported test 네 개 전부(intake, promote, reject, 쓰기 helper)라 하나만 빠져도 실패한다 | REQ-HC-11, S10 |
| 실행자 결정 8단계 재개 예외 | 게시 전 set이 invalid여도 task가 이 promote의 bytes이고 link가 없으면 11부터 재개해 task가 원인이면 rollback한다(C1로 link 조건 추가) | REQ-HC-06, S6 |
| 실행자 결정 text-invalid member 제외 | pattern(`candidate_text_invalid`)이나 evidence(`learning_field_invalid`) 검사에 실패한 entry는 `skipped`이고 fingerprint 그룹의 `learning_refs`와 prune 보호에 들지 않는다 | REQ-HC-03, REQ-HC-05 |
| 기존 test 변경 | 이 SPEC이 만든 `TestStoreWriter_RedactsEveryFreeTextField`의 files 값을 `/Users/alice/…`에서 repo 상대 경로로 바꿨다(S2로 거부되는 값). 성질 test는 raw `detect` 대신 pass의 span으로 검사한다. main의 기존 test는 바꾸지 않는다 | Existing Test Changes |
