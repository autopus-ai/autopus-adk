<div align="center">

# 🐙 Autopus-ADK

### 에이전트*로 이루어진*, 에이전트*에 의해 운영되는*, 에이전트*를 위한* 하네스.

AI 코딩 도구(Claude Code, Codex, Antigravity CLI, OpenCode, Oh My Pi)가 진짜 개발팀처럼 일하게 만듭니다 — 기획, 테스트, 코드 리뷰, 보안 감사까지 자동으로.

**16개 에이전트. 53개 스킬 라이브러리와 작은 기본 목록. 여러 플랫폼을 하나의 설정으로 관리합니다.**

[![GitHub Stars](https://img.shields.io/github/stars/autopus-ai/autopus-adk?style=social)](https://github.com/autopus-ai/autopus-adk/stargazers)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)](https://golang.org)
[![Platforms](https://img.shields.io/badge/Platforms-5-orange)](#-하나의-설정-다섯-개-플랫폼)
[![Agents](https://img.shields.io/badge/Agents-16-blueviolet)](#-16개-전문-에이전트)
[![Skills](https://img.shields.io/badge/Skills-40-ff69b4)](#-전체-명령어)

**AI 코딩 도구의 채팅창에 아래 명령을 붙여넣으세요 — 에이전트가 실행해서 설치부터 설정까지 알아서 합니다. 터미널에서 직접 실행해도 됩니다.**

```bash
# macOS / Linux
curl -sSfL https://raw.githubusercontent.com/autopus-ai/autopus-adk/main/install.sh | sh

# Windows (CMD or PowerShell)
powershell -c "irm https://raw.githubusercontent.com/autopus-ai/autopus-adk/main/install.ps1 | iex"
```

[왜 Autopus인가](#-문제점) · [**핵심 워크플로우**](#-워크플로우) · [주요 기능](#-autopus가-다른-이유) · [파이프라인](#-파이프라인) · [보안](#-보안) · [명령어](#-전체-명령어)

[🇺🇸 English](../README.md)

</div>

---

## 🎬 실제 동작

<p align="center"><img src="../demo/hero.gif" width="720" alt="Autopus-ADK 데모 — version, doctor, platform, status, skills" /></p>

```bash
# 3개 AI 모델이 서로 토론하며 브레인스토밍
/auto idea "Google과 GitHub 프로바이더로 OAuth2 인증 추가" --multi --ultrathink

# 한 명령으로 나머지 전부 — 기획, 16개 에이전트 구현, 문서화까지
/auto dev "Google과 GitHub 프로바이더로 OAuth2 인증 추가"
```

단계별 제어를 원한다면:

```bash
/auto plan "Google과 GitHub 프로바이더로 OAuth2 인증 추가" --auto --multi --ultrathink
/auto go SPEC-AUTH-001 --auto --loop --team
/auto sync SPEC-AUTH-001
```

```
🐙 Pipeline ─────────────────────────────────────────────
  ✓ Phase 1:   Planning         planner가 5개 태스크 분해
  ✓ Phase 1.5: Test Scaffold    12개 실패 테스트 생성 (RED)
  ✓ Phase 2:   Implementation   3개 executor가 병렬 워크트리에서 구현
  ✓ Phase 3:   Testing          커버리지: 62% → 91%
  ✓ Phase 4:   Review           TRUST 5: APPROVE | 보안: PASS
  ───────────────────────────────────────────────────────
  ✅ 5/5 태스크 │ 91% 커버리지 │ 보안 이슈 0건 │ 4분 32초
```

> 💡 명령 하나로. 테스트, 보안 감사, 문서, 의사결정 이력이 포함된 프로덕션 수준의 코드.

---

## 😤 문제점

AI 코딩 도구를 사용하고 계시죠. 강력합니다. 하지만...

- 🔄 **플랫폼 종속** — Claude에서 Codex로 바꾸려면? 모든 규칙과 프롬프트를 처음부터 다시 작성.
- 🎲 **희망 주도 개발** — "인증 추가해줘" → AI가 코드를 쓰고, 테스트를 건너뛰고, 보안을 무시하고, 문서를 잊음. *아마* 동작할 수도.
- 🧠 **건망증** — 다음 세션에서 AI는 모든 결정을 잊음. "왜 이 패턴을 썼지?" → 침묵.
- 👤 **솔로 에이전트** — 하나의 모델, 하나의 컨텍스트, 한 번의 기회. 다중 파일 리팩토링? 행운을 빕니다.

---

## 🧠 철학: AX — Agent Experience

> **AX**는 "AI Transformation"이 아닙니다. AX는 **Agent Experience** — AI 에이전트가 코드베이스를 인식하고, 탐색하고, 작업하는 방식입니다. UX가 사용자를 위해 설계하고 DX가 개발자를 위해 설계하듯, **AX는 에이전트를 위해 설계합니다.**

```mermaid
flowchart LR
    UX["🧑 UX\nUser Experience"]
    DX["👩‍💻 DX\nDeveloper Experience"]
    AX["🤖 AX\nAgent Experience"]

    UX -->|"설계 대상"| U["사용자"]
    DX -->|"설계 대상"| D["개발자"]
    AX -->|"설계 대상"| A["AI 에이전트"]

    style AX fill:#ff6b6b,stroke:#c92a2a,color:#fff
```

대부분의 AI 코딩 도구는 단순한 모델에 기반합니다: **당신이 지시하면, AI가 응답한다.**

Autopus는 다른 질문에서 시작합니다: *프로젝트 문서의 1차 독자가 에이전트라면?*

신입 엔지니어 온보딩을 떠올려 보세요. 빈 에디터를 주고 "인증 시스템 만들어"라고 하지 않죠. 이런 것들을 줍니다:
- 시스템을 이해할 **아키텍처 문서**
- 코드가 기존과 어울리도록 **코딩 컨벤션**
- 같은 실수를 반복하지 않도록 **의사결정 이력**
- 실수가 배포 전에 잡히도록 **리뷰 프로세스**

**AI 에이전트에게도 같은 것이 필요합니다.** 차이점은 매 세션이 첫 출근이라는 것.

Autopus는 **하네스**입니다 — 에이전트가 시니어 엔지니어가 승인할 코드를 생산하기 위해 필요한 맥락, 제약, 워크플로우를 제공하는 구조화된 환경. 희망이 아닌 설계로.

### 에이전트로. 에이전트에 의해. 에이전트를 위해.

```mermaid
flowchart TB
    subgraph OF ["🧬 에이전트로 이루어진 (Of)"]
        direction TB
        O1["16개 특화 에이전트가\n소프트웨어 팀을 구성"]
        O2["Planner · Executor · Tester\nReviewer · Architect · ..."]
    end

    subgraph BY ["⚡ 에이전트에 의해 운영되는 (By)"]
        direction TB
        B1["에이전트가 파이프라인을\n자율적으로 운영"]
        B2["셀프힐링 게이트\n병렬 워크트리\n멀티모델 토론"]
    end

    subgraph FOR ["🎯 에이전트를 위해 설계된 (For)"]
        direction TB
        F1["모든 파일, 규칙, 문서가\n에이전트가 파싱하도록 설계"]
        F2["프로젝트별 한도 · 선택적 @AX\n구조화된 Lore · SPEC 형식"]
    end

    OF --> BY --> FOR

    style OF fill:#4c6ef5,stroke:#364fc7,color:#fff
    style BY fill:#7950f2,stroke:#5f3dc4,color:#fff
    style FOR fill:#f06595,stroke:#c2255c,color:#fff
```

| 원칙 | 의미 |
|------|------|
| **에이전트로 이루어진 (Of)** | 16개 특화 에이전트가 실제 엔지니어링 팀을 구성합니다 — 기획자, 구현자, 테스터, 리뷰어, 보안 감사관 등. 하나의 챗봇이 아닌 팀. |
| **에이전트에 의해 운영 (By)** | 에이전트가 파이프라인을 자율적으로 운영합니다 — 셀프힐링 품질 게이트, 병렬 워크트리, 멀티모델 토론. 사람은 목표를 설정하고, 에이전트가 실행합니다. |
| **에이전트를 위해 설계 (For)** | 모든 파일, 규칙, 문서가 에이전트가 파싱하도록 설계됩니다. 산문보다 구조. 그것이 AX입니다. |
| **매 세션이 첫 출근** | 에이전트는 세션 간 모든 맥락을 잃습니다. 하네스가 아키텍처, 의사결정, 컨벤션이라는 조직 기억을 제공합니다. |

> 🐙 **Autopus는 에이전트를 더 똑똑하게 만들지 않습니다. 더 잘 알게 만듭니다. 그것이 AX입니다.**

---

## 🔥 Autopus가 다른 이유

### 📏 에이전트가 읽을 수 있는 코드

파일 길이만으로 응집도나 정확성을 판단할 수는 없습니다. Autopus는 큰 소스 파일을 알리되, 프로젝트가 `architecture.max_file_lines`에 양수를 지정했을 때만 한도를 강제합니다. 생략하거나 `0`이면 권고만 표시합니다. 한 역할을 맡은 코드는 함께 두고, 실제 책임이 나뉠 때 분리하세요. 저장소 CI가 별도로 정한 한도는 계속 적용됩니다.

```
❌ 기존 방식:
   service.go (1,200줄) → 에이전트가 중간에서 맥락을 잃음

✅ Autopus 방식:
   service.go       (180줄)  핸들러 로직
   service_auth.go  (120줄)  인증 미들웨어
   service_repo.go  (150줄)  데이터 접근
   → 모든 파일이 하나의 컨텍스트 윈도우에 들어갑니다. 모든 파일이 하나의 역할을 합니다.
```

이것은 단순히 파일 크기만의 문제가 아닙니다. 전체 하네스가 **설계 단계부터 에이전트가 읽기 쉽게** 만들어져 있습니다:

| 레이어 | 에이전트 친화적 설계 |
|--------|---------------------|
| **규칙** | IMPORTANT 마커가 포함된 구조화된 마크다운 — 에이전트가 파싱합니다 |
| **스킬** | 트리거가 있는 YAML 프론트매터 — 에이전트가 적절한 스킬을 자동 활성화 |
| **문서** | 문단 대신 표, 산문 대신 체크리스트 — 읽히는 게 아니라 파싱됩니다 |
| **코드** | 명확한 책임과 프로젝트가 정한 한도 — 모든 파일에 같은 줄 수를 강제하지 않습니다 |

> 🐙 **사람이 읽기 좋은 것은 보너스입니다. 에이전트가 읽을 수 있는 것이 요구사항입니다.**

### 🤖 챗봇이 아닌, 팀을 구성하는 AI 에이전트

Autopus는 하나의 AI 어시스턴트가 아닌 — 역할 정의, 품질 게이트, 재시도 로직을 갖춘 **16개 전문 에이전트 소프트웨어 엔지니어링 팀**을 제공합니다.

```
🧠 Planner        →  요구사항을 태스크로 분해
⚡ Executor ×N    →  병렬 워크트리에서 코드 구현
🧪 Tester         →  코드 작성 전에 테스트 먼저 (TDD 강제)
✅ Validator       →  빌드, 린트, vet 검사
🔍 Reviewer       →  TRUST 5 코드 리뷰
🛡️ Security       →  OWASP Top 10 보안 감사
📝 Annotator      →  @AX 태그로 코드 문서화
🏗️ Architect      →  시스템 설계 결정
🔬 Deep Worker    →  장시간 자율 탐색 + 구현
... 외 7개
```

### ⚔️ AI 모델들이 서로 토론한다 (`--multi`)

하나의 모델에는 맹점이 있습니다. **세 모델이 서로의 실수를 잡습니다.**

모든 AI 모델에는 고유한 강점과 편향이 있습니다 — Claude는 꼼꼼하지만 장황하고, Codex는 빠르지만 때로 얕고, Gemini는 완전히 다른 시각을 가져옵니다. `--multi`를 사용하면 단순히 병렬 실행되는 게 아니라 — **서로의 아이디어를 리뷰하고, 도전하고, 발전시킵니다.**

```bash
# 어떤 명령에든 --multi를 추가하면 멀티 모델 지능이 활성화
/auto idea "새 기능" --multi          # 3개 모델 브레인스토밍 → 교차 수분 → ICE 점수
/auto plan "새 기능" --multi          # 3개 모델의 읽기 전용 기획 검토 → 단일 SPEC 작성 → 독립 리뷰
/auto go SPEC-ID --multi              # 3개 모델이 코드 리뷰 토론
```

```mermaid
flowchart TB
    C["🔍 Claude\n독립 분석"] --> D["⚔️ 교차 수분\n각 모델이 다른 모델의 아이디어를 봄"]
    X["🔍 Codex\n독립 분석"] --> D
    G["🔍 Gemini\n독립 분석"] --> D
    D --> R["🔄 라운드 2\n수용 · 통합 · 리스크"]
    R --> J["🏛️ 블라인드 판사\n익명화된 채점"]
```

**왜 중요한가:**
- Claude가 놓치는 버그를 Codex가 잡고, Codex가 무시하는 엣지 케이스를 Gemini가 지적합니다.
- 하나의 모델이라면 절대 생각하지 못할 아이디어가 교차 수분에서 나옵니다.
- 블라인드 판사가 익명화된 결과를 채점합니다 — 모델 편향 없음.
- 연구에 따르면 멀티 에이전트 토론이 단일 모델보다 높은 품질의 결과를 냅니다.

> **`/auto dev`는 `--multi`를 기본 활성화합니다.** 모든 기획이 멀티 모델 리뷰를 거칩니다. 모든 코드 리뷰가 교차 검증됩니다. 신경 쓸 필요 없습니다.

4가지 전략: **Consensus** (합의 병합) · **Debate** (적대적 리뷰 + 판사) · **Pipeline** (출력 체이닝) · **Fastest** (최초 완료 우선)

### 🔁 자가 치유 파이프라인 (RALF 루프)

품질 게이트는 실패만 하지 않습니다 — **스스로 고치고 재시도합니다.**

```bash
/auto go SPEC-AUTH-001 --auto --loop
```

```
🐙 RALF [Gate 2] ──────────────────
  Iteration: 1/5 │ Issues: 3
  → golangci-lint 경고 수정을 위해 executor 스폰 중...

🐙 RALF [Gate 2] ──────────────────
  Iteration: 2/5 │ Issues: 3 → 0
  Status: PASS ✅
```

**RALF = RED → GREEN → REFACTOR → LOOP** — TDD 원칙을 파이프라인 자체에 적용. 내장 서킷 브레이커가 무한 루프를 방지합니다.

### 🌳 격리된 워크트리에서 병렬 에이전트 실행

여러 executor가 **동시에** 작업합니다 — 각각 자체 git 워크트리에서. 충돌 없음. 손상 없음.

```
Phase 2: Implementation
  ├── ⚡ Executor 1 (worktree/T1) → pkg/auth/provider.go     ✓
  ├── ⚡ Executor 2 (worktree/T2) → pkg/auth/handler.go      ✓
  └── ⚡ Executor 3 (worktree/T3) → pkg/auth/middleware.go    ✓

Phase 2.1: Merge (태스크 ID 순서)
  ✓ T1 병합 → T2 병합 → T3 병합 → 작업 브랜치
```

파일 소유권으로 충돌 방지. GC 억제로 손상 방지. 최대 **5개 동시 워크트리.**

### 📜 Lore: 코드베이스는 절대 잊지 않는다

모든 커밋이 what이 아닌 **why**를 기록합니다. 영원히 조회 가능.

```
feat(auth): OAuth2 프로바이더 추상화 추가

Why: Google + GitHub 지원이 필요하고, 향후 프로바이더 확장 가능해야 함
Decision: 직접 SDK 사용 대신 인터페이스 기반 추상화
Alternatives: 직접 SDK 호출 (거부: 결합도 높음)
Ref: SPEC-AUTH-001

🐙 Autopus <noreply@autopus.co>
```

9개 구조화된 트레일러. `auto lore query "왜 인터페이스?"`로 조회. 90일 지난 결정은 자동 감지.

### 🧪 자율 실험 루프

AI가 자율적으로 반복합니다 — 측정하고, 유지 또는 폐기하고, 반복합니다.

```bash
/auto experiment --metric "go test -bench=BenchmarkProcess" --direction lower --max-iter 5
```

```
🐙 Experiment ───────────────────────
  Iter 1: baseline  │ 1200 ns/op
  Iter 2: optimize  │  850 ns/op  ✓ keep (29% improvement)
  Iter 3: refactor  │  900 ns/op  ✗ discard (regression)
  Iter 4: cache     │  620 ns/op  ✓ keep (27% improvement)
  ─────────────────────────────────────
  Result: 1200 → 620 ns/op (48% improvement)
```

내장 **서킷 브레이커**로 무한 반복을 방지합니다. **단순성 점수**가 과도하게 복잡한 솔루션에 패널티를 부여합니다. 각 반복은 git 커밋으로 기록되어 리뷰 및 롤백이 용이합니다.

> **상태: 실험적** — CLI 명령어(`auto experiment`)는 사용 가능하지만 스킬 레벨 통합은 진행 중입니다. 핵심 반복 루프는 작동하며, 전체 파이프라인 통합은 준비 중입니다.

### 🧠 실패에서 배우는 파이프라인

Autopus 파이프라인은 단순히 실패하지 않습니다 — **왜 실패했는지 기억하고** 다음에 같은 실수를 방지합니다.

```
Gate 2 FAIL: golangci-lint — pkg/auth/에서 미사용 변수
→ .autopus/learnings/pipeline.jsonl에 자동 기록
→ 다음 /auto go: 학습 내용이 executor 프롬프트에 주입
→ 같은 실수 반복 없음
```

모든 파이프라인 실패가 구조화된 학습 항목으로 기록됩니다. 다음 실행 시 관련 학습이 자동으로 에이전트 프롬프트에 주입되어 — 세션을 넘어선 **조직 기억**을 파이프라인에 부여합니다.

### 🏥 배포 후 헬스 체크

먼저 배포하고, 즉시 검증합니다. `canary`는 빌드 검증, E2E 테스트, 브라우저 헬스 체크를 라이브 배포에 대해 실행합니다.

```bash
/auto canary                          # 빌드 + E2E + 브라우저 자동 검증
/auto canary --url https://myapp.com  # 특정 배포 URL 대상
/auto canary --watch 5m               # 5분마다 반복
/auto canary --compare                # 이전 카나리 리포트와 비교
```

빌드 상태, 테스트 결과, 접근성 점수, 스크린샷 diff가 포함된 `canary.md` 진단 보고서를 생성합니다.

### 🔀 스마트 모델 라우팅

모든 태스크에 Opus가 필요하지 않습니다. Autopus가 메시지 복잡도를 분석하고 적절한 모델로 자동 라우팅합니다.

```
단순 질의     → Haiku  (빠르고 저렴)
코드 리뷰     → Sonnet (균형)
아키텍처      → Opus   (깊은 추론)
```

설정 불필요 — 라우터가 토큰 수, 코드 복잡도, 도메인 신호를 평가하여 최적 모델을 선택합니다. `--quality ultra`로 언제든 오버라이드 가능.

### 🔌 프로바이더 연결 마법사

AI 프로바이더 설정에 문서를 읽을 필요 없습니다. `auto connect`가 3단계 가이드 설정을 안내합니다.

```bash
auto connect         # 대화형 마법사: server auth → workspace → OpenAI OAuth
auto connect status  # 저장된 연결 상태를 로컬에서 결정적으로 검증
```

현재 릴리스는 Autopus 서버 인증, 워크스페이스 선택, OpenAI OAuth handoff까지를 수행합니다. 저장된 로컬 상태 검증은 `auto connect status` 또는 `auto desktop status --json`으로 확인합니다.

Desktop runtime ownership 메모:
- packaged `autopus-desktop-runtime` 의 source/build/release provenance 는 이제 `autopus-desktop/runtime-helper/` 가 source of truth 입니다.
- ADK의 `auto connect`, `auto desktop ...`, `auto worker ...` 는 harness 또는 compatibility surface 로 유지되지만, 정상적인 desktop runtime shipping 은 더 이상 `autopus-adk` checkout 에 의존하지 않습니다.

### 🤖 ADK Worker — 로컬 에이전트 실행

Autopus CLI와 Autopus 플랫폼 간의 브릿지입니다. ADK Worker는 A2A + MCP 하이브리드 태스크를 로컬에서 실행하여, 클라우드 의존 없이 플랫폼급 오케스트레이션을 가능하게 합니다.

어떤 기능인가요?
- 로컬 워크스페이스를 Autopus 플랫폼의 worker loop에 연결합니다
- 플랫폼에서 내려온 태스크를 로컬 도구로 실행합니다
- 메인 하네스와 같은 보안, 예산, 감사 레일을 재사용합니다

지금은 어떻게 보면 되나요?
- `auto init`, Codex `@auto ...`, OpenCode `/auto ...`를 쓰려는 용도라면 Worker는 일단 무시해도 됩니다
- `auto worker ...`는 선택형 고급 기능이며, 현재는 점진적으로 정리 및 문서화 중입니다

### 💰 반복 예산 관리

Worker가 영원히 실행되지 않습니다. 각 executor에 도구 호출 예산이 할당되어 — 복잡한 태스크를 완료할 충분한 여유를 보장하면서 폭주 에이전트를 방지합니다.

### 📦 컨텍스트 압축

파이프라인이 단계를 진행하면서 이전 컨텍스트는 고정 스키마로 자동 압축됩니다: Goal, Constraints, Progress, Decisions, Relevant Files, Next Steps, Critical Context. 도구 호출과 결과는 쌍으로 보존/정리되고, 안전하지 않은 provider payload 본문은 생략되며, 적용된 압축은 summary id, source refs, reason codes, budget/blocker 상태가 포함된 메타데이터 이벤트로 남습니다.

### 🔄 죽지 않는 파이프라인

파이프라인 중간에 크래시? 마지막 체크포인트에서 정확히 재개합니다.

```bash
/auto go SPEC-AUTH-001 --continue    # 마지막 체크포인트에서 재개
```

YAML 기반 체크포인트가 매 단계 후 파이프라인 상태를 저장합니다. Stale 감지가 오래된 세션 재개를 방지합니다. `--auto --loop`과 결합하면 **완전 복원력 있는 자율 파이프라인**을 얻습니다.

### 🧪 코드에서 E2E 시나리오 자동 생성

E2E 테스트 시나리오를 자동 생성하고 실행합니다 — 수동 테스트 작성 불필요.

```bash
auto test run                    # 모든 시나리오 실행
auto test run -s init --verbose  # 특정 시나리오 실행
```

Autopus가 코드베이스(Cobra 커맨드, API 라우트, 프론트엔드 페이지)를 분석하여 **검증 프리미티브** (`exit_code`, `stdout_contains`, `status_code`, `json_path` 등)가 포함된 타입 시나리오를 생성합니다. 증분 동기화로 코드 변경에 따라 시나리오가 자동 갱신됩니다.

### 🔧 더 많은 도구

| 기능 | 명령어 | 설명 |
|------|--------|------|
| **Reaction Engine** | `auto react check/apply` | CI 실패 감지, 로그 분석, 수정 보고서 자동 생성 |
| **Meta-Agent Builder** | `auto agent create` / `auto skill create` | 패턴 기반 커스텀 에이전트/스킬 스캐폴드 생성 |
| **Hard Gate** | `auto check --gate` | 파이프라인 게이트 강제 (mandatory/advisory 모드) |
| **Self-Update** | `auto update --self` | 검증된 바이너리 업데이트 — Darwin/Linux 원자 교환, Windows `.old` 복구 |
| **비용 추적** | `auto telemetry cost` | 모델별 토큰 기반 파이프라인 비용 추정 |
| **이슈 리포터** | `auto issue report` | 에러 컨텍스트 자동 수집, 시크릿 정제, GitHub 이슈 생성 |
| **시그니처 맵** | `auto setup` | AST 분석으로 exported API 시그니처 추출 (Go + TypeScript) |
| **테스트 러너 감지** | `auto init` | jest, vitest, pytest, cargo 테스트 프레임워크 자동 감지 |

### 🌐 하나의 설정, 다섯 개 플랫폼

```bash
auto init   # 지원되는 설치된 AI 코딩 CLI 자동 감지
```

하나의 `autopus.yaml`이 감지된 지원 플랫폼에 **네이티브 설정**을 생성합니다.

| 플랫폼 | 생성되는 파일 |
|--------|-------------|
| **Claude Code** | `.claude/rules/`, `.claude/skills/<name>/SKILL.md`, `.claude/agents/`, `.claude/workflows/`, `.claude/settings.json`, `CLAUDE.md` |
| **Codex** | `.codex/skills/codex-<name>/SKILL.md`, `.codex/agents/`, `.codex/hooks.json`, `.codex/config.toml`, `.agents/plugins/marketplace.json`, `.autopus/plugins/auto/`, `AGENTS.md` |
| **Antigravity CLI** | `.agents/plugins/autopus/`, `.agents/hooks.json`, 기존 호환성을 위한 `.gemini/` / `GEMINI.md` |
| **OpenCode** | `.opencode/rules/`, `.opencode/agents/`, `.opencode/commands/`, `.opencode/plugins/`, `.agents/skills/`, `AGENTS.md`, `opencode.json` |
| **Oh My Pi (OMP)** | `.omp/rules/autopus-*.md`, `.omp/skills/<name>/SKILL.md`, `.omp/commands/`, 선택형 `.omp/extensions/` |

Antigravity는 워크스페이스 플러그인을 직접 발견합니다. `auto init`과 `auto update`는
프로젝트별 내용을 전역 플러그인으로 설치하지 않으며, 사용하지 않는 `.agents/commands/`
복제본도 생성하지 않습니다. `agy plugin validate .agents/plugins/autopus`로 생성된
구조를 확인할 수 있습니다. 이 결과는 권한이나 훅이 실제 실행됐다는 증거는 아니며,
네이티브 사용자 권한 설정은 보존합니다.

2026-09-13 호환성 점검에는 Claude Code 2.1.263, Codex CLI 0.153.4, Antigravity CLI 1.1.26, OpenCode 1.18.7, OMP 18.1.19를 사용했습니다. 이는 관측한 점검 버전이며, 이후 릴리스의 모든 기능을 검증했다는 뜻은 아닙니다. [Codex 0.154.0의 worktree 지원](https://github.com/openai/codex/releases/tag/rust-v0.154.0)은 아직 실험적 기능입니다. 하네스는 실험적 토폴로지를 자동 활성화하거나 사용자 권한 설정을 대체하지 않습니다.

Codex 참고:
- `auto init` 또는 `auto update` 직후 `$codex-auto-plan ...`, `$codex-auto-go ...` 등 `$codex-auto-<route>` 스킬을 바로 사용할 수 있습니다
- `.agents/plugins/marketplace.json`에 등록된 로컬 플러그인(`.autopus/plugins/auto`)을 설치하면 `@auto ...` 문법도 사용할 수 있습니다
- 플러그인은 `@auto ...` 라우터만 제공합니다. 상세 워크플로는 `.codex/skills/codex-<name>/SKILL.md`에 고유한 네이티브 스킬로 생성하며, 저장소 안에 `.codex/prompts/`나 Markdown `.codex/rules/`를 만들지 않습니다
- 멀티에이전트 모드는 `[features.multi_agent_v2]`와 현재의 협업 도구 6개를 사용합니다. 모든 worker가 같은 cwd와 파일시스템을 공유하며, 기존 `send_input`, `resume_agent`, `close_agent`는 사용하지 않습니다
- 요청할 작업 에이전트 수는 `autopus.yaml`의 `codex.agents.max_concurrent_threads`로 설정합니다(기본 4, 범위 1–64). 주 에이전트는 포함하지 않습니다. `auto update`가 이 값을 `.codex/config.toml`에 반영하며, 명시한 값은 재생성 후에도 유지됩니다. CLI 호환성을 확인하지 못한 버전에는 문서화된 `[agents] max_concurrent_threads_per_session`을 사용하고 그 가정을 표시합니다. 실제 실행에서는 호스트와 계정의 제한이 우선합니다.
- `auto doctor`는 요청한 수, 디스크에서 확인한 설정, 실행 중인 세션이 읽은 값, 실제 적용 한도를 구분합니다. 설정 파일을 읽었다고 활성 세션이 그 값을 적용했다고 간주하지 않으며, 관측할 수 없는 세션 값은 이유와 함께 `unknown`으로 표시합니다. 설정을 바꾼 뒤에는 새 세션을 시작해야 합니다. 활성 에이전트를 자동으로 중단하지 않으며, 이 값으로 로컬 빌드·테스트 동시 실행 수를 제어하지 않습니다.
- `.codex/hooks.json`은 기본으로 생성하고, `.codex/config.toml`은 관련 없는 사용자 설정을 보존하도록 구조적으로 병합합니다
- `features.multi_agent`는 현재도 유효한 네이티브 설정입니다. 명시한 `true`와 `false`, 사용자 주석을 재생성과 정리 후에도 보존합니다. 현재 기본값과 같다는 이유만으로 하네스가 소유한 값으로 간주하지 않습니다.

OpenCode 참고:
- `/auto ...`와 `/auto-plan ...` 같은 직접 alias가 `.opencode/commands/`에 생성됩니다
- 네이티브 규칙/에이전트/플러그인은 `.opencode/` 아래에, 재사용 스킬은 `.agents/skills/` 아래에 생성됩니다
- 기본 `skills.compiler.mode: split`은 핵심 스킬과 필수 참조를 배포합니다. 추가로 선택한 스킬 묶음은 `.opencode/skills/`에 놓으며, 워크플로 전용 규칙은 파일로 유지하되 매 세션 시작 시 읽지 않습니다.
- `/auto status`, `/auto map`, `/auto why`, `/auto verify`, `/auto secure`, `/auto test`, `/auto dev`, `/auto doctor` 같은 helper workflow도 OpenCode 명령 래퍼로 함께 생성됩니다
- `opencode.json`이 관리형 hook plugin을 자동 등록하므로 `auto init` 또는 `auto update` 직후 `.opencode/plugins/autopus-hooks.js`가 바로 활성화됩니다

Oh My Pi 참고:
- 규칙은 `.omp/rules/autopus-<name>.md` 평면 구조로 생성됩니다. OMP는 각 규칙 루트를 비재귀로만 탐색하므로 `rules/autopus/` 같은 하위 디렉터리는 세션에 도달하지 않습니다. 네임스페이스는 디렉터리가 아니라 `autopus-` 파일명 접두사입니다
- ADK 소유는 manifest에 기록된 `autopus-` 접두사 파일뿐입니다. 같은 디렉터리에 둔 사용자 규칙(`.omp/rules/mine.md`)은 `auto update`와 `auto platform remove omp`가 건드리지 않습니다
- `auto init`은 `.omp/rules/`를 디렉터리 패턴으로 gitignore에 넣습니다. `.omp/rules/autopus-*.md` 같은 파일명 글롭을 쓰면 생성된 규칙이 OMP 발견에서 전부 사라지기 때문에 디렉터리 형태를 씁니다. 무시된 상태에서도 자기 규칙을 추적하려면 `git add -f .omp/rules/mine.md`를 쓰거나(무시된 디렉터리 안에서는 gitignore negation이 동작하지 않습니다) 규칙을 `.omp/rules/` 밖에 두세요
- OMP는 우선순위 100인 네이티브 `.omp/skills/<name>/SKILL.md`와 `.omp/commands/*.md`를 사용합니다. `.agents/skills`를 사용자 지정 디렉터리로 다시 등록하지 않으며, 프로젝트 관리 설정이 필요하지 않으면 기본 `.omp/config.yml`도 만들지 않습니다
- `auto doctor`는 버전·도움말·설정 메타데이터와 provider-free RPC handshake로 OMP 18.0.5를 검증합니다. handshake는 로컬에서 bootstrap 모델만 선택하고 prompt나 provider 요청은 보내지 않습니다

### OMP 역할 라우팅과 컨텍스트 최적화(선택 적용)

평소 모델 설정에는 이 명령 하나만 사용하세요.

```bash
auto quality
```

`auto quality`는 먼저 무엇을 설정할지 묻습니다. 공용 품질 모드 또는 한 코딩 도구의 에이전트 모델입니다. 생성 에이전트가 모델을 갖는 도구만 목록에 올립니다 — Claude Code, Codex, OMP. Antigravity CLI와 OpenCode는 세션 모델을 상속하므로 조용히 빼는 대신 그렇게 표시합니다.

**Claude Code / Codex**: 에이전트별 상대 티어와 그 티어가 해당 도구에서 어떤 구체 모델이 되는지 함께 보여 주고, `agent=tier`(`fable`, `opus`, `sonnet`, `haiku`) 형식으로 수정합니다. 미리보기를 확인하고 `y`를 입력하면 `quality.presets.<name>`으로 저장하고 `quality.providers.<tool>`로 연결합니다. 도구별로 preset이 따로 있으므로 Claude Code의 executor를 `fable`로 올려도 Codex는 그대로입니다. `quality.default`는 바뀌지 않습니다. 생성 파일에 반영하려면 `auto update`(또는 `auto quality --apply`)를 실행하세요.

**OMP**: **balanced/ultra → GPT/Claude**를 고르거나, **custom**으로 기본 에이전트별 설치 모델을 직접 지정합니다. custom도 지정하지 않은 에이전트를 위해 기준 계열을 먼저 고르고, 설치된 카탈로그와 각 모델의 thinking 단계를 보여 주며, 각 질문에 그 에이전트 경로가 요구하는 capability를 함께 표시합니다. 카탈로그가 해당 capability를 선언하지 않은 모델은 그 자리에서 거절하고 가능한 모델을 알려 줍니다. 마지막 역할·모델·추론 강도 표를 확인하고 `y`를 입력하면 적용합니다.

확인 단계에서 Enter, `n`, EOF로 취소하면 아무것도 바꾸지 않습니다. `--apply`도 필요 없습니다.
기존 에이전트별 지정값과 멀티프로바이더 리뷰 설정은 유지합니다.
명시적인 사용자 프로필은 그 프로필의 계열을 그대로 사용합니다.
적용한 뒤에는 새 OMP 세션을 시작하세요.

자동화와 고급 설정에는 기존 명령을 사용할 수 있습니다.

```bash
auto platform omp models                 # 설치된 모델 카탈로그
auto platform omp profile apply balanced --family gpt --plan
auto platform omp profile apply balanced --family gpt
auto platform omp explain
auto status --platform omp
```

설치된 OMP 카탈로그에 family, capability, 인증 가능 여부가 없더라도 `models`는 정확한 native selector와 thinking 지원 정보를 `degraded` 조회 결과로 표시합니다. 자동 프로필 생성과 strict 라우팅은 계속 `catalog_metadata_insufficient`로 차단되며, Autopus는 누락된 값을 추측하거나 credential을 읽지 않습니다. 대신 이미 저장된 명시적 프로필은 `catalog_trust: operator-attested`를 선택할 수 있습니다. 이 모드도 먼저 같은 bounded strict probe를 실행한 뒤, native selector와 운영자 선언의 정확한 교집합만 사용하며 auth/keyless는 관측하지 않은 상태로 유지합니다.

활성화 단계에서는 생성한 overlay를 로드한 OMP RPC `get_state` 세션으로 모든 `@role`을 확인합니다. prompt나 모델 프로바이더 요청은 보내지 않습니다. 확인한 provider/model/thinking map은 receipt에 결속하며 `explain`과 doctor가 독립적으로 다시 검증합니다.

`auto init`은 OMP용 에이전트 정의를 만들지 않습니다. OMP는 자체 기본 에이전트(`task`, `scout`, `reviewer`, `security-reviewer`, `sonic`)를 등록하고 이름으로 정확히 찾으므로, 생성한 `.omp/agents/<name>.md`는 기본 에이전트를 가리기만 합니다. 역할 프로필은 대신 `task.agentModelOverrides`로 모델을 지정하며 16개 ADK 역할은 이 다섯 에이전트로 모입니다. `auto platform omp explain`과 `auto status --platform omp`는 기본 에이전트별로 역할·capability 출처와 실제 selector를 한 행씩 표시하고, `.omp/agents/<기본 이름>.md` 파일이 있으면 `native_agent_shadowed`로 보고합니다.

여러 역할 키가 한 기본 에이전트로 모일 수 있습니다. 이때는 대표 역할이 결정합니다 — `task`는 planner, `scout`는 explorer, `sonic`은 validator를 따릅니다. plan·explain·receipt의 각 행이 적용된 키를 표시하므로 16개 역할을 모두 적은 기존 설정도 그대로 동작하고 어떤 항목이 적용됐는지 확인할 수 있습니다. 대표 역할이 없는 조합은 우선순위를 정할 근거가 없어 `omp_native_agent_conflict`로 거절하며 정리할 항목을 함께 알립니다.

#### OMP balanced: GPT형과 Claude형 선택

`profile apply balanced --family gpt|claude`로 OMP 모드와 계열을 함께 선택합니다.
설정에는 각각 `openai`, `anthropic`으로 저장하며 이 이름으로도 선택할 수 있습니다.
`--plan`은 기본 에이전트의 요청·실제 모델, 추론 강도, 후보 순서, fallback 시도와
차단 사유를 보여 줍니다. 설정을 쓰거나 프로필을 활성화하지 않습니다.
적용할 때는 같은 경로를 설치된 OMP 카탈로그와 provider-free RPC로 검증합니다.

| 기본 에이전트 (대표 역할) | GPT형 balanced | Claude형 balanced |
|---|---|---|
| `task` (planner), `reviewer`, `security-reviewer` (security-auditor) | GPT-6 Astra `max` | Claude Fable 5.1 `max` |
| `scout` (explorer), `sonic` (validator) | GPT-5.6 Luna `max` | Claude Sonnet 5 `high` |

일반 reviewer도 선택한 계열을 따릅니다. 멀티프로바이더 리뷰는 별도의
`orchestra.providers` 정책이며, 이 프로필을 바꿔도 리뷰 모델과 judge는 유지합니다.
최상위 리뷰 설정은 Fable `max`, Astra `max`, 선택한 Gemini 모델의 최고 지원
추론 강도를 사용하세요. Gemini 3.1 Pro는 `max`가 없으므로 `high`를 사용합니다.

Balanced는 낮은 모델로 자동 전환하지 않습니다. 모델이 없거나 추론 강도가 지원되지
않으면 설정을 쓰기 전에 적용을 차단합니다. 미리보기도 에이전트별 사유를 보여 주고
실패 코드로 종료합니다. 명시적인 fallback 체인이 없으면 `retry.modelFallback`은 false입니다.

전체 프로필을 복사하지 않고 특정 에이전트만 바꿀 수 있습니다.

```bash
auto platform omp profile apply balanced --family gpt --agent executor=openai-codex/gpt-6-astra:max --plan
auto platform omp profile apply balanced --family gpt --agent executor=openai-codex/gpt-6-astra:max
auto platform omp profile apply balanced --agent executor=inherit
```

여러 에이전트를 바꾸려면 `--agent`를 반복합니다. 지정값은
`role_model_policy.agents.<name>.candidates`에 저장하며, 프로필이나 계열을 바꿔도
유지합니다. `=inherit`로 지우면 선택한 프로필의 기본 배치로 돌아갑니다.
의미 메타데이터가 없는 native 카탈로그에서는 내장 프로필이나 선택한 사용자 프로필에
해당 selector의 family가 명시돼 있어야 합니다. 선언되지 않은 모델을 임의로 추측하지 않습니다.

이 표준 역할표는 Claude Code와 Codex 네이티브 에이전트도 공유합니다. 다만 공통
`quality.presets.balanced.agents`의 사용자 티어 지정은 OMP에 적용하지 않습니다.
OMP는 `role_model_policy.agents`로 재정의하며 Ultra와 사용자 OMP 프로필의 동작은 유지합니다.
`role_model_policy.profiles.balanced`에 명시적 정의가 있으면 내장 프로필보다 우선합니다.
이 경우 `--family`는 조용히 무시하지 않고 오류로 처리합니다.
OMP 프로필 선택은 `quality.default`, 독립 Claude/Codex 설정, 주 세션의 native 모델 역할을
바꾸지 않습니다. `auto quality show`에서 OMP 선택을 별도로 확인할 수 있습니다.

OMP 정책은 특정 프로바이더에 종속되지 않으며 이름 있는 프로필을 선택하기 전에는 활성화되지 않습니다. 기존 설정을 직접 소유하지 않는 `overlay` 모드를 우선 사용하세요. 내장 프로필은 배포된 operator-attested 선언과 설치된 카탈로그의 교집합으로 검증합니다. 사용자 프로필은 의미 메타데이터가 있으면 strict 모드를 사용하고, 없으면 정확한 selector·family·capability·thinking을 검토한 뒤 operator-attested 모드를 선택하세요. 사용자 프로필 예시:

```yaml
role_model_policy:
  version: v1
  profile: omp-balanced
  profiles:
    omp-balanced:
      config_mode: overlay
      catalog_trust: operator-attested
      capabilities:
        deep_reasoning:
          required: true
          candidates:
            - selector: provider-a/reasoner
              family: family-a
              thinking: high
        coding_tool_use:
          required: true
          candidates:
            - selector: provider-a/coder
              family: family-a
              thinking: medium
        fast_validation:
          required: true
          candidates:
            - selector: provider-b/fast
              family: family-b
              thinking: medium
        vision_design:
          required: true
          candidates:
            - selector: provider-b/vision
              family: family-b
              thinking: high
        independent_dissent:
          required: true
          candidates:
            - selector: provider-b/reviewer
              family: family-b
              thinking: high
        deterministic_transform:
          required: true
          candidates:
            - selector: provider-a/transform
              family: family-a
              thinking: medium
      family_diversity:
        enabled: true
        roles: [autopus_reviewer, autopus_security_auditor]
```

- candidate 순서가 fallback 순서입니다. strict 모드에서는 `selector`, `family`, `thinking`이 관측된 semantic catalog metadata와 일치해야 합니다. operator-attested 모드에서는 bounded native catalog에 selector가 있어야 하며, family/capability/thinking은 명시적 프로필에서만 가져옵니다. 이 모드는 인증 성공을 보장하지 않습니다. 위 placeholder는 모델 추천이 아닙니다.
- `safety`는 선택 사항입니다. 설치된 버전의 capability probe가 지원을 확인한 뒤에만 `approval_mode`나 `isolation_mode`를 명시하세요. 생략하면 해당 키를 소유하지 않습니다.
- 프로젝트가 대상 OMP 키를 의도적으로 소유할 때만 `project-managed`를 사용하세요. 소유를 선언한 모든 키에는 관측한 `prior_fingerprint`와 `complete: true`가 필요하고, `retry.fallbackChains` 같은 배열에는 `full_array_ownership: true`도 필요합니다.

컨텍스트 최적화는 별도로 선택 적용합니다. 아래에서 선택된 프로필은 history를 `shadow`, memory를 `off`로 유지하며, active 프로필은 정의만 하고 선택하지 않습니다.

```yaml
omp_context_policy:
  profile: omp-safe-shadow
  profiles:
    omp-safe-shadow:
      history_mode: shadow
      memory_mode: off
      history_target_tokens: 1000
      fallback: canonical_full
      capability_policy: probe_required
      runtime_root_policy: isolated_task_owned
      mutation_scope: session_overlay
    omp-active-probed:
      history_mode: active
      memory_mode: off
      history_target_tokens: 1000
      fallback: canonical_full
      capability_policy: probe_required
      runtime_root_policy: isolated_task_owned
      mutation_scope: session_overlay
```

- 정확한 capability probe가 성공하고 runtime이 task-owned(`isolated_task_owned`)이며, 최소 20개의 완전하고 균형 잡힌 AB/BA raw pair를 재집계한 최신 promotion attestation이 정확한 policy, session/binding, canary digest에 결합된 경우에만 `omp-active-probed`를 선택하세요. attestation이 없거나 오래됐거나 불일치하거나 duplicate/aggregate-only 주장인 경우 `shadow`를 유지하거나 차단합니다. session runtime이 전혀 없을 때만 `no_session`을 사용합니다.
- 관리형 OMP bridge는 현재 `compaction.methodOrder` 계약으로 long-lived supervisor ACK, 정확한 canonical reinjection, 같은 세션의 compaction·rehydration을 지원합니다. 다만 생성만으로 최적화를 활성화하지는 않습니다. `omp-active-probed`는 task-owned runtime receipt가 설치된 lifecycle과 cleanup을 최신 상태로 증명할 때까지 fail-closed를 유지합니다.
- memory는 `off` 또는 관측 전용 `shadow`만 지원합니다. memory shadow에는 `memory_namespace`가 필요합니다. memory를 active로 주입하지 않으며, active receipt에는 memory injection이나 문서 생략을 기록할 수 없습니다.
- 모델 해석 근거는 Git에서 제외되는 runtime artifact `.autopus/omp-model-resolution-v1.json`에 기록됩니다. task-scoped 컨텍스트 근거는 `.autopus/runtime/omp-context/<task-id>/<session-id>/receipt.json`을 사용합니다.
- `auto doctor`(자동화에서는 `auto doctor --json`)로 설치된 CLI와 설정을 다시 probe하세요. receipt에는 credential, prompt 본문, 절대 경로가 들어가면 안 됩니다. 비용이 발생할 수 있는 live provider transport 확인은 `auto doctor --provider-smoke`로 명시적으로 opt-in한 경우에만 실행되며, 오래된 receipt는 현재 canary 통과 증거가 아닙니다.

### Codex vs OpenCode 비교

| 항목 | Codex | OpenCode |
|------|-------|----------|
| 기본 명령 문법 | `@auto <subcommand> ...` | `/auto <subcommand> ...` |
| `auto init` 직후 바로 되는 것 | `$codex-auto-<route> ...` 네이티브 스킬 | `/auto ...`, `/auto-<subcommand> ...` 래퍼 |
| 추가 설치 단계 | `@auto ...`가 필요할 때만 `.agents/plugins/marketplace.json`의 로컬 플러그인을 설치합니다. `$codex-auto-<route>`는 추가 설치 없이 동작합니다 | 라우터 기준 추가 설치가 없습니다. `opencode.json`이 관리형 플러그인을 자동으로 연결합니다 |
| 생성되는 표면 | `.codex/skills/codex-*/`, `.codex/agents/`, `.codex/hooks.json`, `.codex/config.toml`, `.agents/plugins/marketplace.json`, `.autopus/plugins/auto/`, `AGENTS.md` | `.opencode/commands/`, `.opencode/agents/`, `.opencode/rules/`, `.opencode/plugins/`, `.agents/skills/`, `AGENTS.md`, `opencode.json` |
| 지금 잘 되는 것 | 네이티브 스킬, 로컬 플러그인 라우팅, Multi-Agent V2 Lead/Builder/Guardian 협업 | 네이티브 명령 래퍼, task 기반 worker, 관리형 hook plugin wiring |
| 현재 경계 | Multi-Agent V2 worker는 cwd와 파일시스템을 공유하므로, 병렬 writer에는 겹치지 않는 소유 범위가 필요합니다 | Claude식 Agent Teams와 Workflow primitive는 지원 범위로 주장하지 않습니다 |
| Worker 표면 | `spawn_agent`, `send_message`, `followup_task`, 대상 없는 `wait_agent`, `interrupt_agent`, `list_agents` | OpenCode `task(...)` worker |

split compiler 참고:
- `skills.compiler.mode: split`이 기본입니다. 핵심·참조 스킬과 모든 `/auto` 경로를 배포합니다. 나머지 지침은 `auto skill list`, `auto skill info <name>`으로 필요할 때 확인할 수 있습니다.
- 호환되는 전체 스킬을 배포하려면 `skills.compiler.mode: full`을, 일부만 추가하려면 `bundles` 또는 `explicit_skills`를 지정합니다. split 모드의 추가 스킬은 `.opencode/skills/` 또는 `.autopus/plugins/auto/skills/`에 놓입니다. 네이티브 목록에 노출하려면 `opencode_long_tail_target: shared`, `codex_long_tail_target: repo`를 사용합니다.

파이프라인 진입 스킬은 필요한 단계의 `references/` 문서만 읽습니다. 일반 작업은
현재 세션에서 처리하고, 독립 작업·전문 검토·컨텍스트 격리가 필요할 때 위임합니다.
`auto pipeline run`은 단일 변경 계약과 실제 변경 경로를 재검증해 저위험으로
판정한 경우에만 `implement → validate → review` 세 단계를 실행합니다.
계약이 없거나 중복·손상·불일치하면 다섯 단계를 유지합니다. 재개할 때도 저장된
route와 현재 승인된 route가 같아야 합니다. 어노테이션은 `auto spec gates --annotation`으로
명시적으로 요청하며, 검증·보안·데이터 손실·결정적 오라클 게이트는 유지합니다.

`workflow.coverage_threshold`의 기본값은 `85`입니다. 프로젝트나 route가 다른 값을
선언하면 그 값을 따르고, `0`이면 수치 게이트를 끕니다.
일반 `auto workflow context`에서 아키텍처 문서는 `--conditional-profile architecture`
또는 `--required-document`로 선택합니다. 필수 core·SPEC 본문은 완전성과 해시를
검증하며, 서명된 OMP canonical context와 릴리스 증거 경로는 바꾸지 않습니다.

---

## 🚀 빠른 시작 가이드

5분 안에 첫 AI 기반 기능을 만들어 보세요.

### 1단계 · 설치 (한 줄)

> **AI 코딩 도구(Claude Code, Codex, OpenCode 등)의 채팅창에 아래 명령을 붙여넣으세요** — 에이전트가 대신 실행해서 설치까지 처리합니다. 터미널에서 직접 실행해도 됩니다.

```bash
# macOS / Linux — 바이너리 설치 + 필수 도구 점검
cd your-project    # 프로젝트 폴더로 이동 (예: cd ~/my-app)
curl -sSfL https://raw.githubusercontent.com/autopus-ai/autopus-adk/main/install.sh | sh

# Windows (CMD or PowerShell)
cd your-project
powershell -c "irm https://raw.githubusercontent.com/autopus-ai/autopus-adk/main/install.ps1 | iex"
```

설치 스크립트는 `auto` CLI와 `autopus` alias를 함께 설치하고, 필수 도구를 점검한 뒤 이미 있는 것은 건너뛰고 누락된 `git`, GitHub CLI, Antigravity CLI 같은 핵심 도구를 자동 설치합니다. 대신 `auto init`은 자동으로 실행하지 않고 사용자에게 맡깁니다.

<details>
<summary>기타 설치 방법</summary>

```bash
# Homebrew (macOS) — 신규 설치 기준
brew install --cask Insajin/autopus/auto

# go install (Go 1.26+ 필요)
go install github.com/insajin/autopus-adk/cmd/auto@latest

# 소스에서 빌드
git clone https://github.com/autopus-ai/autopus-adk.git
cd autopus-adk && make build && make install
```

`make install`은 `$(go env GOPATH)/bin`에 복사하며, 다른 위치는 `INSTALL_DIR=...`로 지정합니다.
서명된 릴리스 설치본이 `PATH`에서 앞서면 install 타깃이 그 사실을 알립니다 — git 훅은 계속
그 바이너리를 실행합니다. 검증 대상 빌드로 훅을 돌리려면 `AUTOPUS_BIN=$PWD/bin/auto`를 씁니다.

Homebrew 신규 설치는 Cask를 기준으로 합니다. 기존 Formula로 설치했다면 Formula를 제거한
뒤 Cask로 옮기세요.

```bash
brew uninstall --formula auto
brew install --cask Insajin/autopus/auto
```

</details>

설치가 끝나면 스크립트가 아래 세 명령을 설명해줍니다.

- `auto init`: 현재 프로젝트를 초기화하고 `autopus.yaml`과 플랫폼 파일을 생성
- `auto update --self`: `auto` CLI 바이너리만 최신 버전으로 업데이트
- `auto update`: 현재 프로젝트의 생성된 규칙, 스킬, 에이전트, 플랫폼 파일을 갱신

### 2단계 · 프로젝트 초기화

```bash
cd your-project
auto init
```

Windows에서 `powershell -c ...`를 Git Bash 안에서 실행한 경우에는, 부모 Bash 프로세스의 `PATH`가 즉시 갱신되지 않으므로 설치 후 Git Bash를 다시 열어야 합니다. 이 경우 설치 스크립트가 실제 설치 경로와 `export PATH=...` 안내를 함께 출력합니다.

`auto init`은 설치된 AI 코딩 CLI(Claude Code, Codex, Antigravity CLI, OpenCode, Oh My Pi)를 자동 감지하고, 각 플랫폼에 맞는 **네이티브 설정** — 규칙, 스킬, 에이전트, 명령 — 을 하나의 `autopus.yaml`에서 생성합니다.

Claude Code statusline 참고:
- `.claude/settings.json`에 기존 `statusLine.command`가 있으면, `auto init` / `auto update` 인터랙티브 실행 시 `keep`, `merge`, `replace` 중 하나를 고를 수 있습니다.
- 비대화형에서는 `--statusline-mode keep|merge|replace`로 같은 동작을 명시할 수 있습니다.

플랫폼별 명령 문법:
- Codex: 생성된 로컬 플러그인을 설치하면 `@auto ...`를 사용하고, 설치 전에는 `$codex-auto-<route> ...`를 실행합니다
- OpenCode: `/auto ...` 또는 `/auto-<subcommand> ...`를 사용합니다
- Claude Code / Antigravity CLI / Oh My Pi: `/auto ...`를 사용합니다

```
✓ 감지됨: claude-code, codex, antigravity-cli, opencode, omp
✓ 생성됨: .claude/rules/, .claude/skills/, .claude/agents/, .claude/workflows/, CLAUDE.md
✓ 생성됨: .codex/skills/, .codex/agents/, .codex/hooks.json, .codex/config.toml, AGENTS.md
✓ 생성됨: .gemini/, GEMINI.md
✓ 생성됨: .opencode/, .agents/skills/, AGENTS.md, opencode.json
✓ 생성됨: .omp/rules/, .omp/skills/, .omp/commands/, 선택형 .omp/extensions/
✓ 생성됨: autopus.yaml
```

### 3단계 · 프로젝트 컨텍스트 생성 (`/auto setup`)

**가장 중요한 단계입니다.** AI 에이전트는 세션 간 모든 기억을 잃습니다 — 매번 프로젝트를 처음 보는 것과 같습니다. `/auto setup`은 에이전트가 프로젝트를 즉시 이해할 수 있게 해주는 "온보딩 문서"를 생성합니다.

```bash
/auto setup     # Claude Code, Antigravity CLI, OpenCode
@auto setup     # Codex 로컬 플러그인 설치 후
$codex-auto-setup  # Codex 플러그인 설치 전 네이티브 스킬
```

코드베이스를 분석하여 5개의 컨텍스트 문서를 생성합니다:

```
ARCHITECTURE.md                    # 도메인, 레이어, 의존성 맵
.autopus/project/product.md       # 프로젝트 설명, 핵심 기능
.autopus/project/structure.md     # 디렉토리 구조, 패키지 역할, 엔트리포인트
.autopus/project/tech.md          # 기술 스택, 빌드, 테스트 전략
.autopus/project/scenarios.md     # 코드에서 추출된 E2E 테스트 시나리오
```

> 💡 **왜 중요한가요?** 이 문서 없이 AI가 프로젝트를 보는 것은, 온보딩 없이 첫 출근한 신입사원과 같습니다 — 아키텍처를 추측하고, 컨벤션을 놓치고, 이미 존재하는 패턴을 다시 만들게 됩니다. `/auto setup`으로 모든 에이전트 세션이 정보를 갖고 시작합니다.

### 선택 사항: UI 작업용 `DESIGN.md`

프론트엔드 검증과 리뷰는 프로젝트 루트의 `DESIGN.md`를 가벼운 디자인 컨텍스트로 사용할 수 있습니다. `auto init`은 `autopus.yaml` 옆에 starter `DESIGN.md`를 만들되 기존 파일은 덮어쓰지 않고, `auto update`는 오래된 하네스 설치에 starter 파일과 `design:` 설정 블록을 백필합니다. 문서는 source of truth, palette roles, typography hierarchy, component guardrails, layout/responsive rules, agent guidance를 짧게 담는 것을 권장합니다. `DESIGN.md` 또는 설정된 디자인 baseline이 없으면 `/auto verify`, Phase 3.5, `/auto review`, `auto orchestra review`는 기존처럼 실행되며 `Design context: skipped (not configured)`를 non-error로 보고합니다.

디자인 컨텍스트는 `.tsx`, `.jsx`, CSS 계열 파일, theme/token 파일, design-system 경로 같은 UI diff에만 주입됩니다. UI finding은 palette-role drift, typography hierarchy drift, component guardrail violation, layout/responsive regression, source-of-truth mismatch를 확인합니다. 리뷰 surface는 읽기 전용이며 직접 수정하지 않고 executor/fixer로 수정 작업을 위임합니다.

생성된 플랫폼 surface는 canonical source가 아닙니다. `.claude/*`, `.codex/*`, `.gemini/*`, `.opencode/*`, `.omp/*`, OpenCode 소유 `.agents/skills/*`, plugin surface를 고치려면 `autopus-adk`의 content/template을 수정한 뒤 대상 프로젝트에서 `auto update`를 실행하세요.

외부 디자인 reference는 명시적으로 promote되기 전까지 신뢰하지 않는 supplemental context입니다. `auto design import`는 sanitized artifact를 `.autopus/design/imports/<import-id>/` 아래에 저장하며, 기존 사람이 관리하는 `DESIGN.md`를 기본으로 덮어쓰지 않습니다. URL import는 public HTTPS만 허용하는 SSRF 보호 경로로 동작하고 localhost/private/metadata 대상, unsafe redirect, 과도한 redirect, timeout, 1 MiB 초과 응답을 차단하며 거부 시 redacted diagnostics만 남깁니다.

### 4단계 · 첫 기능 만들기

준비 완료. 원하는 것을 자연어로 설명하세요:

```bash
# 1. 기획 — AI가 전체 SPEC 생성 (요구사항, 태스크, 수락 기준)
/auto plan "GET /healthz 헬스 체크 엔드포인트 추가"

# 2. 구현 — 16개 에이전트가 구현, 테스트, 리뷰 처리
/auto go SPEC-HEALTH-001 --auto

# 3. 배포 — 문서 동기화, SPEC 상태 업데이트, 의사결정 이력과 함께 커밋
/auto sync SPEC-HEALTH-001
```

```
╭────────────────────────────────────╮
│ 🐙 파이프라인 완료!                 │
│ SPEC-HEALTH-001: 헬스 체크          │
│ 태스크: 3/3 │ 커버리지: 92%         │
│ 리뷰: APPROVE                      │
╰────────────────────────────────────╯
```

이게 전부입니다 — 테스트, 보안 감사, 완전한 문서화가 포함된 프로덕션 수준 코드가 세 개의 명령으로 완성됩니다.

### 빠른 참조

| 하고 싶은 것 | 명령어 |
|-------------|--------|
| **아이디어 브레인스토밍** | `/auto idea "설명" --multi --ultrathink` |
| **풀 사이클 (추천)** | `/auto dev "설명"` |
| 새 기능 기획 | `/auto plan "설명"` |
| SPEC 구현 | `/auto go SPEC-ID --auto --loop --team` |
| 버그 수정 (SPEC 불필요) | `/auto fix "설명"` |
| 자연어로 설명만 | `/auto 로그인 페이지에 2FA 추가` |
| 배포 후 헬스 체크 | `/auto canary` |
| 코드 리뷰 | `/auto review` |
| 보안 감사 | `/auto secure` |
| 중단된 파이프라인 재개 | `/auto go SPEC-ID --continue` |
| 변경 후 문서 동기화 | `/auto sync SPEC-ID` |

### 업데이트

Autopus-ADK 업데이트는 서로 다른 두 단계로 진행합니다. 새 릴리스를 적용할 때는 갱신하려는
프로젝트에서 아래 순서대로 실행하세요.

**1. 바이너리 업데이트** — `auto` CLI 자체를 최신 버전으로:

```bash
auto update --self
```

GitHub Releases에서 최신 버전을 확인하고 CLI 바이너리만 교체합니다. v0.50.73부터 포함된
업데이터는 ECDSA P-256 게시자 서명 envelope(`checksums.txt.signatures`)로 `checksums.txt`를 먼저
인증한 다음, 압축을 풀기 전에 아카이브의 SHA256 체크섬을 검증합니다. 서명 데이터가 없거나
형식이 잘못됐거나 신뢰할 수 없거나 만료됐으면 체크섬만 검증하는 방식으로 돌아가지 않고 중단합니다.
이 명령은 프로젝트의 생성 파일을 갱신하지 않습니다. 현재 버전은 `auto version`으로
확인하세요. macOS에서는 v0.50.72부터 포함된 업데이터가 다운로드한 릴리스 바이트와
Developer ID 서명도 그대로 보존합니다.

Darwin과 Linux에서는 대상 파일시스템에 새 바이너리를 준비한 뒤 원자 교환으로 반영합니다.
커널이나 파일시스템이 원자 교환을 지원하지 않으면 설치된 바이너리를 변경하지 않고
중단합니다. 이때는 패키지 관리자를 사용하거나 릴리스를 직접 다시 설치하세요.

Windows에서는 새 바이너리를 배치하는 동안 기존 바이너리를 같은 위치의 `<binary>.old`에
보존합니다. 두 파일 이동 사이에 프로세스가 강제 종료되거나 전원이 끊기면 대상 경로가
일시적으로 비어 있을 수 있습니다. 실제 설치 경로를 사용해 PowerShell에서 복구한 뒤
업데이트를 다시 실행하세요.

```powershell
Move-Item -LiteralPath "C:\path\to\auto.exe.old" -Destination "C:\path\to\auto.exe"
auto update --self
```

`auto.exe`와 `auto.exe.old`가 모두 있을 때는 이전 설치가 완료된 뒤 업데이터가 직접 기록한
`<binary>.old.autopus-complete` marker가 유효한 경우에만 `.old`를 자동 정리합니다. marker가
없거나 유효하지 않은 복구본은 자동으로 삭제하지 않습니다. 유지할 바이너리를 확인한 뒤
`.old`를 직접 복원하거나 삭제하세요.

> **v0.50.71 이하 버전에서 시작하는 macOS 1회 이행 절차:** 이때는 아래의 한 줄 단축 명령을
> 사용하지 말고, 다음 명령을 순서대로 각각 실행하세요.
>
> ```bash
> auto update --self
> auto update --self --force
> auto update
> ```
>
> 첫 번째 명령은 기존 업데이터가 실행합니다. SHA256 체크섬은 검증하지만 게시자 서명
> envelope를 인증하지 않으며, 다운로드한 Developer ID 서명을 ad hoc 서명으로 바꿀 수
> 있습니다. 첫 실행으로 v0.50.72 이상을 설치하면 디스크의 수정된 업데이터가 두 번째 명령을
> 실행합니다. 이 명령이 릴리스와
> 정확히 같은 바이너리 바이트를 다시 설치하여 Developer ID 서명과
> `TeamIdentifier=GP2PFA2PUV`를 복원합니다. 마지막 `auto update`는 현재 프로젝트의 생성
> 파일을 갱신합니다. 이미 v0.50.72 이상을 사용 중이라면 이후 바이너리 self-update에는
> `auto update --self`를 한 번만 실행하면 됩니다. 프로젝트까지 갱신하려면 이어서
> `auto update`를 실행하세요. Cask 신규 설치나 기존 Formula에서 Cask로의 이행은 서명된
> 릴리스 아티팩트를 직접 설치하므로 self-update를 두 번 실행할 필요가 없습니다.

**2. 하네스 업데이트** — 설치된 CLI의 템플릿을 현재 프로젝트에 적용:

```bash
auto update
```

설치된 CLI의 템플릿으로 규칙, 스킬, 에이전트와 `.claude/*`, `.codex/*`, `.gemini/*`,
`.opencode/*`, `.omp/*`, OpenCode 소유 `.agents/skills/*` 같은 플랫폼별 파일을 갱신합니다.
`skills.compiler.mode: split`을 사용하면 `.opencode/skills/*`와
`.autopus/plugins/auto/skills/*`도 미리보기와 적용 대상에 포함하며, 오래된 생성 파일도 함께
정리합니다. `AUTOPUS:BEGIN`~`AUTOPUS:END` 마커 바깥의 사용자 편집은 보존됩니다. 새로
설치된 플랫폼이 있으면 자동으로 감지하여 해당 파일도 생성합니다.

Claude Code에 기존 user-managed `statusLine.command`가 있으면, update는 기본적으로 이를 보존합니다. 필요하면 `--statusline-mode keep|merge|replace`로 기존 statusline 유지, Autopus와 2줄 병합, 완전 교체를 명시할 수 있습니다.

**한 줄로 둘 다(v0.50.72 이상을 사용 중인 경우):**

```bash
auto update --self && auto update
```

> **언제 업데이트하나요?** `auto update --self`로 새 바이너리를 설치한 뒤 `auto update`까지
> 성공해야 현재 프로젝트에 생성된 규칙, 스킬, 에이전트와 플랫폼 파일에 새 버전이 반영됩니다.

### Autopus Desktop 관리 런처

Autopus Desktop을 설치한 macOS에서는 앱이 PATH의 `auto` 항목(보통 `~/.local/bin/auto`)을
소유하며, 모든 호출을 Desktop이 관리하는 CLI 슬롯으로 넘기는 작은 런처 스크립트로 대체합니다.
앱이 이 호출을 거절하면 `auto`는 종료 코드 126과 함께 다음 메시지를 출력합니다.

```text
Autopus Desktop managed ADK broker: managed_adk_broker_current_slot_rejected
```

이때도 CLI 자체는 설치돼 있습니다. 관리 슬롯의 바이너리를 직접 실행하거나(경로에 공백이 있으니
반드시 따옴표로 감싸세요) 현재 셸에 alias를 걸면 됩니다.

```bash
"$HOME/Library/Application Support/co.autopus.desktop/managed-adk/current/auto" doctor
alias auto="$HOME/Library/Application Support/co.autopus.desktop/managed-adk/current/auto"
```

그다음 이 관리 바이너리에서 `auto update`를 실행하거나 Autopus Desktop을 재설치하면 런처와
관리 슬롯이 다시 일치합니다. `auto doctor`는 이 런처를 `doctor.launcher.desktop_shim` 검사로
보고하며, 런처 경로와 관리 슬롯 경로, 관리 바이너리 존재 여부를 함께 알려 줍니다.

### 일반적인 시나리오

<details>
<summary><strong>"버그를 수정하고 싶어요"</strong></summary>

```bash
/auto fix "로그인 페이지에서 500 에러"
```

에이전트가 자동으로:
1. 재현 테스트 작성 (실패 확인)
2. 근본 원인 분석
3. 최소한의 수정 적용
4. 모든 테스트 통과 확인

SPEC 불필요 — 즉시 실행.
</details>

<details>
<summary><strong>"새 기능을 추가하고 싶어요"</strong></summary>

```bash
# 작은 기능 — SPEC만, PRD 생략
/auto plan "GET /healthz 헬스 체크 엔드포인트 추가" --skip-prd

# 큰 기능 — 전체 PRD + SPEC
/auto plan "OAuth2 Google + GitHub 프로바이더 지원"

# 아이디어부터 탐색 — 멀티 프로바이더 브레인스토밍
/auto idea "마이크로서비스로 전환해야 할까?" --multi
```

`/auto idea`는 ICE 채점(Impact, Confidence, Ease)과 멀티 프로바이더 브레인스토밍을 실행하고, BS 파일을 생성하며, `/auto plan`으로 직접 연결할 수 있습니다.
</details>

<details>
<summary><strong>"코드 리뷰를 받고 싶어요"</strong></summary>

```bash
/auto review                    # 현재 변경사항 TRUST 5 리뷰
/auto secure                    # OWASP Top 10 보안 스캔
/auto review --multi            # 멀티 모델 교차 리뷰 (토론 전략)
```
</details>

<details>
<summary><strong>"그냥 자연어로 설명하고 싶어요"</strong></summary>

```bash
/auto 로그인 페이지에 2FA 추가
```

Autopus Triage가 자동으로 요청을 분석합니다:
- 복잡도 평가 (LOW / MEDIUM / HIGH)
- 영향 범위 스캔
- 추천 워크플로우 (fix / plan / idea)

```
🐙 Triage ────────────────────────────
  Request: "로그인 페이지에 2FA 추가"
  Complexity: HIGH → /auto idea --multi (추천)
```

Codex에서는 `.agents/plugins/marketplace.json`의 로컬 플러그인을 설치한 뒤 `@auto ...`를 쓰거나, `$codex-auto-<route> ...`를 바로 실행하면 됩니다. 플러그인은 라우터만 추가하고, 상세 워크플로는 고유한 네이티브 `.codex/skills/codex-<name>/SKILL.md`에 둡니다.
</details>

### 🔄 업데이트

현재 설치된 CLI가 v0.50.72 이상이라면 새 릴리스를 프로젝트에 모두 반영할 때 두 명령을
순서대로 실행합니다.

```bash
auto update --self && auto update
```

첫 번째 명령은 CLI 바이너리만 교체합니다. 두 번째 명령이 생성된 규칙, 스킬, 에이전트와
플랫폼 파일을 현재 프로젝트에 반영합니다. 마커 바깥의 사용자 편집은 그대로 보존합니다.
v0.50.71 이하에서 macOS self-update로 이행할 때는 위의 **업데이트** 섹션에 있는 1회 이행
절차(`auto update --self`, `auto update --self --force`, `auto update`)를 사용하세요.

---

## 🤖 파이프라인

### 위험도에 맞춘 실행

일반 작업은 현재 세션에서 처리하고, 검증된 compact 계약은 런타임의 세 단계 경로를 사용합니다.
아래 그림은 전체 작업의 역할을 보여주는 예시이며, 단계마다 별도 에이전트를 강제하지 않습니다.

```mermaid
sequenceDiagram
    participant S as SPEC
    participant P as 🧠 Planner
    participant T as 🧪 Tester
    participant E as ⚡ Executor ×N
    participant A as 📝 Annotator
    participant V as ✅ Validator
    participant R as 🔍 Reviewer + 🛡️

    S->>P: Phase 1: 태스크 분해 + 에이전트 할당
    P->>T: Phase 1.5: 실패하는 테스트 스캐폴드 (RED)

    rect rgb(230, 245, 255)
        Note over E: Phase 2: 병렬 워크트리에서 TDD 구현
        T->>E: T1, T2, T3 ... (병렬)
    end

    opt 명시적인 어노테이션 요청
        E->>A: 요청한 @AX 태그 적용
        A-->>E: 어노테이션 결과
    end
    E->>V: Gate 2: 빌드 + 린트 + vet
    V->>T: Phase 3: 관련 테스트 + 커버리지 게이트 (기본 85%)
    T->>R: Phase 4: TRUST 5 리뷰 + OWASP 감사
    R-->>S: ✅ APPROVE
```

### 16개 전문 에이전트

| 에이전트 | 역할 | 실행 시점 |
|---------|------|----------|
| **Planner** | SPEC 분해, 태스크 할당, 복잡도 평가 | Phase 1 |
| **Spec Writer** | spec.md, plan.md, acceptance.md, research.md 생성 | `/auto plan` |
| **Tester** | 테스트 스캐폴드 (RED) + 커버리지 부스트 (GREEN) | Phase 1.5, 3 |
| **Executor** | 구현; 지원되고 필요한 경우 네이티브 격리 사용 | 위임할 때 |
| **Annotator** | @AX 태그 라이프사이클 관리 | 명시적으로 요청할 때만 |
| **Validator** | 빌드, vet, 린트, 파일 크기 검사 | Gate 2 |
| **Reviewer** | TRUST 5 코드 리뷰 | Phase 4 |
| **Security Auditor** | OWASP Top 10 취약점 스캔 | Phase 4 |
| **Architect** | 시스템 설계, 아키텍처 결정 | 온디맨드 |
| **Debugger** | 재현 우선 버그 수정 | `/auto fix` |
| **DevOps** | CI/CD, Docker, 인프라 | 온디맨드 |
| **Frontend Specialist** | Playwright E2E + VLM 시각적 회귀 감지 | Phase 3.5 |
| **UX Validator** | 프론트엔드 컴포넌트 시각적 검증 | Phase 3.5 |
| **Perf Engineer** | 벤치마크, pprof, 성능 회귀 감지 | 온디맨드 |
| **Deep Worker** | 장시간 자율 탐색 + 구현 | 온디맨드 |
| **Explorer** | 코드베이스 구조 분석 | `/auto map` |

### 품질 모드

```bash
/auto go SPEC-ID --quality ultra      # 모든 역할에 프리미엄 경로 적용, Codex effort는 역할별 차등
/auto go SPEC-ID --quality balanced   # 기획·리뷰·디버깅은 최상위, 구현·검증은 경량 모델

auto quality ultra --apply            # Ultra를 기본값으로 저장하고 현재 프로젝트에 반영
auto quality balanced --apply         # Balanced를 기본값으로 저장하고 현재 프로젝트에 반영
auto quality provider claude ultra --apply   # Claude에만 적용 (claude-code alias도 허용)
auto quality provider codex balanced --apply # Codex에만 적용
auto quality provider claude inherit --apply # Claude override 제거
auto quality supervisor inherit --apply  # Codex 주 세션에서 사용자 기본 모델을 사용
auto quality show                     # 저장된 품질 모드와 주 세션 정책 확인
```

Claude Code와 Codex를 함께 쓰는 프로젝트는 품질 모드를 서로 다르게 지정할 수 있습니다.

```yaml
quality:
  default: balanced
  providers:
    claude: ultra
    codex: balanced
```

`quality.providers`의 canonical key는 `claude`, `codex`입니다. override가 없는 provider는
기존 `quality.default`를 그대로 상속하므로 이전 설정 파일의 동작은 바뀌지 않습니다.
Custom preset 이름은 1–64자의 ASCII 영숫자로 시작하고, 이후에는 영숫자·하이픈·밑줄만
사용할 수 있습니다.
한 번의 실행에 지정한 `--quality`는 YAML을 수정하지 않는 명시적 전역 override이며 저장된
두 provider 값보다 우선합니다. Provider별 `--apply`는 해당 configured platform만 갱신하고,
기존 전역 `auto quality <mode> --apply`는 구성된 모든 플랫폼을 계속 갱신합니다.

새 프로젝트의 `supervisor_model_policy` 기본값은 `inherit`입니다. 따라서 Autopus는 Codex 주
세션의 사용자 기본 모델을 덮어쓰지 않습니다. 품질 모드는 관리형 에이전트와 quality-managed
orchestra provider에 계속 적용됩니다. 이 정책이 없는 기존 프로젝트는 이전의 quality-managed
해석을 유지하되, 소유권이 모호한 markerless root assignment는 마이그레이션 중 보존합니다.
알려진 생성 프로필을 명시적으로 제거하려면 `auto quality supervisor inherit --apply`를,
변경되지 않은 Autopus 관리 주 세션 설정에 선택한 품질 모드의 Astra 프로필을 적용하려면
`auto quality supervisor quality --apply`를 실행하세요. 사용자가 직접 설정한 프로젝트의
`model` 또는 `model_reasoning_effort` 값은 그대로 유지되며 이 정책보다 먼저 적용됩니다.
관리형 에이전트 정의는 세션을 시작할 때 불러오므로, 설정을 반영한 뒤 새 Codex 세션을
시작해야 합니다.

GPT/Codex Ultra 기능이 CLI에 포함되어 있어도 프로젝트에서 자동으로 활성화되지는 않습니다.
먼저 `auto update`로 프로젝트의 생성 파일을 갱신하고, 사용할 프로젝트에서
`auto quality ultra --apply`를 명시적으로 실행한 뒤 새 Codex 세션을 시작해야 합니다.
`Ultra compact` 활성화와 promotion은 별도로 진행하며, 위 업데이트 명령만으로 활성화되지 않습니다.

Claude Code와 Codex도 OMP와 같은 표준 Balanced 역할표를 사용합니다.

| 네이티브 에이전트 그룹 | Claude Code balanced | Codex balanced |
|---|---|---|
| planner, architect, spec-writer, reviewer, security-auditor, debugger, deep-worker | `claude-fable-5-1` / `max` | `gpt-6-astra` / `max` |
| executor, tester, devops, frontend-specialist, perf-engineer | `claude-sonnet-5` / `max` | `gpt-5.6-luna` / `max` |
| explorer, annotator, validator, ux-validator | `claude-sonnet-5` / `high` | `gpt-5.6-luna` / `max` |

실제 생성 파일은 `.claude/agents/autopus/*.md`의 `model`·`effort`와
`.codex/agents/*.toml`의 `model`·`model_reasoning_effort`입니다.
`auto quality balanced --apply`로 함께 적용하거나 위의 provider별 명령을 사용하세요.
과거 기본 배치와 완전히 일치하는 설정은 YAML을 다시 쓰지 않고 표준 배치로 해석합니다.
특정 역할을 다른 티어로 지정했거나 사용자 품질 프리셋을 만들었다면 기존 해석을 유지합니다.
한 에이전트의 지정값을 바꿔도 다른 역할의 배치는 바뀌지 않습니다.

Codex 카탈로그를 읽을 수 없으면 요청한 표준 모델을 유지하고 미검증임을 알립니다.
관측한 카탈로그가 모델이나 추론 강도를 지원하지 않으면, 낮은 모델로 대체하지 않고
파일을 쓰기 전에 생성을 차단합니다.

Ultra는 기존대로 핵심 7개 역할에 Fable/Astra, 나머지에 Opus/Sol을 사용합니다.
Claude Ultra의 Fable/Opus는 max, Codex Ultra는 Astra/max와 Sol/xhigh입니다.
주 세션은 기본적으로 사용자 모델을 상속합니다. quality-managed Codex 주 세션만
Ultra에서 Astra/ultra, Balanced에서 Astra/xhigh를 사용합니다.
네이티브 멀티프로바이더 리뷰 기본값은 두 모드 모두 Fable 5.1/max와 Astra/max이며,
명시적인 provider 모델·추론 강도 지정은 보존합니다.

### 실행 모드

| 플래그 | 모드 | 설명 |
|--------|------|------|
| *(기본)* | 현재 세션 우선 | 독립 작업이나 전문 검토·컨텍스트 격리가 필요한 작업만 위임 |
| `--team` | 팀 토폴로지 | 플랫폼 네이티브 Lead / Builder / Guardian 책임 프로필 |
| `--solo` | 단일 세션 | 서브에이전트 없이 직접 TDD |
| `--auto --loop` | 완전 자율 | RALF 자가 치유, 사용자 승인 없음 |
| `--multi` | 프로바이더 다양성 | 멀티프로바이더 기획·리뷰이며 실행 토폴로지가 아님 |

OMP에서는 두 축을 분리합니다. `--team`은 owner `omp`의 네이티브 `task`/`hub`/`todo` 토폴로지를 선택하고, `--multi`는 멀티프로바이더 기획·리뷰를 추가합니다. `--team --multi`를 함께 사용할 수 있습니다.

---

## 📐 워크플로우

### 빠른 경로 — 두 개의 명령

대부분의 기능에는 두 개의 명령만 필요합니다:

```bash
# 1. 브레인스토밍 — 멀티 프로바이더 토론 + 깊은 분석
/auto idea "재시도와 데드 레터 큐를 갖춘 웹훅 전송 추가" --multi --ultrathink

# 2. 빌드 & 배포 — 전체 자율 파이프라인
/auto dev "재시도와 데드 레터 큐를 갖춘 웹훅 전송 추가"
```

`/auto idea`는 멀티 프로바이더 브레인스토밍(Claude x Codex x Gemini 토론)을 딥 순차 사고와 함께 실행하고, ICE로 아이디어를 채점하고, 결과를 저장합니다.

`/auto dev`는 나머지를 처리합니다 — **plan → go → sync**를 한 번에, 모든 파워 플래그가 기본 활성화:

| 단계 | 수행 내용 | 플래그 (자동 적용) |
|------|----------|-------------------|
| **plan** | PRD + SPEC + 멀티 프로바이더 리뷰 | `--auto --multi --ultrathink` |
| **go** | 16개 에이전트 Agent Teams + 자가 치유 | `--auto --loop --team` |
| **sync** | 문서 + 변경 이력 + Lore 커밋 | — |

> 풀 파워가 필요 없다면? `--solo`로 단일 세션 모드, `--no-multi`로 멀티 프로바이더 리뷰 생략, 또는 `plan` / `go` / `sync`를 개별 실행하여 세밀하게 제어하세요.

### 수동 경로 — 세 개의 명령

더 세밀한 제어를 원한다면, 각 단계를 별도로 실행하세요:

```mermaid
flowchart LR
    PLAN["📋 plan\n기획"]
    GO["🚀 go\n구현"]
    SYNC["📦 sync\n배포"]

    PLAN -->|SPEC 생성| GO -->|코드 + 테스트| SYNC
```

### 📋 1단계 · `/auto plan` — 원하는 것을 설명하세요

자연어 설명을 완전한 **SPEC**으로 변환합니다 — 요구사항, 태스크, 수락 기준, 리스크 분석까지.

```bash
/auto plan "재시도와 데드 레터 큐를 갖춘 웹훅 전송 추가"
```

spec-writer 에이전트가 5개 문서를 생성합니다:

```
.autopus/specs/SPEC-HOOK-001/
├── prd.md          # 제품 요구사항 문서
├── spec.md         # EARS 형식 요구사항
├── plan.md         # 태스크 분해 + 에이전트 할당
├── acceptance.md   # Given-When-Then 수락 기준
└── research.md     # 기술 조사 + 리스크
```

옵션: `--multi` 읽기 전용 멀티 프로바이더 기획 검토와 최종 SPEC 리뷰 · `--prd-mode minimal` 경량 PRD · `--skip-prd` PRD 건너뛰고 바로 SPEC

### 🚀 2단계 · `/auto go` — 구현하기

SPEC을 **16개 에이전트**에 전달합니다. 기획, 테스트 스캐폴드, 병렬 구현, 검증, 어노테이션, 테스트, 리뷰까지 — 모두 자동으로.

```bash
/auto go SPEC-HOOK-001 --auto --loop
```

```
Phase 1    │ 🧠 Planner         │ SPEC → 태스크 + 에이전트 할당
Phase 1.5  │ 🧪 Tester          │ 실패하는 테스트 스켈레톤 (RED)
Phase 2    │ ⚡ Executor ×N      │ 병렬 워크트리에서 TDD
Optional   │ 📝 Annotator       │ 명시적으로 요청한 @AX 태그만 적용
Gate  2    │ ✅ Validator        │ 빌드 + 린트 + vet
Phase 3    │ 🧪 Tester          │ 관련 테스트 + 커버리지 게이트 (기본 85%)
Phase 4    │ 🔍 Reviewer + 🛡️    │ TRUST 5 + OWASP 감사
```

옵션: `--team` Agent Teams · `--solo` 단일 세션 TDD · `--quality ultra` 프리미엄 실행 경로 · `--multi` 멀티 모델 리뷰

### 📦 3단계 · `/auto sync` — 배포하고 문서화하기

SPEC 상태 업데이트, 프로젝트 문서 재생성, @AX 태그 라이프사이클 관리, 구조화된 Lore 이력으로 커밋.

```bash
/auto sync SPEC-HOOK-001
auto sync verify --spec SPEC-HOOK-001 --strict
```

커밋하기 전에 `auto sync verify`로 read-only 스테이징 계획을 확인합니다. 명령은 감지한 토폴로지를 함께 출력합니다. 멀티 리포 워크스페이스는 Phase A(모듈)와 Phase B(메타)로 나뉘고, `autopus.yaml`을 가진 단일 Git 리포지터리는 linked worktree를 포함해 커밋 그룹 하나로 묶입니다. 두 토폴로지 모두 generated/runtime, tracked-but-ignored, 미분류, 셸에서 안전하지 않은 경로를 계획에서 제외하며, 둘 중 어디에도 해당하지 않는 위치는 별도 종료 코드와 함께 `unsupported topology:` 진단으로 실패합니다. `--spec`은 SPEC 호스트가 정확히 하나인지 확인한 뒤 워크스페이스 상대 경로 가운데 해당 SPEC이 소유한 파일만 남깁니다. `--strict`는 제외되거나 무관한 경로가 하나라도 있으면 실패합니다.

```
╭────────────────────────────────────╮
│ 🐙 파이프라인 완료!                 │
│ SPEC-HOOK-001: 웹훅 전송           │
│ 태스크: 5/5 │ 커버리지: 91%         │
│ 리뷰: APPROVE                      │
╰────────────────────────────────────╯
```

**끝입니다.** 세 개의 명령: 기획 → 구현 → 배포. 모든 결정이 기록됩니다. 모든 테스트가 강제됩니다.

---

## 🎯 TRUST 5 코드 리뷰

모든 리뷰는 5개 차원으로 평가됩니다:

| | 차원 | 검사 항목 |
|---|------|----------|
| **T** | Tested (테스트) | 관련 동작, 경계 조건, race 검사, 커버리지 게이트 (기본 85%) |
| **R** | Readable (가독성) | 명확한 이름, 응집된 책임, 프로젝트가 정한 한도 |
| **U** | Unified (일관성) | gofmt, goimports, golangci-lint, 일관된 패턴 |
| **S** | Secured (보안) | OWASP Top 10, 인젝션 없음, 하드코딩된 시크릿 없음 |
| **T** | Trackable (추적성) | 의미 있는 로그, 에러 컨텍스트, SPEC/Lore 참조 |

---

## 📊 멀티 모델 오케스트레이션

| 전략 | 작동 방식 | 적합한 용도 |
|------|----------|------------|
| **🤝 Consensus** | 독립 응답을 키 합의로 병합 | 기획, 코드 리뷰 |
| **⚔️ Debate** | 2단계 토론 + 판사 판정 | 중요 결정, 보안 |
| **🔗 Pipeline** | N번째 출력 → N+1번째 입력 | 반복 정제 |
| **⚡ Fastest** | 가장 먼저 완료된 응답 사용 | 빠른 질문 |

프로바이더: **Claude** · **Codex** · **Gemini** · **OpenCode** — 그레이스풀 디그레이드 지원.

**인터랙티브 토론**을 실시간 창 시각화(cmux/tmux)로. **훅 기반 결과 수집**으로 구조화된 JSON 출력. Context7 문서 불가 시 **WebSearch 폴백**.

---

## 📖 전체 명령어

<details>
<summary><strong>CLI 명령어</strong> (루트 28개, 서브커맨드 포함 110개 이상)</summary>

| 명령어 | 설명 |
|--------|------|
| `auto init` | 하네스 초기화 — 플랫폼 감지, 파일 생성 |
| `auto update` | 하네스 업데이트 (마커 기반, 사용자 편집 보존) |
| `auto quality` | 품질 모드 저장·조회, 관리형 프로필 반영, 주 세션 모델 소유권 선택 |
| `auto doctor` | 상태 진단 |
| `auto platform` | 플랫폼 관리 (list / add / remove) |
| `auto arch` | 아키텍처 분석 (generate / enforce) |
| `auto spec` | SPEC 관리 (new / validate / review / gates — exact-input 증거 재사용을 포함한 gate applicability receipt) |
| `auto lore` | 의사결정 추적 (context / commit / validate / stale) |
| `auto orchestra` | 멀티 모델 오케스트레이션 (review / plan / secure / brainstorm / job-status / job-wait / job-result) |
| `auto setup` | 프로젝트 컨텍스트 문서 (generate / update / validate / status) |
| `auto status` | SPEC 대시보드 (done / in-progress / draft) |
| `auto telemetry` | 파이프라인 텔레메트리 (record / summary / cost / compare / leadtime — first-slice·critical-path 리드타임과 baseline 회귀 게이트) |
| `auto skill` | 스킬 관리 (list / info / create) |
| `auto search` | 지식 검색 (Exa) |
| `auto docs` | 라이브러리 문서 조회 (Context7) |
| `auto design` | 디자인 컨텍스트와 provider 문서 (context / import / pack / docs / figma) |
| `auto lsp` | LSP 연동 (diagnostics / refs / rename / symbols / definition) |
| `auto verify` | 프론트엔드 UX 검증 (Playwright + VLM) |
| `auto check` | 하네스 규칙 검사 (안티패턴 스캔) |
| `auto hash` | 파일 해싱 (xxhash) |
| `auto issue` | 자동 이슈 리포터 (report / list / search) |
| `auto experiment` | 자율 실험 루프 (init / metric / record / commit / reset / summary / status) |
| `auto test` | E2E 시나리오 실행기 (run) |
| `auto react` | Reaction Engine (check / apply) |
| `auto agent` | 에이전트 관리 (create / run) |
| `auto terminal` | 터미널 멀티플렉서 관리 (detect / workspace / split / send / notify) |
| `auto pipeline` | 파이프라인 상태 관리 및 모니터링 |
| `auto permission` | 권한 모드 감지 (bypass / safe) |
| `auto browse` | 브라우저 자동화 (cmux browser / agent-browser) |
| `auto canary` | 배포 후 헬스 체크 (빌드 + E2E + 브라우저) |
| `auto connect` | 프로바이더 연결 마법사 (server auth → workspace → OpenAI OAuth) |
| `auto connect status` | 저장된 연결 상태 로컬 검증/ready 요약 |
| `auto update --self` | CLI 바이너리 자동 업데이트 (게시자 서명 + SHA256) |

</details>

<details>
<summary><strong>슬래시 명령어</strong> (AI 코딩 CLI 내부)</summary>

| 명령어 | 설명 |
|--------|------|
| `/auto plan "설명"` | 새 기능의 SPEC 작성 |
| `/auto go SPEC-ID` | 전체 파이프라인으로 구현 |
| `/auto go SPEC-ID --auto --loop` | 완전 자율 + 자가 치유 |
| `/auto go SPEC-ID --team` | Agent Teams (Lead/Builder/Guardian) |
| `/auto go SPEC-ID --multi` | 멀티 프로바이더 오케스트레이션 |
| `/auto fix "버그"` | 재현 우선 버그 수정 |
| `/auto review` | TRUST 5 코드 리뷰 |
| `/auto secure` | OWASP Top 10 보안 감사 |
| `/auto map` | 코드베이스 구조 분석 |
| `/auto sync SPEC-ID` | 구현 후 문서 동기화 |
| `auto sync verify [--spec SPEC-ID] [--strict]` | 읽기 전용 fail-closed 커밋 계획. 단일 리포는 그룹 1개, 멀티 리포 워크스페이스는 Phase A/B |
| `auto spec change SPEC-ID --class small_ui --ac AC-001 --surface path` | 저위험 작업용 짧은 변경 계약. 고위험 클래스는 정식 SPEC으로 escalate |
| `auto spec gates SPEC-ID --change-class small_ui --json` | gate별 `required/not_applicable/blocked` 판정과 이유 |
| `auto spec review SPEC-ID --single-pass` | provider round 1회. receipt에 `loop_status`·`blocking_reasons` 기록 |
| `/auto dev "설명"` | 풀 파워: plan(--multi --ultrathink) → go(--team --loop) → sync |
| `/auto setup` | 프로젝트 컨텍스트 문서 생성/업데이트 |
| `/auto stale` | 오래된 결정 및 패턴 감지 |
| `/auto why "질문"` | 의사결정 근거 조회 |
| `/auto experiment` | 자율 실험 루프 (메트릭 기반 반복) |
| `/auto test` | 프로젝트 E2E 시나리오 실행 |
| `/auto go SPEC-ID --continue` | 중단된 파이프라인 체크포인트에서 재개 |
| `/auto browse` | 브라우저 자동화 — 열기, 스냅샷, 클릭, 검증 |
| `/auto idea "설명"` | 멀티 프로바이더 브레인스토밍 + ICE 채점 |
| `/auto canary` | 배포 후 헬스 체크 (빌드 + E2E + 브라우저) |

</details>

---

## ⚙️ 설정

<details>
<summary><strong><code>autopus.yaml</code></strong> — 모든 것을 위한 단일 설정</summary>

```yaml
mode: full                    # full 또는 lite
project_name: my-project
platforms:
  - claude-code

architecture:
  auto_generate: true
  enforce: true

lore:
  enabled: true
  required_trailers: [Why, Decision]
  stale_threshold_days: 90

spec:
  review_gate:
    enabled: true
    strategy: debate
    providers: [claude, gemini]
    judge: claude

methodology:
  mode: tdd
  enforce: true

orchestra:
  enabled: true
  default_strategy: consensus
  providers:
    claude:
      binary: claude
    codex:
      binary: codex
    gemini:
      binary: agy
    opencode:
      binary: opencode
```

</details>

---

## 🏗️ 아키텍처

```
autopus-adk/
├── cmd/auto/           # 진입점
├── internal/cli/       # Cobra 명령어 28개 (서브커맨드 포함 110개 이상)
├── pkg/
│   ├── adapter/        # 플랫폼 어댑터 4개 (Claude, Codex, Gemini, OpenCode)
│   ├── arch/           # 아키텍처 분석 + 규칙 강제
│   ├── browse/         # 브라우저 자동화 백엔드 (cmux/agent-browser 라우팅)
│   ├── config/         # 설정 스키마 + YAML 로딩
│   ├── constraint/     # 안티패턴 스캔
│   ├── content/        # 에이전트/스킬/훅/프로필 생성 + 스킬 활성화
│   ├── cost/           # 토큰 기반 비용 추정
│   ├── detect/         # 플랫폼/프레임워크/권한 감지
│   ├── e2e/            # E2E 시나리오 생성, 실행, 검증
│   ├── experiment/     # 자율 실험 루프 (메트릭 실행, 서킷 브레이커)
│   ├── issue/          # 자동 이슈 리포터 (컨텍스트 수집, 정제)
│   ├── lore/           # 의사결정 추적 (9-trailer 프로토콜)
│   ├── lsp/            # LSP 연동
│   ├── orchestra/      # 멀티 모델 오케스트레이션 (전략 4개 + brainstorm + 인터랙티브 토론 + 훅)
│   ├── pipeline/       # 파이프라인 상태 지속성 + 체크포인트 + 팀 모니터
│   ├── search/         # 지식 검색 (Context7/Exa) + 해시 기반 검색
│   ├── selfupdate/     # CLI 바이너리 자동 업데이트 (게시자 서명, SHA256, 트랜잭션 교체)
│   ├── setup/          # 프로젝트 문서 생성 + 검증
│   ├── sigmap/         # AST 기반 API 시그니처 추출 (Go + TypeScript)
│   ├── spec/           # EARS 요구사항 파싱/검증
│   ├── telemetry/      # 파이프라인 텔레메트리 (JSONL 이벤트 기록)
│   ├── template/       # Go 템플릿 렌더링
│   ├── terminal/       # 터미널 멀티플렉서 어댑터 (cmux, tmux, plain)
│   └── version/        # 빌드 메타데이터
├── templates/          # 플랫폼별 템플릿
├── content/            # 임베디드 콘텐츠 (16개 에이전트, 53개 스킬)
└── configs/            # 기본 설정
```

---

## 🔒 보안

### 🛡️ 패키지 공급망 공격 방어

> *"월 수천만 다운로드의 인기 Python 패키지에 악성 코드가 삽입됐습니다. 단순 `pip install`만으로 SSH 키, AWS 자격증명, DB 비밀번호를 탈취할 수 있었죠 — 직접 설치한 패키지가 아니라, 의존성 트리 깊숙한 곳에서."* — [Andrej Karpathy](https://x.com/karpathy)

AI 코딩 환경은 이 문제를 더 심각하게 만듭니다: 에이전트가 패키지를 자동 설치하고, 의존성 트리를 확장하고, 코드를 실행합니다 — 전부 사람의 검토 없이. **Autopus는 파이프라인 자체에 방어를 내장합니다.**

#### Autopus가 개발 워크플로우를 보호하는 방법

| 레이어 | 보호 내용 | 방법 |
|--------|----------|------|
| **파이프라인 게이트** | 모든 `/auto go`에서 의존성 취약점 스캔 | Security Auditor 에이전트가 Phase 4에서 `govulncheck ./...` 실행 |
| **시크릿 탐지** | 하드코딩된 자격증명을 커밋 전 탐지 | `gitleaks detect`로 변경 파일 전체 스캔 |
| **의존성 감사** | 의존성 트리의 알려진 CVE 탐지 | Go 프로젝트: `go list -m -json all \| nancy sleuth` |
| **락 파일 무결성** | 체크섬 검증된 의존성 | Go의 `go.sum`으로 재현 가능하고 변조 불가능한 빌드 보장 |
| **OWASP Top 10** | 인젝션, 인증 우회, SSRF 등 체계적 검사 | Security Auditor가 A01–A10 전체 커버 |
| **AI 에이전트 가드레일** | 에이전트가 패키지를 맹목적으로 설치 불가 | 하네스 규칙이 에이전트 행동 제약; 보안 게이트 FAIL 시 배포 차단 |

#### Go 이외 프로젝트

Autopus가 Python, Node.js 등 다른 생태계를 관리할 때도 동일한 원칙 적용:

```yaml
# autopus.yaml — 생태계별 보안 스캔 설정
security:
  scanners:
    go: "govulncheck ./..."
    python: "pip-audit && safety check"
    node: "npm audit --audit-level=high"
```

**하네스가 강제하는 모범 사례:**
- **버전 고정** — 모든 의존성을 정확한 버전으로 잠금 (`go.sum`, `package-lock.json`, `requirements.txt`)
- **의존성 최소화** — 기존 코드와 네이티브 기능을 먼저 활용하고, 필요한 의존성이나 추상화만 추가합니다
- **격리** — 지원되는 네이티브 격리를 사용합니다. 작업 디렉터리를 공유할 때는 쓰기 소유권을 분리하며, 대화 분기만으로 파일이 격리되지는 않습니다.
- **맹목적 설치 금지** — Security Auditor 에이전트가 미확인/미검증 패키지를 코드베이스 진입 전 차단

### 바이너리 배포 안전성

v0.50.73부터 모든 바이너리 릴리즈에는 **SHA256 체크섬**(`checksums.txt`)과 ECDSA P-256
게시자 서명 envelope(`checksums.txt.signatures`)가 포함됩니다. 현재 POSIX 설치기, Windows 설치기,
그리고 v0.50.73부터 포함된 self-updater는 아카이브를 다운로드하거나 압축 해제하기 전에
체크섬 매니페스트의 게시자 서명을 인증하고, 이어서 아카이브 체크섬을 검증합니다. 체크섬만
검증하는 설치 방식으로 돌아가지 않으며, 설치기는 v0.50.72 이하의 unsigned 릴리스를 거부합니다.

POSIX 경로에는 OpenSSL과 `sha256sum` 또는 `shasum`이 필요하며, 검증 도구가 없으면 안전하게
중단합니다. Windows 경로는 운영체제의 CNG 구현과 `Get-FileHash`를 사용합니다.

**권장: 설치 전 스크립트 확인**

```bash
# 1. 스크립트를 먼저 다운로드 — 실행 전에 내용 확인
curl -sSfL https://raw.githubusercontent.com/autopus-ai/autopus-adk/main/install.sh -o install.sh
less install.sh          # 무엇을 하는지 확인
sh install.sh            # 확인 후에만 실행
```

**또는 수동 검증:**

```bash
# 바이너리 + 체크섬 별도 다운로드
VERSION=$(curl -s https://api.github.com/repos/autopus-ai/autopus-adk/releases/latest | grep tag_name | sed 's/.*"v\(.*\)".*/\1/')
curl -LO "https://github.com/autopus-ai/autopus-adk/releases/download/v${VERSION}/autopus-adk_${VERSION}_$(uname -s | tr A-Z a-z)_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz"
curl -LO "https://github.com/autopus-ai/autopus-adk/releases/download/v${VERSION}/checksums.txt"

# SHA256 무결성만 검증(게시자 인증은 하지 않음)
shasum -a 256 -c checksums.txt --ignore-missing
```

게시자까지 인증하려면 내용을 확인한 현재 설치기 또는 v0.50.73부터 포함된 self-updater를
사용하세요. 두 경로 모두 신뢰할 수 있는 게시자 서명이 통과해야 `checksums.txt`를 수용합니다.

신뢰 경계: 한 줄 설치기는 저장소 `main` 브랜치에서 제공됩니다. 설치기나 업데이터의 신뢰된
바이트가 유지되는 동안 게시자 서명은 GitHub와 CDN이 전달하는 릴리스 자산의 변조를
방어합니다. 그러나 설치기 bootstrap 자체를 인증하지는 않습니다. 저장소 `main`이나 raw-main
전달 경로가 침해되면 공격자는 릴리스 자산을 장악하지 않고도 검증기·핀·다운로드 대상을
바꿀 수 있습니다. 독립적으로 고정된 설치기 origin은 아직 제공하지 않습니다.

### 하지 않는 것들

- 텔레메트리나 분석 데이터 수집 없음
- 명시적 명령(`orchestra`, `search`, `update --self`) 외 네트워크 호출 없음
- AI 프로바이더 API 키 접근 없음 — Autopus는 API가 아닌 CLI 도구를 오케스트레이션

---

## 🤝 기여하기

Autopus-ADK는 MIT 라이선스의 오픈 소스입니다. PR 환영합니다!

```bash
make test       # 레이스 디텍션 테스트 실행
make lint       # go vet 실행
make coverage   # 커버리지 리포트 생성
```

---

<div align="center">

**🐙 Autopus** — 에이전트로. 에이전트에 의해. 에이전트를 위해.

</div>
