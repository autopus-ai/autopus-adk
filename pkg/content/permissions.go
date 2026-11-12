package content

import (
	"path/filepath"

	"github.com/insajin/autopus-adk/pkg/adapter"
	"github.com/insajin/autopus-adk/pkg/config"
)

// defaultReadOnlyAllow is the platform-neutral allowlist every project gets.
// Shell entries are limited to commands that only read project or repository
// state; anything that can run arbitrary code or reach the network on the
// caller's behalf is left to the user's own approval or ExtraAllow.
var defaultReadOnlyAllow = []string{
	"Bash(auto status:*)",
	"Bash(auto doctor)",
	"Bash(auto check:*)",
	"Bash(auto spec validate:*)",
	"Bash(auto lore validate:*)",
	"Bash(auto version:*)",
	"Bash(git status:*)",
	"Bash(git diff:*)",
	"Bash(git log:*)",
	"Bash(git show:*)",
	"Bash(git branch --show-current)",
	"Bash(git branch --list:*)",
	"Bash(git rev-parse:*)",
	"Bash(git ls-files:*)",
	"Bash(ls:*)",
	"Bash(cat:*)",
	"Bash(grep:*)",
	"Bash(wc:*)",
	"Bash(sort:*)",
	"Bash(mkdir:*)",
	"Bash(echo:*)",
	"Bash(gh pr view:*)",
	"Bash(gh pr checks:*)",
	"Bash(gh pr list:*)",
	"Bash(gh pr diff:*)",
	"Bash(gh run view:*)",
	"Bash(gh run list:*)",
	"Bash(gh issue view:*)",
	"Bash(gh issue list:*)",
	"mcp__sequential-thinking__sequentialthinking",
	"WebSearch",

	// Pipeline: current Claude orchestration tools.
	"Agent",
	"AskUserQuestion",
	"TaskCreate",
	"TaskGet",
	"TaskList",
	"TaskUpdate",
	"SendMessage",
	"Workflow",
	"ToolSearch",
}

// goDevLoopAllow covers the Go build/test loop. These run project code, which
// is the accepted boundary of a development loop; commands that fetch or run
// arbitrary modules (go run, go tool, go get, go install) are excluded.
var goDevLoopAllow = []string{
	"Bash(go build:*)", "Bash(go test:*)", "Bash(go vet:*)",
	"Bash(go mod:*)", "Bash(go version:*)",
	"Bash(go env:*)", "Bash(go clean:*)",
	"Bash(golangci-lint:*)", "Bash(gofmt:*)",
}

// nodeDevLoopAllow covers the package.json test/build/lint scripts only.
var nodeDevLoopAllow = []string{
	"Bash(npm test:*)", "Bash(npm run test:*)", "Bash(npm run build:*)",
	"Bash(npm run lint:*)", "Bash(npm run typecheck:*)",
	"Bash(pnpm test:*)", "Bash(pnpm run test:*)", "Bash(pnpm run build:*)",
	"Bash(pnpm run lint:*)", "Bash(pnpm run typecheck:*)",
	"Bash(yarn test:*)", "Bash(yarn run test:*)", "Bash(yarn build:*)",
	"Bash(yarn lint:*)", "Bash(yarn typecheck:*)",
}

// DetectPermissions는 프로젝트 루트를 분석하여 기본 권한을 생성한다.
func DetectPermissions(projectRoot string, extra config.PermissionsConf) *adapter.PermissionSet {
	allow := append([]string{}, defaultReadOnlyAllow...)
	if fileExists(filepath.Join(projectRoot, "go.mod")) {
		allow = append(allow, goDevLoopAllow...)
	}
	if fileExists(filepath.Join(projectRoot, "package.json")) {
		allow = append(allow, nodeDevLoopAllow...)
	}

	// Merge extra permissions from autopus.yaml
	allow = append(allow, extra.ExtraAllow...)

	var deny []string
	deny = append(deny, extra.ExtraDeny...)

	return &adapter.PermissionSet{
		Allow: allow,
		Deny:  deny,
	}
}
