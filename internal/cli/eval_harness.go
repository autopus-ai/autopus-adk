package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/pkg/harneval"
)

// evalHarnessDeps holds the seams of the `auto eval harness` commands. The
// zero value is production: the pinned adapters, the real template check,
// the wall clock, and `go list` for the dependency closure.
type evalHarnessDeps struct {
	// run carries the harneval.Run seams; tests swap the surface and clock.
	run harneval.RunOptions
	// closure lists the in-module package directories pkg/harneval depends
	// on, relative to the repository root; nil selects goListClosure.
	closure func(ctx context.Context, root string) ([]string, error)
}

// errHarnessReported is the cause of an exit 1 whose explanation is already
// written (the result document on stdout, the guidance on stderr), so
// Execute exits without printing it again.
var errHarnessReported = errors.New("harness eval: failure already reported")

// harnessFailure ends a command with exit 1 and no further output.
func harnessFailure() error { return &jsonFatalError{cause: errHarnessReported} }

// harnessExit maps a result exit code to the command error.
func harnessExit(code int) error {
	if code == 0 {
		return nil
	}
	return harnessFailure()
}

func newEvalCmd() *cobra.Command { return newEvalCmdWith(evalHarnessDeps{}) }

func newEvalCmdWith(deps evalHarnessDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:           "eval",
		Short:         "Evaluate harness changes",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(newEvalHarnessCmd(deps))
	return cmd
}

// newEvalHarnessCmd is the SPEC-HARNEVAL-001 deterministic lane: golden-task
// evaluation of the five generated platform surfaces against a committed
// baseline.
func newEvalHarnessCmd(deps evalHarnessDeps) *cobra.Command {
	dir := "."
	cmd := &cobra.Command{
		Use:   "harness",
		Short: "Golden-task eval of the generated harness surfaces",
	}
	cmd.PersistentFlags().StringVar(&dir, "dir", ".", "repository root holding evals/harness")
	cmd.AddCommand(
		newEvalHarnessRunCmd(deps, &dir),
		newEvalHarnessBaselineCmd(deps, &dir),
		newEvalHarnessApplicableCmd(deps, &dir),
		newEvalHarnessDigestCmd(deps, &dir),
		newEvalHarnessReportCmd(),
		newEvalHarnessIntakeCmd(&dir),
		newEvalHarnessRejectCmd(&dir),
	)
	return cmd
}

// requireHarnessJSON accepts the only output format the commands write.
func requireHarnessJSON(format string) error {
	if format != "json" {
		return fmt.Errorf("unsupported --format %q: only json is supported", format)
	}
	return nil
}

// writeHarnessJSON writes one indented JSON document and a trailing newline.
func writeHarnessJSON(w io.Writer, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(data, '\n'))
	return err
}

// harnessInputGlobs are the fixed members of the derived harness input set
// (REQ-HE-05). The in-module dependency closure of pkg/harneval completes it,
// so the generator packages are derived rather than listed by hand.
var harnessInputGlobs = []string{
	"content/**", "templates/**", "evals/harness/**", "scripts/benchmarks/harness/**",
	"go.mod", "go.sum", ".github/workflows/ci.yaml", "internal/cli/eval_harness*.go",
}

// Applicability statuses and reasons of `auto eval harness applicable`.
const (
	harnessApplicable          = "applicable"
	harnessNotApplicable       = "not_applicable"
	harnessNonPullRequestEvent = "non_pull_request_event"
	harnessClosureUnavailable  = "closure_unavailable"
	harnessInputChanged        = "harness_input_changed"
	harnessNoInputChanged      = "no_harness_input_changed"
)

// harnessApplicability is the decision document of `applicable`.
type harnessApplicability struct {
	Status  string   `json:"status"`
	Reason  string   `json:"reason"`
	Matched []string `json:"matched"`
}

// newEvalHarnessApplicableCmd decides whether the CI job evaluates: every
// event other than pull_request does, and a pull request does when a changed
// path meets the derived input set. A closure that cannot be computed is
// evaluated, never skipped. The decision exits 0; only a bad invocation fails.
func newEvalHarnessApplicableCmd(deps evalHarnessDeps, dir *string) *cobra.Command {
	var event, changedFiles, format string
	cmd := &cobra.Command{
		Use:   "applicable",
		Short: "Decide whether a CI event must run the harness eval",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := requireHarnessJSON(format); err != nil {
				return err
			}
			if event == "" {
				return errors.New("--event is required")
			}
			if event != "pull_request" {
				return writeHarnessJSON(cmd.OutOrStdout(), harnessApplicability{
					Status: harnessApplicable, Reason: harnessNonPullRequestEvent, Matched: []string{},
				})
			}
			if changedFiles == "" {
				return errors.New("--changed-files is required for a pull_request event")
			}
			changed, err := readChangedFiles(changedFiles)
			if err != nil {
				return err
			}
			closure := deps.closure
			if closure == nil {
				closure = goListClosure
			}
			dirs, closureErr := closure(cmd.Context(), *dir)
			decision := harnessApplicability{
				Status: harnessApplicable, Reason: harnessInputChanged, Matched: matchHarnessInputs(changed, dirs),
			}
			switch {
			case closureErr != nil:
				fmt.Fprintln(cmd.ErrOrStderr(), "harness-eval: closure unavailable: "+closureErr.Error())
				decision.Reason = harnessClosureUnavailable
			case len(decision.Matched) == 0:
				decision.Status, decision.Reason = harnessNotApplicable, harnessNoInputChanged
			}
			return writeHarnessJSON(cmd.OutOrStdout(), decision)
		},
	}
	cmd.Flags().StringVar(&event, "event", "", "GitHub event name; only pull_request may be not_applicable")
	cmd.Flags().StringVar(&changedFiles, "changed-files", "", "file listing the changed paths of the pull request, one per line")
	cmd.Flags().StringVar(&format, "format", "json", "output format (json)")
	return cmd
}

// readChangedFiles reads one path per line. A path the diff C-quoted
// (non-ASCII or special bytes under core.quotePath) is unquoted, so it still
// matches the input set.
func readChangedFiles(name string) ([]string, error) {
	data, err := os.ReadFile(name)
	if err != nil {
		return nil, fmt.Errorf("read --changed-files: %w", err)
	}
	var files []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if unquoted, err := strconv.Unquote(line); err == nil && strings.HasPrefix(line, `"`) {
			line = unquoted
		}
		if line != "" {
			files = append(files, line)
		}
	}
	return files, nil
}

// matchHarnessInputs returns the sorted unique changed paths that lie under
// a fixed glob or a closure package directory.
func matchHarnessInputs(changed, closure []string) []string {
	seen := map[string]bool{}
	matched := []string{}
	for _, file := range changed {
		if !seen[file] && isHarnessInput(file, closure) {
			seen[file] = true
			matched = append(matched, file)
		}
	}
	sort.Strings(matched)
	return matched
}

func isHarnessInput(file string, closure []string) bool {
	for _, glob := range harnessInputGlobs {
		if dir, isTree := strings.CutSuffix(glob, "/**"); isTree {
			if underDir(file, dir) {
				return true
			}
		} else if ok, _ := path.Match(glob, file); ok {
			return true
		}
	}
	for _, dir := range closure {
		if underDir(file, dir) {
			return true
		}
	}
	return false
}

// underDir matches at a path-segment boundary: pkg/a holds pkg/a/x.go but not
// pkg/ab/x.go. The module root "." holds every path.
func underDir(file, dir string) bool {
	return dir == "." || strings.HasPrefix(file, dir+"/")
}

// goListClosure runs `go list -deps ./pkg/harneval` under root and returns
// the main-module package directories relative to root.
func goListClosure(ctx context.Context, root string) ([]string, error) {
	list := exec.CommandContext(ctx, "go", "list", "-deps", "-f",
		"{{with .Module}}{{if .Main}}{{.Path}} {{$.ImportPath}}{{end}}{{end}}", "./pkg/harneval")
	list.Dir = root
	var stderr bytes.Buffer
	list.Stderr = &stderr
	out, err := list.Output()
	if err != nil {
		return nil, fmt.Errorf("go list -deps ./pkg/harneval: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	var dirs []string
	for _, line := range strings.Split(string(out), "\n") {
		module, pkg, ok := strings.Cut(strings.TrimSpace(line), " ")
		if !ok {
			continue
		}
		dir := strings.TrimPrefix(strings.TrimPrefix(pkg, module), "/")
		if dir == "" {
			dir = "."
		}
		dirs = append(dirs, dir)
	}
	if len(dirs) == 0 {
		return nil, errors.New("go list -deps ./pkg/harneval reported no main-module package")
	}
	return dirs, nil
}
