# base golden 재기록 (2026-10-17, v0.50.126)

1–3절은 RC(8a0da3a1)의 첫 재기록이고, 지금 golden은 5절의 release/v0.50.126(9fe18fc2) 재기록이다.

- 증상: RC `rc/v0.50.126`(8a0da3a1)에서 `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`의 init-yes, update-yes, update-ci, init-pipe, init-in과 `TestInitReview_S6ShowsTheSettingsBeforeAnyWrite`가 NPD 차이로 실패했다. 단언하는 stdout golden은 모두 같았다.
- 환경: go1.27.0 darwin/arm64. 모든 run은 test harness(`tuiux_env_test.go`의 temp HOME·XDG·PATH, `LANG=ko_KR.UTF-8`, CAT-A)로 돌렸고 `-run` filter만 썼다.

## 1. TUIUX 회귀가 아니다

`go test ./internal/cli/ -count=1 -run '^(TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6ShowsTheSettingsBeforeAnyWrite)$'`를 옛 golden으로 돌린 결과:

| tree | 결과 |
|---|---|
| 81cda519 (b08ea644 + T1 harness, golden을 기록한 자리) | 통과 |
| 78191e1a (TUIUX tip, RC merge 전) | 통과(S6 포함) |
| 57809767 (+ REVMODEL merge) | 통과 |
| 32da02c6 (+ REA merge) | 통과 |
| 9b6a839e (+ TEAMAUTO merge) | 실패: 위 다섯 흐름과 S6 |
| 8a0da3a1 (+ origin/main merge, RC) | 실패: 같은 다섯 흐름. writer 출력이 9b6a839e와 bytes가 같다 |
| RC에서 TUIUX를 뺀 tree(3절) | 실패: 같은 다섯 흐름. TUIUX 코드가 전혀 없다 |

## 2. 차이와 그 출처

NPD 차이는 init 흐름마다 4줄, update 흐름마다 8줄이다. 추가되거나 사라진 경로는 없다.

- `.claude/skills/agent-teams/SKILL.md`: 124a1fdf… → 6ce8b7e9…
- `.claude/skills/auto-go/SKILL.md`: fd9c9ec2… → f05a89fc…
- `.autopus/claude-code-manifest.json`: 위 두 파일의 `checksum` 두 줄만 다르다(`generated_at`은 정규화 대상). `codex-manifest.json`은 `generated_at`만 다르다.
- `.autopus/txns/<TX>-claude-code/journal.json`: `tuiuxJournal`처럼 정규화한 decode 값에서 위 두 파일 entry의 `after_checksum`만 다르다.
- update 흐름: 위 셋의 backup 사본(`.autopus/backup/<TS>/transaction/claude-code/…`)과 둘째 journal. backup은 init이 같은 코드로 만든 파일이라 같은 차이를 갖는다.

내용 diff(init-yes 생성 파일, 81cda519 대 8a0da3a1):

| 생성 파일 | 더해진 절 | 소스(RC) | 줄의 commit(`git blame`) |
|---|---|---|---|
| `agent-teams/SKILL.md` | `## Automatic Entry from Route A`, 32줄 | `content/skills/agent-teams.md:115-146` | edb38731 29줄 `docs(skills): agent-teams에 Route A 자동 진입 절을 더한다 (SPEC-TEAMAUTO-001 T8)`, 3f29f32b 3줄 `docs(readme): 옛 auto의 workflow.auto_team 호환 범위를 0.50.113–0.50.125로 바로잡는다 (SPEC-TEAMAUTO-001)` |
| `auto-go/SKILL.md` | `#### Route A Phase 2 Entry: Agent Team Auto-Selection`, 66줄 | `templates/claude/commands/auto-workflows.md.tmpl:1192-1257` | 69b0be54 50줄 `docs(templates): auto-go Route A에 Phase 2 자동 팀 선택 절을 더한다 (SPEC-TEAMAUTO-001 T7)`, 0d28e7a4 16줄 `fix(templates): facts를 한 Bash 호출에서 quoted heredoc으로 쓰고 shell 특수 문자 path를 거부한다 (SPEC-TEAMAUTO-001)` |

- RC 출력에서 그 절(소스 줄 그대로, 한 번 나온다)을 빼면 두 파일 모두 base 출력과 bytes가 같다.
- `b08ea644..8a0da3a1`에서 두 소스 파일을 바꾼 commit은 이 넷뿐이다. 넷 모두 `feat/teamauto-001`에서 왔고 TUIUX tip 78191e1a의 조상이 아니다.

## 3. 다시 기록할 기준: RC에서 TUIUX를 뺀 tree

- S6·S15·INV-TUX-06이 지키는 것은 "폼이 열리지 않는 자리에서 TUIUX가 출력을 바꾸지 않는다"이다. b08ea644 기록을 그대로 두면 다른 SPEC의 내용 변경만으로 깨진다. RC 자체에서 writer를 돌리면 검사 대상인 TUIUX 코드가 기준을 만든다. 그러면 invariant가 비고, 단언하지 않는 `init-in.stdout`에 TUIUX 확인 화면 19줄이 들어간다. 그래서 기준은 같은 release tree에서 TUIUX 소스 변경만 뺀 tree의 출력이다.
- 구성 근거:
  - TUIUX가 바꾼 파일(`b08ea644..78191e1a`)은 merge한 네 branch(REVMODEL ec595161, REA e6f8e497, TEAMAUTO 361ebf00, origin/main 76f6bdc3)가 각자의 merge base 뒤로 하나도 바꾸지 않았다. 겹치는 파일은 `CHANGELOG.md`·`README.md`뿐이다.
  - RC의 네 merge에는 충돌도 수동 수정도 없다(`git show --remerge-diff` 0줄).
  - 그래서 이 tree는 81cda519(b08ea644 + T1 harness, 소스 변경 없음)에 네 branch를 RC와 같은 순서로 merge한 것이다.
- 만든 방법: `git merge-tree --write-tree`를 네 번 이어서 돌렸다. 충돌은 없었고 tree는 63b711d7cace08012f296431bc56faac41f31116이다. 분석용 commit 객체만 만들었고 어느 branch에도 남기지 않았다.
- 확인:
  - 이 tree와 RC의 차이 파일은 `81cda519..78191e1a`에서 TUIUX가 바꾼 50개와 정확히 같다.
  - 이 tree에서 그 파일들(문서 둘 제외)은 81cda519와 같다.
  - RC 쪽 같은 파일들은 TUIUX tip과 bytes가 같다.
- 명령: 그 tree에서 `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli/ -count=1 -run '^TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput$'`. writer는 흐름마다 1.1초 띄운 두 run이 같을 때만 쓴다.
- 결과: 바뀐 것은 NPD 다섯 개다(init-yes·init-pipe·init-in 4줄씩, update-yes·update-ci 8줄씩, 모두 2절의 경로). stdout 일곱 개는 b08ea644 기록과 bytes가 같다. `init-in.stdout`도 TUIUX 확인 화면이 없는 base 출력 그대로다. 같은 writer를 RC에서 돌린 출력과 비교하면 단언하는 파일은 모두 같다. 다른 것은 단언하지 않는 `init-in.stdout`(TUIUX 확인 화면)뿐이다.
- 검증:
  - 새 golden으로 RC 8a0da3a1, `fix/v0126-sel`, RC에서 TUIUX를 뺀 tree에서 두 test가 통과한다.
  - 음성 대조: `content/skills/agent-teams.md`에 한 줄을 더하면 NPD 흐름 다섯 개가 다시 실패한다.

## 4. 다음에 다시 기록하는 절차

RC에 TUIUX 밖의 생성 내용 변경이 더 들어오면 golden은 다시 깨진다. 그때는 아래 순서를 따른다.

1. 같은 tree에서 TUIUX를 뺀 tree에서도 같은 test가 같은 흐름으로 실패하는지 본다. 실패하지 않으면 TUIUX 회귀이므로 golden을 고치지 않는다.
2. 그 tree는 81cda519에 TUIUX 밖 변경을 원래 순서로 merge해 만든다. TUIUX가 든 tree에서 갈라진 branch(RC 뒤의 fix branch)는 commit을 그대로 merge하면 그 history의 TUIUX가 다시 들어온다. 그래서 `git merge-tree --write-tree --merge-base=<그 merge의 merge base>`로 branch 자신의 변경만 얹는다. 대조로 tip에서 TUIUX 범위를 되돌린 tree(5절)를 만들어 두 tree가 문서 밖에서 같은지 본다. TUIUX가 아닌 변경이 TUIUX 소스 파일에서 충돌하면 그 변경을 따로 본다.
3. 그 tree에서 writer를 돌려 나온 파일을 가져온다. 검사 대상 tree에서 writer를 직접 돌리지 않는다.

## 5. 최종 재기록 (release/v0.50.126 9fe18fc2)

- 증상: fix branch를 모두 합친 tip에서 같은 다섯 흐름과 S6이 NPD 차이로 실패했다. 단언하는 stdout은 같았다. 853a9a5c에서 한 번 기록한 뒤 tip이 9fe18fc2로 옮겨(teamline a1e5e460·537dab36·580d7ef3, REA dd2df735와 증거 commit) 9fe18fc2에서 다시 기록했다.
- 뺄 TUIUX: SPEC-TUIUX-001(`81cda519..78191e1a`)과 fix/v0126-sel의 229962e0(`NavSelect`, `composeSelect`, 그 test). agy, reasave, omphub, ledger, teamtop은 base에 넣는다.
- 첫 번째 방법(정방향): 3절 tree 63b711d7에 release의 fix merge를 같은 순서로 얹었다. 각 merge에는 그 merge의 merge base를 `--merge-base`로 줬다.
  - agy 714634fa(8a0da3a1), reasave d26839b6(8a0da3a1), omphub 384439a8(8a0da3a1), ledger caeba74a(8a0da3a1), ledger 보안 d8c7b9dc(caeba74a), omphub 리뷰 c6d588d9(384439a8), teamtop f7d98506(8a0da3a1), agy 후속 1a031ec0(714634fa), 그다음 `853a9a5c..9fe18fc2`(`--merge-base=853a9a5c`). sel 80823212는 넣지 않았다.
  - 충돌은 문서에서만 났다(`CHANGELOG.md`, `SPEC-REA-001/spec.md`, `SPEC-TUIUX-001/research.md`). TUIUX와 reasave·agy 후속이 함께 바꾼 `internal/cli/rea_run.go`는 충돌 없이 합쳐졌다.
- 두 번째 방법(되돌리기): `git merge-tree --write-tree --merge-base=78191e1a 9fe18fc2 81cda519`로 SPEC-TUIUX-001을 되돌리고, `--merge-base=229962e0 <그 결과> 8a0da3a1`로 229962e0을 되돌렸다. 결과 tree는 f974358638af4a36b9d1b3bfaeedfcdbefd92b72다.
- 두 tree는 문서(`CHANGELOG.md`, `SPEC-REA-001/spec.md`, SPEC-TUIUX-001 문서·golden)와 harness 머리 주석 밖에서 같다. 그래서 두 번째 tree로 기록했다.
- tree 확인:
  - tip과 base tree의 차이는 문서 밖에서 TUIUX 파일과 정확히 같다(TUIUX가 아닌 경로 0, 같은 TUIUX 파일 0).
  - `rea_run.go`에서 tip→base 차이는 TUIUX를 되돌리는 hunk뿐이다.
  - `quality_compose_form.go`에서 tip→base 차이는 TUIUX를 되돌리는 hunk와 `NavSelect` 줄뿐이다.
  - base tree에는 `tui/nav.go`, `tui/nav_select.go`, `init_review.go`가 없다.
- tip golden(80823212 + caeba74a의 `.gitignore` 줄)을 base tree에 대 보면 같은 다섯 흐름이 실패한다. 실패는 TUIUX 코드에서 온 것이 아니다.
- writer 결과: committed golden과 다른 것은 NPD 다섯 개다(init 흐름 10줄씩, update 흐름 20줄씩). stdout 일곱 개는 b08ea644 기록 그대로다. 같은 writer를 tip에서 돌린 출력과 비교하면 단언하는 파일은 모두 같고, 다른 것은 단언하지 않는 `init-in.stdout`(TUIUX 확인 화면)뿐이다.
- 3절 tree에서 이 base tree까지 바뀐 생성 파일과 그 출처는 아래와 같다. 생성 파일에서 더하거나 뺀 줄은 모두 소스 diff(`8a0da3a1..9fe18fc2`)에 그대로 있다.

| 생성 파일 | 소스 | 줄의 commit(`git blame`) |
|---|---|---|
| `.gitignore` | `internal/cli/init_helpers.go:70-73` `permissionLedgerRel` | caeba74a(ledger) |
| `.claude/agents/autopus/reverse-engineer.md`, `.codex/agents/reverse-engineer.toml` | `content/agents/reverse-engineer.md`, 2줄 | a4031069(agy) |
| `.claude/rules/autopus/deferred-tools.md` | `content/rules/deferred-tools.md`, 6줄 | 2ceb6a0a(teamtop) |
| `.claude/skills/agent-teams/SKILL.md` | `content/skills/agent-teams.md`, 37줄 | 2ceb6a0a 36줄, 85161370 1줄(teamtop) |
| `.claude/skills/auto-go/SKILL.md` | `templates/claude/commands/auto-workflows.md.tmpl`, 36줄과 7줄 | 2ceb6a0a(teamtop) 36줄, 580d7ef3 4줄·a1e5e460 3줄(teamline) |
| `.codex/config.toml` | `pkg/reamcp/shape_codex.go:14` `codexTimeoutLine`(`startup_timeout_sec = 30` → `120`) | dd2df735(REA) |

- manifest와 journal:
  - claude-code manifest는 위 Claude 파일 네 entry의 checksum만, codex manifest는 `reverse-engineer.toml`과 `config.toml` checksum만 다르다.
  - journal은 그 entry의 `after_checksum`만 다르다.
  - `.gitignore` 줄은 caeba74a가 golden에 손으로 고친 값과 base tree 출력이 같다.
- teamtop branch가 f7d98506에서 TUIUX가 든 tree로 다시 쓴 NPD는 release merge에서 HEAD 쪽을 남겨 쓰지 않았다. 이 재기록이 둘을 대신한다.
- 검증:
  - fix/v0126-gold(9fe18fc2 + 이 golden)에서 두 test가 통과한다.
  - 음성 대조: `content/skills/agent-teams.md`에 한 줄을 더하면 다섯 흐름이 실패한다.
  - `go test ./internal/cli/` 전체를 돌렸다. 853a9a5c 기록은 `-timeout 60m`에서 3511.7초로 통과해 한도에 가까웠으므로, 9fe18fc2에서는 `-timeout 90m`으로 돌렸다(결과는 이 재기록 commit 메시지에 적는다).

## 6. learn 저장소 수정 재기록 (fix/learn-store-defects, 2026-10-24)

- 증상: PR #214 CI에서 같은 다섯 흐름과 S6이 NPD 차이로 실패했다. 단언하는 stdout은 같았다.
- 원인: PR #214가 `templates/claude/commands/auto-workflows.md.tmpl`의 learn 명령 세 곳을 바꿨다. `/auto go` Phase 0.8 query에 `--top 5 --mark-reused`, `/auto fix` Step 0 query에 같은 flag, `/auto sync` summary를 `--top 3`으로 바꾸고 그 대체 문구를 고쳤다.
- 기록: TUIUX는 v0.50.126에 이미 들어갔고 5절이 보였듯 형식 없는 흐름의 NPD에 TUIUX 코드가 닿지 않으므로, 이 branch tip에서 `TUIUX_WRITE_GOLDENS=1`로 기록했다. writer가 다시 쓴 stdout golden 일곱 개는 `git checkout`으로 되돌려 b08ea644 기록 그대로 두었다.
- 차이(init 흐름과 update 흐름): `.claude/skills/auto-go/SKILL.md`, `auto-fix/SKILL.md`, `auto-sync/SKILL.md`의 checksum, claude-code manifest의 그 세 entry checksum, claude-code journal의 그 entry `after_checksum`, update 흐름의 backup 사본. 다른 경로는 바뀌지 않았다.
- 검증: 두 test가 이 golden으로 통과한다.

## 7. SPEC-LEARN-003 재기록 (feat/learn-evidence-scope, 2026-10-24)

- 원인: SPEC-LEARN-003이 Claude `/auto fix` Step 5와 `/auto sync` Target 4.5, Codex `auto-go`·`auto-fix`·`auto-sync` 템플릿에 학습 조회·기록·정리 문구를 더했다. Gemini 템플릿도 바꿨지만 golden 흐름은 Claude와 Codex만 생성한다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer, stdout golden 되돌림).
- 차이: `.claude/skills/auto-fix`·`auto-sync`, `.codex/skills/codex-auto-go`·`codex-auto-fix`·`codex-auto-sync`의 checksum, 두 manifest의 그 entry, 두 journal의 `after_checksum`, update 흐름의 backup 사본. 다른 경로는 바뀌지 않았다.
- 검증: 두 test가 이 golden으로 통과한다.
