package cli

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// composeFormOptions is what an entry point adds to the shared form flow.
type composeFormOptions struct {
	// Presets offers 기본 프리셋 고르기, which only `auto quality` has.
	Presets bool
	// OMP is the run's OMP tab; nil leaves OMP out of the tool tab.
	OMP *composeOMPTab
}

// composeFormChoice is one menu choice.
type composeFormChoice int

const (
	composeFormKeep composeFormChoice = iota
	composeFormRefresh
	composeFormEdit
	composeFormPreset
)

// composeFormRun is one form-driven composition step (SPEC-MODELUX-002
// Form Flow Contract). It reuses the line driver's state for the table, the
// preview, and the run's one snapshot, and never reads the input itself:
// every answer comes through a form.
type composeFormRun struct {
	*composeDriverState
	cmd     *cobra.Command
	form    composeFormText
	options composeFormOptions
	// applied reports a write of this run that succeeded.
	applied bool
	// ompNoticed reports that the detected-only OMP notice was printed.
	ompNoticed bool
}

// runForms runs the form flow of a session newComposeSession built
// (REQ-MUX2-03): the Codex catalog is probed once for the run, and the menu
// follows every table until the operator keeps. The baseline is the file
// and config after the last successful write: a refused or failed write
// prints one warning line and returns to the menu with it unchanged, and
// nothing undoes an earlier confirmed write. Ctrl+C prints the interruption
// line and returns errComposeInterrupted. The result is applied when a
// write of the run succeeded; its snapshot is the one every table read,
// which the same run's generation must use.
func (s composeDriverSession) runForms(cmd *cobra.Command, options composeFormOptions) (composeDriverResult, error) {
	f := &composeFormRun{composeDriverState: s.start(cmd), cmd: cmd, form: composeFormTextFor(s.shared.lang), options: options}
	err := f.menu()
	if errors.Is(err, errComposeInterrupted) {
		fmt.Fprintln(f.out, f.form.Interrupted)
	}
	outcome := composeDriverKept
	if f.applied {
		outcome = composeDriverApplied
	}
	return composeDriverResult{Outcome: outcome, Snapshot: f.snapshot}, err
}

// menu draws the table, computed from the config every write keeps in sync,
// and asks the menu until the operator keeps.
func (f *composeFormRun) menu() error {
	for {
		if err := f.draw(); err != nil {
			return err
		}
		choice, err := composeSelect(f.cmd, f.form.Menu, "", f.menuOptions(), composeFormKeep)
		switch {
		case err != nil:
			return err
		case choice == composeFormKeep:
			return nil
		case choice == composeFormRefresh:
			err = f.refresh()
		case choice == composeFormEdit:
			err = f.edit()
		default:
			err = f.choosePreset()
		}
		if err != nil {
			return err
		}
	}
}

func (f *composeFormRun) menuOptions() []composeOption[composeFormChoice] {
	options := []composeOption[composeFormChoice]{{f.form.MenuKeep, composeFormKeep}}
	if len(composeDriverHistorical(f.Config.Quality)) > 0 {
		options = append(options, composeOption[composeFormChoice]{f.form.MenuRefresh, composeFormRefresh})
	}
	options = append(options, composeOption[composeFormChoice]{f.form.MenuEdit, composeFormEdit})
	if f.options.Presets {
		options = append(options, composeOption[composeFormChoice]{f.form.MenuPreset, composeFormPreset})
	}
	return options
}

// allToolsPlanner is a fresh 모든 도구 같게 planner for one pass of the
// menu, so a refresh planned in one pass never carries into the next.
func (f *composeFormRun) allToolsPlanner() *composeSessionPlanner {
	return &composeSessionPlanner{run: f.shared}
}

// refresh reviews the `u` plan; keep editing opens the 모든 도구 같게 method
// with the refreshed composition, and the edit binds on top of the refresh.
func (f *composeFormRun) refresh() error {
	planner := f.allToolsPlanner()
	plan, err := planner.PlanRefresh()
	if err != nil {
		f.warn(err)
		return nil
	}
	again, err := f.review(plan)
	if err != nil || !again {
		return err
	}
	return f.compose(f.withOverrideQuestion(planner), newComposeAllToolsLabeler(planner), plan.Composition)
}

// withOverrideQuestion asks, before the preview of a 모든 도구 같게 edit,
// whether each provider override follows it (REQ-MUX2-18).
func (f *composeFormRun) withOverrideQuestion(planner *composeSessionPlanner) composeDriverPlanner {
	return composeOverridePlanner{composeSessionPlanner: planner, cmd: f.cmd, form: true}
}

// edit asks the tool tab and runs the chosen tab from its start.
func (f *composeFormRun) edit() error {
	platform, err := f.chooseTab()
	if err != nil {
		return err
	}
	if platform == "omp" {
		return f.omp()
	}
	for _, tool := range qualityModelTools {
		if tool.provider != "" && tool.platform == platform {
			planner, labeler := newComposeToolTab(f.shared, tool)
			return f.compose(planner, labeler, planner.Start())
		}
	}
	planner := f.allToolsPlanner()
	return f.compose(f.withOverrideQuestion(planner), newComposeAllToolsLabeler(planner), planner.Start())
}

// chooseTab asks the tool tab (REQ-MUX2-04): 모든 도구 같게, whose answer is
// "", then Claude Code and Codex when the display view installs them, and
// OMP by the OMP Tab Contract. OpenCode and Antigravity bind no model and
// never appear, and a tab of one option is skipped.
func (f *composeFormRun) chooseTab() (string, error) {
	f.noteDetectedOMP()
	options := []composeOption[string]{{f.form.TabAll, ""}}
	view := f.shared.view()
	for _, tool := range qualityModelTools {
		if tool.provider != "" && containsPlatform(view.Platforms, tool.platform) {
			options = append(options, composeOption[string]{tool.label, tool.platform})
		}
	}
	if f.ompOffered() {
		options = append(options, composeOption[string]{"OMP", "omp"})
	}
	if len(options) == 1 {
		return "", nil
	}
	return composeSelect(f.cmd, f.form.Tab, "", options, "")
}

// compose runs one tab from working: the method, the edit, the preview, and
// the confirmation, back at the method while the operator keeps editing
// (REQ-MUX2-09). An edit equal to the tab's start writes nothing.
func (f *composeFormRun) compose(planner composeDriverPlanner, labeler composeLabeler, working map[string]string) error {
	for {
		perAgent, err := composeSelect(f.cmd, f.form.Method, "",
			[]composeOption[bool]{{f.form.MethodGroups, false}, {f.form.MethodAgents, true}}, false)
		if err != nil {
			return err
		}
		step := f.groups
		if perAgent {
			step = f.agents
		}
		edited, err := step(working, labeler)
		if err != nil {
			return err
		}
		plan, err := planner.PlanEdit(edited)
		if errors.Is(err, errComposeInterrupted) {
			return err
		}
		if err != nil {
			f.warn(err)
			return nil
		}
		if plan.NoChange {
			fmt.Fprintln(f.out, f.Text.NoChange)
			return nil
		}
		again, err := f.review(plan)
		if err != nil || !again {
			return err
		}
		working = plan.Composition
	}
}

// review previews plan and asks the confirmation, whose default cancels. It
// reports whether the operator keeps editing.
func (f *composeFormRun) review(plan composeDriverPlan) (bool, error) {
	if err := f.preview(plan); err != nil {
		return false, err
	}
	choice, err := composeSelect(f.cmd, f.form.Confirm, "", []composeOption[int]{
		{f.form.ConfirmCancel, 0}, {f.form.ConfirmApply, 1}, {f.form.ConfirmEdit, 2},
	}, 0)
	switch {
	case err != nil:
		return false, err
	case choice == 1:
		f.apply(plan)
	case choice == 2:
		return true, nil
	default:
		fmt.Fprintln(f.out, f.Text.Cancelled)
	}
	return false, nil
}

// apply performs a confirmed write. The writer leaves the file and the
// config as it found them on refusal or failure (Write Safety Contract), so
// a warning line is all that is left to do.
func (f *composeFormRun) apply(plan composeDriverPlan) {
	if plan.Apply == nil {
		f.warn(errors.New("composition plan carries no write"))
		return
	}
	if err := plan.Apply(); err != nil {
		f.warn(err)
		return
	}
	f.applied = true
	fmt.Fprintln(f.out, f.Text.Applied)
}

// choosePreset asks quality.default among the configured presets, the
// current default first selected, and writes another choice through the
// line writer on a detached copy: cfg follows only a successful write. A
// default that names no configured preset, such as an empty one, gets a
// keep option of its own, so Enter never writes the first preset.
func (f *composeFormRun) choosePreset() error {
	current := f.Config.Quality.Default
	var options []composeOption[string]
	if _, configured := f.Config.Quality.Presets[current]; !configured {
		shown := current
		if shown == "" {
			shown = f.form.PresetNone
		}
		options = append(options, composeOption[string]{f.form.Keep + " — " + shown, current})
	}
	for _, name := range orderedQualityPresets(f.Config) {
		label := name
		if description := strings.TrimSpace(f.Config.Quality.Presets[name].Description); description != "" {
			label += " — " + description
		}
		if name == current {
			label += " " + f.form.PresetCurrent
		}
		options = append(options, composeOption[string]{label, name})
	}
	choice, err := composeSelect(f.cmd, f.form.Preset, "", options, current)
	if err != nil || choice == current {
		return err
	}
	plan := composeBindingPlan{Writer: composeWriterLine, Default: choice}
	if err := persistComposeBinding(f.shared.dir, f.shared.cfg, plan); err != nil {
		f.warn(err)
		return nil
	}
	f.applied = true
	fmt.Fprintf(f.out, "quality.default = %s\n", terminalSafe(choice))
	return nil
}

func (f *composeFormRun) warn(err error) {
	warnUpdateCompose(f.out, f.shared.lang, err)
}
