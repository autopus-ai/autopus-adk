package cli

import "testing"

func TestAgentRuntimeVersionOutput(t *testing.T) {
	for _, tc := range []struct{ output, want string }{
		{"opencode v2.0.10\n", "2.0.10"},
		{"1.18.31\n", "1.18.31"},
		{"2.1.278 (Claude Code)\n", "2.1.278"},
		{"omp/18.2.6\n", "18.2.6"},
		{"codex-cli 0.155.1\n", "0.155.1"},
		{"v2.0.10-beta.1\n", "2.0.10-beta.1"},
		{"unavailable", ""},
	} {
		t.Run(tc.output, func(t *testing.T) {
			if got := parseAgentRuntimeVersion(tc.output); got != tc.want {
				t.Fatalf("version = %q, want %q", got, tc.want)
			}
		})
	}
}
