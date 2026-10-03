// Package testscenario reads qamesh.test-scenarios.v1 documents: one file per
// SPEC that pins every acceptance criterion to a case and says how the case
// is verified (a GUI scenario, an allowlisted command, or a human).
package testscenario

import "path/filepath"

// SchemaVersion is the only schema_version a document may declare.
const SchemaVersion = "qamesh.test-scenarios.v1"

var (
	// DirRel holds active documents, the only ones the candidate compiler reads.
	DirRel = filepath.Join(".autopus", "qa", "test-scenarios")
	// CandidatesDirRel holds generated documents awaiting promotion. Keeping
	// them out of DirRel is what stops unreviewed agent output from running.
	CandidatesDirRel = filepath.Join(DirRel, "candidates")
)

// Case kinds.
const (
	KindHappy    = "happy"
	KindNegative = "negative"
	KindEdge     = "edge"
)

// Automation types.
const (
	AutomationGUI     = "gui"
	AutomationCommand = "command"
	AutomationManual  = "manual"
)

// Document is one test-scenarios file.
type Document struct {
	SchemaVersion string `yaml:"schema_version" json:"schema_version"`
	Spec          string `yaml:"spec" json:"spec"`
	Cases         []Case `yaml:"cases" json:"cases"`
	// Path is the file's base name, set by the loader for error messages.
	Path string `yaml:"-" json:"path,omitempty"`
}

// Case ties one acceptance criterion to one verification.
type Case struct {
	ID         string     `yaml:"id" json:"id"`
	Ac         string     `yaml:"ac" json:"ac"`
	Kind       string     `yaml:"kind" json:"kind"`
	Title      string     `yaml:"title" json:"title"`
	Given      string     `yaml:"given" json:"given"`
	When       string     `yaml:"when" json:"when"`
	Then       string     `yaml:"then" json:"then"`
	Automation Automation `yaml:"automation" json:"automation"`
}

// Automation says how a case is verified. Exactly one payload applies per
// type: Scenario for gui, Check for command, Reason for manual.
type Automation struct {
	Type     string `yaml:"type" json:"type"`
	Scenario string `yaml:"scenario,omitempty" json:"scenario,omitempty"`
	Check    *Check `yaml:"check,omitempty" json:"check,omitempty"`
	Reason   string `yaml:"reason,omitempty" json:"reason,omitempty"`
}

// Check is a command verification. There is deliberately no env field: only
// variable names may be allowlisted, never values written into the repo.
type Check struct {
	Adapter      string   `yaml:"adapter,omitempty" json:"adapter,omitempty"`
	Argv         []string `yaml:"argv" json:"argv"`
	CWD          string   `yaml:"cwd,omitempty" json:"cwd,omitempty"`
	Timeout      string   `yaml:"timeout,omitempty" json:"timeout,omitempty"`
	EnvAllowlist []string `yaml:"env_allowlist,omitempty" json:"env_allowlist,omitempty"`
}

// CommandAllowlist is the set of test runners a command case may start
// (SPEC-QALOOP-001 REQ-5). Anything else compiles to a deferred candidate.
var CommandAllowlist = []string{
	"go", "npm", "npx", "pnpm", "yarn", "bun", "pytest", "python", "python3",
	"uv", "cargo", "make", "deno", "node",
}

// AllowedCommand reports whether argv[0] names an allowlisted runner.
//
// argv[0] must be the bare name, resolved through PATH. A path-qualified
// program such as /tmp/x/go has the right base name but is an arbitrary
// binary, and the compiler would infer the permissive custom-command adapter
// for it, so nothing downstream would catch it.
func AllowedCommand(argv []string) bool {
	if len(argv) == 0 || argv[0] == "" || filepath.Base(argv[0]) != argv[0] {
		return false
	}
	for _, name := range CommandAllowlist {
		if argv[0] == name {
			return true
		}
	}
	return false
}
