package testscenario

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
)

// AutomationExec runs the product itself and checks what a user observes:
// the exit code, stdout, stderr, and files it leaves. A command case only
// learns whether a test runner exited 0, so its expectations live in tests
// the implementer wrote; an exec case carries the expectation itself.
const AutomationExec = "exec"

// Placeholders an exec argv may use. $BIN is a fresh directory a build step
// writes into; $PROJECT is the project root. The product runs in a fresh work
// directory holding only the case's files, so a relative path in argv or in
// an expectation names a fixture.
const (
	PlaceholderBin     = "$BIN"
	PlaceholderProject = "$PROJECT"
)

// Exec is one run of the product.
type Exec struct {
	// Build runs first, in the project root, with an allowlisted runner. It
	// typically writes the program into $BIN.
	Build   []string          `yaml:"build,omitempty" json:"build,omitempty"`
	Argv    []string          `yaml:"argv" json:"argv"`
	Stdin   string            `yaml:"stdin,omitempty" json:"stdin,omitempty"`
	Files   map[string]string `yaml:"files,omitempty" json:"files,omitempty"`
	Timeout string            `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	Expect  ExecExpect        `yaml:"expect" json:"expect"`
}

// ExecExpect holds what the user observes. Each set field is checked; unset
// fields are not. Stdout is an exact match, the Contains lists are substrings.
// StdoutLines pins order and count without pinning the line format: stdout
// must have exactly that many non-empty lines, and line i must contain entry
// i. It is how "exactly these 3, in this order" is checked when the request
// never said what a line looks like.
type ExecExpect struct {
	ExitCode          *int              `yaml:"exit_code,omitempty" json:"exit_code,omitempty"`
	Stdout            *string           `yaml:"stdout,omitempty" json:"stdout,omitempty"`
	StdoutContains    []string          `yaml:"stdout_contains,omitempty" json:"stdout_contains,omitempty"`
	StdoutNotContains []string          `yaml:"stdout_not_contains,omitempty" json:"stdout_not_contains,omitempty"`
	StdoutLines       []string          `yaml:"stdout_lines,omitempty" json:"stdout_lines,omitempty"`
	StderrContains    []string          `yaml:"stderr_contains,omitempty" json:"stderr_contains,omitempty"`
	Files             map[string]string `yaml:"files,omitempty" json:"files,omitempty"`
}

// empty reports whether nothing would be checked.
func (e ExecExpect) empty() bool {
	return e.ExitCode == nil && e.Stdout == nil && len(e.StdoutContains) == 0 &&
		len(e.StdoutNotContains) == 0 && len(e.StdoutLines) == 0 &&
		len(e.StderrContains) == 0 && len(e.Files) == 0
}

// ValidateExec checks an exec payload. argv[0] must be the program under
// test, never an arbitrary system binary: a $BIN build output, a path inside
// the project, or an allowlisted runner such as python3.
func ValidateExec(x *Exec) error {
	if x == nil || len(x.Argv) == 0 || strings.TrimSpace(x.Argv[0]) == "" {
		return fmt.Errorf("exec automation requires exec.argv")
	}
	if !execProgramAllowed(x.Argv) {
		return fmt.Errorf("exec.argv[0] %q must be $BIN/<name>, a path inside the project such as ./bin/tool, or one of: %s",
			x.Argv[0], strings.Join(CommandAllowlist, ", "))
	}
	if len(x.Build) > 0 && !AllowedCommand(x.Build) {
		return fmt.Errorf("exec.build[0] %q must be one of: %s", x.Build[0], strings.Join(CommandAllowlist, ", "))
	}
	if x.Expect.empty() {
		return fmt.Errorf("exec.expect must state at least one observable outcome")
	}
	if x.Timeout != "" {
		if d, err := time.ParseDuration(x.Timeout); err != nil || d <= 0 || d > 10*time.Minute {
			return fmt.Errorf("exec.timeout %q must be a duration up to 10m", x.Timeout)
		}
	}
	for _, files := range []map[string]string{x.Files, x.Expect.Files} {
		for name := range files {
			if !SafeRelPath(name) {
				return fmt.Errorf("exec file %q must be a relative path without ..", name)
			}
		}
	}
	return nil
}

func execProgramAllowed(argv []string) bool {
	program := argv[0]
	if rest, ok := strings.CutPrefix(program, PlaceholderBin+"/"); ok {
		return rest != "" && SafeRelPath(rest)
	}
	if rest, ok := strings.CutPrefix(program, PlaceholderProject+"/"); ok {
		return rest != "" && SafeRelPath(rest)
	}
	if strings.Contains(program, "/") {
		return SafeRelPath(strings.TrimPrefix(program, "./"))
	}
	return AllowedCommand(argv)
}

// SafeRelPath reports whether p is a non-empty relative path that stays
// inside the directory it is joined to.
func SafeRelPath(p string) bool {
	if p == "" || filepath.IsAbs(p) || strings.Contains(p, `\`) {
		return false
	}
	clean := filepath.Clean(p)
	return clean != "." && clean != ".." && !strings.HasPrefix(clean, "../")
}
