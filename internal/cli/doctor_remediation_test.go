package cli

import (
	"strings"
	"testing"
)

// The banner named `auto doctor --fix` for every failure. On a repo that
// gitignores its harness, a fresh clone fails every platform check and `--fix`
// installs nothing, so the operator was sent in a circle: same errors, same
// advice, zero files written. Each failure class must name the command that
// repairs it.
func TestDoctorRemediationAdvice_NamesTheEffectiveCommand(t *testing.T) {
	for _, tc := range []struct {
		name            string
		platform, deps  bool
		wantContains    string
		wantNotContains string
	}{
		{
			name:            "missing managed surface points at the installer",
			platform:        true,
			wantContains:    "auto update",
			wantNotContains: "--fix",
		},
		{
			name:            "missing dependencies point at the dependency installer",
			deps:            true,
			wantContains:    "auto doctor --fix",
			wantNotContains: "auto update",
		},
		{
			name:            "errors without a known repair advertise no command",
			wantContains:    "review the [ERROR] lines",
			wantNotContains: "auto",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := doctorRemediationAdvice(tc.platform, tc.deps)
			if !strings.Contains(got, tc.wantContains) {
				t.Fatalf("advice %q does not contain %q", got, tc.wantContains)
			}
			if tc.wantNotContains != "" && strings.Contains(got, tc.wantNotContains) {
				t.Fatalf("advice %q must not contain %q", got, tc.wantNotContains)
			}
		})
	}
}

// The both-failed case must name both repairs, so neither is lost when a clone
// is missing the managed surface and a tool at the same time.
func TestDoctorRemediationAdvice_BothFailuresNameBothCommands(t *testing.T) {
	got := doctorRemediationAdvice(true, true)
	for _, want := range []string{"auto update", "auto doctor --fix"} {
		if !strings.Contains(got, want) {
			t.Fatalf("advice %q does not contain %q", got, want)
		}
	}
}
