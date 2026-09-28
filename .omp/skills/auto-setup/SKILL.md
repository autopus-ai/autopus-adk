---
name: auto-setup
description: 프로젝트 컨텍스트 생성 — 코드베이스를 분석하고 ARCHITECTURE.md 및 .autopus/project 문서를 생성합니다
compatibility: omp
---

# auto-setup — 프로젝트 컨텍스트 생성 스킬

## OMP Invocation

- `/auto setup ...`
- `/auto-setup ...`
- Load detail skill `auto-setup` for either entrypoint.


**프로젝트**: autopus-adk | **모드**: full

## 설명

프로젝트 구조와 기술 스택을 분석해 세션 간 지속되는 컨텍스트 문서를 생성하거나 갱신합니다.
이 스킬은 첫 세션 온보딩, 오래된 프로젝트 재진입, `/auto plan`/`/auto sync` 전 컨텍스트 복구에 사용합니다.

## Workspace Folder Boundary

ADK는 workspace folder profile의 소유자가 아니라 workspace 또는 repo checkout 안에 설치되는 harness/execution layer입니다. `auto setup`은 루트 역할과 추적 정책을 문서화하지만, canonical Knowledge Hub의 folder identity, promotion, admission 정책을 ADK generated surface로 대체하지 않습니다.

Generated/runtime/harness surfaces are excluded from canonical Knowledge Hub indexing unless they are explicitly human-managed project/spec documents:
- Excluded: `.autopus/runtime/**`, `.autopus/qa/{runs,cache,gui,feedback,evidence,releases}/**` raw artifacts, `.autopus/context/signatures.md`, `.autopus/*-manifest.json`, `.autopus/plugins/**`, `.autopus/orchestra/**`, `.autopus/brainstorms/**`, `.autopus/design/{imports,verify}/**`, `.autopus/canary/**`, `.codex/**`, `.claude/**`, `.gemini/**`, `.opencode/**`, `.agents/{skills,plugins,commands}/**`, `.agents/hooks.json`, `.symphony/artifacts/**`, `config.toml`
- Indexable local docs: `.autopus/project/**`, `.autopus/specs/**`, `.autopus/vault/**`, `README.md`, `docs/**`
- Candidate/projection only: `.autopus/inbox/**` requires promotion; sanitized `.autopus/learnings/pipeline.jsonl` rows remain ADK Decision/Quality Index projection evidence.

## 사용법

```bash
/auto setup
/auto setup --auto
```

## Codex 기본 실행 모델

- Codex에서는 `setup`도 `task` batch 기반 subagent-first 로 진행합니다.
- Step 1의 전체 코드베이스 분석은 반드시 `explorer` 서브에이전트로 먼저 수행합니다.
- 메인 세션은 결과를 검토한 뒤 `ARCHITECTURE.md`와 `.autopus/project/*` 산출물을 최종 정리하고 저장합니다.
- 현재 Codex 런타임 정책이 암묵적 `task` batch 호출을 제한하면, 하네스 기본값과 제약을 명시적으로 설명한 뒤 사용자에게 서브에이전트 진행 여부 또는 `--solo` 성격의 단일 세션 진행을 확인받습니다.

전체 라우팅 규칙은 `/auto setup` 라우터를 우선합니다.

## 실행 순서

### [REQUIRED] Step 1: 코드베이스 분석

IMPORTANT: 이 단계는 반드시 `explorer` 서브에이전트를 먼저 호출해야 합니다. 분석 없이 Step 2로 건너뛰면 안 됩니다.

`explorer` 에이전트가 최소한 아래 정보를 반환하게 합니다.
- 디렉터리 구조와 패키지 역할
- 루트 저장소 역할(product repo / monorepo root / meta workspace)과 nested git repo 경계
- 기술 스택(언어, 프레임워크, 빌드 도구)
- 엔트리포인트와 주요 공개 API
- 아키텍처 패턴과 도메인 경계
- 사용자 접점이 있는 라우트, 명령, 화면
- generated surface / runtime artifact 경로와 source-of-truth repo
- ADK가 harness layer이며 folder profile owner가 아니라는 경계
- canonical Knowledge Hub에서 제외되는 generated/runtime/harness surface와 indexable/candidate/projection-only 경로
- QA/Journey Pack 대상 리포, `auto qa init --project-dir <repo>` 실행 명령, 루트 `.autopus/qa/**`가 project-owned인지 runtime/generated인지

> **POST-STEP**: explorer 결과를 받으면 Step 2로 진행합니다. 탐색 요약 없이 완료 안내를 출력하지 않습니다.

### [REQUIRED] Step 2: `ARCHITECTURE.md` 생성 또는 갱신

Explorer 분석을 바탕으로 루트 `ARCHITECTURE.md`를 작성합니다.

포함 항목:
- 핵심 도메인과 레이어
- 의존성 흐름 및 경계
- 진입점과 주요 데이터 흐름
- 관찰된 규칙 위반 또는 위험 지점

> **POST-STEP**: `ARCHITECTURE.md` 저장 후 Step 3으로 진행합니다.

### [REQUIRED] Step 3: `.autopus/project/` 핵심 문서 생성 또는 갱신

다음 4개 파일을 explorer 결과 기준으로 작성합니다.
- `product.md` — 프로젝트 목적, 핵심 기능, 주요 유스케이스, 모드
- `structure.md` — 디렉터리 구조, 패키지 역할, 엔트리포인트, 주요 파일 분포
- `tech.md` — 언어, 프레임워크, 빌드/테스트 도구, 아키텍처 패턴
- `workspace.md` — 루트 저장소 역할, nested repo 경계, generated/runtime 경로, 추적/커밋 정책, source-of-truth 위치, QA/Journey Pack 대상 리포와 `auto qa init --project-dir <repo>` 명령

규칙:
- 추측보다 실제 코드베이스 증거를 우선합니다.
- monorepo면 workspace/module 경계를 드러냅니다.
- meta workspace면 keep 대상과 generated/runtime drop 대상을 명시합니다.
- meta workspace면 root `.autopus/qa/**`를 제품 Journey Pack 위치로 쓰지 말고, 대상 리포별 `--project-dir` 명령을 문서에 남깁니다.
- 기존 파일이 있어도 현재 상태를 반영해 갱신합니다.
- `structure.md`는 package family와 경계만 요약합니다. 개별 파일을 전수 나열하지 말고 18,000 bytes 이하를 목표로 하며, doctor warning 경계인 20,000 bytes를 넘기지 않습니다.

> **POST-STEP**: `.autopus/project/` 4개 파일 저장 후 Step 4로 진행합니다.

### [REQUIRED] Step 4: `scenarios.md` 생성 또는 갱신

`.autopus/project/scenarios.md`에 사용자 관점 E2E 시나리오를 정리합니다.

프로젝트 유형별로 다음을 추출합니다.
- CLI: 주요 명령, 플래그, 정상/실패 경로
- API: 메서드, 경로, 인증/사전조건, 기대 응답
- Frontend: 핵심 페이지, 사용자 동작, 검증 포인트
- Library: 공개 API 진입점과 대표 사용 시나리오

각 시나리오는 최소한 아래를 포함합니다.
- **Command/Action**
- **Precondition**
- **Expect**
- **Verify**
- **Status**

> **POST-STEP**: `scenarios.md` 저장 후 Step 5로 진행합니다.

### [REQUIRED] Step 5: `canary.md` 생성 또는 갱신

`.autopus/project/canary.md`에 빌드/엔드포인트/브라우저 건강 검진 대상을 정의합니다.

우선 탐지 대상:
- `Dockerfile`, `docker-compose.yml`
- `railway.json`, `vercel.json`, `fly.toml`, `render.yaml`
- `k8s/`, `helm/`
- `.github/workflows/`
- 소스 코드의 health/status endpoint
- 프론트엔드 라우트와 대표 브라우저 타깃

최소 산출물:
- build health check
- endpoint health check 또는 해당 없음
- browser health check 또는 해당 없음
- detection source 요약

> **POST-STEP**: `canary.md` 저장 후 Pre-Completion Verification으로 진행합니다.

### [REQUIRED] Pre-Completion Verification

완료 메시지 전에 아래 항목이 모두 충족됐는지 확인합니다.

- [ ] Step 1: explorer 서브에이전트 호출 완료
- [ ] Step 2: `ARCHITECTURE.md` 저장 완료
- [ ] Step 3: `.autopus/project/product.md`, `structure.md`, `tech.md`, `workspace.md` 저장 완료
- [ ] Step 4: `.autopus/project/scenarios.md` 저장 완료
- [ ] Step 5: `.autopus/project/canary.md` 저장 완료

하나라도 비어 있으면 완료 메시지를 출력하지 말고 해당 단계로 돌아갑니다.

## Completion Message

```text
Project context documents have been generated/updated.
- ARCHITECTURE.md
- .autopus/project/product.md
- .autopus/project/structure.md
- .autopus/project/tech.md
- .autopus/project/workspace.md
- .autopus/project/scenarios.md
- .autopus/project/canary.md
Context will be loaded automatically on the next session.
```

## First Win Guidance

완료 후 현재 상태에 맞는 다음 액션을 최대 3개까지 추천합니다.

우선순위:
1. 항상 표시 가능: `/auto review`, `/auto idea`
2. SPEC이 없으면: `/auto plan "기능 설명"`
3. draft SPEC이 있으면: `/auto go {SPEC-ID}` 또는 `auto spec review {SPEC-ID}`

출력 형식:

```text
🐙 바로 해볼 수 있는 것 ──────────────
1. {command} — {one-line value}
```

## 출력

- `ARCHITECTURE.md`
- `.autopus/project/product.md`
- `.autopus/project/structure.md`
- `.autopus/project/tech.md`
- `.autopus/project/workspace.md`
- `.autopus/project/scenarios.md`
- `.autopus/project/canary.md`
