---
name: auto-canary
description: 배포 검증 — build, E2E, 브라우저 건강 검진을 실행합니다
compatibility: omp
---

# auto-canary — 배포 검증 스킬

## OMP Invocation

- `/auto canary ...`
- `/auto-canary ...`
- Load detail skill `auto-canary` for either entrypoint.


## Context Profile: canary

- Required: core,canary
- Optional: learning
- Excluded: test,signature

**프로젝트**: autopus-adk | **모드**: full

## 설명

배포 후 건강 검진을 실행합니다. 빌드, E2E, 브라우저 검사를 순서대로 수행하고 PASS/WARN/FAIL 판정을 냅니다.

## Scope Boundary

- `auto canary`는 post-deploy 또는 staging smoke/status gate입니다. 목표는 현재 빌드, endpoint, browser surface가 운영 가능한지 빠르게 판정하고 `.autopus/canary/latest.json`에 최신 상태를 남기는 것입니다.
- QAMESH manifest, redacted artifact, run index, repair prompt bundle이 필요하거나 browser/desktop/mobile 사용자 journey를 evidence로 남겨야 하면 `auto qa ...`를 사용합니다.
- `auto qa release` 안의 `canary-explicit` lane은 명시된 post-deploy smoke Journey Pack을 release gate에 연결하는 bridge lane입니다. Canary 자체가 전체 QAMESH QA mesh를 대체하지 않습니다.

## 사용법

```bash
/auto canary
/auto canary --url https://example.com
/auto canary --watch 5m
/auto canary --compare abc123
```

## 플래그

| Flag | Description |
|------|-------------|
| `--url <url>` | 브라우저 검진 대상 URL. `canary.md`의 browser 타깃보다 우선합니다. |
| `--watch <interval>` | 지정 주기로 반복 실행합니다. 기본 5m, 최대 30m. |
| `--compare <commit>` | 저장된 이전 실행 결과와 비교합니다. |

## 실행 순서

1. `.autopus/project/canary.md`에 선언된 build/smoke 검증
2. canary 설정에 선언된 endpoint/browser health check 실행
3. endpoint/browser 건강 검진 수행
4. PASS/WARN/FAIL 판정
5. `.autopus/canary/latest.json` 에 결과 저장

## Codex Notes

- 전체 파이프라인 규칙과 검증 체크리스트는 `/auto canary` 라우터 본문을 우선합니다.
- `canary.md`가 없으면 설정 부재를 명시하고 `/auto setup` 안내를 표시합니다.
- `--compare` 값은 commit SHA 형식 검증 후 사용합니다.

## 판정 기준

- **PASS**: 빌드 OK + 전체 E2E 통과 + 치명적 브라우저 오류 없음
- **WARN**: 빌드 OK + 일부 E2E 실패 또는 비치명적 브라우저 경고
- **FAIL**: 빌드 실패 또는 치명적 E2E/브라우저 오류
