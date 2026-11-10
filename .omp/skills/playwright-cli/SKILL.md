---
name: playwright-cli
description: playwright-cli를 사용한 브라우저 자동화 — 폼 입력, 스크린샷, 테스트 생성, 세션 관리
compatibility: omp
---

# Browser Automation with playwright-cli

## Quick Start

```bash
playwright-cli open                         # open new browser
playwright-cli goto https://playwright.dev  # navigate
playwright-cli snapshot                     # get element refs
playwright-cli click e15                    # interact with ref
playwright-cli type "search query"          # type text
playwright-cli screenshot                   # capture screenshot
playwright-cli close                        # close browser
```

## Commands

### Core

```bash
playwright-cli open [url]
playwright-cli goto <url>
playwright-cli type "text"
playwright-cli click e3
playwright-cli dblclick e7
playwright-cli fill e5 "user@example.com"
playwright-cli drag e2 e8
playwright-cli hover e4
playwright-cli select e9 "option-value"
playwright-cli upload ./document.pdf
playwright-cli check e12
playwright-cli uncheck e12
playwright-cli snapshot
playwright-cli snapshot --filename=after-click.yaml
playwright-cli eval "document.title"
playwright-cli eval "el => el.textContent" e5
playwright-cli dialog-accept ["text"]
playwright-cli dialog-dismiss
playwright-cli resize 1920 1080
playwright-cli close
```

### Navigation

```bash
playwright-cli go-back
playwright-cli go-forward
playwright-cli reload
```

### Keyboard & Mouse

```bash
playwright-cli press Enter
playwright-cli press ArrowDown
playwright-cli keydown Shift
playwright-cli keyup Shift
playwright-cli mousemove 150 300
playwright-cli mousedown [right]
playwright-cli mouseup [right]
playwright-cli mousewheel 0 100
```

### Screenshot & PDF

```bash
playwright-cli screenshot                    # full page
playwright-cli screenshot e5                 # element only
playwright-cli screenshot --filename=page.png
playwright-cli pdf --filename=page.pdf
```

### Tabs

```bash
playwright-cli tab-list
playwright-cli tab-new [url]
playwright-cli tab-close [n]
playwright-cli tab-select 0
```

### Storage & Cookies

```bash
# State save/load
playwright-cli state-save [auth.json]
playwright-cli state-load auth.json

# Cookies
playwright-cli cookie-list [--domain=example.com]
playwright-cli cookie-get <name>
playwright-cli cookie-set <name> <value> [--domain --httpOnly --secure]
playwright-cli cookie-delete <name>
playwright-cli cookie-clear

# LocalStorage
playwright-cli localstorage-list
playwright-cli localstorage-get <key>
playwright-cli localstorage-set <key> <value>
playwright-cli localstorage-delete <key>
playwright-cli localstorage-clear

# SessionStorage
playwright-cli sessionstorage-list
playwright-cli sessionstorage-get <key>
playwright-cli sessionstorage-set <key> <value>
playwright-cli sessionstorage-delete <key>
playwright-cli sessionstorage-clear
```

### Network Mocking

```bash
playwright-cli route "**/*.jpg" --status=404
playwright-cli route "https://api.example.com/**" --body='{"mock": true}'
playwright-cli route-list
playwright-cli unroute "**/*.jpg"
playwright-cli unroute
```

### DevTools

```bash
playwright-cli console [warning]
playwright-cli network
playwright-cli run-code "async page => await page.context().grantPermissions(['geolocation'])"
playwright-cli tracing-start
playwright-cli tracing-stop
playwright-cli video-start
playwright-cli video-stop video.webm
```

### Browser Sessions

```bash
playwright-cli -s=mysession open example.com --persistent
playwright-cli -s=mysession click e6
playwright-cli -s=mysession close
playwright-cli -s=mysession delete-data
playwright-cli list                          # list sessions
playwright-cli close-all                     # close all
playwright-cli kill-all                      # force kill
```

### Configuration

```bash
playwright-cli open --browser=chrome         # chrome/firefox/webkit/msedge
playwright-cli open --extension              # connect via extension
playwright-cli open --persistent             # persistent profile
playwright-cli open --profile=/path/to/profile
playwright-cli open --config=my-config.json
playwright-cli delete-data                   # delete user data
```

### Install

```bash
playwright-cli install --skills
playwright-cli install-browser
```

## Workflow: Snapshot-Act-Verify

```bash
# 1. Open & snapshot
playwright-cli open https://example.com/form
playwright-cli snapshot

# 2. Interact using element refs (e1, e2, e3...)
playwright-cli fill e1 "user@example.com"
playwright-cli fill e2 "password123"
playwright-cli click e3

# 3. Verify result
playwright-cli snapshot
playwright-cli screenshot --filename=result.png
playwright-cli close
```

## playwright-cli vs agent-browser

| 기능 | playwright-cli | agent-browser |
|------|---------------|---------------|
| Ref 형식 | `e1`, `e2` | `@e1`, `@e2` |
| Snapshot | YAML 형식 | 접근성 트리 |
| Session | 명시적 세션 관리 (`-s=`) | 자동 세션 유지 |
| DevTools | tracing, video, console, network | HAR, network route |
| 브라우저 선택 | chrome/firefox/webkit/msedge | Chrome only |
| State 관리 | state-save/load (auth.json) | cookies set/get |
| 강점 | 테스트 생성, 멀티 브라우저, DevTools | AI 최적화 JSON 출력, Rust 성능 |

둘 다 사용 가능. 테스트 생성/멀티 브라우저는 playwright-cli, AI 에이전트 직접 조작은 agent-browser 권장.
