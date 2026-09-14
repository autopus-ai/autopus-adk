BINARY := auto
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
DATE    ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS := -ldflags "-s -w -X github.com/insajin/autopus-adk/pkg/version.version=$(VERSION) -X github.com/insajin/autopus-adk/pkg/version.commit=$(COMMIT) -X github.com/insajin/autopus-adk/pkg/version.date=$(DATE)"
# GOPATH is a `go env` value, not an environment variable, so make never had it:
# `$(GOPATH)/bin` expanded to `/bin` and `make install` died on SIP instead of
# installing. Ask the toolchain, and let INSTALL_DIR name another destination.
GOPATH      ?= $(shell go env GOPATH)
INSTALL_DIR ?= $(GOPATH)/bin

.PHONY: build test test-unit test-integration test-e2e test-all update-golden lint clean install generate-templates

build:
	go build $(LDFLAGS) -o bin/$(BINARY) ./cmd/auto

# Go의 기본 테스트 타임아웃은 10분이다. 이 저장소에서 internal/cli와
# internal/companionmanifest는 단독으로도 각각 7분을 넘기므로, 타임아웃 없는
# `go test ./...`는 병렬 경합이 얹히는 순간 항상 실패한다. CI는 레인마다
# -timeout을 명시해서 (ci.yaml의 5m/8m/15m/20m/30m) 통과해 왔고, 그래서
# 로컬 진입점만 깨진 상태로 오래 남아 있었다.
UNIT_TIMEOUT        ?= 20m
INTEGRATION_TIMEOUT ?= 30m
ISOLATED_TIMEOUT    ?= 10m

# 프로세스가 무거운 테스트는 공유 Go 패키지 스케줄러에서 떼어낸다. ci.yaml이
# TestReleaseHardeningBashContract 에 대해 이미 세운 규칙이고
# (@AX:REASON: "Keep the process-heavy Bash release fixture off the shared Go
# package scheduler while preserving its race gate"), 같은 이유가 나머지에도
# 그대로 적용된다: 이들은 프로세스 그룹 종료, 상속된 파이프 배수, 실제 바이너리
# 프로브를 재므로 스케줄러 지연을 측정 대상에 섞는다. 코어 수만큼 패키지를
# 병렬로 돌리면서 -race 계측까지 얹으면 예산이 아니라 부하를 재게 된다.
# 예산을 키우는 방식은 이 저장소에서 이미 세 번 실패했다. 격리는 억제가
# 아니다 - 같은 테스트를 같은 -race 게이트로, 다만 -p 1 로 돌린다.
#
# TestDetect 와 TestProbeOMPIdentity_ 는 pkg/detect 의 PATH 기반 실제 CLI
# 프로브를 함께 격리한다. 이름을 하나씩 추가하는 방식은 세 번 연속 다음
# 테스트에서 실패했다. 순수한 두어 개가 함께 격리되는 비용은 없다.
#
# TestExecuteDesktopObservation_ 은 실제 provider subprocess 를 2초 고정 시계로
# 잰다 (desktop_observe_resolver_test.go 의 "strict two-second subprocess
# integration clock"). 같은 패키지의 GUI capture 테스트가 npm/node 프로세스를
# 띄우기 시작하면서 공유 스케줄러에서는 그 2초가 예산이 아니라 부하 측정이
# 된다. 억제가 아니라 격리다 - 같은 테스트를 -p 1 로 그대로 돌린다.
#
# 실측한 근본 원인(2026-09-05, pkg/processprobe 를 K=24 동시 실행으로 재현):
# macOS 는 처음 실행되는 실행 파일마다 최초 실행 평가(syspolicyd/XProtect)를
# 시스템 전역으로 직렬화해 파일당 약 190ms 를 물린다. 테스트마다 t.TempDir()
# 에 새 #!/bin/sh fixture 를 쓰고 그대로 exec 하면 그 큐 대기가 시계에
# 들어간다. 스크립트를 `/bin/sh <script>` 로 넘겨 이미 평가된 바이너리만
# exec 하면 비용이 사라진다 - pkg/processprobe 는 그렇게 고쳐 이 목록에서
# 뺐다. 코드가 fixture 경로를 직접 exec 해야 하는 나머지(가짜 omp/claude
# 바이너리 프로브)는 아직 격리한다.
#
# TestOrchestraBrainstorm_ spawns real shell providers per test and one case
# measures a 2s timeout kill; on the shared scheduler that clock reads load,
# not budget. Same rule, same -race gate, -p 1.
# These native-executable fixtures also measure process/pipe deadlines. They
# pass with the same -race assertions in isolation, not under competing launches.
PROCESS_HEAVY_TESTS ?= ^(TestReleaseHardeningBashContract|TestValidateReadiness|TestDetect|TestProbeOMPIdentity_|TestPOSIXInstaller|TestExecuteDesktopObservation_|TestOrchestraBrainstorm_|TestClaudeVersionGrandchildPipeReturnsWithinBound|TestCurrentOMPContextPromotionExecutableSHA256V3_LaunchThenReplace_DoesNotTrustReplacementPath)

test:
	go test -race -count=1 -timeout=$(INTEGRATION_TIMEOUT) -tags integration -skip '$(PROCESS_HEAVY_TESTS)' ./...
	go test -race -count=1 -timeout=$(ISOLATED_TIMEOUT) -tags integration -p 1 -run '$(PROCESS_HEAVY_TESTS)' ./...

test-unit:
	go test -race -count=1 -timeout=$(UNIT_TIMEOUT) -skip '$(PROCESS_HEAVY_TESTS)' ./...
	go test -race -count=1 -timeout=$(ISOLATED_TIMEOUT) -p 1 -run '$(PROCESS_HEAVY_TESTS)' ./...

test-integration:
	go test -race -count=1 -timeout=$(INTEGRATION_TIMEOUT) -tags integration -skip '$(PROCESS_HEAVY_TESTS)' ./...
	go test -race -count=1 -timeout=$(ISOLATED_TIMEOUT) -tags integration -p 1 -run '$(PROCESS_HEAVY_TESTS)' ./...

test-e2e: build
	AUTOPUS_TEST_BINARY=./bin/auto go test -race -count=1 -timeout=$(INTEGRATION_TIMEOUT) -tags e2e ./e2e/...

test-all:
	go test -race -count=1 -timeout=$(INTEGRATION_TIMEOUT) -tags 'integration e2e' -skip '$(PROCESS_HEAVY_TESTS)' ./...
	go test -race -count=1 -timeout=$(ISOLATED_TIMEOUT) -tags 'integration e2e' -p 1 -run '$(PROCESS_HEAVY_TESTS)' ./...

update-golden:
	go test -race -count=1 -tags e2e ./e2e/... -update

lint:
	go vet ./...

coverage:
	go test -race -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

generate-templates:
	go run ./cmd/generate-templates

clean:
	rm -rf bin/ coverage.out

install: build
	@mkdir -p $(INSTALL_DIR)
	cp bin/$(BINARY) $(INSTALL_DIR)/$(BINARY)
	@if [ "$$(uname -s)" = "Darwin" ]; then \
		xattr -cr $(INSTALL_DIR)/$(BINARY) 2>/dev/null || true; \
		codesign --force --sign - $(INSTALL_DIR)/$(BINARY) >/dev/null 2>&1 || true; \
	fi
# A signed release install under ~/.local/bin outranks $(INSTALL_DIR) on PATH, so
# this copy can succeed while every hook and CLI run keeps answering from the
# older released binary. Say so instead of letting the skew stay invisible.
	@active=$$(command -v $(BINARY) 2>/dev/null || true); \
	if [ -n "$$active" ] && [ "$$active" != "$(INSTALL_DIR)/$(BINARY)" ]; then \
		echo "warning: installed $(INSTALL_DIR)/$(BINARY), but PATH resolves $(BINARY) to $$active"; \
		echo "         that binary answers the git hooks; run with AUTOPUS_BIN=$(CURDIR)/bin/$(BINARY) to validate this build"; \
	fi
