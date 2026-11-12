<div align="center">

# 🐙 Autopus-ADK

### AI가 바꾼 코드를, 직접 돌려 보고 확인합니다.

Autopus-ADK는 AI 코딩 CLI(Claude Code, Codex, Antigravity CLI, OpenCode, Oh My Pi)에 검증 하네스를
설치합니다. 계획과 구현은 지금처럼 에이전트가 합니다. Autopus는 세션이 끝나기 전에 그 결과를 빌드하고
테스트하고, diff를 리뷰하고, 별도의 QA 에이전트가 제품을 실제로 돌려 버그를 찾게 합니다.

[![GitHub Stars](https://img.shields.io/github/stars/autopus-ai/autopus-adk?style=social)](https://github.com/autopus-ai/autopus-adk/stargazers)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![Platforms](https://img.shields.io/badge/Platforms-5-orange)](#platforms)
[![Agents](https://img.shields.io/badge/Agents-4-blueviolet)](#installed)
[![Skills](https://img.shields.io/badge/Skills-9-ff69b4)](#installed)

```bash
# macOS / Linux
curl -sSfL https://raw.githubusercontent.com/autopus-ai/autopus-adk/main/install.sh | sh

# Windows (CMD 또는 PowerShell)
powershell -c "irm https://raw.githubusercontent.com/autopus-ai/autopus-adk/main/install.ps1 | iex"
```

[왜 검증인가](#why) · [Stop hook](#stop-hook) · [QA 버그 헌트](#qa-hunt) · [라우트](#routes) · [설치](#install) · [설정](#configuration) · [보안](#security)

[🇺🇸 English](../README.md)

</div>

---

<a id="why"></a>
## 왜 워크플로가 아니라 검증인가

v0.50.x까지 Autopus는 구현 하네스 전체를 설치했습니다. plan/SPEC 문서, `/auto go`, `/auto fix`,
`/auto idea`, 17개 에이전트, 각종 작업 규율 규칙이 그것입니다. 그런데 이제 frontier 모델(Opus급)은
혼자서도 계획과 구현을 잘 해내고, 그 틀은 대부분 비용만 늘렸습니다.

작은 파일럿도 같은 방향을 가리켰습니다. 숨겨진 테스트로 채점하는 실제 upstream PR 과제 4개에서
아무것도 얹지 않은 Claude Code는 약 17분, $3.00로 4/4를 통과했고, 예전 full 하네스는 약 3.6시간 동안
최소 $37를 쓰고 0/4였습니다. 하네스의 plan 단계가 모든 과제에서 시간 상한에 걸려 비용을 남기지 못했으므로
$37는 하한입니다. 작은 Go 변경 4건(n=4)이라 벤치마크가 아니라 방향을 보여 주는 결과입니다.

에이전트가 여전히 못하는 일은 자기 결과를 확인하는 것입니다. 빌드되지 않는 코드, 돌리지 않은 테스트,
한 번도 실행해 보지 않은 동작을 두고 "완료"라고 보고합니다. 그래서 하네스는 이제 한 가지 일만 합니다.
변경을 실제로 돌려 보고, 버티지 못하면 에이전트를 다시 일하게 합니다.

## 하는 일

| 단계 | 무엇이 도는가 | 언제 |
| --- | --- | --- |
| 빌드·테스트 | `auto verify stop`: 바뀐 파일이 속한 toolchain만 | Claude Code 세션이 바뀐 작업 트리를 둔 채 끝나려 할 때마다 |
| 리뷰 | `reviewer` 에이전트가 `git diff HEAD`를 검토 | `/auto review`로 직접, 또는 opt-in 후속 라운드에서 |
| QA 버그 헌트 | `qa-explorer` 에이전트가 제품을 조작하고, `auto qa hunt verify`가 repro를 다시 돌림 | `/auto qa`로 직접, 또는 opt-in 후속 라운드에서 실행할 수 있는 동작이 있을 때 |
| 수정 | 작업하던 에이전트가 blocker와 확인된 버그를 고치고 테스트를 다시 돌림 | 같은 라운드 |

빌드·테스트 게이트는 사용자가 시작하라고 말할 필요가 없습니다. 리뷰와 QA는 비용이 훨씬 커서, 직접
부르거나 후속 라운드를 켰을 때만 자동으로 돕니다. 어느 쪽이든 에이전트는 정말 사용자가 정해야 하는
결정만 객관식으로 묻도록 지시받습니다.

<a id="stop-hook"></a>
## Stop hook: 세션이 끝나기 전에 빌드하고 테스트한다

`auto init`은 Claude Code에 `Stop` hook `auto verify stop`을 등록합니다. 에이전트가 세션을 끝내려 할 때
작업 트리가 `HEAD`와 다르면 hook은 다음을 합니다.

1. 바뀐 파일이 속한 toolchain의 명령만 고릅니다.
   - **Go**: `go build ./...`, 그리고 바뀐 패키지의 `go test`
   - **Node**: 프로젝트가 정한 package manager(`packageManager` 필드 → `pnpm-workspace.yaml` → lockfile 순)로
     `typecheck`·`test` script. pnpm·yarn이 PATH에 없으면 `corepack`으로 실행합니다
   - **Python**: `python3 -m pytest -q`
   - **Rust**: `cargo test --quiet`
   - **QA 회귀**: 위 명령이 하나라도 골라졌고 `.autopus/qa/test-scenarios/`에 exec case가 있으면 마지막에
     `auto qa exec --all`. QA가 한 번 확인한 동작이 이후 변경으로 깨지면 `Expected:`/`Received:`와 함께 막습니다.
     case 자체를 돌릴 수 없는 경우(종료 코드 2)는 이번 변경 탓이 아니므로 막지 않습니다. CI에서도 같은 명령을 씁니다.
2. 전체 시간 예산(`--budget`, 기본 8분) 안에서 돌립니다.
3. 실패하면 종료를 막고, 실패한 명령과 출력의 마지막 60줄을 에이전트에게 넘깁니다.
4. 한 세션에서 세 번 막은 뒤에는 더 막지 않고, 빌드가 아직 실패한다는 경고를 남기고 끝냅니다. 세션이
   끝없이 돌지 않게 하려는 장치입니다.
5. 통과한 트리를 기억합니다. 마지막 통과 이후 바뀐 것이 없으면 다음 stop에서 다시 돌리지 않습니다.

문서만 바뀐 트리와 설치되지 않은 도구는 건너뜁니다. 의존성이 설치되지 않았거나(`node_modules` 없음) script 안의
명령을 찾지 못한 경우(exit 127)도 코드 실패가 아니라 환경 문제라 종료를 막지 않고 경고만 남기며, 통과로 기록하지
않아 환경을 고친 뒤 다시 검사합니다. 작업 트리를 바꿀 수 있는 백그라운드 서브에이전트·workflow·teammate가 아직
돌고 있으면 검증을 다음 종료로 미룹니다.

**후속 라운드(opt-in).** `AUTOPUS_FOLLOWUP=on`(또는 `auto verify stop --followup`)일 때, 빌드가 통과했고 바뀐 소스가 40줄 이상이면(`HEAD` 대비 추가와 삭제의 합, 추적되지 않은
새 소스 파일 포함) hook이 한 번 더 종료를 막고 다음을 지시합니다.

1. `reviewer` 에이전트로 `git diff HEAD`를 검토하고 blocker만 보고한다.
2. 변경에 실행할 수 있는 동작이 있으면 `qa-explorer`를 hunt 모드로 돌리고, 발견한 것을
   `auto qa hunt verify`로 판정한다.
3. 모든 blocker와 확인된 버그를 고치고, 테스트를 다시 돌린 뒤 무엇을 확인했는지 요약한다.

후속 라운드는 세션당 한 번이라, 리뷰 지적을 고친 것만으로 다음 라운드가 열리지는 않습니다.

비용이 커서 기본으로 꺼 둡니다. 실제 업스트림 Go PR로 돌린 작은 파일럿(두 arm 모두 돈 과제 5개, 과제당 한 번)에서 기본
Claude Code가 이미 푸는 네 과제는 통과율 변화 없이 비용만 약 14배였습니다. 더 어려운 과제에서는 숨은
테스트가 다루지 않는 확인된 견고성 버그 3개를 찾아 고쳤고, 비용은 약 12배였습니다. 그 거래가 맞는 곳에서
켜세요.

**omp 세션.** omp에는 Claude Code의 Stop hook이 없어서, `platforms`에 omp가 있으면 `auto update`가 프로젝트 확장
`.omp/extensions/autopus-stop-gate.ts`를 씁니다. 리드 에이전트의 실행이 끝날 때(`agent_end`, subagent 제외)
`auto verify stop`과 `auto verify language`를 같은 입력으로 실행하고, 막히면 그 이유를 followUp 메시지로 돌려보내 이어서
고치게 합니다. 돌려보낸 메시지에 대한 답은 다시 막지 않습니다. 켜고 끄는 방법은 Claude Code와 같습니다(`hooks.verify_stop`,
`hooks.language_stop`, `AUTOPUS_VERIFY_STOP=off`). omp의 print 모드(`-p`)는 `agent_end`를 확장에 전달하기 전에 끝나므로,
이 게이트는 대화형·RPC 세션에서만 동작합니다. OMP context policy를 켠 프로젝트는 `.omp/extensions`를 관리형 런타임의
고정 확장 두 개(bridge, pipeline route)만으로 잠그므로(SPEC-OMP-004), 이 게이트를 설치하지 않습니다.
**자격 증명 확인.** 빌드를 돌리기 전에, 이번 턴이 `HEAD`에 더한 줄(추적 파일의 수정과 gitignore되지 않은
새 파일)을 내장 패턴으로 검사합니다. 개인 키 헤더, AWS access key id, GitHub 토큰(`ghp_`, `gho_`, `ghu_`/`ghs_`,
`ghr_`, `github_pat_`), Slack 토큰과 webhook, Stripe live 키, Google API 키를 찾고, JWT는 자격 증명 이름의 키에
대입되거나 bearer 토큰으로 보낼 때만 봅니다. 몇 개를 놓치더라도 잘못 막지 않는 쪽을 택했고, 외부 도구가
필요 없습니다. 걸리면 값은 빼고 `파일:줄 (규칙)`만 담아 한 번 돌려보내, 값을 환경 변수나 secret 저장소로 옮기게
합니다. 같은 발견이 다시 나오면 경고만 남겨 반복되지 않습니다. 바이너리 파일, 1MiB가 넘는 파일, 하네스 디렉터리는
보지 않습니다. 일부러 둔 테스트 fixture는 그 줄에 `autopus:allow-secret`(또는 `gitleaks:allow`) 주석을 달거나
`.gitleaksignore`에 `file:rule:line`을 적으면 통과합니다.

**버전 확인.** 에이전트는 현재 버전이 아니라 학습 때 기억한 버전(`postgres:13`, `"react": "^18"`, `go 1.22`,
`node:20-alpine`)을 고정합니다. 빌드를 돌리기 전에, 이번 턴이 바꾸거나 새로 만든 manifest(`go.mod`, `package.json`,
`requirements*.txt`, `pyproject.toml`, `Cargo.toml`, `.nvmrc`, `.python-version`, `.tool-versions`, Dockerfile, YAML의
`image:` 키)를 읽고 `HEAD` 대비 새로 추가하거나 바꾼 고정만 남깁니다. 이번 턴이 건드리지 않은 오래된 고정은 지적하지
않습니다. 남은 항목은 레지스트리에서 최신 stable을 조회합니다([의존성 버전](#의존성-버전)). 최신 stable보다
`hooks.version_stop.max_major_behind`(기본 0, 즉 릴리스 라인 하나라도) 라인을 넘게 뒤처진 항목이 있으면 항목, 고른
버전, 최신 버전, 출처를 담아 한 번 돌려보냅니다. minor만 뒤처진 항목은 경고만 합니다. 일부러 고른 버전은 답변에
`version-pin: <name> <reason>` 한 줄을 쓰거나, 주석을 허용하는 형식이면 그 줄에 `version-pin: <reason>` 주석을 달면
통과합니다. 조회는 모두 합쳐 10초 안에 끝나야 하고, 오프라인·사설·느린 레지스트리는 경고만 남기고 막지 않습니다.

**테스트 먼저 확인(methodology: tdd).** `methodology.mode: tdd`와 `enforce: true`이면, 빌드와 테스트가 통과한 뒤
한 단계를 더 봅니다. 바뀐 테스트 파일만 `HEAD`의 소스 위에 얹어 임시 복사본에서 실행하고, 거기서 실패해야
통과합니다. 변경 전 코드에서 실패하고 변경 후 코드에서 통과했으니 RED → GREEN이 기계적으로 증명됩니다.
에이전트가 RED 실행을 따로 기록할 필요가 없어 토큰이 늘지 않습니다.

- 소스(Go, JS/TS, Python)가 바뀌었는데 테스트 변경이 없으면 돌려보냅니다.
- 바뀐 테스트가 변경 전 코드에서도 통과하면, 변경을 검사하지 않는 테스트이므로 돌려보냅니다.
- 순수 리팩터링, 이름 변경, 생성 파일처럼 검사할 동작이 없으면 응답 끝에 `TDD-exempt: <이유>` 한 줄을 두면 통과합니다.
- 임시 복사본은 `git archive HEAD`로 만들어 저장소의 worktree·index·ref를 건드리지 않고, node 프로젝트는 설치된
  `node_modules`를 연결해 씁니다. 실행할 수 없으면 막지 않고 경고만 남깁니다. Rust는 테스트가 소스 안에 있어 대상이 아닙니다.
- 이 확인만 끄려면 `AUTOPUS_TDD_STOP=off`.

**끄는 방법.**

| 범위 | 방법 |
| --- | --- |
| hook을 아예 등록하지 않기 | `autopus.yaml`에 `hooks.verify_stop: false`를 두고 `auto update` |
| 한 세션이나 셸에서만 끄기 | `AUTOPUS_VERIFY_STOP=off` |
| 자격 증명 확인만 끄기 | `hooks.secret_stop: false`, 또는 `AUTOPUS_SECRET_STOP=off` |
| 버전 확인만 끄기 | `hooks.version_stop: false`, 또는 `AUTOPUS_VERSION_STOP=off`. N 라인까지 허용하려면 `hooks.version_stop: {max_major_behind: N}` |
| 후속 라운드 켜기 | `AUTOPUS_FOLLOWUP=on`(예: `.claude/settings.json`의 `env` 블록), 또는 `auto verify stop --followup` |
| 켠 곳에서도 강제로 끄기 | `AUTOPUS_FOLLOWUP=off` |

**응답 언어 확인.** `language.ai_responses`가 `ko`·`ja`·`zh`이면 `auto update`가 Stop hook
`auto verify language`도 등록합니다. 그 턴에 쓴 응답의 산문(코드 블록·인라인 코드·URL 제외)이 그 언어로 쓰이지
않았으면 한 번 돌려보내 다시 쓰게 합니다. 마지막 응답뿐 아니라 도구 호출 사이의 진행 메모도 transcript에서 읽어
검사합니다. 진행 메모가 걸리면 그 메모가 무슨 내용이었는지 설정한 언어로 다시 알리게 합니다. 다시 쓴 응답은 다시 막지 않습니다. 영어 용어가 섞인 한국어 문장은 통과하고,
"Merged."처럼 아주 짧은 응답은 검사하지 않습니다. 끄려면 `hooks.language_stop: false` 뒤 `auto update`,
한 세션에서만 끄려면 `AUTOPUS_LANGUAGE_STOP=off`. Claude Code는 hook 설정을 세션을 시작할 때 읽으므로,
`auto update`로 hook을 새로 등록했으면 세션을 다시 시작하거나 `/hooks`에서 확인해야 적용됩니다.

Stop hook은 Claude Code에만 있습니다. 다른 플랫폼에서는 `/auto review`와 `/auto qa`(아래의 플랫폼별
문법)를 직접 실행하세요.

<a id="qa-hunt"></a>
## QA 버그 헌트

`/auto qa`를 하위 명령 없이 부르거나 자연어로 요청하면 **Bug Hunt** 섹션이 시작됩니다. AI 플레이테스터가
게임 버그를 찾듯이 움직입니다. `qa-explorer` 에이전트는 제품을 어떻게 조작하는지 알아내려고만 코드를 읽고,
실제로 제품(CLI, 서버, UI, 라이브러리 API)을 실행하고 조작하면서 작성자가 떠올리지 못한 실패를 찾습니다.

- 의심 하나마다 `.autopus/qa/hunt/findings/<id>/finding.json`과 `repro.sh`를 남깁니다. `repro.sh`는 버그가
  있으면 exit 1, 고쳐졌으면 exit 0으로 끝납니다.
- `qa-explorer`는 `.autopus/qa/hunt/` 아래에만 씁니다. 제품 코드, 테스트, 설정은 고치지 않습니다. 만든
  사람과 확인하는 사람을 나누는 것이 이 에이전트를 따로 둔 이유입니다.
- `auto qa hunt verify`가 repro를 모두 다시 돌리고 이렇게 분류합니다.

| 판정 | 뜻 |
| --- | --- |
| `confirmed` | repro가 매번 실패한다. 버그다. |
| `flaky` | 일부 실행에서만 실패한다. |
| `not_reproduced` | repro가 통과한다. 처음부터 버그가 아니었거나 고쳐졌다. |
| `broken_repro` | repro 자체가 제대로 돌지 못했다. |

버그로 치는 것은 `confirmed`뿐이고, 나머지는 의심으로 따로 보고합니다. 작업하던 에이전트는 확인된 버그를
고치고 `auto qa hunt verify`가 `not_reproduced`를 낼 때까지 다시 돌립니다. 통과시키려고 repro를 고치지는
않습니다.

```bash
auto qa hunt verify                       # 모든 finding을 다시 실행 (기본 각 2회)
auto qa hunt verify --id <id> --runs 3    # finding 하나를 더 여러 번
auto qa hunt verify --json --fail-on-confirmed   # CI: 확인된 버그가 있으면 exit 1
```

그 밖의 flag: `--dir`(프로젝트 루트), `--timeout`(repro 1회당, 기본 2분).

**시나리오가 버그를 잡는지 재기.** `auto qa mutate`는 제품 소스에 작은 결함을 하나씩 심은 임시 복사본에서
exec case를 돌리고, 아무 case도 못 잡은 결함을 보고합니다. 원본은 쓰지 않습니다.

```bash
auto qa mutate                    # 기본 20개, 살아남은 결함을 파일:줄·바뀐 코드·힌트와 출력
auto qa mutate --max 50 --min-score 0.7   # CI: 잡은 비율이 70% 미만이면 exit 1
```

`auto qa`의 나머지(QAMESH journey, `auto qa run`, `auto qa release`, `auto qa report` 등)는 선언된 journey와
릴리스 근거용으로 그대로 있습니다. `auto qa --help`를 보세요.

<a id="routes"></a>
## 라우트

코딩 에이전트의 채팅창에서 실행합니다.

| 라우트 | 하는 일 |
| --- | --- |
| `/auto review` | 바뀐 코드를 TRUST 5(Tested, Readable, Unified, Secured, Trackable) 기준으로 검토하고, 프로젝트의 build·test·lint 명령 결과를 근거로 씁니다. 여러 provider에 나눠 맡길 수 있습니다. |
| `/auto qa` | 기본은 QA 버그 헌트, 선언된 journey와 릴리스 근거에는 QAMESH. |
| `/auto secure` | 보안 헌트: `auto secure scan --json`으로 스캐너 단서를 모으고, 바뀐 범위의 위협 모델을 짧게 세운 뒤, 사용자 소유의 로컬·스테이징 대상에 실제 공격을 시도합니다. `auto qa hunt verify`가 재현 스크립트로 확인한 것만 취약점으로 보고하고, 나머지는 "확인되지 않음"으로 따로 적습니다. `--multi`는 `auto orchestra secure`를 더하고, 경로를 주면 범위를 좁힙니다. |
| `/auto verify` | 프론트엔드 UX 검증: 바뀐 UI의 Playwright 스크린샷과 시각 분석. |
| `/auto test` | `.autopus/project/scenarios.md`의 E2E 시나리오를 `auto test run`으로 실행. |
| `/auto sync` | diff를 읽고 그 변경이 낡게 만든 문서(README, ARCHITECTURE, CHANGELOG, `docs/`, `.autopus/project/*.md`)만 고친 뒤, 문서가 이름 붙인 명령·flag·경로가 실제로 있는지 확인합니다. SPEC이 없어도 됩니다. |
| `/auto update` | 생성된 하네스 파일을 새로 고칩니다. |
| `/auto doctor` | 설치된 플랫폼 연결을 점검합니다. |
| `/auto browse` | browser-automation 스킬의 별칭. |

플랫폼별 문법:

- **Claude Code, Antigravity CLI, Oh My Pi**: `/auto <route>`
- **OpenCode**: `/auto <route>` 또는 `/auto-<route>`
- **Codex**: `$codex-auto-<route>`, 또는 `.agents/plugins/marketplace.json`의 로컬 플러그인을 설치한 뒤
  `@auto <route>`

<a id="installed"></a>
## 설치되는 것

| 종류 | 설치 내용 |
| --- | --- |
| 에이전트 (4) | `reviewer`, `qa-explorer`, `frontend-specialist`, `ux-validator` |
| 규칙 (4) | `language-policy`, `lore-commit`, `file-size-limit`, `autonomy` |
| 스킬 (13) | `review`, `verification`, `qa-hunt`, `secure-hunt`, `disk-triage`, `ci-optimize`, `stack-freshness`, `qa-autopilot`, `frontend-verify`, `browser-automation`, `playwright-cli`, `testing-strategy`, `lore-commit` |
| 라우트 | `review`, `qa`, `secure`, `verify`, `test`, `sync`, `update`, `doctor`, 그리고 `browse` 별칭 |
| Hook | Claude Code Stop hook `auto verify stop`, Lore `commit-msg` git hook |

`autonomy` 규칙은 에이전트가 단계마다 허락을 구하지 말고 일을 끝까지 진행하게 합니다. 결정이 정말
사용자의 몫일 때는 객관식으로 물어야 하고(Claude Code에서는 추천 선택지를 맨 앞에 둔 `AskUserQuestion`
도구), 열린 질문은 하지 않습니다. 데이터 삭제, force-push, 배포, 메시지 전송처럼 되돌리기 어렵거나
바깥으로 나가는 행동은 여전히 사용자 확인을 받습니다.

Claude Code에서 `lore-commit`은 항상 켜진 규칙이 아니라, 에이전트가 셸 명령을 실행할 때 불러오는 조건부
규칙으로 들어갑니다.

### Lore 커밋

`commit-msg` hook은 Lore 형식을 검사합니다. type과 scope가 있는 제목(`feat`, `fix`, `refactor`, `test`,
`docs`, `chore`, `perf`, `merge`, `revert`), `🐙 Autopus <noreply@autopus.co>` 서명, `lore.required_trailers`에
적힌 trailer(기본 `Constraint`), 그리고 `lore.forbidden_trailers`(기본 `Co-Authored-By`)에 든 trailer가 없는지를
봅니다. `auto check --lore`도 같은 검사를 합니다.

### 멀티프로바이더 리뷰

`/auto review`는 `auto orchestra review`를 통해 여러 provider CLI에 리뷰를 맡기고, consensus나 debate 전략과
judge로 결과를 모을 수 있습니다. provider는 `orchestra.providers`에서 정합니다. 새 설정에는 `claude`, `codex`,
`gemini`(`agy` 바이너리)가 들어가고, provider를 OMP를 거쳐(`backend: omp`) 돌릴 수도 있습니다. provider별 리뷰
model은 `auto quality review model <provider> <model>`로 바꿉니다.

<a id="platforms"></a>
## 플랫폼

`auto init`이 찾아낸 지원 CLI마다, `autopus.yaml` 하나에서 그 CLI의 네이티브 파일을 만듭니다.

| 플랫폼 | 생성되는 표면 |
| --- | --- |
| **Claude Code** | `.claude/agents/autopus/`, `.claude/rules/autopus/`, `.claude/skills/<name>/SKILL.md`, `.claude/hooks/autopus/`, `.claude/settings.json`(Stop hook), `CLAUDE.md` |
| **Codex** | `.codex/skills/codex-<name>/SKILL.md`, `.codex/agents/`, `.codex/hooks.json`, `.codex/config.toml`, `.agents/plugins/marketplace.json`, `.autopus/plugins/auto/`, `AGENTS.md` |
| **Antigravity CLI** | `.agents/plugins/autopus/`, `.agents/hooks.json`, 그리고 호환용 `.gemini/`·`GEMINI.md` |
| **OpenCode** | `.opencode/agents/`, `.opencode/commands/`, `.opencode/rules/`, `.opencode/plugins/`, `.agents/skills/`, `AGENTS.md`, `opencode.json` |
| **Oh My Pi (OMP)** | `.omp/agents/`, `.omp/rules/autopus-*.md`, `.omp/skills/<name>/SKILL.md`, `.omp/commands/` |

플랫폼 목록은 `auto platform add|remove|list`로 관리하고, `auto doctor`로 생성 파일이 제대로 연결됐는지
확인합니다. Antigravity에는 선택 기능인 REA 역공학 MCP를 위한 `rea-mcp-guard` 규칙도 남아 있습니다. REA MCP는
`--rea`(또는 wizard에서 Yes)를 고르고 Node 버전이 맞을 때만 `auto init`이 등록합니다.

<a id="install"></a>
## 설치

이 페이지 맨 위의 한 줄 명령을 에이전트 채팅창에 붙여넣거나 터미널에서 실행하세요. 설치 스크립트는 `auto`
CLI와 `autopus` 별칭을 설치하고, 필요한 도구를 확인해 `git`, GitHub CLI처럼 빠진 필수 도구를 설치합니다.
`auto init`은 실행하지 않습니다.

<details>
<summary>다른 설치 방법</summary>

```bash
# Homebrew (macOS)
brew install --cask Insajin/autopus/auto

# go install (Go 1.26 이상)
go install github.com/insajin/autopus-adk/cmd/auto@latest

# 소스에서 빌드
git clone https://github.com/autopus-ai/autopus-adk.git
cd autopus-adk && make build && make install
```

`make install`은 `$(go env GOPATH)/bin`에 복사합니다. 다른 곳에 두려면 `INSTALL_DIR=...`를 넘기세요.

설치 스크립트 환경 변수: `INSTALL_DIR`(기본 `/usr/local/bin`), `VERSION`(기본 최신).

Homebrew 6 이상에서는 기존 설치가 ADK cask를 명시적으로 신뢰해야 할 수 있습니다. Homebrew가
`Refusing to load cask insajin/autopus/auto from untrusted tap`을 보이면
`brew trust --cask insajin/autopus/auto`를 실행하세요. `auto doctor`는 이 상태를
`doctor.homebrew.tap_trust`로 보고합니다.

</details>

그다음 프로젝트를 초기화합니다.

```bash
cd your-project
auto init          # 대화형. 기본값으로는 --yes, 플랫폼 선택은 --platforms claude-code,codex
auto doctor        # 설치된 연결 점검
```

자주 쓰는 `auto init` flag: `--yes`, `--platforms`, `--project`, `--dir`, `--no-review-gate`,
`--statusline-mode keep|merge|replace`, `--rea` / `--no-rea`.

### 최신 상태로 유지하기

```bash
auto update --self   # auto 바이너리 업데이트 (publisher 서명과 SHA256 검증)
auto update          # 설치된 바이너리의 템플릿으로 이 프로젝트의 하네스 파일을 다시 생성
```

`auto update --preview`(`--plan`, `--dry-run`도 같음)는 쓰지 않고 바뀔 파일만 보여 줍니다. `CLAUDE.md`,
`AGENTS.md` 같은 파일에서 `AUTOPUS:BEGIN` / `AUTOPUS:END` 마커 밖에 쓴 내용은 남습니다. 그 밖의 생성 파일은
덮어쓰고, 원본은 `.autopus/backup/<timestamp>/` 아래에 백업합니다. `--workspace`는 메타 워크스페이스 안의
하위 Git 리포까지 업데이트합니다.

### 0.50.x에서 올라올 때

v0.51.0부터 하네스는 위에 적은 검증 표면만 설치합니다. 프로젝트마다 `auto update --self && auto update`를
실행하세요.

- `auto update`는 플랫폼 manifest에 기록된 예전 표면을 지웁니다. 라우트 `plan`, `go`, `fix`, `idea`, `setup`,
  `status`, `goal`, `map`, `why`, `secure`, `dev`, `canary`, `reverse`, 은퇴한 에이전트 14개(planner, executor,
  tester, architect, spec-writer 등), 은퇴한 스킬과 규칙(`branding`, `subagent-delegation`, `worktree-safety`
  등), Claude `.claude/workflows/*.js`가 대상입니다.
- 한 번도 업데이트하지 않은 설치는 `auto platform remove <platform>`으로 정리할 수 있습니다.
- 새 설정의 기본값은 `methodology.mode: none`입니다. `tdd` + `enforce: true`는 `auto verify stop`의 테스트 먼저 확인으로
  집행되고, 그 밖의 예전 방법론 설정은 `auto doctor`와 `auto check`에서 실패가 아니라 안내로 보고됩니다.
- CLI 명령은 바이너리에 그대로 있고 계속 동작합니다. 생성되는 스킬·규칙·hook이 부르지 않는 구현 하네스 명령
  16개(`auto pipeline`, `auto fix`, `auto experiment`, `auto learn` 등)는 `auto --help`에 더 이상 나오지 않고,
  다음 릴리즈에서 지웁니다.

<a id="configuration"></a>
## 설정

모든 설정은 `autopus.yaml`에 있습니다. 검증과 관련된 key만 남긴 예시입니다.

```yaml
project_name: my-project
platforms: [claude-code, codex]

language:
  comments: en          # 코드 주석
  commits: en           # 커밋 메시지
  ai_responses: en      # 에이전트 응답

architecture:
  max_file_lines: 300   # file-size-limit의 선택적 상한. 없거나 0이면 권고만

hooks:
  verify_stop: true     # Claude Code Stop hook (없으면 켜짐)
  secret_stop: true     # Stop hook 안의 자격 증명 확인 (없으면 켜짐)
  version_stop: true    # Stop hook 안의 버전 확인. {max_major_behind: N}도 가능 (없으면 켜짐, N=0)
  language_stop: true   # 응답 언어 Stop hook (없으면 켜짐, ai_responses가 ko·ja·zh일 때만)
  pre_commit_lore: true # Lore commit-msg hook
  pre_commit_arch: true

lore:
  enabled: true
  required_trailers: [Constraint]
  forbidden_trailers: [Co-Authored-By]   # 기본값. []이면 허용

orchestra:              # 멀티프로바이더 리뷰용 provider
  enabled: true
  default_strategy: consensus
  judge: claude
  providers:
    claude:
      binary: claude
    codex:
      binary: codex
```

설정 하위 명령은 `auto config --help`에서, 품질 프리셋과 리뷰 model 선택은 `auto quality`에서 합니다.

프리셋, 에이전트별 model, OMP model pin, 리뷰 model, REA MCP flag는 [docs/quality.ko.md](quality.ko.md)에 정리했습니다.

<a id="security"></a>
## 보안 스캔

`auto secure scan`은 이미 설치된 보안 스캐너를 실행하고 결과를 하나로 요약합니다.

```bash
auto secure scan                    # 현재 디렉터리
auto secure scan --dir ./service --json
auto secure scan --staged           # gitleaks: stage된 변경만
auto secure scan --log-opts origin/main..HEAD --fail-on critical
```

`--dir`의 파일로 생태계를 알아내고, 설치된 스캐너를 실행합니다.

| 스캐너 | 조건 | 명령 |
| --- | --- | --- |
| gitleaks | 항상 | git 작업 트리면 `gitleaks git`, 아니면 `gitleaks dir` (8.19 이전 릴리스는 `detect`/`protect`) |
| govulncheck | `go.mod` | `govulncheck -format json ./...` |
| npm / pnpm / yarn | `package.json`과 lockfile | `npm audit --json`, `pnpm audit --json`, `yarn audit --json`, `yarn npm audit --json`(Berry) |
| cargo audit | `Cargo.lock` | `cargo audit --json` |
| pip-audit | `requirements*.txt` | `pip-audit -f json --no-deps --disable-pip -r ...` |

설치되지 않은 스캐너나, lockfile·requirements 파일이 없는 프로젝트는 이유를 적고 건너뜁니다. 아무것도 설치하지
않습니다. pip-audit는 환경을 만들지 않고 적힌 버전 그대로 검사합니다. audit 명령과 govulncheck는 advisory
데이터베이스를 네트워크로 조회합니다. 각 스캐너는 프로젝트 디렉터리에서 시간 제한(`--timeout`, 기본 5분) 안에 돕니다.

발견은 한 가지 모양(`tool`, `kind`(secret 또는 vuln), `id`, `severity`, `package`/`file`, `location`,
`fixed_version`, `summary`)으로 모입니다. secret 값은 출력하지 않습니다. gitleaks는 `--redact`로 실행하고, 파서는
`Secret`·`Match` 필드를 읽지 않으며, secret은 `파일:줄`, 규칙, fingerprint(`.gitleaksignore`에 쓰는 형식)로만 보입니다.

종료 코드: 발견이 `--fail-on`에 닿으면 1(기본 `high`: secret 전부, 또는 high·critical 취약점. govulncheck, cargo
audit, pip-audit는 등급을 주지 않으므로 high로 셉니다), 다른 문제 없이 스캐너가 실패했으면 2, 그 밖에는 0입니다.
설치된 스캐너가 하나도 없어도 0입니다. `--fail-on`은 `none`, `secret`, `critical`, `high`, `medium`, `low`를 받습니다.

## 의존성 버전

`auto deps`는 모델의 기억 대신 레지스트리에 묻습니다.

```bash
auto deps latest npm:react             # 19.x, 출처 URL과 조회 시각
auto deps latest image:postgres        # Docker Hub 공식 이미지, 숫자 릴리스 태그만
auto deps latest toolchain:node        # current와 active LTS
auto deps latest django                # 생태계 없이 쓰면 프로젝트 manifest에서 찾는다
auto deps outdated                     # 직접 의존성, 툴체인 고정, 이미지를 최신 stable과 비교
auto deps outdated --dir ./service --json
```

| 생태계 | 출처 |
| --- | --- |
| `go:` 모듈 | `GOPROXY`(기본 proxy.golang.org)의 `/@latest` |
| `npm:` | registry.npmjs.org의 `dist-tags.latest`, 또는 단순한 `.npmrc`가 지정한 registry(전역 또는 scope별) |
| `pypi:` | pypi.org JSON API |
| `crates:` | crates.io API(`max_stable_version`) |
| `image:` | Docker Hub 태그. `node`, `golang`, `python` 이미지는 아래 툴체인 출처를 쓴다 |
| `toolchain:go` | go.dev/dl |
| `toolchain:node` | nodejs.org/dist/index.json. `engines`, `.nvmrc`, `node:` 이미지는 active LTS와 비교한다 |
| `toolchain:python` | endoflife.date(릴리스 주기) |

pre-release는 "latest"가 되지 않습니다. `outdated`는 각 고정을 `current`, `minor-behind`, `major-behind (N)`(릴리스
라인 N개 뒤. Go·Python 툴체인과 `0.x` 패키지는 minor 하나가 한 라인), `unknown`, `skipped`로 나누고 항상 0으로
끝납니다. 개인정보: `GOPRIVATE`, `GONOPROXY`, `GONOSUMDB`(환경 변수나 `go env -w`)에 걸린 Go 모듈은 proxy로 보내지
않고, `GOPROXY=off`/`direct`이면 unknown입니다. `PIP_INDEX_URL`이 사설 index를 가리키면 PyPI 조회를 건너뛰고,
Docker Hub가 아닌 registry의 이미지, digest, 변수는 보지 않습니다. 요청은 각각 5초 timeout으로 네 개씩 돌고,
답은 `$XDG_STATE_HOME/autopus/deps`(없으면 `~/.local/state/autopus/deps`, `AUTOPUS_DEPS_CACHE_DIR`로 변경)에
24시간 캐시합니다. `AUTOPUS_DEPS_OFFLINE=1`이면 캐시로만 답합니다. 언제 조회하고 어떻게 의도적으로 고정하는지는
`stack-freshness` 스킬이 알려 줍니다.

## 디스크 공간

에이전트는 빌드하고, 테스트하고, 패키지를 설치하고, worktree를 만듭니다. 한 머신에서 세션 여러 개가 함께
돌면 여유 공간이 한 시간 안에 수십 GiB씩 오르내릴 수 있습니다. 디스크가 차면 빌드가 `no space left on
device`로 실패하는데, 이 실패는 코드 문제처럼 보입니다. `auto disk`는 무엇이 늘고 있는지 보여 주고, 공간이
모자랄 때 무거운 명령이 시작되지 않게 막고, 끝난 작업을 작업 중에 정리합니다.

```bash
auto disk status                       # 여유 공간, 모든 소비자, 지난 한 시간 동안 늘어난 것
auto disk check --need 8               # 8 GiB(또는 disk.min_free_gb)가 없으면 exit 3
auto disk reclaim                      # dry run: 회수 대상 목록
auto disk reclaim --apply              # 실제로 지운다
auto disk trim --apply                 # 쉬고 있는 Go 빌드 캐시 항목을 지운다
auto disk lease --need 8 -- go test ./...   # 머신 전체 예약을 잡고 무거운 단계를 실행한다
auto disk log                          # trim, reclaim, 경고, 게이트가 한 일
```

**status가 재는 것.** Go 빌드 캐시와 작업 디렉터리, linked worktree(그 안의 `node_modules`와 Cargo
`target`), 세션 scratchpad(그 안의 1 GiB 이상 파일), 그리고 내 머신에 있는 것만: Go 모듈 캐시, npm, pnpm,
yarn, bun, Cargo registry, pip, uv, Gradle, Maven, Xcode DerivedData와 시뮬레이터(macOS), Docker Desktop,
colima, lima, podman의 VM 디스크, 읽을 수 있을 때 `/var/lib/docker`(Linux). VM 디스크 이미지는 sparse이므로
할당된 크기를 셉니다. 각 측정에는 시간 예산이 있고, 예산 안에 다 세지 못한 값은 하한으로 표시하며 증가 표에서
뺍니다. CI 러너 작업 디렉터리처럼 이 머신에만 있는 경로는 `disk.consumers.extra_roots`에 적습니다. 읽을 수
없는 경로는 0이 아니라 `unreadable`로 보고합니다.

```yaml
disk:
  min_free_gb: 10              # 경고, 게이트, lease가 쓰는 바닥선
  consumers:
    extra_roots: [/path/to/runner/_work]
```

status, `auto disk check`, 프롬프트마다 붙는 경고가 실행될 때마다 디스크 상태 디렉터리
(`$XDG_STATE_HOME/autopus/disk` 또는 `~/.local/state/autopus/disk`)의 작은 고리 버퍼에 표본을 남깁니다.
status는 이 표본으로 소비자를 증가량 순서로 보여 주고(예: `colima +38.0 GiB in 52m`), Claude Code의 공간 부족
경고는 가장 많이 늘어난 소비자를 함께 알려 줍니다.

**디스크 게이트(Claude Code).** PreToolUse hook `auto disk gate`가 Bash 호출마다 statfs를 한 번 합니다.
`disk.min_free_gb` 아래이면 go, cargo, npm, pnpm, yarn, bun, pip, uv, poetry, Gradle, Maven의 빌드·테스트·
설치, docker와 podman의 build와 `compose up`, xcodebuild, `git worktree add`를 막고, 에이전트에게 멈추고
`auto disk status`를 본 뒤 회수하거나 기다리라고 알립니다. 하위 에이전트의 도구 호출도 같이 막습니다. `auto`
명령은 모두 통과하므로, 그래도 돌려야 하는 무거운 작업은 `auto disk lease -- <command>`로 실행합니다. `make`는
기본으로 막지 않습니다. 무거운 대상은 `hooks.disk_gate.extra_commands: ["make release"]`로 더합니다. 끄려면
`hooks.disk_gate: false` 또는 `AUTOPUS_DISK_GATE=off`를 씁니다. OMP에는 pre-tool hook이 없고, 다른 플랫폼에는
아직 게이트를 등록하지 않습니다. 그 플랫폼에서는 무거운 단계 전에 `auto disk check`를 실행합니다.

**회수.** `--apply`를 주지 않으면 모두 dry run이고, 심볼릭 링크는 따라가지 않으며, 스캔 뒤에 바뀐 항목은
건너뜁니다. `auto disk reclaim`이 지우는 것:

- 위치와 상관없이, HEAD가 기본 원격 브랜치(`origin/HEAD` 또는 `origin/main`)에 들어 있고, 추적하지 않은 파일
  없이 깨끗하며, `disk.reclaim.worktree_idle_minutes`(기본 360)보다 오래 쉬었고, 실행 중인 어떤 프로세스의
  작업 디렉터리도 아닌 linked worktree. `git worktree remove`를 `--force` 없이 쓰지만 git이 무시하는 파일은
  그래도 지워지므로, 아래의 재생성 가능한 빌드 디렉터리 밖에 무시된 파일(`.env`, 로컬 DB 등)이 하나라도 있으면
  그 worktree는 남깁니다. branch는 남깁니다.
- `claude agent agent-<id> (pid N start T)`로 잠긴 Claude Code agent worktree 중 깨끗하고 merge된 것. 잠근
  프로세스가 사라졌으면(pid와 시작 시각으로 확인) 바로, 살아 있으면 worktree가 쉬고 있고 그 안에 프로세스가
  없을 때 잠금을 풀고 지웁니다. dirty하거나 merge되지 않은 worktree는 지우지 않습니다.
- 쉬고 있는 worktree 안에서 git이 무시하는 `node_modules`, `.next`, `dist`(옆에 `package.json`이 있을 때),
  Cargo `target`
- 쉬고 있는 세션 scratchpad. scratchpad 루트의 1 GiB 이상 파일은 백업일 수 있으므로 목록에만 올리고
  `--include-large-files`를 줄 때만 지웁니다.

**자동 정리(Claude Code).** 매 턴 끝의 Stop hook(`auto verify stop`)이 모든 세션을 통틀어
`disk.reclaim.auto_interval_minutes`(기본 30)마다 한 번까지 `auto disk reclaim --auto --apply`를
백그라운드로 시작합니다. merge되고 깨끗한 worktree와 쉬고 있는 재생성 가능 디렉터리만 지우고, scratchpad와
대용량 파일은 건드리지 않으며, 턴을 늦추지 않습니다. 끄려면 `disk.reclaim.auto: false` 또는
`AUTOPUS_DISK_AUTO_RECLAIM=off`를 씁니다. `disk-triage` 스킬(core, 기본 설치)은 에이전트에게 같은 순서를
안내하고, 다른 세션의 작업, 백업, VM 디스크, 다른 빌드가 쓰는 캐시처럼 지우기 전에 사용자에게 물어야 할 것을
알려 줍니다.

## CI: 먼저 재고, 기계는 나눠 쓴다

`auto ci report`는 GitHub Actions 저장소를 바꾸기 전에 잽니다. 사용자의 `gh` 로그인으로 읽기만 하고(`gh`가
없거나 로그인되지 않았으면 할 일을 알려 주고 끝납니다), jobs API 타임스탬프만 씁니다. `filter=all`로 재실행까지
세고, 폐지된 `/timing` endpoint는 쓰지 않습니다.

```bash
auto ci report                          # origin 저장소, 최근 14일, run 200개까지 고르게 표본
auto ci report --repo owner/name --since 30d --workflow ci.yaml --json
```

finding은 심각도, 분/일 순서로 정렬되고 근거 수치가 붙습니다. 실행보다 runner 대기가 긴 job, 가장 비싼
job(중앙값·p90), 실패·취소·재실행 attempt에 쓴 분, 같은 workflow의 성공한 PR run이 이미 시험한 commit tree를
다시 돈 push run, 새 push 뒤에도 끝까지 돈 PR run, 포화되거나 한쪽으로 몰린 self-hosted runner를 찾습니다.
로컬 `.github/workflows` lint는 `timeout-minutes`가 없는 job, 취소하는 concurrency group이 없는 PR workflow,
push·release·`workflow_call` run까지 취소하는 group, fork PR이 닿는 public 저장소의 self-hosted PR job, PR head를
checkout하는 `pull_request_target`, PR lane과 `runs-on` 라벨을 공유하는 release·publish job을 더합니다. `--json`은
모든 표를 담습니다. 요청은 `--max-requests`(기본 1000, 동시 4개)로 묶이고 실행 동안만 캐시됩니다. run이
`--limit-runs`보다 많으면 합계에 보고된 보정 비율을 곱합니다.

**공유 기계의 CPU lease.** GitHub Actions에는 job 우선순위가 없어서, runner 여러 개가 도는 self-hosted 기계의
진짜 문제는 경합입니다. job마다 `GOMAXPROCS`와 `go test -p`를 전체 코어 수로 잡고, macOS에는 이를 막을 cgroup이
없습니다. `auto disk lease --cpu N`은 명령 하나에 기계의 CPU 슬롯 N개를 예약합니다. 디스크 예약과 함께 써도 되고
`--cpu`만 써도 됩니다. 슬롯 수는 기본이 논리 CPU 수이고(`disk.lease.cpu_slots`, 기계별로는
`AUTOPUS_LEASE_CPU_SLOTS`), 그보다 큰 N은 슬롯 수로 줄입니다. 공유 기계의 admission control입니다. runner는
job을 계속 병렬로 시작하고, 무거운 단계만 우선순위대로 들어가기를 기다립니다. `--priority interactive|ci|background`로
class를 정합니다(기본 `ci`. GitHub Actions 안에서는 `GITHUB_EVENT_NAME`을 따라 `pull_request`·`pull_request_target`·
`merge_group`은 interactive, `schedule`은 background, 나머지는 ci이며, 플래그가 우선합니다). 높은 class가 먼저,
같은 class 안에서는 선착순으로 들어가고, 대기자는 `disk.lease.aging_minutes`(기본 10, 0이면 끔)마다 class가 한
단계 올라서 background도 굶지 않습니다. 이미 도는 holder는 끊지 않습니다. `--wait`가 대기를 제한하며 시간이
다하면 exit 3입니다. 죽은 holder나 대기자는 자리를 내놓습니다. 명령은 `AUTOPUS_LEASE_CPUS=N`과, 따로 정하지
않았다면 `GOMAXPROCS=N`을 받습니다. `auto disk status`가 holder와, 들어갈 순서대로 대기자의 class, 올라간 class,
대기 시간을 보여 줍니다.

슬롯만으로는 기계가 한가한지 바쁜지 알 수 없고, 다른 세션과 runner는 lease 없이 기계를 씁니다. 그래서 기계에
여유가 있으면 무거운 작업을 병렬로 돌리고, 여유가 없으면 우선순위대로 들여보냅니다. 맨 앞 대기자는 슬롯이 비어도
다음 동안 기다립니다. 1분 load average가 `disk.lease.max_load_ratio` × 논리 CPU 수(기본 0.85)를 넘는 동안(아직
average에 드러나지 않은 최근 허가분을 더해 세므로 한꺼번에 들어가도 넘치지 않습니다), 가용 메모리에서 도는
lease들의 `--mem` 예약을 뺀 값이 자기 `--mem SIZE`(예: `8G`, 기본 없음)와 `disk.lease.min_free_mem_gb`(기본 8)의
합보다 작은 동안, 그리고 macOS에서 커널이 메모리 압력을 critical로 알리는 동안입니다. 값은 프로세스를 띄우지 않고
읽고(macOS는 `sysctl`, Linux는 `/proc`) 2초 안팎마다 다시 봅니다. 기계별로는 `AUTOPUS_LEASE_MAX_LOAD_RATIO`와
`AUTOPUS_LEASE_MIN_FREE_MEM_GB`가 키를 덮어씁니다. `auto disk status`는 맨 앞 대기자를 붙잡은 이유를 보여
줍니다(`waiting: load 22.1 > 20.4`, `waiting: mem 6.2 GiB < 8+4 GiB`). 바닥선과 함께 결코 들어갈 수 없는
`--mem`은 바로 실패합니다. 도는 lease가 하나도 없는데 load만으로 맨 앞 대기자가 `disk.lease.max_load_wait_minutes`
(기본 10) 넘게 막히면 한 번에 하나씩 그냥 들여보내고 기록합니다(`lease_forced`). 이런 lease에는 그보다 긴
`--wait`를 주세요. Windows에는 lease가 없어서 명령을 lease 없이 실행합니다.

```yaml
# .github/workflows/ci.yaml (self-hosted job)
- name: Test
  run: auto disk lease --cpu 8 --mem 8G --wait 30m --label test -- sh -c 'go test -p "$AUTOPUS_LEASE_CPUS" ./...'
```

무거운 작업을 lock 하나로 줄 세우는 셸 wrapper(빌드마다 `flock`)를 쓰고 있다면, wrapper를
`auto disk lease --cpu N --mem SIZE --priority … -- cmd`로 바꾸세요. lock 하나는 기계가 아무리 한가해도 한 번에
하나만 돌리지만, lease는 슬롯·load·메모리가 허락하는 만큼 함께 돌립니다.

lease는 기본으로 사용자 자신의 state dir에 있어서 한 사용자의 job끼리만 조율합니다. runner 계정 여러 개가 한
기계를 쓰면 모두 `AUTOPUS_LEASE_DIR`(예: 각 runner의 `.env`)이나 `disk.lease.dir`로 같은 디렉터리를 가리키게
합니다. 없는 디렉터리는 `/tmp`처럼 sticky·world-writable로 만들고, 그룹이나 다른 사용자가 쓸 수 있는 기존
디렉터리는 sticky(`chmod +t`)여야 합니다. lease를 쥘 수 없는 시스템(Windows)에서 `--cpu`만 준 요청은 경고를
남기고 같은 환경 변수로 명령을 실행합니다.

## 보안

- **서명된 릴리스.** v0.50.73 이후 모든 릴리스에는 `checksums.txt`와 ECDSA P-256 publisher envelope
  `checksums.txt.signatures`가 들어 있습니다. POSIX 설치 스크립트, Windows 설치 스크립트, `auto update --self`는
  내려받거나 풀기 전에 체크섬 목록을 인증하고, 그다음 아카이브의 SHA256을 검증합니다. 서명 데이터가 없거나
  신뢰할 수 없으면 실패하고, 체크섬만으로 설치하는 fallback은 없습니다.
- **설치 전에 읽어 보기.** `install.sh`를 내려받아 읽은 뒤 실행하세요.

  ```bash
  curl -sSfL https://raw.githubusercontent.com/autopus-ai/autopus-adk/main/install.sh -o install.sh
  less install.sh && sh install.sh
  ```

- **신뢰 경계.** 한 줄 설치 스크립트는 리포의 `main` 브랜치에서 내려받습니다. publisher 검증은 릴리스
  asset을 보호하지만 설치 스크립트 자체를 인증하지는 않습니다. `main`이 오염되면 검증기 자체가 바뀔 수 있습니다.
- **QA의 쓰기 범위.** `qa-explorer`는 `.autopus/qa/hunt/` 아래에만 씁니다. repro 스크립트는 사용자 권한으로
  사용자 기기에서 돌기 때문에, 직접 실행하기 전에 한 번 읽어 보세요.
- **API key를 다루지 않음.** Autopus는 이미 설치된 provider CLI를 실행할 뿐, provider API key를 갖지 않습니다.
  네트워크 호출은 `auto update --self`, `auto orchestra`, 멀티프로바이더 리뷰, `auto search`, `auto connect`처럼
  명시적인 명령에서만 일어납니다.

## 기여하기

Autopus-ADK는 MIT 라이선스의 오픈 소스입니다. PR을 환영합니다.

```bash
make test       # race detection을 켠 테스트
make lint       # go vet
make coverage   # 커버리지 리포트
```

CI는 `internal/cli` 테스트를 이름 해시 shard로 나눕니다. `test-cli (3/5)` 같은 이름의 job이 실패하면 로컬에서
`make test-cli-shard SHARD=3/5`로 그 shard만 다시 돌릴 수 있습니다.

커밋은 위에서 설명한 Lore 형식을 따릅니다. 릴리스 노트는 [CHANGELOG.md](../CHANGELOG.md)에 있습니다.

## 라이선스

[MIT](../LICENSE)

---

<div align="center">

**🐙 Autopus** — 에이전트가 만든 것을, 돌려 보고 확인합니다.

</div>
