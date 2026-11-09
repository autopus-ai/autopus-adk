# QA agent matrix

배포 전에 실제 에이전트 CLI로 QA 체인 전체를 돌려 보는 스크립트입니다. 에이전트마다 고유한 결함이
드러났기 때문에(codex의 `exit_code: nonzero`, agy의 조용한 권한 거부, opencode의 예시 경로 복사), 한 에이전트로만
확인하고 "지원한다"고 말할 수 없습니다.

```bash
scripts/qa-agent-matrix/run.sh                     # claude codex agy opencode × wordtop itemsapi
scripts/qa-agent-matrix/run.sh -a claude -f itemsapi
scripts/qa-agent-matrix/run.sh -b ./auto -o /tmp/matrix   # 빌드된 바이너리와 출력 위치 지정
```

칸마다 하는 일:

1. `testdata/<fixture>/`의 제품(버그 하나가 심겨 있음)을 임시 git 저장소에 복사합니다.
2. `auto qa intent` → `auto qa intent answer --recommended` → `auto qa go --auto`를 돌립니다.
3. 가장 최근 loop 브랜치의 끝을 꺼내 `testdata/oracles/<fixture>_oracle_test.go`로 판정합니다. 에이전트는 이
   oracle을 볼 수 없습니다. loop가 통과해도 에이전트가 쓴 시나리오끼리만 맞는 수정이면 여기서 걸립니다.

oracle은 요청이 모호하지 않게 정한 것만 고정합니다. 출력 줄 형식처럼 요청이 정하지 않은 것은 권장 답에 따라
달라질 수 있으므로 검사하지 않습니다.

| fixture | 심은 버그 | 제품 종류 |
|---|---|---|
| `wordtop` | 대소문자가 다른 단어를 따로 셈 | CLI (exec case) |
| `itemsapi` | 저장한 이름을 대문자로 돌려줌 | HTTP 서버 (exec `requests`) |

모든 칸이 oracle을 통과하면 0, 아니면 1로 끝납니다. 결과는 `<out>/results.tsv`, 칸별 로그는 그 옆에 남습니다.
실제 모델을 호출하므로 비용이 들고, 공개 저장소 CI에서는 돌리지 않습니다(fork PR이 자격 증명에 닿지 않게).
