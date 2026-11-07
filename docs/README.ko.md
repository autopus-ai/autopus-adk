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
아무것도 얹지 않은 Claude Code는 4/4를 통과했고, 예전 full 하네스는 10-35배 비용을 쓰고 0/4였습니다.
n=4라 벤치마크라고 할 수는 없고 방향을 보여 주는 정도지만, 평소 사용에서 느낀 것과 같았습니다.

에이전트가 여전히 못하는 일은 자기 결과를 확인하는 것입니다. 빌드되지 않는 코드, 돌리지 않은 테스트,
한 번도 실행해 보지 않은 동작을 두고 "완료"라고 보고합니다. 그래서 하네스는 이제 한 가지 일만 합니다.
변경을 실제로 돌려 보고, 버티지 못하면 에이전트를 다시 일하게 합니다.

## 하는 일

| 단계 | 무엇이 도는가 | 언제 |
| --- | --- | --- |
| 빌드·테스트 | `auto verify stop`: 바뀐 파일이 속한 toolchain만 | Claude Code 세션이 바뀐 작업 트리를 둔 채 끝나려 할 때마다 |
| 리뷰 | `reviewer` 에이전트가 `git diff HEAD`를 검토 | 바뀐 소스가 40줄 이상인 변경의 빌드가 통과하면 자동으로 |
| QA 버그 헌트 | `qa-explorer` 에이전트가 제품을 조작하고, `auto qa hunt verify`가 repro를 다시 돌림 | 같은 후속 라운드에서 실행할 수 있는 동작이 있을 때, 또는 `/auto qa`로 직접 |
| 수정 | 작업하던 에이전트가 blocker와 확인된 버그를 고치고 테스트를 다시 돌림 | 같은 후속 라운드 |

어느 단계도 사용자가 시작하라고 말할 필요가 없습니다. 에이전트는 라운드를 끝까지 진행하고, 정말
사용자가 정해야 하는 결정만 객관식으로 묻도록 지시받습니다.

<a id="stop-hook"></a>
## Stop hook: 세션이 끝나기 전에 빌드하고 테스트한다

`auto init`은 Claude Code에 `Stop` hook `auto verify stop`을 등록합니다. 에이전트가 세션을 끝내려 할 때
작업 트리가 `HEAD`와 다르면 hook은 다음을 합니다.

1. 바뀐 파일이 속한 toolchain의 명령만 고릅니다.
   - **Go**: `go build ./...`, 그리고 바뀐 패키지의 `go test`
   - **Node**: lockfile이 가리키는 package manager로 `typecheck`·`test` script
   - **Python**: `python3 -m pytest -q`
   - **Rust**: `cargo test --quiet`
2. 전체 시간 예산(`--budget`, 기본 8분) 안에서 돌립니다.
3. 실패하면 종료를 막고, 실패한 명령과 출력의 마지막 60줄을 에이전트에게 넘깁니다.
4. 한 세션에서 세 번 막은 뒤에는 더 막지 않고, 빌드가 아직 실패한다는 경고를 남기고 끝냅니다. 세션이
   끝없이 돌지 않게 하려는 장치입니다.
5. 통과한 트리를 기억합니다. 마지막 통과 이후 바뀐 것이 없으면 다음 stop에서 다시 돌리지 않습니다.

문서만 바뀐 트리와 설치되지 않은 도구는 건너뜁니다.

**후속 라운드.** 빌드가 통과했고 바뀐 소스가 40줄 이상이면(`HEAD` 대비 추가와 삭제의 합, 추적되지 않은
새 소스 파일 포함) hook이 한 번 더 종료를 막고 다음을 지시합니다.

1. `reviewer` 에이전트로 `git diff HEAD`를 검토하고 blocker만 보고한다.
2. 변경에 실행할 수 있는 동작이 있으면 `qa-explorer`를 hunt 모드로 돌리고, 발견한 것을
   `auto qa hunt verify`로 판정한다.
3. 모든 blocker와 확인된 버그를 고치고, 테스트를 다시 돌린 뒤 무엇을 확인했는지 요약한다.

후속 라운드는 세션당 최대 2번이고, 새 라운드는 변경이 다시 40줄 늘어야 시작합니다. 그래서 리뷰 지적을
고친 것만으로 세 번째 라운드가 열리지는 않습니다.

**끄는 방법.**

| 범위 | 방법 |
| --- | --- |
| hook을 아예 등록하지 않기 | `autopus.yaml`에 `hooks.verify_stop: false`를 두고 `auto update` |
| 한 세션이나 셸에서만 끄기 | `AUTOPUS_VERIFY_STOP=off` |
| 빌드·테스트는 두고 후속 라운드만 끄기 | `AUTOPUS_FOLLOWUP=off`, 또는 `auto verify stop --followup=false` |

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

`auto qa`의 나머지(QAMESH journey, `auto qa run`, `auto qa release`, `auto qa report` 등)는 선언된 journey와
릴리스 근거용으로 그대로 있습니다. `auto qa --help`를 보세요.

<a id="routes"></a>
## 라우트

코딩 에이전트의 채팅창에서 실행합니다.

| 라우트 | 하는 일 |
| --- | --- |
| `/auto review` | 바뀐 코드를 TRUST 5(Tested, Readable, Unified, Secured, Trackable) 기준으로 검토하고, 프로젝트의 build·test·lint 명령 결과를 근거로 씁니다. 여러 provider에 나눠 맡길 수 있습니다. |
| `/auto qa` | 기본은 QA 버그 헌트, 선언된 journey와 릴리스 근거에는 QAMESH. |
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
| 스킬 (9) | `review`, `verification`, `qa-hunt`, `qa-autopilot`, `frontend-verify`, `browser-automation`, `playwright-cli`, `testing-strategy`, `lore-commit` |
| 라우트 | `review`, `qa`, `verify`, `test`, `sync`, `update`, `doctor`, 그리고 `browse` 별칭 |
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
Node 버전이 맞으면 `auto init`이 등록합니다(`--rea` / `--no-rea`).

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
- 새 설정의 기본값은 `methodology.mode: none`입니다. `tdd`를 쓰던 예전 설정은 `auto doctor`와 `auto check`에서
  실패가 아니라 안내로 보고됩니다.
- CLI 명령은 바이너리에 그대로 있습니다. `auto spec`, `auto pipeline`, `auto orchestra`, `auto workflow`를
  비롯해 `auto --help`의 명령은 계속 동작합니다. 바뀐 것은 에이전트 채팅에 생성되는 표면뿐입니다.

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

<a id="security"></a>
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
