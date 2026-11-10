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

## 8. REA npx `--prefix=/` 재기록 (fix/rea-npm-registry, 2026-10-27)

- 원인: SPEC-REA-001 보안 감사 L4가 REA managed 항목의 실행 형식에 `--prefix=/`를 더했다(`pkg/reamcp/shape.go` `jsonScriptPrefix`, `shape_codex.go` `codexArgsPrefix`). 템플릿·content는 그대로이고 생성 `.mcp.json`과 `.codex/config.toml`의 rea 항목 bytes만 바뀐다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer를 돌리고, writer가 다시 쓴 `init-in.stdout`은 `git checkout`으로 되돌림).
- 차이: init 흐름(init-in·init-pipe·init-yes)은 `.mcp.json`, `.codex/config.toml`, 두 manifest, 두 journal의 checksum 6줄씩이다. update 흐름(update-ci·update-yes)은 거기에 두 번째 transaction의 journal 두 줄과 backup 사본 네 줄(`.mcp.json`, `.codex/config.toml`, 두 manifest)을 더한 12줄씩이다. 다른 경로는 바뀌지 않았다.
- 검증: 기록 전 다섯 흐름이 정확히 이 경로들의 차이로 실패했고, 기록 뒤 test가 통과한다.

## 9. Lore 타입 접두사 정렬 재기록 (fix/lore-type-prefix-consistency, 2026-10-27)

- 원인: 외부 기여 PR #192가 `content/hooks/pre-commit-lore.sh`의 허용 타입을 `auto check --lore`(`internal/cli/check_rules.go` `loreValidTypes`)와 맞추고, `content/rules/lore-commit.md`·`content/skills/lore-commit.md`에 merge·revert 타입과 scope 필수 규칙을 더했다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer, stdout golden 되돌림).
- 차이: init 흐름은 `.claude/hooks/autopus/pre-commit-lore.sh`, `.claude/hooks/autopus/conditional/lore-commit.md`, claude-code manifest, claude-code journal의 checksum 4줄씩이다. update 흐름은 거기에 두 번째 journal 줄과 backup 사본 세 줄을 더한 8줄씩이다. Codex·Gemini lore skill은 이 흐름이 생성하지 않는다. 다른 경로는 바뀌지 않았다.
- 검증: 두 test가 이 golden으로 통과한다.

## 10. qualityloop 삭제 재기록 (chore/remove-qualityloop, 2026-10-29)

- 원인: 운영 코드가 부르지 않던 `pkg/qualityloop`를 지우면서 reviewer agent, review skill, auto-go·auto-review 템플릿의 "`qualityloop`/`skillevolve` candidate" 문구를 `skillevolve`만 남기도록 고쳤다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer, stdout golden 되돌림).
- 차이: `.claude/agents/autopus/reviewer.md`, `.claude/skills/auto-go`·`review`, `.codex/agents/reviewer.toml`, `.codex/skills/codex-auto-go`·`codex-auto-review`·`codex-review`의 checksum, 두 manifest의 그 entry, 두 journal의 `after_checksum`, update 흐름의 backup 사본. 다른 경로는 바뀌지 않았다.
- 검증: 두 test가 이 golden으로 통과한다.

## 11. 자율 공격 대비 점검 재기록 (feat/security-audit-autonomous-lens, 2026-10-30)

- 원인: `security-audit` 스킬에 "자율 공격 대비 점검" 절을, `security-auditor` agent에 감사 범위와 "운영 설계 질문" 출력을 더했다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer, stdout golden 되돌림).
- 차이: `.claude/agents/autopus/security-auditor.md`, `.claude/skills/security-audit`, `.codex/agents/security-auditor.toml`, `.codex/skills/codex-security-audit`의 checksum, 두 manifest의 그 entry, 두 journal의 `after_checksum`, update 흐름의 backup 사본. 다른 경로는 바뀌지 않았다.
- 검증: 두 test가 이 golden으로 통과한다.

## 12. SPEC lean profile 재기록 (feat/spec-plan-latency, 2026-10-31)

- 원인: plan 워크플로의 Compact Path Gate에 저위험 신규 SPEC용 lean profile 분기를 더하고, spec-writer agent에 "Lean Profile" 절을 더했다. 뒤이은 커밋이 같은 파일에 `surface` 선언 요구를 더해 다시 기록했다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer, stdout golden 되돌림).
- 차이: `.claude/agents/autopus/spec-writer.md`, `.claude/skills/auto-plan`, `.codex/agents/spec-writer.toml`, `.codex/skills/codex-auto-plan`의 checksum, 두 manifest의 그 entry, 두 journal의 `after_checksum`, update 흐름의 backup 사본. 다른 경로는 바뀌지 않았다.
- 검증: 두 test가 이 golden으로 통과한다.

## 13. 리뷰 model 안내 줄 재기록 (fix/review-model-ux, 2026-10-31)

- 원인: 터미널이 없어 멀티프로바이더 리뷰 탭을 열 수 없는 실행이 리뷰 model을 바꾸는 곳을 한 줄로 알린다. `auto quality`의 TTY 오류는 오류 앞 stderr에 `참고: 리뷰 model은 터미널에서 auto quality로, 또는 auto quality review model <provider> <model>로 바꿉니다.`를 쓴다. 이 줄은 리뷰 설정이 있는 설정(`spec.review_gate.enabled`이고 `orchestra.providers`가 둘 이상)에서만 나온다.
- 기록: 이 branch tip에서 writer를 돌렸다. 바뀐 줄은 TUIUX 코드가 아니라 이 branch가 의도한 출력이므로 tip에서 기록한 `quality-no-tty.stdout`을 그대로 두었다. writer가 다시 쓴 `init-in.stdout`(단언하지 않는 TUIUX 확인 화면)은 `git checkout`으로 되돌렸다.
- 차이: `quality-no-tty.stdout`에 위 한 줄이 TTY 오류 앞에 더해졌다(fixture D는 리뷰 게이트가 켜져 있고 provider가 셋이다). init-yes·update-yes·update-ci는 이 환경의 PATH에 provider가 없어 Review Gate가 꺼지므로(`false (0 providers)`) 안내 줄이 없다. quality-line-b와 init-pipe는 대화형 경로다. 이 다섯 stdout과 NPD 일곱 개는 바뀌지 않았다.
- 검증: 두 test가 이 golden으로 통과한다. stdout golden 가운데 b08ea644 기록과 다른 것은 이 한 줄뿐이다.

## 14. 품질 스킬의 auto quality review 재기록 (fix/review-model-ux, 2026-11-01)

- 원인: `adaptive-quality`·`using-autopus` 스킬에 터미널 없이 리뷰 model을 보고 바꾸는 `auto quality review` 절을 더했다. Codex·Gemini 템플릿은 `go run ./cmd/generate-templates`로 다시 만들었다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer, stdout golden 되돌림). 되돌린 stdout은 13절의 `quality-no-tty.stdout`을 포함해 직전 commit의 기록 그대로다.
- 차이: `.claude/skills/using-autopus`, `.codex/skills/codex-using-autopus`의 checksum, 두 manifest의 그 entry, 두 journal의 `after_checksum`, update 흐름의 backup 사본. `adaptive-quality`는 기본 compact catalog가 생성하지 않아 이 흐름에 없다. 다른 경로는 바뀌지 않았다.
- 검증: 두 test가 이 golden으로 통과한다.

## 15. 디스크 사전 점검 안내 재기록 (feat/disk-management, 2026-11-02)

- 원인: SPEC-DISK-001 REQ-DISK-GUIDE-01이 agent-pipeline의 delegation·verification reference, worktree-isolation 스킬, codex worktree-isolation native 본문에 `auto disk check --need` 사전 점검(exit 0에서만 진행)을 더했다. delegation의 `df`·5 GiB 문구는 대체됐다.
- 기록: 6절과 같은 방법이다(#247이 들어간 main 위로 rebase한 branch tip에서 writer, stdout golden 되돌림).
- 차이: `.claude/skills/agent-pipeline/references/delegation.md`·`verification.md`, `.claude/skills/worktree-isolation/SKILL.md`, `.codex/skills/codex-agent-pipeline/references/delegation.md`·`verification.md`, `.codex/skills/codex-worktree-isolation/SKILL.md`의 checksum, 두 manifest의 그 entry, 두 journal의 `after_checksum`, update 흐름의 backup 사본. 다른 경로는 바뀌지 않았다.
- 검증: 두 test가 이 golden으로 통과한다.

## 16. 디스크 회수·lease 안내 재기록 (feat/disk-reclaim, 2026-11-02)

- 원인: SPEC-DISK-002 REQ-DISK2-GUIDE-01이 같은 안내 면에 `auto disk register`·`release`로 harness 밖 worktree를 등록·해제하고, 전체 빌드·테스트를 `auto disk lease --need <GiB> -- <command>`로 돌리고, 디스크 경보나 사전 점검 실패 때 lead가 worker를 멈추게 하는 문장을 더했다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer, stdout golden 되돌림).
- 차이: 15절과 같은 여섯 생성 파일의 checksum, 두 manifest의 그 entry, 두 journal의 `after_checksum`, update 흐름의 backup 사본. 다른 경로는 바뀌지 않았다.
- 검증: 두 test가 이 golden으로 통과한다.

## 17. pipeline 수정 루프·재개 안내 재기록 (fix/pipeline-resume-retry-contract, 2026-11-02)

- 원인: SPEC-PIPELINE-RESUME-001(PR #251)이 agent-pipeline reference의 `phases.md`(Gate 2의 `auto pipeline run` 수정·재검증 루프와 `circuit_break_no_progress`·`failed_budget` 종료, Route selection 아래의 `pipeline_checkpoint.v2` 재개)와 `review.md`(Retry budget의 엔진 대응)에 절을 더했다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer). writer가 다시 쓴 `init-in.stdout`(단언하지 않는 TUIUX 확인 화면)은 `git checkout`으로 되돌렸다.
- 차이: `.claude/skills/agent-pipeline/references/phases.md`·`review.md`, `.codex/skills/codex-agent-pipeline/references/phases.md`·`review.md`의 checksum(두 플랫폼이 같은 값), 두 manifest의 그 entry, 두 journal의 `after_checksum`, update 흐름의 backup 사본. quality 두 흐름과 stdout golden은 바뀌지 않았다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`과 `TestInitReview_*`가 이 golden으로 통과한다.
- 재기록 시점: main(#249, 16절)을 이 branch에 merge한 뒤 다시 기록했다.

## 18. SPEC 리뷰 정책 재기록 (feat/spec-review-policy-budget, 2026-11-02)

- 원인: SPEC-REVIEWPOLICY-001 REQ-RP-POL-09가 spec-writer agent, spec-review 스킬, spec-quality 체크리스트, auto-plan 워크플로(Claude·Codex·Gemini)에 lean floor와 lean allowed omissions 줄을 더하고, spec-review 스킬에 리뷰어·judge·runtime이 공유하는 판정 규칙을 적었다. Codex·Gemini 템플릿은 `go run ./cmd/generate-templates`로 다시 만들었다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer, stdout golden 되돌림).
- 차이: `.claude/agents/autopus/spec-writer.md`, `.claude/skills/auto-plan`·`spec-review`, `.claude/hooks/autopus/conditional/spec-quality.md`, `.codex/agents/spec-writer.toml`, `.codex/skills/codex-auto-plan`·`codex-spec-review`의 checksum, 두 manifest의 그 entry, 두 journal의 `after_checksum`, update 흐름의 backup 사본. 다른 경로는 바뀌지 않았다.
- 검증: 두 test가 이 golden으로 통과한다.
- 추가 재기록(R7 커밋): spec-review 스킬의 CLI Usage에 `--profile`·`--judge`와 리뷰 계획 출력 설명을 더해 다시 기록했다. 차이는 `.claude/skills/spec-review`, `.codex/skills/codex-spec-review`의 checksum과 두 manifest·journal 항목뿐이다.
- 추가 재기록(R10 커밋): spec-review 스킬에 리뷰 세션 예산(`review-session.json`, `--max-calls`·`--extend-budget`·`--new-session`, `spec.review_gate.budget`) 설명을 더해 다시 기록했다. 차이는 `.claude/skills/spec-review`, `.codex/skills/codex-spec-review`의 checksum과 두 manifest·journal 항목뿐이다.
- 추가 재기록(.gitignore 커밋): 관리 `.gitignore` 패턴(`gitignorePatterns`)에 `**/.autopus/specs/**/review-session.json`과 `**/.autopus/specs/**/.review-session.lock`을 더해 다시 기록했다. 차이는 다섯 NPD의 `.gitignore` checksum 한 줄뿐이고 stdout golden은 바뀌지 않았다.
- 재기록 시점: main(#249 16절, #251 17절)을 이 branch에 merge한 뒤 다시 기록했다.

## 19. init 다음 단계 안내 재기록 (feat/status-execution-evidence, 2026-11-02)

- 원인: SPEC-STATUSEVIDENCE-001 REQ-SEV-NEXT-03이 `auto init` 요약 뒤의 다음 단계 안내를 바꿨다. worker 설정이 없을 때 Desktop 연결만 권하던 `Next: open Autopus Desktop Connect ...` 줄 대신, 설정된 플랫폼마다의 문법으로 새 작업(`/auto plan`, `$codex-auto-plan`)과 SPEC 없는 작은 수정(`/auto fix`, `$codex-auto-fix`)을 먼저 안내하고, Desktop 연결(`auto connect`)은 마지막 `Optional:` 줄에서 선택 사항으로 안내한다. 단계마다 둘째 줄에 status의 다음 명령과 같은 사실(실행 디렉터리, 실행 위치, provider, 외부 호출, 쓰기)을 적는다. 안내는 설정만 읽고 기계의 설치 상태는 읽지 않는다.
- 기록: main(#249, #250, #251 merge 포함) 위로 rebase한 이 branch tip에서 writer를 돌렸다. 바뀐 줄은 이 branch가 의도한 출력이므로 `init-yes.stdout`과 `init-pipe.stdout`을 그대로 두었다. writer가 다시 쓴 `init-in.stdout`(단언하지 않는 TUIUX 확인 화면)은 `git checkout`으로 되돌렸다. 실행 디렉터리는 `<DIR>`로 정규화된다. Desktop 단계의 첫 줄은 `cd` 줄이 아닌 `auto connect`만 보인다. 임시 디렉터리 이름에 따라 따옴표가 붙는 `cd` 줄은 golden을 흔들기 때문이다.
- 차이: 두 stdout에서 `Next:` 한 줄이 여섯 줄(새 작업, 작은 수정, 선택 사항인 Desktop과 각각의 사실 줄)로 바뀌었다. 두 흐름 모두 플랫폼이 `claude-code`, `codex`라 두 문법이 함께 나온다. NPD 일곱 개와 나머지 stdout golden은 바뀌지 않았다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`과 `TestInitReview_*`가 이 golden으로 세 번 연속 통과한다.
- 재기록 시점: main(#252, 18절)을 이 branch에 merge한 뒤 다시 확인했다.

## 20. judge model 안내 재기록 (feat/review-judge-model, 2026-11-02)

- 원인: SPEC-JUDGEMODEL-001 REQ-JM-DOC-01이 `spec-review` 스킬(리뷰 계획 문단과 설정 예시의 `judge_model`·`judge_effort`)과 `using-autopus` 스킬(멀티프로바이더 리뷰 model 절의 `auto quality review judge <provider> --model ...`)에 judge model 설명을 더했다. `adaptive-quality` 스킬과 Codex·Gemini 템플릿 사본도 같은 글로 고쳤다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer, `init-in.stdout`은 `git checkout`으로 되돌림).
- 차이: `.claude/skills/spec-review`·`using-autopus`, `.codex/skills/codex-spec-review`·`codex-using-autopus`의 checksum, 두 manifest의 그 entry, 두 journal의 `after_checksum`, update 흐름의 backup 사본. stdout golden과 다른 경로는 바뀌지 않았다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`이 이 golden으로 통과한다.

## 21. agy stream-json 기본 인자 재기록 (fix/agy-stream-json, 2026-11-05)

- 원인: gemini(agy) orchestra provider 기본 인자에 `--output-format stream-json`을 더했다(`DefaultGeminiProviderEntry`). 생성되는 `autopus.yaml`의 `orchestra.providers.gemini.args`가 `[--print, "", --output-format, stream-json]`이 된다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer, `init-in.stdout`은 HEAD로 되돌림).
- 차이: 일곱 NPD의 `autopus.yaml` checksum 한 줄씩뿐이다. stdout golden과 다른 경로는 바뀌지 않았다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`과 `TestInitReview_*`가 이 golden으로 통과한다.

## 22. claude json 기본 인자 재기록 (fix/agy-stream-json, 2026-11-05)

- 원인: claude orchestra provider 기본 인자에 `--output-format json`을 더했다(`DefaultClaudeProviderEntry`). 생성되는 `autopus.yaml`의 `orchestra.providers.claude.args`가 `[--print, --model, claude-fable-5-1, --effort, max, --output-format, json]`이 된다.
- 기록: 6절과 같은 방법이다(이 branch tip에서 writer, `init-in.stdout`은 HEAD로 되돌림).
- 차이: 일곱 NPD의 `autopus.yaml` checksum 한 줄씩뿐이다. stdout golden과 다른 경로는 바뀌지 않았다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`과 `TestInitReview_*`가 이 golden으로 통과한다.

## 23. 리뷰 provider claude 모델 줄 재기록 (fix/claude-review-model-no-silent-upgrade, 2026-11-06)

- 원인: 구성 표를 그릴 때마다 표 아래에 리뷰 provider claude 모델이 Claude Code 프리셋과 별개 설정임을 알리는 한 줄을 더했다(`리뷰 provider claude 모델 = claude-fable-5-1 (Claude Code 프리셋과 별개 설정 · 변경: auto quality review model claude <model>)`). 프리셋의 최상위 Claude 등급과 그 모델의 등급이 다르면 `주의:` 줄이 된다. 모델은 autopus.yaml 원문의 `--model` 값에서만 읽으므로 `${NAME}` 값이나 원문으로 읽을 수 없는 설정에서는 줄이 없다.
- 기록: 이 branch tip에서 writer를 돌렸다. 바뀐 줄은 이 branch가 의도한 출력이므로 `quality-line-b.stdout`을 그대로 두었다. writer가 다시 쓴 `init-in.stdout`(단언하지 않는 TUIUX 확인 화면)은 `git checkout`으로 되돌렸다.
- 차이: `quality-line-b.stdout`의 첫 표 아래 `Codex catalog:` 줄 뒤에 위 한 줄이 더해졌다. init-pipe는 표를 그릴 때 autopus.yaml이 아직 없어 줄이 없고, init-yes·update-yes·update-ci·quality-no-tty는 표를 그리지 않는다. NPD 일곱 개와 나머지 stdout golden은 바뀌지 않았다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`이 이 golden으로 통과한다.

## 24. balanced 최상위와 claude 리뷰 기본 모델 Opus 재기록 (feat/balanced-opus-default, 2026-11-06)

- 원인: balanced 최상위 rung의 Anthropic 후보가 `anthropic/claude-fable-5-1`(max)에서 `anthropic/claude-opus-5-5`(max)로, 최상위 등급이 `fable`에서 `opus`로 바뀌었다(`balancedRungCandidates`, `balancedTierForRung`). claude orchestra provider 기본 인자도 `[--print, --model, claude-opus-5-5, --effort, max, --output-format, json]`이 된다(`claudeDefaultModelArgs`). Codex·OpenAI rung과 Ultra는 그대로다.
- 기록: 6절과 같은 방법이다. 이 branch tip에서 `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`로 writer를 돌리고, writer가 다시 쓴 `init-in.stdout`(단언하지 않는 TUIUX 확인 화면)은 `git checkout`으로 되돌렸다. S16 parity golden은 `AUTOPUS_PANERM_S16_CAPTURE=1 go test ./internal/cli -run TestPanermS16`로 다시 기록했다. quality_compose 표 golden은 `-update-compose-table`로, writer가 없는 `line-quality-D-*.golden`은 저장소 파일을 고치지 않는 `go test -overlay` 사본으로 실제 출력을 써서 다시 기록했다.
- 차이:
  - stdout: `init-pipe.stdout`과 `quality-line-b.stdout`의 구성 표에서 최상위 7개 에이전트 줄이 `fable   fable-5-1 · max`에서 `opus    opus-5-5 · max`가 됐다. Codex 칸(`6-astra · max`)은 그대로다. `quality-line-b.stdout`의 리뷰 provider 줄은 `claude-opus-5-5`가 됐다.
  - NPD: init-in·init-pipe·init-yes·update-yes·update-ci·quality-line-b·quality-no-tty의 `autopus.yaml`, 최상위 7개 `.claude/agents/autopus/*.md`, manifest·journal checksum, 그리고 문서를 고친 `using-autopus` skill checksum이 바뀌었다.
  - S16: `brainstorm`·`plan`·`recheck`·`review`·`run`·`secure`·`spec-review-receipt` golden의 claude argv와 receipt model이 `claude-opus-5-5`, args hash가 그에 맞는 값이 됐다. 다른 줄은 바뀌지 않았다.
  - quality_compose: D·B2·H·O1·P 표와 D→K 미리보기, `line-quality-D-global`·`line-quality-D-groups`의 최상위 칸이 opus로 바뀌었다. H(HB1)는 적어 둔 `opus`가 이제 최상위 등급과 같아 표준 배치(`opus-5-5 · max`, `6-astra · max`)를 받는다. K와 D→K 미리보기에서 유지한 opus 최상위 rung 에이전트는 custom에서도 최상위 rung 모델(`opus-5-5 · max`, `6-astra · max`)을 그대로 써서 바뀌는 칸이 아니다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`, `TestPanermS16_*`, `TestComposeTable_Golden*`, `TestComposeLineInput_MatchesTheSPECMODELUX001Output`이 이 golden으로 통과한다.

## 역할 그룹을 생성 에이전트로 줄인 뒤 (2026-11-07)

- 원인: 구성 화면의 역할 그룹(`composeRoleGroups`)이 생성하지 않는 레거시 역할 14개를 담고 있었다. 그룹은 이제 구현(frontend-specialist)과 검증·리뷰(reviewer, qa-explorer, ux-validator) 두 개다.
- 기록: `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestInitReview_S6|TUIUX|Tuiux'`로 writer를 돌리고, 단언하지 않는 `init-in.stdout`은 `git checkout`으로 되돌렸다. quality_compose 표와 줄 입력 golden은 `-update-compose-table`로 다시 썼다(줄 입력 golden도 이제 이 flag로 다시 쓴다).
- 차이: 구성 표가 네 행(구현 1, 검증·리뷰 3)이 되고 qa-explorer 행이 생겼다. NPD는 `autopus.yaml`과 생성 에이전트 파일·manifest checksum이 바뀌었다. 빈 그룹이나 삭제된 에이전트 행은 없다.

## REA MCP opt-in 재기록 (fix/v051-followups, 2026-11-07)

- 원인: 설치되는 에이전트 중 REA MCP를 쓰는 것이 없어 REA를 opt-in으로 바꿨다. `--yes` init은 이제 REA를 정하지 않은 채로 두고 아무것도 등록하지 않으며(`decideInit`), 정하지 않은 update는 hint 한 줄을 쓴다.
- 기록: `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`로 writer를 돌리고, 단언하지 않는 `init-in.stdout`은 `git checkout`으로 되돌렸다.
- 차이:
  - stdout: `init-yes.stdout`에서 `REA 역공학 MCP를 등록했습니다: …` 줄이 빠지고, `update-yes.stdout`·`update-ci.stdout`에 `REA 역공학 MCP를 아직 정하지 않았습니다. …` hint 줄이 더해졌다. 다른 줄은 그대로다.
  - NPD: init-yes·update-yes·update-ci의 `.mcp.json`·`.codex/config.toml`과 그 backup, manifest·journal checksum이 rea 항목이 없는 값으로 바뀌었다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`이 이 golden으로 통과한다.

## writer가 단언하지 않는 stdout을 쓰지 않는다 (ci/darwin-lint-golden, 2026-11-07)

- 원인: writer가 모든 flow의 `.stdout`을 썼다. `init-in`은 stdout을 단언하지 않는 flow(`stdout: false`)라 그 출력에 Desktop 설치 상태나 로컬 모델 catalog 같은 기계 의존 줄이 그대로 들어갔고, 재기록할 때마다 `git checkout`으로 되돌려야 했다.
- 변경: writer는 `stdout: true`인 flow의 `.stdout`만 쓴다. 어떤 테스트도 읽지 않는 `init-in.stdout`은 지웠다. `init-in.npd`는 `TestInitReview_*`가 읽으므로 그대로다.
- 검증: `TUIUX_WRITE_GOLDENS=1`로 writer를 돌린 뒤 `git status`에 golden 변경이 없고, `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 통과한다.

## QA 스킬에 `auto qa intent` 흐름 추가 (feat/qa-intent, 2026-11-07)

- 원인: `auto-qa`와 `qa-autopilot` 스킬(Claude·Codex)에 SPEC 없이 요청에서 의도를 만드는 흐름을 더했다.
- 기록: `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6'`. writer는 이제 단언하는 stdout만 써서 되돌릴 파일이 없다.
- 차이: NPD 다섯 개에서 `.claude/skills/{auto-qa,qa-autopilot}/SKILL.md`, `.codex/skills/{codex-auto-qa,codex-qa-autopilot}/SKILL.md`와 그 backup, manifest·journal checksum만 바뀌었다. stdout golden은 바뀌지 않았다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 이 golden으로 통과한다.

## qa-autopilot 스킬에 exec case 설명 추가 (feat/qa-exec-check, 2026-11-08)

- 원인: `content/skills/qa-autopilot.md`에 CLI 제품을 직접 돌리는 `exec` case와 에이전트 비용 상한 설명을 더했다.
- 기록: `make generate-templates` 뒤 `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6'`.
- 차이: NPD 다섯 개에서 `qa-autopilot` 스킬(Claude·Codex)과 그 backup, manifest·journal checksum만 바뀌었다. stdout golden은 그대로다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 통과한다.

## qa-autopilot 스킬에 stdout_lines·CLI 프로젝트 설명 추가 (fix/qa-go-cli-exec, 2026-11-08)

- 원인: `content/skills/qa-autopilot.md`에 exec `stdout_lines`·`stdout_not_contains`와 웹 UI 없는 프로젝트에서의 `auto qa go` 동작 설명을 더했다.
- 기록: `make generate-templates` 뒤 `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6'`.
- 차이: NPD 다섯 개에서 `qa-autopilot` 스킬(Claude·Codex)과 그 backup, manifest·journal checksum만 바뀌었다. stdout golden은 그대로다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 통과한다.

## intent 질문 답을 `auto qa intent answer`로 기록 (feat/qa-intent-answers, 2026-11-08)

- 원인: `auto-qa`·`qa-autopilot` 스킬(Claude·Codex·Gemini)이 답을 acceptance.md에 직접 반영하는 대신 `auto qa intent answer`로 기록하게 바꿨다.
- 기록: `make generate-templates` 뒤 `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6'`.
- 차이: NPD 다섯 개에서 해당 스킬과 그 backup, manifest·journal checksum만 바뀌었다. stdout golden은 그대로다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 통과한다.

## qa-autopilot 스킬에 build 실패 분류 설명 추가 (feat/qa-build-triage, 2026-11-08)

- 원인: `content/skills/qa-autopilot.md`의 `auto qa exec` 종료 코드 설명에 제품 컴파일 에러가 1로 끝난다는 문장을 더했다.
- 기록: `make generate-templates` 뒤 `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6'`.
- 차이: NPD 다섯 개에서 `qa-autopilot` 스킬과 그 backup, manifest·journal checksum만 바뀌었다. stdout golden은 그대로다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 통과한다.

## qa-autopilot 스킬에 재실행 시 시나리오 복원 설명 추가 (fix/qa-remaining, 2026-11-08)

- 원인: `content/skills/qa-autopilot.md`의 loop 설명에 SPEC 없는 재실행이 최근 loop 브랜치의 시나리오를 되살린다는 문장을 더했다.
- 기록: `make generate-templates` 뒤 `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6'`.
- 차이: NPD 다섯 개에서 `qa-autopilot` 스킬과 그 backup, manifest·journal checksum만 바뀌었다. stdout golden은 그대로다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 통과한다.

## 응답 언어 Stop hook 추가 (feat/language-stop-hook, 2026-11-08)

- 원인: `language.ai_responses`가 `ko`인 fixture에서 Claude Code `settings.json`에 Stop hook `auto verify language`가 더해졌다.
- 기록: `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6'`.
- 차이: NPD 다섯 개에서 `.claude/settings.json`과 manifest·journal checksum만 바뀌었다. stdout golden은 그대로다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 통과한다.

## qa-autopilot 스킬에 `qa exec --all` 회귀 검사 설명 추가 (feat/qa-regression-gate, 2026-11-09)

- 원인: `content/skills/qa-autopilot.md`에 `auto qa exec --all`과 Stop 게이트 연결을 설명하는 항목을 더했다.
## qa-autopilot 스킬에 `qa mutate` 설명 추가 (feat/qa-mutate, 2026-11-09)

- 원인: `content/skills/qa-autopilot.md`에 `auto qa mutate`를 설명하는 항목을 더했다.
- 기록: `make generate-templates` 뒤 `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6'`.
- 차이: NPD 다섯 개에서 `qa-autopilot` 스킬과 그 backup, manifest·journal checksum만 바뀌었다. stdout golden은 그대로다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 통과한다.

## qa-autopilot 스킬에 `qa go --resume` 설명 추가 (feat/qa-resume, 2026-11-09)

- 원인: `content/skills/qa-autopilot.md`의 재실행 설명에 `--resume`을 더했다.
## qa-autopilot 스킬에 서버형 case(`requests`) 설명 추가 (feat/qa-http-service, 2026-11-09)

- 원인: `content/skills/qa-autopilot.md`에 서버형 제품용 exec `requests`를 설명하는 항목을 더했다.
- 기록: `make generate-templates` 뒤 `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6'`.
- 차이: NPD 다섯 개에서 `qa-autopilot` 스킬과 그 backup, manifest·journal checksum만 바뀌었다. stdout golden은 그대로다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 통과한다.

## qa-autopilot 스킬에 서비스 재시작 단계 설명 추가 (feat/qa-service-restart, 2026-11-10)

- 원인: `content/skills/qa-autopilot.md`의 서버형 case 설명에 `restart: true` 단계를 더했다.
- 기록: `make generate-templates` 뒤 `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6'`.
- 차이: NPD 다섯 개에서 `qa-autopilot` 스킬과 그 backup, manifest·journal checksum만 바뀌었다. stdout golden은 그대로다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 통과한다.

## 긴 작업 에이전트의 기본 maxTurns 제거 (fix/agent-max-turns-default, 2026-11-10)

- 원인: `content/agents/{qa-explorer,reviewer,frontend-specialist}.md`에서 `maxTurns`를 지우고, 네 에이전트 본문에 결과를 먼저 남기고 한 명령 안에서 기다리라는 지침을 더했다 (#301).
- 기록: `make generate-templates` 뒤 `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6'`.
- 차이: NPD 다섯 개에서 네 에이전트의 `.claude/agents/autopus/*.md`·`.codex/agents/*.toml`과 manifest·journal checksum만 바뀌었다. stdout golden은 그대로다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 통과한다.

## secure 라우트와 secure-hunt 스킬 복원 (feat/secure-hunt, 2026-11-11)

- 원인: `content/skills/secure-hunt.md`를 core 스킬로 더하고, `/auto secure` 라우트(`auto-secure`)를 다섯 플랫폼에 다시 설치했다. 라우터 표와 codex plugin 설명에도 `secure`가 들어갔다.
- 기록: `go run ./cmd/generate-templates` 뒤 `TUIUX_WRITE_GOLDENS=1 go test ./internal/cli -run 'TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput|TestInitReview_S6'`.
- 차이: NPD 다섯 개에서 `.claude/skills/{auto-secure,secure-hunt}`·`.codex/skills/codex-{auto-secure,secure-hunt}` 추가, router 스킬과 codex plugin.json, manifest·journal checksum만 바뀌었다. stdout golden은 그대로다.
- 검증: `TestTUIUXBaseGoldens_FormlessPlacesKeepTheBaseOutput`·`TestInitReview_*`가 통과한다.
