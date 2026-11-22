package diskgate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Review findings on the first cut: compiling installs, verbose flags,
// option flags in front of the tool, wrappers, and suite-running tools.
func TestClassifyCPU_ReviewCases(t *testing.T) {
	t.Parallel()
	heavy := map[string]string{
		"mvn clean install":                           "mvn install",
		"./mvnw -B install":                           "mvn install",
		"gradle install":                              "gradle install",
		"pytest -v":                                   "pytest",
		"uv run pytest -v":                            "pytest",
		"uv run --frozen python -m pytest":            "python -m pytest",
		"uv run --with pytest-cov pytest":             "pytest",
		"uv --directory api run --python 3.12 mypy .": "mypy",
		"poetry run -C api pytest":                    "pytest",
		"bash -lc 'go test ./...'":                    "go test",
		"zsh -ec 'pnpm -r lint'":                      "pnpm lint",
		"/usr/bin/time go test ./...":                 "go test",
		"nice -10 go test ./...":                      "go test",
		"nice -n 5 pytest":                            "pytest",
		"find . -name '*.go' | xargs go test":         "go test",
		"xargs -n 1 -P 4 pytest":                      "pytest",
		"cargo nextest run":                           "cargo nextest run",
		"turbo run test":                              "turbo run test",
		"turbo run build --filter web":                "turbo run build",
		"turbo lint":                                  "turbo lint",
		"yarn workspace web test":                     "yarn test",
		"yarn workspace web lint":                     "yarn lint",
		"go tool golangci-lint run ./...":             "go tool golangci-lint run",
		"mypy .":                                      "mypy",
		"npx playwright test":                         "npx playwright",
		"npx turbo run test":                          "npx turbo run test",
		"pnpm exec tsc --noEmit":                      "pnpm exec tsc",
		"auto qa exec --all":                          "auto qa exec",
		"auto qa run":                                 "auto qa run",
		"auto qa loop":                                "auto qa loop",
		"auto qa mutate":                              "auto qa mutate",
		"auto checklist run":                          "auto checklist run",
		"auto workflow gate":                          "auto workflow gate",
		"auto disk lease --need 8 -- auto qa run":     "auto qa run",
	}
	for line, match := range heavy {
		v := ClassifyCPU(line)
		assert.True(t, v.Heavy, line)
		assert.Equal(t, match, v.Match, line)
	}
	for _, line := range []string{
		"npx playwright install", "npx playwright install --with-deps chromium", "pytest -V", "mypy --version",
		"eslint -v", "uv run --frozen python -c 'print(1)'", "turbo run dev", "yarn workspace web add left-pad",
		"auto qa intent", "auto checklist list", "auto disk lease --cpu 4 -- auto qa run", "bash -l",
		"nice -10 ls", "xargs rm", "auto workflow status", "cargo nextest list",
	} {
		assert.False(t, ClassifyCPU(line).Heavy, line)
	}
	for _, extra := range []string{"make ci", "just ci"} {
		assert.Equal(t, extra, ClassifyCPU(extra, extra).Match, "an extra command ending in ci is CPU work")
	}
}

// A here-document body is data: the command words in it are not run.
func TestClassify_HeredocBodyIsData(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		"cat > /tmp/m.txt <<'EOF'\ngo test ./...\nnpm ci\nEOF",
		"cat <<EOF > /tmp/m.txt\ngo build ./...\nEOF\nls",
		"cat <<\"END\"\npytest -q\nEND",
		"cat <<-EOF\n\tgo test ./...\n\tEOF\necho done",
		"git commit -F - <<EOF\nfix: go test passes\nEOF",
		"cat <<A <<B\ngo test\nA\ncargo build\nB",
		"cat<<EOF\ngo test\nEOF",
	} {
		assert.False(t, Classify(line).Heavy, line)
		assert.False(t, ClassifyCPU(line).Heavy, line)
	}
	for line, match := range map[string]string{
		"cat > /tmp/m.txt <<'EOF'\nignored\nEOF\ngo test ./...": "go test",
		"cat <<EOF && go build ./...\nbody\nEOF":                "go build",
		"cat <<-EOF\n\tbody\n\tEOF\ncargo build":                "cargo build",
		"cat <<<'go test' && go test ./...":                     "go test",
	} {
		v := Classify(line)
		assert.True(t, v.Heavy, line)
		assert.Equal(t, match, v.Match, line)
	}
}

// Follow-ups to the CPU gate review: help flags after an `auto` suite step,
// lookups of a tool's path, and here-documents a shell reads as its script.
func TestClassifyCPU_FollowupCases(t *testing.T) {
	t.Parallel()
	for _, line := range []string{
		"auto qa exec --help", "auto qa run --help", "auto workflow gate --help", "auto checklist run --help",
		"auto qa loop -h", "auto qa mutate --version",
		"command -v pytest", "command -v tsc", "command -V eslint", "command -pv mypy", "type pytest",
		"which tsc", "hash eslint", "command -v pytest && pytest --version",
		"ssh host <<EOF\ngo test ./...\nEOF", "cat <<EOF | ssh host\npytest\nEOF",
		"bash script.sh <<EOF\ngo test ./...\nEOF",
		"echo $((1<<4))", "echo $((1 << 4)) && ls",
	} {
		assert.False(t, ClassifyCPU(line).Heavy, line)
	}
	heavy := map[string]string{
		"auto qa exec --all":                       "auto qa exec",
		"command -v pytest && pytest":              "pytest",
		"command pytest -q":                        "pytest",
		"bash <<'EOF'\ngo test ./...\nEOF":         "go test",
		"sh -s <<EOF\npnpm test\nEOF":              "pnpm test",
		"cat <<'EOF' | bash\ncargo build\nEOF":     "cargo build",
		"cat <<EOF | sudo bash -s\npytest\nEOF":    "pytest",
		"zsh <<-EOF\n\ttsc --noEmit\n\tEOF":        "tsc",
		"echo $((1<<4)) && go test ./...":          "go test",
		"echo $((1<<4))\ngo test ./...":            "go test",
		"bash > /tmp/log 2>&1 <<EOF\npytest\nEOF":  "pytest",
		"bash -s -- arg <<EOF\nmypy .\nEOF\nls":    "mypy",
		"cat <<A | bash\ngo vet ./...\nA\necho ok": "go vet",
	}
	for line, match := range heavy {
		v := ClassifyCPU(line)
		assert.True(t, v.Heavy, line)
		assert.Equal(t, match, v.Match, line)
	}
	// The disk gate reads a heredoc script the same way.
	for line, match := range map[string]string{
		"bash <<'EOF'\ngo test ./...\nEOF":     "go test",
		"cat <<'EOF' | bash\ncargo build\nEOF": "cargo build",
	} {
		v := Classify(line)
		assert.True(t, v.Heavy, line)
		assert.Equal(t, match, v.Match, line)
	}
	assert.False(t, Classify("ssh host <<EOF\ngo test ./...\nEOF").Heavy)
}
