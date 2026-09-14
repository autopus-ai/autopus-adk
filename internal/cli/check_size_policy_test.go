package cli

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func writeFileSizePolicy(t *testing.T, dir string, limit int) {
	t.Helper()
	writeTestFile(t, dir, "autopus.yaml", fmt.Sprintf("mode: full\nproject_name: size-policy\nplatforms: [claude-code]\narchitecture:\n  max_file_lines: %d\n", limit))
}

func TestCheckArchProjectSizePolicy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name       string
		limit      int
		configured bool
		passed     bool
	}{
		{name: "unconfigured is advisory", passed: true},
		{name: "zero is advisory", configured: true, passed: true},
		{name: "explicit boundary blocks", limit: 300, configured: true},
		{name: "larger project boundary permits", limit: 400, configured: true, passed: true},
		{name: "invalid policy fails closed", limit: -1, configured: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeSourceFileWithLines(t, dir, "large.ts", 301)
			if tc.configured {
				writeFileSizePolicy(t, dir, tc.limit)
			}
			var out bytes.Buffer
			assert.Equal(t, tc.passed, checkArch(dir, &out, false, false), out.String())
		})
	}
}

func TestCheckArchMalformedPolicyFailsClosed(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	writeTestFile(t, dir, "autopus.yaml", "architecture: [broken\n")
	var out bytes.Buffer
	assert.False(t, checkArch(dir, &out, true, false))
}
