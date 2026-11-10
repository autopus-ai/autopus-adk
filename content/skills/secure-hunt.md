---
name: secure-hunt
description: 스캐너 결과를 단서로 삼고, 변경된 표면의 위협 모델을 세운 뒤 실제 공격을 시도해 재현 스크립트로 확인된 취약점만 보고하는 보안 검증 스킬
triggers:
  - secure
  - security
  - 보안 검증
  - 보안 점검
  - 취약점 찾기
  - security hunt
  - pentest
category: security
level1_metadata: "auto secure scan으로 단서 수집, 변경 표면 위협 모델, 로컬·스테이징 대상에 실제 공격, repro.sh + auto qa hunt verify로 판정, 확인되지 않은 의심은 따로 보고"
---

# Secure Hunt Skill

모델은 OWASP Top 10을 이미 압니다. 이 스킬은 목록을 다시 외우게 하지 않고, **어디를 먼저 보고, 무엇을 버리고,
무엇을 취약점이라고 부를지** 정하는 절차와 판단을 줍니다.

판정은 에이전트가 하지 않습니다. 스캐너 출력과 코드 리딩은 단서일 뿐이고, 공격이 실제로 통하는지 보여 주는
재현 스크립트만 취약점이 됩니다. 재현 스크립트는 `qa-hunt`와 같은 규약으로 쓰고 `auto qa hunt verify`로
판정합니다.

## 안전 경계 (먼저 읽기)

- 공격 대상은 사용자의 로컬 빌드, 로컬 서버, 사용자가 소유한 스테이징뿐입니다. 제3자 시스템, 운영 환경,
  공유 SaaS, 외부 API에는 공격 요청을 보내지 않습니다. 대상이 분명하지 않으면 공격하지 않고 묻습니다.
- 공유 데이터를 지우거나 망가뜨리는 payload(`DROP`, 대량 삭제, 계정 잠금, 과부하)를 쓰지 않습니다.
  파괴를 증명해야 하면 일회용 로컬 DB·임시 디렉터리에서만 합니다.
- 자격 증명을 밖으로 보내지 않습니다. SSRF·유출을 증명할 때는 로컬 listener나 canary 값으로 도달만 확인합니다.
- 비밀값을 출력하지 않습니다. 화면·finding·memory·증거 파일에는 위치(파일:줄, 규칙 id)와 앞 4자 정도의
  지문만 남깁니다.
- 제품 코드·테스트·설정은 고치지 않습니다. 쓰는 곳은 `.autopus/qa/hunt/` 아래뿐입니다.

## 절차

### 1. 기계적 스캔으로 단서 모으기

```bash
auto secure scan --json
```

언어(Go·Node·Rust·Python)를 감지해 설치된 스캐너(gitleaks, govulncheck, npm·pnpm·yarn audit, cargo audit,
pip-audit)를 돌립니다. 비밀값이나 high·critical 결과가 있으면 exit 1입니다. 바이너리가 오래되어 명령이 없으면
그렇다고 말하고, 설치된 스캐너를 직접 돌립니다(`gitleaks detect --redact`, `govulncheck ./...` 등). 설치되지
않은 스캐너는 "돌리지 못함"으로 보고에 남깁니다. 없는 결과를 있다고 쓰지 않습니다.

스캐너 결과는 취약점이 아니라 단서입니다. 하나씩 분류합니다.

- 의존성 취약점: 그 코드가 **실제로 호출되는지** 봅니다. govulncheck는 심볼 도달성을 알려 주므로
  "imported but not called"는 낮게, 호출 경로가 있는 것은 높게 둡니다. npm audit류는 devDependency인지,
  런타임 번들에 들어가는지, 취약한 함수를 쓰는지 확인합니다.
- 비밀값: 실제 키인지, 테스트 fixture·예시값인지, 이미 폐기된 값인지 구분합니다. 실제 키가 git 기록에 있으면
  파일을 지워도 남는다는 점과 폐기(rotate)가 먼저라는 점을 적습니다. 값은 출력하지 않습니다.
- 오탐은 이유와 함께 "확인되지 않음"에 적고 넘어갑니다.

### 2. 변경된 표면의 위협 모델

전체 코드베이스가 아니라 이번 변경(`git diff HEAD --stat`, 또는 사용자가 준 경로)에서 시작합니다. 짧게 적습니다.

| 항목 | 질문 |
|---|---|
| 진입점 | HTTP 핸들러, CLI 인자·환경변수, 파일·아카이브 입력, 메시지 큐, webhook, CI 트리거 중 무엇이 바뀌었나 |
| 신뢰 경계 | 인증 전·후, 사용자·관리자, 테넌트 사이, 프로세스·컨테이너·네트워크 경계가 어디서 넘어가나 |
| 자산 | 지켜야 할 것: 자격 증명, 개인정보, 다른 사용자의 데이터, 서명 키, 빌드 산출물, 실행 권한 |
| 공격자 입력 | 공격자가 조종할 수 있는 값: 경로, URL, ID, 헤더, 직렬화된 본문, 파일명, PR 제목·브랜치 이름 |

여기서 나온 "입력 → 경계 → 자산" 경로가 다음 단계의 공격 목록입니다. 경로가 없는 범주는 공격하지 않습니다.

### 3. 공격 시도

경로마다 실제로 공격해 봅니다. 코드만 읽고 "취약할 수 있다"고 쓰지 않습니다. 자주 통하는 것:

- 주입: SQL·NoSQL·셸·템플릿·LDAP. 문자열 연결로 쿼리나 명령을 만드는 곳에 구분자와 따옴표를 넣어 본다.
- 인가 우회와 IDOR: 사용자 A의 토큰으로 사용자 B의 ID를 요청한다. 목록·수정·삭제·내보내기 엔드포인트를 모두.
- 경로 순회: `../`, 절대 경로, 심볼릭 링크, URL 인코딩, 아카이브 안의 `../`(zip slip).
- SSRF: 서버가 대신 가져오는 URL에 `127.0.0.1`, 메타데이터 주소, 리다이렉트, DNS 재바인딩. 도달 여부는 로컬
  listener로만 확인한다.
- 역직렬화: 타입 정보를 담는 포맷(gob, pickle, YAML 태그, Java 직렬화)을 신뢰하지 않는 입력에 쓰는 곳.
- 로그와 오류 속 비밀: 실패 경로를 일부러 밟아 토큰·비밀번호·쿠키가 로그나 오류 응답에 찍히는지 본다.
- 안전하지 않은 기본값: 디버그 모드, 넓은 CORS, 인증 없는 관리 엔드포인트, 0.0.0.0 바인딩, 0777 권한, TLS 검증 끔.
- 공급망과 CI: `pull_request_target`이 PR 코드를 checkout해 실행하는지, 공개 repo가 self-hosted runner를 쓰는지,
  PR 제목·브랜치 이름 같은 `github.event` 값이 expression으로 `run:` 스크립트에 그대로 들어가는지, action이 SHA가 아닌 태그로 고정됐는지,
  `GITHUB_TOKEN` 권한이 필요 이상인지. CI는 실행하지 않고 workflow 파일과 트리거 조건으로 재현한다.

`--multi`를 받았을 때만 `auto orchestra secure <files>`로 다른 모델의 의견을 더합니다. 토큰을 많이 쓰고,
그 결과도 단서일 뿐입니다.

### 4. 재현 스크립트로 확인

의심 하나마다 `.autopus/qa/hunt/findings/sec-<id>/`에 `finding.json`과 `repro.sh`를 씁니다. 규약은 `qa-hunt`와
같습니다: **취약점이 있으면 exit 1, 고쳐졌으면 exit 0**, 그 밖의 종료 코드는 스크립트 자체의 실패입니다.
프로젝트 root에서 사람 손 없이 처음부터 끝까지 돌아야 하고, 필요한 서버는 스크립트가 로컬에서 띄우고 내립니다.

```bash
#!/usr/bin/env bash
# Vulnerability present -> exit 1. Fixed -> exit 0.
set -u
status=$(curl -s -o /dev/null -w '%{http_code}' \
  -H "Authorization: Bearer $(cat .autopus/qa/hunt/drivers/user-a.token)" \
  http://127.0.0.1:8080/api/orders/order-of-user-b)
if [ "$status" = "200" ]; then
  echo "user A read user B's order (IDOR)"; exit 1
fi
exit 0
```

`finding.json`은 `qa-hunt`와 같은 스키마(`autopus.qa.hunt.finding.v1`)입니다. `oracle`은 `other`로 두고
`title`에 공격 종류를 적습니다. `severity` 필드는 `auto qa hunt verify`가 `critical`·`major`·`minor`만 받으므로
보안 등급을 다음처럼 옮깁니다: critical → `critical`, high → `major`, medium·low → `minor`. 원래 등급은 보고서에 씁니다.

```bash
auto qa hunt verify --id sec-<id>   # 하나만
auto qa hunt verify                 # 모든 finding
```

`confirmed`만 취약점으로 보고합니다. `flaky`, `not_reproduced`, `broken_repro`는 의심입니다.

### 5. 보고

확인된 것과 확인되지 않은 것을 섞지 않습니다.

```
🐙 secure-hunt ─────────────────────
  scan: gitleaks ✓ govulncheck ✓ npm audit – (미설치)
  confirmed {N} | unconfirmed {N}
```

**확인된 취약점** — `confirmed`마다:

- id, 한 줄 요약, 보안 등급(critical·high·medium·low)과 그 이유(누가, 무엇을, 어떤 전제로 얻나)
- 진입점 → 신뢰 경계 → 자산 경로
- `bash .autopus/qa/hunt/findings/sec-<id>/repro.sh`
- 고치는 방향: 증상이 아니라 원인 위치(파일:줄)와 고칠 방식. 입력 필터보다 parameterized query, 경로 정규화 후
  base 디렉터리 검사, 객체 단위 인가 검사처럼 구조적인 수정을 먼저 제안합니다.

**확인되지 않음** — 스캐너가 보고했지만 도달하지 않는 것, 재현하지 못한 의심, 대상이 없어 공격하지 못한 범주,
돌리지 못한 스캐너. 각각 왜 확인되지 않았는지 한 줄로.

등급 기준: critical은 인증 없이 원격에서 코드 실행·전체 데이터 접근·자격 증명 탈취, high는 인증된 사용자가
권한 밖 데이터·동작에 접근, medium은 조건이 까다롭거나 영향이 제한적인 것, low는 방어 심층화 차원의 문제입니다.

### 6. memory 갱신

`.autopus/qa/hunt/memory.md`에 다음 실행이 알아야 할 것만 덧붙입니다: 대상 서버를 띄우는 방법, 테스트 계정을
만드는 방법(값이 아니라 절차), 오탐으로 판정한 스캐너 항목과 이유, 확인된 취약점 id.

## 금지 사항

- `auto qa hunt verify`를 거치지 않은 의심을 취약점이라고 보고하지 않습니다.
- 스캐너 경고 개수를 보안 수준이라고 보고하지 않습니다.
- repro를 통과시키려고 기대값을 바꾸지 않습니다.
- 안전 경계 밖의 대상에 공격 요청을 보내지 않고, 비밀값을 어디에도 출력하지 않습니다.
