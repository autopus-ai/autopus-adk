# 품질과 모델 설정

Autopus 에이전트와 리뷰 provider가 어떤 모델로 도는지 고르는 방법의 레퍼런스입니다. 품질 프리셋,
`auto quality` 구성 단계와 도구 탭, OMP 에이전트 model pin, 멀티프로바이더 리뷰 model과 SPEC 리뷰
judge, `auto init`의 REA MCP flag를 다룹니다. 여기 나오는 명령은 모두 자기 `--help`가 있고, 동작은
그 help가 최종 기준입니다.

[🇺🇸 English](quality.md)

## 명령

```bash
auto quality                          # 역할 그룹별로 에이전트 등급 구성 (대화형)
auto quality show                     # 저장된 품질 모드와 주 세션 정책 확인
auto quality show --agents            # 에이전트×플랫폼 모델 표도 읽기 전용으로 출력
auto quality ultra --apply            # Ultra를 기본값으로 저장하고 현재 프로젝트에 반영
auto quality balanced --apply         # Balanced를 기본값으로 저장하고 현재 프로젝트에 반영
auto quality provider claude ultra --apply   # Claude에만 적용 (claude-code alias도 허용)
auto quality provider codex balanced --apply # Codex에만 적용
auto quality provider claude inherit --apply # Claude override 제거
auto quality supervisor inherit --apply  # Codex 주 세션에서 사용자 기본 모델을 사용
auto quality review                   # 리뷰 provider의 참여·backend·model·effort와 judge 확인
auto quality review model claude claude-opus-5-5 --effort high  # 리뷰 provider 하나의 model 변경 (터미널 불필요)
auto quality review off gemini        # auto spec review에서 provider 빼기 (on으로 다시 켬)
auto quality review judge codex       # 리뷰 judge 고르기 (none은 judge 없는 리뷰)
auto quality review judge claude --model claude-opus-5-5 --effort max  # judge만 쓰는 model, 리뷰어는 그대로
```

## 프리셋과 provider override

`quality.default`는 모든 도구가 기본으로 따르는 프리셋입니다. 새 `auto init`은 built-in 프리셋
`balanced`(기본값)와 `ultra`를 등급 어휘 `fable`, `opus`, `sonnet`, `haiku`로 씁니다. Autopus가
설치하는 에이전트는 4개입니다.

| 에이전트 | balanced | ultra |
|---|---|---|
| frontend-specialist | sonnet | opus |
| reviewer | opus | fable |
| qa-explorer | opus | fable |
| ux-validator | sonnet | opus |

도구마다 등급을 자기 model id로 바꿉니다. `balanced`에서 생성되는 값은 다음과 같습니다
(`.claude/agents/autopus/<agent>.md`의 `model`·`effort`, `.codex/agents/<agent>.toml`의
`model`·`model_reasoning_effort`. Codex 열은 Codex catalog를 읽었을 때의 해석입니다).

| 에이전트 | Claude Code balanced | Codex balanced |
|---|---|---|
| frontend-specialist | `claude-sonnet-5-5` / `max` | `gpt-6-luna` / `max` |
| reviewer | `claude-opus-5-5` / `max` | `gpt-6-astra` / `max` |
| qa-explorer | `claude-opus-5-5` / `high` | `gpt-6.1-sol` / `xhigh` |
| ux-validator | `claude-sonnet-5-5` / `high` | `gpt-6-luna` / `max` |

Claude Code와 Codex는 서로 다른 프리셋을 따를 수 있습니다.

```yaml
quality:
  default: balanced
  providers:
    claude: ultra
    codex: balanced
```

`quality.providers`의 canonical key는 `claude`, `codex`이고, override가 없는 provider는
`quality.default`를 따릅니다. 한 번의 실행에 준 `--quality`는 YAML을 고치지 않는 명시적 전역
override이며 저장된 두 provider 값보다 우선합니다. Provider별 `--apply`는 그 플랫폼만, `auto
quality <mode> --apply`는 구성된 모든 플랫폼을 갱신합니다. Antigravity CLI와 OpenCode 에이전트에는
model이 없어 세션 모델로 돕니다.

새 프로젝트의 기본값 `quality.supervisor_model_policy: inherit`는 Codex 주 세션을 사용자 모델에
그대로 두고, 품질 모드는 관리형 에이전트에만 적용됩니다. `auto quality supervisor quality --apply`는
Autopus가 관리하는 주 세션 설정에 선택한 품질 프로필을 적용합니다. 사용자가 직접 정한 프로젝트
`model`이나 `model_reasoning_effort`는 그대로이고 먼저 적용됩니다. 반영한 뒤에는 새 Codex 세션을
시작하세요.

## 구성 단계

인자 없는 `auto quality`, 대화형 `auto update`, `auto init`의 품질 단계는 같은 구성 단계를 씁니다.
에이전트마다 한 행, 설치된 플랫폼마다 한 열인 표를 보여 줍니다.

| 열 | 칸 |
|---|---|
| 등급 | 생성에 쓰이는 등급 |
| Claude Code | `.claude/agents/autopus/<agent>.md`에 쓰일 `model · effort` |
| Codex | `.codex/agents/<agent>.toml`에 쓰일 `model · effort`. 이번 실행에서 한 번 읽은 Codex catalog로 해석하고, 같은 실행의 생성도 그 결과를 씁니다 |
| OMP | OMP 프로필이 요청하는 route. 머리말은 `요청 route · catalog 확인 전`이고 프로필도 pin도 없으면 `플랫폼 기본값` |
| OpenCode, Antigravity | `플랫폼 기본값` (에이전트 파일에 모델이 없음) |

편집은 역할 그룹으로 합니다. 그룹은 화면에만 있고 `autopus.yaml`에 쓰지 않습니다.

| 역할 그룹 | 에이전트 |
|---|---|
| 구현 | frontend-specialist |
| 검증·리뷰 | reviewer, qa-explorer, ux-validator |

`구성을 바꿀까요?`에서 Enter는 그대로 두고 아무것도 쓰지 않으며, `g`는 그룹마다 등급을 고르고
(Enter는 그 그룹을 그대로 둠), `a`는 에이전트마다 `agent=tier` 한 줄을 받고(빈 줄로 끝),
`t`는 도구별 설정을 열고, `u`(그런 프리셋이 있을 때만 보임)는 이전 버전 기본 프리셋을 갱신합니다.
미리보기는 바뀐 칸에 `*`를 붙이고 쓰는 key를 모두 보이며, `y`일 때만 쓰고 `e`는 편집을 이어 갑니다.
결과가 현재 `balanced`나 `ultra` 기본값과 같으면 그 프리셋을 고르고, 그 프리셋이 없거나 이전 버전
사본이면 프리셋도 씁니다. 그 밖의 결과는 `quality.presets.custom`과 `quality.default: custom`으로
저장합니다. 파일의 `balanced`와 `ultra`는 `현재 기본값`, `이전 버전 기본값 (갱신 가능)`, `사용자 수정
(N개 다름)`으로 표시되고, 사용자 수정 프리셋은 바꾸지 않습니다. `quality.providers.claude`나
`.codex`가 다른 프리셋을 가리키면 그 도구도 따를지 먼저 묻습니다. `auto quality --apply`는 확인 직후
재생성하고, 아니면 `auto update`를 실행하세요. `auto quality show --agents`는 어떤 stdin에서도 같은
표를 읽기 전용으로 출력합니다.

`auto update`는 대화형 실행마다 프로젝트 설정 질문 뒤에 한 번 묻습니다. Enter나 입력 끝은
`autopus.yaml`을 byte 단위로 그대로 두고, 확인한 변경은 같은 실행에서 생성됩니다. `--yes`/`-y`,
`--auto`, `--check`, truthy `CI`, TTY가 아닌 입력, `--workspace`는 묻지도 바꾸지도 않고, `--plan`,
`--preview`, `--dry-run`에는 이 단계가 없습니다.

구성 단계와 도구별 편집기는 quality 블록에 설정된 환경 변수의 `${NAME}` 값이나 YAML alias·merge
key가 있으면 쓰지 않고 `autopus.yaml`을 그대로 둡니다. 그때는 직접 고치세요.

### 폼과 도구 탭

터미널에서는 이 단계가 init wizard와 같은 폼입니다. 화살표와 Enter로 고르고, 스크립트나 파이프
입력은 줄 질문 그대로입니다. 메뉴는 유지 · 최신 기본값으로 갱신(built-in 프리셋이 이전 버전
기본값일 때) · 구성 편집 · 기본 프리셋 고르기(`auto quality`만)입니다. **구성 편집**은 도구 탭을 엽니다.

- **모든 도구 같게**는 위의 역할 그룹 구성입니다.
- **Claude Code**와 **Codex**는 그 도구만 바꿉니다. 결과는 `quality.presets.<provider>-custom`과
  `quality.providers.<provider>: <provider>-custom`으로 저장됩니다.
- **OMP**는 `auto update`·`auto quality`에서 autopus.yaml에 저장된 platforms에 omp가 있을 때,
  `auto init`에서 이번 init에서 고른 platforms에 omp가 있을 때 나옵니다.
- **멀티프로바이더 리뷰**는 [리뷰 model](#리뷰-model)의 조건에서 마지막 선택지로 나옵니다.

선택지마다 그 도구가 받을 model과 effort를 보입니다. Claude 라벨은 model slug(`opus-5-5`)이고 표
칸은 frontmatter 값입니다. 폼에서 Ctrl+C는 `auto update`를 생성 전에 끝내고, `auto init`은 파일을
쓰기 전에 취소하며, `auto quality`는 더 쓰지 않고 끝냅니다.

**Claude Code**와 **Codex** 탭에서는 등급 Select마다 그 도구의 effort Select가 이어집니다
(SPEC-MODELUX-003). 기본은 그대로이고, Claude Code는 `low`·`medium`·`high`·`xhigh`·`max`를 보이며
haiku 에이전트는 뺍니다. Codex는 catalog가 그 에이전트들의 model에 보고하는 단계만 보이고, catalog를
읽지 못하면 effort를 자동으로 둔다고 알립니다. 자동 단계에는 `(자동)`이 붙고 저장하지 않습니다.
고른 effort는 `quality.presets.<provider>-custom.efforts`에 쓰이고, 미리보기는 v0.50.127보다 오래된
auto가 `quality.presets.*.efforts`를 읽지 못한다고 알립니다. `efforts`가 없는 파일은 이전과 똑같이
생성됩니다.

## OMP 에이전트와 model pin

OMP는 Autopus 에이전트 4개를 `.omp/agents/<agent>.md`로 쓰고, 프로젝트 `.omp/config.yml`의
`task.disabledAgents: [scout, security-reviewer, sonic, designer, librarian, seance]`로 `task`를 뺀
OMP 기본 에이전트를 끕니다. `auto update`는 이 이동을 `OMP는 이제 Autopus 에이전트 4개를 씁니다.`로
알립니다. 프로필도 pin도 없으면 모든 에이전트가 OMP 세션 모델을 상속합니다.

**OMP** 탭은 프로필과 계열을 묻지 않습니다. 지금 pin이나 프로필 route에서 시작해 역할 그룹 2개나
에이전트별로(자기 pin이 있으면 `task` 포함) 고칩니다. model Select는 그대로 · `OMP 세션 모델 그대로` ·
Autopus가 증명할 수 있는 모든 provider의 설치 model을 보이고, family·capability metadata가 없는
catalog는 built-in 프로필이 선언한 model만 보입니다. thinking Select는 고른 model이 보고한 단계만
보입니다. 결과는 프로필 없는 pin으로 저장됩니다.

```yaml
role_model_policy:
  version: v1
  agents:
    reviewer:
      candidates:
        - {selector: <provider>/<model>, thinking: <level>, family: <family>}
```

미리보기에는 프로필 없는 `role_model_policy.agents`를 v0.50.127보다 오래된 auto가 읽지 못한다는
줄이 더해집니다. 자기 pin이 없는 `task`는 OMP 세션 모델을 쓰고, catalog에서 사라진 pin model은
`required_route_unresolved`로 막힙니다. 프로필 위에서 적용하면 확인 전에 프로필 route를 에이전트별
지정으로 옮긴다는 줄을 보이고, ultra 프로필은 더 이상 `quality.presets.ultra`를 따르지 않습니다.

줄 OMP 편집기와 프로필 명령은 그대로 프로필을 씁니다.

```bash
auto platform omp models                 # 설치된 model catalog
auto platform omp profile apply balanced --family gpt --plan
auto platform omp profile apply balanced --agent reviewer=<provider/model>:<thinking>
auto platform omp profile apply balanced --agent reviewer=inherit
auto platform omp explain
```

`role_model_policy.agent_source: native`는 v0.50.131에서 제거되어 `role_model_policy.agent_source_removed`로
로드에 실패합니다. key를 지우고 `auto update`를 실행하세요.

## 리뷰 model

`auto spec review`와 orchestra 명령은 `orchestra.providers`로 돕니다. 이 model은 에이전트 프리셋과
따로이며, `auto quality show`는 claude 리뷰 provider model을 별도 줄로 보입니다.

**멀티프로바이더 리뷰** 탭은 `spec.review_gate.enabled`가 켜져 있고 autopus.yaml에
`orchestra.providers` 항목이 둘 이상이며 `CI`가 아닐 때 도구 탭 마지막에 나옵니다(SPEC-REVMODEL-001).
표 아래 요약 줄이 리뷰 provider마다 원문의 model·effort와 judge를 보입니다. 탭이 정하는 것은 다음과
같습니다.

- `auto spec review`에 참여하는 provider(`spec.review_gate.providers`). 둘보다 적으면 거부합니다.
- provider마다 model과 effort. `backend: omp` provider는 그 family에서 Autopus가 증명하는 설치
  model과 thinking을 고릅니다(`orchestra.providers.<p>.model`). CLI backend의 `claude`는 Claude 탭
  model(`claude-fable-5-1`·`claude-opus-5-5`·`claude-sonnet-5-5`·`claude-haiku-4-5`)과 effort
  (`--model`·`--effort`), `codex`는 Codex catalog model과 그 effort(`-m`·`model_reasoning_effort`,
  `model_policy: pinned`), `agy`는 `agy models`가 보인 id(`--model`)를 고르고, 그 args 값만 바뀝니다.
  omp가 그 family의 model을 증명하면 CLI provider는 OMP backend(`backend: omp`와 `model`)로 전환할
  수 있습니다.
- judge(`spec.review_gate.judge`)와, 원하면 judge만 쓰는 model.

provider를 끄면 `auto spec review`만 바뀌고, orchestra 명령(review·secure)과
`auto spec review --multi`는 모든 provider를 계속 씁니다. 확인 호출을 고르면 리뷰 provider n개에
요청 1회씩(재시도 포함 최대 3n회 model 호출) 보내고 아무것도 쓰지 않습니다. 미리보기는 바뀐 CLI 값만
원문 그대로 보이고 다른 args는 보이지 않으며, `${NAME}`에서 오는 값은 바꾸지 않습니다. 바뀐 값만
다시 쓰므로 주석과 서식이 그대로입니다.

### judge model

judge Select의 마지막 선택지는 `<judge> — judge model 고르기`이고, 새 judge를 고르면 바로 묻습니다.
judge의 model과 effort를 같은 목록에서 골라 `spec.review_gate.judge_model`·`judge_effort`로
씁니다(SPEC-JUDGEMODEL-001). `auto spec review`의 judge 호출만 이 model을 쓰고, 리뷰어는 모두 자기
model을 씁니다. `리뷰어 model과 같음`은 두 key를 지우며, 이 key를 모르는 auto 릴리스는 이 key가 든
파일을 거부합니다.

```yaml
spec:
  review_gate:
    enabled: true
    providers: [claude, codex, gemini]
    judge: claude
    judge_model: claude-opus-5-5   # 선택
    judge_effort: max              # 선택
```

### 터미널 없이

`auto quality review`(또는 `show`, `--json`)는 provider마다 리뷰 참여·backend·원문의 model·effort와
judge를 보입니다. 아래 명령은 탭과 같은 검사로 바꿉니다.

```bash
auto quality review model <provider> <model> [--effort <effort>] [--dry-run]
auto quality review on <provider>... [--dry-run]
auto quality review off <provider>... [--dry-run]
auto quality review judge <provider|none> [--model <model>|default] [--effort <effort>] [--dry-run]
```

`model`은 탭이 그 provider에 주는 값만 받고, 받지 않는 값이면 가능한 값을 보입니다. `judge --model
default`는 리뷰어 model로 되돌립니다. 모든 편집은 바뀌는 설정을 먼저 보이고, 바뀐 것이 없거나
`--dry-run`이면 쓰지 않습니다. 비대화형 `auto init`과 `auto update`는 리뷰 model을 바꾸는 곳을 한 줄로
알립니다.

## 이동과 init 확인 화면

`auto init` wizard, 구성 단계, init 확인 화면의 폼마다 그 폼에 온 경로를 담은 header(예:
`init · 4/6 리뷰 게이트`, `update · 모델 구성 › 구성 편집 › Codex`), 지금까지 고른 값, footer
`방향키 이동 · Enter 선택 · Esc 뒤로 · Ctrl+C 취소`가 터미널 높이 안에 보입니다(SPEC-TUIUX-001).
Esc는 한 폼 뒤로 가고 다시 보인 폼은 앞 답을 고른 채입니다. 첫 폼의 Esc는 `첫 단계입니다. 취소하려면
Ctrl+C를 누르세요.`를 보이고 머뭅니다. Esc는 이미 적용한 쓰기를 되돌리지 않습니다. 접근성 줄
모드(`TERM=dumb`)에서는 `b`·`back`·`뒤로` 한 줄이 Esc이고, 줄 질문에는 뒤로가 없습니다.

폼으로 묻는 `auto init`은 아무 파일도 쓰기 전에 `적용할 설정`을 보입니다: 답한 값, 그 값이 바꾸는
설정 key 전부, REA 결정과 동작(예: `REA MCP: 켬 · 등록`). 그 뒤 `이대로 진행할까요?`에 적용(기본값)·
뒤로·취소를 고르고, 취소와 Ctrl+C는 `init cancelled.`를 쓰고 프로젝트를 그대로 둡니다. 확인 화면에
없는 것: 그 뒤의 orchestra 정규화, statusLine·parent rule 질문, 생성 뒤 정하는 `features.rea`, 생성
파일. `--yes`, TTY가 아닌 입력, 줄 질문에는 뒤로와 확인 화면이 없습니다.

## REA MCP init flag

`auto init`은 Claude Code·Codex·Antigravity CLI에 프로젝트 범위 REA 역공학 MCP(`rea-agents@6.5.0`)를
등록합니다(SPEC-REA-001). v0.51.0부터 opt-in이라 새 init은 기본으로 끄고, `--rea`·`--no-rea`로 명시적으로 정하며, 둘을 함께
쓰면 아무것도 쓰기 전에 끝납니다. Node가 `^22.19.0 || ^24.11.0 || >=26.0.0`일 때만 등록합니다. Claude
Code·Antigravity 서버는 POSIX `/bin/bash`로 시작하므로 Windows에서는 시작되지 않습니다.
OMP·OpenCode에는 항목을 만들지 않습니다.

`features.rea.enabled`는 managed 항목이 결정을 담지 못할 때만 쓰며, `features.rea는 v0.50.127보다
오래된 auto가 읽지 못합니다`(해당 릴리스는 v0.50.113–v0.50.125). `auto update`는 결정이 없으면 대화형
실행에서 한 번 묻고 그 밖에는 hint만 씁니다. `--no-rea`는 managed 항목을 지우고 사용자 소유 항목은
그대로 둡니다. `auto doctor`는 서버를 띄우지 않고 결정, Node probe, 플랫폼마다의 항목을 보고합니다.
