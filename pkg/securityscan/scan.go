package securityscan

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Status says what happened to one scanner.
type Status string

const (
	StatusRan     Status = "ran"
	StatusMissing Status = "missing"
	StatusSkipped Status = "skipped"
	StatusError   Status = "error"
)

// ScannerResult is one scanner's line in the report.
type ScannerResult struct {
	Name     string `json:"name"`
	Status   Status `json:"status"`
	Detail   string `json:"detail,omitempty"`
	Findings int    `json:"findings"`
}

// Report is the result of one scan.
type Report struct {
	Dir        string          `json:"dir"`
	Ecosystems []Ecosystem     `json:"ecosystems"`
	Scanners   []ScannerResult `json:"scanners"`
	Findings   []Finding       `json:"findings"`
}

// ExecFunc runs bin with args in dir and returns its output. A non-zero
// exit is an error, but stdout is still returned: most scanners exit
// non-zero exactly when they find something.
type ExecFunc func(ctx context.Context, dir, bin string, args ...string) (stdout, stderr []byte, err error)

// Options configure a scan. LookPath and Exec default to the real ones.
type Options struct {
	Dir      string
	Timeout  time.Duration // per scanner
	Staged   bool          // gitleaks: only staged changes
	LogOpts  string        // gitleaks: git log range, such as main..HEAD
	LookPath func(string) (string, error)
	Exec     ExecFunc
}

// job is one scanner to run: bin is looked up on PATH, skip explains why it
// does not apply, and run produces its findings.
type job struct {
	name, bin, skip string
	run             func(ctx context.Context, o Options, path string) ([]Finding, error)
}

// Scan detects the project's ecosystems and runs every applicable scanner
// that is installed. A missing tool is noted, never an error.
func Scan(ctx context.Context, o Options) Report {
	if o.LookPath == nil {
		o.LookPath = exec.LookPath
	}
	if o.Exec == nil {
		o.Exec = runCommand
	}
	if o.Timeout <= 0 {
		o.Timeout = 5 * time.Minute
	}
	rep := Report{Dir: o.Dir, Ecosystems: Detect(o.Dir), Findings: []Finding{}}
	for _, j := range plan(o, rep.Ecosystems) {
		res := ScannerResult{Name: j.name}
		path, lookErr := o.LookPath(j.bin)
		switch {
		case j.skip != "":
			res.Status, res.Detail = StatusSkipped, j.skip
		case lookErr != nil:
			res.Status, res.Detail = StatusMissing, j.bin+" not installed"
		default:
			jobCtx, cancel := context.WithTimeout(ctx, o.Timeout)
			findings, err := j.run(jobCtx, o, path)
			if errors.Is(jobCtx.Err(), context.DeadlineExceeded) {
				err = fmt.Errorf("timed out after %s", o.Timeout)
			}
			cancel()
			if err != nil {
				res.Status, res.Detail = StatusError, err.Error()
				break
			}
			res.Status, res.Findings = StatusRan, len(findings)
			rep.Findings = append(rep.Findings, relativize(o.Dir, findings)...)
		}
		rep.Scanners = append(rep.Scanners, res)
	}
	return rep
}

// runParsed runs a scanner that prints JSON on stdout and parses it. When
// the output does not parse, the scanner's own first stderr line explains
// why better than the parse error does.
func runParsed(parse func([]byte) ([]Finding, error), args ...string) func(context.Context, Options, string) ([]Finding, error) {
	return func(ctx context.Context, o Options, path string) ([]Finding, error) {
		stdout, stderr, runErr := o.Exec(ctx, o.Dir, path, args...)
		findings, err := parse(stdout)
		if err == nil {
			return findings, nil
		}
		if line := firstLine(stderr); line != "" {
			return nil, errors.New(line)
		}
		if runErr != nil && len(bytes.TrimSpace(stdout)) == 0 {
			return nil, runErr
		}
		return nil, err
	}
}

func runCommand(ctx context.Context, dir, bin string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.WaitDelay = 5 * time.Second
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

func firstLine(b []byte) string {
	for _, line := range strings.Split(string(b), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return line
		}
	}
	return ""
}

// relativize shortens absolute paths under dir.
func relativize(dir string, findings []Finding) []Finding {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return findings
	}
	prefix := abs + string(filepath.Separator)
	for i := range findings {
		findings[i].File = strings.TrimPrefix(findings[i].File, prefix)
		findings[i].Location = strings.TrimPrefix(findings[i].Location, prefix)
	}
	return findings
}

// Failed reports whether any finding reaches the threshold.
func (r Report) Failed(f FailOn) bool {
	for _, fd := range r.Findings {
		if f.Fails(fd) {
			return true
		}
	}
	return false
}

// HasErrors reports whether an installed scanner could not complete.
func (r Report) HasErrors() bool {
	for _, s := range r.Scanners {
		if s.Status == StatusError {
			return true
		}
	}
	return false
}
