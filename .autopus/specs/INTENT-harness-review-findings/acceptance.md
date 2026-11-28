<!-- intent_source: request. Written by `auto qa intent` from request.md alone, without the code.
     Record answers with `auto qa intent answer`; scenario generation reads them from questions.json. -->

### AC-1: 동기 리뷰 결과에서 file:line 지적을 모은다
Given 리뷰어의 동기 리뷰 결과에 프로젝트 안 파일을 가리키는 `src/auth/login.go:42` 같은 지적이 들어 있는 세션 기록이 있다
When 세션이 끝나 Stop hook이 실행된 뒤 `auto harness report`를 실행한다
Then 리포트에 그 지적이 리뷰 지적 출처로 집계되어 있다(그 지적이 영역 `src/auth`의 세션 하나로 세어진다)

### AC-2: 백그라운드 리뷰어가 나중에 보낸 메시지도 모은다
Given 백그라운드 리뷰어가 나중에 보낸 메시지에 프로젝트 안 file:line 지적이 들어 있는 세션 기록이 있다
When Stop hook이 실행된 뒤 `auto harness report`를 실행한다
Then 그 지적이 동기 리뷰 결과의 지적과 똑같이 집계되어 있다

### AC-3: 지적을 모을 때 모델을 부르지 않는다
Given 어떤 provider도 쓸 수 없는 환경(자격 증명 없음, 네트워크 차단)이고, 리뷰 지적이 들어 있는 세션 기록이 있다
When Stop hook이 실행된다
Then hook이 오류 없이 끝나고, 이어서 실행한 `auto harness report`에 그 세션의 리뷰 지적이 집계되어 있다

### AC-4: 프로젝트 밖 경로는 지적으로 세지 않는다
Given 리뷰어 보고서에 프로젝트 밖 경로를 가리키는 지적(예: `/usr/lib/go/src/net/http.go:10`, `~/other-repo/a.go:3`)만 들어 있는 세션 기록이 있다
When Stop hook이 실행된 뒤 `auto harness report`를 실행한다
Then 리포트에 그 경로들의 지적이 하나도 집계되어 있지 않다

### AC-5: 코드 블록 안의 재현 명령은 지적으로 세지 않는다
Given 리뷰어 보고서의 코드 블록 안에만 file:line 형태의 텍스트(예: 재현 명령 `go test ./src/auth/login_test.go:12`)가 있는 세션 기록이 있다
When Stop hook이 실행된 뒤 `auto harness report`를 실행한다
Then 코드 블록 안의 file:line은 지적으로 집계되어 있지 않다

### AC-6: 재리뷰의 상태 표는 지적으로 세지 않는다
Given 재리뷰 보고서에 이전 지적의 file:line과 상태(예: fixed / still open)를 나열한 표만 있는 세션 기록이 있다
When Stop hook이 실행된 뒤 `auto harness report`를 실행한다
Then 상태 표의 file:line은 새로운 지적으로 집계되어 있지 않다

### AC-7: 리뷰어가 아닌 에이전트의 메시지는 읽지 않는다
Given 리뷰어가 아닌 에이전트(예: 구현이나 탐색 에이전트)의 메시지에만 프로젝트 안 file:line이 들어 있는 세션 기록이 있다
When Stop hook이 실행된 뒤 `auto harness report`를 실행한다
Then 그 file:line은 리뷰 지적으로 집계되어 있지 않다

### AC-8: 같은 영역에서 세션 3개 이상이면 반복 패턴이 된다
Given 서로 다른 세션 3개에서 각각 경로의 앞 두 단계가 같은 파일(예: `src/auth/login.go:42`, `src/auth/token/jwt.go:7`, `src/auth/session.go:88`)을 가리키는 리뷰 지적이 나왔다
When `auto harness report`를 실행한다
Then 리포트에 영역 `src/auth`의 반복 리뷰 지적 패턴이 나오고, 세 세션 각각의 예시 지적이 하나씩 보인다

### AC-9: 세션 2개뿐이면 반복 패턴이 아니다
Given 영역 `src/auth`의 리뷰 지적이 서로 다른 세션 2개에서만 나왔다(한 세션 안에서 여러 개 나와도 마찬가지)
When `auto harness report`를 실행한다
Then 리포트에 영역 `src/auth`의 반복 리뷰 지적 패턴이 나오지 않는다

### AC-10: 경로 앞 두 단계가 다르면 같은 영역으로 묶지 않는다
Given 세션 3개에서 각각 `src/auth/a.go:1`, `src/billing/b.go:2`, `src/auth/c.go:3`을 가리키는 지적이 나왔다
When `auto harness report`를 실행한다
Then `src/auth`도 `src/billing`도 반복 패턴으로 나오지 않는다(`src/auth`는 세션 2개, `src/billing`은 세션 1개)

### AC-11: 초안이 같은 실수를 구체적으로 쓰면 후보가 된다
Given 리뷰 지적 반복 패턴이 있고, 그 지적들이 lint·타입 검사·테스트로 잡을 수 있는 하나의 같은 실수다
When 초안 단계가 그 패턴을 처리한다
Then 사람이 승인할 후보가 만들어지고, 후보 내용에 검사로 바꿀 수 있을 만큼 구체적으로 쓴 실수(어떤 코드 모양인지)가 들어 있다

### AC-12: 초안이 NONE이면 후보를 만들지 않고 대기한다
Given 리뷰 지적 반복 패턴이 있지만 그 지적들이 하나의 같은 실수가 아니거나 검사가 잡을 수 있는 코드 모양이 아니어서 초안이 NONE으로 답한다
When 초안 단계가 끝난 뒤 `auto harness report`를 확인하고 곧바로 harness를 다시 실행한다
Then 그 패턴의 후보는 승인 대기 목록에 없고, 그 패턴은 실패한 초안과 같은 대기(backoff) 상태로 보이며, 대기 기간 안에는 초안을 다시 쓰지 않는다

### AC-13: 반증 단계는 지적이 그 실수를 보여 주지 않으면 반증한다
Given 리뷰 지적 패턴의 초안이 만들어졌지만 모은 지적들이 초안이 쓴 실수를 보여 주지 않는다
When 다른 provider의 반증 단계가 그 초안을 검토한다
Then 초안이 반증되고 승인 대기 후보로 넘어가지 않는다

### AC-14: 반증 단계는 검사로 구분할 수 없는 실수를 반증한다
Given 리뷰 지적 패턴의 초안이 쓴 실수가 lint·타입 검사·테스트로 맞는 코드와 구분할 수 없는 것이다
When 반증 단계가 그 초안을 검토한다
Then 초안이 반증되고 승인 대기 후보로 넘어가지 않는다

### AC-15: 반증을 통과한 후보는 사람 승인과 encode로 이어진다
Given 리뷰 지적 패턴에서 나온 후보가 반증 단계를 통과했다
When 사람이 그 후보를 승인한 뒤 `auto harness encode`를 실행한다
Then 승인한 교훈이 다른 출처의 교훈과 같은 방식으로 저장소 검사로 바뀐다
