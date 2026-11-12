package tui

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/huh"
)

// NavRunner runs one navigation form with its frame (REQ-TUX-16).
type NavRunner func(form *huh.Form, frame NavFrame) error

// The wizard's step kinds, in buildStepList order.
const (
	wizardStepProfile     = "profile"
	wizardStepLang        = "lang"
	wizardStepQuality     = "quality"
	wizardStepReviewGate  = "reviewgate"
	wizardStepRea         = "rea"
	wizardStepMethodology = "methodology"
)

// wizardStepKinds lists the steps buildStepList builds for opts, by the same
// skip rules, so the navigation loop can name each step.
func wizardStepKinds(opts InitWizardOpts) []string {
	var kinds []string
	if opts.ExistingProfile == "" {
		kinds = append(kinds, wizardStepProfile)
	}
	kinds = append(kinds, wizardStepLang)
	if opts.Quality == "" {
		kinds = append(kinds, wizardStepQuality)
	}
	if !opts.NoReviewGate {
		kinds = append(kinds, wizardStepReviewGate)
	}
	if reaStepWanted(opts) {
		kinds = append(kinds, wizardStepRea)
	}
	return append(kinds, wizardStepMethodology)
}

// wizardStepName is a step's name in the header and the summary: Korean for
// ko, otherwise the English title without its [n/N] (Summary Contract).
func wizardStepName(kind, lang string) string {
	if kind == wizardStepRea {
		return reaStepTextFor(lang).Title
	}
	names := map[string][2]string{
		wizardStepProfile:     {"사용 프로필", "Usage Profile"},
		wizardStepLang:        {"언어 설정", "Language Settings"},
		wizardStepQuality:     {"품질 모드", "Quality Mode"},
		wizardStepReviewGate:  {"리뷰 게이트", "Review Gate"},
		wizardStepMethodology: {"방법론", "Methodology"},
	}[kind]
	if lang == "ko" {
		return names[0]
	}
	return names[1]
}

// wizardChoiceLabels are the summary values of the profile and methodology
// options: their labels in buildProfileStep and buildMethodologyStep before
// " — ".
var wizardChoiceLabels = map[string]string{
	"developer": "Developer", "fullstack": "Fullstack", "tdd": "TDD", "none": "None",
}

// wizardStepValue is the summary value r holds for a step.
func wizardStepValue(kind string, r *InitWizardResult, lang string) string {
	switch kind {
	case wizardStepProfile:
		return wizardChoice(r.UsageProfile)
	case wizardStepLang:
		if lang == "ko" {
			return fmt.Sprintf("주석 %s · 커밋 %s · AI 응답 %s", r.CommentsLang, r.CommitsLang, r.AILang)
		}
		return fmt.Sprintf("comments %s · commits %s · AI responses %s", r.CommentsLang, r.CommitsLang, r.AILang)
	case wizardStepQuality:
		for _, option := range qualityOptions(lang, nil) {
			if option.Value == r.Quality {
				label, _, _ := strings.Cut(option.Key, " — ")
				return label
			}
		}
		return r.Quality
	case wizardStepReviewGate:
		return wizardOnOff(r.ReviewGate, lang)
	case wizardStepRea:
		// No answer leaves init's default, which is on (SPEC-REA-001).
		return wizardOnOff(r.Rea == nil || *r.Rea, lang)
	}
	return wizardChoice(r.Methodology)
}

func wizardChoice(value string) string {
	if label, ok := wizardChoiceLabels[value]; ok {
		return label
	}
	return value
}

func wizardOnOff(on bool, lang string) string {
	switch {
	case lang == "ko" && on:
		return "켬"
	case lang == "ko":
		return "끔"
	case on:
		return "on"
	}
	return "off"
}

// wizardStepLine is a step's summary line.
func wizardStepLine(kind string, r *InitWizardResult, lang string) string {
	return "  " + wizardStepName(kind, lang) + ": " + wizardStepValue(kind, r, lang)
}

// WizardAnswerLines are the answer lines of every step the wizard asks for
// opts, in step order: the init review's answers (Init Review Contract).
func WizardAnswerLines(opts InitWizardOpts, r *InitWizardResult, lang string) []string {
	var lines []string
	for _, kind := range wizardStepKinds(opts) {
		lines = append(lines, wizardStepLine(kind, r, lang))
	}
	return lines
}

// cloneWizardResult copies r with its own REA answer, so a saved state never
// shares the pointer a REA form writes through.
func cloneWizardResult(r InitWizardResult) InitWizardResult {
	if r.Rea != nil {
		rea := *r.Rea
		r.Rea = &rea
	}
	return r
}

// wizardStreams are the default runner's streams, huh's own defaults: the
// accessible mode writes to os.Stdout and the TUI to os.Stderr.
func wizardStreams(accessible bool) (io.Reader, io.Writer) {
	if accessible {
		return os.Stdin, os.Stdout
	}
	return os.Stdin, os.Stderr
}

func runWizardForm(form *huh.Form, frame NavFrame) error {
	in, out := wizardStreams(frame.Accessible)
	return RunNav(form, frame, in, out)
}

// runWizardSteps runs the wizard's steps as one navigation flow (Navigation
// Contract, init wizard rows): Esc on a step after the first restores the
// answers saved before it and shows the step before, whose answer is
// preselected; the first step stays with its notice. A resume result starts
// the flow at the last step with every earlier step answered. Any other form
// error, Ctrl+C included, cancels the wizard as before.
func runWizardSteps(opts InitWizardOpts, result *InitWizardResult) (*InitWizardResult, error) {
	lang := DetectSystemLang()
	kinds := wizardStepKinds(opts)
	steps := buildStepList(opts)
	run := opts.Runner
	if run == nil {
		run = runWizardForm
	}
	var history NavHistory[InitWizardResult]
	step := 0
	if opts.Resume != nil {
		*result = cloneWizardResult(*opts.Resume)
		for ; step < len(steps)-1; step++ {
			history.Show(cloneWizardResult(*result))
			history.Answer(wizardStepLine(kinds[step], result, lang))
		}
	}
	for step < len(steps) {
		if history.Len() == step {
			history.Show(cloneWizardResult(*result))
		}
		frame := NavFrame{
			Header:     fmt.Sprintf("init · %d/%d %s", step+1, len(steps), wizardStepName(kinds[step], lang)),
			Summary:    NavSummary(lang, history.Summary()),
			First:      step == 0,
			Lang:       lang,
			Accessible: NavAccessible(),
		}
		err := run(steps[step](result), frame)
		switch {
		case err == nil:
			history.Answer(wizardStepLine(kinds[step], result, lang))
			step++
		case errors.Is(err, ErrNavBack):
			if saved, ok := history.Back(); ok {
				*result = cloneWizardResult(saved)
				step--
			}
		default:
			return &InitWizardResult{Cancelled: true}, nil
		}
	}
	return result, nil
}
