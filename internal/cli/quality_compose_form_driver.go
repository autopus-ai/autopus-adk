package cli

import (
	"errors"
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/insajin/autopus-adk/internal/cli/tui"
)

// composeFormOptions is what an entry point adds to the shared form flow.
type composeFormOptions struct {
	// Presets offers 기본 프리셋 고르기, which only `auto quality` has.
	Presets bool
	// OMP is the run's OMP tab; nil leaves OMP out of the tool tab.
	OMP *composeOMPTab
	// BackOut makes Esc on the menu leave the step with tui.ErrNavBack, as
	// init's goes back to its wizard; otherwise the menu is a first form.
	BackOut bool
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
	// reviewResult is the line that ended the review tab, printed again under
	// the next menu table; entering any tab drops it.
	reviewResult string
	// nav is the run's position for each form's frame (SPEC-TUIUX-001).
	nav *composeNav
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
	f := &composeFormRun{composeDriverState: s.start(cmd), cmd: cmd, form: composeFormTextFor(s.shared.lang), options: options,
		nav: &composeNav{command: cmd.Name(), lang: s.shared.lang, backOut: options.BackOut}}
	defer f.nav.attach(cmd)()
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
// and asks the menu until the operator keeps. Esc in a menu choice comes
// back here; Esc on the menu itself leaves the step only with BackOut, since
// the navigation helper keeps a first form open.
func (f *composeFormRun) menu() error {
	for {
		if err := f.draw(); err != nil {
			return err
		}
		f.writeReviewSummary()
		choice, err := composeSelect(f.cmd, f.form.Menu, "", f.menuOptions(), composeFormKeep)
		switch {
		case errors.Is(err, tui.ErrNavBack) && !f.options.BackOut:
			continue
		case err != nil:
			return err
		case choice == composeFormKeep:
			return nil
		}
		if err := f.menuChoice(choice); err != nil && !errors.Is(err, tui.ErrNavBack) {
			return err
		}
	}
}

// menuChoice runs one menu choice as a stage of the header path.
func (f *composeFormRun) menuChoice(choice composeFormChoice) error {
	switch choice {
	case composeFormRefresh:
		defer f.nav.enter(f.form.MenuRefresh)()
		return f.refresh()
	case composeFormEdit:
		defer f.nav.enter(f.form.MenuEdit)()
		return f.edit()
	}
	defer f.nav.enter(f.form.MenuPreset)()
	return f.choosePreset()
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
	defer f.nav.enter(f.form.TabAll)()
	return f.compose(f.withOverrideQuestion(planner), newComposeAllToolsLabeler(planner), plan.Composition)
}

// withOverrideQuestion asks, before the preview of a 모든 도구 같게 edit,
// whether each provider override follows it (REQ-MUX2-18).
func (f *composeFormRun) withOverrideQuestion(planner *composeSessionPlanner) composeDriverPlanner {
	return composeOverridePlanner{composeSessionPlanner: planner, cmd: f.cmd, form: true}
}

// edit asks the tool tab and runs the chosen tab from its start. Esc on a
// tab's first form asks the tool tab again with that tab selected; Esc on
// the tool tab, or in a tab the tool tab did not ask, returns to the menu.
func (f *composeFormRun) edit() error {
	last := ""
	for {
		tab, asked, err := f.chooseTab(last)
		if err != nil {
			return err
		}
		if err := f.runTab(tab); !errors.Is(err, tui.ErrNavBack) || !asked {
			return err
		}
		last = tab.value
	}
}

// runTab runs one tab as a stage of the header path.
func (f *composeFormRun) runTab(tab composeOption[string]) error {
	defer f.nav.enter(tab.label)()
	f.reviewResult = ""
	switch tab.value {
	case "omp":
		return f.omp()
	case composeReviewTabValue:
		return f.reviewTab()
	}
	for _, tool := range qualityModelTools {
		if tool.provider != "" && tool.platform == tab.value {
			planner, labeler := newComposeToolTab(f.shared, tool)
			return f.compose(planner, labeler, planner.Start())
		}
	}
	planner := f.allToolsPlanner()
	return f.compose(f.withOverrideQuestion(planner), newComposeAllToolsLabeler(planner), planner.Start())
}

// chooseTab asks the tool tab (REQ-MUX2-04): 모든 도구 같게, whose answer is
// "", then Claude Code and Codex when the display view installs them, OMP
// by the OMP Tab Contract, and last the review tab (SPEC-REVMODEL-001
// REQ-RVM-01). OpenCode and Antigravity bind no model and never appear, and
// a tab of one option is skipped. last, the tab Esc came back from, is
// selected; it reports whether it asked.
func (f *composeFormRun) chooseTab(last string) (composeOption[string], bool, error) {
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
	options = append(options, f.composeReviewTabOption()...)
	if len(options) == 1 {
		return options[0], false, nil
	}
	value, err := composeSelect(f.cmd, f.form.Tab, "", options, last)
	index := slices.IndexFunc(options, func(option composeOption[string]) bool { return option.value == value })
	return options[max(index, 0)], true, err
}

// compose runs one tab from working: the method, the edit, the preview, and
// the confirmation, back at the method while the operator keeps editing
// (REQ-MUX2-09). An edit equal to the tab's start writes nothing. Esc on the
// method leaves the tab; Esc on an edit pass's first form drops the pass and
// asks the method again with that method selected.
func (f *composeFormRun) compose(planner composeDriverPlanner, labeler composeLabeler, working map[string]string) error {
	method := false
	for {
		perAgent, err := composeSelect(f.cmd, f.form.Method, "",
			[]composeOption[bool]{{f.form.MethodGroups, false}, {f.form.MethodAgents, true}}, method)
		if err != nil {
			return err
		}
		label := f.form.MethodGroups
		if perAgent {
			label = f.form.MethodAgents
		}
		leave := f.nav.enter(label)
		next, again, err := f.composePass(planner, labeler, working, perAgent)
		leave()
		if errors.Is(err, tui.ErrNavBack) {
			method = perAgent
			continue
		}
		if err != nil || !again {
			return err
		}
		working, method = next, false
	}
}

func (f *composeFormRun) warn(err error) {
	warnUpdateCompose(f.out, f.shared.lang, err)
}
