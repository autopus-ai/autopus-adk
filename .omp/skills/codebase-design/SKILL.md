---
name: codebase-design
description: 작은 인터페이스 뒤에 많은 동작을 숨기는 deep module을 설계하기 위한 공용 어휘(module, interface, seam, adapter, depth)와 원칙
compatibility: omp
---

# Codebase Design

**deep module**을 설계한다: 작은 인터페이스 뒤에 많은 동작, 깨끗한 seam에 놓이고, 그 인터페이스를
통해 테스트된다. 코드를 설계하거나 재구성하는 모든 자리에서 이 언어와 원칙을 쓴다. 목표는
호출자에게 leverage, 유지보수자에게 locality, 모두에게 testability다. 에이전트는 코딩을
가속하는 만큼 엔트로피도 가속하므로 설계에 *매일* 투자한다.

## 어휘

정확히 이 단어를 쓴다. "component", "service", "API", "boundary"로 바꿔 부르지 않는다.

- **Module**: 인터페이스와 구현을 가진 모든 것. 규모 무관 — 함수, 클래스, 패키지, 계층을
  가로지르는 슬라이스. _Avoid_: unit, component, service.
- **Interface**: 호출자가 모듈을 올바르게 쓰기 위해 알아야 하는 전부. 타입 시그니처뿐 아니라
  불변 조건, 호출 순서, 에러 모드, 필수 설정, 성능 특성, 그리고 **암묵 입력·출력**(cwd, 환경
  변수, ctx에 숨은 값, stdout/stderr, 파일 부수효과). 평가할 때 이 항목을 점검표로 훑는다.
  _Avoid_: API, signature(타입 표면만 가리켜 너무 좁다).
- **Implementation**: 모듈 안쪽의 코드 본체. **Adapter**와 구분한다 — 작은 adapter에 큰 구현
  (Postgres repo)도, 큰 adapter에 작은 구현(in-memory fake)도 있다.
- **Depth**: 인터페이스에서의 leverage. 호출자(또는 테스트)가 배워야 하는 인터페이스 단위당
  쓸 수 있는 동작의 양. 작은 인터페이스 뒤에 많은 동작이면 **deep**, 인터페이스가 구현만큼
  복잡하면 **shallow**.
- **Seam** _(Michael Feathers)_: 그 자리를 편집하지 않고 동작을 바꿀 수 있는 장소. 모듈의
  인터페이스가 *놓이는 위치*. 어디에 둘지는 무엇을 뒤에 둘지와 별개의 설계 결정이다.
  _Avoid_: boundary(DDD의 bounded context와 겹친다).
- **Adapter**: seam에서 인터페이스를 만족하는 구체물. 무엇이 들어 있는지가 아니라 어떤
  *역할*(슬롯)을 채우는지를 말한다.
- **Leverage**: depth가 호출자에게 주는 것. 배운 인터페이스 단위당 더 많은 능력. 구현 하나가
  N개 호출 지점과 M개 테스트에서 회수된다.
- **Locality**: depth가 유지보수자에게 주는 것. 변경, 버그, 지식, 검증이 여러 호출자로 퍼지지
  않고 한 곳에 모인다. 한 번 고치면 전부 고쳐진다.

## Deep vs shallow

```
deep:     ┌──────────────┐        shallow:  ┌──────────────────────────┐
          │ 작은 인터페이스 │                  │      큰 인터페이스        │
          ├──────────────┤                  ├──────────────────────────┤
          │              │                  │  얇은 구현(그냥 넘겨줌)     │
          │  깊은 구현     │                  └──────────────────────────┘
          └──────────────┘
```

인터페이스를 설계할 때 묻는다: 메서드 수를 줄일 수 있나? 파라미터를 단순화할 수 있나?
더 많은 복잡성을 안에 숨길 수 있나?

## 원칙

- **Depth는 구현이 아니라 인터페이스의 속성이다.** deep module 안쪽은 작고 교체 가능한 부품으로
  구성돼도 된다 — 인터페이스의 일부가 아닐 뿐. 모듈은 외부 seam 외에 자기 테스트가 쓰는
  **internal seam**을 가질 수 있다. 테스트가 쓴다는 이유로 internal seam을 인터페이스로
  노출하지 않는다.
- **삭제 테스트.** 모듈을 지운다고 상상한다. 복잡성이 사라지면 pass-through였다. 복잡성이
  N개 호출자로 다시 나타나면 제 몫을 하던 모듈이다. 판정할 때 운영 호출자 수, 테스트 호출자 수,
  사라지는 복잡성, 다른 곳으로 *이동*하는 복잡성을 따로 적는다 — 이동은 사라짐이 아니다.
- **인터페이스가 테스트 표면이다.** 호출자와 테스트는 같은 seam을 건넌다. 인터페이스 *너머*를
  테스트하고 싶다면 모듈 모양이 잘못됐을 가능성이 크다. 먼저 호출자가 관측해야 할 불변식
  (실패, 부수효과, 보안상 순서)과 교체 가능한 구현 세부(정확한 argv 배열, 명령 순서)를 가른다 —
  전자를 고정하는 테스트는 인터페이스 테스트고 후자를 고정하면 너머를 보는 것이다.
- **adapter 하나면 가설적 seam, 둘이면 진짜 seam.** 실제로 그 seam을 가로질러 변하는 것이
  없으면 seam(포트)을 만들지 않는다. adapter 하나짜리 seam은 그냥 간접화다. adapter는 운영과
  테스트를 나눠 *종류*로 센다; 테스트 adapter가 실제 fault(stale, tamper, 누락)를 나타내면 그
  seam은 internal seam으로 인정한다. 같은 역할의 람다 여럿은 하나다.

## Testability를 위한 설계

1. **의존성은 받는다, 만들지 않는다.** `processOrder(order, paymentGateway)`는 테스트 가능,
   함수 안에서 `new StripeGateway()`는 어렵다.
2. **결과를 반환한다, 부수효과를 내지 않는다.** `calculateDiscount(cart) Discount`는 테스트
   가능, `applyDiscount(cart)`가 `cart.total`을 고치면 어렵다.
3. **표면적을 작게.** 메서드가 적으면 테스트가 적고, 파라미터가 적으면 setup이 단순하다.

## Deepening: 의존성 범주별 테스트 전략

얕은 모듈 묶음을 깊게 합칠 때 의존성을 분류하고, 범주가 seam 너머 테스트 방식을 정한다.
한 모듈에 여러 범주가 섞이면(로컬 CLI가 원격 모델을 부르는 식) 모듈 전체가 아니라 **의존성마다**
분류하고 층을 나눈다.

| 범주 | 예 | 전략 |
|---|---|---|
| In-process | 순수 계산, 메모리 상태 | 합치고 새 인터페이스로 직접 테스트. adapter 불필요 |
| Local-substitutable | Postgres→PGLite, 메모리 FS | 대체물을 테스트 스위트에서 띄운다. seam은 internal |
| Remote but owned | 내부 서비스, 내부 API | seam에 port를 정의, 운영은 HTTP/gRPC adapter, 테스트는 in-memory adapter |
| True external | Stripe, Twilio | port로 주입, 테스트는 mock adapter |

**교체하되 겹치지 않는다**: 깊어진 모듈의 인터페이스 테스트가 생기면 얕은 모듈의 옛 단위
테스트는 낭비이므로 지운다. 구현이 바뀔 때 고쳐야 하는 테스트는 인터페이스 너머를 테스트하고
있는 것이다.

## Design it twice

첫 아이디어가 최선일 가능성은 낮다. 후보 인터페이스를 여러 개 놓고 비교한다:

1. 문제 공간을 먼저 사용자에게 설명한다 — 제약, 의존성과 그 범주, 제약을 구체화하는 스케치
   (제안이 아니다).
2. 서브에이전트 3개 이상을 **병렬**로 띄워 각각 *급진적으로 다른* 인터페이스를 만들게 한다.
   위임할 수 없는 환경(읽기 전용 worker, task 도구 없음)이면 혼자서 제약 축을 바꿔 가며 독립된
   설계 2개 이상을 순서대로 쓴다 — 첫 설계를 본 채로 두 번째를 쓰되, 첫 설계의 형태를 재사용하지
   않는 것이 규칙이다.
   Agent 1 "진입점 1~3개로 최소화, 진입점당 leverage 최대", Agent 2 "유연성·확장 최대",
   Agent 3 "가장 흔한 호출자에 최적화, 기본 케이스를 사소하게", 필요하면 Agent 4 "ports &
   adapters". 브리프에는 이 스킬의 어휘와 `CONTEXT.md`의 도메인 어휘를 함께 넣는다.
3. 각 설계를 순서대로 보여 준 뒤 **depth**, **locality**, **seam 위치**로 비교하고, 의견을 가진
   추천(필요하면 하이브리드)을 낸다. 메뉴가 아니라 강한 읽기를 준다.

## 거부한 프레이밍

- 구현 줄 수 / 인터페이스 줄 수 비율로서의 depth(Ousterhout): 구현 부풀리기를 보상한다.
  leverage로서의 depth를 쓴다.
- TypeScript `interface` 키워드나 public method로서의 "interface": 너무 좁다.
- "boundary": bounded context와 겹친다. **seam** 또는 **interface**라고 말한다.

관련 스킬: `refactoring`(동작 보존 변환), `entropy-scan`(hotspot 탐지), `tdd`(seam에서 테스트).

Adapted from mattpocock/skills `codebase-design` (MIT).
