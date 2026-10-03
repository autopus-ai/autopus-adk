package record

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// Runner starts a command in dir and waits for it to exit. Live uses the real
// process by default; tests inject one that writes the recording file.
type Runner func(ctx context.Context, dir, name string, args ...string) error

// LiveOptions configures a live codegen session.
type LiveOptions struct {
	// Origin is the explicit origin; empty inherits the journey's first
	// allowed origin.
	Origin       string
	ID           string
	Title        string
	Journey      string
	AllowPartial bool
	Exec         Runner
}

// Live opens Playwright codegen on the origin, waits for its window to close,
// and imports the recording as a candidate (REQ-15). When the import fails the
// raw recording is kept and its path returned, so a fixable line never costs
// the person the session they just recorded.
func Live(ctx context.Context, projectDir string, opts LiveOptions) (Result, error) {
	journeyID, origin, err := importTarget(projectDir, opts.Journey, opts.Origin)
	if err != nil {
		return Result{}, err
	}
	canonical, err := CanonicalOrigin(origin)
	if err != nil {
		return Result{}, err
	}
	run := opts.Exec
	if run == nil {
		run = runAttached
	}
	dir, err := os.MkdirTemp("", "autopus-record-")
	if err != nil {
		return Result{}, err
	}
	out := filepath.Join(dir, "recording.js")
	if err := run(ctx, projectDir, "npx", "playwright", "codegen", "--target", "javascript", "-o", out, canonical); err != nil {
		_ = os.RemoveAll(dir)
		if errors.Is(err, exec.ErrNotFound) {
			return Result{}, &Error{Code: CodePlaywrightMissing, SetupGap: true,
				Message: "npx was not found on PATH: install Node.js and @playwright/test (npm i -D @playwright/test) to record journeys"}
		}
		return Result{}, failf(CodeCodegenFailed, "playwright codegen failed: %v", err)
	}
	if info, statErr := os.Stat(out); statErr != nil || info.Size() == 0 {
		_ = os.RemoveAll(dir)
		return Result{}, failf(CodeEmpty, "playwright codegen saved no recording; record at least one step before closing the window")
	}
	result, err := Import(projectDir, ImportOptions{From: out, Format: FormatCodegen, ID: opts.ID, Title: opts.Title,
		Journey: journeyID, Origin: canonical, AllowPartial: opts.AllowPartial})
	if err != nil {
		result.Recording = out
		return result, fmt.Errorf("%w (recording kept at %s)", err, out)
	}
	_ = os.RemoveAll(dir)
	return result, nil
}

// runAttached runs the command on the person's terminal. Its stdout goes to
// stderr so a --format json caller still reads exactly one JSON document.
func runAttached(ctx context.Context, dir, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stderr, os.Stderr
	return cmd.Run()
}
