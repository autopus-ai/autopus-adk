# 릴리즈 좌표 옮기기 (A{n} = vX → A{n+1} = vY)

게시된 릴리즈 vX 다음 좌표 vY로 옮기는 일은 측정과 적용 두 명령이다. A44, A45, A46
이동에서 손으로 하던 일(validate-source.sh phase 행과 ancestor 핀, 계보 선언과 분기, phase
목록, upgrade canary·preflight·Homebrew 전임자 핀, 현재 좌표를 고정한 테스트, 두 runbook)을
모두 스크립트가 한다. 깨끗한 checkout에서 실행한다.

```bash
# 1. vX를 읽기만 해서 잰다. 전임자 commit은 계보 좌표의 {이전 phase}_COMMIT_SHA다.
scripts/release-tools/measure-published-release.sh vX \
  --predecessor-commit "$(sed -nE "s/^readonly A{n-1}_COMMIT_SHA='([0-9a-f]{40})'$/\1/p" \
    scripts/companion-release/verify-public-key-lineage-coordinates.sh)" >/tmp/vX.json

# 2. 측정값으로 옮긴다.
scripts/release-tools/advance-release-coordinate.sh vX A{n} vY A{n+1} --measurements /tmp/vX.json
```

## 측정 (`measure-published-release.sh`)

GitHub에는 아무것도 쓰지 않는다. gh API와, 임시 bare 저장소로 받는 tree 없는 fetch만 쓴다.
하나라도 어긋나면 JSON을 내지 않고 멈춘다.

- release: `releases/tags/vX`가 draft 아님, prerelease 아님, immutable, author `204883817`,
  asset 15개다. Latest 여부는 기록만 한다.
- tag: API의 tag object가 Git의 것과 같고, 태그 헤더가 정확히 그 commit, `commit`, `vX`를
  가리키며, SSH 서명이 R2 trust root(`release-tag-signing-2026-q3-r2.pub`)로 검증된다. tree는
  API와 Git이 같다. release commit은 main의 조상이고, `--predecessor-commit`은 release
  commit의 조상이다.
- asset: `checksums.txt`와 darwin·linux 아카이브 네 개를 새 `mktemp -d`에 내려받아 바이트
  sha256이 API digest와 같고, 아카이브는 `checksums.txt` 항목과 같다.
- manifest: darwin 아카이브 안 `adk-companion-manifest.json`의 sha256을 재고, manifest가
  version, platform, architecture, 옆 `auto` 바이너리의 `artifact_digest`, build provenance
  commit을 정확히 말하는지 본다.
- 증거 태그 `omp-context-evidence-vX`의 object와 commit.
- Homebrew tap: head가 "Publish signed Cask for vX" 단일 parent commit이고, Cask blob이
  내용과 같고, Cask가 version과 아카이브 digest 네 개를 담고, Formula blob이 동결값이다.

## 적용 (`advance-release-coordinate.sh … --measurements`)

vX가 게시돼 있을 때만 측정 모드로 넘어가고(`advance-release-coordinate-measured.sh`), 게시
안 된 vX에 `--measurements`를 주면 거절한다. 측정 없이 게시된 vX를 옮기려 하면 이전처럼
멈추고 위 두 명령을 안내한다. 측정값은 추측하지 않는다는 기존 guard는 그대로다.

1. vX runbook의 Published 기록 값(Release run과 결론, Upgrade Canary run과 결론,
   `autopus-vX-release-authority` ruleset id와 봉인 여부, deployment tag policy id)을 gh로
   읽는다. 못 읽은 값이 있으면 아무것도 쓰기 전에 멈춘다. `--release-run`,
   `--release-conclusion`, `--canary-run`, `--canary-conclusion`, `--ruleset-id`,
   `--ruleset-state`, `--tag-policy-id`, `--note`로 직접 줄 수 있고, `--no-runbooks`는
   runbook을 건너뛴다.
2. `advance-release-coordinate-apply.pl`이 `advance-release-coordinate-rules.pl`의 파일을
   모두 옮긴다. 이전 전임자 핀은 tree(계보 선언, Homebrew publisher)에서, 새 핀은 측정 JSON에서
   온다. 측정이 tree의 사슬을 잇지 않으면(전임자 commit, tap parent, Formula blob) 거절한다.
   규칙 하나라도 맞지 않으면 파일을 하나도 쓰지 않는다.
3. `docs/runbooks/release-coordinate-template.md`로 `release-vY.md`를 만들고 vX runbook 맨
   위에 Published 기록을 넣는다.
4. 바뀐 셸 파일 `bash -n`, Go 파일 `gofmt`, 바뀐 소스 파일의 300줄 한도를 확인한다. 계보
   coordinates 파일이 300줄을 넘으면 오래된 핀 블록을
   `verify-public-key-lineage-shipped-pins.sh`로 먼저 옮겨야 한다.

## 규칙 (`advance-release-coordinate-rules.pl`)

이동은 한 좌표씩 미는 일이다. 전임자를 가리키던 것은 vX를, vX를 가리키던 것은 vY를 가리키게
되고, 이전 전임자 핀은 측정한 vX 핀으로 바뀐다. 한 줄에서 한 번에 치환하므로 A44 → A45가
A46까지 이어 바뀌지 않는다. 파일마다 방식이 정해져 있다.

- `shift`: 파일 전체를 민다. `keep`이 지킨 줄은 빼고 민다.
- `keep`: 이력 줄이다. 버전이 아니라 사건 내용으로 찾는다(예: "ruleset was the 34th",
  "published at 08:13:08"). 버전으로 찾으면 다음 이동에서 그 버전도 밀리기 때문이다. 아무 줄도
  못 찾은 keep은 오류다.
- `clone`: 쌓이는 항목이다. vX 항목은 그대로 두고 민 사본을 바로 뒤에 넣는다(phase 행,
  ancestor 핀과 arm, receipt 행, 계보 선언과 분기, shipped phase 표, fail-closed matrix 행).
- `append`: phase 목록과 frozen-policy 문구 끝에 새 phase를 붙인다. 맞은 횟수가 정해진 수와
  다르면 오류다.

새 파일이 현재 좌표를 고정하게 되면 이 목록에 넣는다. 이력 주석을 새로 남기면 그 내용을 `keep`에
넣는다. `scripts/companion-release/tests/release-coordinate-autopilot-test.sh`가 현재 tree를
fixture 측정으로 한 좌표 옮겨 보고, 이력 줄이 바이트 그대로인지, 지운 줄마다 좌표 토큰이
있는지(엉뚱한 줄이 바뀌지 않았는지) 확인한다. Go 테스트
`TestReleaseCoordinateAutopilotBashContract`가 이를 CI에서 돌린다.

## 증명

이 규칙은 A45 → A46 이동(`795a40f9`)에서 나왔고, 그 부모 `04bafae0`에서 다시 돌리면 코드
파일 43개가 `795a40f9`와 바이트 단위로 같다(`git diff 795a40f9`가 비고 numstat이 일치).
다른 것은 runbook 문장뿐이다. 자동 runbook은 측정값과 절차만 담고, 사건 설명은
`--note`나 이후 손 편집으로 더한다.
