package diskgate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestClassifyCPU(t *testing.T) {
	t.Parallel()
	heavy := map[string]string{
		"go test ./...":                             "go test",
		"go install ./cmd/x":                        "go install",
		"go vet ./...":                              "go vet",
		"cargo clippy --all":                        "cargo clippy",
		"pnpm test":                                 "pnpm test",
		"pnpm -r lint":                              "pnpm lint",
		"pnpm --filter web typecheck":               "pnpm typecheck",
		"npm --prefix web run lint":                 "npm run lint",
		"npm run lint:fix":                          "npm run lint:fix",
		"pnpm exec vitest run":                      "pnpm exec vitest",
		"pnpm vitest":                               "pnpm vitest",
		"npx eslint .":                              "npx eslint",
		"eslint . --max-warnings=0":                 "eslint",
		"pytest -q":                                 "pytest",
		"uv run pytest -x":                          "pytest",
		"python3 -m pytest":                         "python3 -m pytest",
		"golangci-lint run ./...":                   "golangci-lint run",
		"auto disk lease --need 8 -- go test ./...": "go test",
		"cd web && CI=1 vitest run":                 "vitest",
		"bash -c 'pnpm -r lint'":                    "pnpm lint",
		"npm run build:docs":                        "npm run build:docs",
	}
	for line, match := range heavy {
		v := ClassifyCPU(line)
		assert.True(t, v.Heavy, line)
		assert.Equal(t, match, v.Match, line)
	}
	for _, line := range []string{
		"auto disk lease --cpu 4 -- go test ./...", "auto disk lease --cpu=4 --need 8 -- pnpm test", "auto disk status",
		"npm i left-pad", "pnpm install", "uv sync", "python -m pip install x", "git worktree add ../w",
		"tsc --version", "eslint --help", "git status", "pnpm run dev", "heavy-lock.sh x go test",
	} {
		assert.False(t, ClassifyCPU(line).Heavy, line)
	}
	assert.Equal(t, "make release", ClassifyCPU("make release", "make release").Match, "extra prefixes apply")
}
