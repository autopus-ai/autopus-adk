package cli

import (
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/insajin/autopus-adk/pkg/config"
)

// composeToolText is the change-line wording of a tool tab in one locale.
// Only the terminal sees it; nothing written depends on it.
type composeToolText struct {
	provider   string // provider, before, after
	none       string
	givesUp    string // preset name
	oldReaders string
	// The efforts lines of SPEC-MODELUX-003 REQ-MUX3-04: preset name and
	// agent count, and the reader line of a file's first efforts mapping.
	effortsNew, effortsRewrite, effortsRemoved, effortReaders string
}

var composeToolTexts = map[string]composeToolText{
	"ko": {
		provider:       "quality.providers.%s %s → %s",
		none:           "없음",
		givesUp:        "%s은 balanced 표준 배치를 받지 않음",
		oldReaders:     "quality.providers는 v0.50.88보다 오래된 auto가 읽지 않습니다",
		effortsNew:     "quality.presets.%s.efforts 새로 씀 (%d개 에이전트)",
		effortsRewrite: "quality.presets.%s.efforts 고쳐 씀 (%d개 에이전트)",
		effortsRemoved: "quality.presets.%s.efforts 지움",
		effortReaders:  "quality.presets.*.efforts는 v0.50.126보다 오래된 auto가 읽지 못합니다",
	},
	"en": {
		provider:       "quality.providers.%s %s → %s",
		none:           "none",
		givesUp:        "%s does not get the balanced standard placement",
		oldReaders:     "auto older than v0.50.88 does not read quality.providers",
		effortsNew:     "quality.presets.%s.efforts written (%d agents)",
		effortsRewrite: "quality.presets.%s.efforts rewritten (%d agents)",
		effortsRemoved: "quality.presets.%s.efforts removed",
		effortReaders:  "auto older than v0.50.126 cannot read quality.presets.*.efforts",
	},
}

func composeToolTextFor(lang string) composeToolText {
	if lang == "ko" {
		return composeToolTexts["ko"]
	}
	return composeToolTexts["en"]
}

// composeToolPlanner turns a Claude Code or Codex tab's edits into
// Per-Tool Binding Contract writes (SPEC-MODELUX-002 REQ-MUX2-07). It is a
// composeDriverPlanner, so the form driver runs both kinds of tab alike.
type composeToolPlanner struct {
	run  *composeSessionRun
	tool qualityModelTool
	// effort is the tab's effort state (SPEC-MODELUX-003); without one the
	// planner plans the efforts the tool's preset stores.
	effort *composeEffortTab
}

func (p composeToolPlanner) Start() map[string]string { return p.run.toolStart(p.tool) }

// PlanEdit plans edited: nothing when it and the working efforts equal the
// start, otherwise <provider>-custom holding every canonical agent's tier
// and the working composition's explicit efforts, and the tool's binding to
// it. quality.default and every other preset stay.
func (p composeToolPlanner) PlanEdit(edited map[string]string) (composeDriverPlan, error) {
	start, working := p.efforts()
	if maps.Equal(edited, p.Start()) && maps.Equal(working, start) {
		return composeDriverPlan{NoChange: true}, nil
	}
	source, err := p.run.source()
	if err != nil {
		return composeDriverPlan{}, err
	}
	name := qualityToolPresetName(p.tool)
	agents, efforts := maps.Clone(edited), maps.Clone(working)
	candidate := withQualityToolPresetEfforts(p.run.view(), p.tool, name, agents, efforts)
	return composeDriverPlan{
		Candidate:   candidate,
		Composition: maps.Clone(agents),
		Changes:     p.changes(name, candidate, source, efforts),
		Apply:       func() error { return p.persist(name, agents, efforts) },
	}, nil
}

// efforts returns the efforts the tab started from and the ones its working
// composition holds.
func (p composeToolPlanner) efforts() (start, working map[string]string) {
	if p.effort == nil {
		stored := qualityToolEfforts(p.run.cfg.Quality, p.tool)
		return stored, stored
	}
	return p.effort.start, p.effort.working
}

// PlanRefresh has no meaning on a tool tab: `u` refreshes the built-in
// presets every tool shares.
func (p composeToolPlanner) PlanRefresh() (composeDriverPlan, error) {
	return composeDriverPlan{}, errors.New("a tool tab does not refresh the built-in presets")
}

// changes lists the preview lines of the write: the preset write, its
// efforts when they change, the binding when it moves, the Codex default
// session when it moves, what a balanced tool gives up, the block rewrite of
// a file write, the version that started reading quality.providers, and the
// one that started reading efforts when this write adds the file's first.
func (p composeToolPlanner) changes(name string, candidate *config.HarnessConfig, source []byte, efforts map[string]string) []string {
	quality := p.run.cfg.Quality
	bind := composeBindTextFor(p.run.lang)
	text := composeToolTextFor(p.run.lang)
	kind := composePresetNew
	if _, exists := quality.Presets[name]; exists {
		kind = composePresetOverwrite
		if quality.EffectiveMode(p.tool.provider) == name {
			kind = composePresetRewrite
		}
	}
	lines := []string{fmt.Sprintf(bind.presetKinds[kind], name, len(config.CanonicalAgentNames()))}
	if line := text.effortsLine(name, quality.Presets[name].Efforts, efforts); line != "" {
		lines = append(lines, line)
	}
	if before := strings.TrimSpace(quality.Providers[p.tool.provider]); before != name {
		if before == "" {
			before = text.none
		}
		lines = append(lines, fmt.Sprintf(text.provider, p.tool.provider, terminalSafe(before), name))
	}
	if from, to, moved := composeSupervisorMove(p.run.view(), candidate, p.run.snapshot); moved {
		lines = append(lines, fmt.Sprintf(bind.supervisor, from, to))
	}
	if quality.EffectiveMode(p.tool.provider) == "balanced" {
		lines = append(lines, fmt.Sprintf(text.givesUp, name))
	}
	if source != nil {
		if comments, ok := qualityBlockCommentLines(source); ok {
			lines = append(lines, fmt.Sprintf(bind.blockRewrite, comments))
		} else {
			lines = append(lines, bind.blockUnreadable)
		}
	}
	lines = append(lines, text.oldReaders)
	if len(efforts) > 0 && !composeQualityHasEfforts(quality) {
		lines = append(lines, text.effortReaders)
	}
	return lines
}

// effortsLine is the change line of a preset's efforts going from before to
// after, "" when they stay.
func (t composeToolText) effortsLine(name string, before, after map[string]string) string {
	switch {
	case maps.Equal(before, after):
		return ""
	case len(after) == 0:
		return fmt.Sprintf(t.effortsRemoved, name)
	case len(before) == 0:
		return fmt.Sprintf(t.effortsNew, name, len(after))
	default:
		return fmt.Sprintf(t.effortsRewrite, name, len(after))
	}
}

// composeQualityHasEfforts reports whether any preset stores an efforts
// mapping, which auto older than v0.50.126 cannot read.
func composeQualityHasEfforts(quality config.QualityConf) bool {
	for _, preset := range quality.Presets {
		if len(preset.Efforts) > 0 {
			return true
		}
	}
	return false
}

// persist writes the confirmed tiers and efforts: through
// saveQualityToolPresetEfforts into autopus.yaml, which refuses and keeps
// the file and cfg as they were on any failure, or for init into its
// config, which init then saves whole.
func (p composeToolPlanner) persist(name string, agents, efforts map[string]string) error {
	if p.run.writer == composeSessionMemory {
		setQualityToolPreset(&p.run.cfg.Quality, p.tool, name, agents, efforts)
		return nil
	}
	return saveQualityToolPresetEfforts(p.run.dir, p.run.cfg, p.tool, name, agents, efforts)
}

// toolStart is the starting composition of tool's tab (SPEC-MODELUX-002
// Per-Tool Binding Contract): every canonical agent at the tier the tool's
// provider view projects, its override preset when it has one and the
// global binding otherwise, with the fallback tier its embedded source
// declares.
func (r *composeSessionRun) toolStart(tool qualityModelTool) map[string]string {
	view := r.cfg.Quality.ForProvider(tool.provider)
	agents := config.CanonicalAgentNames()
	start := make(map[string]string, len(agents))
	for _, agent := range agents {
		start[agent] = view.EffectiveAgentTier(agent, r.sources[agent].FallbackTier)
	}
	return start
}

// toolCandidate returns the candidate function of tool's tab with the
// efforts its preset stores: a composition equal to the start leaves the
// loaded config as it is, and any other binds the tool to <provider>-custom
// holding it, in a detached copy built by withQualityToolPresetEfforts.
func (r *composeSessionRun) toolCandidate(tool qualityModelTool) func(map[string]string) *config.HarnessConfig {
	_, labeler := newComposeToolTab(r, tool)
	return labeler.candidate
}
