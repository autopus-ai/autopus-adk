package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
)

// composeDriverState is one running session.
type composeDriverState struct {
	composeDriverSession
	out      io.Writer
	in       *bufio.Reader
	snapshot driftGenerationSnapshot
}

func (s composeDriverSession) text() composeText {
	if s.Text == (composeText{}) {
		return currentComposeText()
	}
	return s.Text
}

// run probes the Codex catalog once, draws the table, and asks the one
// composition question. Every keep path leaves autopus.yaml byte-identical.
// A failed write returns its error with a kept result that still carries the
// snapshot.
func (s composeDriverSession) run(cmd *cobra.Command) (composeDriverResult, error) {
	d := s.start(cmd)
	d.in = composeSharedReader(cmd)
	if err := d.draw(); err != nil {
		return d.kept(), err
	}
	return d.menu()
}

// show draws the table without asking anything, for an init that chose
// balanced or ultra (REQ-MUX-15). It returns the snapshot the table read so
// the same run generates with it.
func (s composeDriverSession) show(cmd *cobra.Command) (driftGenerationSnapshot, error) {
	d := s.start(cmd)
	return d.snapshot, d.draw()
}

func (s composeDriverSession) start(cmd *cobra.Command) *composeDriverState {
	s.Text = s.text()
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}
	return &composeDriverState{
		composeDriverSession: s,
		out:                  cmd.OutOrStdout(),
		snapshot:             captureComposeSnapshot(ctx, s.Config),
	}
}

// composeSharedReader wraps the command input in one buffered reader and
// installs it as the input. Every later prompt, including an existing editor
// the menu hands off to, then reads the same buffer: bufio.NewReader returns
// a large enough *bufio.Reader unchanged, while independent readers over one
// stream would each buffer ahead and lose the answers queued behind them.
func composeSharedReader(cmd *cobra.Command) *bufio.Reader {
	reader := bufio.NewReader(cmd.InOrStdin())
	cmd.SetIn(reader)
	return reader
}

func (d *composeDriverState) draw() error {
	fmt.Fprintf(d.out, "\n%s %s\n", d.Text.Title, strings.Repeat("─", 40))
	fmt.Fprintln(d.out, composeDriverStatus(d.Config.Quality, d.Text))
	return d.Table.Table(d.out, d.Config, d.snapshot)
}

// menu asks the composition question until it gets a usable answer. Enter
// and input end keep; an invalid answer shows the choices again, and the
// third keeps (REQ-MUX-13). `u` exists only while a built-in preset is
// historical, and `t` with the target names only for `auto quality`.
func (d *composeDriverState) menu() (composeDriverResult, error) {
	refresh := len(composeDriverHistorical(d.Config.Quality)) > 0
	for range composeDriverMaxInvalid {
		fmt.Fprintln(d.out, d.question(refresh))
		answer, ok := d.ask()
		if !ok || answer == "" {
			return d.kept(), nil
		}
		switch strings.ToLower(answer) {
		case "u":
			if refresh {
				return d.refresh()
			}
		case "g":
			return d.compose(d.groupStep)
		case "a":
			return d.compose(d.advancedStep)
		}
		if target, ok := d.toolTarget(answer); ok {
			return composeDriverResult{Outcome: composeDriverRouted, Snapshot: d.snapshot}, d.Tools.Route(target)
		}
	}
	fmt.Fprintln(d.out, d.Text.InvalidKeep)
	return d.kept(), nil
}

// toolTarget matches `t` or a target name the way readQualityChoice matches
// names, ignoring case.
func (d *composeDriverState) toolTarget(answer string) (string, bool) {
	if d.Tools == nil {
		return "", false
	}
	if strings.EqualFold(answer, "t") {
		return "", true
	}
	for _, target := range d.Tools.Targets() {
		if strings.EqualFold(answer, target) {
			return target, true
		}
	}
	return "", false
}

// refresh reviews the `u` plan. Unlike a group or advanced edit, it is an
// explicit rewrite even when the table stays the same (REQ-MUX-07).
func (d *composeDriverState) refresh() (composeDriverResult, error) {
	plan, err := d.Planner.PlanRefresh()
	if err != nil {
		return d.kept(), err
	}
	return d.review(plan)
}

// compose runs one editor from the starting composition and reviews the
// result.
func (d *composeDriverState) compose(step composeDriverStep) (composeDriverResult, error) {
	plan, ok, err := d.edit(step, d.Planner.Start())
	if !ok || err != nil {
		return d.kept(), err
	}
	return d.review(plan)
}

// review previews plan and asks for confirmation (REQ-MUX-06). Only y or yes
// applies. `e` returns to the edit menu with the candidate kept; every other
// answer, including an empty line and input end, cancels.
func (d *composeDriverState) review(plan composeDriverPlan) (composeDriverResult, error) {
	for {
		if err := d.preview(plan); err != nil {
			return d.kept(), err
		}
		fmt.Fprintln(d.out, d.Text.Confirm)
		answer, _ := d.ask()
		switch strings.ToLower(answer) {
		case "y", "yes":
			return d.apply(plan)
		case "e":
			next, ok, err := d.editAgain(plan)
			if !ok || err != nil {
				return d.kept(), err
			}
			plan = next
			continue
		}
		fmt.Fprintln(d.out, d.Text.Cancelled)
		return d.kept(), nil
	}
}

// preview draws the after-table against the starting config with the run's
// snapshot, then every changed key and effect outside the table.
func (d *composeDriverState) preview(plan composeDriverPlan) error {
	fmt.Fprintf(d.out, "\n%s\n", d.Text.PreviewHeading)
	if err := d.Table.Preview(d.out, d.Config, plan.Candidate, d.snapshot); err != nil {
		return err
	}
	if len(plan.Changes) == 0 {
		return nil
	}
	fmt.Fprintln(d.out, d.Text.ChangesHeading)
	for _, change := range plan.Changes {
		fmt.Fprintf(d.out, "  %s\n", terminalSafe(change))
	}
	return nil
}

// apply performs the confirmed write. A refused or failed write keeps the
// original file and returns the writer's error unwrapped, so the caller can
// match errQualityWriteRefused and print its text as is.
func (d *composeDriverState) apply(plan composeDriverPlan) (composeDriverResult, error) {
	if plan.Apply == nil {
		return d.kept(), errors.New("composition plan carries no write")
	}
	if err := plan.Apply(); err != nil {
		return d.kept(), err
	}
	fmt.Fprintln(d.out, d.Text.Applied)
	return composeDriverResult{Outcome: composeDriverApplied, Snapshot: d.snapshot, Plan: plan}, nil
}

func (d *composeDriverState) question(refresh bool) string {
	options := []string{d.Text.OptionKeep}
	if refresh {
		options = append(options, d.Text.OptionRefresh)
	}
	options = append(options, d.Text.OptionGroups, d.Text.OptionAdvanced)
	if d.Tools != nil {
		options = append(options, d.Text.OptionTools)
	}
	return d.Text.Question + " " + strings.Join(options, " · ")
}

// ask prints the prompt marker and reads one answer. ok is false when input
// ends (EOF or a read error) before an answer; a last line without a newline
// still counts as an answer.
func (d *composeDriverState) ask() (string, bool) {
	fmt.Fprint(d.out, "> ")
	line, err := d.in.ReadString('\n')
	answer := strings.TrimSpace(line)
	if err != nil && answer == "" {
		fmt.Fprintln(d.out)
		return "", false
	}
	return answer, true
}

func (d *composeDriverState) kept() composeDriverResult {
	return composeDriverResult{Outcome: composeDriverKept, Snapshot: d.snapshot}
}
