# SPEC-HARNEVAL-002 수락 기준

## Test Scenarios

fixture는 temp project 안에서 만든다. fingerprint 기대값은 아래 canonical JSON의 SHA-256이며 문서 작성 시 직접 계산했다. 합성 token은 test 실행 중에만 만들고 문서와 fixture 파일에는 넣지 않는다.

- Y = `{"v":1,"type":"fix_pattern","pattern":"hook missing in codex","files":["pkg/content/a.go","pkg/content/hooks.go","pkg/content/z.go"],"packages":["pkg/adapter","pkg/content"]}` → `023e9302ff0bb28b37a0ebe3bba8af3b50713391bc8aea65cb3fc66ae7f42c86` (`GTC-023e9302ff0b`, `GT-INC-023E9302`)
- Y' = Y에서 type만 `gate_fail` → `b375e2216af3bbffc15e1af07d84d283661be97c0591a9bc75abcfd5d59ea885`
- X = `{"v":1,"type":"review_issue","pattern":"router drops detail mapping","files":["content/skills/plan.md"],"packages":["pkg/content"]}` → `8e80c7a180298de062857e2a5f03c4d9fcdb5875bb50e0efa2f1b2b093e502fd` (`GTC-8e80c7a18029`, `GT-INC-8E80C7A1`)

### S1: schema 확장과 첫 쓰기 redaction
Priority: Must
Given 기존 형식 entry 3건이 있는 store와 실행 중 만든 합성 `github_pat_` token이 있다
When `auto learn record --type fix_pattern --pattern "p <token>" --phase "review API_KEY=<12자>" --expected e --actual "<token> leaked" --repro "auto init"`, 세 flag 없는 record, `spec_id`에 token을 넣은 `learn.AppendAtomic` 직접 호출(pipeline hook 경로)을 실행한다
Then 4번째 줄의 `pattern`은 `p [REDACTED_SECRET]`, `phase`는 `review [REDACTED_SECRET]`, `actual`은 `[REDACTED_SECRET] leaked`이고 `"expected":"e"`, `"repro":"auto init"`이 있으며, stdout은 `Recorded fix_pattern entry: p [REDACTED_SECRET]`이다
And store bytes와 stdout 어디에도 합성 token이 없고, 5번째 줄에는 `expected` key가 없으며, 직접 호출 경로의 `spec_id`도 `[REDACTED_SECRET]`다
And 같은 문구를 두 번 기록한 두 줄의 가려진 field bytes가 같고(`Redact` 멱등), 기존 3줄은 bytes가 그대로다
And 깨진 JSON 줄에 token을 넣은 store를 prune하면 그 줄은 token 자리에 `[REDACTED_SECRET]`를 담아 다시 쓰인다. 파싱된 줄의 field 값은 바뀌지 않는다(기존 canonical encoding에 따른 시간대 표기 정규화는 예외). 기존 `TestStore_UpdateReuseCount`, `TestStore_ConcurrentAppendAndUpdateReuseCount`, `TestStore_ReadTolerant_FallbackTime`, `TestStore_S3_CanonicalRewritePreservesGarbageAndPrunesAged`는 수정 없이 통과한다
And 개행이 든 `--expected`와 1025 byte `--actual`은 각각 종료 코드 1, `learning_field_invalid`이고 store SHA-256이 실행 전과 같다
And `token=a`가 든 1020 byte `--actual`은 가린 값이 1024 byte를 넘어 종료 코드 1과 `learning_field_invalid` detail `over_cap_after_redaction`이고, 4097 byte 이상의 raw 값은 가리기 전에 detail `raw_over_limit`으로 거부된다
And template static test는 template·content 전체에서 store 직접 append·삭제·수정 지시 0건이다. 모든 `auto learn prune`은 정수 `--days`를 쓰고, `auto-workflows.md.tmpl` Sync Target 4.5는 `auto learn prune --days 90`이며 실패 시 store를 고치지 않는다

### S2: fingerprint는 정규화 순서를 정확히 따른다
Priority: Must
Given E1(type `fix_pattern`, pattern `"  Hook   MISSING in\tCodex "`, files `["pkg\\content\\hooks.go"," ./pkg/content/z.go","pkg/content/a.go","","pkg/content/hooks.go"]`, packages `[" pkg/content","pkg/adapter","pkg/content",""]`, id L-004, severity high)이 있다
When `Fingerprint`를 E1, id·timestamp·phase·resolution·expected만 다른 E2, type만 `gate_fail`인 E3에 적용한다
Then E1과 E2는 둘 다 Y의 hash `023e9302ff0bb28b37a0ebe3bba8af3b50713391bc8aea65cb3fc66ae7f42c86`이고 E1의 canonical 입력은 Y의 JSON과 byte 단위로 같다
And E3은 Y'의 hash `b375e2216af3bbffc15e1af07d84d283661be97c0591a9bc75abcfd5d59ea885`이다
And files·packages가 `null`인 entry와 files `["", "  "]`, packages `[" "]`인 entry(둘 다 type `review_issue`, pattern `x y`)는 canonical 입력이 둘 다 `{"v":1,"type":"review_issue","pattern":"x y","files":[],"packages":[]}`이고 hash가 `f40f496b633d95641415c9b1c3006e17a26d4ac27596bc58768460c1ed1fea63`이다(`null`로 encode하면 나오는 `82be6a0f…`가 아니다)
And 이 SPEC 이전 binary 형식으로 쓴 legacy 줄(pattern `deploy failed token=abc123 in ci`)은 `auto learn prune` 뒤에도 pattern이 그대로이고 fingerprint가 prune 전후 모두 `f77ecdb8d8a7be3e3b84e451be34bdcd5efafbf3bf21e8689ebf97614f5828d3`다. 같은 문구를 새로 기록한 entry는 `deploy failed [REDACTED_SECRET] in ci`로 저장되고 fingerprint는 `0d7acc2510a283b12a217612e065ce511007584bb2d32b7514c26c7253db534f`다. prune 때 다시 가리는 구현이면 legacy 값이 두 번째 값으로 바뀌어 실패한다
And E1을 기록한 store에 `auto learn prune --days 30`(E1은 최근 entry)을 실행한 뒤 다시 계산해도 값이 같다(파싱된 entry는 다시 가리지 않음). `Fingerprint`는 detector를 부르지 않는다

### S3: intake는 숫자 순서로 묶고 대표 entry에서만 증거를 가져온다
Priority: Must
Given L-1000(X, expected `x2`, actual `a2`, repro `r2`), L-002(Y, expected `y`, actual `ya`), L-999(X, expected `x1`, actual `a1`, repro 없음), expected/actual이 없는 L-010이 있다
When `auto eval harness intake --all-eligible --format json`을 실행한다
Then stdout은 JSON 하나이고 `rows`는 순서대로 `{L-002, created, GTC-023e9302ff0b, learning_refs ["L-002"]}`, `{L-999, created, GTC-8e80c7a18029, learning_refs ["L-999","L-1000"]}`, `{L-1000, grouped, GTC-8e80c7a18029}`이며 L-010은 없고 종료 코드 0이다
And `GTC-8e80c7a18029`는 `representative` `L-999`, `expected` `x1`, `actual` `a1`, `repro` `""`(L-1000의 `r2`를 쓰지 않음)이고, 초안 task는 `schema_version` `harness_golden_task.v1`, `id` `GT-INC-8E80C7A1`, `kind` `surface`, `category` `""`, `variants` `[]`, `assertions` `[]`, provenance `{incident, L-999, X}`, status `active`다
And `--learning L-010`은 `skipped`(`learning_missing_expected_actual`)와 종료 코드 2, `--learning L-002 --all-eligible`은 `selection_invalid`, `--learning L-002,L-999 --expected a --actual b`는 `flag_requires_single_learning`, `--learning L-002 --expected a`는 `flag_pair_required`, `--kind agent`는 `kind_unsupported`로 모두 종료 코드 1과 새 파일 0개다
And 후보 파일은 공백 2칸 indent와 끝 LF이고, 같은 입력으로 다시 만든 bytes가 같다

### S4: 중복, 충돌, 거절은 기존 파일을 건드리지 않는다
Priority: Must
Given S3이 끝난 project가 있다
When `--all-eligible`, `--learning L-1000`, Y 승격 뒤 Y fingerprint인 새 L-1001, `auto eval harness reject GTC-8e80c7a18029 --reason "not a harness issue"` 뒤의 `--all-eligible`과 `--learning L-999`, fingerprint만 바꾼 `GTC-8e80c7a18029.json` fixture 순서로 실행한다
Then 첫 실행은 `rows` `[]`이고 모든 후보 SHA-256이 같으며, `--learning L-1000`은 `{L-1000, duplicate_candidate, match GTC-8e80c7a18029}`, L-1001은 `{L-1001, already_promoted, match GT-INC-023E9302}`다
And 거절 뒤 `evals/harness/candidates/rejected/GTC-8e80c7a18029.json`에는 fingerprint X, `learning_refs` `["L-999","L-1000"]`, reason이 있고 열린 후보는 없으며, `--all-eligible`은 후보를 다시 만들지 않고 `--learning L-999`는 `already_rejected`다
And 충돌 fixture는 `skipped`(`candidate_id_collision`)와 종료 코드 2이고 fixture SHA-256이 같다
And 거절된 L-001(fingerprint X)이 prune된 뒤 새 사고가 같은 id L-001(fingerprint Z, expected/actual 있음)을 받으면, `--all-eligible`은 `{L-001, created, GTC-<Z 앞 12 hex>}`를 낸다. X 문구를 다시 기록한 entry는 `--all-eligible`에서 건너뛰고, `--learning`에서는 `already_rejected`다
And store를 손으로 고쳐 `expected`를 1100 byte로 만든 entry는 `--learning`에서 행 `skipped`(`learning_field_invalid`)이고 종료 코드는 2다

### S5: 문구는 가려지고 repro는 성공한 승격에서도 실행되지 않는다
Priority: Must
Given intake flag `--actual "<sk-ant- token> seen"`, pattern에 ESC 문자가 든 entry, `repro`가 PATH의 sentinel 명령 `harneval-repro-sentinel`인 entry가 있다
When intake 뒤 sentinel 후보에 category `hooks_settings`와 assertion 하나를 써서 promote하고, 이어서 reject와 prune을 실행한다
Then flag 후보의 `actual`은 `[REDACTED_SECRET] seen`, `redacted_fields` `["actual"]`, `redacted` true이고 store, 후보, stdout bytes에 token이 없다
And ESC entry는 `skipped`(`candidate_text_invalid`)이고 파일을 만들지 않는다
And sentinel 후보의 promote는 `result` `promoted`로 성공하며, intake·promote(current_outcome 평가 포함)·reject·prune 뒤에도 sentinel 로그는 비어 있고 `repro` 값은 link record에 그대로 남는다

### S6: 승격은 정해진 순서로 검사하고 원자적으로 게시한다
Priority: Must
Given baseline과 일치하는 SPEC-HARNEVAL-001 fixture(floors 1, active path `evals/harness/tasks/surface`와 `evals/harness/tasks/agent`, surface task 1개, 유효한 `corpus_ref`와 `expected_tests`를 가진 agent task 1개)와 intake가 만든 `GTC-023e9302ff0b`가 있다
When promote를 그대로, `category` `hooks_settings`만 쓴 뒤, assertion `{"kind":"file_exists","platform":"codex","path":".codex/hooks.json"}`을 더한 뒤 순서대로 실행한다
Then 앞의 둘은 종료 코드 1과 `draft_incomplete` detail `category`, `assertions`이고 후보 SHA-256이 같다
And 세 번째는 `promoted`, `current_outcome` `pass`이고 `evals/harness/tasks/surface/GT-INC-023E9302.json`과 `evals/harness/candidates/promoted/GT-INC-023E9302.json`(`learning_refs`, expected/actual/repro 사본)이 생기며 후보와 temp 파일이 없고, stdout은 JSON 하나, 안내는 stderr에만 있다
And 이어진 `auto eval harness run`은 `GT-INC-023E9302 new`와 `["set_digest_mismatch"]`이고 `auto eval harness baseline --update` 뒤에는 `status` `pass`다
And 거부는 `promote ../x`가 `candidate_id_invalid`, 같은 id가 다른 파일에 있으면 `task_id_exists`, 같은 fingerprint의 active task가 있으면 `already_promoted`, `status.state` retired면 `draft_incomplete` detail `status`, surface active path가 없으면 `no_active_path_for_kind`, `--all`은 unknown flag다
And 게시 직후 seam이 다른 kind 디렉터리에 같은 id task를 넣는 fixture는 `promote_rolled_back`(detail `duplicate_task_id`)이고, `tasks/surface/GT-INC-023E9302.json`과 link가 없으며 후보 SHA-256이 실행 전과 같다
And 이미 invalid인 active set에서는 게시 전에 `active_set_invalid`로 멈추고 아무 파일도 바뀌지 않는다
And 10단계 temp 생성 뒤, 10과 11 사이, 12와 13 사이에서 각각 중단한 뒤 다시 실행하면 세 경우 모두 결과 파일 bytes가 한 번에 성공한 경우와 같고 `.json.tmp-` 파일이 남지 않는다. 성공 뒤 다시 실행하면 `candidate_missing`이고 아무 파일도 바뀌지 않는다

### S7: prune은 그룹의 모든 근거를 보존하고 읽기 실패에서 멈춘다
Priority: Must
Given 2020-01-01의 L-001(열린 후보 ref), L-002(승격된 Y의 대표), L-1000(승격된 X 그룹의 대표가 아닌 ref), L-003(연결 없음)과, 현재 시각의 L-004(열린 후보 ref), L-005(연결 없음)가 있다
When manifest가 있는 project, manifest 없이 intake 영역만 있는 project, 후보 JSON 하나를 깨뜨린 project, promoted link 하나를 symlink로 바꾼 project, manifest JSON을 깨뜨린 project, active incident task JSON을 깨뜨린 project, active task를 symlink로 바꾼 project, intake 영역과 manifest가 모두 없는 project에서 `auto learn prune --days 30`을 실행한다
Then 앞의 두 project는 stdout이 `Removed 1 entries older than 30 days.`와 `Kept 3 entries linked to golden-task evals.`이고 남은 id는 `L-001`, `L-002`, `L-004`, `L-005`, `L-1000`이다
And 깨진 후보, symlink link, 깨진 manifest, 깨진 active task, symlink active task project는 각각 종료 코드 1, `eval_links_unreadable`이고 store SHA-256이 실행 전과 같다
And 첫 project에서 prune을 한 번 더 실행하면 `Removed 0 entries older than 30 days.`와 `Kept 3 entries linked to golden-task evals.`이고 store bytes가 그대로다
And intake 영역의 `.gitkeep`과 README는 무시되고, intake 영역과 manifest가 없는 project는 `Removed 4 entries older than 30 days.`만 출력하며 남은 id는 `L-004`, `L-005`다

### S8: intake 영역은 평가 결과와 의존 closure에 들어가지 않는다
Priority: Must
Given baseline과 일치하는 SPEC-HARNEVAL-001 golden set이 있다
When 후보 생성, 수정, 거절, 삭제 단계마다 `auto eval harness run --format json`을 실행하고 `go list -deps ./pkg/harneval`을 실행한다
Then 다섯 결과는 `produced_at`을 지우면 byte-identical이고 `set_digest`가 모두 같은 64자리 hex다
And `go list -deps` 출력에는 `pkg/learn`, `pkg/secretscan`, `pkg/worker/security`, `pkg/harneval/intake`가 없다

### S9: 흐름 문서가 실제 명령과 맞는다
Priority: Should
Given `evals/harness/README.md`와 세 명령의 help text가 있다
When 문서의 명령을 순서대로 temp project에서 실행한다
Then record, intake, assertion 작성, promote, baseline 갱신이 이 순서로 성공하고, README에 mixed-version prune, 중복 entry, 셸 history 주의 문장이 있다

### S10: intake 영역 밖으로 읽기·쓰기·삭제가 새지 않는다
Priority: Must
Given project 밖 디렉터리 O와, `evals/harness/candidates`를 O로 가리키는 symlink, `evals/harness/tasks/surface`를 O로 가리키는 symlink, O의 파일을 가리키는 `GTC-023e9302ff0b.json` symlink를 각각 둔 fixture가 있다
When intake, promote, reject, prune을 각 fixture에서 실행한다
Then 모든 실행은 종료 코드 1과 `path_unsafe`(prune은 `eval_links_unreadable`)이고 O의 파일 목록과 SHA-256은 실행 전과 같다
And `promote GTC-023E9302FF0B`(대문자)와 `promote GTC-023e9302ff0b/../x`는 경로를 만들기 전에 `candidate_id_invalid`로 거부된다
And Windows build의 intake·promote·reject는 종료 코드 1과 `platform_unsupported`이고, 이 동작은 `//go:build windows` test로 고정된다. CI ubuntu job은 `GOOS=windows GOARCH=amd64 go vet ./pkg/harneval/intake/`로 이 test를 type-check하고, windows-runtime job은 `go test -list` 수가 floor 1 이상인지와 PASS 집합이 목록과 같은지 확인하며 실행한다. static test는 그 step에 floor 검사와 PASS 집합 비교가 있음을 단언한다

### S11: detector는 원문 기준 span 병합으로 repo의 secret 형식을 모두 가린다
Priority: Must
Given 실행 중 만든 fixture가 있다. 형식별 19개는 `Bearer ` + 24자, `sk-`·`sk-proj-`·`sk-ant-` + 24자, `ghp_`·`gho_`·`ghu_`·`ghs_`·`ghr_` + 36자, `github_pat_` + 40자, `API_KEY=` + 12자, `--token ` + 12자, JSON `"api_key": "` + 12자 + `"`, `https://user:` + 10자 + `@example.com`, `?token=` + 12자, `/Users/alice/proj`, `AKIA` + 16자, `-----BEGIN RSA PRIVATE KEY-----`, `apjwt_a.b.c`다. worker 전용 6개와 겹침 2개(`see /Users/aws/` + 40자, `aws /Users/bob/` + 40자)도 있다
When `secretscan.Redact`를 각 fixture에 적용한다
Then 형식별 출력의 secret 자리는 이렇다. Bearer는 `[REDACTED_SECRET]`(Bearer 단어 포함), 다음 9개 token은 `[REDACTED_SECRET]`, `API_KEY=` 할당은 할당 전체가 `[REDACTED_SECRET]`, `--token [REDACTED_SECRET]`, `"api_key": "[REDACTED_SECRET]"`, `https://[REDACTED_SECRET]@example.com`, `?[REDACTED_SECRET]`, `/Users/[REDACTED_USER]/proj`, 마지막 3개는 `[REDACTED_SECRET]`이고 모두 두 번째 반환값이 true다
And worker 전용 형식의 출력은 이렇다. `password=ab`와 `azure client_secret = ab`는 `[REDACTED_SECRET]`, `password=letmein was rejected`는 `[REDACTED_SECRET] was rejected`, `aws secret ` + 40자와 `Bearer abc`는 `[REDACTED_SECRET]`, `{"private_key_id": "k1"}`는 `{[REDACTED_SECRET]"}`다
And 겹침 2개의 출력은 `see /Users/[REDACTED_SECRET] end`와 `[REDACTED_SECRET] end`다. rev 4의 순차 합성은 두 경우 모두 40자 값을 남겼다(실행 확인)
And 같은 match에 placeholder와 원문 값이 섞인 fixture도 가린다. `set password=[REDACTED_SECRET]hunter2xyz end`는 `set [REDACTED_SECRET] end`다. `aws [REDACTED_SECRET] ` + 40자와 `[REDACTED_SECRET]; aws ` + 40자는 둘 다 `[REDACTED_SECRET] end`다. `password=[REDACTED_SECRET]` + 24자는 `[REDACTED_SECRET] end`다. `vault note: [REDACTED_PRIVATE_NOTE] plus diary text here`는 `vault note: [REDACTED_PRIVATE_NOTE]`다. `{"api_key": "[REDACTED_SECRET] rawvalue123"}`는 `{"api_key": "[REDACTED_SECRET]"}`다
And 인접 match `API_KEY=[REDACTED_SECRET] token=` + 12자는 `[REDACTED_SECRET] [REDACTED_SECRET] end`이고, `[REDACTED_SECRET]` 하나뿐인 입력은 그대로다. rev 5의 '겹치면 버림' 규칙은 위 혼합 fixture 여섯 개에서 모두 원문 값을 남겼다(실행 확인)
And 모든 fixture에서 세 가지가 성립한다. `Redact`를 다시 적용해도 결과가 같다. 출력에 detector를 다시 돌리면 placeholder 밖 span이 0개다. 원문 secret span에서 placeholder를 뺀 부분의 8자 이상 부분 문자열이 출력에 없다
And `Bearer token header was dropped by the hook`은 `[REDACTED_SECRET] header was dropped by the hook`(수용한 오탐), `router drops detail mapping for plan`은 그대로다. `pkg/secretscan` 표의 정규식 출처는 `SecretDetectorSources()`와 `DefaultPatternSources()`를 이은 목록과 정확히 같다

## Oracle Acceptance Notes

- 모든 Must 시나리오는 concrete expected output(정확한 SHA-256, 후보 id, 정렬된 `learning_refs`, 행 결과와 reason literal, stdout 줄, 남은 id 목록, 종료 코드 0/1/2의 구분)을 가진다. 파일 존재, heading, 종료 코드만으로 닫는 Must 시나리오는 없다.
- S11의 모든 출력과 세 성질은 2026-10-06 probe A1에서 이 SPEC의 span 병합 알고리즘을 그대로 구현한 scratch 프로그램으로 실제 확인했다. 순차 합성의 누출은 `pkg/qa/evidence.RedactText` overlay로 확인했다.
- S2/S3은 heterogeneous 입력(대소문자·공백·경로 구분자·중복·빈 값·null이 섞인 entry, 자릿수가 다른 id, member마다 다른 expected/actual/repro)으로 정규화, 정렬, grouping, 대표 선택을 검증한다. `L-999` < `L-1000`은 문자열 순서와 다른 숫자 순서다.
- S6의 `.codex/hooks.json`은 기본 full config 생성 결과에 있음을 SPEC-HARNEVAL-001 probe A2의 생성 tree에서 확인했다.
