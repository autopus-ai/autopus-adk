package diskgate

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Second review of the follow-ups: command substitution of a subshell,
// shell options that take a value, redirections, help flags anywhere, and
// line continuations.
func TestClassify_ShellSyntaxReReview(t *testing.T) {
	t.Parallel()
	heavy := map[string]string{
		// A subshell in a command substitution is not arithmetic.
		"out=$((cd web && npm run build) 2>&1)": "npm run build",
		"res=$((go test ./... 2>&1) | tail -5)": "go test",
		"x=$((go build ./...) )":                "go build",
		// Shell options that take a value do not end the option list.
		"bash -euo pipefail <<'EOF'\ngo test ./...\nEOF":       "go test",
		"bash -eo pipefail <<EOF\ngo test ./...\nEOF":          "go test",
		"bash -euo pipefail -s <<EOF\ngo test ./...\nEOF":      "go test",
		"cat <<'EOF' | bash -euo pipefail\ngo test ./...\nEOF": "go test",
		"bash -O extglob <<EOF\ngo build ./...\nEOF":           "go build",
		"bash --rcfile f <<EOF\ngo build ./...\nEOF":           "go build",
		"bash --init-file f <<EOF\ngo build ./...\nEOF":        "go build",
		"bash -o pipefail -eu <<EOF\ngo test ./...\nEOF":       "go test",
		// A quoted or escaped < or > before & is not a redirection.
		"echo \"done ->\"& go test ./...": "go test",
		"echo '<'& go build ./...":        "go build",
		"echo \\>& go test ./...":         "go test",
		"printf '%s>' x& go build ./...":  "go build",
		// Line continuations join the command.
		"cd /x && \\\n  go test ./...":                          "go test",
		"GOFLAGS=-count=1 \\\n  go test ./...":                  "go test",
		"GOTMPDIR=$TMPDIR \\\n  go test ./pkg/... -timeout 40m": "go test",
		"go test \\\n  ./...":                                   "go test",
		// Redirections do not hide the command.
		"go test ./... > /tmp/out 2>&1":           "go test",
		">/tmp/log go build ./...":                "go build",
		"go test ./... &>/dev/null":               "go test",
		"bash > /tmp/log 2>&1 <<EOF\npytest\nEOF": "pytest",
		// A backquote in a redirection target runs its command.
		"echo hi >`go build ./...`":  "go build",
		"echo hi > `go test ./...`":  "go test",
		">`go build -o x ./...`.log": "go build",
		// Command substitutions inside arithmetic run.
		"echo $(( $(go test ./... | wc -l) + 1 ))": "go test",
		"echo $(( `go build ./... | wc -l` * 2 ))": "go build",
		"n=$(( $(echo $(go build ./...)) + 0 ))":   "go build",
	}
	for line, match := range heavy {
		v := Classify(line)
		if match == "pytest" {
			v = ClassifyCPU(line)
		}
		assert.True(t, v.Heavy, line)
		assert.Equal(t, match, v.Match, line)
	}
	for _, line := range []string{
		"echo $(( (1+2) * 3 ))", "echo $(( $(nproc) * 2 ))", "echo $((1<<4))\nls",
		"bash 2>&1 script.sh <<EOF\ngo test ./...\nEOF", "bash -x 2>&1 script.sh <<EOF\ngo test ./...\nEOF",
		"bash >&2 script.sh <<EOF\ngo test ./...\nEOF",
	} {
		assert.False(t, Classify(line).Heavy, line)
		assert.False(t, ClassifyCPU(line).Heavy, line)
	}
	for _, line := range []string{
		"auto qa exec --help 2>&1 | head -40", "auto qa exec -h > /tmp/h.txt", "auto qa exec --all --help",
		"auto qa --help exec", "auto workflow gate --help 2>/dev/null",
	} {
		assert.False(t, ClassifyCPU(line).Heavy, line)
	}
	assert.True(t, ClassifyCPU("auto qa exec --all 2>&1 | tail -5").Heavy)
	assert.True(t, ClassifyCPU("auto disk lease --need 5 -- auto qa run --verbose").Heavy)
}
