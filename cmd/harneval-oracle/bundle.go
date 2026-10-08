package main

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Wire schema identifiers: the stdin bundle the runner writes and the result
// document the harness writes.
const (
	InputSchema  = "harness_oracle_input.v1"
	ResultSchema = "harness_oracle_result.v1"
)

// OutputLimit is the most bytes the harness reads of one output file, and the
// most bytes of stdout or of one expected output a bundle may carry.
const OutputLimit = 1 << 20

// maxAssertions bounds the assertions of one bundle.
const maxAssertions = 32

// Assertion kinds: the artifact's exit status, its captured stdout, and one
// output file at a fixed relative path below the output root.
const (
	KindExitCode = "exit_code"
	KindStdout   = "stdout"
	KindFile     = "file"
)

// Output checks of a result. not_checked is the only check of a timed-out run.
const (
	CheckOK           = "ok"
	CheckLinkRejected = "link_rejected"
	CheckTooLarge     = "too_large"
	CheckNotChecked   = "not_checked"
)

var assertionID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// input is the harness_oracle_input.v1 bundle. ArtifactExit is null when the
// artifact did not exit on its own with a status (a signal, a timeout, or no
// launch); Stdout holds at most OutputLimit bytes and StdoutOverflow says the
// artifact wrote more, which the runner did not keep.
type input struct {
	SchemaVersion  string      `json:"schema_version"`
	TaskID         string      `json:"task_id"`
	ArtifactExit   *int        `json:"artifact_exit"`
	TimedOut       bool        `json:"timed_out"`
	Stdout         []byte      `json:"stdout"`
	StdoutOverflow bool        `json:"stdout_overflow"`
	Assertions     []assertion `json:"assertions"`
}

// assertion is one pinned expectation from the main task definition. An
// exit_code assertion carries ExitCode, a stdout assertion Expected, and a
// file assertion Path and Expected.
type assertion struct {
	ID       string  `json:"id"`
	Kind     string  `json:"kind"`
	ExitCode *int    `json:"exit_code,omitempty"`
	Path     string  `json:"path,omitempty"`
	Expected *[]byte `json:"expected,omitempty"`
}

// result is the harness_oracle_result.v1 document. ArtifactExit and TimedOut
// copy the bundle; a timed-out run is not_checked, and a result whose check is
// not ok holds no assertion.
type result struct {
	SchemaVersion string            `json:"schema_version"`
	TaskID        string            `json:"task_id"`
	OutputCheck   string            `json:"output_check"`
	Assertions    []assertionResult `json:"assertions"`
	ArtifactExit  *int              `json:"artifact_exit"`
	TimedOut      bool              `json:"timed_out"`
}

type assertionResult struct {
	ID     string `json:"id"`
	Passed bool   `json:"passed"`
}

func (in input) validate(task string) error {
	switch {
	case in.SchemaVersion != InputSchema:
		return fmt.Errorf("schema_version %q is not %s", in.SchemaVersion, InputSchema)
	case in.TaskID != task:
		return fmt.Errorf("task_id %q is not --task %q", in.TaskID, task)
	case in.TimedOut && in.ArtifactExit != nil:
		return fmt.Errorf("a timed-out artifact has no exit status")
	case len(in.Stdout) > OutputLimit:
		return fmt.Errorf("stdout holds more than %d bytes", OutputLimit)
	case len(in.Assertions) == 0 || len(in.Assertions) > maxAssertions:
		return fmt.Errorf("a bundle needs 1 to %d assertions", maxAssertions)
	}
	seen := map[string]bool{}
	for _, item := range in.Assertions {
		if !assertionID.MatchString(item.ID) || seen[item.ID] {
			return fmt.Errorf("assertion id %q is malformed or repeated", item.ID)
		}
		seen[item.ID] = true
		if err := item.validate(); err != nil {
			return fmt.Errorf("assertion %s: %w", item.ID, err)
		}
	}
	return nil
}

func (a assertion) validate() error {
	switch a.Kind {
	case KindExitCode:
		if a.ExitCode == nil || a.Path != "" || a.Expected != nil {
			return fmt.Errorf("an exit_code assertion carries exit_code only")
		}
		return nil
	case KindStdout:
		if a.ExitCode != nil || a.Path != "" {
			return fmt.Errorf("a stdout assertion carries expected only")
		}
	case KindFile:
		if a.ExitCode != nil || !fixedPath(a.Path) {
			return fmt.Errorf("a file assertion needs a clean relative path, not %q", a.Path)
		}
	default:
		return fmt.Errorf("kind %q is not exit_code, stdout or file", a.Kind)
	}
	if a.Expected == nil || len(*a.Expected) > OutputLimit {
		return fmt.Errorf("expected output is missing or larger than %d bytes", OutputLimit)
	}
	return nil
}

// fixedPath accepts a clean, slash-separated relative path with no "." or
// ".." element: the only kind of output path the harness opens.
func fixedPath(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || path.Clean(name) != name {
		return false
	}
	for _, element := range strings.Split(name, "/") {
		if element == "." || element == ".." {
			return false
		}
	}
	return true
}

// matches reports whether one assertion holds for the run; contents maps each
// file assertion path to its bytes, or nil when the file does not exist.
func (a assertion) matches(in input, contents map[string][]byte) bool {
	switch a.Kind {
	case KindExitCode:
		return in.ArtifactExit != nil && *in.ArtifactExit == *a.ExitCode
	case KindStdout:
		return string(in.Stdout) == string(*a.Expected)
	default:
		data := contents[a.Path]
		return data != nil && string(data) == string(*a.Expected)
	}
}
